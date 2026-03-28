package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/domain/node"
)

type NodeHealthRepo interface {
	ListAll(ctx context.Context) ([]*node.Node, error)
	UpdateHealth(ctx context.Context, id string, status string, lastSeenAt time.Time) error
}

type NodeHealthWorker struct {
	Logger       *slog.Logger
	Repo         NodeHealthRepo
	Client       *http.Client
	PollInterval time.Duration
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
	for _, n := range nodes {
		healthy := checkNodeHealth(ctx, client, n.AgentBaseURL)
		targetStatus := node.StatusDown
		if healthy {
			targetStatus = node.StatusActive
		}

		if n.Status == targetStatus {
			continue
		}

		if err := w.Repo.UpdateHealth(ctx, n.ID, targetStatus, now); err != nil {
			w.log("update node health failed", slog.String("node_id", n.ID), slog.Any("error", err))
			continue
		}
		w.log("node health status changed", slog.String("node_id", n.ID), slog.String("status", targetStatus))
	}
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
