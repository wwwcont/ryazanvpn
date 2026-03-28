package configrenderer

import (
	"strings"
	"testing"

	"github.com/example/ryazanvpn/internal/app"
)

func TestRenderAmneziaWG(t *testing.T) {
	r := NewAmneziaWGRenderer()
	cfg, err := r.RenderAmneziaWG(app.RenderAmneziaWGInput{
		DevicePrivateKey: "priv",
		ServerPublicKey:  "pub",
		AssignedIP:       "10.0.0.2/32",
		DNS:              []string{"1.1.1.1"},
		EndpointHost:     "vpn.example.com",
		EndpointPort:     51820,
		Keepalive:        25,
	})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	if !strings.Contains(cfg, "PrivateKey = priv") {
		t.Fatal("missing private key")
	}
}
