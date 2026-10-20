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
	user       *domain.User
	session    *service.Session
	err        error
	refreshErr error
	logoutErr  error
	gotRefresh *string
	gotLogout  *string
}

func (f fakeAuth) Register(context.Context, string, string) (*domain.User, error) {
	return f.user, f.err
}

func (f fakeAuth) Login(context.Context, string, string) (*service.Session, error) {
	return f.session, f.err
}

func (f fakeAuth) Refresh(_ context.Context, token string) (*service.Session, error) {
	if f.gotRefresh != nil {
		*f.gotRefresh = token
	}
	return f.session, f.refreshErr
}

func (f fakeAuth) Logout(_ context.Context, token string) error {
	if f.gotLogout != nil {
		*f.gotLogout = token
	}
	return f.logoutErr
}

func testSession() *service.Session {
	return &service.Session{
		AccessToken:      "signed-token",
		ExpiresAt:        time.Now().UTC().Add(15 * time.Minute),
		RefreshToken:     "raw-refresh-token",
		RefreshExpiresAt: time.Now().UTC().Add(24 * time.Hour),
		Email:            "surfer@example.com",
	}
}

func newHandler(auth fakeAuth) http.Handler {
	return New(auth, DefaultCookieConfig())
}

func TestRegisterReturnsCreatedUserWithoutPasswordHash(t *testing.T) {
	handler := newHandler(fakeAuth{user: &domain.User{
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

func TestRegisterReturnsEveryPasswordPolicyProblem(t *testing.T) {
	handler := newHandler(fakeAuth{err: &service.PasswordPolicyError{
		Problems: []string{"must be at least 10 characters long", "is too common; choose something less guessable"},
	}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/auth/register",
		strings.NewReader(`{"email":"surfer@example.com","password":"password"}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", response.Code)
	}
	var body struct {
		Error    string   `json:"error"`
		Problems []string `json:"problems"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Problems) != 2 || !strings.Contains(body.Error, "password") {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func TestLoginDoesNotApplyTheRegistrationPasswordPolicy(t *testing.T) {
	handler := newHandler(fakeAuth{session: testSession()})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/auth/login",
		strings.NewReader(`{"email":"legacy@example.com","password":"short8pw"}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("legacy short passwords must still reach the service, got %d", response.Code)
	}
}

func TestRegisterRejectsDuplicateEmail(t *testing.T) {
	handler := newHandler(fakeAuth{err: repository.ErrEmailInUse})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/auth/register",
		strings.NewReader(`{"email":"surfer@example.com","password":"correct horse battery"}`)))
	if response.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", response.Code)
	}
}

func TestLoginReturnsBearerToken(t *testing.T) {
	handler := newHandler(fakeAuth{session: testSession()})
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
	if strings.Contains(response.Body.String(), "raw-refresh-token") {
		t.Fatal("refresh token must only travel in the cookie, not the body")
	}
	cookie := findCookie(response, "shaka_refresh")
	if cookie == nil {
		t.Fatal("expected refresh cookie")
	}
	if cookie.Value != "raw-refresh-token" || !cookie.HttpOnly || !cookie.Secure ||
		cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/api/v1/auth" || cookie.MaxAge <= 0 {
		t.Fatalf("unexpected refresh cookie attributes: %+v", cookie)
	}
}

func findCookie(response *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func refreshRequest(method, path, token string) *http.Request {
	request := httptest.NewRequest(method, path, nil)
	if token != "" {
		request.AddCookie(&http.Cookie{Name: "shaka_refresh", Value: token})
	}
	return request
}

func TestRefreshRotatesCookieAndReturnsAccessToken(t *testing.T) {
	var got string
	handler := newHandler(fakeAuth{session: testSession(), gotRefresh: &got})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, refreshRequest(http.MethodPost, "/auth/refresh", "old-token"))
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if got != "old-token" {
		t.Fatalf("service received %q, want the cookie value", got)
	}
	cookie := findCookie(response, "shaka_refresh")
	if cookie == nil || cookie.Value != "raw-refresh-token" || !cookie.HttpOnly {
		t.Fatalf("expected a rotated HttpOnly cookie, got %+v", cookie)
	}
	if !strings.Contains(response.Body.String(), `"email":"surfer@example.com"`) {
		t.Fatalf("expected email in response: %s", response.Body.String())
	}
}

func TestRefreshWithoutCookieIsUnauthorized(t *testing.T) {
	response := httptest.NewRecorder()
	newHandler(fakeAuth{}).ServeHTTP(response, refreshRequest(http.MethodPost, "/auth/refresh", ""))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

func TestRefreshWithRejectedTokenClearsCookie(t *testing.T) {
	response := httptest.NewRecorder()
	newHandler(fakeAuth{refreshErr: service.ErrInvalidRefreshToken}).
		ServeHTTP(response, refreshRequest(http.MethodPost, "/auth/refresh", "stale"))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
	cookie := findCookie(response, "shaka_refresh")
	if cookie == nil || cookie.MaxAge >= 0 || cookie.Value != "" {
		t.Fatalf("expected the cookie to be cleared, got %+v", cookie)
	}
}

func TestRefreshUnexpectedFailureIsInternalError(t *testing.T) {
	response := httptest.NewRecorder()
	newHandler(fakeAuth{refreshErr: errors.New("db down")}).
		ServeHTTP(response, refreshRequest(http.MethodPost, "/auth/refresh", "tok"))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", response.Code)
	}
}

func TestLogoutRevokesAndClearsCookie(t *testing.T) {
	var got string
	response := httptest.NewRecorder()
	newHandler(fakeAuth{gotLogout: &got}).
		ServeHTTP(response, refreshRequest(http.MethodPost, "/auth/logout", "tok"))
	if response.Code != http.StatusNoContent || got != "tok" {
		t.Fatalf("expected 204 and revoked token, got %d / %q", response.Code, got)
	}
	cookie := findCookie(response, "shaka_refresh")
	if cookie == nil || cookie.MaxAge >= 0 {
		t.Fatalf("expected cookie to be cleared, got %+v", cookie)
	}
}

func TestLogoutWithoutCookieStillSucceeds(t *testing.T) {
	response := httptest.NewRecorder()
	newHandler(fakeAuth{}).ServeHTTP(response, refreshRequest(http.MethodPost, "/auth/logout", ""))
	if response.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", response.Code)
	}
}

func TestCookieConfigIsHonoured(t *testing.T) {
	handler := New(fakeAuth{session: testSession()}, CookieConfig{
		Name: "shaka_refresh", Path: "/auth", Secure: false, SameSite: http.SameSiteStrictMode,
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/auth/login",
		strings.NewReader(`{"email":"surfer@example.com","password":"correct horse battery"}`)))
	cookie := findCookie(response, "shaka_refresh")
	if cookie == nil || cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/auth" {
		t.Fatalf("unexpected cookie: %+v", cookie)
	}
}

func TestLoginHidesCredentialFailure(t *testing.T) {
	handler := newHandler(fakeAuth{err: service.ErrInvalidCredentials})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/auth/login",
		strings.NewReader(`{"email":"surfer@example.com","password":"correct horse battery"}`)))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

func TestRegisterRejectsUnknownJSONFields(t *testing.T) {
	handler := newHandler(fakeAuth{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/auth/register",
		strings.NewReader(`{"email":"surfer@example.com","password":"correct horse battery","is_admin":true}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", response.Code)
	}
}

func TestLoginReturnsInternalErrorForUnexpectedFailure(t *testing.T) {
	handler := newHandler(fakeAuth{err: errors.New("database unavailable")})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/auth/login",
		strings.NewReader(`{"email":"surfer@example.com","password":"correct horse battery"}`)))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", response.Code)
	}
}
