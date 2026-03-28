package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/domain/node"
)

type fakeNodeHealthRepo struct {
	nodes   []*node.Node
	updates map[string]string
}

func (f *fakeNodeHealthRepo) ListAll(ctx context.Context) ([]*node.Node, error) {
	return f.nodes, nil
}

func (f *fakeNodeHealthRepo) UpdateHealth(ctx context.Context, id string, status string, lastSeenAt time.Time) error {
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
		{ID: "n1", Endpoint: healthySrv.URL, Status: node.StatusDown},
		{ID: "n2", Endpoint: unhealthySrv.URL, Status: node.StatusActive},
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
