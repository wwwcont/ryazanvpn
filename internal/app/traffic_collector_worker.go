package app

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"strings"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/domain/access"
	"github.com/wwwcont/ryazanvpn/internal/domain/traffic"
)

var errAccessNotResolved = errors.New("access not resolved by id or assigned ip")

type TrafficClientFactory interface {
	ForNode(agentBaseURL string) NodeTrafficClient
}

type TrafficCollectorWorker struct {
	Logger        *slog.Logger
	Nodes         NodeRepository
	Accesses      DeviceAccessRepository
	Traffic       TrafficRepository
	ClientFactory TrafficClientFactory
	PollInterval  time.Duration
	SampleStep    time.Duration
	SampleRetention time.Duration
	Now           func() time.Time
}

func (w TrafficCollectorWorker) Run(ctx context.Context) {
	interval := w.PollInterval
	if interval <= 0 {
		interval = 60 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	w.collect(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.collect(ctx)
		}
	}
}

func (w TrafficCollectorWorker) collect(ctx context.Context) {
	nodes, err := w.Nodes.ListActive(ctx)
	if err != nil {
		w.logErr("list active nodes", err)
		return
	}
	now := time.Now().UTC()
	if w.Now != nil {
		now = w.Now().UTC()
	}

	for _, n := range nodes {
		cli := w.ClientFactory.ForNode(n.AgentBaseURL)
		counters, err := cli.GetTrafficCounters(ctx)
		if err != nil {
			w.logErr("get traffic counters", err)
			continue
		}
		var nodeRXDelta int64
		var nodeTXDelta int64
		resolvedPeers := 0
		for _, c := range counters {
			acc, err := w.resolveAccess(ctx, n.ID, c)
			if err != nil {
				if errors.Is(err, access.ErrNotFound) || errors.Is(err, errAccessNotResolved) {
					w.logInfo("resolve access skipped", "node_id", n.ID, "access_id", c.DeviceAccessID, "allowed_ip", c.AllowedIP, "protocol", valueOrDefault(c.Protocol, "wireguard"))
					continue
				}
				w.logErr("resolve access", err)
				continue
			}
			resolvedPeers++
			prev, err := w.Traffic.GetLastSnapshotByDeviceID(ctx, acc.DeviceID, now)
			if err != nil {
				w.logErr("get previous snapshot", err)
				continue
			}
			if _, err := w.Traffic.CreateSnapshot(ctx, traffic.CreateSnapshotParams{
				DeviceID:        acc.DeviceID,
				DeviceAccessID:  &acc.AccessID,
				VPNNodeID:       &n.ID,
				Protocol:        valueOrDefault(c.Protocol, "wireguard"),
				CapturedAt:      now,
				RXTotalBytes:    c.RXTotalBytes,
				TXTotalBytes:    c.TXTotalBytes,
				LastHandshakeAt: c.LastHandshakeAt,
			}); err != nil {
				w.logErr("create snapshot", err)
				continue
			}

			rxDelta := c.RXTotalBytes
			txDelta := c.TXTotalBytes
			if prev != nil {
				rxDelta = c.RXTotalBytes - prev.RXTotalBytes
				txDelta = c.TXTotalBytes - prev.TXTotalBytes
				if rxDelta < 0 {
					rxDelta = c.RXTotalBytes
				}
				if txDelta < 0 {
					txDelta = c.TXTotalBytes
				}
			}
			if err := w.Traffic.AddDailyUsageDelta(ctx, traffic.AddDailyUsageDeltaParams{
				DeviceID:  acc.DeviceID,
				VPNNodeID: &n.ID,
				Protocol:  valueOrDefault(c.Protocol, "wireguard"),
				UsageDate: now,
				MonthDate: time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC),
				RXDelta:   rxDelta,
				TXDelta:   txDelta,
			}); err != nil {
				w.logErr("add daily usage delta", err)
			}
			nodeRXDelta += rxDelta
			nodeTXDelta += txDelta
		}
		stepSec := int64(w.SampleStep.Seconds())
		if stepSec <= 0 {
			stepSec = 60
		}
		windowSec := int64(w.PollInterval.Seconds())
		if windowSec <= 0 {
			windowSec = stepSec
		}
		if err := w.Traffic.AddNodeThroughputSample(ctx, traffic.AddNodeThroughputSampleParams{
			NodeID:        n.ID,
			CapturedAt:    now,
			StepSec:       stepSec,
			WindowSec:     windowSec,
			RXDelta:       nodeRXDelta,
			TXDelta:       nodeTXDelta,
			PeerCount:     len(counters),
			ResolvedPeers: resolvedPeers,
		}); err != nil {
			w.logErr("add node throughput sample", err)
		}
	}
	w.cleanupOldNodeSamples(ctx, now)
}

func (w TrafficCollectorWorker) cleanupOldNodeSamples(ctx context.Context, now time.Time) {
	retention := w.SampleRetention
	if retention <= 0 {
		retention = 48 * time.Hour
	}
	if err := w.Traffic.CleanupNodeThroughputSamples(ctx, now.Add(-retention)); err != nil {
		w.logErr("cleanup node throughput samples", err)
	}
}

func (w TrafficCollectorWorker) resolveAccess(ctx context.Context, nodeID string, c NodeTrafficCounter) (*trafficAccess, error) {
	publicKey := strings.TrimSpace(c.PeerPublicKey)
	if publicKey != "" && !strings.EqualFold(publicKey, "(none)") {
		accByKey, keyErr := w.Accesses.GetActiveByNodeAndPublicKey(ctx, nodeID, publicKey)
		if keyErr == nil && accByKey != nil {
			return &trafficAccess{DeviceID: accByKey.DeviceID, AccessID: accByKey.ID}, nil
		}
		if keyErr != nil && !errors.Is(keyErr, access.ErrNotFound) {
			return nil, keyErr
		}
	}

	allowedIP := strings.TrimSpace(c.AllowedIP)
	if i := strings.IndexByte(allowedIP, '/'); i > 0 {
		allowedIP = allowedIP[:i]
	}
	if strings.EqualFold(allowedIP, "(none)") {
		allowedIP = ""
	}
	if allowedIP != "" {
		if _, parseErr := netip.ParseAddr(allowedIP); parseErr != nil {
			return nil, parseErr
		}
		accByIP, ipErr := w.Accesses.GetActiveByNodeAndAssignedIP(ctx, nodeID, allowedIP)
		if ipErr == nil && accByIP != nil {
			return &trafficAccess{DeviceID: accByIP.DeviceID, AccessID: accByIP.ID}, nil
		}
		if ipErr != nil && !errors.Is(ipErr, access.ErrNotFound) {
			return nil, ipErr
		}
	}
	accessID := strings.TrimSpace(c.DeviceAccessID)
	if accessID != "" && !strings.EqualFold(accessID, "(none)") {
		acc, err := w.Accesses.GetByID(ctx, accessID)
		if err == nil && acc != nil {
			return &trafficAccess{DeviceID: acc.DeviceID, AccessID: acc.ID}, nil
		}
		if err != nil && !errors.Is(err, access.ErrNotFound) {
			return nil, err
		}
	}
	return nil, errAccessNotResolved
}

type trafficAccess struct {
	DeviceID string
	AccessID string
}

func (w TrafficCollectorWorker) logErr(msg string, err error) {
	if w.Logger != nil {
		w.Logger.Error(msg, slog.Any("error", err))
	}
}

func (w TrafficCollectorWorker) logInfo(msg string, kv ...string) {
	if w.Logger == nil {
		return
	}
	attrs := make([]any, 0, len(kv))
	for i := 0; i+1 < len(kv); i += 2 {
		attrs = append(attrs, slog.String(kv[i], kv[i+1]))
	}
	w.Logger.Info(msg, attrs...)
}
