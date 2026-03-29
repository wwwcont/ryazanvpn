package telegram

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
)

type X25519KeyGenerator struct{}

func (X25519KeyGenerator) Generate(ctx context.Context) (string, string, error) {
	curve := ecdh.X25519()
	priv, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	pub := priv.PublicKey()
	return base64.StdEncoding.EncodeToString(pub.Bytes()), base64.StdEncoding.EncodeToString(priv.Bytes()), nil
}

func (X25519KeyGenerator) GeneratePresharedKey(ctx context.Context) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}
