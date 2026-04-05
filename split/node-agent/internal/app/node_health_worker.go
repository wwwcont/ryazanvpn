package app

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/domain/node"
)

type NodeHealthRepo interface {
	ListAll(ctx context.Context) ([]*node.Node, error)
	UpdateHealth(ctx context.Context, id string, status string, lastSeenAt time.Time) error
}

type NodeHealthWorker struct {
	Logger            *slog.Logger
	Repo              NodeHealthRepo
	Client            *http.Client
	PollInterval      time.Duration
	CheckTimeout      time.Duration
	MaxParallelChecks int
	PollJitter        time.Duration
	RandSource        *rand.Rand
}

func (w NodeHealthWorker) Run(ctx context.Context) {
	if w.Repo == nil {
		return
	}

	interval := w.PollInterval
	if interval <= 0 {
		interval = 15 * time.Second
	}

	client := w.Client
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	w.poll(ctx, client)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if d := w.nextJitterDelay(); d > 0 {
				timer := time.NewTimer(d)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
			w.poll(ctx, client)
		}
	}
}

func (w NodeHealthWorker) poll(ctx context.Context, client *http.Client) {
	nodes, err := w.Repo.ListAll(ctx)
	if err != nil {
		w.log("list nodes failed", slog.Any("error", err))
		return
	}

	now := time.Now().UTC()
	maxParallel := w.MaxParallelChecks
	if maxParallel <= 0 {
		maxParallel = 8
	}
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup

	for _, n := range nodes {
		if n == nil {
			continue
		}
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(n *node.Node) {
			defer wg.Done()
			defer func() { <-sem }()
			checkCtx := ctx
			if w.CheckTimeout > 0 {
				var cancel context.CancelFunc
				checkCtx, cancel = context.WithTimeout(ctx, w.CheckTimeout)
				defer cancel()
			}
			healthy := checkNodeHealth(checkCtx, client, n.AgentBaseURL)
			targetStatus := node.StatusDown
			if healthy {
				targetStatus = node.StatusActive
			}
			if n.Status == targetStatus {
				return
			}
			if err := w.Repo.UpdateHealth(ctx, n.ID, targetStatus, now); err != nil {
				w.log("update node health failed", slog.String("node_id", n.ID), slog.Any("error", err))
				return
			}
			w.log("node health status changed", slog.String("node_id", n.ID), slog.String("status", targetStatus))
		}(n)
	}
	wg.Wait()
}

func checkNodeHealth(ctx context.Context, client *http.Client, agentBaseURL string) bool {
	agentBaseURL = strings.TrimSpace(agentBaseURL)
	if agentBaseURL == "" {
		return false
	}
	healthURL := strings.TrimRight(agentBaseURL, "/") + "/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func (w NodeHealthWorker) log(msg string, attrs ...any) {
	if w.Logger == nil {
		slog.Default().Info(fmt.Sprintf("%s", msg), attrs...)
		return
	}
	w.Logger.Info(msg, attrs...)
}

func (w NodeHealthWorker) nextJitterDelay() time.Duration {
	jitter := w.PollJitter
	if jitter <= 0 {
		return 0
	}
	rng := w.RandSource
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UTC().UnixNano()))
	}
	return time.Duration(rng.Int63n(jitter.Nanoseconds() + 1))
}
