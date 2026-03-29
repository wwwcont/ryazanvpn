package app

import "testing"

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
