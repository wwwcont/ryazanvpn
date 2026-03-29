package vpnkey

import (
	"context"
	"testing"

	"github.com/wwwcont/ryazanvpn/internal/app"
)

const sampleWGConfig = `[Interface]
PrivateKey = client_private_key_base64
Address = 10.8.1.10/32
DNS = 1.1.1.1, 8.8.8.8

[Peer]
PublicKey = server_public_key_base64
PresharedKey = psk_base64
Endpoint = vpn.example.com:41475
AllowedIPs = 0.0.0.0/0, ::/0
PersistentKeepalive = 25
`

func TestDefaultVPNEncodeDecodeRoundtrip(t *testing.T) {
	exporter := NewDefaultVPNExporter()
	key, err := exporter.ExportDefaultVPN(context.Background(), app.ExportVPNKeyInput{
		Config:             sampleWGConfig,
		Description:        "RyazanVPN default",
		HostName:           "vpn.example.com",
		Port:               41475,
		DNS1:               "1.1.1.1",
		DNS2:               "8.8.8.8",
		ProtocolVersion:    1,
		TransportProto:     "udp",
		SubnetAddress:      "10.8.1.10/32",
		ClientPublicKey:    "client_pub_key_base64",
		DefaultContainerID: "awg",
		AWG: app.DefaultVPNAWGFields{
			Jc: 3, Jmin: 50, Jmax: 1000,
			S1: 15, S2: 77, S3: 19, S4: 22,
			H1: 1, H2: 2, H3: 3, H4: 4,
			I1: 7, I2: 8, I3: 9, I4: 10, I5: 11,
		},
	})
	if err != nil {
		t.Fatalf("ExportDefaultVPN error: %v", err)
	}

	decoded, err := DecodeDefaultVPN(key)
	if err != nil {
		t.Fatalf("DecodeDefaultVPN error: %v", err)
	}
	if decoded.HostName != "vpn.example.com" {
		t.Fatalf("unexpected hostname: %s", decoded.HostName)
	}
	if len(decoded.Containers) != 1 {
		t.Fatalf("expected one container, got %d", len(decoded.Containers))
	}
	if decoded.Containers[0].AWG.Jmax != 1000 {
		t.Fatalf("unexpected Jmax: %d", decoded.Containers[0].AWG.Jmax)
	}
}

func TestParseWGConfigCompatibility(t *testing.T) {
	parsed, err := ParseWGConfig(sampleWGConfig)
	if err != nil {
		t.Fatalf("ParseWGConfig error: %v", err)
	}
	if parsed.EndpointHost != "vpn.example.com" || parsed.EndpointPort != 41475 {
		t.Fatalf("unexpected endpoint: %s:%d", parsed.EndpointHost, parsed.EndpointPort)
	}
	if parsed.ClientIP != "10.8.1.10/32" {
		t.Fatalf("unexpected client ip: %s", parsed.ClientIP)
	}
	if parsed.PersistentKeepalive != 25 {
		t.Fatalf("unexpected keepalive: %d", parsed.PersistentKeepalive)
	}
}
