//go:build integration

package repository_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/omer-asraf/shaka-auth-service/internal/domain"
	"github.com/omer-asraf/shaka-auth-service/internal/repository"
)

func newRepo(t *testing.T) *repository.PostgresUserRepository {
	t.Helper()
	if _, err := testPool.Exec(context.Background(), `DELETE FROM users`); err != nil {
		t.Fatalf("reset users: %v", err)
	}
	return repository.NewPostgresUserRepository(testPool)
}

func TestCreateAndGetByEmail(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)

	u := &domain.User{ID: "u-1", Email: "kelly@example.com", PasswordHash: "hash"}
	if err := repo.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if u.CreatedAt.IsZero() {
		t.Error("CreatedAt should be filled from RETURNING")
	}
	got, err := repo.GetByEmail(ctx, "kelly@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "u-1" || got.PasswordHash != "hash" {
		t.Fatalf("unexpected user: %+v", got)
	}
}

func TestCreateDuplicateEmailReturnsErrEmailInUse(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)

	if err := repo.Create(ctx, &domain.User{ID: "u-1", Email: "dup@example.com", PasswordHash: "h"}); err != nil {
		t.Fatal(err)
	}
	err := repo.Create(ctx, &domain.User{ID: "u-2", Email: "dup@example.com", PasswordHash: "h"})
	if !errors.Is(err, repository.ErrEmailInUse) {
		t.Fatalf("expected ErrEmailInUse, got %v", err)
	}
}

func TestCreateDuplicateIDIsNotReportedAsEmailConflict(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)

	if err := repo.Create(ctx, &domain.User{ID: "same", Email: "a@example.com", PasswordHash: "h"}); err != nil {
		t.Fatal(err)
	}
	err := repo.Create(ctx, &domain.User{ID: "same", Email: "b@example.com", PasswordHash: "h"})
	if err == nil || errors.Is(err, repository.ErrEmailInUse) {
		t.Fatalf("expected a generic error for primary-key clash, got %v", err)
	}
}

func TestGetByEmailUnknownReturnsErrUserNotFound(t *testing.T) {
	repo := newRepo(t)
	if _, err := repo.GetByEmail(context.Background(), "nobody@example.com"); !errors.Is(err, repository.ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
}

func TestConcurrentRegistrationsWithSameEmailYieldExactlyOneWinner(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)

	const n = 8
	results := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			results <- repo.Create(ctx, &domain.User{ID: fmt.Sprintf("race-%d", i), Email: "race@example.com", PasswordHash: "h"})
		}(i)
	}
	var ok, conflicts int
	for i := 0; i < n; i++ {
		switch err := <-results; {
		case err == nil:
			ok++
		case errors.Is(err, repository.ErrEmailInUse):
			conflicts++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != 1 || conflicts != n-1 {
		t.Fatalf("got %d successes and %d conflicts, want 1 and %d", ok, conflicts, n-1)
	}
}
