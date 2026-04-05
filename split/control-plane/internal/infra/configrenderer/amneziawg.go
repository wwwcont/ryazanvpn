package configrenderer

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/wwwcont/ryazanvpn/internal/app"
)

type AmneziaWGRenderer struct{}

func NewAmneziaWGRenderer() *AmneziaWGRenderer {
	return &AmneziaWGRenderer{}
}

func (r *AmneziaWGRenderer) RenderAmneziaWG(in app.RenderAmneziaWGInput) (string, error) {
	missing := make([]string, 0, 6)
	if strings.TrimSpace(in.DevicePrivateKey) == "" {
		missing = append(missing, "device_private_key")
	}
	if strings.TrimSpace(in.ServerPublicKey) == "" {
		missing = append(missing, "server_public_key")
	}
	if strings.TrimSpace(in.AssignedIP) == "" {
		missing = append(missing, "assigned_ip")
	}
	if strings.TrimSpace(in.EndpointHost) == "" {
		missing = append(missing, "endpoint_host")
	}
	if in.EndpointPort <= 0 {
		missing = append(missing, "endpoint_port")
	}
	if strings.TrimSpace(in.PresharedKey) == "" {
		missing = append(missing, "preshared_key")
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("missing required fields for amneziawg config: %s", strings.Join(missing, ","))
	}
	if err := app.ValidateConfigEndpointHost(in.EndpointHost); err != nil {
		return "", err
	}
	if err := app.ValidateConfigEndpointPort(in.EndpointPort); err != nil {
		return "", err
	}

	if in.Keepalive <= 0 {
		in.Keepalive = 25
	}
	if len(in.DNS) == 0 {
		in.DNS = []string{"1.1.1.1", "8.8.8.8"}
	}
	if len(in.AllowedIPs) == 0 {
		in.AllowedIPs = []string{"0.0.0.0/0", "::/0"}
	}
	if in.MTU <= 0 {
		in.MTU = 1376
	}

	cfg := fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s
MTU = %d
DNS = %s
Jc = %d
Jmin = %d
Jmax = %d
S1 = %d
S2 = %d
S3 = %d
S4 = %d
H1 = %s
H2 = %s
H3 = %s
H4 = %s
I1 = %s
I2 = %s
I3 = %s
I4 = %s
I5 = %s

[Peer]
PublicKey = %s
PresharedKey = %s
Endpoint = %s:%d
AllowedIPs = %s
PersistentKeepalive = %d
`,
		in.DevicePrivateKey,
		in.AssignedIP,
		in.MTU,
		strings.Join(in.DNS, ", "),
		in.AWG.Jc,
		in.AWG.Jmin,
		in.AWG.Jmax,
		in.AWG.S1,
		in.AWG.S2,
		in.AWG.S3,
		in.AWG.S4,
		in.AWG.H1,
		in.AWG.H2,
		in.AWG.H3,
		in.AWG.H4,
		in.AWG.I1,
		in.AWG.I2,
		in.AWG.I3,
		in.AWG.I4,
		in.AWG.I5,
		in.ServerPublicKey,
		in.PresharedKey,
		in.EndpointHost,
		in.EndpointPort,
		strings.Join(in.AllowedIPs, ", "),
		in.Keepalive,
	)
	return cfg, nil
}

func (r *AmneziaWGRenderer) RenderXrayReality(in app.RenderXrayRealityInput) (string, error) {
	missing := make([]string, 0, 6)
	if strings.TrimSpace(in.UserUUID) == "" {
		missing = append(missing, "uuid")
	}
	if strings.TrimSpace(in.ServerHost) == "" {
		missing = append(missing, "server_host")
	}
	if in.ServerPort <= 0 {
		missing = append(missing, "server_port")
	}
	if strings.TrimSpace(in.ServerName) == "" {
		missing = append(missing, "server_name")
	}
	if strings.TrimSpace(in.PublicKey) == "" {
		missing = append(missing, "reality_public_key")
	}
	if strings.TrimSpace(in.ShortID) == "" {
		missing = append(missing, "short_id")
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("missing required fields for xray reality config: %s", strings.Join(missing, ","))
	}
	if err := app.ValidateConfigEndpointHost(in.ServerHost); err != nil {
		return "", err
	}
	if err := app.ValidateConfigEndpointPort(in.ServerPort); err != nil {
		return "", err
	}
	fingerprint := fallback(in.Fingerprint, "chrome")
	flow := fallback(in.Flow, "xtls-rprx-vision")
	serverName := strings.TrimSpace(in.ServerName)
	query := url.Values{}
	query.Set("type", "tcp")
	query.Set("security", "reality")
	query.Set("pbk", strings.TrimSpace(in.PublicKey))
	query.Set("fp", fingerprint)
	query.Set("sni", serverName)
	query.Set("sid", strings.TrimSpace(in.ShortID))
	query.Set("flow", flow)
	uri := fmt.Sprintf("vless://%s@%s:%d?%s#%s", strings.TrimSpace(in.UserUUID), strings.TrimSpace(in.ServerHost), in.ServerPort, query.Encode(), url.QueryEscape(fallback(in.DeviceID, "RyazanVPN-Health")))
	cfg := map[string]any{
		"log":      map[string]any{"loglevel": "warning"},
		"inbounds": []any{},
		"outbounds": []any{
			map[string]any{
				"tag":      "proxy",
				"protocol": "vless",
				"settings": map[string]any{
					"vnext": []any{
						map[string]any{
							"address": strings.TrimSpace(in.ServerHost),
							"port":    in.ServerPort,
							"users": []any{
								map[string]any{"id": strings.TrimSpace(in.UserUUID), "encryption": "none", "flow": flow},
							},
						},
					},
				},
				"streamSettings": map[string]any{
					"network":  "tcp",
					"security": "reality",
					"realitySettings": map[string]any{
						"serverName":  serverName,
						"fingerprint": fingerprint,
						"publicKey":   strings.TrimSpace(in.PublicKey),
						"shortId":     strings.TrimSpace(in.ShortID),
						"spiderX":     "/",
					},
				},
			},
			map[string]any{"tag": "direct", "protocol": "freedom"},
		},
		"routing": map[string]any{"domainStrategy": "AsIs"},
		"xray_export": map[string]any{
			"uri":         uri,
			"uuid":        strings.TrimSpace(in.UserUUID),
			"server_host": strings.TrimSpace(in.ServerHost),
			"server_port": in.ServerPort,
			"server_name": serverName,
			"public_key":  strings.TrimSpace(in.PublicKey),
			"short_id":    strings.TrimSpace(in.ShortID),
			"fingerprint": fingerprint,
			"flow":        flow,
			"transport":   "tcp",
			"security":    "reality",
		},
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func fallback(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
