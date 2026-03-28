package nodeclient

import (
	"context"

	"github.com/wwwcont/ryazanvpn/internal/app"
)

type AppAdapter struct {
	Client *Client
}

func (a AppAdapter) ApplyPeer(ctx context.Context, req app.NodeAgentOperationRequest) error {
	return a.Client.ApplyPeer(ctx, OperationRequest{
		OperationID:    req.OperationID,
		DeviceAccessID: req.DeviceAccessID,
		Protocol:       req.Protocol,
		PeerPublicKey:  req.PeerPublicKey,
		AssignedIP:     req.AssignedIP,
		Keepalive:      req.Keepalive,
		EndpointMeta:   req.EndpointMeta,
	})
}

func (a AppAdapter) RevokePeer(ctx context.Context, req app.NodeAgentOperationRequest) error {
	return a.Client.RevokePeer(ctx, OperationRequest{
		OperationID:    req.OperationID,
		DeviceAccessID: req.DeviceAccessID,
		Protocol:       req.Protocol,
		PeerPublicKey:  req.PeerPublicKey,
		AssignedIP:     req.AssignedIP,
		Keepalive:      req.Keepalive,
		EndpointMeta:   req.EndpointMeta,
	})
}

func (a AppAdapter) GetTrafficCounters(ctx context.Context) ([]app.NodeTrafficCounter, error) {
	items, err := a.Client.GetTrafficCounters(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]app.NodeTrafficCounter, 0, len(items))
	for _, it := range items {
		out = append(out, app.NodeTrafficCounter{
			DeviceAccessID:  it.DeviceAccessID,
			RXTotalBytes:    it.RXTotalBytes,
			TXTotalBytes:    it.TXTotalBytes,
			LastHandshakeAt: it.LastHandshakeAt,
		})
	}
	return out, nil
}
