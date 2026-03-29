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
