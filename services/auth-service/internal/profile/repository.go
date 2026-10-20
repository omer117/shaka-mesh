package profile

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	Get(ctx context.Context, userID string) (*Profile, error)
	Update(ctx context.Context, userID string, p *Profile) error
	// SetAvatarURL stores the new URL and returns the previous one.
	SetAvatarURL(ctx context.Context, userID, url string) (previous string, err error)
}

type PostgresRepository struct{ db *pgxpool.Pool }

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Get(ctx context.Context, userID string) (*Profile, error) {
	p := &Profile{}
	err := r.db.QueryRow(ctx, `
		SELECT id, email, display_name, bio, COALESCE(home_beach_id, ''), avatar_url, created_at
		FROM users WHERE id = $1
	`, userID).Scan(&p.ID, &p.Email, &p.DisplayName, &p.Bio, &p.HomeBeachID, &p.AvatarURL, &p.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("select profile: %w", err)
	}
	return p, nil
}

func (r *PostgresRepository) Update(ctx context.Context, userID string, p *Profile) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE users SET display_name = $2, bio = $3, home_beach_id = NULLIF($4, '')
		WHERE id = $1
	`, userID, p.DisplayName, p.Bio, p.HomeBeachID)
	if err != nil {
		return fmt.Errorf("update profile: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) SetAvatarURL(ctx context.Context, userID, url string) (string, error) {
	var previous string
	err := r.db.QueryRow(ctx, `
		UPDATE users u SET avatar_url = $2
		FROM (SELECT avatar_url FROM users WHERE id = $1) old
		WHERE u.id = $1
		RETURNING old.avatar_url
	`, userID, url).Scan(&previous)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("update avatar url: %w", err)
	}
	return previous, nil
}
