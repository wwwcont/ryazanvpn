package configrenderer

import (
	"fmt"
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

	if in.Keepalive <= 0 {
		in.Keepalive = 25
	}
	if len(in.DNS) == 0 {
		in.DNS = []string{"1.1.1.1", "8.8.8.8"}
	}
	if len(in.AllowedIPs) == 0 {
		in.AllowedIPs = []string{"0.0.0.0/0", "::/0"}
	}

	cfg := fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s
DNS = %s

[Peer]
PublicKey = %s
PresharedKey = %s
Endpoint = %s:%d
AllowedIPs = %s
PersistentKeepalive = %d
`,
		in.DevicePrivateKey,
		in.AssignedIP,
		strings.Join(in.DNS, ", "),
		in.ServerPublicKey,
		in.PresharedKey,
		in.EndpointHost,
		in.EndpointPort,
		strings.Join(in.AllowedIPs, ", "),
		in.Keepalive,
	)
	return cfg, nil
}
