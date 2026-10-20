package authn_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/omer-asraf/shaka-api-gateway/internal/authn"
	"github.com/omer-asraf/shaka-api-gateway/internal/authn/authntest"
)

func validClaims(userID string) authn.Claims {
	return authn.Claims{
		UserID: userID,
		Email:  "surfer@example.com",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "test-issuer", Subject: userID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	}
}

func okHandler(m *authn.Middleware) http.Handler {
	return m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
}

func bearerRequest(token string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/beaches", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	return request
}

func TestMiddlewareAllowsPublicBeachListAuthAndJWKSRoutes(t *testing.T) {
	signer := authntest.NewSigner(t, "kid-1")
	middleware, err := authn.NewMiddleware(signer.Keys(), "test-issuer")
	if err != nil {
		t.Fatalf("create middleware: %v", err)
	}
	handler := okHandler(middleware)

	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/v1/beaches", nil),
		httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil),
		httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil),
		httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil),
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Errorf("%s %s: expected 204, got %d", request.Method, request.URL.Path, response.Code)
		}
	}
}

func TestMiddlewareRequiresTokenForBeachMutation(t *testing.T) {
	signer := authntest.NewSigner(t, "kid-1")
	middleware, _ := authn.NewMiddleware(signer.Keys(), "test-issuer")
	response := httptest.NewRecorder()
	okHandler(middleware).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/beaches", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

func TestMiddlewareRejectsExpiredWrongKeyUnknownKidAndWrongIssuer(t *testing.T) {
	signer := authntest.NewSigner(t, "kid-1")
	attacker := authntest.Generate(t, "kid-1") // same kid, different key
	stranger := authntest.Generate(t, "kid-unknown")
	middleware, _ := authn.NewMiddleware(signer.Keys(), "test-issuer")
	handler := okHandler(middleware)

	expired := validClaims("user-1")
	expired.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
	wrongIssuer := validClaims("user-1")
	wrongIssuer.Issuer = "someone-else"
	subjectMismatch := validClaims("user-1")
	subjectMismatch.Subject = "user-2"

	cases := map[string]string{
		"expired":          signer.Token(t, expired),
		"forged signature": attacker.Token(t, validClaims("user-1")),
		"unknown kid":      stranger.Token(t, validClaims("user-1")),
		"wrong issuer":     signer.Token(t, wrongIssuer),
		"subject mismatch": signer.Token(t, subjectMismatch),
		"garbage":          "not.a.jwt",
	}
	for name, token := range cases {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, bearerRequest(token))
		if response.Code != http.StatusUnauthorized {
			t.Errorf("%s: expected 401, got %d", name, response.Code)
		}
	}
}

func TestMiddlewareRejectsHMACAndNoneTokens(t *testing.T) {
	signer := authntest.NewSigner(t, "kid-1")
	middleware, _ := authn.NewMiddleware(signer.Keys(), "test-issuer")
	handler := okHandler(middleware)

	// Algorithm confusion: an HS256 token "signed" with public key material,
	// and an unsigned token, both naming a real kid.
	hmacToken := jwt.NewWithClaims(jwt.SigningMethodHS256, validClaims("user-1"))
	hmacToken.Header["kid"] = "kid-1"
	hmacSigned, _ := hmacToken.SignedString(signer.Key.N.Bytes())
	noneToken := jwt.NewWithClaims(jwt.SigningMethodNone, validClaims("user-1"))
	noneToken.Header["kid"] = "kid-1"
	noneSigned, _ := noneToken.SignedString(jwt.UnsafeAllowNoneSignatureType)

	// A validly signed RS256 token without a kid header must also be refused.
	noKid := jwt.NewWithClaims(jwt.SigningMethodRS256, validClaims("user-1"))
	noKidSigned, _ := noKid.SignedString(signer.Key)

	for name, token := range map[string]string{"hs256": hmacSigned, "none": noneSigned, "no kid": noKidSigned} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, bearerRequest(token))
		if response.Code != http.StatusUnauthorized {
			t.Errorf("%s: expected 401, got %d", name, response.Code)
		}
	}
}

func TestMiddlewareAcceptsValidRS256TokenAndAttachesClaims(t *testing.T) {
	signer := authntest.NewSigner(t, "kid-1")
	middleware, _ := authn.NewMiddleware(signer.Keys(), "test-issuer")

	var got *authn.Claims
	handler := middleware.Wrap(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, _ = authn.UserClaimsFromContext(r.Context())
	}))
	handler.ServeHTTP(httptest.NewRecorder(), bearerRequest(signer.Token(t, validClaims("user-1"))))
	if got == nil || got.UserID != "user-1" || got.Email != "surfer@example.com" {
		t.Fatalf("expected user claims in context, got %+v", got)
	}
	if _, ok := authn.UserClaimsFromContext(context.Background()); ok {
		t.Fatal("unexpected claims in empty context")
	}
}

func TestNewMiddlewareValidatesArguments(t *testing.T) {
	signer := authntest.NewSigner(t, "kid-1")
	if _, err := authn.NewMiddleware(nil, "test-issuer"); err == nil {
		t.Error("nil key source accepted")
	}
	if _, err := authn.NewMiddleware(signer.Keys(), " "); err == nil {
		t.Error("blank issuer accepted")
	}
}
