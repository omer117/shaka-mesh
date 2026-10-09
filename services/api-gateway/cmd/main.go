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
	"syscall"
	"time"

	"github.com/omer-asraf/shaka-api-gateway/internal/authn"
	"github.com/omer-asraf/shaka-api-gateway/internal/proxy"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return errors.New("JWT_SECRET must be set")
	}
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
	jwtMiddleware, err := authn.NewMiddleware(jwtSecret, issuer)
	if err != nil {
		return err
	}
	handler, err := proxy.NewHandler(authTarget, beachTarget, communityTarget, jwtMiddleware, allowedOrigin)
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
