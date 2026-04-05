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
	Reconciler    PeerConsistencyReconciler
	PollInterval  time.Duration
}

type PeerConsistencyReconciler interface {
	ReconcileMissingPeer(ctx context.Context, nodeID string, counter NodeTrafficCounter) (bool, error)
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
		w.compareNodePeers(ctx, n.ID, dbPeers, runtimeCounters)
	}
}

func (w PeerConsistencyWorker) compareNodePeers(ctx context.Context, nodeID string, dbPeers []*access.DeviceAccess, runtimePeers []NodeTrafficCounter) {
	dbByAccessID := make(map[string]*access.DeviceAccess, len(dbPeers))
	dbByIP := make(map[string]*access.DeviceAccess, len(dbPeers))
	filtered := make([]*access.DeviceAccess, 0, len(dbPeers))
	for _, p := range dbPeers {
		if p == nil || strings.EqualFold(strings.TrimSpace(p.Protocol), "xray") {
			continue
		}
		filtered = append(filtered, p)
		dbByAccessID[p.ID] = p
		if p.AssignedIP != nil && strings.TrimSpace(*p.AssignedIP) != "" {
			dbByIP[*p.AssignedIP] = p
		}
	}

	seenAccessIDs := make(map[string]struct{}, len(runtimePeers))
	controlPlaneMissing := 0
	runtimeMissing := 0
	staleNodeRefs := 0
	reconciledStale := 0
	for _, rp := range runtimePeers {
		if strings.EqualFold(strings.TrimSpace(rp.Protocol), "xray") {
			continue
		}
		id := strings.TrimSpace(rp.DeviceAccessID)
		if id != "" {
			if _, ok := dbByAccessID[id]; !ok {
				found, err := w.Accesses.GetByID(ctx, id)
				if err == nil && found != nil && found.VPNNodeID != nodeID {
					staleNodeRefs++
					continue
				}
				controlPlaneMissing++
				if w.tryReconcileMissing(ctx, nodeID, rp) {
					reconciledStale++
				}
				continue
			}
			seenAccessIDs[id] = struct{}{}
			continue
		}

		if strings.TrimSpace(rp.AllowedIP) == "" {
			if pubKey := strings.TrimSpace(rp.PeerPublicKey); pubKey != "" {
				if byKey, err := w.Accesses.GetActiveByNodeAndPublicKey(ctx, nodeID, pubKey); err == nil && byKey != nil {
					seenAccessIDs[byKey.ID] = struct{}{}
					continue
				}
			}
			controlPlaneMissing++
			if w.tryReconcileMissing(ctx, nodeID, rp) {
				reconciledStale++
			}
			continue
		}
		if matched := dbByIP[rp.AllowedIP]; matched == nil {
			if pubKey := strings.TrimSpace(rp.PeerPublicKey); pubKey != "" {
				if byKey, err := w.Accesses.GetActiveByNodeAndPublicKey(ctx, nodeID, pubKey); err == nil && byKey != nil {
					seenAccessIDs[byKey.ID] = struct{}{}
					continue
				}
			}
			controlPlaneMissing++
			if w.tryReconcileMissing(ctx, nodeID, rp) {
				reconciledStale++
			}
		} else {
			seenAccessIDs[matched.ID] = struct{}{}
		}
	}

	for _, p := range filtered {
		if _, ok := seenAccessIDs[p.ID]; ok {
			continue
		}
		runtimeMissing++
	}
	if staleNodeRefs > 0 {
		w.logWarn("peer_consistency.stale_node_reference", "node_id", nodeID, "count", fmt.Sprintf("%d", staleNodeRefs))
	}
	if controlPlaneMissing > 0 {
		w.logWarn("peer_consistency.control_plane_missing", "node_id", nodeID, "count", fmt.Sprintf("%d", controlPlaneMissing), "reconciled", fmt.Sprintf("%d", reconciledStale))
	}
	if runtimeMissing > 0 {
		w.logWarn("peer_consistency.runtime_missing", "node_id", nodeID, "count", fmt.Sprintf("%d", runtimeMissing))
	}
}

func (w PeerConsistencyWorker) tryReconcileMissing(ctx context.Context, nodeID string, counter NodeTrafficCounter) bool {
	if w.Reconciler == nil {
		return false
	}
	ok, err := w.Reconciler.ReconcileMissingPeer(ctx, nodeID, counter)
	if err != nil {
		w.logErr("peer_consistency.reconcile_failed", err)
		return false
	}
	return ok
}

func (w PeerConsistencyWorker) logErr(msg string, err error) {
	if w.Logger != nil {
		w.Logger.Error(msg, slog.Any("error", err), slog.String("error_code", "E_RECONCILE_PEER"))
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
