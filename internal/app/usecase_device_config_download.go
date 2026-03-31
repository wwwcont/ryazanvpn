package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/domain/access"
	"github.com/wwwcont/ryazanvpn/internal/domain/token"
	"github.com/wwwcont/ryazanvpn/internal/infra/wgkeys"
)

type IssueDeviceConfigInput struct {
	DeviceAccessID   string
	Protocol         string
	DevicePrivateKey string
	DevicePublicKey  string
	ServerPublicKey  string
	PresharedKey     string
	AssignedIP       string
	MTU              int
	DNS              []string
	EndpointHost     string
	EndpointPort     int
	Keepalive        int
	AllowedIPs       []string
	AWG              DefaultVPNAWGFields
	TokenTTL         time.Duration
}

type IssueDeviceConfigOutput struct {
	Token     string
	ExpiresAt time.Time
	QRPayload string
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
	derivedPublicKey, err := wgkeys.DerivePublicKey(in.DevicePrivateKey)
	if err != nil {
		slog.Error("config keypair derivation failed", "device_access_id", in.DeviceAccessID, "error", err)
		return nil, fmt.Errorf("derive public key from private key: %w", err)
	}
	if strings.TrimSpace(in.DevicePublicKey) != "" {
		if err := wgkeys.ValidateKeyPair(in.DevicePrivateKey, in.DevicePublicKey); err != nil {
			slog.Error("config keypair mismatch", "device_access_id", in.DeviceAccessID, "stored_public_key", in.DevicePublicKey, "derived_public_key", derivedPublicKey)
			return nil, fmt.Errorf("device key mismatch: stored public key does not match private key")
		}
	}

	protocol := strings.TrimSpace(in.Protocol)
	if protocol == "" {
		protocol = "wireguard"
	}
	var cfg string
	if protocol == "xray" {
		cfg, err = uc.Renderer.RenderXrayReality(RenderXrayRealityInput{
			DeviceID:     in.DeviceAccessID,
			DevicePublic: derivedPublicKey,
			ServerName:   "www.cloudflare.com",
			ServerHost:   in.EndpointHost,
			ServerPort:   in.EndpointPort,
			UserUUID:     hashToken(in.DeviceAccessID)[:32],
		})
	} else {
		cfg, err = uc.Renderer.RenderAmneziaWG(RenderAmneziaWGInput{
			DevicePrivateKey: in.DevicePrivateKey,
			ServerPublicKey:  in.ServerPublicKey,
			PresharedKey:     in.PresharedKey,
			AssignedIP:       in.AssignedIP,
			MTU:              in.MTU,
			DNS:              in.DNS,
			EndpointHost:     in.EndpointHost,
			EndpointPort:     in.EndpointPort,
			Keepalive:        in.Keepalive,
			AllowedIPs:       in.AllowedIPs,
			AWG:              in.AWG,
		})
	}
	if err != nil {
		if strings.Contains(err.Error(), "missing required fields") {
			slog.Error("config render input incomplete", "device_access_id", in.DeviceAccessID, "error", err)
		}
		return nil, err
	}
	slog.Info("config render input complete", "device_access_id", in.DeviceAccessID, "has_device_private_key", strings.TrimSpace(in.DevicePrivateKey) != "", "has_assigned_ip", strings.TrimSpace(in.AssignedIP) != "", "has_server_public_key", strings.TrimSpace(in.ServerPublicKey) != "", "has_endpoint_host", strings.TrimSpace(in.EndpointHost) != "", "endpoint_port", in.EndpointPort, "has_preshared_key", strings.TrimSpace(in.PresharedKey) != "")

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
	slog.Info("config download token created", "device_access_id", in.DeviceAccessID, "expires_at", expiresAt)

	return &IssueDeviceConfigOutput{Token: rawToken, ExpiresAt: expiresAt, QRPayload: base64.StdEncoding.EncodeToString([]byte(cfg))}, nil
}

type DownloadDeviceConfigByToken struct {
	Tokens    ConfigDownloadTokenRepository
	Accesses  DeviceAccessRepository
	Encryptor EncryptionService
	Now       func() time.Time
}

type ReissueDeviceConfigByProtocolInput struct {
	DeviceID         string
	Protocol         string
	DevicePrivateKey string
	DevicePublicKey  string
	ServerPublicKey  string
	PresharedKey     string
	AssignedIP       string
	EndpointHost     string
	EndpointPort     int
	AllowedIPs       []string
	AWG              DefaultVPNAWGFields
}

type ReissueDeviceConfigByProtocol struct {
	Accesses DeviceAccessRepository
	Issuer   IssueDeviceConfig
}

func (uc ReissueDeviceConfigByProtocol) Execute(ctx context.Context, in ReissueDeviceConfigByProtocolInput) (*IssueDeviceConfigOutput, error) {
	items, err := uc.Accesses.GetActiveByDeviceID(ctx, in.DeviceID)
	if err != nil {
		return nil, err
	}
	for _, acc := range items {
		if strings.EqualFold(strings.TrimSpace(acc.Protocol), strings.TrimSpace(in.Protocol)) {
			return uc.Issuer.Execute(ctx, IssueDeviceConfigInput{
				DeviceAccessID:   acc.ID,
				Protocol:         in.Protocol,
				DevicePrivateKey: in.DevicePrivateKey,
				DevicePublicKey:  in.DevicePublicKey,
				ServerPublicKey:  in.ServerPublicKey,
				PresharedKey:     in.PresharedKey,
				AssignedIP:       in.AssignedIP,
				EndpointHost:     in.EndpointHost,
				EndpointPort:     in.EndpointPort,
				AllowedIPs:       in.AllowedIPs,
				AWG:              in.AWG,
			})
		}
	}
	return nil, access.ErrNotFound
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
