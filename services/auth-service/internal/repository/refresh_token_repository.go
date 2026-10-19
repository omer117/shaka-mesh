package repository

import (
	"context"
	"errors"

	"github.com/omer-asraf/shaka-auth-service/internal/domain"
)

// ErrRefreshTokenNotFound means no stored token matches the presented hash.
var ErrRefreshTokenNotFound = errors.New("refresh token not found")

type RefreshTokenRepository interface {
	Create(ctx context.Context, token *domain.RefreshToken) error
	// Consume atomically marks the token with this hash as revoked. active is
	// true only for the caller that flipped it from not-revoked to revoked, so
	// concurrent presentations of one token yield exactly one winner. When the
	// token was already revoked, the stored row is returned with active=false.
	Consume(ctx context.Context, tokenHash string) (token *domain.RefreshToken, active bool, err error)
	// RevokeFamily revokes every token that shares the family ID.
	RevokeFamily(ctx context.Context, familyID string) error
}
