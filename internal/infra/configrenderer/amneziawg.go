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
	if in.DevicePrivateKey == "" || in.ServerPublicKey == "" || in.AssignedIP == "" || in.EndpointHost == "" || in.EndpointPort <= 0 {
		return "", fmt.Errorf("missing required fields for amneziawg config")
	}
	if in.Keepalive <= 0 {
		in.Keepalive = 25
	}
	if len(in.DNS) == 0 {
		in.DNS = []string{"1.1.1.1", "8.8.8.8"}
	}

	cfg := fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s
DNS = %s

[Peer]
PublicKey = %s
Endpoint = %s:%d
PersistentKeepalive = %d
AllowedIPs = 0.0.0.0/0, ::/0
`,
		in.DevicePrivateKey,
		in.AssignedIP,
		strings.Join(in.DNS, ", "),
		in.ServerPublicKey,
		in.EndpointHost,
		in.EndpointPort,
		in.Keepalive,
	)
	return cfg, nil
}
