package httpcontrol

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/wwwcont/ryazanvpn/internal/agent/auth"
)

func TestNodeIdentityMiddleware_RequiresHeaders(t *testing.T) {
	h := nodeIdentityMiddleware("token")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/nodes/desired-state?node_id=n1", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestNodeIdentityMiddleware_InjectsNodeID(t *testing.T) {
	h := nodeIdentityMiddleware("token")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if nodeIDFromContext(r.Context()) != "n1" {
			t.Fatalf("expected node id in context")
		}
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/nodes/desired-state?node_id=n1", nil)
	req.Header.Set("X-Node-Id", "n1")
	req.Header.Set("X-Node-Token", "token")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestNodeAgentAuthMiddleware_RequiresRequestID(t *testing.T) {
	secret := "secret"
	ts := time.Now().UTC().Unix()
	body := "{}"
	sig := auth.Sign([]byte(secret), strconv.FormatInt(ts, 10), []byte(body))
	h := nodeAgentAuthMiddleware(secret, nil, nil, time.Minute, time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/nodes/heartbeat", strings.NewReader(body))
	req.Header.Set(auth.HeaderTimestamp, strconv.FormatInt(ts, 10))
	req.Header.Set(auth.HeaderSignature, sig)
	req.Header.Set("X-Node-Id", "n1")
	req.Header.Set("X-Node-Token", "token")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestNodeAgentAuthMiddleware_ReplayCheckFailure(t *testing.T) {
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	defer redisClient.Close()

	secret := "secret"
	ts := time.Now().UTC().Unix()
	body := `{"node_id":"n1"}`
	sig := auth.Sign([]byte(secret), strconv.FormatInt(ts, 10), []byte(body))
	h := nodeAgentAuthMiddleware(secret, redisClient, nil, time.Minute, time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	makeReq := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/nodes/heartbeat", strings.NewReader(body))
		req.Header.Set(auth.HeaderTimestamp, strconv.FormatInt(ts, 10))
		req.Header.Set(auth.HeaderSignature, sig)
		req.Header.Set("X-Node-Id", "n1")
		req.Header.Set("X-Node-Token", "token")
		req.Header.Set("X-Request-Id", "req-1")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr
	}

	got := makeReq()
	if got.Code != http.StatusUnauthorized {
		t.Fatalf("expected replay check failure 401, got %d", got.Code)
	}
}

func TestRateLimitMiddleware_Returns429WhenExceeded(t *testing.T) {
	limiter := newWindowRateLimiter(1, time.Minute)
	h := rateLimitMiddleware(limiter, func(r *http.Request) string { return "client-1" })(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	first := httptest.NewRecorder()
	h.ServeHTTP(first, req)
	if first.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", first.Code)
	}

	second := httptest.NewRecorder()
	h.ServeHTTP(second, req)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", second.Code)
	}
}
