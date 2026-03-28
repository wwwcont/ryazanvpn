package nodeclient

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

type OperationRequest struct {
	OperationID    string            `json:"operation_id"`
	DeviceAccessID string            `json:"device_access_id"`
	Protocol       string            `json:"protocol"`
	PeerPublicKey  string            `json:"peer_public_key"`
	AssignedIP     string            `json:"assigned_ip"`
	Keepalive      int               `json:"keepalive"`
	EndpointMeta   map[string]string `json:"endpoint_metadata,omitempty"`
}

type Config struct {
	BaseURL    string
	Secret     string
	Timeout    time.Duration
	MaxRetries int
}

type Client struct {
	baseURL    string
	secret     []byte
	httpClient *http.Client
	maxRetries int
}

func New(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{
		baseURL:    cfg.BaseURL,
		secret:     []byte(cfg.Secret),
		httpClient: &http.Client{Timeout: timeout},
		maxRetries: cfg.MaxRetries,
	}
}

func (c *Client) ApplyPeer(ctx context.Context, req OperationRequest) error {
	return c.do(ctx, http.MethodPost, "/agent/v1/operations/apply-peer", req)
}

func (c *Client) RevokePeer(ctx context.Context, req OperationRequest) error {
	return c.do(ctx, http.MethodPost, "/agent/v1/operations/revoke-peer", req)
}

func (c *Client) do(ctx context.Context, method, path string, payload OperationRequest) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	attempts := c.maxRetries + 1
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt-1) * 100 * time.Millisecond):
			}
		}

		ts := strconv.FormatInt(time.Now().UTC().Unix(), 10)
		sig := c.sign(ts, body)

		httpReq, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("X-Agent-Timestamp", ts)
		httpReq.Header.Set("X-Agent-Signature", sig)

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			lastErr = err
			continue
		}

		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("node-agent temporary error: status=%d body=%s", resp.StatusCode, string(respBody))
			continue
		}
		return fmt.Errorf("node-agent request failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	return fmt.Errorf("node-agent request failed after retries: %w", lastErr)
}

func (c *Client) sign(ts string, body []byte) string {
	mac := hmac.New(sha256.New, c.secret)
	_, _ = mac.Write([]byte(ts))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
