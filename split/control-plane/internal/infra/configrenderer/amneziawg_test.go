package configrenderer

import (
	"encoding/json"
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

func TestRenderAmneziaWG_RejectsDocumentationEndpoint(t *testing.T) {
	r := NewAmneziaWGRenderer()
	_, err := r.RenderAmneziaWG(app.RenderAmneziaWGInput{
		DevicePrivateKey: "priv",
		ServerPublicKey:  "pub",
		PresharedKey:     "psk",
		AssignedIP:       "10.0.0.2/32",
		EndpointHost:     "203.0.113.10",
		EndpointPort:     51820,
	})
	if err == nil || !strings.Contains(err.Error(), "documentation placeholder") {
		t.Fatalf("expected documentation placeholder validation error, got %v", err)
	}
}

func TestRenderXrayReality_ProducesImportableJSONWithVLESSURI(t *testing.T) {
	r := NewAmneziaWGRenderer()
	cfg, err := r.RenderXrayReality(app.RenderXrayRealityInput{
		DeviceID:    "dev-1",
		ServerName:  "www.cloudflare.com",
		ServerHost:  "vpn.example.com",
		ServerPort:  8443,
		UserUUID:    "11111111-1111-1111-1111-111111111111",
		PublicKey:   "pubKey",
		ShortID:     "0123456789abcdef",
		Fingerprint: "chrome",
		Flow:        "xtls-rprx-vision",
	})
	if err != nil {
		t.Fatalf("render xray config failed: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(cfg), &parsed); err != nil {
		t.Fatalf("expected valid json config, got error: %v", err)
	}
	exportBlock, ok := parsed["xray_export"].(map[string]any)
	if !ok {
		t.Fatalf("expected xray_export block in config: %s", cfg)
	}
	uri := strings.TrimSpace(exportBlock["uri"].(string))
	if !strings.HasPrefix(uri, "vless://11111111-1111-1111-1111-111111111111@vpn.example.com:8443?") {
		t.Fatalf("unexpected uri host/port/uuid: %s", uri)
	}
	if !strings.Contains(uri, "security=reality") || !strings.Contains(uri, "pbk=pubKey") || !strings.Contains(uri, "sid=0123456789abcdef") {
		t.Fatalf("uri misses required reality fields: %s", uri)
	}
}
