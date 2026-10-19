package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/omer-asraf/shaka-auth-service/internal/domain"
)

type PostgresRefreshTokenRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRefreshTokenRepository(db *pgxpool.Pool) *PostgresRefreshTokenRepository {
	return &PostgresRefreshTokenRepository{db: db}
}

func (r *PostgresRefreshTokenRepository) Create(ctx context.Context, token *domain.RefreshToken) error {
	err := r.db.QueryRow(ctx, `
		INSERT INTO refresh_tokens (id, user_id, family_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at
	`, token.ID, token.UserID, token.FamilyID, token.TokenHash, token.ExpiresAt).Scan(&token.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert refresh token: %w", err)
	}
	return nil
}

func (r *PostgresRefreshTokenRepository) Consume(ctx context.Context, tokenHash string) (*domain.RefreshToken, bool, error) {
	token := &domain.RefreshToken{}
	// Single UPDATE: only one concurrent caller can flip revoked to true.
	err := r.db.QueryRow(ctx, `
		UPDATE refresh_tokens SET revoked = TRUE
		WHERE token_hash = $1 AND revoked = FALSE
		RETURNING id, user_id, family_id, token_hash, expires_at, created_at
	`, tokenHash).Scan(&token.ID, &token.UserID, &token.FamilyID, &token.TokenHash, &token.ExpiresAt, &token.CreatedAt)
	if err == nil {
		token.Revoked = true
		return token, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, fmt.Errorf("consume refresh token: %w", err)
	}
	err = r.db.QueryRow(ctx, `
		SELECT id, user_id, family_id, token_hash, expires_at, revoked, created_at
		FROM refresh_tokens WHERE token_hash = $1
	`, tokenHash).Scan(&token.ID, &token.UserID, &token.FamilyID, &token.TokenHash, &token.ExpiresAt, &token.Revoked, &token.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, ErrRefreshTokenNotFound
		}
		return nil, false, fmt.Errorf("select refresh token: %w", err)
	}
	return token, false, nil
}

func (r *PostgresRefreshTokenRepository) RevokeFamily(ctx context.Context, familyID string) error {
	if _, err := r.db.Exec(ctx, `
		UPDATE refresh_tokens SET revoked = TRUE WHERE family_id = $1 AND revoked = FALSE
	`, familyID); err != nil {
		return fmt.Errorf("revoke refresh token family: %w", err)
	}
	return nil
}
