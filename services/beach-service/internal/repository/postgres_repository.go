package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/omer-asraf/shaka-beach-service/internal/domain"
)

type PostgresBeachRepository struct {
	db *pgxpool.Pool
}

// NewPostgresBeachRepository יוצר מופע חדש של ה-Repository
func NewPostgresBeachRepository(db *pgxpool.Pool) *PostgresBeachRepository {
	return &PostgresBeachRepository{db: db}
}

// GetAll שולף את כל החופים הקיימים
func (r *PostgresBeachRepository) GetAll(ctx context.Context) ([]domain.Beach, error) {
	query := `
		SELECT id, name, latitude, longitude, COALESCE(wave_height, 0), COALESCE(wind_speed, 0),
		       COALESCE(description, ''), created_at, updated_at
		FROM beaches
	`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query beaches: %w", err)
	}
	defer rows.Close()

	var beaches []domain.Beach
	for rows.Next() {
		var b domain.Beach
		err := rows.Scan(
			&b.ID, &b.Name, &b.Latitude, &b.Longitude,
			&b.WaveHeight, &b.WindSpeed, &b.Description,
			&b.CreatedAt, &b.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan beach: %w", err)
		}
		beaches = append(beaches, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate beaches: %w", err)
	}

	return beaches, nil
}

// FindNearby מבצעת חיפוש מרחבי באמצעות PostGIS לפי רדיוס בקילומטרים
func (r *PostgresBeachRepository) FindNearby(ctx context.Context, lat, lon float64, radiusKm float64) ([]domain.Beach, error) {
	// ST_DWithin מודדת מרחק במטרים (לכן radiusKm * 1000)
	query := `
		SELECT id, name, latitude, longitude, COALESCE(wave_height, 0), COALESCE(wind_speed, 0),
		       COALESCE(description, ''), created_at, updated_at
		FROM beaches
		WHERE ST_DWithin(
			location,
			ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography,
			$3
		)
		ORDER BY ST_Distance(
			location,
			ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography
		) ASC
	`
	radiusMeters := radiusKm * 1000
	rows, err := r.db.Query(ctx, query, lon, lat, radiusMeters)
	if err != nil {
		return nil, fmt.Errorf("failed to query nearby beaches: %w", err)
	}
	defer rows.Close()

	var beaches []domain.Beach
	for rows.Next() {
		var b domain.Beach
		err := rows.Scan(
			&b.ID, &b.Name, &b.Latitude, &b.Longitude,
			&b.WaveHeight, &b.WindSpeed, &b.Description,
			&b.CreatedAt, &b.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan nearby beach: %w", err)
		}
		beaches = append(beaches, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate nearby beaches: %w", err)
	}

	return beaches, nil
}

// GetByID שולף חוף בודד
func (r *PostgresBeachRepository) GetByID(ctx context.Context, id string) (*domain.Beach, error) {
	query := `
		SELECT id, name, latitude, longitude, COALESCE(wave_height, 0), COALESCE(wind_speed, 0),
		       COALESCE(description, ''), created_at, updated_at
		FROM beaches
		WHERE id = $1
	`
	var b domain.Beach
	err := r.db.QueryRow(ctx, query, id).Scan(
		&b.ID, &b.Name, &b.Latitude, &b.Longitude,
		&b.WaveHeight, &b.WindSpeed, &b.Description,
		&b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", domain.ErrBeachNotFound, id)
		}
		return nil, fmt.Errorf("failed to get beach: %w", err)
	}
	return &b, nil
}

// Create מוסיף חוף חדש
func (r *PostgresBeachRepository) Create(ctx context.Context, b *domain.Beach) error {
	query := `
		INSERT INTO beaches (id, name, latitude, longitude, location, wave_height, wind_speed, description)
		VALUES ($1, $2, $3, $4, ST_SetSRID(ST_MakePoint($4, $3), 4326)::geography, $5, $6, $7)
		RETURNING created_at, updated_at
	`
	if err := r.db.QueryRow(ctx, query,
		b.ID, b.Name, b.Latitude, b.Longitude, b.WaveHeight, b.WindSpeed, b.Description,
	).Scan(&b.CreatedAt, &b.UpdatedAt); err != nil {
		return fmt.Errorf("failed to create beach: %w", err)
	}
	return nil
}

func (r *PostgresBeachRepository) UpdateWaveHeight(ctx context.Context, id string, height float64) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE beaches
		SET wave_height = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, id, height)
	if err != nil {
		return fmt.Errorf("failed to update beach swell height: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", domain.ErrBeachNotFound, id)
	}
	return nil
}
