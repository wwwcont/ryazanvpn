package telegram

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wwwcont/ryazanvpn/internal/app"
	"github.com/wwwcont/ryazanvpn/internal/domain/access"
	"github.com/wwwcont/ryazanvpn/internal/domain/accessgrant"
	"github.com/wwwcont/ryazanvpn/internal/domain/audit"
	"github.com/wwwcont/ryazanvpn/internal/domain/device"
	"github.com/wwwcont/ryazanvpn/internal/domain/invitecode"
	"github.com/wwwcont/ryazanvpn/internal/domain/node"
	"github.com/wwwcont/ryazanvpn/internal/domain/token"
	"github.com/wwwcont/ryazanvpn/internal/domain/user"
)

var inviteCodePattern = regexp.MustCompile(`^\d{4}$`)

const (
	cbEnterCode = "c:code"
	cbMyAccess  = "c:acc"
	cbConfig    = "c:cfg"
	cbDelete    = "c:del"
	cbHelp      = "c:hlp"
	cbDeleteYes = "c:d:y"
	cbDeleteNo  = "c:d:n"
	cbCfgFile   = "c:f"
	cbCfgQR     = "c:q"
	cbCfgText   = "c:t"
	cbCfgDefVPN = "c:dv"

	cbAdminMenu   = "a:menu"
	cbAdminSingle = "a:c1"
	cbAdminBatch  = "a:cb"
	cbAdminLast   = "a:last"
	cbAdminUsers  = "a:usr"
	cbAdminStat   = "a:st"
	cbAdminRev    = "a:rv"
	cbAdminNode   = "a:nd"
)

type registerUserUseCase interface {
	Execute(ctx context.Context, in app.RegisterTelegramUserInput) (*user.User, error)
}

type activateInviteCodeUseCase interface {
	Execute(ctx context.Context, in app.ActivateInviteCodeInput) error
}

type getActiveGrantUseCase interface {
	Execute(ctx context.Context, in app.GetActiveAccessGrantByUserInput) (*accessgrant.AccessGrant, error)
}

type createDeviceUseCase interface {
	Execute(ctx context.Context, in app.CreateDeviceForUserInput) (*app.CreateDeviceForUserOutput, error)
}

type revokeAccessUseCase interface {
	Execute(ctx context.Context, in app.RevokeDeviceAccessInput) error
}

type TelegramService struct {
	Logger *slog.Logger
	Bot    BotClient
	States StateStore

	RegisterUC       registerUserUseCase
	ActivateInviteUC activateInviteCodeUseCase
	GetGrantUC       getActiveGrantUseCase
	CreateDeviceUC   createDeviceUseCase
	RevokeAccessUC   revokeAccessUseCase

	Users        app.UserRepository
	Devices      app.DeviceRepository
	Accesses     app.DeviceAccessRepository
	Tokens       app.ConfigDownloadTokenRepository
	AccessGrants app.AccessGrantRepository
	InviteCodes  app.InviteCodeRepository
	AuditLogs    app.AuditLogRepository
	Nodes        app.NodeRepository
	Traffic      app.TrafficRepository

	DownloadBaseURL string
	TokenTTL        time.Duration
	AdminIDs        map[int64]struct{}
	NodeCapacity    int
	ConfigEncryptor app.EncryptionService
	VPNExporter     app.VPNKeyExporter
	DefaultVPNMTU   int
	DefaultVPNAWG   app.DefaultVPNAWGFields
}

func (s *TelegramService) HandleUpdate(ctx context.Context, upd Update) {
	if upd.CallbackQuery != nil {
		s.handleCallback(ctx, upd.CallbackQuery)
		return
	}
	if upd.Message != nil {
		s.handleMessage(ctx, upd.Message)
	}
}

func (s *TelegramService) handleMessage(ctx context.Context, msg *Message) {
	if msg == nil {
		return
	}
	u, ok := s.ensureUser(ctx, msg.From, msg.Chat.ID)
	if !ok {
		return
	}

	if s.isAdmin(msg.From.ID) {
		if s.handleAdminText(ctx, msg, u) {
			return
		}
	}

	text := strings.TrimSpace(msg.Text)
	switch text {
	case "/start":
		_ = s.States.Set(ctx, msg.From.ID, StateIdle)
		s.sendMainMenu(ctx, msg.Chat.ID, s.isAdmin(msg.From.ID))
		return
	case "Ввести код":
		s.promptInviteCode(ctx, msg.Chat.ID, msg.From.ID)
		return
	case "Мой доступ":
		s.sendAccessStatus(ctx, msg.Chat.ID, u.ID)
		return
	case "Получить конфиг":
		s.handleGetConfig(ctx, msg.Chat.ID, u.ID)
		return
	case "Удалить устройство":
		s.askDeleteConfirm(ctx, msg.Chat.ID, msg.From.ID)
		return
	case "Помощь":
		s.sendHelp(ctx, msg.Chat.ID)
		return
	}

	state, err := s.States.Get(ctx, msg.From.ID)
	if err != nil {
		s.logErr("get state", err)
	}
	if state.State == StateAwaitInvite && inviteCodePattern.MatchString(text) {
		s.handleInviteCode(ctx, msg.Chat.ID, msg.From.ID, u.ID, text)
		return
	}

	if inviteCodePattern.MatchString(text) {
		s.handleInviteCode(ctx, msg.Chat.ID, msg.From.ID, u.ID, text)
		return
	}

	_ = s.Bot.SendMessage(ctx, msg.Chat.ID, "Не понял сообщение. Используйте кнопки ниже 👇", mainMenu(s.isAdmin(msg.From.ID)))
}

func (s *TelegramService) handleAdminText(ctx context.Context, msg *Message, currentUser *user.User) bool {
	state, _ := s.States.Get(ctx, msg.From.ID)
	text := strings.TrimSpace(msg.Text)

	switch state.State {
	case StateAwaitBatchCnt:
		n, err := strconv.Atoi(text)
		if err != nil || n <= 0 || n > 1000 {
			_ = s.Bot.SendMessage(ctx, msg.Chat.ID, "Введите число от 1 до 1000.", nil)
			return true
		}
		s.createBatchCodes(ctx, msg.Chat.ID, currentUser.ID, n)
		_ = s.States.Set(ctx, msg.From.ID, StateIdle)
		return true
	case StateAwaitUserStat:
		s.adminUserStats(ctx, msg.Chat.ID, currentUser.ID, text)
		_ = s.States.Set(ctx, msg.From.ID, StateIdle)
		return true
	case StateAwaitRevokeID:
		s.adminRevokeUser(ctx, msg.Chat.ID, currentUser.ID, text)
		_ = s.States.Set(ctx, msg.From.ID, StateIdle)
		return true
	}
	return false
}

func (s *TelegramService) handleCallback(ctx context.Context, cb *CallbackQuery) {
	if cb == nil {
		return
	}
	chatID := cb.From.ID
	if cb.Message != nil {
		chatID = cb.Message.Chat.ID
	}
	u, ok := s.ensureUser(ctx, cb.From, chatID)
	if !ok {
		return
	}

	if s.isAdmin(cb.From.ID) {
		if s.handleAdminCallback(ctx, cb, chatID, u) {
			return
		}
	}

	switch cb.Data {
	case cbEnterCode:
		s.promptInviteCode(ctx, chatID, cb.From.ID)
		_ = s.Bot.AnswerCallbackQuery(ctx, cb.ID, "Введите 4-значный код")
	case cbMyAccess:
		s.sendAccessStatus(ctx, chatID, u.ID)
		_ = s.Bot.AnswerCallbackQuery(ctx, cb.ID, "Показываю доступ")
	case cbConfig:
		s.handleGetConfig(ctx, chatID, u.ID)
		_ = s.Bot.AnswerCallbackQuery(ctx, cb.ID, "Готовлю конфиг")
	case cbDelete:
		s.askDeleteConfirm(ctx, chatID, cb.From.ID)
		_ = s.Bot.AnswerCallbackQuery(ctx, cb.ID, "Нужно подтверждение")
	case cbDeleteYes:
		s.handleDeleteDevice(ctx, chatID, cb.From.ID, u.ID)
		_ = s.Bot.AnswerCallbackQuery(ctx, cb.ID, "Удаляю устройство")
	case cbDeleteNo:
		_ = s.States.Set(ctx, cb.From.ID, StateIdle)
		_ = s.Bot.SendMessage(ctx, chatID, "Окей, ничего не меняю.", mainMenu(s.isAdmin(cb.From.ID)))
		_ = s.Bot.AnswerCallbackQuery(ctx, cb.ID, "Отменено")
	case cbHelp:
		s.sendHelp(ctx, chatID)
		_ = s.Bot.AnswerCallbackQuery(ctx, cb.ID, "Помощь")
	case cbCfgFile:
		s.sendConfigDocument(ctx, chatID, u.ID)
		_ = s.Bot.AnswerCallbackQuery(ctx, cb.ID, "Отправляю .conf")
	case cbCfgQR:
		s.sendConfigQR(ctx, chatID, u.ID)
		_ = s.Bot.AnswerCallbackQuery(ctx, cb.ID, "Готовлю QR")
	case cbCfgText:
		s.sendConfigText(ctx, chatID, u.ID)
		_ = s.Bot.AnswerCallbackQuery(ctx, cb.ID, "Показываю текст")
	case cbCfgDefVPN:
		s.sendDefaultVPNKey(ctx, chatID, u.ID)
		_ = s.Bot.AnswerCallbackQuery(ctx, cb.ID, "Готовлю ключ")
	default:
		_ = s.Bot.AnswerCallbackQuery(ctx, cb.ID, "Неизвестное действие")
	}
}

func (s *TelegramService) handleAdminCallback(ctx context.Context, cb *CallbackQuery, chatID int64, u *user.User) bool {
	switch cb.Data {
	case cbAdminMenu:
		_ = s.Bot.SendMessage(ctx, chatID, "Админка:", adminMenu())
		_ = s.Bot.AnswerCallbackQuery(ctx, cb.ID, "Открываю админку")
		_ = s.logAudit(ctx, u.ID, "telegram.admin.menu.open", map[string]any{"chat_id": chatID})
		return true
	case cbAdminSingle:
		s.createSingleCode(ctx, chatID, u.ID)
		_ = s.Bot.AnswerCallbackQuery(ctx, cb.ID, "Код создан")
		return true
	case cbAdminBatch:
		_ = s.States.Set(ctx, cb.From.ID, StateAwaitBatchCnt)
		_ = s.Bot.SendMessage(ctx, chatID, "Введите количество кодов (1..1000):", nil)
		_ = s.Bot.AnswerCallbackQuery(ctx, cb.ID, "Жду количество")
		return true
	case cbAdminLast:
		s.adminLastCodes(ctx, chatID)
		_ = s.logAudit(ctx, u.ID, "telegram.admin.codes.list_recent", nil)
		_ = s.Bot.AnswerCallbackQuery(ctx, cb.ID, "Показываю")
		return true
	case cbAdminUsers:
		s.adminActiveUsers(ctx, chatID)
		_ = s.logAudit(ctx, u.ID, "telegram.admin.users.list_active", nil)
		_ = s.Bot.AnswerCallbackQuery(ctx, cb.ID, "Показываю")
		return true
	case cbAdminStat:
		_ = s.States.Set(ctx, cb.From.ID, StateAwaitUserStat)
		_ = s.Bot.SendMessage(ctx, chatID, "Введите telegram id или @username:", nil)
		_ = s.Bot.AnswerCallbackQuery(ctx, cb.ID, "Жду пользователя")
		return true
	case cbAdminRev:
		_ = s.States.Set(ctx, cb.From.ID, StateAwaitRevokeID)
		_ = s.Bot.SendMessage(ctx, chatID, "Введите telegram id или @username для отзыва доступа:", nil)
		_ = s.Bot.AnswerCallbackQuery(ctx, cb.ID, "Жду пользователя")
		return true
	case cbAdminNode:
		s.adminNodeStatus(ctx, chatID)
		_ = s.logAudit(ctx, u.ID, "telegram.admin.nodes.status", nil)
		_ = s.Bot.AnswerCallbackQuery(ctx, cb.ID, "Показываю")
		return true
	default:
		return false
	}
}

func (s *TelegramService) ensureUser(ctx context.Context, from User, chatID int64) (*user.User, bool) {
	u, err := s.RegisterUC.Execute(ctx, app.RegisterTelegramUserInput{TelegramID: from.ID, Username: from.Username, FirstName: from.FirstName, LastName: from.LastName})
	if err != nil {
		s.logErr("register user", err)
		_ = s.Bot.SendMessage(ctx, chatID, "Не удалось обработать запрос. Попробуйте позже.", nil)
		return nil, false
	}
	return u, true
}

func (s *TelegramService) sendMainMenu(ctx context.Context, chatID int64, isAdmin bool) {
	text := "Добро пожаловать в RyazanVPN!\n\nПреимущества:\n• быстрый VPN\n• не более 40 пользователей на сервер\n• низкая загрузка\n• стабильность\n• личная выдача доступа\n• простой импорт конфига"
	_ = s.Bot.SendMessage(ctx, chatID, text, mainMenu(isAdmin))
	if isAdmin {
		_ = s.Bot.SendMessage(ctx, chatID, "Админ-меню:", adminMenu())
	}
}

func (s *TelegramService) promptInviteCode(ctx context.Context, chatID, telegramID int64) {
	_ = s.States.Set(ctx, telegramID, StateAwaitInvite)
	_ = s.Bot.SendMessage(ctx, chatID, "Введите 4-значный invite code:", nil)
}

func (s *TelegramService) handleInviteCode(ctx context.Context, chatID, telegramID int64, userID, code string) {
	grant, err := s.AccessGrants.GetLatestByUserID(ctx, userID)
	if err != nil && !errors.Is(err, accessgrant.ErrNotFound) {
		s.logErr("get active access grant", err)
		_ = s.Bot.SendMessage(ctx, chatID, "Ошибка проверки доступа. Попробуйте позже.", nil)
		return
	}
	if grant == nil || grant.Status != accessgrant.StatusActive {
		if err := s.ActivateInviteUC.Execute(ctx, app.ActivateInviteCodeInput{UserID: userID, Code: code}); err != nil {
			s.handleInviteError(ctx, chatID, err)
			return
		}
		_ = s.Bot.SendMessage(ctx, chatID, "Код активирован ✅", nil)
	}
	_ = s.States.Set(ctx, telegramID, StateIdle)
	s.handleGetConfig(ctx, chatID, userID)
}

func (s *TelegramService) handleInviteError(ctx context.Context, chatID int64, err error) {
	switch {
	case errors.Is(err, app.ErrInvalidInviteCodeFormat):
		_ = s.Bot.SendMessage(ctx, chatID, "Код должен состоять из 4 цифр.", nil)
	case errors.Is(err, app.ErrInviteCodeNotUsable):
		_ = s.Bot.SendMessage(ctx, chatID, "Код неактивен или просрочен.", nil)
	case errors.Is(err, app.ErrInviteAlreadyUsed):
		_ = s.Bot.SendMessage(ctx, chatID, "Этот код уже использован вами.", nil)
	case errors.Is(err, app.ErrUserAlreadyHasActiveAccessGrant):
		_ = s.Bot.SendMessage(ctx, chatID, "У вас уже есть активный доступ.", nil)
	default:
		s.logErr("activate invite", err)
		_ = s.Bot.SendMessage(ctx, chatID, "Не удалось активировать код.", nil)
	}
}

func (s *TelegramService) sendAccessStatus(ctx context.Context, chatID int64, userID string) {
	grant, err := s.AccessGrants.GetLatestByUserID(ctx, userID)
	if err != nil && !errors.Is(err, accessgrant.ErrNotFound) {
		s.logErr("get access status", err)
		_ = s.Bot.SendMessage(ctx, chatID, "Не удалось получить статус доступа.", nil)
		return
	}
	if grant == nil {
		_ = s.Bot.SendMessage(ctx, chatID, "Доступ: отсутствует\nСтатус: expired\nИстекает: —\nУстройств: 0", mainMenu(false))
		return
	}
	deviceCount := 0
	if devices, err := s.Devices.ListByUserID(ctx, userID); err == nil {
		for _, d := range devices {
			if d.Status == device.StatusActive {
				deviceCount++
			}
		}
	}
	text := fmt.Sprintf("Доступ: есть\nСтатус: %s\nИстекает: %s\nУстройств: %d/%d", grant.Status, grant.ExpiresAt.Format(time.RFC3339), deviceCount, grant.DevicesLimit)
	if s.Traffic != nil {
		total, _ := s.Traffic.GetUserTrafficTotal(ctx, userID)
		last30, _ := s.Traffic.GetUserTrafficLastNDays(ctx, userID, 30, time.Now().UTC())
		text += fmt.Sprintf("\nТрафик всего: %s\nТрафик за 30 дней: %s", humanBytes(total), humanBytes(last30))
	}
	_ = s.Bot.SendMessage(ctx, chatID, text, nil)
}

func (s *TelegramService) handleGetConfig(ctx context.Context, chatID int64, userID string) {
	grant, err := s.GetGrantUC.Execute(ctx, app.GetActiveAccessGrantByUserInput{UserID: userID})
	if err != nil {
		if errors.Is(err, accessgrant.ErrNotFound) {
			_ = s.Bot.SendMessage(ctx, chatID, "Сначала активируйте invite code.", nil)
			return
		}
		s.logErr("get grant for config", err)
		_ = s.Bot.SendMessage(ctx, chatID, "Ошибка проверки доступа.", nil)
		return
	}
	if grant.Status != accessgrant.StatusActive {
		_ = s.Bot.SendMessage(ctx, chatID, "Ваш доступ неактивен. Статус: "+grant.Status, nil)
		return
	}

	d, err := s.Devices.GetActiveByUserID(ctx, userID)
	if err != nil && !errors.Is(err, device.ErrNotFound) {
		s.logErr("get active device", err)
		_ = s.Bot.SendMessage(ctx, chatID, "Не удалось получить устройство.", nil)
		return
	}

	var accessID string
	if d == nil {
		s.logInfo("telegram.create_device.start", "user_id", userID, "chat_id", chatID)
		out, err := s.CreateDeviceUC.Execute(ctx, app.CreateDeviceForUserInput{UserID: userID, Name: "telegram-device", Platform: "telegram"})
		if err != nil {
			s.logErr("create device", err)
			_ = s.Bot.SendMessage(ctx, chatID, "Не удалось создать устройство.", nil)
			return
		}
		deviceID := ""
		nodeID := ""
		if out.Device != nil {
			deviceID = out.Device.ID
		}
		if out.Node != nil {
			nodeID = out.Node.ID
		}
		s.logInfo("telegram.create_device.success", "user_id", userID, "device_id", deviceID, "access_id", out.Access.ID, "node_id", nodeID)
		accessID = out.Access.ID
	} else {
		actives, err := s.Accesses.GetActiveByDeviceID(ctx, d.ID)
		if err != nil || len(actives) == 0 {
			_ = s.Bot.SendMessage(ctx, chatID, "Нет активного доступа устройства. Удалите устройство и создайте заново.", nil)
			return
		}
		accessID = actives[0].ID
	}

	s.sendConfigDocumentByAccessID(ctx, chatID, userID, accessID)
}

func (s *TelegramService) askDeleteConfirm(ctx context.Context, chatID, telegramID int64) {
	_ = s.States.Set(ctx, telegramID, StateAwaitDeleteOK)
	_ = s.Bot.SendMessage(ctx, chatID, "Удалить активное устройство?", &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{{Text: "Да", Data: cbDeleteYes}, {Text: "Нет", Data: cbDeleteNo}}}})
}

func (s *TelegramService) handleDeleteDevice(ctx context.Context, chatID, telegramID int64, userID string) {
	_ = s.States.Set(ctx, telegramID, StateIdle)
	d, err := s.Devices.GetActiveByUserID(ctx, userID)
	if err != nil {
		_ = s.Bot.SendMessage(ctx, chatID, "Активных устройств нет.", nil)
		return
	}
	actives, err := s.Accesses.GetActiveByDeviceID(ctx, d.ID)
	if err == nil && len(actives) > 0 {
		_ = s.RevokeAccessUC.Execute(ctx, app.RevokeDeviceAccessInput{AccessID: actives[0].ID})
	}
	_ = s.Devices.Revoke(ctx, d.ID)
	_ = s.Bot.SendMessage(ctx, chatID, "Устройство удалено ✅", nil)
}

func (s *TelegramService) sendHelp(ctx context.Context, chatID int64) {
	_ = s.Bot.SendMessage(ctx, chatID, "Помощь:\n1) Введите код\n2) Получите конфиг\n3) При необходимости удалите устройство.", nil)
}

func (s *TelegramService) createSingleCode(ctx context.Context, chatID int64, actorUserID string) {
	code, err := s.createUniqueInviteCode(ctx, actorUserID)
	if err != nil {
		s.logErr("create single invite", err)
		_ = s.Bot.SendMessage(ctx, chatID, "Не удалось создать код.", nil)
		return
	}
	_ = s.Bot.SendMessage(ctx, chatID, "Новый код: `"+code.Code+"`\nСрок доступа после активации: 30 дней.", nil)
}

func (s *TelegramService) createBatchCodes(ctx context.Context, chatID int64, actorUserID string, n int) {
	codes := make([]string, 0, n)
	for i := 0; i < n; i++ {
		ic, err := s.createUniqueInviteCode(ctx, actorUserID)
		if err != nil {
			_ = s.Bot.SendMessage(ctx, chatID, fmt.Sprintf("Создано %d/%d кодов, далее ошибка: %v", len(codes), n, err), nil)
			return
		}
		codes = append(codes, ic.Code)
	}
	msg := "Пачка кодов:\n" + strings.Join(codes, "\n")
	if len(msg) > 3500 {
		msg = "Пачка кодов (первые 200):\n" + strings.Join(codes[:minInt(200, len(codes))], "\n")
	}
	_ = s.Bot.SendMessage(ctx, chatID, msg, nil)
	_ = s.logAudit(ctx, actorUserID, "telegram.admin.invite_codes.batch", map[string]any{"count": n})
}

func (s *TelegramService) adminLastCodes(ctx context.Context, chatID int64) {
	items, err := s.InviteCodes.ListRecent(ctx, 20)
	if err != nil {
		_ = s.Bot.SendMessage(ctx, chatID, "Не удалось получить список кодов.", nil)
		return
	}
	if len(items) == 0 {
		_ = s.Bot.SendMessage(ctx, chatID, "Кодов пока нет.", nil)
		return
	}
	b := &strings.Builder{}
	b.WriteString("Последние коды:\n")
	for _, it := range items {
		fmt.Fprintf(b, "%s | %s | %d/%d | %s\n", it.Code, it.Status, it.CurrentActivations, it.MaxActivations, it.CreatedAt.Format(time.RFC3339))
	}
	_ = s.Bot.SendMessage(ctx, chatID, b.String(), nil)
}

func (s *TelegramService) adminActiveUsers(ctx context.Context, chatID int64) {
	items, err := s.AccessGrants.ListActiveUsers(ctx, 200)
	if err != nil {
		_ = s.Bot.SendMessage(ctx, chatID, "Не удалось получить активных пользователей.", nil)
		return
	}
	if len(items) == 0 {
		_ = s.Bot.SendMessage(ctx, chatID, "Активных пользователей нет.", nil)
		return
	}
	b := &strings.Builder{}
	b.WriteString("Активные пользователи:\n")
	for _, it := range items {
		fmt.Fprintf(b, "id=%d @%s expires=%s\n", it.TelegramID, it.Username, it.ExpiresAt.Format(time.RFC3339))
	}
	_ = s.Bot.SendMessage(ctx, chatID, b.String(), nil)
}

func (s *TelegramService) adminUserStats(ctx context.Context, chatID int64, actorUserID, token string) {
	u, err := s.findUser(ctx, token)
	if err != nil {
		_ = s.Bot.SendMessage(ctx, chatID, "Пользователь не найден.", nil)
		return
	}
	grant, _ := s.AccessGrants.GetLatestByUserID(ctx, u.ID)
	d, _ := s.Devices.GetActiveByUserID(ctx, u.ID)
	deviceInfo := "нет"
	if d != nil {
		deviceInfo = d.Name + " (" + d.Platform + ")"
	}
	status := "expired"
	exp := "—"
	if grant != nil {
		status = grant.Status
		exp = grant.ExpiresAt.Format(time.RFC3339)
	}
	total := int64(0)
	last30 := int64(0)
	if s.Traffic != nil {
		total, _ = s.Traffic.GetUserTrafficTotal(ctx, u.ID)
		last30, _ = s.Traffic.GetUserTrafficLastNDays(ctx, u.ID, 30, time.Now().UTC())
	}
	msg := fmt.Sprintf("Пользователь: id=%d @%s\nСтатус доступа: %s\nExpires: %s\nУстройство: %s\nТрафик total: %s\nТрафик 30 дней: %s", u.TelegramID, u.Username, status, exp, deviceInfo, humanBytes(total), humanBytes(last30))
	_ = s.Bot.SendMessage(ctx, chatID, msg, nil)
	_ = s.logAudit(ctx, actorUserID, "telegram.admin.user_stats", map[string]any{"target_user_id": u.ID})
}

func (s *TelegramService) adminRevokeUser(ctx context.Context, chatID int64, actorUserID, token string) {
	u, err := s.findUser(ctx, token)
	if err != nil {
		_ = s.Bot.SendMessage(ctx, chatID, "Пользователь не найден.", nil)
		return
	}
	revoked, err := s.AccessGrants.RevokeActiveByUserID(ctx, u.ID)
	if err != nil {
		_ = s.Bot.SendMessage(ctx, chatID, "Не удалось отозвать access grant.", nil)
		return
	}
	if d, err := s.Devices.GetActiveByUserID(ctx, u.ID); err == nil && d != nil {
		if activeAccesses, err := s.Accesses.GetActiveByDeviceID(ctx, d.ID); err == nil && len(activeAccesses) > 0 {
			_ = s.RevokeAccessUC.Execute(ctx, app.RevokeDeviceAccessInput{AccessID: activeAccesses[0].ID})
		}
		_ = s.Devices.Revoke(ctx, d.ID)
	}
	_ = s.Bot.SendMessage(ctx, chatID, fmt.Sprintf("Доступ пользователя отозван. revoke grants=%d", revoked), nil)
	_ = s.logAudit(ctx, actorUserID, "telegram.admin.revoke_access", map[string]any{"target_user_id": u.ID, "revoked_grants": revoked})
}

func (s *TelegramService) adminNodeStatus(ctx context.Context, chatID int64) {
	nodes, err := s.Nodes.ListActive(ctx)
	if err != nil {
		_ = s.Bot.SendMessage(ctx, chatID, "Не удалось получить статус нод.", nil)
		return
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
	b := &strings.Builder{}
	b.WriteString("Статус нод:\n")
	for _, n := range nodes {
		capacity := n.UserCapacity
		if capacity <= 0 {
			capacity = s.NodeCapacity
		}
		if capacity <= 0 {
			capacity = 40
		}
		fmt.Fprintf(b, "%s | active_users=%d | capacity=%d | health=%s\n", n.Name, n.CurrentLoad, capacity, n.Status)
	}
	_ = s.Bot.SendMessage(ctx, chatID, b.String(), nil)
}

func (s *TelegramService) createUniqueInviteCode(ctx context.Context, actorUserID string) (*invitecode.InviteCode, error) {
	for i := 0; i < 50; i++ {
		code := fmt.Sprintf("%04d", randIntN(10000))
		created, err := s.InviteCodes.Create(ctx, invitecode.CreateParams{Code: code, Status: invitecode.CodeStatusActive, MaxActivations: 1, CreatedByUserID: &actorUserID})
		if err == nil {
			_ = s.logAudit(ctx, actorUserID, "telegram.admin.invite_code.create", map[string]any{"code": code})
			return created, nil
		}
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			continue
		}
		return nil, err
	}
	return nil, fmt.Errorf("failed to generate unique code after retries")
}

func (s *TelegramService) findUser(ctx context.Context, token string) (*user.User, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, user.ErrNotFound
	}
	if strings.HasPrefix(token, "@") {
		token = strings.TrimPrefix(token, "@")
		return s.Users.GetByUsername(ctx, token)
	}
	if n, err := strconv.ParseInt(token, 10, 64); err == nil {
		return s.Users.GetByTelegramID(ctx, n)
	}
	return s.Users.GetByUsername(ctx, token)
}

func mainMenu(isAdmin bool) *InlineKeyboardMarkup {
	rows := [][]InlineKeyboardButton{{{Text: "Ввести код", Data: cbEnterCode}, {Text: "Мой доступ", Data: cbMyAccess}}, {{Text: "Получить конфиг", Data: cbConfig}}, {{Text: "Удалить устройство", Data: cbDelete}, {Text: "Помощь", Data: cbHelp}}}
	if isAdmin {
		rows = append(rows, []InlineKeyboardButton{{Text: "Админка", Data: cbAdminMenu}})
	}
	return &InlineKeyboardMarkup{InlineKeyboard: rows}
}

func configReadyMenu() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{
		{{Text: "Скачать .conf", Data: cbCfgFile}},
		{{Text: "Показать QR", Data: cbCfgQR}, {Text: "Показать текст", Data: cbCfgText}},
		{{Text: "Ключ для DefaultVPN", Data: cbCfgDefVPN}},
	}}
}

func adminMenu() *InlineKeyboardMarkup {
	return &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{
		{{Text: "Создать 1 код", Data: cbAdminSingle}, {Text: "Создать пачку кодов", Data: cbAdminBatch}},
		{{Text: "Последние коды", Data: cbAdminLast}, {Text: "Активные пользователи", Data: cbAdminUsers}},
		{{Text: "Статус нод", Data: cbAdminNode}, {Text: "Статистика пользователя", Data: cbAdminStat}},
		{{Text: "Отозвать доступ", Data: cbAdminRev}},
	}}
}

func (s *TelegramService) issueDownloadToken(ctx context.Context, accessID string) (string, error) {
	acc, err := s.Accesses.GetByID(ctx, accessID)
	if err != nil {
		return "", err
	}
	if len(acc.ConfigBlobEncrypted) == 0 {
		return "", fmt.Errorf("config not ready for access_id=%s: encrypted blob is empty", accessID)
	}
	if s.TokenTTL <= 0 {
		s.TokenTTL = 15 * time.Minute
	}
	raw, err := randomToken()
	if err != nil {
		return "", err
	}
	_, err = s.Tokens.Create(ctx, token.CreateParams{DeviceAccessID: accessID, TokenHash: hashToken(raw), Status: token.StatusIssued, ExpiresAt: time.Now().UTC().Add(s.TokenTTL)})
	if err != nil {
		return "", err
	}
	s.logInfo("telegram.token.created", "access_id", accessID, "ttl", s.TokenTTL.String())
	return raw, nil
}

func (s *TelegramService) sendConfigDocument(ctx context.Context, chatID int64, userID string) {
	accessID, err := s.resolveActiveAccessID(ctx, userID)
	if err != nil {
		_ = s.Bot.SendMessage(ctx, chatID, "Нет готового конфига. Нажмите «Получить конфиг».", nil)
		return
	}
	s.sendConfigDocumentByAccessID(ctx, chatID, userID, accessID)
}

func (s *TelegramService) sendConfigDocumentByAccessID(ctx context.Context, chatID int64, userID, accessID string) {
	configPlain, err := s.loadConfigPlaintext(ctx, accessID)
	if err != nil {
		s.logErr("load config plaintext", err)
		_ = s.Bot.SendMessage(ctx, chatID, "Конфиг ещё не готов. Проверьте корректность invite code и попробуйте заново.", nil)
		return
	}

	if err := s.Bot.SendDocument(ctx, chatID, "rznvpn.conf", []byte(configPlain), "Готово ✅ Конфиг для AmneziaWG/WireGuard", configReadyMenu()); err != nil {
		s.logErr("telegram send document", err)
		_ = s.Bot.SendMessage(ctx, chatID, "Не удалось отправить .conf файлом, используйте кнопку «Показать текст».", configReadyMenu())
		return
	}
	s.logInfo("telegram.delivery.document", "user_id", userID, "access_id", accessID, "chat_id", chatID)
}

func (s *TelegramService) sendConfigQR(ctx context.Context, chatID int64, userID string) {
	accessID, err := s.resolveActiveAccessID(ctx, userID)
	if err != nil {
		_ = s.Bot.SendMessage(ctx, chatID, "Нет готового конфига. Нажмите «Получить конфиг».", nil)
		return
	}
	configPlain, err := s.loadConfigPlaintext(ctx, accessID)
	if err != nil {
		s.logErr("load config plaintext for qr", err)
		_ = s.Bot.SendMessage(ctx, chatID, "QR недоступен: конфиг ещё не готов.", configReadyMenu())
		return
	}
	png, err := s.generateConfigQR(ctx, configPlain)
	if err != nil {
		s.logErr("generate qr", err)
		_ = s.Bot.SendMessage(ctx, chatID, "Не удалось сгенерировать QR.", configReadyMenu())
		return
	}
	if err := s.Bot.SendPhoto(ctx, chatID, "rznvpn-qr.png", png, "QR для быстрого импорта конфига", configReadyMenu()); err != nil {
		s.logErr("telegram send qr image", err)
		_ = s.Bot.SendMessage(ctx, chatID, "Не удалось отправить QR, попробуйте снова.", configReadyMenu())
		return
	}
	s.logInfo("telegram.delivery.qr", "user_id", userID, "access_id", accessID, "chat_id", chatID)
}

func (s *TelegramService) generateConfigQR(ctx context.Context, configPlain string) ([]byte, error) {
	endpoint := "https://api.qrserver.com/v1/create-qr-code/?size=512x512&format=png&data=" + url.QueryEscape(configPlain)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("qr service status=%d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

func (s *TelegramService) sendConfigText(ctx context.Context, chatID int64, userID string) {
	accessID, err := s.resolveActiveAccessID(ctx, userID)
	if err != nil {
		_ = s.Bot.SendMessage(ctx, chatID, "Нет готового конфига. Нажмите «Получить конфиг».", nil)
		return
	}
	configPlain, err := s.loadConfigPlaintext(ctx, accessID)
	if err != nil {
		s.logErr("load config plaintext for text fallback", err)
		_ = s.Bot.SendMessage(ctx, chatID, "Конфиг ещё не готов.", configReadyMenu())
		return
	}
	_ = s.Bot.SendMessage(ctx, chatID, "Текстовый fallback (лучше использовать файл .conf):\n```\n"+configPlain+"\n```", configReadyMenu())
	s.logInfo("telegram.delivery.text", "user_id", userID, "access_id", accessID, "chat_id", chatID)
}

func (s *TelegramService) sendDefaultVPNKey(ctx context.Context, chatID int64, userID string) {
	if s.VPNExporter == nil {
		_ = s.Bot.SendMessage(ctx, chatID, "Экспорт ключа пока недоступен.", configReadyMenu())
		return
	}
	accessID, err := s.resolveActiveAccessID(ctx, userID)
	if err != nil {
		_ = s.Bot.SendMessage(ctx, chatID, "Нет активного доступа для генерации ключа.", configReadyMenu())
		return
	}
	configPlain, err := s.loadConfigPlaintext(ctx, accessID)
	if err != nil {
		s.logErr("load config plaintext for defaultvpn", err)
		_ = s.Bot.SendMessage(ctx, chatID, "Конфиг ещё не готов.", configReadyMenu())
		return
	}

	d, err := s.Devices.GetActiveByUserID(ctx, userID)
	if err != nil {
		s.logErr("load device for defaultvpn", err)
		_ = s.Bot.SendMessage(ctx, chatID, "Не удалось получить данные устройства.", configReadyMenu())
		return
	}

	key, err := s.VPNExporter.ExportDefaultVPN(ctx, app.ExportVPNKeyInput{
		Config:             configPlain,
		Description:        "RyazanVPN",
		ProtocolVersion:    2,
		TransportProto:     "udp",
		DefaultContainerID: "amnezia-awg2",
		ClientPublicKey:    d.PublicKey,
		MTU:                s.DefaultVPNMTU,
		AWG:                s.DefaultVPNAWG,
	})
	if err != nil {
		s.logErr("export defaultvpn key", err)
		_ = s.Bot.SendMessage(ctx, chatID, "Не удалось сформировать DefaultVPN ключ.", configReadyMenu())
		return
	}
	_ = s.Bot.SendMessage(ctx, chatID, "Ключ для DefaultVPN:\n`"+key+"`", configReadyMenu())
	s.logInfo("telegram.delivery.defaultvpn", "user_id", userID, "access_id", accessID, "chat_id", chatID)
}

func (s *TelegramService) resolveActiveAccessID(ctx context.Context, userID string) (string, error) {
	d, err := s.Devices.GetActiveByUserID(ctx, userID)
	if err != nil {
		return "", err
	}
	actives, err := s.Accesses.GetActiveByDeviceID(ctx, d.ID)
	if err != nil || len(actives) == 0 {
		return "", fmt.Errorf("active access is missing for device_id=%s", d.ID)
	}
	return actives[0].ID, nil
}

func (s *TelegramService) loadConfigPlaintext(ctx context.Context, accessID string) (string, error) {
	acc, err := s.Accesses.GetByID(ctx, accessID)
	if err != nil {
		return "", err
	}
	if len(acc.ConfigBlobEncrypted) == 0 {
		return "", fmt.Errorf("config not ready for access_id=%s: encrypted blob is empty", accessID)
	}
	if s.ConfigEncryptor == nil {
		return "", fmt.Errorf("config decryptor is not configured")
	}
	plain, err := s.ConfigEncryptor.Decrypt(acc.ConfigBlobEncrypted)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (s *TelegramService) isAdmin(telegramID int64) bool {
	_, ok := s.AdminIDs[telegramID]
	return ok
}

func (s *TelegramService) logAudit(ctx context.Context, actorUserID, action string, details map[string]any) error {
	if s.AuditLogs == nil {
		return nil
	}
	actor := actorUserID
	detail := "{}"
	if len(details) > 0 {
		detail = fmt.Sprintf("%v", details)
	}
	_, err := s.AuditLogs.Create(ctx, audit.CreateParams{ActorUserID: &actor, EntityType: "telegram_admin", Action: action, DetailsJSON: detail})
	return err
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

func randIntN(n int) int {
	if n <= 0 {
		return 0
	}
	var b [2]byte
	_, _ = rand.Read(b[:])
	return (int(b[0])<<8 | int(b[1])) % n
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func humanBytes(v int64) string {
	const unit = 1024
	if v < unit {
		return fmt.Sprintf("%d B", v)
	}
	div, exp := int64(unit), 0
	for n := v / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(v)/float64(div), "KMGTPE"[exp])
}

func (s *TelegramService) logErr(msg string, err error) {
	if s.Logger != nil {
		s.Logger.Error(msg, slog.Any("error", err))
	}
}

func (s *TelegramService) logInfo(msg string, args ...any) {
	if s.Logger != nil {
		s.Logger.Info(msg, args...)
	}
}

var _ = access.StatusActive
var _ = node.StatusActive
