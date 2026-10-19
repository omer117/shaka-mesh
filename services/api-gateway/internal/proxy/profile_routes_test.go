package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/omer-asraf/shaka-api-gateway/internal/authn"
)

func TestProfileAndMediaRoutes(t *testing.T) {
	type seen struct{ path, method, userID string }
	got := make(chan seen, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- seen{r.URL.Path, r.Method, r.Header.Get("X-Authenticated-User-ID")}
		_, _ = io.WriteString(w, "ok")
	}))
	defer upstream.Close()
	other, _ := url.Parse("http://127.0.0.1:1")
	authURL, _ := url.Parse(upstream.URL)
	secret := "0123456789abcdef0123456789abcdef"
	mw, err := authn.NewMiddleware(secret, "test-issuer")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(authURL, other, other, mw, "http://localhost:3000")
	if err != nil {
		t.Fatal(err)
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, authn.Claims{
		UserID: "user-7", Email: "a@b.c",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "test-issuer", Subject: "user-7",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	}).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		method     string
		path       string
		authed     bool
		wantStatus int
		wantPath   string // "" means upstream must not be reached
		wantUser   string
	}{
		{"get me", "GET", "/api/v1/me", true, 200, "/me", "user-7"},
		{"patch me", "PATCH", "/api/v1/me", true, 200, "/me", "user-7"},
		{"upload avatar", "POST", "/api/v1/me/avatar", true, 200, "/me/avatar", "user-7"},
		{"get me without token", "GET", "/api/v1/me", false, 401, "", ""},
		{"patch me without token", "PATCH", "/api/v1/me", false, 401, "", ""},
		{"upload without token", "POST", "/api/v1/me/avatar", false, 401, "", ""},
		{"media is public and strips forged identity", "GET", "/media/avatars/abc.jpg", false, 200, "/media/avatars/abc.jpg", ""},
		{"media write is not routed", "POST", "/media/avatars/abc.jpg", true, 404, "", ""},
		{"other media paths are not routed", "GET", "/media/other/abc.jpg", false, 404, "", ""},
		{"no per-user profile url", "PATCH", "/api/v1/me/someone-else", true, 404, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.Header.Set("X-Authenticated-User-ID", "forged")
			if tt.authed {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status %d, want %d", rec.Code, tt.wantStatus)
			}
			select {
			case s := <-got:
				if tt.wantPath == "" {
					t.Fatalf("upstream reached unexpectedly: %+v", s)
				}
				if s.path != tt.wantPath || s.userID != tt.wantUser {
					t.Fatalf("upstream saw %+v, want path %q user %q", s, tt.wantPath, tt.wantUser)
				}
			default:
				if tt.wantPath != "" {
					t.Fatal("upstream not reached")
				}
			}
		})
	}
}

func TestCORSAllowsPatch(t *testing.T) {
	u, _ := url.Parse("http://localhost:1")
	mw, _ := authn.NewMiddleware("0123456789abcdef0123456789abcdef", "test-issuer")
	handler, _ := NewHandler(u, u, u, mw, "http://localhost:3000")
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/me", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "PATCH")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	methods := rec.Header().Get("Access-Control-Allow-Methods")
	if rec.Code != http.StatusNoContent || !strings.Contains(methods, "PATCH") {
		t.Fatalf("code=%d methods=%q", rec.Code, methods)
	}
}
