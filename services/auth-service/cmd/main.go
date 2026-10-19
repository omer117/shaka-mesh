package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/omer-asraf/shaka-auth-service/internal/handler"
	"github.com/omer-asraf/shaka-auth-service/internal/repository"
	"github.com/omer-asraf/shaka-auth-service/internal/service"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errors.New("DATABASE_URL must be set")
	}
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return errors.New("JWT_SECRET must be set")
	}
	issuer := os.Getenv("JWT_ISSUER")
	if issuer == "" {
		issuer = "shaka-mesh-auth"
	}
	tokenTTL, err := durationEnv("JWT_TTL", "15m")
	if err != nil {
		return err
	}
	tokenManager, err := service.NewJWTManager(jwtSecret, issuer, tokenTTL)
	if err != nil {
		return err
	}
	refreshTTL, err := durationEnv("REFRESH_TOKEN_TTL", "168h")
	if err != nil {
		return err
	}
	cookies, err := cookieConfigFromEnv()
	if err != nil {
		return err
	}
	address := os.Getenv("HTTP_ADDR")
	if address == "" {
		address = ":8081"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("create database pool: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}

	authService := service.NewAuthService(
		repository.NewPostgresUserRepository(pool),
		tokenManager,
		repository.NewPostgresRefreshTokenRepository(pool),
		refreshTTL,
	)
	server := &http.Server{
		Addr:              address,
		Handler:           handler.New(authService, cookies),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("auth-service listening on %s", address)
		serverErrors <- server.ListenAndServe()
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

func durationEnv(name, fallback string) (time.Duration, error) {
	raw := os.Getenv(name)
	if raw == "" {
		raw = fallback
	}
	duration, err := time.ParseDuration(raw)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return duration, nil
}

// cookieConfigFromEnv reads the refresh-cookie settings. Secure defaults to
// true; set REFRESH_COOKIE_SECURE=false only for plain-HTTP local development,
// because browsers drop Secure cookies on http:// origins (Safari always does,
// Chrome and Firefox make an exception for localhost).
func cookieConfigFromEnv() (handler.CookieConfig, error) {
	cfg := handler.DefaultCookieConfig()
	if raw := os.Getenv("REFRESH_COOKIE_SECURE"); raw != "" {
		secure, err := strconv.ParseBool(raw)
		if err != nil {
			return cfg, errors.New("REFRESH_COOKIE_SECURE must be true or false")
		}
		cfg.Secure = secure
	}
	if raw := os.Getenv("REFRESH_COOKIE_PATH"); raw != "" {
		cfg.Path = raw
	}
	switch strings.ToLower(os.Getenv("REFRESH_COOKIE_SAMESITE")) {
	case "", "lax":
		cfg.SameSite = http.SameSiteLaxMode
	case "strict":
		cfg.SameSite = http.SameSiteStrictMode
	case "none":
		if !cfg.Secure {
			return cfg, errors.New("REFRESH_COOKIE_SAMESITE=none requires REFRESH_COOKIE_SECURE=true")
		}
		cfg.SameSite = http.SameSiteNoneMode
	default:
		return cfg, errors.New("REFRESH_COOKIE_SAMESITE must be lax, strict or none")
	}
	return cfg, nil
}
