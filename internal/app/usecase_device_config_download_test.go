package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/domain/access"
	"github.com/wwwcont/ryazanvpn/internal/domain/token"
)

func TestIssueAndDownloadDeviceConfig(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	accessRepo := &cfgAccessRepo{entry: &access.DeviceAccess{ID: "a1"}}
	tokenRepo := &cfgTokenRepo{}
	encrypt := fakeEncryptor{}

	issue := IssueDeviceConfig{Accesses: accessRepo, Tokens: tokenRepo, Renderer: fakeRenderer{}, Encryptor: encrypt, Now: func() time.Time { return now }}
	issued, err := issue.Execute(context.Background(), IssueDeviceConfigInput{
		DeviceAccessID:   "a1",
		DevicePrivateKey: "FSfGSg9HVUWcRaOzggEUxGafoi8I8JfemfSWLIUhxuI=",
		DevicePublicKey:  "jVcMIlprLo8VEAAXIBMDf08IxK0oRWLSArQryOk0DDE=",
		ServerPublicKey:  "srv",
		PresharedKey:     "psk",
		AssignedIP:       "10.0.0.2/32",
		EndpointHost:     "example.com",
		EndpointPort:     51820,
	})
	if err != nil {
		t.Fatalf("issue error: %v", err)
	}
	if issued.Token == "" {
		t.Fatal("empty token")
	}

	download := DownloadDeviceConfigByToken{Tokens: tokenRepo, Accesses: accessRepo, Encryptor: encrypt, Now: func() time.Time { return now }}
	cfg, err := download.Execute(context.Background(), issued.Token)
	if err != nil {
		t.Fatalf("download error: %v", err)
	}
	if cfg == "" {
		t.Fatal("empty config")
	}
}

func TestIssueDeviceConfig_RejectsMismatchedPublicKey(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	accessRepo := &cfgAccessRepo{entry: &access.DeviceAccess{ID: "a1"}}
	issue := IssueDeviceConfig{Accesses: accessRepo, Tokens: &cfgTokenRepo{}, Renderer: fakeRenderer{}, Encryptor: fakeEncryptor{}, Now: func() time.Time { return now }}

	_, err := issue.Execute(context.Background(), IssueDeviceConfigInput{
		DeviceAccessID:   "a1",
		DevicePrivateKey: "FSfGSg9HVUWcRaOzggEUxGafoi8I8JfemfSWLIUhxuI=",
		DevicePublicKey:  "KS3O5dK5fty5waMzWBFE92ovd3xpOEOEY6P2j84a+Cg=",
		ServerPublicKey:  "srv",
		PresharedKey:     "psk",
		AssignedIP:       "10.0.0.2/32",
		EndpointHost:     "example.com",
		EndpointPort:     51820,
	})
	if err == nil {
		t.Fatal("expected key mismatch error")
	}
}

type cfgAccessRepo struct{ entry *access.DeviceAccess }

func (r *cfgAccessRepo) Create(context.Context, access.CreateParams) (*access.DeviceAccess, error) {
	return r.entry, nil
}
func (r *cfgAccessRepo) GetByID(context.Context, string) (*access.DeviceAccess, error) {
	return r.entry, nil
}
func (r *cfgAccessRepo) Activate(context.Context, string, time.Time) error          { return nil }
func (r *cfgAccessRepo) Revoke(context.Context, string, time.Time) error            { return nil }
func (r *cfgAccessRepo) MarkError(context.Context, string, time.Time, string) error { return nil }
func (r *cfgAccessRepo) SetConfigBlobEncrypted(_ context.Context, _ string, blob []byte) error {
	r.entry.ConfigBlobEncrypted = blob
	return nil
}
func (r *cfgAccessRepo) ClearConfigBlobEncrypted(_ context.Context, _ string) error {
	r.entry.ConfigBlobEncrypted = nil
	return nil
}
func (r *cfgAccessRepo) GetActiveByDeviceID(context.Context, string) ([]*access.DeviceAccess, error) {
	return []*access.DeviceAccess{r.entry}, nil
}
func (r *cfgAccessRepo) GetActiveByNodeAndAssignedIP(context.Context, string, string) (*access.DeviceAccess, error) {
	return r.entry, nil
}
func (r *cfgAccessRepo) ListActiveByNodeID(context.Context, string) ([]*access.DeviceAccess, error) {
	return []*access.DeviceAccess{r.entry}, nil
}

type cfgTokenRepo struct {
	entry *token.ConfigDownloadToken
	hash  string
}

func (r *cfgTokenRepo) Create(ctx context.Context, in token.CreateParams) (*token.ConfigDownloadToken, error) {
	r.hash = in.TokenHash
	r.entry = &token.ConfigDownloadToken{ID: "t1", DeviceAccessID: in.DeviceAccessID, TokenHash: in.TokenHash, ExpiresAt: in.ExpiresAt}
	return r.entry, nil
}
func (r *cfgTokenRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*token.ConfigDownloadToken, error) {
	if tokenHash != r.hash {
		return nil, token.ErrNotFound
	}
	return r.entry, nil
}
func (r *cfgTokenRepo) MarkUsed(ctx context.Context, id string, usedAt time.Time) error {
	r.entry.UsedAt = &usedAt
	return nil
}
func (r *cfgTokenRepo) RevokeIssuedByAccessID(ctx context.Context, deviceAccessID string, revokedAt time.Time) error {
	return nil
}

type fakeRenderer struct{}

func (fakeRenderer) RenderAmneziaWG(in RenderAmneziaWGInput) (string, error) {
	return "[Interface]\nPrivateKey = " + in.DevicePrivateKey + "\n[Peer]\nPublicKey = " + in.ServerPublicKey + "\nPresharedKey = " + in.PresharedKey + "\n", nil
}
func (fakeRenderer) RenderXrayReality(in RenderXrayRealityInput) (string, error) {
	return "{\"protocol\":\"xray\"}", nil
}

type trackingRenderer struct {
	amneziaCalls int
	xrayCalls    int
}

func (r *trackingRenderer) RenderAmneziaWG(in RenderAmneziaWGInput) (string, error) {
	r.amneziaCalls++
	return "[Interface]\nPrivateKey = " + in.DevicePrivateKey + "\n[Peer]\nPublicKey = " + in.ServerPublicKey + "\nPresharedKey = " + in.PresharedKey + "\n", nil
}

func (r *trackingRenderer) RenderXrayReality(in RenderXrayRealityInput) (string, error) {
	r.xrayCalls++
	return "{\"id\":\"x\"}", nil
}

func TestIssueDeviceConfig_XrayDoesNotUseWGRenderer(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	accessRepo := &cfgAccessRepo{entry: &access.DeviceAccess{ID: "a1"}}
	renderer := &trackingRenderer{}
	issue := IssueDeviceConfig{Accesses: accessRepo, Tokens: &cfgTokenRepo{}, Renderer: renderer, Encryptor: fakeEncryptor{}, Now: func() time.Time { return now }}

	_, err := issue.Execute(context.Background(), IssueDeviceConfigInput{
		DeviceAccessID: "a1",
		Protocol:       "xray",
		EndpointHost:   "x.example.com",
		EndpointPort:   8443,
		XrayServerName: "www.cloudflare.com",
	})
	if err != nil {
		t.Fatalf("issue xray config failed: %v", err)
	}
	if renderer.amneziaCalls != 0 {
		t.Fatalf("wg renderer must not be called for xray flow, calls=%d", renderer.amneziaCalls)
	}
	if renderer.xrayCalls != 1 {
		t.Fatalf("xray renderer calls=%d, want 1", renderer.xrayCalls)
	}
}

func TestIssueDeviceConfig_WireguardDoesNotUseXrayRenderer(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	accessRepo := &cfgAccessRepo{entry: &access.DeviceAccess{ID: "a1"}}
	renderer := &trackingRenderer{}
	issue := IssueDeviceConfig{Accesses: accessRepo, Tokens: &cfgTokenRepo{}, Renderer: renderer, Encryptor: fakeEncryptor{}, Now: func() time.Time { return now }}

	_, err := issue.Execute(context.Background(), IssueDeviceConfigInput{
		DeviceAccessID:   "a1",
		Protocol:         "wireguard",
		DevicePrivateKey: "FSfGSg9HVUWcRaOzggEUxGafoi8I8JfemfSWLIUhxuI=",
		DevicePublicKey:  "jVcMIlprLo8VEAAXIBMDf08IxK0oRWLSArQryOk0DDE=",
		ServerPublicKey:  "srv",
		PresharedKey:     "psk",
		AssignedIP:       "10.0.0.2/32",
		EndpointHost:     "example.com",
		EndpointPort:     51820,
	})
	if err != nil {
		t.Fatalf("issue wireguard config failed: %v", err)
	}
	if renderer.xrayCalls != 0 {
		t.Fatalf("xray renderer must not be called for wireguard flow, calls=%d", renderer.xrayCalls)
	}
	if renderer.amneziaCalls != 1 {
		t.Fatalf("wg renderer calls=%d, want 1", renderer.amneziaCalls)
	}
}

func TestIssueDeviceConfig_RejectsRenderedServerPublicKeyMismatch(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	accessRepo := &cfgAccessRepo{entry: &access.DeviceAccess{ID: "a1"}}
	issue := IssueDeviceConfig{
		Accesses:  accessRepo,
		Tokens:    &cfgTokenRepo{},
		Renderer:  mismatchServerKeyRenderer{},
		Encryptor: fakeEncryptor{},
		Now:       func() time.Time { return now },
	}

	_, err := issue.Execute(context.Background(), IssueDeviceConfigInput{
		DeviceAccessID:   "a1",
		Protocol:         "wireguard",
		DevicePrivateKey: "FSfGSg9HVUWcRaOzggEUxGafoi8I8JfemfSWLIUhxuI=",
		DevicePublicKey:  "jVcMIlprLo8VEAAXIBMDf08IxK0oRWLSArQryOk0DDE=",
		ServerPublicKey:  "expectedSrv",
		PresharedKey:     "psk",
		AssignedIP:       "10.0.0.2/32",
		EndpointHost:     "example.com",
		EndpointPort:     51820,
	})
	if err == nil {
		t.Fatal("expected rendered server key mismatch error")
	}
}

type mismatchServerKeyRenderer struct{}

func (mismatchServerKeyRenderer) RenderAmneziaWG(in RenderAmneziaWGInput) (string, error) {
	return "[Interface]\nPrivateKey = " + in.DevicePrivateKey + "\n[Peer]\nPublicKey = wrongSrv\nPresharedKey = " + in.PresharedKey + "\n", nil
}

func (mismatchServerKeyRenderer) RenderXrayReality(in RenderXrayRealityInput) (string, error) {
	return "{\"protocol\":\"xray\"}", nil
}

type mismatchPresharedKeyRenderer struct{}

func (mismatchPresharedKeyRenderer) RenderAmneziaWG(in RenderAmneziaWGInput) (string, error) {
	return "[Interface]\nPrivateKey = " + in.DevicePrivateKey + "\n[Peer]\nPublicKey = " + in.ServerPublicKey + "\nPresharedKey = wrongPsk\n", nil
}

func (mismatchPresharedKeyRenderer) RenderXrayReality(in RenderXrayRealityInput) (string, error) {
	return "{\"protocol\":\"xray\"}", nil
}

func TestIssueDeviceConfig_RejectsRenderedPresharedKeyMismatch(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	accessRepo := &cfgAccessRepo{entry: &access.DeviceAccess{ID: "a1"}}
	issue := IssueDeviceConfig{
		Accesses:  accessRepo,
		Tokens:    &cfgTokenRepo{},
		Renderer:  mismatchPresharedKeyRenderer{},
		Encryptor: fakeEncryptor{},
		Now:       func() time.Time { return now },
	}
	_, err := issue.Execute(context.Background(), IssueDeviceConfigInput{
		DeviceAccessID:   "a1",
		Protocol:         "wireguard",
		DevicePrivateKey: "FSfGSg9HVUWcRaOzggEUxGafoi8I8JfemfSWLIUhxuI=",
		DevicePublicKey:  "jVcMIlprLo8VEAAXIBMDf08IxK0oRWLSArQryOk0DDE=",
		ServerPublicKey:  "srv",
		PresharedKey:     "psk",
		AssignedIP:       "10.0.0.2/32",
		EndpointHost:     "example.com",
		EndpointPort:     51820,
	})
	if err == nil {
		t.Fatal("expected rendered preshared key mismatch error")
	}
}

type amneziaFieldRenderer struct{}

func (amneziaFieldRenderer) RenderAmneziaWG(in RenderAmneziaWGInput) (string, error) {
	return "[Interface]\nPrivateKey = " + in.DevicePrivateKey + "\nJc = 4\nH1 = custom\nI1 = payload\n[Peer]\nPublicKey = " + in.ServerPublicKey + "\nPresharedKey = " + in.PresharedKey + "\n", nil
}

func (amneziaFieldRenderer) RenderXrayReality(in RenderXrayRealityInput) (string, error) {
	return "{\"protocol\":\"xray\"}", nil
}

func TestIssueDeviceConfig_KeepsAmneziaCustomParams(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	accessRepo := &cfgAccessRepo{entry: &access.DeviceAccess{ID: "a1"}}
	encrypt := fakeEncryptor{}
	issue := IssueDeviceConfig{
		Accesses:  accessRepo,
		Tokens:    &cfgTokenRepo{},
		Renderer:  amneziaFieldRenderer{},
		Encryptor: encrypt,
		Now:       func() time.Time { return now },
	}

	_, err := issue.Execute(context.Background(), IssueDeviceConfigInput{
		DeviceAccessID:   "a1",
		Protocol:         "wireguard",
		DevicePrivateKey: "FSfGSg9HVUWcRaOzggEUxGafoi8I8JfemfSWLIUhxuI=",
		DevicePublicKey:  "jVcMIlprLo8VEAAXIBMDf08IxK0oRWLSArQryOk0DDE=",
		ServerPublicKey:  "srv",
		PresharedKey:     "psk",
		AssignedIP:       "10.0.0.2/32",
		EndpointHost:     "example.com",
		EndpointPort:     51820,
		AWG:              DefaultVPNAWGFields{Jc: 4, H1: "custom", I1: "payload"},
	})
	if err != nil {
		t.Fatalf("issue wireguard config failed: %v", err)
	}
	plain, err := encrypt.Decrypt(accessRepo.entry.ConfigBlobEncrypted)
	if err != nil {
		t.Fatalf("decrypt config failed: %v", err)
	}
	config := string(plain)
	if !strings.Contains(config, "Jc = 4") || !strings.Contains(config, "H1 = custom") || !strings.Contains(config, "I1 = payload") {
		t.Fatalf("expected custom amnezia params to be preserved, got config: %s", config)
	}
}

type fakeEncryptor struct{}

func (fakeEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	return append([]byte("enc:"), plaintext...), nil
}
func (fakeEncryptor) Decrypt(ciphertext []byte) ([]byte, error) { return ciphertext[4:], nil }
