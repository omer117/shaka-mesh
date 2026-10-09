package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/omer-asraf/shaka-auth-service/internal/domain"
	"github.com/omer-asraf/shaka-auth-service/internal/repository"
	"github.com/omer-asraf/shaka-auth-service/internal/service"
)

type fakeAuth struct {
	user      *domain.User
	token     string
	expiresAt time.Time
	err       error
}

func (f fakeAuth) Register(context.Context, string, string) (*domain.User, error) {
	return f.user, f.err
}

func (f fakeAuth) Login(context.Context, string, string) (string, time.Time, error) {
	return f.token, f.expiresAt, f.err
}

func TestRegisterReturnsCreatedUserWithoutPasswordHash(t *testing.T) {
	handler := New(fakeAuth{user: &domain.User{
		ID: "user-1", Email: "surfer@example.com", PasswordHash: "never-return-hash",
	}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/auth/register",
		strings.NewReader(`{"email":"surfer@example.com","password":"correct horse battery"}`)))

	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "never-return-hash") {
		t.Fatal("response leaked password hash")
	}
}

func TestRegisterRejectsDuplicateEmail(t *testing.T) {
	handler := New(fakeAuth{err: repository.ErrEmailInUse})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/auth/register",
		strings.NewReader(`{"email":"surfer@example.com","password":"correct horse battery"}`)))
	if response.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", response.Code)
	}
}

func TestLoginReturnsBearerToken(t *testing.T) {
	expiresAt := time.Now().UTC().Add(time.Minute)
	handler := New(fakeAuth{token: "signed-token", expiresAt: expiresAt})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/auth/login",
		strings.NewReader(`{"email":"surfer@example.com","password":"correct horse battery"}`)))

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	var body struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if body.AccessToken != "signed-token" || body.TokenType != "Bearer" {
		t.Fatalf("unexpected login response: %+v", body)
	}
}

func TestLoginHidesCredentialFailure(t *testing.T) {
	handler := New(fakeAuth{err: service.ErrInvalidCredentials})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/auth/login",
		strings.NewReader(`{"email":"surfer@example.com","password":"correct horse battery"}`)))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

func TestRegisterRejectsUnknownJSONFields(t *testing.T) {
	handler := New(fakeAuth{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/auth/register",
		strings.NewReader(`{"email":"surfer@example.com","password":"correct horse battery","is_admin":true}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", response.Code)
	}
}

func TestLoginReturnsInternalErrorForUnexpectedFailure(t *testing.T) {
	handler := New(fakeAuth{err: errors.New("database unavailable")})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/auth/login",
		strings.NewReader(`{"email":"surfer@example.com","password":"correct horse battery"}`)))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", response.Code)
	}
}
