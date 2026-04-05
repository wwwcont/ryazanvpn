package vpnkey

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/wwwcont/ryazanvpn/internal/app"
)

type XrayRealityExporter struct{}

func NewXrayRealityExporter() *XrayRealityExporter {
	return &XrayRealityExporter{}
}

func (e *XrayRealityExporter) ExportVLESSReality(_ context.Context, in app.ExportXrayRealityInput) (string, error) {
	missing := make([]string, 0, 6)
	if strings.TrimSpace(in.UUID) == "" {
		missing = append(missing, "uuid")
	}
	if strings.TrimSpace(in.ServerHost) == "" {
		missing = append(missing, "server_host")
	}
	if in.Port <= 0 {
		missing = append(missing, "port")
	}
	if strings.TrimSpace(in.RealityPublicKey) == "" {
		missing = append(missing, "reality_public_key")
	}
	if strings.TrimSpace(in.ServerName) == "" {
		missing = append(missing, "server_name")
	}
	if strings.TrimSpace(in.ShortID) == "" {
		missing = append(missing, "short_id")
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("missing required fields for vless reality export: %s", strings.Join(missing, ","))
	}

	fingerprint := strings.TrimSpace(in.Fingerprint)
	if fingerprint == "" {
		fingerprint = "chrome"
	}
	flow := strings.TrimSpace(in.Flow)
	if flow == "" {
		flow = "xtls-rprx-vision"
	}
	label := url.QueryEscape(strings.TrimSpace(in.Label))

	query := url.Values{}
	query.Set("type", "tcp")
	query.Set("security", "reality")
	query.Set("pbk", strings.TrimSpace(in.RealityPublicKey))
	query.Set("fp", fingerprint)
	query.Set("sni", strings.TrimSpace(in.ServerName))
	query.Set("sid", strings.TrimSpace(in.ShortID))
	query.Set("flow", flow)

	return fmt.Sprintf("vless://%s@%s:%d?%s#%s", strings.TrimSpace(in.UUID), strings.TrimSpace(in.ServerHost), in.Port, query.Encode(), label), nil
}
