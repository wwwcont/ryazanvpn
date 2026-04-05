package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

type BotClient interface {
	SendMessage(ctx context.Context, chatID int64, text string, markup *InlineKeyboardMarkup) error
	SendDocument(ctx context.Context, chatID int64, filename string, content []byte, caption string, markup *InlineKeyboardMarkup) error
	SendPhoto(ctx context.Context, chatID int64, filename string, content []byte, caption string, markup *InlineKeyboardMarkup) error
	AnswerCallbackQuery(ctx context.Context, callbackID string, text string) error
}

type HTTPBotClient struct {
	Token   string
	Client  *http.Client
	BaseURL string
}

func (c *HTTPBotClient) SendMessage(ctx context.Context, chatID int64, text string, markup *InlineKeyboardMarkup) error {
	startedAt := time.Now()
	slog.Info("telegram.send_message.start", "chat_id", chatID)
	payload := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}
	if markup != nil {
		payload["reply_markup"] = markup
	}
	if err := c.call(ctx, "sendMessage", payload); err != nil {
		slog.Error("telegram.send_message.error", "chat_id", chatID, "duration_ms", time.Since(startedAt).Milliseconds(), "error", err)
		return err
	}
	slog.Info("telegram.send_message.success", "chat_id", chatID, "duration_ms", time.Since(startedAt).Milliseconds())
	return nil
}

func (c *HTTPBotClient) AnswerCallbackQuery(ctx context.Context, callbackID string, text string) error {
	payload := map[string]any{"callback_query_id": callbackID, "text": text}
	return c.call(ctx, "answerCallbackQuery", payload)
}

func (c *HTTPBotClient) SendDocument(ctx context.Context, chatID int64, filename string, content []byte, caption string, markup *InlineKeyboardMarkup) error {
	slog.Info("telegram.send_document.start", "chat_id", chatID, "filename", filename)
	fields := map[string]string{
		"chat_id": fmt.Sprintf("%d", chatID),
	}
	if caption != "" {
		fields["caption"] = caption
	}
	if markup != nil {
		raw, err := json.Marshal(markup)
		if err != nil {
			return err
		}
		fields["reply_markup"] = string(raw)
	}
	if err := c.callMultipart(ctx, "sendDocument", fields, "document", filename, content); err != nil {
		slog.Error("telegram.send_document.error", "chat_id", chatID, "filename", filename, "error", err)
		return err
	}
	slog.Info("telegram.send_document.success", "chat_id", chatID, "filename", filename)
	return nil
}

func (c *HTTPBotClient) SendPhoto(ctx context.Context, chatID int64, filename string, content []byte, caption string, markup *InlineKeyboardMarkup) error {
	fields := map[string]string{
		"chat_id": fmt.Sprintf("%d", chatID),
	}
	if caption != "" {
		fields["caption"] = caption
	}
	if markup != nil {
		raw, err := json.Marshal(markup)
		if err != nil {
			return err
		}
		fields["reply_markup"] = string(raw)
	}
	return c.callMultipart(ctx, "sendPhoto", fields, "photo", filename, content)
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

func (c *HTTPBotClient) callMultipart(ctx context.Context, method string, fields map[string]string, fileField, filename string, content []byte) error {
	if strings.TrimSpace(c.Token) == "" {
		return fmt.Errorf("telegram bot token is empty")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.telegram.org"
	}
	url := fmt.Sprintf("%s/bot%s/%s", baseURL, c.Token, method)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for k, v := range fields {
		_ = writer.WriteField(k, v)
	}
	part, err := writer.CreateFormFile(fileField, filename)
	if err != nil {
		return err
	}
	if _, err := part.Write(content); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	hc := c.Client
	if hc == nil {
		hc = &http.Client{Timeout: 8 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

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
