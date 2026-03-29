package vpnkey

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/wwwcont/ryazanvpn/internal/app"
)

const sampleAWGConfig = `[Interface]
PrivateKey = client_private_key_base64
Address = 10.8.1.10/32
DNS = 1.1.1.1, 8.8.8.8
MTU = 1376
Jc = 4
Jmin = 10
Jmax = 50
S1 = 50
S2 = 74
S3 = 45
S4 = 16
H1 = 1391505721-1463481553
H2 = 1725378175-1834354614
H3 = 2076643873-2118219660
H4 = 2141781406-2147031473
I1 = <r 2><b 0x858000010001000000000669636c6f756403636f6d0000010001c00c000100010000105a00044d583737>
I2 =
I3 =
I4 =
I5 =
ClientId = client_pub_key_base64

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
		Config:             sampleAWGConfig,
		Description:        "RyazanVPN default",
		HostName:           "vpn.example.com",
		Port:               41475,
		DNS1:               "1.1.1.1",
		DNS2:               "8.8.8.8",
		ProtocolVersion:    2,
		TransportProto:     "udp",
		SubnetAddress:      "10.8.1.0/24",
		ClientPublicKey:    "client_pub_key_base64",
		DefaultContainerID: "amnezia-awg2",
		AWG: app.DefaultVPNAWGFields{
			Jc:   4,
			Jmin: 10,
			Jmax: 50,
			S1:   50,
			S2:   74,
			S3:   45,
			S4:   16,
			H1:   "1391505721-1463481553",
			H2:   "1725378175-1834354614",
			H3:   "2076643873-2118219660",
			H4:   "2141781406-2147031473",
			I1:   "<r 2><b 0x858000010001000000000669636c6f756403636f6d0000010001c00c000100010000105a00044d583737>",
			MTU:  1376,
		},
	})
	if err != nil {
		t.Fatalf("ExportDefaultVPN error: %v", err)
	}

	decoded, err := DecodeDefaultVPN(key)
	if err != nil {
		t.Fatalf("DecodeDefaultVPN error: %v", err)
	}
	if decoded.DefaultContainer != "amnezia-awg2" {
		t.Fatalf("defaultContainer mismatch: %s", decoded.DefaultContainer)
	}
	if len(decoded.Containers) != 1 || decoded.Containers[0].Container != "amnezia-awg2" {
		t.Fatalf("container mismatch: %+v", decoded.Containers)
	}
	if decoded.Containers[0].AWG.ProtocolVersion != 2 {
		t.Fatalf("protocol_version mismatch: %d", decoded.Containers[0].AWG.ProtocolVersion)
	}
	if decoded.Containers[0].AWG.SubnetAddress != "10.8.1.0" {
		t.Fatalf("subnet_address mismatch: %s", decoded.Containers[0].AWG.SubnetAddress)
	}

	var lastCfg map[string]any
	if err := json.Unmarshal([]byte(decoded.Containers[0].AWG.LastConfig), &lastCfg); err != nil {
		t.Fatalf("last_config json: %v", err)
	}
	allowed, ok := lastCfg["allowed_ips"].([]any)
	if !ok || len(allowed) != 2 {
		t.Fatalf("allowed_ips must be array, got %#v", lastCfg["allowed_ips"])
	}
	required := []string{"clientId", "mtu", "H1", "H2", "H3", "H4", "I1", "I2", "I3", "I4", "I5", "Jc", "Jmin", "Jmax", "S1", "S2", "S3", "S4", "server_pub_key", "client_priv_key", "client_pub_key", "psk_key", "client_ip", "hostName", "port", "persistent_keep_alive", "config"}
	for _, field := range required {
		if _, exists := lastCfg[field]; !exists {
			t.Fatalf("missing last_config field %s", field)
		}
	}
}

func TestParseWGConfigCompatibility(t *testing.T) {
	parsed, err := ParseWGConfig(sampleAWGConfig)
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
	if parsed.Jc != 4 || parsed.Jmin != 10 || parsed.Jmax != 50 {
		t.Fatalf("unexpected J params: %+v", parsed)
	}
}
