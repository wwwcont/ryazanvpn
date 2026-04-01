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
	XrayUserUUID     string
	XrayServerName   string
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
	slog.Info("issue_device_config.start", "access_id", in.DeviceAccessID, "protocol", in.Protocol)
	now := time.Now().UTC()
	if uc.Now != nil {
		now = uc.Now().UTC()
	}
	if in.TokenTTL <= 0 {
		in.TokenTTL = 15 * time.Minute
	}

	acc, err := uc.Accesses.GetByID(ctx, in.DeviceAccessID)
	if err != nil {
		slog.Error("issue_device_config.error", "access_id", in.DeviceAccessID, "protocol", in.Protocol, "error", err)
		return nil, err
	}
	protocol := strings.TrimSpace(in.Protocol)
	if protocol == "" {
		protocol = "wireguard"
	}
	if strings.EqualFold(protocol, "amnezia") || strings.EqualFold(protocol, "amneziawg") {
		protocol = "wireguard"
	}
	protocol = strings.ToLower(protocol)
	var cfg string
	switch protocol {
	case "xray":
		userUUID := strings.TrimSpace(in.XrayUserUUID)
		if userUUID == "" {
			userUUID = hashToken(in.DeviceAccessID)[:32]
		}
		cfg, err = uc.Renderer.RenderXrayReality(RenderXrayRealityInput{
			DeviceID:     in.DeviceAccessID,
			DevicePublic: strings.TrimSpace(in.DevicePublicKey),
			ServerName:   strings.TrimSpace(in.XrayServerName),
			ServerHost:   in.EndpointHost,
			ServerPort:   in.EndpointPort,
			UserUUID:     userUUID,
		})
	case "wireguard":
		slog.Info("wg.config.render.access_id", "access_id", in.DeviceAccessID)
		slog.Info("wg.config.render.server_public_key", "access_id", in.DeviceAccessID, "server_public_key", strings.TrimSpace(in.ServerPublicKey))
		derivedPublicKey, derErr := wgkeys.DerivePublicKey(in.DevicePrivateKey)
		if derErr != nil {
			slog.Error("config keypair derivation failed", "device_access_id", in.DeviceAccessID, "error", derErr)
			slog.Error("issue_device_config.error", "access_id", in.DeviceAccessID, "protocol", in.Protocol, "error", derErr)
			return nil, fmt.Errorf("derive public key from private key: %w", derErr)
		}
		if strings.TrimSpace(in.DevicePublicKey) != "" {
			if valErr := wgkeys.ValidateKeyPair(in.DevicePrivateKey, in.DevicePublicKey); valErr != nil {
				slog.Error("config keypair mismatch", "device_access_id", in.DeviceAccessID, "stored_public_key", in.DevicePublicKey, "derived_public_key", derivedPublicKey)
				slog.Error("issue_device_config.error", "access_id", in.DeviceAccessID, "protocol", in.Protocol, "error", valErr)
				return nil, fmt.Errorf("device key mismatch: stored public key does not match private key")
			}
		}
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
	default:
		err = fmt.Errorf("unsupported protocol: %s", protocol)
	}
	if err != nil {
		if strings.Contains(err.Error(), "missing required fields") {
			slog.Error("config render input incomplete", "device_access_id", in.DeviceAccessID, "error", err)
		}
		slog.Error("issue_device_config.error", "access_id", in.DeviceAccessID, "protocol", protocol, "error", err)
		return nil, err
	}
	if protocol == "wireguard" {
		renderedServerPublicKey, parseErr := extractWireguardPeerPublicKey(cfg)
		if parseErr != nil {
			slog.Error("wg.config.render.server_public_key", "access_id", in.DeviceAccessID, "error", parseErr)
			slog.Error("issue_device_config.error", "access_id", in.DeviceAccessID, "protocol", protocol, "error", parseErr)
			return nil, parseErr
		}
		expected := strings.TrimSpace(in.ServerPublicKey)
		if expected != "" && !strings.EqualFold(renderedServerPublicKey, expected) {
			err = fmt.Errorf("rendered wireguard server public key mismatch: rendered=%q expected=%q", renderedServerPublicKey, expected)
			slog.Error("wg.config.render.server_public_key", "access_id", in.DeviceAccessID, "error", err)
			slog.Error("issue_device_config.error", "access_id", in.DeviceAccessID, "protocol", protocol, "error", err)
			return nil, err
		}
	}
	slog.Info("config render input complete", "device_access_id", in.DeviceAccessID, "has_device_private_key", strings.TrimSpace(in.DevicePrivateKey) != "", "has_assigned_ip", strings.TrimSpace(in.AssignedIP) != "", "has_server_public_key", strings.TrimSpace(in.ServerPublicKey) != "", "has_endpoint_host", strings.TrimSpace(in.EndpointHost) != "", "endpoint_port", in.EndpointPort, "has_preshared_key", strings.TrimSpace(in.PresharedKey) != "")

	encrypted, err := uc.Encryptor.Encrypt([]byte(cfg))
	if err != nil {
		slog.Error("issue_device_config.error", "access_id", in.DeviceAccessID, "protocol", protocol, "error", err)
		return nil, err
	}
	if err := uc.Accesses.SetConfigBlobEncrypted(ctx, in.DeviceAccessID, encrypted); err != nil {
		slog.Error("issue_device_config.error", "access_id", in.DeviceAccessID, "protocol", protocol, "error", err)
		return nil, err
	}
	if protocol == "wireguard" {
		slog.Info("wg.config.render.blob_regenerated", "access_id", in.DeviceAccessID, "value", len(acc.ConfigBlobEncrypted) > 0)
	}

	rawToken, err := randomToken()
	if err != nil {
		slog.Error("issue_device_config.error", "access_id", in.DeviceAccessID, "protocol", protocol, "error", err)
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
		slog.Error("issue_device_config.error", "access_id", in.DeviceAccessID, "protocol", protocol, "error", err)
		return nil, err
	}
	slog.Info("config download token created", "device_access_id", in.DeviceAccessID, "expires_at", expiresAt)
	slog.Info("issue_device_config.success", "access_id", in.DeviceAccessID, "protocol", protocol, "expires_at", expiresAt)

	return &IssueDeviceConfigOutput{Token: rawToken, ExpiresAt: expiresAt, QRPayload: base64.StdEncoding.EncodeToString([]byte(cfg))}, nil
}

func extractWireguardPeerPublicKey(config string) (string, error) {
	inPeerSection := false
	for _, raw := range strings.Split(config, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inPeerSection = strings.EqualFold(line, "[Peer]")
			continue
		}
		if !inPeerSection {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(line), "publickey") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid wireguard public key line")
		}
		key := strings.TrimSpace(parts[1])
		if key == "" {
			return "", fmt.Errorf("wireguard peer public key is empty")
		}
		return key, nil
	}
	return "", fmt.Errorf("wireguard peer public key not found in rendered config")
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
