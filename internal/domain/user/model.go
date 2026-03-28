package user

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("user not found")

type User struct {
	ID         string
	TelegramID int64
	Username   string
	FirstName  string
	LastName   string
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Repository interface {
	GetByTelegramID(ctx context.Context, telegramID int64) (*User, error)
	Create(ctx context.Context, in CreateParams) (*User, error)
	GetByID(ctx context.Context, id string) (*User, error)
}

type CreateParams struct {
	TelegramID int64
	Username   string
	FirstName  string
	LastName   string
	Status     string
}
