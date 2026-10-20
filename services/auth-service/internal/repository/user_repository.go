package repository

import (
	"context"
	"errors"

	"github.com/omer-asraf/shaka-auth-service/internal/domain"
)

var (
	ErrUserNotFound = errors.New("user not found")
	ErrEmailInUse   = errors.New("email already registered")
)

type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetByID(ctx context.Context, id string) (*domain.User, error)
}
