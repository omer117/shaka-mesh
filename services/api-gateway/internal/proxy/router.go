package proxy

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/omer-asraf/shaka-api-gateway/internal/authn"
)

// Option customizes the gateway handler.
type Option func(*options)

type options struct {
	authLimiter func(http.Handler) http.Handler
}

// WithAuthRateLimit wraps the routes (inside the CORS layer, so 429 responses
// still carry CORS headers) with brute-force protection middleware.
func WithAuthRateLimit(middleware func(http.Handler) http.Handler) Option {
	return func(o *options) { o.authLimiter = middleware }
}

func NewHandler(authTarget, beachTarget, communityTarget *url.URL, jwtMiddleware *authn.Middleware, allowedOrigin string, opts ...Option) (http.Handler, error) {
	var settings options
	for _, opt := range opts {
		opt(&settings)
	}
	if authTarget == nil || beachTarget == nil || communityTarget == nil {
		return nil, errors.New("auth, beach, and community upstream URLs are required")
	}
	if allowedOrigin == "" {
		return nil, errors.New("CORS allowed origin is required")
	}
	// Credentialed CORS (cookies) is incompatible with a wildcard origin.
	if allowedOrigin == "*" {
		return nil, errors.New("CORS allowed origin must be an explicit origin, not '*'")
	}
	authProxy := newProxy(authTarget, true)
	beachProxy := newProxy(beachTarget, false)
	communityProxy := newProxy(communityTarget, false)
	routes := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/health" && r.Method == http.MethodGet:
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		case strings.HasPrefix(r.URL.Path, "/api/v1/auth/"):
			authProxy.ServeHTTP(w, r)
		case r.URL.Path == "/api/v1/beaches" || strings.HasPrefix(r.URL.Path, "/api/v1/beaches/"):
			attachIdentity(r)
			beachProxy.ServeHTTP(w, r)
		case r.URL.Path == "/api/v1/shop" || strings.HasPrefix(r.URL.Path, "/api/v1/shop/"),
			r.URL.Path == "/api/v1/forum" || strings.HasPrefix(r.URL.Path, "/api/v1/forum/"):
			attachIdentity(r)
			communityProxy.ServeHTTP(w, r)
		default:
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "route not found"})
		}
	})
	if jwtMiddleware == nil {
		return nil, errors.New("JWT middleware is required")
	}
	authenticatedRoutes := jwtMiddleware.Wrap(routes)
	if settings.authLimiter != nil {
		authenticatedRoutes = settings.authLimiter(authenticatedRoutes)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Add("Vary", "Origin")
			if origin == allowedOrigin {
				w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept")
				w.Header().Set("Access-Control-Max-Age", "600")
			}
		}
		if r.Method == http.MethodOptions {
			if r.Header.Get("Origin") != allowedOrigin {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "origin is not allowed"})
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if !isKnownRoute(r) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "route not found"})
			return
		}
		authenticatedRoutes.ServeHTTP(w, r)
	}), nil
}

func attachIdentity(r *http.Request) {
	if claims, ok := authn.UserClaimsFromContext(r.Context()); ok {
		r.Header.Set("X-Authenticated-User-ID", claims.UserID)
		r.Header.Set("X-Authenticated-User-Email", claims.Email)
		return
	}
	r.Header.Del("X-Authenticated-User-ID")
	r.Header.Del("X-Authenticated-User-Email")
}

func isKnownRoute(r *http.Request) bool {
	if r.URL.Path == "/health" {
		return r.Method == http.MethodGet
	}
	return strings.HasPrefix(r.URL.Path, "/api/v1/auth/") ||
		r.URL.Path == "/api/v1/beaches" ||
		strings.HasPrefix(r.URL.Path, "/api/v1/beaches/") ||
		r.URL.Path == "/api/v1/shop" ||
		strings.HasPrefix(r.URL.Path, "/api/v1/shop/") ||
		r.URL.Path == "/api/v1/forum" ||
		strings.HasPrefix(r.URL.Path, "/api/v1/forum/")
}

func newProxy(target *url.URL, stripAPIPrefix bool) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target)
	director := proxy.Director
	proxy.Director = func(request *http.Request) {
		director(request)
		if stripAPIPrefix {
			request.URL.Path = strings.TrimPrefix(request.URL.Path, "/api/v1")
			request.URL.RawPath = ""
		}
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("proxy request to %s failed: %v", target, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream service unavailable"})
	}
	proxy.FlushInterval = 100 * time.Millisecond
	return proxy
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write gateway response: %v", err)
	}
}
