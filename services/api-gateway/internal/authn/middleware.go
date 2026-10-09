package authn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

type Middleware struct {
	secret []byte
	issuer string
}

func NewMiddleware(secret, issuer string) (*Middleware, error) {
	if len(secret) < 32 {
		return nil, errors.New("JWT_SECRET must be at least 32 bytes")
	}
	if strings.TrimSpace(issuer) == "" {
		return nil, errors.New("JWT_ISSUER must not be empty")
	}
	return &Middleware{secret: []byte(secret), issuer: issuer}, nil
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
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected JWT signing method %q", token.Method.Alg())
		}
		return m.secret, nil
	}, jwt.WithIssuer(m.issuer), jwt.WithExpirationRequired())
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
	if r.Method == http.MethodGet &&
		(r.URL.Path == "/api/v1/beaches" || strings.HasPrefix(r.URL.Path, "/api/v1/beaches/")) {
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
