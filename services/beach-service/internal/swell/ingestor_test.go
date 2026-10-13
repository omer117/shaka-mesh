package swell

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/omer-asraf/shaka-beach-service/internal/domain"
)

type stubRepository struct {
	beaches    []domain.Beach
	conditions map[string]Conditions
	err        error
}

func (r *stubRepository) GetAll(context.Context) ([]domain.Beach, error) {
	return r.beaches, r.err
}

func (r *stubRepository) GetByID(_ context.Context, id string) (*domain.Beach, error) {
	if r.err != nil {
		return nil, r.err
	}
	for index := range r.beaches {
		if r.beaches[index].ID == id {
			beach := r.beaches[index]
			return &beach, nil
		}
	}
	return nil, domain.ErrBeachNotFound
}

func (r *stubRepository) UpdateConditions(_ context.Context, id string, waveHeight, windSpeed float64) error {
	if r.err != nil {
		return r.err
	}
	if r.conditions == nil {
		r.conditions = make(map[string]Conditions)
	}
	r.conditions[id] = Conditions{WaveHeight: waveHeight, WindSpeed: windSpeed}
	return nil
}

type stubConditionsProvider struct {
	conditions Conditions
	err        error
}

func (p stubConditionsProvider) CurrentConditions(context.Context, float64, float64) (Conditions, error) {
	return p.conditions, p.err
}

type stubAlertPublisher struct {
	events []domain.SwellAlertEvent
	err    error
}

func (p *stubAlertPublisher) PublishSwellAlert(_ context.Context, event domain.SwellAlertEvent) error {
	p.events = append(p.events, event)
	return p.err
}

func TestIngestorRefreshUpdatesBeachSwell(t *testing.T) {
	repository := &stubRepository{
		beaches: []domain.Beach{{ID: "b1", Latitude: 32.08, Longitude: 34.77}},
	}
	ingestor := NewIngestor(repository, stubConditionsProvider{conditions: Conditions{WaveHeight: 1.25, WindSpeed: 7.5}}, &stubAlertPublisher{})
	if err := ingestor.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh swell: %v", err)
	}
	if got := repository.conditions["b1"]; got != (Conditions{WaveHeight: 1.25, WindSpeed: 7.5}) {
		t.Fatalf("expected stored live conditions, got %+v", got)
	}
}

func TestIngestorRefreshReportsProviderFailure(t *testing.T) {
	ingestor := NewIngestor(
		&stubRepository{beaches: []domain.Beach{{ID: "b1"}}},
		stubConditionsProvider{err: errors.New("API unavailable")},
		&stubAlertPublisher{},
	)
	if err := ingestor.Refresh(context.Background()); err == nil {
		t.Fatal("expected provider failure to be returned")
	}
}

func TestIngestorRunRequiresPositiveInterval(t *testing.T) {
	ingestor := NewIngestor(&stubRepository{}, stubConditionsProvider{}, &stubAlertPublisher{})
	if err := ingestor.Run(context.Background(), 0); err == nil {
		t.Fatal("expected a non-positive interval to be rejected")
	}
	if err := ingestor.Run(context.Background(), -time.Second); err == nil {
		t.Fatal("expected a negative interval to be rejected")
	}
}

func TestIngestorPublishesAlertsAboveThreshold(t *testing.T) {
	repository := &stubRepository{
		beaches: []domain.Beach{{ID: "b1", Name: "Hilton Beach", Latitude: 32.08, Longitude: 34.77}},
	}
	publisher := &stubAlertPublisher{}
	ingestor := NewIngestor(repository, stubConditionsProvider{conditions: Conditions{WaveHeight: 1.6, WindSpeed: 11.2}}, publisher)
	if err := ingestor.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh swell: %v", err)
	}
	if len(publisher.events) != 1 {
		t.Fatalf("expected one swell alert, got %d", len(publisher.events))
	}
	event := publisher.events[0]
	if event.BeachID != "b1" || event.BeachName != "Hilton Beach" || event.WaveHeight != 1.6 {
		t.Fatalf("unexpected swell alert: %+v", event)
	}
	if event.Timestamp.IsZero() || event.Timestamp.Location() != time.UTC {
		t.Fatalf("expected UTC timestamp, got %v", event.Timestamp)
	}
}

func TestIngestorDoesNotPublishAtOrBelowThreshold(t *testing.T) {
	for _, height := range []float64{1.5, 1.2} {
		publisher := &stubAlertPublisher{}
		ingestor := NewIngestor(
			&stubRepository{beaches: []domain.Beach{{ID: "b1"}}},
			stubConditionsProvider{conditions: Conditions{WaveHeight: height, WindSpeed: 8}},
			publisher,
		)
		if err := ingestor.Refresh(context.Background()); err != nil {
			t.Fatalf("refresh swell at %v: %v", height, err)
		}
		if len(publisher.events) != 0 {
			t.Fatalf("expected no alert at wave height %v, got %d", height, len(publisher.events))
		}
	}
}

func TestIngestorDoesNotPublishWhenStoreFails(t *testing.T) {
	publisher := &stubAlertPublisher{}
	ingestor := NewIngestor(
		&stubRepository{
			beaches: []domain.Beach{{ID: "b1"}},
			err:     errors.New("database unavailable"),
		},
		stubConditionsProvider{conditions: Conditions{WaveHeight: 2, WindSpeed: 8}},
		publisher,
	)
	if err := ingestor.Refresh(context.Background()); err == nil {
		t.Fatal("expected database failure")
	}
	if len(publisher.events) != 0 {
		t.Fatalf("expected no alert after store failure, got %d", len(publisher.events))
	}
}

func TestIngestorReturnsPublisherFailure(t *testing.T) {
	ingestor := NewIngestor(
		&stubRepository{beaches: []domain.Beach{{ID: "b1"}}},
		stubConditionsProvider{conditions: Conditions{WaveHeight: 2, WindSpeed: 8}},
		&stubAlertPublisher{err: errors.New("RabbitMQ unavailable")},
	)
	if err := ingestor.Refresh(context.Background()); err == nil {
		t.Fatal("expected publisher failure to be returned")
	}
}
