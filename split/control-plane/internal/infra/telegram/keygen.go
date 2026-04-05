package telegram

import (
	"context"
	"crypto/rand"
	"encoding/base64"

	"github.com/wwwcont/ryazanvpn/internal/infra/wgkeys"
)

type X25519KeyGenerator struct{}

func (X25519KeyGenerator) Generate(ctx context.Context) (string, string, error) {
	return wgkeys.GenerateKeyPair()
}

func (X25519KeyGenerator) GeneratePresharedKey(ctx context.Context) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}
