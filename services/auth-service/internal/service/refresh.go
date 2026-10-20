package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/omer-asraf/shaka-auth-service/internal/domain"
	"github.com/omer-asraf/shaka-auth-service/internal/repository"
)

// ErrInvalidRefreshToken is returned for unknown, expired, revoked or replayed
// refresh tokens. The caller deliberately cannot tell which one it was.
var ErrInvalidRefreshToken = errors.New("invalid refresh token")

type RefreshTokenRepository interface {
	Create(ctx context.Context, token *domain.RefreshToken) error
	Consume(ctx context.Context, tokenHash string) (*domain.RefreshToken, bool, error)
	RevokeFamily(ctx context.Context, familyID string) error
}

// Session is what a successful login or refresh hands back to the transport
// layer: a short-lived access token and an opaque refresh token.
type Session struct {
	AccessToken      string
	ExpiresAt        time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
	Email            string
}

func (s *AuthService) newSession(ctx context.Context, user *domain.User, familyID string) (*Session, error) {
	access, expiresAt, err := s.tokens.Issue(user)
	if err != nil {
		return nil, fmt.Errorf("issue access token: %w", err)
	}
	raw, err := newRefreshTokenValue()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	id, err := newUserID()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token ID: %w", err)
	}
	refreshExpiresAt := s.now().UTC().Add(s.refreshTTL)
	record := &domain.RefreshToken{
		ID:        id,
		UserID:    user.ID,
		FamilyID:  familyID,
		TokenHash: HashRefreshToken(raw),
		ExpiresAt: refreshExpiresAt,
	}
	if err := s.refresh.Create(ctx, record); err != nil {
		return nil, err
	}
	return &Session{
		AccessToken:      access,
		ExpiresAt:        expiresAt,
		RefreshToken:     raw,
		RefreshExpiresAt: refreshExpiresAt,
		Email:            user.Email,
	}, nil
}

// Refresh rotates a refresh token: the presented token is revoked and a new
// token in the same family is issued. Presenting a token that was already
// revoked is treated as theft or replay and revokes the whole family.
func (s *AuthService) Refresh(ctx context.Context, rawToken string) (*Session, error) {
	if rawToken == "" {
		return nil, ErrInvalidRefreshToken
	}
	stored, active, err := s.refresh.Consume(ctx, HashRefreshToken(rawToken))
	if err != nil {
		if errors.Is(err, repository.ErrRefreshTokenNotFound) {
			return nil, ErrInvalidRefreshToken
		}
		return nil, fmt.Errorf("consume refresh token: %w", err)
	}
	if !active {
		if err := s.refresh.RevokeFamily(ctx, stored.FamilyID); err != nil {
			return nil, fmt.Errorf("revoke token family after reuse: %w", err)
		}
		return nil, ErrInvalidRefreshToken
	}
	if !s.now().Before(stored.ExpiresAt) {
		return nil, ErrInvalidRefreshToken
	}
	user, err := s.users.GetByID(ctx, stored.UserID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, ErrInvalidRefreshToken
		}
		return nil, fmt.Errorf("load user: %w", err)
	}
	return s.newSession(ctx, user, stored.FamilyID)
}

// Logout revokes the whole token family of the presented refresh token. It is
// idempotent: unknown or already-revoked tokens are not an error.
func (s *AuthService) Logout(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return nil
	}
	stored, _, err := s.refresh.Consume(ctx, HashRefreshToken(rawToken))
	if err != nil {
		if errors.Is(err, repository.ErrRefreshTokenNotFound) {
			return nil
		}
		return fmt.Errorf("consume refresh token: %w", err)
	}
	return s.refresh.RevokeFamily(ctx, stored.FamilyID)
}

// HashRefreshToken returns the hex SHA-256 of a raw token. The token carries
// 256 bits of entropy, so a fast hash is sufficient (bcrypt is for low-entropy
// passwords) and lets the database look tokens up by hash.
func HashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func newRefreshTokenValue() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}
