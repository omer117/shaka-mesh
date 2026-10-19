package authn

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

type contextKey struct{}

// jwksPath is where the gateway republishes auth-service's public keys so
// other services (and the outside world) can verify tokens too.
const jwksPath = "/.well-known/jwks.json"

// Middleware verifies RS256 access tokens with auth-service's public keys. The
// gateway never holds a signing secret: it cannot mint tokens, and a leak of
// the gateway reveals nothing that lets an attacker forge one.
type Middleware struct {
	keys   KeySource
	issuer string
}

func NewMiddleware(keys KeySource, issuer string) (*Middleware, error) {
	if keys == nil {
		return nil, errors.New("JWT key source is required")
	}
	if strings.TrimSpace(issuer) == "" {
		return nil, errors.New("JWT_ISSUER must not be empty")
	}
	return &Middleware{keys: keys, issuer: issuer}, nil
}

func (m *Middleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublic(r) {
			next.ServeHTTP(w, r)
			return
		}

		authorization := r.Header.Get("Authorization")
		scheme, rawToken, found := strings.Cut(authorization, " ")
		if !found || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(rawToken) == "" {
			writeUnauthorized(w)
			return
		}
		claims, err := m.parse(strings.TrimSpace(rawToken))
		if err != nil {
			log.Printf("reject invalid bearer token: %v", err)
			writeUnauthorized(w)
			return
		}
		ctx := context.WithValue(r.Context(), contextKey{}, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *Middleware) parse(rawToken string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(rawToken, claims, func(token *jwt.Token) (any, error) {
		kid, _ := token.Header["kid"].(string)
		return m.keys.Key(kid)
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer(m.issuer), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	if !token.Valid || claims.UserID == "" || claims.Subject != claims.UserID {
		return nil, errors.New("invalid user claims")
	}
	return claims, nil
}

func UserClaimsFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(contextKey{}).(*Claims)
	return claims, ok
}

func isPublic(r *http.Request) bool {
	if r.URL.Path == "/health" && r.Method == http.MethodGet {
		return true
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/auth/") {
		return true
	}
	if r.URL.Path == jwksPath && r.Method == http.MethodGet {
		return true
	}
	// Avatars are plain <img> loads, which cannot send an Authorization header.
	if (r.Method == http.MethodGet || r.Method == http.MethodHead) &&
		strings.HasPrefix(r.URL.Path, "/media/avatars/") {
		return true
	}
	if r.Method == http.MethodGet &&
		(r.URL.Path == "/api/v1/beaches" || strings.HasPrefix(r.URL.Path, "/api/v1/beaches/")) {
		return true
	}
	if r.Method == http.MethodGet &&
		(r.URL.Path == "/api/v1/shop/products" ||
			r.URL.Path == "/api/v1/forum/categories" ||
			r.URL.Path == "/api/v1/forum/threads" ||
			strings.HasPrefix(r.URL.Path, "/api/v1/forum/threads/")) {
		return true
	}
	return false
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	if err := json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{Error: "authentication required"}); err != nil {
		log.Printf("write unauthorized response: %v", err)
	}
}
