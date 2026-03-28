package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/domain/traffic"
)

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
		for _, c := range counters {
			acc, err := w.Accesses.GetByID(ctx, c.DeviceAccessID)
			if err != nil {
				w.logErr("get access by id", err)
				continue
			}
			prev, err := w.Traffic.GetLastSnapshotByDeviceID(ctx, acc.DeviceID, now)
			if err != nil {
				w.logErr("get previous snapshot", err)
				continue
			}
			if _, err := w.Traffic.CreateSnapshot(ctx, traffic.CreateSnapshotParams{DeviceID: acc.DeviceID, CapturedAt: now, RXTotalBytes: c.RXTotalBytes, TXTotalBytes: c.TXTotalBytes}); err != nil {
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
			if err := w.Traffic.AddDailyUsageDelta(ctx, traffic.AddDailyUsageDeltaParams{DeviceID: acc.DeviceID, UsageDate: now, RXDelta: rxDelta, TXDelta: txDelta}); err != nil {
				w.logErr("add daily usage delta", err)
			}
		}
	}
}

func (w TrafficCollectorWorker) logErr(msg string, err error) {
	if w.Logger != nil {
		w.Logger.Error(msg, slog.Any("error", err))
	}
}
