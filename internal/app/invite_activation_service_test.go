package app

import (
	"context"
	"errors"
	"testing"

	"github.com/wwwcont/ryazanvpn/internal/domain/invitecode"
	"github.com/wwwcont/ryazanvpn/internal/domain/user"
)

func TestInviteActivationService_ActivateByTelegramID_SuccessAndExhaust(t *testing.T) {
	t.Parallel()

	repo := &fakeInviteRepo{
		code: &invitecode.InviteCode{
			ID:                 "invite-1",
			Code:               "1111",
			Status:             invitecode.CodeStatusActive,
			MaxActivations:     1,
			CurrentActivations: 0,
		},
	}

	svc := &InviteActivationService{
		Store: fakeStore{
			users: &fakeUserRepo{u: &user.User{ID: "user-1", TelegramID: 1}},
			code:  repo,
		},
	}

	err := svc.ActivateByTelegramID(context.Background(), 1, "1111")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if repo.activationCount != 1 {
		t.Fatalf("expected activationCount=1, got %d", repo.activationCount)
	}
	if repo.incrementCount != 1 {
		t.Fatalf("expected incrementCount=1, got %d", repo.incrementCount)
	}
	if repo.code.Status != invitecode.CodeStatusExhausted {
		t.Fatalf("expected code status exhausted, got %s", repo.code.Status)
	}
}

func TestInviteActivationService_ActivateByTelegramID_AlreadyUsed(t *testing.T) {
	t.Parallel()

	svc := &InviteActivationService{
		Store: fakeStore{
			users: &fakeUserRepo{u: &user.User{ID: "user-1", TelegramID: 1}},
			code: &fakeInviteRepo{
				code: &invitecode.InviteCode{ID: "invite-1", Code: "1111", Status: invitecode.CodeStatusActive, MaxActivations: 5},
				has:  true,
			},
		},
	}

	err := svc.ActivateByTelegramID(context.Background(), 1, "1111")
	if !errors.Is(err, ErrInviteAlreadyUsed) {
		t.Fatalf("expected ErrInviteAlreadyUsed, got %v", err)
	}
}

type fakeStore struct {
	users user.Repository
	code  invitecode.Repository
}

func (f fakeStore) WithinTx(ctx context.Context, fn func(repos InviteActivationRepos) error) error {
	return fn(fakeRepos{users: f.users, code: f.code})
}

type fakeRepos struct {
	users user.Repository
	code  invitecode.Repository
}

func (f fakeRepos) Users() user.Repository {
	return f.users
}

func (f fakeRepos) InviteCodes() invitecode.Repository {
	return f.code
}

type fakeUserRepo struct {
	u   *user.User
	err error
}

func (f *fakeUserRepo) GetByTelegramID(ctx context.Context, telegramID int64) (*user.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.u, nil
}

func (f *fakeUserRepo) GetByUsername(ctx context.Context, username string) (*user.User, error) {
	panic("unexpected call")
}

func (f *fakeUserRepo) Create(ctx context.Context, in user.CreateParams) (*user.User, error) {
	panic("unexpected call")
}

func (f *fakeUserRepo) GetByID(ctx context.Context, id string) (*user.User, error) {
	panic("unexpected call")
}

type fakeInviteRepo struct {
	code            *invitecode.InviteCode
	has             bool
	activationCount int
	incrementCount  int
}

func (f *fakeInviteRepo) GetByCodeForUpdate(ctx context.Context, code string) (*invitecode.InviteCode, error) {
	return f.code, nil
}

func (f *fakeInviteRepo) IncrementUsageAndMaybeExhaust(ctx context.Context, inviteCodeID string) (*invitecode.InviteCode, error) {
	f.incrementCount++
	f.code.CurrentActivations++
	if f.code.CurrentActivations >= f.code.MaxActivations {
		f.code.Status = invitecode.CodeStatusExhausted
	}
	return f.code, nil
}

func (f *fakeInviteRepo) CreateActivation(ctx context.Context, in invitecode.CreateActivationParams) (*invitecode.Activation, error) {
	f.activationCount++
	return &invitecode.Activation{ID: "act-1", InviteCodeID: in.InviteCodeID, UserID: in.UserID}, nil
}

func (f *fakeInviteRepo) HasActivationByUser(ctx context.Context, inviteCodeID, userID string) (bool, error) {
	return f.has, nil
}

func (f *fakeInviteRepo) Create(ctx context.Context, in invitecode.CreateParams) (*invitecode.InviteCode, error) {
	panic("unexpected call")
}

func (f *fakeInviteRepo) ListRecent(ctx context.Context, limit int) ([]*invitecode.InviteCode, error) {
	panic("unexpected call")
}
