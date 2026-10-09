package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/omer-asraf/shaka-auth-service/internal/domain"
	"github.com/omer-asraf/shaka-auth-service/internal/repository"
)

type memoryUsers struct {
	user *domain.User
	err  error
}

func (r *memoryUsers) Create(_ context.Context, user *domain.User) error {
	if r.err != nil {
		return r.err
	}
	if r.user != nil {
		return repository.ErrEmailInUse
	}
	copy := *user
	r.user = &copy
	return nil
}

func (r *memoryUsers) GetByEmail(_ context.Context, _ string) (*domain.User, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.user == nil {
		return nil, repository.ErrUserNotFound
	}
	copy := *r.user
	return &copy, nil
}

func TestRegisterAndLogin(t *testing.T) {
	users := &memoryUsers{}
	tokens, err := NewJWTManager("0123456789abcdef0123456789abcdef", "test-issuer", time.Minute)
	if err != nil {
		t.Fatalf("create JWT manager: %v", err)
	}
	auth := NewAuthService(users, tokens)
	user, err := auth.Register(context.Background(), "Surfer@Example.com", "correct horse battery")
	if err != nil {
		t.Fatalf("register user: %v", err)
	}
	if user.Email != "surfer@example.com" {
		t.Fatalf("expected normalized email, got %q", user.Email)
	}
	if user.PasswordHash == "" || user.PasswordHash == "correct horse battery" {
		t.Fatal("expected password to be stored as a hash")
	}

	rawToken, expiresAt, err := auth.Login(context.Background(), "SURFER@example.com", "correct horse battery")
	if err != nil {
		t.Fatalf("login user: %v", err)
	}
	claims, err := tokens.Parse(rawToken)
	if err != nil {
		t.Fatalf("parse issued token: %v", err)
	}
	if claims.UserID != user.ID || claims.Email != user.Email {
		t.Fatalf("unexpected JWT claims: %+v", claims)
	}
	if !expiresAt.Equal(claims.ExpiresAt.Time) {
		t.Fatalf("returned expiration does not match JWT claim")
	}
}

func TestRegisterRejectsInvalidEmailAndPassword(t *testing.T) {
	tokens, err := NewJWTManager("0123456789abcdef0123456789abcdef", "test-issuer", time.Minute)
	if err != nil {
		t.Fatalf("create JWT manager: %v", err)
	}
	auth := NewAuthService(&memoryUsers{}, tokens)
	cases := []struct {
		email    string
		password string
	}{
		{"not-an-email", "correct horse battery"},
		{"surfer@example.com", "short"},
		{"surfer@example.com", string(make([]byte, MaxPasswordBytes+1))},
	}
	for _, testCase := range cases {
		if _, err := auth.Register(context.Background(), testCase.email, testCase.password); err == nil {
			t.Errorf("Register(%q, password length %d) succeeded unexpectedly", testCase.email, len(testCase.password))
		}
	}
}

func TestLoginUsesGenericInvalidCredentialsError(t *testing.T) {
	tokens, err := NewJWTManager("0123456789abcdef0123456789abcdef", "test-issuer", time.Minute)
	if err != nil {
		t.Fatalf("create JWT manager: %v", err)
	}
	auth := NewAuthService(&memoryUsers{}, tokens)
	_, _, err = auth.Login(context.Background(), "missing@example.com", "correct horse battery")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected generic invalid credentials error, got %v", err)
	}
}

func TestJWTManagerRejectsWeakConfiguration(t *testing.T) {
	if _, err := NewJWTManager("short", "test", time.Minute); err == nil {
		t.Fatal("expected weak secret to be rejected")
	}
	if _, err := NewJWTManager("0123456789abcdef0123456789abcdef", "", time.Minute); err == nil {
		t.Fatal("expected empty issuer to be rejected")
	}
	if _, err := NewJWTManager("0123456789abcdef0123456789abcdef", "test", 0); err == nil {
		t.Fatal("expected non-positive TTL to be rejected")
	}
}
