package ratelimit

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxInspectedBodyBytes = 1 << 20

// AuthConfig configures AuthMiddleware.
type AuthConfig struct {
	// IP limits attempts per client address, Email per submitted email
	// address. Each is applied separately to login and register.
	IP    Limiter
	Email Limiter
	// TrustedProxyHops is passed to ClientIP.
	TrustedProxyHops int
}

// AuthMiddleware rate-limits POST /api/v1/auth/login and /api/v1/auth/register
// per client IP and per email, answering 429 with a Retry-After header. Other
// requests pass through untouched.
func AuthMiddleware(cfg AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			action, limited := limitedAction(r)
			if !limited {
				next.ServeHTTP(w, r)
				return
			}

			ip := ClientIP(r, cfg.TrustedProxyHops)
			if ok, wait := cfg.IP.Allow(action + ":ip:" + ip); !ok {
				reject(w, wait)
				return
			}

			if email := peekEmail(r); email != "" {
				if ok, wait := cfg.Email.Allow(action + ":email:" + email); !ok {
					reject(w, wait)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func limitedAction(r *http.Request) (string, bool) {
	if r.Method != http.MethodPost {
		return "", false
	}
	switch r.URL.Path {
	case "/api/v1/auth/login":
		return "login", true
	case "/api/v1/auth/register":
		return "register", true
	}
	return "", false
}

// peekEmail reads the JSON body to find the email, then puts the body back so
// the proxy can forward it unchanged. Unparseable bodies return "" and are
// still covered by the per-IP limit.
func peekEmail(r *http.Request) string {
	if r.Body == nil {
		return ""
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxInspectedBodyBytes+1))
	if err != nil || len(raw) > maxInspectedBodyBytes {
		// Too large or unreadable: restore what we have; auth-service rejects it.
		r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(raw), r.Body))
		return ""
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	var payload struct {
		Email string `json:"email"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return ""
	}
	email := strings.ToLower(strings.TrimSpace(payload.Email))
	if len(email) > 320 {
		email = email[:320]
	}
	return email
}

func reject(w http.ResponseWriter, wait time.Duration) {
	seconds := int(math.Ceil(wait.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusTooManyRequests)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"error":               "too many attempts, please try again later",
		"retry_after_seconds": seconds,
	}); err != nil {
		log.Printf("write rate-limit response: %v", err)
	}
}
