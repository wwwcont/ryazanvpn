package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const webhookUpdateTimeout = 55 * time.Second

type WebhookHandler struct {
	SecretToken string
	Service     *TelegramService
}

func (h WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if strings.TrimSpace(h.SecretToken) != "" {
		if r.Header.Get("X-Telegram-Bot-Api-Secret-Token") != h.SecretToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}
	if h.Service == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	defer r.Body.Close()
	var upd Update
	if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
	go func(update Update) {
		ctx, cancel := context.WithTimeout(context.Background(), webhookUpdateTimeout)
		defer cancel()
		h.Service.HandleUpdate(ctx, update)
	}(upd)
}
