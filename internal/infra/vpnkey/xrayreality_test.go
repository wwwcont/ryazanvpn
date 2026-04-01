package vpnkey

import (
	"context"
	"strings"
	"testing"

	"github.com/wwwcont/ryazanvpn/internal/app"
)

func TestXrayRealityExporter_ExportVLESSReality(t *testing.T) {
	exporter := NewXrayRealityExporter()
	link, err := exporter.ExportVLESSReality(context.Background(), app.ExportXrayRealityInput{
		UUID:             "11111111-1111-1111-1111-111111111111",
		ServerHost:       "vpn.example.com",
		Port:             8443,
		RealityPublicKey: "pubkey123",
		ServerName:       "www.cloudflare.com",
		ShortID:          "0123456789abcdef",
		Fingerprint:      "firefox",
		Flow:             "xtls-rprx-vision",
		Label:            "Ryazan VPN #1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(link, "vless://11111111-1111-1111-1111-111111111111@vpn.example.com:8443?") {
		t.Fatalf("unexpected prefix: %s", link)
	}
	if !strings.Contains(link, "security=reality") || !strings.Contains(link, "pbk=pubkey123") || !strings.Contains(link, "sid=0123456789abcdef") {
		t.Fatalf("unexpected params: %s", link)
	}
}

func TestXrayRealityExporter_LabelEncodingAndDefaults(t *testing.T) {
	exporter := NewXrayRealityExporter()
	link, err := exporter.ExportVLESSReality(context.Background(), app.ExportXrayRealityInput{
		UUID:             "11111111-1111-1111-1111-111111111111",
		ServerHost:       "vpn.example.com",
		Port:             8443,
		RealityPublicKey: "pubkey123",
		ServerName:       "www.cloudflare.com",
		ShortID:          "0123456789abcdef",
		Label:            "Health Link / RU",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(link, "fp=chrome") {
		t.Fatalf("default fingerprint missing: %s", link)
	}
	if !strings.Contains(link, "flow=xtls-rprx-vision") {
		t.Fatalf("default flow missing: %s", link)
	}
	if !strings.HasSuffix(link, "#Health+Link+%2F+RU") {
		t.Fatalf("label is not url-encoded: %s", link)
	}
}

func TestXrayRealityExporter_ValidateRequired(t *testing.T) {
	exporter := NewXrayRealityExporter()
	_, err := exporter.ExportVLESSReality(context.Background(), app.ExportXrayRealityInput{})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
