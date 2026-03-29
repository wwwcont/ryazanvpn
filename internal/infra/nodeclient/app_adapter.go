package nodeclient

import (
	"context"
	"strings"

	"github.com/wwwcont/ryazanvpn/internal/app"
)

type AppAdapter struct {
	Client *Client
	Config Config
}

func (a AppAdapter) ApplyPeer(ctx context.Context, req app.NodeAgentOperationRequest) error {
	cli := a.clientForBaseURL(req.AgentBaseURL)
	return cli.ApplyPeer(ctx, OperationRequest{
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
	cli := a.clientForBaseURL(req.AgentBaseURL)
	return cli.RevokePeer(ctx, OperationRequest{
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
	if a.Client == nil {
		return nil, nil
	}
	items, err := a.Client.GetTrafficCounters(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]app.NodeTrafficCounter, 0, len(items))
	for _, it := range items {
		out = append(out, app.NodeTrafficCounter{
			DeviceAccessID:  it.DeviceAccessID,
			PeerPublicKey:   it.PeerPublicKey,
			AllowedIP:       it.AllowedIP,
			Endpoint:        it.Endpoint,
			PresharedKey:    it.PresharedKey,
			RXTotalBytes:    it.RXTotalBytes,
			TXTotalBytes:    it.TXTotalBytes,
			LastHandshakeAt: it.LastHandshakeAt,
		})
	}
	return out, nil
}

func (a AppAdapter) clientForBaseURL(baseURL string) *Client {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" && a.Client != nil {
		return a.Client
	}
	cfg := a.Config
	cfg.BaseURL = baseURL
	if cfg.BaseURL == "" && a.Client != nil {
		return a.Client
	}
	return New(cfg)
}
