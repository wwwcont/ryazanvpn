package app

import (
	"context"
	"errors"

	"github.com/wwwcont/ryazanvpn/internal/domain/user"
)

type RegisterTelegramUserInput struct {
	TelegramID int64
	Username   string
	FirstName  string
	LastName   string
}

type RegisterTelegramUser struct {
	Users UserRepository
}

func (uc RegisterTelegramUser) Execute(ctx context.Context, in RegisterTelegramUserInput) (*user.User, error) {
	existing, err := uc.Users.GetByTelegramID(ctx, in.TelegramID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, user.ErrNotFound) {
		return nil, err
	}

	return uc.Users.Create(ctx, user.CreateParams{
		TelegramID: in.TelegramID,
		Username:   in.Username,
		FirstName:  in.FirstName,
		LastName:   in.LastName,
		Status:     user.StatusActive,
	})
}
