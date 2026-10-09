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

func (r *PostgresBeachRepository) EnsureConditionDetails(ctx context.Context) error {
	_, err := r.db.Exec(ctx, `
		ALTER TABLE beaches
			ADD COLUMN IF NOT EXISTS wave_period_seconds DOUBLE PRECISION NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS swell_direction_degrees DOUBLE PRECISION NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS water_temperature_celsius DOUBLE PRECISION NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS tide_height_meters DOUBLE PRECISION NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS tide_phase VARCHAR(20) NOT NULL DEFAULT 'N/A';

		UPDATE beaches
		SET wind_speed = CASE
				WHEN wind_speed = 0 THEN CASE id
					WHEN 'b1' THEN 7.5 WHEN 'b2' THEN 6.2 WHEN 'b3' THEN 9.1 ELSE wind_speed END
				ELSE wind_speed
			END,
			wave_period_seconds = CASE id
				WHEN 'b1' THEN 7.8 WHEN 'b2' THEN 6.9 WHEN 'b3' THEN 8.4 ELSE wave_period_seconds END,
			swell_direction_degrees = CASE id
				WHEN 'b1' THEN 285 WHEN 'b2' THEN 290 WHEN 'b3' THEN 305 ELSE swell_direction_degrees END,
			water_temperature_celsius = CASE id
				WHEN 'b1' THEN 24.1 WHEN 'b2' THEN 24.0 WHEN 'b3' THEN 23.2 ELSE water_temperature_celsius END,
			tide_height_meters = CASE id
				WHEN 'b1' THEN 0.42 WHEN 'b2' THEN 0.38 WHEN 'b3' THEN 0.51 ELSE tide_height_meters END,
			tide_phase = CASE id
				WHEN 'b1' THEN 'Rising' WHEN 'b2' THEN 'Rising' WHEN 'b3' THEN 'Falling' ELSE tide_phase END,
			description = CASE id
				WHEN 'b1' THEN CASE WHEN description = 'Top surf spot with solid reef break'
					THEN 'Exposed reef break with a fast takeoff; best on a clean west swell.' ELSE description END
				WHEN 'b2' THEN CASE WHEN description = 'Protected bay, good for longboarding'
					THEN 'Sheltered sandy bay with a mellow longboard peak and easy beach access.' ELSE description END
				WHEN 'b3' THEN CASE WHEN description = 'Popular surf beach in northern Israel'
					THEN 'Open northern coast beach that handles longer-period winter swell.' ELSE description END
				ELSE description
			END
		WHERE id IN ('b1', 'b2', 'b3')
		  AND (wind_speed = 0 OR wave_period_seconds = 0 OR swell_direction_degrees = 0
		       OR water_temperature_celsius = 0 OR tide_height_meters = 0 OR tide_phase = 'N/A')
	`)
	if err != nil {
		return fmt.Errorf("ensure beach condition details: %w", err)
	}
	return nil
}

// GetAll שולף את כל החופים הקיימים
func (r *PostgresBeachRepository) GetAll(ctx context.Context) ([]domain.Beach, error) {
	query := `
		SELECT id, name, latitude, longitude, COALESCE(wave_height, 0), COALESCE(wind_speed, 0),
		       COALESCE(wave_period_seconds, 0), COALESCE(swell_direction_degrees, 0),
		       COALESCE(water_temperature_celsius, 0), COALESCE(tide_height_meters, 0),
		       COALESCE(tide_phase, 'N/A'), COALESCE(description, ''), created_at, updated_at
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
			&b.WaveHeight, &b.WindSpeed, &b.WavePeriodSeconds, &b.SwellDirectionDegrees,
			&b.WaterTemperatureCelsius, &b.TideHeightMeters, &b.TidePhase, &b.Description,
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
		       COALESCE(wave_period_seconds, 0), COALESCE(swell_direction_degrees, 0),
		       COALESCE(water_temperature_celsius, 0), COALESCE(tide_height_meters, 0),
		       COALESCE(tide_phase, 'N/A'), COALESCE(description, ''), created_at, updated_at
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
			&b.WaveHeight, &b.WindSpeed, &b.WavePeriodSeconds, &b.SwellDirectionDegrees,
			&b.WaterTemperatureCelsius, &b.TideHeightMeters, &b.TidePhase, &b.Description,
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
		       COALESCE(wave_period_seconds, 0), COALESCE(swell_direction_degrees, 0),
		       COALESCE(water_temperature_celsius, 0), COALESCE(tide_height_meters, 0),
		       COALESCE(tide_phase, 'N/A'), COALESCE(description, ''), created_at, updated_at
		FROM beaches
		WHERE id = $1
	`
	var b domain.Beach
	err := r.db.QueryRow(ctx, query, id).Scan(
		&b.ID, &b.Name, &b.Latitude, &b.Longitude,
		&b.WaveHeight, &b.WindSpeed, &b.WavePeriodSeconds, &b.SwellDirectionDegrees,
		&b.WaterTemperatureCelsius, &b.TideHeightMeters, &b.TidePhase, &b.Description,
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
