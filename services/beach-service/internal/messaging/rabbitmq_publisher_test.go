package messaging

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/omer-asraf/shaka-beach-service/internal/domain"
	amqp "github.com/rabbitmq/amqp091-go"
)

func TestRabbitMQPublisherIntegration(t *testing.T) {
	amqpURL := os.Getenv("RABBITMQ_TEST_URL")
	if amqpURL == "" {
		t.Skip("set RABBITMQ_TEST_URL to run the RabbitMQ integration test")
	}

	publisher, err := NewRabbitMQPublisher(amqpURL)
	if err != nil {
		t.Fatalf("create RabbitMQ publisher: %v", err)
	}
	t.Cleanup(func() {
		if err := publisher.Close(); err != nil {
			t.Errorf("close RabbitMQ publisher: %v", err)
		}
	})

	connection, err := amqp.Dial(amqpURL)
	if err != nil {
		t.Fatalf("connect integration-test consumer: %v", err)
	}
	t.Cleanup(func() {
		if err := connection.Close(); err != nil {
			t.Errorf("close integration-test consumer: %v", err)
		}
	})
	channel, err := connection.Channel()
	if err != nil {
		t.Fatalf("open integration-test channel: %v", err)
	}
	t.Cleanup(func() {
		if err := channel.Close(); err != nil {
			t.Errorf("close integration-test channel: %v", err)
		}
	})

	queue, err := channel.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		t.Fatalf("declare temporary test queue: %v", err)
	}
	if err := channel.QueueBind(queue.Name, "surf.swell.#", SurfEventsExchange, false, nil); err != nil {
		t.Fatalf("bind temporary test queue: %v", err)
	}
	messages, err := channel.Consume(queue.Name, "", true, true, false, false, nil)
	if err != nil {
		t.Fatalf("consume from temporary test queue: %v", err)
	}

	event := domain.SwellAlertEvent{
		BeachID:    "integration-test",
		BeachName:  "Test Beach",
		WaveHeight: 1.7,
		Timestamp:  time.Now().UTC().Truncate(time.Second),
	}
	if err := publisher.PublishSwellAlert(context.Background(), event); err != nil {
		t.Fatalf("publish integration-test event: %v", err)
	}

	select {
	case message := <-messages:
		var got domain.SwellAlertEvent
		if err := json.Unmarshal(message.Body, &got); err != nil {
			t.Fatalf("decode published event: %v", err)
		}
		if got != event {
			t.Fatalf("published event mismatch: got %+v, want %+v", got, event)
		}
		if message.RoutingKey != SwellAlertRoutingKey {
			t.Fatalf("unexpected routing key %q", message.RoutingKey)
		}
		if message.ContentType != "application/json" {
			t.Fatalf("unexpected content type %q", message.ContentType)
		}
		if message.DeliveryMode != amqp.Persistent {
			t.Fatalf("expected persistent delivery mode, got %d", message.DeliveryMode)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for published swell alert")
	}
}
