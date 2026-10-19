package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/omer-asraf/shaka-notification-service/internal/domain"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	SurfEventsExchange = "surf_events"
	SwellQueue         = "notifications_swell_queue"
	SwellRoutingKey    = "surf.swell.#"

	// Dead-letter topology: rejected (Reject(false)) messages from SwellQueue
	// are routed by the broker to DeadLetterExchange and land in DeadLetterQueue.
	DeadLetterExchange   = "swell-alerts.dlx"
	DeadLetterQueue      = "swell-alerts.dlq"
	deadLetterRoutingKey = "swell-alerts.dead"
)

type Consumer struct {
	amqpURL string
	logger  *log.Logger
	onAlert func(domain.SwellAlertEvent)
}

type incomingDelivery interface {
	Ack(multiple bool) error
	Reject(requeue bool) error
	Body() []byte
}

type rabbitDelivery struct {
	delivery amqp.Delivery
}

func (d rabbitDelivery) Ack(multiple bool) error {
	return d.delivery.Ack(multiple)
}

func (d rabbitDelivery) Reject(requeue bool) error {
	return d.delivery.Reject(requeue)
}

func (d rabbitDelivery) Body() []byte {
	return d.delivery.Body
}

func New(amqpURL string, logger *log.Logger) *Consumer {
	return NewWithHandler(amqpURL, logger, nil)
}

func NewWithHandler(amqpURL string, logger *log.Logger, onAlert func(domain.SwellAlertEvent)) *Consumer {
	if logger == nil {
		logger = log.Default()
	}
	return &Consumer{amqpURL: amqpURL, logger: logger, onAlert: onAlert}
}

func (c *Consumer) Run(ctx context.Context) error {
	return c.runQueue(ctx, SwellQueue)
}

func (c *Consumer) runQueue(ctx context.Context, queueName string) error {
	if err := ctx.Err(); err != nil {
		return nil
	}
	connection, err := amqp.Dial(c.amqpURL)
	if err != nil {
		return fmt.Errorf("connect to RabbitMQ: %w", err)
	}
	stopClose := context.AfterFunc(ctx, func() {
		_ = connection.Close()
	})
	defer func() {
		stopClose()
		_ = connection.Close()
	}()

	channel, err := connection.Channel()
	if err != nil {
		return fmt.Errorf("open RabbitMQ channel: %w", err)
	}
	defer channel.Close()

	if err := channel.ExchangeDeclare(
		SurfEventsExchange,
		amqp.ExchangeTopic,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("declare RabbitMQ exchange %q: %w", SurfEventsExchange, err)
	}
	if err := declareDeadLetterTopology(channel); err != nil {
		return err
	}
	queueArgs := amqp.Table{
		"x-dead-letter-exchange":    DeadLetterExchange,
		"x-dead-letter-routing-key": deadLetterRoutingKey,
	}
	if _, err := channel.QueueDeclare(queueName, true, false, false, false, queueArgs); err != nil {
		return fmt.Errorf("declare RabbitMQ queue %q: %w", queueName, err)
	}
	if err := channel.QueueBind(queueName, SwellRoutingKey, SurfEventsExchange, false, nil); err != nil {
		return fmt.Errorf("bind RabbitMQ queue %q: %w", queueName, err)
	}
	if err := channel.Qos(1, 0, false); err != nil {
		return fmt.Errorf("set RabbitMQ consumer QoS: %w", err)
	}

	deliveries, err := channel.Consume(queueName, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("start consuming RabbitMQ queue %q: %w", queueName, err)
	}
	c.logger.Printf("notification consumer listening on %s with routing key %s", queueName, SwellRoutingKey)

	for {
		select {
		case <-ctx.Done():
			return nil
		case delivery, ok := <-deliveries:
			if !ok {
				if ctx.Err() != nil {
					return nil
				}
				return errors.New("RabbitMQ delivery channel closed")
			}
			if err := c.handleIncomingDelivery(rabbitDelivery{delivery: delivery}); err != nil {
				return err
			}
		}
	}
}

type topologyDeclarer interface {
	ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)
	QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error
}

func declareDeadLetterTopology(channel topologyDeclarer) error {
	if err := channel.ExchangeDeclare(DeadLetterExchange, amqp.ExchangeDirect, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare dead-letter exchange %q: %w", DeadLetterExchange, err)
	}
	if _, err := channel.QueueDeclare(DeadLetterQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare dead-letter queue %q: %w", DeadLetterQueue, err)
	}
	if err := channel.QueueBind(DeadLetterQueue, deadLetterRoutingKey, DeadLetterExchange, false, nil); err != nil {
		return fmt.Errorf("bind dead-letter queue %q: %w", DeadLetterQueue, err)
	}
	return nil
}

func (c *Consumer) handleIncomingDelivery(delivery incomingDelivery) error {
	var event domain.SwellAlertEvent
	if err := json.Unmarshal(delivery.Body(), &event); err != nil {
		c.logger.Printf("discarding invalid swell alert JSON: %v", err)
		if rejectErr := delivery.Reject(false); rejectErr != nil {
			return fmt.Errorf("reject invalid swell alert: %w", rejectErr)
		}
		return nil
	}
	if err := validateEvent(event); err != nil {
		c.logger.Printf("discarding invalid swell alert: %v", err)
		if rejectErr := delivery.Reject(false); rejectErr != nil {
			return fmt.Errorf("reject invalid swell alert: %w", rejectErr)
		}
		return nil
	}

	c.logger.Printf(
		"SWELL ALERT: beach=%q (id=%q), wave_height=%.2fm, observed_at=%s",
		event.BeachName,
		event.BeachID,
		event.WaveHeight,
		event.Timestamp.UTC().Format(time.RFC3339),
	)
	if c.onAlert != nil {
		c.onAlert(event)
	}
	if err := delivery.Ack(false); err != nil {
		return fmt.Errorf("acknowledge swell alert for beach %q: %w", event.BeachID, err)
	}
	return nil
}

func validateEvent(event domain.SwellAlertEvent) error {
	if event.BeachID == "" {
		return errors.New("beach_id is required")
	}
	if event.BeachName == "" {
		return errors.New("beach_name is required")
	}
	if math.IsNaN(event.WaveHeight) || math.IsInf(event.WaveHeight, 0) || event.WaveHeight < 0 {
		return errors.New("wave_height must be a finite non-negative number")
	}
	if event.Timestamp.IsZero() {
		return errors.New("timestamp is required")
	}
	return nil
}
