package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/domain/access"
	"github.com/wwwcont/ryazanvpn/internal/domain/accessgrant"
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
func (f *fakeAccess) GetActiveByNodeAndAssignedIP(ctx context.Context, nodeID string, assignedIP string) (*access.DeviceAccess, error) {
	return nil, access.ErrNotFound
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
func (fakeKeys) GeneratePresharedKey(ctx context.Context) (string, error) { return "psk", nil }

type fakeIPAlloc struct{ ip string }

func (f fakeIPAlloc) Allocate(ctx context.Context, nodeID string) (string, error) {
	return f.ip, nil
}
