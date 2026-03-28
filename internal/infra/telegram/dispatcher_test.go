package telegram

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/app"
	"github.com/wwwcont/ryazanvpn/internal/domain/access"
	"github.com/wwwcont/ryazanvpn/internal/domain/accessgrant"
	"github.com/wwwcont/ryazanvpn/internal/domain/device"
	"github.com/wwwcont/ryazanvpn/internal/domain/token"
	"github.com/wwwcont/ryazanvpn/internal/domain/user"
)

func TestStartRegistersAndShowsMenu(t *testing.T) {
	bot := &fakeBot{}
	state := &MemoryStateStore{}
	svc := &TelegramService{
		Bot:        bot,
		States:     state,
		RegisterUC: fakeRegister{u: &user.User{ID: "u1"}},
	}

	svc.HandleUpdate(context.Background(), Update{Message: &Message{Text: "/start", From: User{ID: 101}, Chat: Chat{ID: 101}}})

	if len(bot.messages) == 0 {
		t.Fatal("expected message")
	}
	if bot.messages[0].chatID != 101 {
		t.Fatalf("unexpected chat id: %d", bot.messages[0].chatID)
	}
	if bot.messages[0].markup == nil || len(bot.messages[0].markup.InlineKeyboard) == 0 {
		t.Fatal("expected inline keyboard menu")
	}
}

func TestEnterCodePutsIntoAwaitState(t *testing.T) {
	bot := &fakeBot{}
	state := &MemoryStateStore{}
	svc := &TelegramService{
		Bot:        bot,
		States:     state,
		RegisterUC: fakeRegister{u: &user.User{ID: "u1"}},
	}

	svc.HandleUpdate(context.Background(), Update{CallbackQuery: &CallbackQuery{ID: "1", Data: cbEnterCode, From: User{ID: 201}}})

	st, err := state.Get(context.Background(), 201)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != StateAwaitInvite {
		t.Fatalf("expected %s, got %s", StateAwaitInvite, st.State)
	}
}

func TestInviteCodeActivatesAndIssuesToken(t *testing.T) {
	bot := &fakeBot{}
	state := &MemoryStateStore{}
	accRepo := &fakeAccesses{byID: &access.DeviceAccess{ID: "a1", DeviceID: "d1", ConfigBlobEncrypted: []byte("x")}}
	tokRepo := &fakeTokens{}
	svc := &TelegramService{
		Bot:              bot,
		States:           state,
		RegisterUC:       fakeRegister{u: &user.User{ID: "u1"}},
		ActivateInviteUC: fakeActivate{},
		GetGrantUC: &fakeGetGrant{grant: nil, afterActivate: &accessgrant.AccessGrant{ID: "g1", UserID: "u1", Status: accessgrant.StatusActive,
			ExpiresAt: time.Now().Add(24 * time.Hour)}},
		CreateDeviceUC:  fakeCreateDevice{out: &app.CreateDeviceForUserOutput{Access: &access.DeviceAccess{ID: "a1"}}},
		Devices:         &fakeDevices{activeErr: device.ErrNotFound},
		Accesses:        accRepo,
		Tokens:          tokRepo,
		AccessGrants:    &fakeAccessGrantRepo{latest: &accessgrant.AccessGrant{Status: accessgrant.StatusActive}},
		DownloadBaseURL: "https://vpn.example.com",
		TokenTTL:        10 * time.Minute,
	}

	_ = state.Set(context.Background(), 301, StateAwaitInvite)
	svc.HandleUpdate(context.Background(), Update{Message: &Message{Text: "1234", From: User{ID: 301}, Chat: Chat{ID: 301}}})

	if len(tokRepo.created) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokRepo.created))
	}
	if len(bot.messages) == 0 {
		t.Fatal("expected bot messages")
	}
}

type fakeBot struct{ messages []sentMessage }

type sentMessage struct {
	chatID int64
	text   string
	markup *InlineKeyboardMarkup
}

func (b *fakeBot) SendMessage(_ context.Context, chatID int64, text string, markup *InlineKeyboardMarkup) error {
	b.messages = append(b.messages, sentMessage{chatID: chatID, text: text, markup: markup})
	return nil
}
func (b *fakeBot) AnswerCallbackQuery(_ context.Context, callbackID string, text string) error {
	return nil
}

type fakeRegister struct{ u *user.User }

func (f fakeRegister) Execute(ctx context.Context, in app.RegisterTelegramUserInput) (*user.User, error) {
	return f.u, nil
}

type fakeActivate struct{}

func (fakeActivate) Execute(ctx context.Context, in app.ActivateInviteCodeInput) error { return nil }

type fakeGetGrant struct {
	grant         *accessgrant.AccessGrant
	afterActivate *accessgrant.AccessGrant
	calls         int
}

func (f *fakeGetGrant) Execute(ctx context.Context, in app.GetActiveAccessGrantByUserInput) (*accessgrant.AccessGrant, error) {
	f.calls++
	if f.calls > 1 && f.afterActivate != nil {
		return f.afterActivate, nil
	}
	if f.grant == nil {
		return nil, accessgrant.ErrNotFound
	}
	return f.grant, nil
}

type fakeAccessGrantRepo struct{ latest *accessgrant.AccessGrant }

func (f *fakeAccessGrantRepo) Create(ctx context.Context, in accessgrant.CreateParams) (*accessgrant.AccessGrant, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeAccessGrantRepo) GetActiveByUserID(ctx context.Context, userID string) (*accessgrant.AccessGrant, error) {
	if f.latest == nil {
		return nil, accessgrant.ErrNotFound
	}
	return f.latest, nil
}
func (f *fakeAccessGrantRepo) GetLatestByUserID(ctx context.Context, userID string) (*accessgrant.AccessGrant, error) {
	if f.latest == nil {
		return nil, accessgrant.ErrNotFound
	}
	return f.latest, nil
}
func (f *fakeAccessGrantRepo) ExpireActiveBefore(ctx context.Context, now time.Time) (int64, error) {
	return 0, nil
}
func (f *fakeAccessGrantRepo) ListActiveUsers(ctx context.Context, limit int) ([]*accessgrant.ActiveUserGrant, error) {
	return nil, nil
}
func (f *fakeAccessGrantRepo) RevokeActiveByUserID(ctx context.Context, userID string) (int64, error) {
	return 1, nil
}

type fakeCreateDevice struct {
	out *app.CreateDeviceForUserOutput
}

func (f fakeCreateDevice) Execute(ctx context.Context, in app.CreateDeviceForUserInput) (*app.CreateDeviceForUserOutput, error) {
	return f.out, nil
}

type fakeRevoke struct{}

func (fakeRevoke) Execute(ctx context.Context, in app.RevokeDeviceAccessInput) error { return nil }

type fakeDevices struct {
	active    *device.Device
	activeErr error
}

func (f *fakeDevices) Create(ctx context.Context, in device.CreateParams) (*device.Device, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeDevices) ListByUserID(ctx context.Context, userID string) ([]*device.Device, error) {
	return []*device.Device{}, nil
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
func (f *fakeDevices) Revoke(ctx context.Context, id string) error { return nil }
func (f *fakeDevices) AssignNode(ctx context.Context, id string, vpnNodeID string) error {
	return nil
}

type fakeAccesses struct {
	byID *access.DeviceAccess
}

func (f *fakeAccesses) Create(ctx context.Context, in access.CreateParams) (*access.DeviceAccess, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeAccesses) GetByID(ctx context.Context, id string) (*access.DeviceAccess, error) {
	return f.byID, nil
}
func (f *fakeAccesses) Activate(ctx context.Context, id string, grantedAt time.Time) error {
	return nil
}
func (f *fakeAccesses) Revoke(ctx context.Context, id string, revokedAt time.Time) error { return nil }
func (f *fakeAccesses) MarkError(ctx context.Context, id string, failedAt time.Time, message string) error {
	return nil
}
func (f *fakeAccesses) SetConfigBlobEncrypted(ctx context.Context, id string, blob []byte) error {
	return nil
}
func (f *fakeAccesses) GetActiveByDeviceID(ctx context.Context, deviceID string) ([]*access.DeviceAccess, error) {
	return []*access.DeviceAccess{}, nil
}

type fakeTokens struct{ created []token.CreateParams }

func (f *fakeTokens) Create(ctx context.Context, in token.CreateParams) (*token.ConfigDownloadToken, error) {
	f.created = append(f.created, in)
	return &token.ConfigDownloadToken{ID: "t1"}, nil
}
func (f *fakeTokens) GetByTokenHash(ctx context.Context, tokenHash string) (*token.ConfigDownloadToken, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeTokens) MarkUsed(ctx context.Context, id string, consumedAt time.Time) error { return nil }
