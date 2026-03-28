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
