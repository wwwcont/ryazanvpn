package app

import (
	"context"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/domain/node"
)

type fakeNodeHealthRepo struct {
	nodes   []*node.Node
	updates map[string]string
	mu      sync.Mutex
}

func (f *fakeNodeHealthRepo) ListAll(ctx context.Context) ([]*node.Node, error) {
	return f.nodes, nil
}

func (f *fakeNodeHealthRepo) UpdateHealth(ctx context.Context, id string, status string, lastSeenAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.updates == nil {
		f.updates = make(map[string]string)
	}
	f.updates[id] = status
	return nil
}

func TestNodeHealthWorker_Poll(t *testing.T) {
	healthySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthySrv.Close()

	unhealthySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer unhealthySrv.Close()

	repo := &fakeNodeHealthRepo{nodes: []*node.Node{
		{ID: "n1", AgentBaseURL: healthySrv.URL, Status: node.StatusDown},
		{ID: "n2", AgentBaseURL: unhealthySrv.URL, Status: node.StatusActive},
	}}

	worker := NodeHealthWorker{Repo: repo, PollInterval: time.Hour}
	worker.poll(context.Background(), &http.Client{Timeout: time.Second})

	if repo.updates["n1"] != node.StatusActive {
		t.Fatalf("expected n1 -> active, got %q", repo.updates["n1"])
	}
	if repo.updates["n2"] != node.StatusDown {
		t.Fatalf("expected n2 -> down, got %q", repo.updates["n2"])
	}
}

func TestNodeHealthWorker_Poll_CheckTimeoutAndParallelLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	repo := &fakeNodeHealthRepo{nodes: []*node.Node{
		{ID: "n1", AgentBaseURL: srv.URL, Status: node.StatusActive},
		{ID: "n2", AgentBaseURL: srv.URL, Status: node.StatusActive},
		{ID: "n3", AgentBaseURL: srv.URL, Status: node.StatusActive},
	}}

	worker := NodeHealthWorker{
		Repo:              repo,
		PollInterval:      time.Hour,
		CheckTimeout:      10 * time.Millisecond,
		MaxParallelChecks: 2,
	}
	worker.poll(context.Background(), &http.Client{Timeout: time.Second})

	if got := repo.updates["n1"]; got != node.StatusDown {
		t.Fatalf("expected n1 -> down due to timeout, got %q", got)
	}
	if got := repo.updates["n2"]; got != node.StatusDown {
		t.Fatalf("expected n2 -> down due to timeout, got %q", got)
	}
	if got := repo.updates["n3"]; got != node.StatusDown {
		t.Fatalf("expected n3 -> down due to timeout, got %q", got)
	}
}

func TestNodeHealthWorker_NextJitterDelay(t *testing.T) {
	worker := NodeHealthWorker{
		PollJitter: 3 * time.Second,
		RandSource: rand.New(rand.NewSource(7)),
	}
	for i := 0; i < 20; i++ {
		d := worker.nextJitterDelay()
		if d < 0 || d > worker.PollJitter {
			t.Fatalf("jitter delay out of bounds: %s", d)
		}
	}
}

func TestNodeHealthWorker_Poll_RespectsMaxParallelChecks(t *testing.T) {
	var active int64
	var maxSeen int64
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			cur := atomic.AddInt64(&active, 1)
			for {
				seen := atomic.LoadInt64(&maxSeen)
				if cur <= seen || atomic.CompareAndSwapInt64(&maxSeen, seen, cur) {
					break
				}
			}
			time.Sleep(30 * time.Millisecond)
			atomic.AddInt64(&active, -1)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}),
	}

	repo := &fakeNodeHealthRepo{nodes: []*node.Node{
		{ID: "n1", AgentBaseURL: "http://node-1", Status: node.StatusDown},
		{ID: "n2", AgentBaseURL: "http://node-2", Status: node.StatusDown},
		{ID: "n3", AgentBaseURL: "http://node-3", Status: node.StatusDown},
	}}
	worker := NodeHealthWorker{
		Repo:              repo,
		MaxParallelChecks: 2,
	}
	worker.poll(context.Background(), client)

	if maxSeen > 2 {
		t.Fatalf("expected at most 2 concurrent checks, got %d", maxSeen)
	}
	if repo.updates["n1"] != node.StatusActive || repo.updates["n2"] != node.StatusActive || repo.updates["n3"] != node.StatusActive {
		t.Fatalf("expected all nodes updated to active, got %+v", repo.updates)
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
