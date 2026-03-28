package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/domain/access"
	"github.com/wwwcont/ryazanvpn/internal/domain/audit"
	"github.com/wwwcont/ryazanvpn/internal/domain/device"
	"github.com/wwwcont/ryazanvpn/internal/domain/invitecode"
	"github.com/wwwcont/ryazanvpn/internal/domain/node"
	"github.com/wwwcont/ryazanvpn/internal/domain/operation"
	"github.com/wwwcont/ryazanvpn/internal/domain/user"
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
	uc := ActivateInviteCode{Store: fakeActivateStore{}}
	if err := uc.Execute(context.Background(), ActivateInviteCodeInput{UserID: "u1", Code: "12a4"}); !errors.Is(err, ErrInvalidInviteCodeFormat) {
		t.Fatalf("expected format error, got %v", err)
	}
}

func TestActivateInviteCode_Success(t *testing.T) {
	uc := ActivateInviteCode{Store: fakeActivateStore{}, Now: func() time.Time { return time.Unix(100, 0) }}
	if err := uc.Execute(context.Background(), ActivateInviteCodeInput{UserID: "u1", Code: "1111"}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestCreateDeviceForUser_AssignsMinLoadNode(t *testing.T) {
	devRepo := &fakeDevices{activeErr: device.ErrNotFound, created: &device.Device{ID: "d1", UserID: "u1"}}
	nodes := &fakeNodes{active: []*node.Node{{ID: "n1", CurrentLoad: 5}, {ID: "n2", CurrentLoad: 1}}}
	accessRepo := &fakeAccess{created: &access.DeviceAccess{ID: "a1", DeviceID: "d1", VPNNodeID: "n2"}}
	opRepo := &fakeOps{}
	auditRepo := &fakeAudit{}

	uc := CreateDeviceForUser{
		Devices:      devRepo,
		Nodes:        nodes,
		Accesses:     accessRepo,
		Operations:   opRepo,
		AuditLogs:    auditRepo,
		KeyGenerator: fakeKeys{},
		IPAllocator:  fakeIPAlloc{ip: "10.10.0.2"},
		NodeAssigner: MinLoadNodeAssigner{},
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

func TestRevokeDeviceAccess_Confirmed(t *testing.T) {
	accessRepo := &fakeAccess{byID: &access.DeviceAccess{ID: "a1", DeviceID: "d1", VPNNodeID: "n1"}}
	opRepo := &fakeOps{created: &operation.NodeOperation{ID: "op1"}}
	uc := RevokeDeviceAccess{Accesses: accessRepo, Operations: opRepo, AuditLogs: &fakeAudit{}}

	if err := uc.Execute(context.Background(), RevokeDeviceAccessInput{AccessID: "a1"}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if accessRepo.revoked {
		t.Fatal("did not expect direct revoke without executor")
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

type fakeActivateStore struct{}

func (f fakeActivateStore) WithinTx(ctx context.Context, fn func(repos ActivateInviteCodeRepos) error) error {
	repos := fakeActivateRepos{
		users: &fakeUsers{getByID: &user.User{ID: "u1"}},
		codes: &fakeInviteCodes{code: &invitecode.InviteCode{ID: "c1", Code: "1111", Status: invitecode.CodeStatusActive, MaxActivations: 1}},
		audit: &fakeAudit{},
	}
	return fn(repos)
}

type fakeActivateRepos struct {
	users UserRepository
	codes InviteCodeRepository
	audit AuditLogRepository
}

func (f fakeActivateRepos) Users() UserRepository             { return f.users }
func (f fakeActivateRepos) InviteCodes() InviteCodeRepository { return f.codes }
func (f fakeActivateRepos) AuditLogs() AuditLogRepository     { return f.audit }

type fakeDevices struct {
	active    *device.Device
	activeErr error
	created   *device.Device
	list      []*device.Device
}

func (f *fakeDevices) Create(ctx context.Context, in device.CreateParams) (*device.Device, error) {
	return f.created, nil
}
func (f *fakeDevices) ListByUserID(ctx context.Context, userID string) ([]*device.Device, error) {
	return f.list, nil
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
	created *access.DeviceAccess
	byID    *access.DeviceAccess
	revoked bool
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
func (f *fakeAccess) GetActiveByDeviceID(ctx context.Context, deviceID string) ([]*access.DeviceAccess, error) {
	return nil, nil
}

type fakeOps struct{ created *operation.NodeOperation }

func (f *fakeOps) GetByID(ctx context.Context, id string) (*operation.NodeOperation, error) {
	if f.created != nil {
		return f.created, nil
	}
	return &operation.NodeOperation{ID: id, VPNNodeID: "n1"}, nil
}

func (f *fakeOps) Create(ctx context.Context, in operation.CreateParams) (*operation.NodeOperation, error) {
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
	return "pub", "priv", nil
}

type fakeIPAlloc struct{ ip string }

func (f fakeIPAlloc) Allocate(ctx context.Context, nodeID string) (string, error) {
	return f.ip, nil
}
