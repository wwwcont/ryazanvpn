package telegram

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/app"
	"github.com/wwwcont/ryazanvpn/internal/domain/access"
	"github.com/wwwcont/ryazanvpn/internal/domain/accessgrant"
	"github.com/wwwcont/ryazanvpn/internal/domain/device"
	"github.com/wwwcont/ryazanvpn/internal/domain/node"
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
	if st.State != StateAwaitingInviteCode {
		t.Fatalf("expected %s, got %s", StateAwaitingInviteCode, st.State)
	}
}

func TestInviteCodeActivatesAndSendsDocument(t *testing.T) {
	bot := &fakeBot{}
	state := &MemoryStateStore{}
	accRepo := &fakeAccesses{byID: &access.DeviceAccess{ID: "a1", DeviceID: "d1", Protocol: "wireguard", ConfigBlobEncrypted: []byte("[Interface]\nPrivateKey = FSfGSg9HVUWcRaOzggEUxGafoi8I8JfemfSWLIUhxuI=\n[Peer]\nPublicKey = freshKey=\nPresharedKey = psk\n"), VPNNodeID: "n1"}}
	svc := &TelegramService{
		Bot:              bot,
		States:           state,
		RegisterUC:       fakeRegister{u: &user.User{ID: "u1"}},
		ActivateInviteUC: fakeActivate{},
		GetGrantUC: &fakeGetGrant{grant: &accessgrant.AccessGrant{ID: "g1", UserID: "u1", Status: accessgrant.StatusActive,
			ExpiresAt: time.Now().Add(24 * time.Hour)}},
		CreateDeviceUC:  fakeCreateDevice{out: &app.CreateDeviceForUserOutput{Device: &device.Device{ID: "d1", PublicKey: "jVcMIlprLo8VEAAXIBMDf08IxK0oRWLSArQryOk0DDE="}, Access: &access.DeviceAccess{ID: "a1"}}},
		Devices:         &fakeDevices{active: &device.Device{ID: "d1", PublicKey: "jVcMIlprLo8VEAAXIBMDf08IxK0oRWLSArQryOk0DDE="}},
		Accesses:        accRepo,
		Nodes:           &fakeNodes{byID: map[string]*node.Node{"n1": {ID: "n1", ServerPublicKey: "freshKey="}}},
		AccessGrants:    &fakeAccessGrantRepo{latest: nil},
		DownloadBaseURL: "https://vpn.example.com",
		TokenTTL:        10 * time.Minute,
		ConfigEncryptor: fakeEncryptor{},
	}

	_ = state.Set(context.Background(), 301, StateAwaitingInviteCode)
	svc.HandleUpdate(context.Background(), Update{Message: &Message{Text: "1234", From: User{ID: 301}, Chat: Chat{ID: 301}}})

	if len(bot.messages) == 0 {
		t.Fatal("expected bot messages")
	}
	last := bot.messages[len(bot.messages)-1]
	if !strings.Contains(last.text, "Готово") {
		t.Fatalf("expected document caption, got %q", last.text)
	}
}

func TestSendHealthLink_UsesXrayExporter(t *testing.T) {
	bot := &fakeBot{}
	xexp := &fakeXrayExporter{link: "vless://u@h:1?security=reality#x"}
	svc := &TelegramService{
		Bot:             bot,
		Devices:         &fakeDevices{active: &device.Device{ID: "d1"}},
		Accesses:        &fakeAccesses{byIDMap: map[string]*access.DeviceAccess{"ax": {ID: "ax", DeviceID: "d1", Protocol: "xray", ConfigBlobEncrypted: []byte(`{"id":"11111111-1111-1111-1111-111111111111"}`)}}},
		ConfigEncryptor: fakeEncryptor{},
		XrayExporter:    xexp,
		XrayPublicHost:  "x.example.com",
		XrayRealityPort: 8443,
		XrayServerName:  "www.cloudflare.com",
		XrayShortID:     "0123456789abcdef",
		XrayPublicKey:   "pub",
	}
	svc.sendHealthLink(context.Background(), 1, "u1")
	if xexp.calls != 1 {
		t.Fatalf("xray exporter should be called once, got %d", xexp.calls)
	}
	if len(bot.messages) == 0 || !strings.Contains(bot.messages[len(bot.messages)-1].text, "vless://") {
		t.Fatalf("expected vless link message, got %+v", bot.messages)
	}
}

func TestSendHealthLink_WhenXrayConfigEmpty_ShowsConfigNotReady(t *testing.T) {
	bot := &fakeBot{}
	svc := &TelegramService{
		Bot:             bot,
		Devices:         &fakeDevices{active: &device.Device{ID: "d1"}},
		Accesses:        &fakeAccesses{byIDMap: map[string]*access.DeviceAccess{"ax": {ID: "ax", DeviceID: "d1", Protocol: "xray"}}},
		ConfigEncryptor: fakeEncryptor{},
		XrayExporter:    &fakeXrayExporter{link: "vless://u@h:1?security=reality#x"},
	}

	svc.sendHealthLink(context.Background(), 1, "u1")

	if len(bot.messages) == 0 {
		t.Fatal("expected bot message")
	}
	if !strings.Contains(bot.messages[len(bot.messages)-1].text, "Health-конфиг ещё не готов") {
		t.Fatalf("expected config-not-ready message, got %+v", bot.messages[len(bot.messages)-1])
	}
}

func TestHandleGetConfig_DefaultProtocolXray(t *testing.T) {
	bot := &fakeBot{}
	svc := &TelegramService{
		Bot: bot,
		GetGrantUC: &fakeGetGrant{grant: &accessgrant.AccessGrant{
			ID: "g1", UserID: "u1", Status: accessgrant.StatusActive, ExpiresAt: time.Now().Add(24 * time.Hour),
		}},
		Devices: &fakeDevices{active: &device.Device{ID: "d1"}},
		Accesses: &fakeAccesses{byIDMap: map[string]*access.DeviceAccess{
			"ax": {ID: "ax", DeviceID: "d1", Protocol: "xray", ConfigBlobEncrypted: []byte(`{"id":"11111111-1111-1111-1111-111111111111"}`)},
			"aw": {ID: "aw", DeviceID: "d1", Protocol: "wireguard", VPNNodeID: "n1", ConfigBlobEncrypted: []byte("[Interface]\nPrivateKey = FSfGSg9HVUWcRaOzggEUxGafoi8I8JfemfSWLIUhxuI=\n[Peer]\nPublicKey = freshKey=\nPresharedKey = psk\n")},
		}},
		Nodes:           &fakeNodes{byID: map[string]*node.Node{"n1": {ID: "n1", ServerPublicKey: "freshKey="}}},
		ConfigEncryptor: fakeEncryptor{},
		XrayExporter:    &fakeXrayExporter{link: "vless://u@h:1?security=reality#x"},
		XrayPublicHost:  "x.example.com",
		XrayRealityPort: 8443,
		XrayServerName:  "www.cloudflare.com",
		XrayShortID:     "0123456789abcdef",
		XrayPublicKey:   "pub",
	}

	svc.handleGetConfig(context.Background(), 100, "u1")

	if len(bot.messages) < 2 {
		t.Fatalf("expected xray + speed prompt messages, got %+v", bot.messages)
	}
	if !strings.Contains(bot.messages[0].text, "vless://") {
		t.Fatalf("expected first message to be xray link, got %q", bot.messages[0].text)
	}
	if !strings.Contains(bot.messages[1].text, "Speed") {
		t.Fatalf("expected second message to offer speed config, got %q", bot.messages[1].text)
	}
}

func TestHandleGetConfig_FallbacksToWireguardWhenXrayUnavailable(t *testing.T) {
	bot := &fakeBot{}
	svc := &TelegramService{
		Bot: bot,
		GetGrantUC: &fakeGetGrant{grant: &accessgrant.AccessGrant{
			ID: "g1", UserID: "u1", Status: accessgrant.StatusActive, ExpiresAt: time.Now().Add(24 * time.Hour),
		}},
		Devices: &fakeDevices{active: &device.Device{
			ID: "d1", PublicKey: "jVcMIlprLo8VEAAXIBMDf08IxK0oRWLSArQryOk0DDE=",
		}},
		Accesses: &fakeAccesses{byIDMap: map[string]*access.DeviceAccess{
			"ax": {ID: "ax", DeviceID: "d1", Protocol: "xray"},
			"aw": {ID: "aw", DeviceID: "d1", Protocol: "wireguard", VPNNodeID: "n1", ConfigBlobEncrypted: []byte("[Interface]\nPrivateKey = FSfGSg9HVUWcRaOzggEUxGafoi8I8JfemfSWLIUhxuI=\n[Peer]\nPublicKey = freshKey=\nPresharedKey = psk\n")},
		}},
		ConfigEncryptor: fakeEncryptor{},
		XrayExporter:    &fakeXrayExporter{link: "vless://u@h:1?security=reality#x"},
		Nodes:           &fakeNodes{byID: map[string]*node.Node{"n1": {ID: "n1", ServerPublicKey: "freshKey="}}},
	}

	svc.handleGetConfig(context.Background(), 100, "u1")

	foundDoc := false
	for _, m := range bot.messages {
		if strings.Contains(m.text, "AmneziaWG/WireGuard") {
			foundDoc = true
			break
		}
	}
	if !foundDoc {
		t.Fatalf("expected wireguard document fallback, got %+v", bot.messages)
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
func (b *fakeBot) SendDocument(_ context.Context, chatID int64, filename string, content []byte, caption string, markup *InlineKeyboardMarkup) error {
	b.messages = append(b.messages, sentMessage{chatID: chatID, text: caption, markup: markup})
	return nil
}
func (b *fakeBot) SendPhoto(_ context.Context, chatID int64, filename string, content []byte, caption string, markup *InlineKeyboardMarkup) error {
	b.messages = append(b.messages, sentMessage{chatID: chatID, text: caption, markup: markup})
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
func (f *fakeDevices) GetByID(ctx context.Context, id string) (*device.Device, error) {
	if f.active != nil && f.active.ID == id {
		return f.active, nil
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
func (f *fakeDevices) Revoke(ctx context.Context, id string) error { return nil }
func (f *fakeDevices) AssignNode(ctx context.Context, id string, vpnNodeID string) error {
	return nil
}

type fakeAccesses struct {
	byID        *access.DeviceAccess
	byIDMap     map[string]*access.DeviceAccess
	actives     []*access.DeviceAccess
	clearedIDs  []string
	setBlobByID map[string][]byte
}

func (f *fakeAccesses) Create(ctx context.Context, in access.CreateParams) (*access.DeviceAccess, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeAccesses) GetByID(ctx context.Context, id string) (*access.DeviceAccess, error) {
	if f.byIDMap != nil {
		if v, ok := f.byIDMap[id]; ok {
			return v, nil
		}
	}
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
	if f.setBlobByID == nil {
		f.setBlobByID = map[string][]byte{}
	}
	f.setBlobByID[id] = append([]byte(nil), blob...)
	return nil
}
func (f *fakeAccesses) ClearConfigBlobEncrypted(ctx context.Context, id string) error {
	f.clearedIDs = append(f.clearedIDs, id)
	if f.byID != nil && f.byID.ID == id {
		f.byID.ConfigBlobEncrypted = nil
	}
	if f.byIDMap != nil {
		if item, ok := f.byIDMap[id]; ok {
			item.ConfigBlobEncrypted = nil
		}
	}
	return nil
}
func (f *fakeAccesses) GetActiveByDeviceID(ctx context.Context, deviceID string) ([]*access.DeviceAccess, error) {
	if f.actives != nil {
		return f.actives, nil
	}
	if f.byID != nil {
		return []*access.DeviceAccess{f.byID}, nil
	}
	if f.byIDMap != nil {
		out := make([]*access.DeviceAccess, 0, len(f.byIDMap))
		for _, item := range f.byIDMap {
			out = append(out, item)
		}
		return out, nil
	}
	return []*access.DeviceAccess{}, nil
}
func (f *fakeAccesses) GetActiveByNodeAndAssignedIP(ctx context.Context, nodeID string, assignedIP string) (*access.DeviceAccess, error) {
	return nil, access.ErrNotFound
}
func (f *fakeAccesses) ListActiveByNodeID(ctx context.Context, nodeID string) ([]*access.DeviceAccess, error) {
	return nil, nil
}

type fakeEncryptor struct{}

func (fakeEncryptor) Encrypt(v []byte) ([]byte, error) { return v, nil }
func (fakeEncryptor) Decrypt(v []byte) ([]byte, error) { return v, nil }

type fakeXrayExporter struct {
	link  string
	calls int
}

func (f *fakeXrayExporter) ExportVLESSReality(ctx context.Context, in app.ExportXrayRealityInput) (string, error) {
	f.calls++
	if f.link != "" {
		return f.link, nil
	}
	return "vless://x", nil
}

type fakeNodes struct {
	byID map[string]*node.Node
}

func (f *fakeNodes) ListActive(ctx context.Context) ([]*node.Node, error) {
	out := make([]*node.Node, 0, len(f.byID))
	for _, n := range f.byID {
		out = append(out, n)
	}
	return out, nil
}
func (f *fakeNodes) GetByID(ctx context.Context, id string) (*node.Node, error) {
	if f.byID == nil {
		return nil, node.ErrNotFound
	}
	if n, ok := f.byID[id]; ok {
		return n, nil
	}
	return nil, node.ErrNotFound
}
func (f *fakeNodes) UpdateHealth(ctx context.Context, id string, status string, lastSeenAt time.Time) error {
	return nil
}
func (f *fakeNodes) UpdateLoad(ctx context.Context, id string, currentLoad int) error { return nil }

func TestRandIntNRange(t *testing.T) {
	for i := 0; i < 1000; i++ {
		v := randIntN(10000)
		if v < 0 || v >= 10000 {
			t.Fatalf("randIntN out of range: %d", v)
		}
	}
}

func TestLoadConfigPlaintext_RejectsStaleWireguardServerKey(t *testing.T) {
	accRepo := &fakeAccesses{
		byID: &access.DeviceAccess{
			ID:                  "a1",
			DeviceID:            "d1",
			Protocol:            "wireguard",
			VPNNodeID:           "n1",
			ConfigBlobEncrypted: []byte("[Interface]\nPrivateKey = FSfGSg9HVUWcRaOzggEUxGafoi8I8JfemfSWLIUhxuI=\n[Peer]\nPublicKey = staleKey=\nPresharedKey = psk\n"),
		},
	}
	svc := &TelegramService{
		Accesses:        accRepo,
		Devices:         &fakeDevices{active: &device.Device{ID: "d1", PublicKey: "jVcMIlprLo8VEAAXIBMDf08IxK0oRWLSArQryOk0DDE="}},
		Nodes:           &fakeNodes{byID: map[string]*node.Node{"n1": {ID: "n1", ServerPublicKey: "freshKey="}}},
		ConfigEncryptor: fakeEncryptor{},
	}

	_, err := svc.loadConfigPlaintext(context.Background(), "a1")
	if err == nil {
		t.Fatal("expected stale config error")
	}
	if len(accRepo.clearedIDs) != 1 || accRepo.clearedIDs[0] != "a1" {
		t.Fatalf("expected stale blob to be cleared, got %+v", accRepo.clearedIDs)
	}
}

func TestLoadConfigPlaintext_RejectsStaleWireguardPeerKey(t *testing.T) {
	accRepo := &fakeAccesses{
		byID: &access.DeviceAccess{
			ID:                  "a2",
			DeviceID:            "d2",
			Protocol:            "wireguard",
			VPNNodeID:           "n1",
			ConfigBlobEncrypted: []byte("[Interface]\nPrivateKey = FSfGSg9HVUWcRaOzggEUxGafoi8I8JfemfSWLIUhxuI=\n[Peer]\nPublicKey = freshKey=\nPresharedKey = psk\n"),
		},
	}
	svc := &TelegramService{
		Accesses:        accRepo,
		Devices:         &fakeDevices{active: &device.Device{ID: "d2", PublicKey: "KS3O5dK5fty5waMzWBFE92ovd3xpOEOEY6P2j84a+Cg="}},
		Nodes:           &fakeNodes{byID: map[string]*node.Node{"n1": {ID: "n1", ServerPublicKey: "freshKey="}}},
		ConfigEncryptor: fakeEncryptor{},
	}

	_, err := svc.loadConfigPlaintext(context.Background(), "a2")
	if err == nil {
		t.Fatal("expected stale config error")
	}
	if len(accRepo.clearedIDs) != 1 || accRepo.clearedIDs[0] != "a2" {
		t.Fatalf("expected stale blob to be cleared, got %+v", accRepo.clearedIDs)
	}
}
