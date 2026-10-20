package main

import (
	"net/http"
	"testing"
)

func TestCookieConfigFromEnv(t *testing.T) {
	cases := []struct {
		name       string
		secure     string
		sameSite   string
		path       string
		wantErr    bool
		wantSecure bool
		wantSame   http.SameSite
		wantPath   string
	}{
		{name: "defaults are secure lax", wantSecure: true, wantSame: http.SameSiteLaxMode, wantPath: "/api/v1/auth"},
		{name: "insecure for local http", secure: "false", wantSecure: false, wantSame: http.SameSiteLaxMode, wantPath: "/api/v1/auth"},
		{name: "strict", sameSite: "Strict", wantSecure: true, wantSame: http.SameSiteStrictMode, wantPath: "/api/v1/auth"},
		{name: "none with secure", sameSite: "none", wantSecure: true, wantSame: http.SameSiteNoneMode, wantPath: "/api/v1/auth"},
		{name: "none without secure is rejected", secure: "false", sameSite: "none", wantErr: true},
		{name: "bad bool", secure: "maybe", wantErr: true},
		{name: "bad samesite", sameSite: "sideways", wantErr: true},
		{name: "custom path", path: "/auth", wantSecure: true, wantSame: http.SameSiteLaxMode, wantPath: "/auth"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("REFRESH_COOKIE_SECURE", tc.secure)
			t.Setenv("REFRESH_COOKIE_SAMESITE", tc.sameSite)
			t.Setenv("REFRESH_COOKIE_PATH", tc.path)
			cfg, err := cookieConfigFromEnv()
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Secure != tc.wantSecure || cfg.SameSite != tc.wantSame || cfg.Path != tc.wantPath {
				t.Fatalf("unexpected config: %+v", cfg)
			}
		})
	}
}
