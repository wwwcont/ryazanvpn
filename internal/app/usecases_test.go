package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/domain/access"
	"github.com/wwwcont/ryazanvpn/internal/domain/accessgrant"
	"github.com/wwwcont/ryazanvpn/internal/domain/audit"
	"github.com/wwwcont/ryazanvpn/internal/domain/device"
	"github.com/wwwcont/ryazanvpn/internal/domain/invitecode"
	"github.com/wwwcont/ryazanvpn/internal/domain/node"
	"github.com/wwwcont/ryazanvpn/internal/domain/operation"
	"github.com/wwwcont/ryazanvpn/internal/domain/token"
	"github.com/wwwcont/ryazanvpn/internal/domain/user"
	"github.com/wwwcont/ryazanvpn/internal/infra/wgkeys"
)

func TestRegisterTelegramUser_CreateWhenMissing(t *testing.T) {
	repo := &fakeUsers{getByTelegramErr: user.ErrNotFound, created: &user.User{ID: "u1", TelegramID: 1}}
	uc := RegisterTelegramUser{Users: repo}

	got, err := uc.Execute(context.Background(), RegisterTelegramUserInput{TelegramID: 1, Username: "alice"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.ID != "u1" {
		t.Fatalf("unexpected id: %s", got.ID)
	}
}

func TestRegisterTelegramUser_CreateWhenMissingWrappedNotFound(t *testing.T) {
	repo := &fakeUsers{getByTelegramErr: errors.Join(user.ErrNotFound, errors.New("db: no rows")), created: &user.User{ID: "u2", TelegramID: 2}}
	uc := RegisterTelegramUser{Users: repo}

	got, err := uc.Execute(context.Background(), RegisterTelegramUserInput{TelegramID: 2, Username: "bob"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.ID != "u2" {
		t.Fatalf("unexpected id: %s", got.ID)
	}
}

func TestActivateInviteCode_Validation(t *testing.T) {
	uc := ActivateInviteCode{Store: &fakeActivateStore{}}
	if err := uc.Execute(context.Background(), ActivateInviteCodeInput{UserID: "u1", Code: "12a4"}); !errors.Is(err, ErrInvalidInviteCodeFormat) {
		t.Fatalf("expected format error, got %v", err)
	}
}

func TestActivateInviteCode_Success(t *testing.T) {
	store := fakeActivateStore{}
	uc := ActivateInviteCode{Store: &store, Now: func() time.Time { return time.Unix(100, 0).UTC() }}
	if err := uc.Execute(context.Background(), ActivateInviteCodeInput{UserID: "u1", Code: "1111"}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if store.accessGrants == nil || store.accessGrants.lastCreate == nil {
		t.Fatal("expected access grant creation")
	}
	if got, want := store.accessGrants.lastCreate.ExpiresAt, store.accessGrants.lastCreate.ActivatedAt.Add(30*24*time.Hour); !got.Equal(want) {
		t.Fatalf("expected expires_at %v, got %v", want, got)
	}
}

func TestActivateInviteCode_InviteBonus25000(t *testing.T) {
	store := fakeActivateStore{}
	fin := &fakeInviteFinance{}
	uc := ActivateInviteCode{Store: &store, Now: func() time.Time { return time.Unix(100, 0).UTC() }, Finance: fin}
	if err := uc.Execute(context.Background(), ActivateInviteCodeInput{UserID: "u1", Code: "1111"}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if fin.amount != 25_000 {
		t.Fatalf("expected invite bonus 25000 kopecks, got %d", fin.amount)
	}
}

func TestActivateInviteCode_RejectWhenUserHasActiveAccessGrant(t *testing.T) {
	uc := ActivateInviteCode{Store: &fakeActivateStore{activeAccessGrant: true}, Now: func() time.Time { return time.Unix(100, 0) }}
	err := uc.Execute(context.Background(), ActivateInviteCodeInput{UserID: "u1", Code: "1111"})
	if !errors.Is(err, ErrUserAlreadyHasActiveAccessGrant) {
		t.Fatalf("expected ErrUserAlreadyHasActiveAccessGrant, got %v", err)
	}
}

func TestExpireAccessGrants(t *testing.T) {
	repo := &fakeAccessGrants{expiredCount: 3}
	uc := ExpireAccessGrants{AccessGrants: repo, Now: func() time.Time { return time.Unix(1_700_000_000, 0) }}

	count, err := uc.Execute(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 expired rows, got %d", count)
	}
}

func TestGetActiveAccessGrantByUser(t *testing.T) {
	repo := &fakeAccessGrants{activeByUser: &accessgrant.AccessGrant{ID: "ag-1", UserID: "u1", Status: accessgrant.StatusActive}}
	uc := GetActiveAccessGrantByUser{AccessGrants: repo}

	grant, err := uc.Execute(context.Background(), GetActiveAccessGrantByUserInput{UserID: "u1"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if grant.ID != "ag-1" {
		t.Fatalf("unexpected grant id: %s", grant.ID)
	}
}

func TestCreateDeviceForUser_AssignsMinLoadNode(t *testing.T) {
	devRepo := &fakeDevices{activeErr: device.ErrNotFound, created: &device.Device{ID: "d1", UserID: "u1"}}
	nodes := &fakeNodes{active: []*node.Node{{ID: "n1", CurrentLoad: 5, UserCapacity: 40, Status: node.StatusActive}, {ID: "n2", CurrentLoad: 1, UserCapacity: 40, Status: node.StatusActive}}}
	accessRepo := &fakeAccess{created: &access.DeviceAccess{ID: "a1", DeviceID: "d1", VPNNodeID: "n2"}}
	opRepo := &fakeOps{}
	auditRepo := &fakeAudit{}

	uc := CreateDeviceForUser{
		Devices:       devRepo,
		Nodes:         nodes,
		Accesses:      accessRepo,
		Operations:    opRepo,
		AuditLogs:     auditRepo,
		KeyGenerator:  fakeKeys{},
		PresharedKeys: fakeKeys{},
		IPAllocator:   fakeIPAlloc{ip: "10.10.0.2"},
		NodeAssigner:  MinLoadNodeAssigner{},
	}

	out, err := uc.Execute(context.Background(), CreateDeviceForUserInput{UserID: "u1", Name: "iphone"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out.Node.ID != "n2" {
		t.Fatalf("expected n2, got %s", out.Node.ID)
	}
	if out.PrivateKey == "" {
		t.Fatal("expected private key")
	}
}

func TestCreateDeviceForUser_RejectsMismatchedKeypair(t *testing.T) {
	uc := CreateDeviceForUser{
		Devices:       &fakeDevices{activeErr: device.ErrNotFound, created: &device.Device{ID: "d1", UserID: "u1"}},
		Nodes:         &fakeNodes{active: []*node.Node{{ID: "n1", Status: node.StatusActive}}},
		Accesses:      &fakeAccess{created: &access.DeviceAccess{ID: "a1", DeviceID: "d1", VPNNodeID: "n1"}},
		Operations:    &fakeOps{},
		AuditLogs:     &fakeAudit{},
		KeyGenerator:  fakeBrokenKeys{},
		PresharedKeys: fakeKeys{},
		IPAllocator:   fakeIPAlloc{ip: "10.10.0.2/32"},
		NodeAssigner:  MinLoadNodeAssigner{},
	}

	if _, err := uc.Execute(context.Background(), CreateDeviceForUserInput{UserID: "u1", Name: "iphone"}); err == nil {
		t.Fatal("expected error for mismatched keypair")
	}
}

func TestCreateDeviceForUser_PeerPayloadUsesDerivedPublicKey(t *testing.T) {
	devRepo := &fakeDevices{activeErr: device.ErrNotFound, created: &device.Device{ID: "d1", UserID: "u1"}}
	opRepo := &fakeOps{}
	uc := CreateDeviceForUser{
		Devices:       devRepo,
		Nodes:         &fakeNodes{active: []*node.Node{{ID: "n1", CurrentLoad: 1, UserCapacity: 40, Status: node.StatusActive}}},
		Accesses:      &fakeAccess{created: &access.DeviceAccess{ID: "a1", DeviceID: "d1", VPNNodeID: "n1"}},
		Operations:    opRepo,
		AuditLogs:     &fakeAudit{},
		KeyGenerator:  fakeKeys{},
		PresharedKeys: fakeKeys{},
		IPAllocator:   fakeIPAlloc{ip: "10.10.0.2/32"},
		NodeAssigner:  MinLoadNodeAssigner{},
	}

	out, err := uc.Execute(context.Background(), CreateDeviceForUserInput{UserID: "u1", Name: "iphone"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	derived, err := wgkeys.DerivePublicKey(out.PrivateKey)
	if err != nil {
		t.Fatalf("derive public key: %v", err)
	}
	if devRepo.lastCreate == nil || devRepo.lastCreate.PublicKey != derived {
		t.Fatalf("device public key must match derived public key: got=%q want=%q", devRepo.lastCreate.PublicKey, derived)
	}
	if opRepo.lastCreate == nil {
		t.Fatal("expected operation payload")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(opRepo.lastCreate.PayloadJSON), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	got, _ := payload["public_key"].(string)
	if got == "" {
		t.Fatal("operation peer public key must not be empty")
	}
	if protocol, _ := payload["protocol"].(string); strings.EqualFold(protocol, "wireguard") && got != derived {
		t.Fatalf("wireguard operation peer public key mismatch: got=%q want=%q", got, derived)
	}
}

func TestRevokeDeviceAccess_Confirmed(t *testing.T) {
	accessRepo := &fakeAccess{byID: &access.DeviceAccess{ID: "a1", DeviceID: "d1", VPNNodeID: "n1"}}
	opRepo := &fakeOps{created: &operation.NodeOperation{ID: "op1"}}
	tokenRepo := &fakeTokens{}
	uc := RevokeDeviceAccess{Accesses: accessRepo, Operations: opRepo, AuditLogs: &fakeAudit{}, Tokens: tokenRepo}

	if err := uc.Execute(context.Background(), RevokeDeviceAccessInput{AccessID: "a1"}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if accessRepo.revoked {
		t.Fatal("did not expect direct revoke without executor")
	}
	if !accessRepo.configCleared {
		t.Fatal("expected config blob to be cleared")
	}
	if !tokenRepo.revoked {
		t.Fatal("expected issued tokens to be revoked")
	}
}

func TestListUserDevices(t *testing.T) {
	repo := &fakeDevices{list: []*device.Device{{ID: "d1"}, {ID: "d2"}}}
	uc := ListUserDevices{Devices: repo}
	got, err := uc.Execute(context.Background(), ListUserDevicesInput{UserID: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(got))
	}
}

type fakeUsers struct {
	getByTelegramErr error
	getByID          *user.User
	created          *user.User
}

func (f *fakeUsers) GetByTelegramID(ctx context.Context, telegramID int64) (*user.User, error) {
	if f.getByTelegramErr != nil {
		return nil, f.getByTelegramErr
	}
	return f.created, nil
}
func (f *fakeUsers) GetByUsername(ctx context.Context, username string) (*user.User, error) {
	if f.getByID != nil {
		return f.getByID, nil
	}
	return nil, user.ErrNotFound
}
func (f *fakeUsers) Create(ctx context.Context, in user.CreateParams) (*user.User, error) {
	return f.created, nil
}
func (f *fakeUsers) GetByID(ctx context.Context, id string) (*user.User, error) {
	if f.getByID != nil {
		return f.getByID, nil
	}
	return nil, user.ErrNotFound
}

type fakeInviteCodes struct {
	code *invitecode.InviteCode
}

func (f *fakeInviteCodes) GetByCodeForUpdate(ctx context.Context, code string) (*invitecode.InviteCode, error) {
	return f.code, nil
}
func (f *fakeInviteCodes) IncrementUsageAndMaybeExhaust(ctx context.Context, inviteCodeID string) (*invitecode.InviteCode, error) {
	f.code.CurrentActivations++
	if f.code.CurrentActivations >= f.code.MaxActivations {
		f.code.Status = invitecode.CodeStatusExhausted
	}
	return f.code, nil
}
func (f *fakeInviteCodes) CreateActivation(ctx context.Context, in invitecode.CreateActivationParams) (*invitecode.Activation, error) {
	return &invitecode.Activation{ID: "act1"}, nil
}
func (f *fakeInviteCodes) HasActivationByUser(ctx context.Context, inviteCodeID, userID string) (bool, error) {
	return false, nil
}
func (f *fakeInviteCodes) Create(ctx context.Context, in invitecode.CreateParams) (*invitecode.InviteCode, error) {
	if f.code == nil {
		f.code = &invitecode.InviteCode{ID: "c-new", Code: in.Code, Status: in.Status, MaxActivations: in.MaxActivations}
	}
	return f.code, nil
}
func (f *fakeInviteCodes) ListRecent(ctx context.Context, limit int) ([]*invitecode.InviteCode, error) {
	if f.code == nil {
		return nil, nil
	}
	return []*invitecode.InviteCode{f.code}, nil
}

type fakeActivateStore struct {
	activeAccessGrant bool
	accessGrants      *fakeAccessGrants
}

type fakeInviteFinance struct {
	amount int64
}

func (f *fakeInviteFinance) ApplyInviteBonus(ctx context.Context, userID string, inviteCodeID string, amountKopecks int64) error {
	f.amount = amountKopecks
	return nil
}

func (f *fakeActivateStore) WithinTx(ctx context.Context, fn func(repos ActivateInviteCodeRepos) error) error {
	accessRepo := f.accessGrants
	if accessRepo == nil {
		accessRepo = &fakeAccessGrants{hasActive: f.activeAccessGrant}
		f.accessGrants = accessRepo
	}

	repos := fakeActivateRepos{
		users:        &fakeUsers{getByID: &user.User{ID: "u1"}},
		codes:        &fakeInviteCodes{code: &invitecode.InviteCode{ID: "c1", Code: "1111", Status: invitecode.CodeStatusActive, MaxActivations: 1}},
		accessGrants: accessRepo,
		audit:        &fakeAudit{},
	}
	return fn(repos)
}

type fakeActivateRepos struct {
	users        UserRepository
	codes        InviteCodeRepository
	accessGrants AccessGrantRepository
	audit        AuditLogRepository
}

func (f fakeActivateRepos) Users() UserRepository               { return f.users }
func (f fakeActivateRepos) InviteCodes() InviteCodeRepository   { return f.codes }
func (f fakeActivateRepos) AccessGrants() AccessGrantRepository { return f.accessGrants }
func (f fakeActivateRepos) AuditLogs() AuditLogRepository       { return f.audit }

type fakeAccessGrants struct {
	hasActive    bool
	activeByUser *accessgrant.AccessGrant
	expiredCount int64
	lastCreate   *accessgrant.CreateParams
}

func (f *fakeAccessGrants) Create(ctx context.Context, in accessgrant.CreateParams) (*accessgrant.AccessGrant, error) {
	f.lastCreate = &in
	if f.activeByUser == nil {
		f.activeByUser = &accessgrant.AccessGrant{
			ID:           "ag-1",
			UserID:       in.UserID,
			InviteCodeID: in.InviteCodeID,
			Status:       in.Status,
			ActivatedAt:  in.ActivatedAt,
			ExpiresAt:    in.ExpiresAt,
			DevicesLimit: in.DevicesLimit,
		}
	}
	return f.activeByUser, nil
}

func (f *fakeAccessGrants) GetActiveByUserID(ctx context.Context, userID string) (*accessgrant.AccessGrant, error) {
	if f.hasActive {
		return &accessgrant.AccessGrant{ID: "ag-existing", UserID: userID, Status: accessgrant.StatusActive}, nil
	}
	if f.activeByUser != nil {
		return f.activeByUser, nil
	}
	return nil, accessgrant.ErrNotFound
}

func (f *fakeAccessGrants) GetLatestByUserID(ctx context.Context, userID string) (*accessgrant.AccessGrant, error) {
	return f.GetActiveByUserID(ctx, userID)
}

func (f *fakeAccessGrants) ExpireActiveBefore(ctx context.Context, now time.Time) (int64, error) {
	return f.expiredCount, nil
}

func (f *fakeAccessGrants) ListActiveUsers(ctx context.Context, limit int) ([]*accessgrant.ActiveUserGrant, error) {
	return nil, nil
}

func (f *fakeAccessGrants) RevokeActiveByUserID(ctx context.Context, userID string) (int64, error) {
	return 1, nil
}

type fakeDevices struct {
	active     *device.Device
	activeErr  error
	created    *device.Device
	list       []*device.Device
	lastCreate *device.CreateParams
}

func (f *fakeDevices) Create(ctx context.Context, in device.CreateParams) (*device.Device, error) {
	f.lastCreate = &in
	return f.created, nil
}
func (f *fakeDevices) ListByUserID(ctx context.Context, userID string) ([]*device.Device, error) {
	return f.list, nil
}
func (f *fakeDevices) GetByID(ctx context.Context, id string) (*device.Device, error) {
	if f.active != nil && f.active.ID == id {
		return f.active, nil
	}
	for _, item := range f.list {
		if item != nil && item.ID == id {
			return item, nil
		}
	}
	return nil, device.ErrNotFound
}
func (f *fakeDevices) GetActiveByUserID(ctx context.Context, userID string) (*device.Device, error) {
	if f.activeErr != nil {
		return nil, f.activeErr
	}
	if f.active != nil {
		return f.active, nil
	}
	return nil, device.ErrNotFound
}
func (f *fakeDevices) Revoke(ctx context.Context, id string) error                       { return nil }
func (f *fakeDevices) AssignNode(ctx context.Context, id string, vpnNodeID string) error { return nil }

type fakeNodes struct{ active []*node.Node }

func (f *fakeNodes) ListActive(ctx context.Context) ([]*node.Node, error)       { return f.active, nil }
func (f *fakeNodes) GetByID(ctx context.Context, id string) (*node.Node, error) { return nil, nil }
func (f *fakeNodes) UpdateHealth(ctx context.Context, id string, status string, lastSeenAt time.Time) error {
	return nil
}
func (f *fakeNodes) UpdateLoad(ctx context.Context, id string, currentLoad int) error { return nil }

type fakeAccess struct {
	created       *access.DeviceAccess
	byID          *access.DeviceAccess
	revoked       bool
	configCleared bool
}

func (f *fakeAccess) Create(ctx context.Context, in access.CreateParams) (*access.DeviceAccess, error) {
	return f.created, nil
}
func (f *fakeAccess) GetByID(ctx context.Context, id string) (*access.DeviceAccess, error) {
	return f.byID, nil
}
func (f *fakeAccess) Activate(ctx context.Context, id string, grantedAt time.Time) error { return nil }
func (f *fakeAccess) Revoke(ctx context.Context, id string, revokedAt time.Time) error {
	f.revoked = true
	return nil
}
func (f *fakeAccess) MarkError(ctx context.Context, id string, failedAt time.Time, message string) error {
	return nil
}
func (f *fakeAccess) SetConfigBlobEncrypted(ctx context.Context, id string, blob []byte) error {
	return nil
}
func (f *fakeAccess) ClearConfigBlobEncrypted(ctx context.Context, id string) error {
	f.configCleared = true
	return nil
}
func (f *fakeAccess) GetActiveByDeviceID(ctx context.Context, deviceID string) ([]*access.DeviceAccess, error) {
	return nil, nil
}
func (f *fakeAccess) GetActiveByNodeAndAssignedIP(ctx context.Context, nodeID string, assignedIP string) (*access.DeviceAccess, error) {
	return nil, access.ErrNotFound
}
func (f *fakeAccess) ListActiveByNodeID(ctx context.Context, nodeID string) ([]*access.DeviceAccess, error) {
	return nil, nil
}

type fakeTokens struct{ revoked bool }

func (f *fakeTokens) Create(ctx context.Context, in token.CreateParams) (*token.ConfigDownloadToken, error) {
	return nil, nil
}
func (f *fakeTokens) GetByTokenHash(ctx context.Context, tokenHash string) (*token.ConfigDownloadToken, error) {
	return nil, token.ErrNotFound
}
func (f *fakeTokens) MarkUsed(ctx context.Context, id string, consumedAt time.Time) error { return nil }
func (f *fakeTokens) RevokeIssuedByAccessID(ctx context.Context, deviceAccessID string, revokedAt time.Time) error {
	f.revoked = true
	return nil
}

type fakeOps struct {
	created    *operation.NodeOperation
	lastCreate *operation.CreateParams
}

func (f *fakeOps) GetByID(ctx context.Context, id string) (*operation.NodeOperation, error) {
	if f.created != nil {
		return f.created, nil
	}
	return &operation.NodeOperation{ID: id, VPNNodeID: "n1"}, nil
}

func (f *fakeOps) Create(ctx context.Context, in operation.CreateParams) (*operation.NodeOperation, error) {
	f.lastCreate = &in
	if f.created != nil {
		return f.created, nil
	}
	return &operation.NodeOperation{ID: "opx"}, nil
}
func (f *fakeOps) MarkRunning(ctx context.Context, id string, startedAt time.Time) error  { return nil }
func (f *fakeOps) MarkSuccess(ctx context.Context, id string, finishedAt time.Time) error { return nil }
func (f *fakeOps) MarkFailed(ctx context.Context, id string, finishedAt time.Time, message string) error {
	return nil
}

type fakeAudit struct{}

func (f *fakeAudit) Create(ctx context.Context, in audit.CreateParams) (*audit.Log, error) {
	return &audit.Log{ID: "log1"}, nil
}

type fakeKeys struct{}

func (fakeKeys) Generate(ctx context.Context) (string, string, error) {
	priv := "FSfGSg9HVUWcRaOzggEUxGafoi8I8JfemfSWLIUhxuI="
	pub, err := wgkeys.DerivePublicKey(priv)
	if err != nil {
		return "", "", err
	}
	return pub, priv, nil
}
func (fakeKeys) GeneratePresharedKey(ctx context.Context) (string, error) { return "psk", nil }

type fakeBrokenKeys struct{}

func (fakeBrokenKeys) Generate(ctx context.Context) (string, string, error) {
	return "KS3O5dK5fty5waMzWBFE92ovd3xpOEOEY6P2j84a+Cg=", "FSfGSg9HVUWcRaOzggEUxGafoi8I8JfemfSWLIUhxuI=", nil
}

type fakeIPAlloc struct{ ip string }

func (f fakeIPAlloc) Allocate(ctx context.Context, nodeID string) (string, error) {
	return f.ip, nil
}
