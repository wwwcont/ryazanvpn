package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/domain/access"
	"github.com/wwwcont/ryazanvpn/internal/domain/token"
)

type IssueDeviceConfigInput struct {
	DeviceAccessID   string
	DevicePrivateKey string
	ServerPublicKey  string
	PresharedKey     string
	AssignedIP       string
	DNS              []string
	EndpointHost     string
	EndpointPort     int
	Keepalive        int
	AllowedIPs       []string
	TokenTTL         time.Duration
}

type IssueDeviceConfigOutput struct {
	Token     string
	ExpiresAt time.Time
}

type IssueDeviceConfig struct {
	Accesses  DeviceAccessRepository
	Tokens    ConfigDownloadTokenRepository
	Renderer  ConfigRenderer
	Encryptor EncryptionService
	Now       func() time.Time
}

func (uc IssueDeviceConfig) Execute(ctx context.Context, in IssueDeviceConfigInput) (*IssueDeviceConfigOutput, error) {
	now := time.Now().UTC()
	if uc.Now != nil {
		now = uc.Now().UTC()
	}
	if in.TokenTTL <= 0 {
		in.TokenTTL = 15 * time.Minute
	}

	_, err := uc.Accesses.GetByID(ctx, in.DeviceAccessID)
	if err != nil {
		return nil, err
	}

	cfg, err := uc.Renderer.RenderAmneziaWG(RenderAmneziaWGInput{
		DevicePrivateKey: in.DevicePrivateKey,
		ServerPublicKey:  in.ServerPublicKey,
		PresharedKey:     in.PresharedKey,
		AssignedIP:       in.AssignedIP,
		DNS:              in.DNS,
		EndpointHost:     in.EndpointHost,
		EndpointPort:     in.EndpointPort,
		Keepalive:        in.Keepalive,
		AllowedIPs:       in.AllowedIPs,
	})
	if err != nil {
		return nil, err
	}

	encrypted, err := uc.Encryptor.Encrypt([]byte(cfg))
	if err != nil {
		return nil, err
	}
	if err := uc.Accesses.SetConfigBlobEncrypted(ctx, in.DeviceAccessID, encrypted); err != nil {
		return nil, err
	}

	rawToken, err := randomToken()
	if err != nil {
		return nil, err
	}
	hash := hashToken(rawToken)
	expiresAt := now.Add(in.TokenTTL)

	_, err = uc.Tokens.Create(ctx, token.CreateParams{
		DeviceAccessID: in.DeviceAccessID,
		TokenHash:      hash,
		Status:         token.StatusIssued,
		ExpiresAt:      expiresAt,
	})
	if err != nil {
		return nil, err
	}

	return &IssueDeviceConfigOutput{Token: rawToken, ExpiresAt: expiresAt}, nil
}

type DownloadDeviceConfigByToken struct {
	Tokens    ConfigDownloadTokenRepository
	Accesses  DeviceAccessRepository
	Encryptor EncryptionService
	Now       func() time.Time
}

func (uc DownloadDeviceConfigByToken) Execute(ctx context.Context, rawToken string) (string, error) {
	now := time.Now().UTC()
	if uc.Now != nil {
		now = uc.Now().UTC()
	}

	tok, err := uc.Tokens.GetByTokenHash(ctx, hashToken(rawToken))
	if err != nil {
		return "", err
	}
	if tok.ExpiresAt.Before(now) {
		return "", fmt.Errorf("token expired")
	}
	if tok.UsedAt != nil {
		return "", fmt.Errorf("token already used")
	}

	acc, err := uc.Accesses.GetByID(ctx, tok.DeviceAccessID)
	if err != nil {
		return "", err
	}
	if len(acc.ConfigBlobEncrypted) == 0 {
		return "", fmt.Errorf("config blob is empty")
	}

	plaintext, err := uc.Encryptor.Decrypt(acc.ConfigBlobEncrypted)
	if err != nil {
		return "", err
	}

	if err := uc.Tokens.MarkUsed(ctx, tok.ID, now); err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

var _ = access.StatusActive
