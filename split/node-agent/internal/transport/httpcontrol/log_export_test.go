package httpcontrol

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/infra/oplog"
)

func TestAdminLogsExport_TimeRangeFilter(t *testing.T) {
	store, err := oplog.NewStore(t.TempDir(), 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_ = store.Write(context.Background(), oplog.Record{Timestamp: now.Add(-2 * time.Hour), Service: "control-plane", Severity: "error", NodeID: "n1", Message: "old"})
	_ = store.Write(context.Background(), oplog.Record{Timestamp: now.Add(-10 * time.Minute), Service: "control-plane", Severity: "warn", NodeID: "n2", Message: "new"})

	r := NewRouter(Options{
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		AdminSecret: "secret",
		OpsLog:      store,
	})

	from := now.Add(-30 * time.Minute).Format(time.RFC3339)
	to := now.Add(1 * time.Minute).Format(time.RFC3339)
	req := httptest.NewRequest(http.MethodGet, "/admin/logs/export?from="+from+"&to="+to+"&service=control-plane&level=warn", nil)
	req.Header.Set("X-Admin-Secret", "secret")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "ndjson") {
		t.Fatalf("expected ndjson content-type, got %s", ct)
	}
	if !strings.Contains(w.Body.String(), `"message":"new"`) {
		t.Fatalf("expected filtered newer log, got %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"message":"old"`) {
		t.Fatalf("did not expect old record in output: %s", w.Body.String())
	}
}
