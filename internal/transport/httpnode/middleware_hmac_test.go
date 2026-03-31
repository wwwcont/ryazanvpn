package httpnode

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/agent/auth"
)

func TestHMACAuthMiddleware_ValidSignature(t *testing.T) {
	secret := "test-secret"
	body := `{"hello":"world"}`
	ts := time.Now().UTC().Unix()
	sig := sign(secret, ts, body)

	h := HMACAuthMiddleware(secret, time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/agent/v1/operations/apply-peer", strings.NewReader(body))
	req.Header.Set(auth.HeaderTimestamp, strconv.FormatInt(ts, 10))
	req.Header.Set(auth.HeaderSignature, sig)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHMACAuthMiddleware_InvalidSignature(t *testing.T) {
	secret := "test-secret"
	body := `{"hello":"world"}`
	ts := time.Now().UTC().Unix()

	h := HMACAuthMiddleware(secret, time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/agent/v1/operations/apply-peer", strings.NewReader(body))
	req.Header.Set(auth.HeaderTimestamp, strconv.FormatInt(ts, 10))
	req.Header.Set(auth.HeaderSignature, "deadbeef")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestHMACAuthMiddleware_ExpiredTimestamp(t *testing.T) {
	secret := "test-secret"
	body := `{"hello":"world"}`
	ts := time.Now().UTC().Add(-10 * time.Minute).Unix()
	sig := sign(secret, ts, body)

	h := HMACAuthMiddleware(secret, time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/agent/v1/operations/apply-peer", strings.NewReader(body))
	req.Header.Set(auth.HeaderTimestamp, strconv.FormatInt(ts, 10))
	req.Header.Set(auth.HeaderSignature, sig)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func sign(secret string, ts int64, body string) string {
	return auth.Sign([]byte(secret), strconv.FormatInt(ts, 10), []byte(body))
}
