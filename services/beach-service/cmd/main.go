package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/omer-asraf/shaka-beach-service/internal/handler"
	"github.com/omer-asraf/shaka-beach-service/internal/messaging"
	"github.com/omer-asraf/shaka-beach-service/internal/repository"
	"github.com/omer-asraf/shaka-beach-service/internal/swell"
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
	address := os.Getenv("HTTP_ADDR")
	if address == "" {
		address = ":8080"
	}
	amqpURL := os.Getenv("AMQP_URL")
	if amqpURL == "" {
		return errors.New("AMQP_URL must be set")
	}
	refreshInterval, err := swellRefreshInterval(os.Getenv("SWELL_REFRESH_INTERVAL"))
	if err != nil {
		return err
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

	publisher, err := messaging.NewRabbitMQPublisher(amqpURL)
	if err != nil {
		return err
	}
	defer func() {
		if err := publisher.Close(); err != nil {
			log.Printf("close RabbitMQ publisher: %v", err)
		}
	}()

	beachRepository := repository.NewPostgresBeachRepository(pool)
	ingestor := swell.NewIngestor(beachRepository, swell.NewOpenMeteoClient(), publisher)
	ingestorStopped := make(chan struct{})
	go func() {
		defer close(ingestorStopped)
		if err := ingestor.Run(ctx, refreshInterval); err != nil {
			log.Printf("swell ingestor stopped: %v", err)
		}
	}()
	defer func() {
		stop()
		<-ingestorStopped
	}()

	server := &http.Server{
		Addr:              address,
		Handler:           handler.New(beachRepository),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("beach-service listening on %s", address)
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

func swellRefreshInterval(raw string) (time.Duration, error) {
	if raw == "" {
		return 30 * time.Minute, nil
	}
	interval, err := time.ParseDuration(raw)
	if err != nil || interval <= 0 {
		return 0, fmt.Errorf("SWELL_REFRESH_INTERVAL must be a positive duration (for example, 30m)")
	}
	return interval, nil
}
