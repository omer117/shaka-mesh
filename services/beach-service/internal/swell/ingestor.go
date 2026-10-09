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
	UpdateWaveHeight(ctx context.Context, id string, height float64) error
}

type AlertPublisher interface {
	PublishSwellAlert(ctx context.Context, event domain.SwellAlertEvent) error
}

type Ingestor struct {
	repository Repository
	provider   HeightProvider
	publisher  AlertPublisher
}

func NewIngestor(repository Repository, provider HeightProvider, publisher AlertPublisher) *Ingestor {
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
		height, err := i.provider.SwellHeight(ctx, beach.Latitude, beach.Longitude)
		if err != nil {
			refreshErrors = append(refreshErrors, fmt.Errorf("fetch swell for beach %s: %w", beach.ID, err))
			continue
		}
		if err := i.repository.UpdateWaveHeight(ctx, beach.ID, height); err != nil {
			refreshErrors = append(refreshErrors, fmt.Errorf("store swell for beach %s: %w", beach.ID, err))
			continue
		}
		if height > SwellAlertThresholdMeters {
			event := domain.SwellAlertEvent{
				BeachID:    beach.ID,
				BeachName:  beach.Name,
				WaveHeight: height,
				Timestamp:  time.Now().UTC(),
			}
			if err := i.publisher.PublishSwellAlert(ctx, event); err != nil {
				refreshErrors = append(refreshErrors, fmt.Errorf("publish swell alert for beach %s: %w", beach.ID, err))
			}
		}
	}
	return errors.Join(refreshErrors...)
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
