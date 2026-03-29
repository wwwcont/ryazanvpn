package configrenderer

import (
	"strings"
	"testing"

	"github.com/wwwcont/ryazanvpn/internal/app"
)

func TestRenderAmneziaWG(t *testing.T) {
	r := NewAmneziaWGRenderer()
	cfg, err := r.RenderAmneziaWG(app.RenderAmneziaWGInput{
		DevicePrivateKey: "priv",
		ServerPublicKey:  "pub",
		PresharedKey:     "psk",
		AssignedIP:       "10.0.0.2/32",
		MTU:              1376,
		DNS:              []string{"1.1.1.1"},
		EndpointHost:     "vpn.example.com",
		EndpointPort:     51820,
		Keepalive:        25,
		AllowedIPs:       []string{"0.0.0.0/0", "::/0"},
		AWG: app.DefaultVPNAWGFields{
			Jc: 4, Jmin: 10, Jmax: 50,
			S1: 50, S2: 74, S3: 45, S4: 16,
			H1: "1391505721-1463481553",
			H2: "1725378175-1834354614",
			H3: "2076643873-2118219660",
			H4: "2141781406-2147031473",
			I1: "<r 2><b 0x8580>",
		},
	})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	if !strings.Contains(cfg, "PrivateKey = priv") {
		t.Fatal("missing private key")
	}
	if !strings.Contains(cfg, "PresharedKey = psk") {
		t.Fatal("missing preshared key")
	}
	if !strings.Contains(cfg, "AllowedIPs = 0.0.0.0/0, ::/0") {
		t.Fatal("missing allowed ips")
	}
	if !strings.Contains(cfg, "Jc = 4") || !strings.Contains(cfg, "I1 = <r 2><b 0x8580>") {
		t.Fatal("missing amnezia fields")
	}
}
