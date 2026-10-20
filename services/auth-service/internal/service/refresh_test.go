package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/omer-asraf/shaka-auth-service/internal/domain"
	"github.com/omer-asraf/shaka-auth-service/internal/repository"
)

type memoryRefresh struct {
	mu     sync.Mutex
	tokens map[string]*domain.RefreshToken // by hash
}

func newMemoryRefresh() *memoryRefresh {
	return &memoryRefresh{tokens: map[string]*domain.RefreshToken{}}
}

func (r *memoryRefresh) Create(_ context.Context, token *domain.RefreshToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := *token
	r.tokens[token.TokenHash] = &copy
	return nil
}

func (r *memoryRefresh) Consume(_ context.Context, hash string) (*domain.RefreshToken, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	token, ok := r.tokens[hash]
	if !ok {
		return nil, false, repository.ErrRefreshTokenNotFound
	}
	wasActive := !token.Revoked
	token.Revoked = true
	copy := *token
	return &copy, wasActive, nil
}

func (r *memoryRefresh) RevokeFamily(_ context.Context, familyID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, token := range r.tokens {
		if token.FamilyID == familyID {
			token.Revoked = true
		}
	}
	return nil
}

func (r *memoryRefresh) activeInFamily(familyID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, token := range r.tokens {
		if token.FamilyID == familyID && !token.Revoked {
			count++
		}
	}
	return count
}

func loggedInSession(t *testing.T) (*AuthService, *memoryRefresh, *Session) {
	t.Helper()
	auth, _, refresh := newTestAuth(t, &memoryUsers{})
	if _, err := auth.Register(context.Background(), "surfer@example.com", "correct horse battery"); err != nil {
		t.Fatalf("register: %v", err)
	}
	session, err := auth.Login(context.Background(), "surfer@example.com", "correct horse battery")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	return auth, refresh, session
}

func TestLoginStoresOnlyTheHashOfTheRefreshToken(t *testing.T) {
	_, refresh, session := loggedInSession(t)
	if session.RefreshToken == "" {
		t.Fatal("expected a refresh token")
	}
	if _, ok := refresh.tokens[session.RefreshToken]; ok {
		t.Fatal("raw refresh token was stored")
	}
	stored, ok := refresh.tokens[HashRefreshToken(session.RefreshToken)]
	if !ok {
		t.Fatal("hashed refresh token was not stored")
	}
	if strings.Contains(stored.TokenHash, session.RefreshToken) || stored.UserID == "" || stored.FamilyID == "" {
		t.Fatalf("unexpected stored token: %+v", stored)
	}
	if !stored.ExpiresAt.Equal(session.RefreshExpiresAt) {
		t.Fatal("stored expiry does not match the session")
	}
}

func TestRefreshRotatesTokenWithinFamily(t *testing.T) {
	auth, refresh, first := loggedInSession(t)
	second, err := auth.Refresh(context.Background(), first.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if second.RefreshToken == first.RefreshToken || second.AccessToken == "" || second.Email != "surfer@example.com" {
		t.Fatalf("expected a rotated session, got %+v", second)
	}
	oldRecord := refresh.tokens[HashRefreshToken(first.RefreshToken)]
	newRecord := refresh.tokens[HashRefreshToken(second.RefreshToken)]
	if !oldRecord.Revoked || newRecord.Revoked {
		t.Fatal("old token must be revoked and the new one active")
	}
	if oldRecord.FamilyID != newRecord.FamilyID {
		t.Fatal("rotated token must stay in the same family")
	}
}

func TestReusedRefreshTokenRevokesWholeFamily(t *testing.T) {
	auth, refresh, first := loggedInSession(t)
	second, err := auth.Refresh(context.Background(), first.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	// An attacker (or a buggy client) replays the already-rotated token.
	if _, err := auth.Refresh(context.Background(), first.RefreshToken); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("expected reuse to be rejected, got %v", err)
	}
	family := refresh.tokens[HashRefreshToken(second.RefreshToken)].FamilyID
	if refresh.activeInFamily(family) != 0 {
		t.Fatal("reuse must revoke every token in the family")
	}
	if _, err := auth.Refresh(context.Background(), second.RefreshToken); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("the legitimate newest token must also be dead, got %v", err)
	}
}

func TestRefreshRejectsUnknownEmptyAndExpiredTokens(t *testing.T) {
	auth, _, session := loggedInSession(t)
	for name, token := range map[string]string{"empty": "", "unknown": "not-a-real-token"} {
		if _, err := auth.Refresh(context.Background(), token); !errors.Is(err, ErrInvalidRefreshToken) {
			t.Errorf("%s: expected invalid refresh token, got %v", name, err)
		}
	}
	auth.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if _, err := auth.Refresh(context.Background(), session.RefreshToken); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("expected expired token to be rejected, got %v", err)
	}
}

func TestLogoutRevokesFamilyAndIsIdempotent(t *testing.T) {
	auth, refresh, first := loggedInSession(t)
	second, err := auth.Refresh(context.Background(), first.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if err := auth.Logout(context.Background(), second.RefreshToken); err != nil {
		t.Fatalf("logout: %v", err)
	}
	family := refresh.tokens[HashRefreshToken(second.RefreshToken)].FamilyID
	if refresh.activeInFamily(family) != 0 {
		t.Fatal("logout must revoke the family")
	}
	if _, err := auth.Refresh(context.Background(), second.RefreshToken); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("refresh after logout must fail, got %v", err)
	}
	for _, token := range []string{second.RefreshToken, "unknown", ""} {
		if err := auth.Logout(context.Background(), token); err != nil {
			t.Errorf("logout(%q) should be idempotent, got %v", token, err)
		}
	}
}

func TestConcurrentRefreshHasExactlyOneWinner(t *testing.T) {
	auth, _, session := loggedInSession(t)
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := auth.Refresh(context.Background(), session.RefreshToken)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("expected exactly one successful refresh, got %d", wins)
	}
}
