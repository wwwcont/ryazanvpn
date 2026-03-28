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

func TestLoadConfigControlPlaneRequiresPostgres(t *testing.T) {
	t.Setenv("HTTP_ADDR", ":18080")
	t.Setenv("POSTGRES_URL", "")
	_, err := LoadConfig("control-plane")
	if err == nil {
		t.Fatal("expected error when POSTGRES_URL is empty for control-plane")
	}
}
