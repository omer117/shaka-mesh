package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/omer-asraf/shaka-beach-service/internal/domain"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	SurfEventsExchange   = "surf_events"
	SwellAlertRoutingKey = "surf.swell.alert"
)

type RabbitMQPublisher struct {
	connection *amqp.Connection
	channel    *amqp.Channel
}

func NewRabbitMQPublisher(amqpURL string) (*RabbitMQPublisher, error) {
	connection, err := amqp.Dial(amqpURL)
	if err != nil {
		return nil, fmt.Errorf("connect to RabbitMQ: %w", err)
	}
	channel, err := connection.Channel()
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("open RabbitMQ channel: %w", err)
	}
	if err := channel.ExchangeDeclare(
		SurfEventsExchange,
		amqp.ExchangeTopic,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		_ = channel.Close()
		_ = connection.Close()
		return nil, fmt.Errorf("declare RabbitMQ exchange %q: %w", SurfEventsExchange, err)
	}
	return &RabbitMQPublisher{connection: connection, channel: channel}, nil
}

func (p *RabbitMQPublisher) PublishSwellAlert(ctx context.Context, event domain.SwellAlertEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode swell alert: %w", err)
	}
	if err := p.channel.PublishWithContext(
		ctx,
		SurfEventsExchange,
		SwellAlertRoutingKey,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now().UTC(),
			Body:         body,
		},
	); err != nil {
		return fmt.Errorf("publish swell alert: %w", err)
	}
	return nil
}

func (p *RabbitMQPublisher) Close() error {
	channelErr := p.channel.Close()
	connectionErr := p.connection.Close()
	if channelErr != nil {
		return fmt.Errorf("close RabbitMQ channel: %w", channelErr)
	}
	if connectionErr != nil {
		return fmt.Errorf("close RabbitMQ connection: %w", connectionErr)
	}
	return nil
}
