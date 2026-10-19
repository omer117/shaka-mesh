//go:build integration

package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/omer-asraf/shaka-beach-service/internal/domain"
	"github.com/omer-asraf/shaka-beach-service/internal/repository"
)

func resetBeaches(t *testing.T) *repository.PostgresBeachRepository {
	t.Helper()
	if _, err := testPool.Exec(context.Background(), `DELETE FROM beaches`); err != nil {
		t.Fatalf("reset beaches: %v", err)
	}
	return repository.NewPostgresBeachRepository(testPool)
}

func TestFindNearbyFiltersByRadiusAndOrdersByDistance(t *testing.T) {
	ctx := context.Background()
	repo := resetBeaches(t)

	// Query point is Gordon beach, Tel Aviv. Distances are approximate.
	beaches := []domain.Beach{
		{ID: "far-haifa", Name: "Haifa (~80 km)", Latitude: 32.8122, Longitude: 34.9580},
		{ID: "mid-herzliya", Name: "Herzliya (~9 km)", Latitude: 32.1650, Longitude: 34.7950},
		{ID: "near-frishman", Name: "Frishman (~0.6 km)", Latitude: 32.0750, Longitude: 34.7670},
		{ID: "nearer-gordon", Name: "Gordon (0 km)", Latitude: 32.0800, Longitude: 34.7670},
	}
	for i := range beaches {
		if err := repo.Create(ctx, &beaches[i]); err != nil {
			t.Fatalf("create %s: %v", beaches[i].ID, err)
		}
	}

	t.Run("20km radius returns only close beaches, nearest first", func(t *testing.T) {
		got, err := repo.FindNearby(ctx, 32.0800, 34.7670, 20)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"nearer-gordon", "near-frishman", "mid-herzliya"}
		if len(got) != len(want) {
			t.Fatalf("got %d beaches, want %d: %+v", len(got), len(want), got)
		}
		for i, id := range want {
			if got[i].ID != id {
				t.Errorf("position %d: got %s, want %s", i, got[i].ID, id)
			}
		}
	})

	t.Run("1km radius excludes mid-range beaches", func(t *testing.T) {
		got, err := repo.FindNearby(ctx, 32.0800, 34.7670, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].ID != "nearer-gordon" || got[1].ID != "near-frishman" {
			t.Fatalf("unexpected result: %+v", got)
		}
	})

	t.Run("large radius includes everything, farthest last", func(t *testing.T) {
		got, err := repo.FindNearby(ctx, 32.0800, 34.7670, 200)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 4 || got[3].ID != "far-haifa" {
			t.Fatalf("unexpected result: %+v", got)
		}
	})

	t.Run("nothing in range returns empty", func(t *testing.T) {
		got, err := repo.FindNearby(ctx, 0, 0, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Fatalf("expected no beaches, got %+v", got)
		}
	})
}

func TestCreateGetAndUpdateConditions(t *testing.T) {
	ctx := context.Background()
	repo := resetBeaches(t)

	b := domain.Beach{ID: "t1", Name: "Test Beach", Latitude: 32.0, Longitude: 34.7, Description: "d"}
	if err := repo.Create(ctx, &b); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateConditions(ctx, "t1", 1.8, 12.5); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByID(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	if got.WaveHeight != 1.8 || got.WindSpeed != 12.5 {
		t.Fatalf("conditions not stored: %+v", got)
	}

	if _, err := repo.GetByID(ctx, "missing"); !errors.Is(err, domain.ErrBeachNotFound) {
		t.Fatalf("expected ErrBeachNotFound, got %v", err)
	}
	if err := repo.UpdateWaveHeight(ctx, "missing", 1); !errors.Is(err, domain.ErrBeachNotFound) {
		t.Fatalf("expected ErrBeachNotFound, got %v", err)
	}
}

func TestEnsureIsraelSurfBeachesIsIdempotent(t *testing.T) {
	ctx := context.Background()
	repo := resetBeaches(t)

	if err := repo.EnsureIsraelSurfBeaches(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := repo.GetAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 {
		t.Fatal("expected seeded beaches")
	}
	if err := repo.EnsureIsraelSurfBeaches(ctx); err != nil {
		t.Fatal(err)
	}
	second, err := repo.GetAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != len(second) {
		t.Fatalf("seed not idempotent: %d then %d rows", len(first), len(second))
	}
	// Seeded beaches must be reachable through the spatial query.
	near, err := repo.FindNearby(ctx, 32.0800, 34.7670, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(near) == 0 || near[0].ID != "il-tel-aviv-gordon" {
		t.Fatalf("expected Gordon beach first, got %+v", near)
	}
}
