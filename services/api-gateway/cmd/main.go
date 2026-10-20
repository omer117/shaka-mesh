package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/omer-asraf/shaka-api-gateway/internal/authn"
	"github.com/omer-asraf/shaka-api-gateway/internal/proxy"
	"github.com/omer-asraf/shaka-api-gateway/internal/ratelimit"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	issuer := os.Getenv("JWT_ISSUER")
	if issuer == "" {
		issuer = "shaka-mesh-auth"
	}
	authTarget, err := upstreamURL("AUTH_SERVICE_URL", "http://localhost:8081")
	if err != nil {
		return err
	}
	beachTarget, err := upstreamURL("BEACH_SERVICE_URL", "http://localhost:8080")
	if err != nil {
		return err
	}
	communityTarget, err := upstreamURL("COMMUNITY_SERVICE_URL", "http://localhost:8083")
	if err != nil {
		return err
	}
	allowedOrigin := os.Getenv("CORS_ALLOWED_ORIGIN")
	if allowedOrigin == "" {
		allowedOrigin = "http://localhost:3000"
	}
	// Verification uses auth-service's public keys only; there is no shared secret.
	jwksURL := os.Getenv("JWKS_URL")
	if jwksURL == "" {
		jwksURL = strings.TrimRight(authTarget.String(), "/") + "/.well-known/jwks.json"
	}
	jwks := authn.NewJWKSClient(jwksURL)
	prefetchCtx, cancelPrefetch := context.WithTimeout(context.Background(), 5*time.Second)
	if err := jwks.Prefetch(prefetchCtx); err != nil {
		log.Printf("could not load signing keys yet (will retry on first request): %v", err)
	}
	cancelPrefetch()
	jwtMiddleware, err := authn.NewMiddleware(jwks, issuer)
	if err != nil {
		return err
	}
	rateLimit, err := rateLimitFromEnv()
	if err != nil {
		return err
	}
	handler, err := proxy.NewHandler(authTarget, beachTarget, communityTarget, jwtMiddleware, allowedOrigin,
		proxy.WithAuthRateLimit(rateLimit))
	if err != nil {
		return err
	}

	address := os.Getenv("HTTP_ADDR")
	if address == "" {
		address = ":8000"
	}
	server := &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", address, err)
	}
	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("api-gateway listening on %s", address)
		serverErrors <- server.Serve(listener)
	}()

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTP server: %w", err)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shut down HTTP server: %w", err)
		}
		if err := <-serverErrors; !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTP server: %w", err)
		}
	}
	return nil
}

func upstreamURL(envName, fallback string) (*url.URL, error) {
	raw := os.Getenv(envName)
	if raw == "" {
		raw = fallback
	}
	target, err := url.Parse(raw)
	if err != nil || target.Scheme == "" || target.Host == "" {
		return nil, fmt.Errorf("%s must be an absolute HTTP URL", envName)
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return nil, fmt.Errorf("%s must use http or https", envName)
	}
	return target, nil
}

// rateLimitFromEnv builds the login/register brute-force protection.
//
//	AUTH_RATE_LIMIT_IP_BURST / AUTH_RATE_LIMIT_IP_PER_MINUTE        (default 20 / 20)
//	AUTH_RATE_LIMIT_EMAIL_BURST / AUTH_RATE_LIMIT_EMAIL_PER_MINUTE  (default 5 / 5)
//	TRUSTED_PROXY_HOPS: number of reverse proxies in front of the gateway whose
//	X-Forwarded-For entries may be trusted (default 0 = ignore the header; the
//	cloud compose sets 1 for Caddy).
func rateLimitFromEnv() (func(http.Handler) http.Handler, error) {
	ipBurst, err := intEnv("AUTH_RATE_LIMIT_IP_BURST", 20)
	if err != nil {
		return nil, err
	}
	ipPerMinute, err := intEnv("AUTH_RATE_LIMIT_IP_PER_MINUTE", 20)
	if err != nil {
		return nil, err
	}
	emailBurst, err := intEnv("AUTH_RATE_LIMIT_EMAIL_BURST", 5)
	if err != nil {
		return nil, err
	}
	emailPerMinute, err := intEnv("AUTH_RATE_LIMIT_EMAIL_PER_MINUTE", 5)
	if err != nil {
		return nil, err
	}
	hops, err := intEnv("TRUSTED_PROXY_HOPS", 0)
	if err != nil {
		return nil, err
	}
	if hops < 0 {
		return nil, errors.New("TRUSTED_PROXY_HOPS must not be negative")
	}
	for name, value := range map[string]int{
		"AUTH_RATE_LIMIT_IP_BURST": ipBurst, "AUTH_RATE_LIMIT_IP_PER_MINUTE": ipPerMinute,
		"AUTH_RATE_LIMIT_EMAIL_BURST": emailBurst, "AUTH_RATE_LIMIT_EMAIL_PER_MINUTE": emailPerMinute,
	} {
		if value < 1 {
			return nil, fmt.Errorf("%s must be at least 1", name)
		}
	}
	return ratelimit.AuthMiddleware(ratelimit.AuthConfig{
		IP:               ratelimit.NewTokenBucket(ratelimit.PerMinute(ipBurst, float64(ipPerMinute))),
		Email:            ratelimit.NewTokenBucket(ratelimit.PerMinute(emailBurst, float64(emailPerMinute))),
		TrustedProxyHops: hops,
	}), nil
}

func intEnv(name string, fallback int) (int, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return value, nil
}
