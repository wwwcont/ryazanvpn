package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigNodeAgentDoesNotRequirePostgres(t *testing.T) {
	t.Setenv("HTTP_ADDR", ":18081")
	t.Setenv("POSTGRES_URL", "")
	cfg, err := LoadConfig("node-agent")
	if err != nil {
		t.Fatalf("LoadConfig(node-agent) returned error: %v", err)
	}
	if cfg.PostgresURL != "" {
		t.Fatalf("expected empty PostgresURL for node-agent test, got %q", cfg.PostgresURL)
	}
}

func TestLoadConfigUsesScopedNodeAgentAddr(t *testing.T) {
	t.Setenv("HTTP_ADDR", ":18080")
	t.Setenv("NODE_AGENT_HTTP_ADDR", ":18081")
	cfg, err := LoadConfig("node-agent")
	if err != nil {
		t.Fatalf("LoadConfig(node-agent) returned error: %v", err)
	}
	if cfg.HTTPAddr != ":18081" {
		t.Fatalf("expected node-agent scoped address, got %q", cfg.HTTPAddr)
	}
}

func TestLoadConfigUsesScopedControlPlaneAddr(t *testing.T) {
	t.Setenv("POSTGRES_URL", "postgres://example")
	t.Setenv("HTTP_ADDR", ":18090")
	t.Setenv("CONTROL_PLANE_HTTP_ADDR", ":18080")
	cfg, err := LoadConfig("control-plane")
	if err != nil {
		t.Fatalf("LoadConfig(control-plane) returned error: %v", err)
	}
	if cfg.HTTPAddr != ":18080" {
		t.Fatalf("expected control-plane scoped address, got %q", cfg.HTTPAddr)
	}
}

func TestLoadConfigControlPlaneRequiresPostgres(t *testing.T) {
	t.Setenv("HTTP_ADDR", ":18080")
	t.Setenv("POSTGRES_URL", "")
	_, err := LoadConfig("control-plane")
	if err == nil {
		t.Fatal("expected error when POSTGRES_URL is empty for control-plane")
	}
}

func TestLoadConfigPrefersExplicitEnvOverRuntimeMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	amneziaMeta := filepath.Join(tmpDir, "amnezia-runtime.json")
	xrayMeta := filepath.Join(tmpDir, "xray-runtime.json")
	vpnKeyFile := filepath.Join(tmpDir, "server.publickey")
	xrayPubFile := filepath.Join(tmpDir, "reality.publickey")

	if err := os.WriteFile(amneziaMeta, []byte(`{"server_public_key":"runtime-awg-key","listen_port":4242,"jc":"9","h1":"runtime-h1"}`), 0o600); err != nil {
		t.Fatalf("write amnezia metadata: %v", err)
	}
	if err := os.WriteFile(xrayMeta, []byte(`{"reality_public_key":"runtime-xray-meta-key","listen_port":9443,"server_name":"runtime.sni","short_id":"beef","public_host":"runtime.host"}`), 0o600); err != nil {
		t.Fatalf("write xray metadata: %v", err)
	}
	if err := os.WriteFile(vpnKeyFile, []byte("runtime-awg-file-key\n"), 0o600); err != nil {
		t.Fatalf("write vpn key file: %v", err)
	}
	if err := os.WriteFile(xrayPubFile, []byte("runtime-xray-file-key\n"), 0o600); err != nil {
		t.Fatalf("write xray key file: %v", err)
	}

	t.Setenv("POSTGRES_URL", "postgres://example")
	t.Setenv("AMNEZIA_RUNTIME_METADATA_PATH", amneziaMeta)
	t.Setenv("XRAY_RUNTIME_METADATA_PATH", xrayMeta)
	t.Setenv("VPN_SERVER_PUBLIC_KEY_FILE", vpnKeyFile)
	t.Setenv("XRAY_REALITY_PUBLIC_KEY_FILE", xrayPubFile)
	t.Setenv("VPN_SERVER_PUBLIC_KEY", "env-awg-key")
	t.Setenv("XRAY_REALITY_PUBLIC_KEY", "env-xray-key")
	t.Setenv("XRAY_REALITY_PORT", "8443")
	t.Setenv("XRAY_REALITY_SERVER_NAME", "env.sni")
	t.Setenv("XRAY_REALITY_SHORT_ID", "envshort")

	cfg, err := LoadConfig("control-plane")
	if err != nil {
		t.Fatalf("LoadConfig(control-plane) returned error: %v", err)
	}

	if cfg.VPNServerPublicKey != "runtime-awg-file-key" {
		t.Fatalf("expected vpn key from file, got %q", cfg.VPNServerPublicKey)
	}
	if !cfg.VPNServerPublicKeyFromFile {
		t.Fatal("expected VPNServerPublicKeyFromFile=true")
	}
	if cfg.AmneziaPort != "4242" {
		t.Fatalf("expected amnezia port from metadata, got %q", cfg.AmneziaPort)
	}
	if cfg.VPNAWGJc != 9 {
		t.Fatalf("expected VPNAWGJc=9 from metadata, got %d", cfg.VPNAWGJc)
	}
	if cfg.VPNAWGH1 != "runtime-h1" {
		t.Fatalf("expected VPNAWGH1 from metadata, got %q", cfg.VPNAWGH1)
	}
	if cfg.XrayRealityPublicKey != "runtime-xray-file-key" {
		t.Fatalf("expected xray public key from file, got %q", cfg.XrayRealityPublicKey)
	}
	if cfg.XrayRealityPort != 8443 {
		t.Fatalf("expected xray port from env, got %d", cfg.XrayRealityPort)
	}
	if cfg.XrayRealityServerName != "env.sni" {
		t.Fatalf("expected xray server name from env, got %q", cfg.XrayRealityServerName)
	}
	if cfg.XrayRealityShortID != "envshort" {
		t.Fatalf("expected xray short id from env, got %q", cfg.XrayRealityShortID)
	}
	if cfg.XrayPublicHost != "runtime.host" {
		t.Fatalf("expected xray public host from metadata, got %q", cfg.XrayPublicHost)
	}
}

func TestLoadConfigUsesXrayRuntimeMetadataWhenEnvNotProvided(t *testing.T) {
	tmpDir := t.TempDir()
	xrayMeta := filepath.Join(tmpDir, "xray-runtime.json")

	if err := os.WriteFile(xrayMeta, []byte(`{"reality_public_key":"runtime-xray-meta-key","listen_port":9443,"server_name":"runtime.sni","short_id":"beef","public_host":"runtime.host"}`), 0o600); err != nil {
		t.Fatalf("write xray metadata: %v", err)
	}

	t.Setenv("POSTGRES_URL", "postgres://example")
	t.Setenv("XRAY_RUNTIME_METADATA_PATH", xrayMeta)
	for _, key := range []string{"XRAY_REALITY_PORT", "XRAY_REALITY_SERVER_NAME", "XRAY_REALITY_SHORT_ID", "XRAY_PUBLIC_HOST", "XRAY_REALITY_PUBLIC_KEY"} {
		prev, had := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
		t.Cleanup(func() {
			if !had {
				_ = os.Unsetenv(key)
				return
			}
			_ = os.Setenv(key, prev)
		})
	}

	cfg, err := LoadConfig("control-plane")
	if err != nil {
		t.Fatalf("LoadConfig(control-plane) returned error: %v", err)
	}

	if cfg.XrayRealityPort != 9443 {
		t.Fatalf("expected xray port from metadata, got %d", cfg.XrayRealityPort)
	}
	if cfg.XrayRealityServerName != "runtime.sni" {
		t.Fatalf("expected xray server name from metadata, got %q", cfg.XrayRealityServerName)
	}
	if cfg.XrayRealityShortID != "beef" {
		t.Fatalf("expected xray short id from metadata, got %q", cfg.XrayRealityShortID)
	}
	if cfg.XrayPublicHost != "runtime.host" {
		t.Fatalf("expected xray public host from metadata, got %q", cfg.XrayPublicHost)
	}
	if cfg.XrayRealityPublicKey != "runtime-xray-meta-key" {
		t.Fatalf("expected xray public key from metadata, got %q", cfg.XrayRealityPublicKey)
	}
}
