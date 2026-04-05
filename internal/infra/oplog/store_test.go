package oplog

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestExportFilterByRangeAndFields(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_ = s.Write(context.Background(), Record{Timestamp: now.Add(-2 * time.Hour), Service: "control-plane", Severity: "error", NodeID: "n1", ErrorCode: "E_RECONCILE_APPLY"})
	_ = s.Write(context.Background(), Record{Timestamp: now.Add(-time.Hour), Service: "control-plane", Severity: "warn", NodeID: "n2"})
	_ = s.Write(context.Background(), Record{Timestamp: now, Service: "node-agent", Severity: "error", NodeID: "n1"})

	var out bytes.Buffer
	n, err := s.Export(context.Background(), &out, Filter{From: now.Add(-90 * time.Minute), To: now.Add(10 * time.Minute), Service: "control-plane", NodeID: "n2", Level: "warn"})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 line, got %d", n)
	}
	if !strings.Contains(out.String(), `"node_id":"n2"`) {
		t.Fatalf("unexpected payload: %s", out.String())
	}
}

func TestExportLimitForLoadSafety(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for i := 0; i < 200; i++ {
		_ = s.Write(context.Background(), Record{Timestamp: now, Service: "control-plane", Severity: "info", Message: "m"})
	}
	var out bytes.Buffer
	n, err := s.Export(context.Background(), &out, Filter{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if n != 50 {
		t.Fatalf("expected capped export 50, got %d", n)
	}
}
