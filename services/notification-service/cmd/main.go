package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/omer-asraf/shaka-notification-service/internal/consumer"
	"github.com/omer-asraf/shaka-notification-service/internal/domain"
	"github.com/omer-asraf/shaka-notification-service/internal/stream"
)

const (
	maxReconnectDelay = 30 * time.Second
	shutdownTimeout   = 10 * time.Second
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	amqpURL := os.Getenv("AMQP_URL")
	if amqpURL == "" {
		return errors.New("AMQP_URL must be set")
	}
	httpAddr := os.Getenv("HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":8082"
	}
	allowedOrigin := os.Getenv("CORS_ALLOWED_ORIGIN")
	if allowedOrigin == "" {
		allowedOrigin = "http://localhost:3000"
	}

	hub := stream.NewHub()
	handler, err := stream.NewHandler(hub, allowedOrigin)
	if err != nil {
		return fmt.Errorf("configure notification HTTP handler: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{
		Addr:              httpAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
	}

	notificationConsumer := consumer.NewWithHandler(amqpURL, log.Default(), func(event domain.SwellAlertEvent) {
		if dropped := hub.Publish(event); dropped > 0 {
			log.Printf("dropped stale swell alerts for %d slow SSE subscriber(s)", dropped)
		}
	})

	consumerDone := make(chan error, 1)
	go func() {
		consumerDone <- runConsumer(ctx, notificationConsumer)
	}()
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- server.ListenAndServe()
	}()

	var runErr error
	consumerStopped := false
	select {
	case <-ctx.Done():
	case err := <-serverDone:
		if !errors.Is(err, http.ErrServerClosed) {
			runErr = fmt.Errorf("serve notification HTTP API: %w", err)
		}
	case err := <-consumerDone:
		consumerStopped = true
		if err != nil {
			runErr = fmt.Errorf("run notification consumer: %w", err)
		}
	}
	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return fmt.Errorf("shut down notification HTTP API: %w", err)
	}
	if !consumerStopped {
		select {
		case err := <-consumerDone:
			if err != nil {
				return fmt.Errorf("stop notification consumer: %w", err)
			}
		case <-shutdownCtx.Done():
			return errors.New("notification consumer did not stop before the shutdown deadline")
		}
	}
	if runErr != nil {
		return runErr
	}
	return nil
}

func runConsumer(ctx context.Context, notificationConsumer *consumer.Consumer) error {
	delay := time.Second
	for {
		err := notificationConsumer.Run(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err == nil {
			return nil
		}
		log.Printf("notification consumer stopped unexpectedly: %v; reconnecting in %s", err, delay)

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return nil
		case <-timer.C:
		}
		delay *= 2
		if delay > maxReconnectDelay {
			delay = maxReconnectDelay
		}
	}
}
