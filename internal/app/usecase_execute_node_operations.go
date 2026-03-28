package app

import (
	"context"
	"encoding/json"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/domain/operation"
)

type createPeerPayload struct {
	DeviceID   string `json:"device_id"`
	AccessID   string `json:"access_id"`
	PublicKey  string `json:"public_key"`
	AssignedIP string `json:"assigned_ip"`
	Protocol   string `json:"protocol"`
	Keepalive  int    `json:"keepalive"`
}

type revokePeerPayload struct {
	AccessID string `json:"access_id"`
	DeviceID string `json:"device_id"`
}

type ExecuteCreatePeerOperation struct {
	Operations NodeOperationRepository
	Accesses   DeviceAccessRepository
	Nodes      NodeRepository
	NodeClient NodeAgentClient
	Now        func() time.Time
}

func (uc ExecuteCreatePeerOperation) Execute(ctx context.Context, operationID string) error {
	now := time.Now().UTC()
	if uc.Now != nil {
		now = uc.Now().UTC()
	}

	op, err := uc.Operations.GetByID(ctx, operationID)
	if err != nil {
		return err
	}

	if err := uc.Operations.MarkRunning(ctx, op.ID, now); err != nil {
		return err
	}

	var payload createPeerPayload
	if err := json.Unmarshal([]byte(op.PayloadJSON), &payload); err != nil {
		_ = uc.Operations.MarkFailed(ctx, op.ID, now, "invalid payload")
		return err
	}

	accessEntry, err := uc.Accesses.GetByID(ctx, payload.AccessID)
	if err != nil {
		_ = uc.Operations.MarkFailed(ctx, op.ID, now, err.Error())
		return err
	}

	nodeInfo, err := uc.Nodes.GetByID(ctx, op.VPNNodeID)
	if err != nil {
		_ = uc.Operations.MarkFailed(ctx, op.ID, now, err.Error())
		return err
	}

	req := NodeAgentOperationRequest{
		AgentBaseURL:   nodeInfo.AgentBaseURL,
		OperationID:    op.ID,
		DeviceAccessID: accessEntry.ID,
		Protocol:       valueOrDefault(payload.Protocol, "wireguard"),
		PeerPublicKey:  payload.PublicKey,
		AssignedIP:     payload.AssignedIP,
		Keepalive:      valueOrDefaultInt(payload.Keepalive, 25),
	}

	if err := uc.NodeClient.ApplyPeer(ctx, req); err != nil {
		_ = uc.Operations.MarkFailed(ctx, op.ID, now, err.Error())
		_ = uc.Accesses.MarkError(ctx, accessEntry.ID, now, err.Error())
		return err
	}

	if err := uc.Operations.MarkSuccess(ctx, op.ID, now); err != nil {
		return err
	}
	if err := uc.Accesses.Activate(ctx, accessEntry.ID, now); err != nil {
		return err
	}
	if err := uc.updateNodeLoadDelta(ctx, op.VPNNodeID, +1); err != nil {
		return err
	}

	return nil
}

func (uc ExecuteCreatePeerOperation) updateNodeLoadDelta(ctx context.Context, nodeID string, delta int) error {
	n, err := uc.Nodes.GetByID(ctx, nodeID)
	if err != nil {
		return err
	}
	newLoad := n.CurrentLoad + delta
	if newLoad < 0 {
		newLoad = 0
	}
	return uc.Nodes.UpdateLoad(ctx, nodeID, newLoad)
}

type ExecuteRevokePeerOperation struct {
	Operations NodeOperationRepository
	Accesses   DeviceAccessRepository
	Nodes      NodeRepository
	NodeClient NodeAgentClient
	Now        func() time.Time
}

func (uc ExecuteRevokePeerOperation) Execute(ctx context.Context, operationID string, accessID string) error {
	now := time.Now().UTC()
	if uc.Now != nil {
		now = uc.Now().UTC()
	}

	op, err := uc.Operations.GetByID(ctx, operationID)
	if err != nil {
		return err
	}

	if err := uc.Operations.MarkRunning(ctx, op.ID, now); err != nil {
		return err
	}

	accessEntry, err := uc.Accesses.GetByID(ctx, accessID)
	if err != nil {
		_ = uc.Operations.MarkFailed(ctx, op.ID, now, err.Error())
		return err
	}

	nodeInfo, err := uc.Nodes.GetByID(ctx, op.VPNNodeID)
	if err != nil {
		_ = uc.Operations.MarkFailed(ctx, op.ID, now, err.Error())
		return err
	}

	req := NodeAgentOperationRequest{
		AgentBaseURL:   nodeInfo.AgentBaseURL,
		OperationID:    op.ID,
		DeviceAccessID: accessEntry.ID,
		Protocol:       "wireguard",
		PeerPublicKey:  "n/a",
		AssignedIP:     valuePtrOrDefault(accessEntry.AssignedIP, "0.0.0.0/32"),
		Keepalive:      0,
	}

	if err := uc.NodeClient.RevokePeer(ctx, req); err != nil {
		_ = uc.Operations.MarkFailed(ctx, op.ID, now, err.Error())
		_ = uc.Accesses.MarkError(ctx, accessEntry.ID, now, err.Error())
		return err
	}

	if err := uc.Operations.MarkSuccess(ctx, op.ID, now); err != nil {
		return err
	}
	if err := uc.Accesses.Revoke(ctx, accessEntry.ID, now); err != nil {
		return err
	}
	if err := uc.updateNodeLoadDelta(ctx, op.VPNNodeID, -1); err != nil {
		return err
	}

	return nil
}

func (uc ExecuteRevokePeerOperation) updateNodeLoadDelta(ctx context.Context, nodeID string, delta int) error {
	n, err := uc.Nodes.GetByID(ctx, nodeID)
	if err != nil {
		return err
	}
	newLoad := n.CurrentLoad + delta
	if newLoad < 0 {
		newLoad = 0
	}
	return uc.Nodes.UpdateLoad(ctx, nodeID, newLoad)
}

func valueOrDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func valueOrDefaultInt(v, fallback int) int {
	if v == 0 {
		return fallback
	}
	return v
}

func valuePtrOrDefault(v *string, fallback string) string {
	if v == nil {
		return fallback
	}
	return *v
}

var _ = operation.StatusRunning
