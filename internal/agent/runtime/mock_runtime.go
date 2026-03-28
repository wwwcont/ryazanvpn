package runtime

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type operationRecord struct {
	action         string
	deviceAccessID string
}

type MockRuntime struct {
	logger *slog.Logger

	mu         sync.RWMutex
	operations map[string]operationRecord
	peers      map[string]PeerState
}

func NewMockRuntime(logger *slog.Logger) *MockRuntime {
	return &MockRuntime{
		logger:     logger,
		operations: make(map[string]operationRecord),
		peers:      make(map[string]PeerState),
	}
}

func (m *MockRuntime) ApplyPeer(_ context.Context, req PeerOperationRequest) (OperationResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if rec, ok := m.operations[req.OperationID]; ok {
		if rec.action != "apply" || rec.deviceAccessID != req.DeviceAccessID {
			return OperationResult{}, ErrOperationConflict
		}
		return OperationResult{OperationID: req.OperationID, Applied: true, Idempotent: true}, nil
	}

	m.operations[req.OperationID] = operationRecord{action: "apply", deviceAccessID: req.DeviceAccessID}
	m.peers[req.DeviceAccessID] = PeerState{
		DeviceAccessID: req.DeviceAccessID,
		Protocol:       req.Protocol,
		PeerPublicKey:  req.PeerPublicKey,
		AssignedIP:     req.AssignedIP,
		Keepalive:      req.Keepalive,
		EndpointMeta:   req.EndpointMeta,
		UpdatedAt:      time.Now().UTC(),
	}

	m.logger.Info("mock runtime apply peer",
		slog.String("operation_id", req.OperationID),
		slog.String("device_access_id", req.DeviceAccessID),
		slog.String("assigned_ip", req.AssignedIP),
	)

	return OperationResult{OperationID: req.OperationID, Applied: true, Idempotent: false}, nil
}

func (m *MockRuntime) RevokePeer(_ context.Context, req PeerOperationRequest) (OperationResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if rec, ok := m.operations[req.OperationID]; ok {
		if rec.action != "revoke" || rec.deviceAccessID != req.DeviceAccessID {
			return OperationResult{}, ErrOperationConflict
		}
		return OperationResult{OperationID: req.OperationID, Applied: true, Idempotent: true}, nil
	}

	m.operations[req.OperationID] = operationRecord{action: "revoke", deviceAccessID: req.DeviceAccessID}
	delete(m.peers, req.DeviceAccessID)

	m.logger.Info("mock runtime revoke peer",
		slog.String("operation_id", req.OperationID),
		slog.String("device_access_id", req.DeviceAccessID),
	)

	return OperationResult{OperationID: req.OperationID, Applied: true, Idempotent: false}, nil
}

func (m *MockRuntime) Health(context.Context) error {
	return nil
}

func (m *MockRuntime) ListPeerStats(context.Context) ([]PeerStat, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]PeerStat, 0, len(m.peers))
	for _, p := range m.peers {
		t := p.UpdatedAt
		out = append(out, PeerStat{DeviceAccessID: p.DeviceAccessID, RXTotalBytes: 0, TXTotalBytes: 0, LastHandshakeAt: &t})
	}
	return out, nil
}

func (m *MockRuntime) PeerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.peers)
}
