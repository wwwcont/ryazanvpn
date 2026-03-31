package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/domain/access"
)

type PeerConsistencyWorker struct {
	Logger        *slog.Logger
	Nodes         NodeRepository
	Accesses      DeviceAccessRepository
	ClientFactory TrafficClientFactory
	PollInterval  time.Duration
}

func (w PeerConsistencyWorker) Run(ctx context.Context) {
	interval := w.PollInterval
	if interval <= 0 {
		interval = 2 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	w.check(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.check(ctx)
		}
	}
}

func (w PeerConsistencyWorker) check(ctx context.Context) {
	nodes, err := w.Nodes.ListActive(ctx)
	if err != nil {
		w.logErr("peer consistency: list active nodes", err)
		return
	}

	for _, n := range nodes {
		dbPeers, err := w.Accesses.ListActiveByNodeID(ctx, n.ID)
		if err != nil {
			w.logErr("peer consistency: list active accesses", err)
			continue
		}

		runtimeCounters, err := w.ClientFactory.ForNode(n.AgentBaseURL).GetTrafficCounters(ctx)
		if err != nil {
			w.logErr("peer consistency: get runtime peers", err)
			continue
		}
		w.compareNodePeers(n.ID, dbPeers, runtimeCounters)
	}
}

func (w PeerConsistencyWorker) compareNodePeers(nodeID string, dbPeers []*access.DeviceAccess, runtimePeers []NodeTrafficCounter) {
	dbByAccessID := make(map[string]*access.DeviceAccess, len(dbPeers))
	dbByIP := make(map[string]*access.DeviceAccess, len(dbPeers))
	for _, p := range dbPeers {
		dbByAccessID[p.ID] = p
		if p.AssignedIP != nil && strings.TrimSpace(*p.AssignedIP) != "" {
			dbByIP[*p.AssignedIP] = p
		}
	}

	seenAccessIDs := make(map[string]struct{}, len(runtimePeers))
	for _, rp := range runtimePeers {
		id := strings.TrimSpace(rp.DeviceAccessID)
		if id != "" {
			if _, ok := dbByAccessID[id]; !ok {
				w.logWarn("peer consistency mismatch: runtime peer missing in control-plane state", "node_id", nodeID, "access_id", id, "allowed_ip", rp.AllowedIP, "public_key", rp.PeerPublicKey)
				continue
			}
			seenAccessIDs[id] = struct{}{}
			continue
		}

		if strings.TrimSpace(rp.AllowedIP) == "" {
			w.logWarn("peer consistency mismatch: runtime peer without access_id and allowed_ip", "node_id", nodeID, "public_key", rp.PeerPublicKey)
			continue
		}
		if matched := dbByIP[rp.AllowedIP]; matched == nil {
			w.logWarn("peer consistency mismatch: runtime peer by ip missing in control-plane state", "node_id", nodeID, "allowed_ip", rp.AllowedIP, "public_key", rp.PeerPublicKey)
		} else {
			seenAccessIDs[matched.ID] = struct{}{}
		}
	}

	for _, p := range dbPeers {
		if _, ok := seenAccessIDs[p.ID]; ok {
			continue
		}
		w.logWarn("peer consistency mismatch: control-plane active peer absent in runtime", "node_id", nodeID, "access_id", p.ID, "assigned_ip", valuePtrOrDefault(p.AssignedIP, ""))
	}
}

func (w PeerConsistencyWorker) logErr(msg string, err error) {
	if w.Logger != nil {
		w.Logger.Error(msg, slog.Any("error", err))
	}
}

func (w PeerConsistencyWorker) logWarn(msg string, kv ...string) {
	if w.Logger == nil {
		slog.Default().Warn(fmt.Sprintf("%s %v", msg, kv))
		return
	}
	attrs := make([]any, 0, len(kv))
	for i := 0; i+1 < len(kv); i += 2 {
		attrs = append(attrs, slog.String(kv[i], kv[i+1]))
	}
	w.Logger.Warn(msg, attrs...)
}
