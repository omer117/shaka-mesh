package authn

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "0123456789abcdef0123456789abcdef"

func TestMiddlewareAllowsPublicBeachListAndAuthRoutes(t *testing.T) {
	middleware, err := NewMiddleware(testSecret, "test-issuer")
	if err != nil {
		t.Fatalf("create middleware: %v", err)
	}
	handler := middleware.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/v1/beaches", nil),
		httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil),
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Errorf("%s %s: expected 204, got %d", request.Method, request.URL.Path, response.Code)
		}
	}
}

func TestMiddlewareRequiresTokenForBeachMutation(t *testing.T) {
	middleware, err := NewMiddleware(testSecret, "test-issuer")
	if err != nil {
		t.Fatalf("create middleware: %v", err)
	}
	handler := middleware.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/beaches", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

func TestMiddlewareRejectsExpiredAndWrongSignatureTokens(t *testing.T) {
	middleware, err := NewMiddleware(testSecret, "test-issuer")
	if err != nil {
		t.Fatalf("create middleware: %v", err)
	}
	handler := middleware.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	now := time.Now()
	for _, claims := range []*Claims{
		{
			UserID: "user-1",
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer: "test-issuer", Subject: "user-1",
				ExpiresAt: jwt.NewNumericDate(now.Add(-time.Minute)),
			},
		},
		{
			UserID: "user-1",
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer: "test-issuer", Subject: "user-1",
				ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
			},
		},
	} {
		key := []byte(testSecret)
		if claims.ExpiresAt.After(now) {
			key = []byte("a-different-signing-secret-with-enough-bytes")
		}
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(key)
		if err != nil {
			t.Fatalf("sign token: %v", err)
		}
		request := httptest.NewRequest(http.MethodPost, "/api/v1/beaches", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Errorf("expected invalid token to return 401, got %d", response.Code)
		}
	}
}

func TestMiddlewareAttachesClaimsToContext(t *testing.T) {
	middleware, err := NewMiddleware(testSecret, "test-issuer")
	if err != nil {
		t.Fatalf("create middleware: %v", err)
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		UserID: "user-1",
		Email:  "surfer@example.com",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "test-issuer", Subject: "user-1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	}).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	var got *Claims
	handler := middleware.Wrap(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, _ = UserClaimsFromContext(r.Context())
	}))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/beaches", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(httptest.NewRecorder(), request)
	if got == nil || got.UserID != "user-1" || got.Email != "surfer@example.com" {
		t.Fatalf("expected user claims in context, got %+v", got)
	}
	if _, ok := UserClaimsFromContext(context.Background()); ok {
		t.Fatal("unexpected claims in empty context")
	}
}
