package telegram

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebhookRejectsInvalidSecret(t *testing.T) {
	h := WebhookHandler{SecretToken: "secret", Service: &TelegramService{}}
	req := httptest.NewRequest(http.MethodPost, "/internal/telegram/webhook", bytes.NewBufferString(`{"update_id":1}`))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestWebhookAcceptsUpdate(t *testing.T) {
	h := WebhookHandler{Service: &TelegramService{}}
	req := httptest.NewRequest(http.MethodPost, "/internal/telegram/webhook", bytes.NewBufferString(`{"update_id":1}`))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
