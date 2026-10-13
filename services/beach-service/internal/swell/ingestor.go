package swell

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/omer-asraf/shaka-beach-service/internal/domain"
)

const SwellAlertThresholdMeters = 1.5

type Repository interface {
	GetAll(ctx context.Context) ([]domain.Beach, error)
	GetByID(ctx context.Context, id string) (*domain.Beach, error)
	UpdateConditions(ctx context.Context, id string, waveHeight, windSpeed float64) error
}

type AlertPublisher interface {
	PublishSwellAlert(ctx context.Context, event domain.SwellAlertEvent) error
}

type Ingestor struct {
	repository Repository
	provider   ConditionsProvider
	publisher  AlertPublisher
}

func NewIngestor(repository Repository, provider ConditionsProvider, publisher AlertPublisher) *Ingestor {
	return &Ingestor{repository: repository, provider: provider, publisher: publisher}
}

func (i *Ingestor) Refresh(ctx context.Context) error {
	beaches, err := i.repository.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("list beaches for swell refresh: %w", err)
	}

	var refreshErrors []error
	for _, beach := range beaches {
		if err := ctx.Err(); err != nil {
			refreshErrors = append(refreshErrors, err)
			break
		}
		if _, err := i.RefreshBeach(ctx, beach.ID); err != nil {
			refreshErrors = append(refreshErrors, err)
		}
	}
	return errors.Join(refreshErrors...)
}

func (i *Ingestor) RefreshBeach(ctx context.Context, id string) (*domain.Beach, error) {
	beach, err := i.repository.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load beach %s for conditions refresh: %w", id, err)
	}
	conditions, err := i.provider.CurrentConditions(ctx, beach.Latitude, beach.Longitude)
	if err != nil {
		return nil, fmt.Errorf("fetch live conditions for beach %s: %w", id, err)
	}
	if err := i.repository.UpdateConditions(ctx, id, conditions.WaveHeight, conditions.WindSpeed); err != nil {
		return nil, fmt.Errorf("store live conditions for beach %s: %w", id, err)
	}
	beach.WaveHeight = conditions.WaveHeight
	beach.WindSpeed = conditions.WindSpeed
	beach.UpdatedAt = time.Now().UTC()

	if conditions.WaveHeight > SwellAlertThresholdMeters {
		event := domain.SwellAlertEvent{
			BeachID:    beach.ID,
			BeachName:  beach.Name,
			WaveHeight: conditions.WaveHeight,
			Timestamp:  time.Now().UTC(),
		}
		if err := i.publisher.PublishSwellAlert(ctx, event); err != nil {
			return nil, fmt.Errorf("publish swell alert for beach %s: %w", beach.ID, err)
		}
	}
	return beach, nil
}

func (i *Ingestor) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return errors.New("swell refresh interval must be positive")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	if err := i.Refresh(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("swell refresh failed: %v", err)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := i.Refresh(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("swell refresh failed: %v", err)
			}
		}
	}
}
