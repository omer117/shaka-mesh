package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/omer-asraf/shaka-notification-service/internal/consumer"
)

const maxReconnectDelay = 30 * time.Second

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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	notificationConsumer := consumer.New(amqpURL, log.Default())
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
