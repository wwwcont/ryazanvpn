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
	if len(in.AllowedIPs) == 0 {
		in.AllowedIPs = []string{"0.0.0.0/0", "::/0"}
	}
	pskLine := ""
	if strings.TrimSpace(in.PresharedKey) != "" {
		pskLine = fmt.Sprintf("PresharedKey = %s\n", in.PresharedKey)
	}

	cfg := fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s
DNS = %s

[Peer]
PublicKey = %s
%sEndpoint = %s:%d
AllowedIPs = %s
PersistentKeepalive = %d
`,
		in.DevicePrivateKey,
		in.AssignedIP,
		strings.Join(in.DNS, ", "),
		in.ServerPublicKey,
		pskLine,
		in.EndpointHost,
		in.EndpointPort,
		strings.Join(in.AllowedIPs, ", "),
		in.Keepalive,
	)
	return cfg, nil
}
