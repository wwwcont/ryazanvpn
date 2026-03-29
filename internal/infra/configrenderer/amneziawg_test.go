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
		DNS:              []string{"1.1.1.1"},
		EndpointHost:     "vpn.example.com",
		EndpointPort:     51820,
		Keepalive:        25,
		AllowedIPs:       []string{"0.0.0.0/0", "::/0"},
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
}
