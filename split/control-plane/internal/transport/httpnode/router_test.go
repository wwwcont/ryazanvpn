package httpnode

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/agent/runtime"
)

type stubRuntime struct {
	healthErr error
}

func (s stubRuntime) ApplyPeer(context.Context, runtime.PeerOperationRequest) (runtime.OperationResult, error) {
	return runtime.OperationResult{}, nil
}

func (s stubRuntime) RevokePeer(context.Context, runtime.PeerOperationRequest) (runtime.OperationResult, error) {
	return runtime.OperationResult{}, nil
}

func (s stubRuntime) ListPeerStats(context.Context) ([]runtime.PeerStat, error) {
	return nil, nil
}

func (s stubRuntime) Health(context.Context) error {
	return s.healthErr
}

func TestReadyUsesOnlyRuntimeHealth(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouter(Options{
		Logger:           logger,
		ReadinessTimeout: time.Second,
		Runtime:          stubRuntime{},
		HMACSecret:       "secret",
		HMACMaxSkew:      time.Minute,
	})

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"ready"`) {
		t.Fatalf("expected ready status in response, got %s", rr.Body.String())
	}
}

func TestReadyReturnsUnavailableWhenRuntimeUnhealthy(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouter(Options{
		Logger:           logger,
		ReadinessTimeout: time.Second,
		Runtime:          stubRuntime{healthErr: errors.New("runtime down")},
		HMACSecret:       "secret",
		HMACMaxSkew:      time.Minute,
	})

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"reason":"runtime"`) {
		t.Fatalf("expected runtime reason in response, got %s", rr.Body.String())
	}
}
