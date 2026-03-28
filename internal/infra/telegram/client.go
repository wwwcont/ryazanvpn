package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type BotClient interface {
	SendMessage(ctx context.Context, chatID int64, text string, markup *InlineKeyboardMarkup) error
	AnswerCallbackQuery(ctx context.Context, callbackID string, text string) error
}

type HTTPBotClient struct {
	Token   string
	Client  *http.Client
	BaseURL string
}

func (c *HTTPBotClient) SendMessage(ctx context.Context, chatID int64, text string, markup *InlineKeyboardMarkup) error {
	payload := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}
	if markup != nil {
		payload["reply_markup"] = markup
	}
	return c.call(ctx, "sendMessage", payload)
}

func (c *HTTPBotClient) AnswerCallbackQuery(ctx context.Context, callbackID string, text string) error {
	payload := map[string]any{"callback_query_id": callbackID, "text": text}
	return c.call(ctx, "answerCallbackQuery", payload)
}

func (c *HTTPBotClient) call(ctx context.Context, method string, payload any) error {
	if strings.TrimSpace(c.Token) == "" {
		return fmt.Errorf("telegram bot token is empty")
	}

	baseURL := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.telegram.org"
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/bot%s/%s", baseURL, c.Token, method)

	hc := c.Client
	if hc == nil {
		hc = &http.Client{Timeout: 5 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("telegram %s failed: status=%d body=%s", method, resp.StatusCode, string(raw))
	}

	var out struct {
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	if !out.OK {
		return fmt.Errorf("telegram %s response not ok", method)
	}
	return nil
}
