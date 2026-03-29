package vpnkey

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/wwwcont/ryazanvpn/internal/app"
	"github.com/wwwcont/ryazanvpn/internal/infra/configrenderer"
	"github.com/wwwcont/ryazanvpn/internal/infra/wgkeys"
)

const sampleAWGConfig = `[Interface]
PrivateKey = FSfGSg9HVUWcRaOzggEUxGafoi8I8JfemfSWLIUhxuI=
Address = 10.8.1.10/32
MTU = 1376
DNS = 1.1.1.1, 8.8.8.8
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

[Peer]
PublicKey = KS3O5dK5fty5waMzWBFE92ovd3xpOEOEY6P2j84a+Cg=
PresharedKey = k9f4Jq8QqYWXSHI9hJfY2MqhSxYJ6mN7x6jG0Q2cL1A=
Endpoint = vpn.example.com:41475
AllowedIPs = 0.0.0.0/0, ::/0
PersistentKeepalive = 25
`

func sampleAWGFields() app.DefaultVPNAWGFields {
	return app.DefaultVPNAWGFields{
		Jc: 4, Jmin: 10, Jmax: 50,
		S1: 50, S2: 74, S3: 45, S4: 16,
		H1: "1391505721-1463481553",
		H2: "1725378175-1834354614",
		H3: "2076643873-2118219660",
		H4: "2141781406-2147031473",
		I1: "<r 2><b 0x858000010001000000000669636c6f756403636f6d0000010001c00c000100010000105a00044d583737>",
		I2: "",
		I3: "",
		I4: "",
		I5: "",
	}
}

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
		ClientPublicKey:    "jVcMIlprLo8VEAAXIBMDf08IxK0oRWLSArQryOk0DDE=",
		DefaultContainerID: "amnezia-awg2",
		MTU:                1376,
		AWG:                sampleAWGFields(),
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
	if decoded.Containers[0].AWG.Jmax != 50 {
		t.Fatalf("unexpected Jmax: %d", decoded.Containers[0].AWG.Jmax)
	}
}

func TestDefaultVPNGeneratedSchemaCompatibility(t *testing.T) {
	exporter := NewDefaultVPNExporter()
	key, err := exporter.ExportDefaultVPN(context.Background(), app.ExportVPNKeyInput{
		Config:          sampleAWGConfig,
		Description:     "RyazanVPN",
		ClientPublicKey: "jVcMIlprLo8VEAAXIBMDf08IxK0oRWLSArQryOk0DDE=",
		MTU:             1376,
		AWG:             sampleAWGFields(),
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
	container := decoded.Containers[0]
	if container.Container != "amnezia-awg2" {
		t.Fatalf("container mismatch: %s", container.Container)
	}
	if container.AWG.ProtocolVersion != "2" {
		t.Fatalf("protocol_version mismatch: %s", container.AWG.ProtocolVersion)
	}
	if container.AWG.SubnetAddress != "10.8.1.0" {
		t.Fatalf("subnet_address mismatch: %s", container.AWG.SubnetAddress)
	}

	var lastCfg defaultVPNLastConfig
	if err := json.Unmarshal([]byte(container.AWG.LastConfig), &lastCfg); err != nil {
		t.Fatalf("unmarshal last_config: %v", err)
	}
	if lastCfg.ClientID != "jVcMIlprLo8VEAAXIBMDf08IxK0oRWLSArQryOk0DDE=" {
		t.Fatalf("clientId mismatch: %s", lastCfg.ClientID)
	}
	if lastCfg.ClientPublicKey != "jVcMIlprLo8VEAAXIBMDf08IxK0oRWLSArQryOk0DDE=" {
		t.Fatalf("client_pub_key mismatch: %s", lastCfg.ClientPublicKey)
	}
	if lastCfg.ClientIP != "10.8.1.10" {
		t.Fatalf("client_ip mismatch: %s", lastCfg.ClientIP)
	}
	if len(lastCfg.AllowedIPs) != 2 || lastCfg.AllowedIPs[0] != "0.0.0.0/0" {
		t.Fatalf("allowed_ips must be array with expected values: %#v", lastCfg.AllowedIPs)
	}
	if !strings.Contains(lastCfg.Config, "Jc = 4") || !strings.Contains(lastCfg.Config, "I1 = <r 2><b 0x858000010001000000000669636c6f756403636f6d0000010001c00c000100010000105a00044d583737>") {
		t.Fatalf("last_config.config must contain amneziawg fields")
	}
}

func TestDefaultVPNExportRejectsMismatchedClientPublicKey(t *testing.T) {
	exporter := NewDefaultVPNExporter()
	_, err := exporter.ExportDefaultVPN(context.Background(), app.ExportVPNKeyInput{
		Config:          sampleAWGConfig,
		ClientPublicKey: "KS3O5dK5fty5waMzWBFE92ovd3xpOEOEY6P2j84a+Cg=",
		AWG:             sampleAWGFields(),
	})
	if err == nil || !strings.Contains(err.Error(), "client key mismatch") {
		t.Fatalf("expected client key mismatch error, got: %v", err)
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
	if len(parsed.AllowedIPs) != 2 {
		t.Fatalf("expected allowed_ips array, got: %#v", parsed.AllowedIPs)
	}
}

func TestDefaultVPNIntegration_ConfigRendererAndExporterUseSameClientKeypair(t *testing.T) {
	clientPub, clientPriv, err := wgkeys.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate client keypair: %v", err)
	}
	serverPub, _, err := wgkeys.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate server keypair: %v", err)
	}

	cfg, err := configrenderer.NewAmneziaWGRenderer().RenderAmneziaWG(app.RenderAmneziaWGInput{
		DevicePrivateKey: clientPriv,
		ServerPublicKey:  serverPub,
		PresharedKey:     "k9f4Jq8QqYWXSHI9hJfY2MqhSxYJ6mN7x6jG0Q2cL1A=",
		AssignedIP:       "10.8.1.10/32",
		MTU:              1376,
		DNS:              []string{"1.1.1.1", "8.8.8.8"},
		EndpointHost:     "vpn.example.com",
		EndpointPort:     41475,
		Keepalive:        25,
		AllowedIPs:       []string{"0.0.0.0/0", "::/0"},
		AWG:              sampleAWGFields(),
	})
	if err != nil {
		t.Fatalf("render config: %v", err)
	}

	key, err := NewDefaultVPNExporter().ExportDefaultVPN(context.Background(), app.ExportVPNKeyInput{
		Config:          cfg,
		ClientPublicKey: clientPub,
		MTU:             1376,
		AWG:             sampleAWGFields(),
	})
	if err != nil {
		t.Fatalf("export default vpn: %v", err)
	}
	decoded, err := DecodeDefaultVPN(key)
	if err != nil {
		t.Fatalf("decode default vpn: %v", err)
	}

	var lastCfg defaultVPNLastConfig
	if err := json.Unmarshal([]byte(decoded.Containers[0].AWG.LastConfig), &lastCfg); err != nil {
		t.Fatalf("unmarshal last_config: %v", err)
	}
	derived, err := wgkeys.DerivePublicKey(lastCfg.ClientPrivateKey)
	if err != nil {
		t.Fatalf("derive public key from exported private key: %v", err)
	}
	if derived != lastCfg.ClientPublicKey {
		t.Fatalf("exported client keypair mismatch: derived=%q stored=%q", derived, lastCfg.ClientPublicKey)
	}
	if lastCfg.ClientPublicKey != clientPub {
		t.Fatalf("exported client public key mismatch: got=%q want=%q", lastCfg.ClientPublicKey, clientPub)
	}
}
