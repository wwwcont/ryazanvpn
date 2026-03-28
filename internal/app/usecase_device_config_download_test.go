package app

import (
	"context"
	"testing"
	"time"

	"github.com/example/ryazanvpn/internal/domain/access"
	"github.com/example/ryazanvpn/internal/domain/token"
)

func TestIssueAndDownloadDeviceConfig(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	accessRepo := &cfgAccessRepo{entry: &access.DeviceAccess{ID: "a1"}}
	tokenRepo := &cfgTokenRepo{}
	encrypt := fakeEncryptor{}

	issue := IssueDeviceConfig{Accesses: accessRepo, Tokens: tokenRepo, Renderer: fakeRenderer{}, Encryptor: encrypt, Now: func() time.Time { return now }}
	issued, err := issue.Execute(context.Background(), IssueDeviceConfigInput{
		DeviceAccessID:   "a1",
		DevicePrivateKey: "priv",
		ServerPublicKey:  "srv",
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
func (r *cfgAccessRepo) GetActiveByDeviceID(context.Context, string) ([]*access.DeviceAccess, error) {
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

type fakeRenderer struct{}

func (fakeRenderer) RenderAmneziaWG(in RenderAmneziaWGInput) (string, error) {
	return "[Interface]\nPrivateKey = " + in.DevicePrivateKey, nil
}

type fakeEncryptor struct{}

func (fakeEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	return append([]byte("enc:"), plaintext...), nil
}
func (fakeEncryptor) Decrypt(ciphertext []byte) ([]byte, error) { return ciphertext[4:], nil }
