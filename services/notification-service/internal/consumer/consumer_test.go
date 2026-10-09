package consumer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/omer-asraf/shaka-notification-service/internal/domain"
	amqp "github.com/rabbitmq/amqp091-go"
)

type acknowledgingDelivery struct {
	body        []byte
	ackCalls    int
	rejectCalls int
	requeue     bool
	ackErr      error
	rejectErr   error
}

func (d *acknowledgingDelivery) Body() []byte {
	return d.body
}

type synchronizedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *synchronizedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(data)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func (d *acknowledgingDelivery) Ack(bool) error {
	d.ackCalls++
	return d.ackErr
}

func (d *acknowledgingDelivery) Reject(requeue bool) error {
	d.rejectCalls++
	d.requeue = requeue
	return d.rejectErr
}

func (d *acknowledgingDelivery) Nack(uint64, bool, bool) error {
	return nil
}

func TestHandleDeliveryLogsAndAcknowledgesAlert(t *testing.T) {
	var output bytes.Buffer
	event := domain.SwellAlertEvent{
		BeachID:    "b1",
		BeachName:  "Hilton Beach",
		WaveHeight: 1.8,
		Timestamp:  time.Date(2026, time.October, 9, 7, 0, 0, 0, time.UTC),
	}
	var published domain.SwellAlertEvent
	alertConsumer := NewWithHandler("", log.New(&output, "", 0), func(alert domain.SwellAlertEvent) {
		published = alert
	})
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("encode event: %v", err)
	}
	delivery := &acknowledgingDelivery{body: body}

	if err := alertConsumer.handleIncomingDelivery(delivery); err != nil {
		t.Fatalf("handle delivery: %v", err)
	}
	if delivery.ackCalls != 1 || delivery.rejectCalls != 0 {
		t.Fatalf("expected one ack and no rejects, got ack=%d reject=%d", delivery.ackCalls, delivery.rejectCalls)
	}
	if published != event {
		t.Fatalf("expected event to be published before acknowledgement, got %+v", published)
	}
	logged := output.String()
	for _, want := range []string{"SWELL ALERT", `"Hilton Beach"`, `id="b1"`, "wave_height=1.80m"} {
		if !strings.Contains(logged, want) {
			t.Errorf("expected log %q to contain %q", logged, want)
		}
	}
}

func TestHandleDeliveryRejectsMalformedJSON(t *testing.T) {
	alertConsumer := New("", log.New(&bytes.Buffer{}, "", 0))
	delivery := &acknowledgingDelivery{body: []byte("{")}

	if err := alertConsumer.handleIncomingDelivery(delivery); err != nil {
		t.Fatalf("handle malformed delivery: %v", err)
	}
	if delivery.rejectCalls != 1 || delivery.requeue || delivery.ackCalls != 0 {
		t.Fatalf("expected a non-requeued reject, got reject=%d requeue=%t ack=%d", delivery.rejectCalls, delivery.requeue, delivery.ackCalls)
	}
}

func TestHandleDeliveryRejectsInvalidEvent(t *testing.T) {
	alertConsumer := New("", log.New(&bytes.Buffer{}, "", 0))
	delivery := &acknowledgingDelivery{body: []byte(`{"beach_id":"b1"}`)}

	if err := alertConsumer.handleIncomingDelivery(delivery); err != nil {
		t.Fatalf("handle invalid delivery: %v", err)
	}
	if delivery.rejectCalls != 1 || delivery.ackCalls != 0 {
		t.Fatalf("expected invalid event to be rejected, got reject=%d ack=%d", delivery.rejectCalls, delivery.ackCalls)
	}
}

func TestHandleDeliveryReturnsAckFailure(t *testing.T) {
	alertConsumer := New("", log.New(&bytes.Buffer{}, "", 0))
	event := domain.SwellAlertEvent{
		BeachID: "b1", BeachName: "Beach", WaveHeight: 2, Timestamp: time.Now(),
	}
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("encode event: %v", err)
	}
	delivery := &acknowledgingDelivery{
		body:   body,
		ackErr: fmt.Errorf("connection closed"),
	}

	if err := alertConsumer.handleIncomingDelivery(delivery); err == nil {
		t.Fatal("expected ack error")
	}
}

func TestRunConsumesAndLogsRabbitMQAlert(t *testing.T) {
	amqpURL := os.Getenv("RABBITMQ_TEST_URL")
	if amqpURL == "" {
		t.Skip("set RABBITMQ_TEST_URL to run the RabbitMQ integration test")
	}

	loggerOutput := &synchronizedBuffer{}
	alertConsumer := New(amqpURL, log.New(loggerOutput, "", 0))
	queueName := fmt.Sprintf("notifications_swell_integration_%d", time.Now().UnixNano())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	runErrors := make(chan error, 1)
	go func() {
		runErrors <- alertConsumer.runQueue(ctx, queueName)
	}()

	waitForLog(t, loggerOutput, "notification consumer listening on "+queueName)
	connection, err := amqp.Dial(amqpURL)
	if err != nil {
		cancel()
		t.Fatalf("connect test publisher: %v", err)
	}
	defer connection.Close()
	channel, err := connection.Channel()
	if err != nil {
		cancel()
		t.Fatalf("open test publisher channel: %v", err)
	}
	defer channel.Close()
	event := domain.SwellAlertEvent{
		BeachID:    fmt.Sprintf("notification-integration-%d", time.Now().UnixNano()),
		BeachName:  "Integration Test Beach",
		WaveHeight: 1.75,
		Timestamp:  time.Now().UTC().Truncate(time.Second),
	}
	body, err := json.Marshal(event)
	if err != nil {
		cancel()
		t.Fatalf("encode event: %v", err)
	}
	if err := channel.PublishWithContext(ctx, SurfEventsExchange, "surf.swell.alert", false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	}); err != nil {
		cancel()
		t.Fatalf("publish integration-test event: %v", err)
	}
	waitForLog(t, loggerOutput, `SWELL ALERT: beach="Integration Test Beach"`)

	cancel()
	select {
	case err := <-runErrors:
		if err != nil {
			t.Fatalf("consumer returned an error on shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("consumer did not stop after cancellation")
	}

	cleanupConnection, err := amqp.Dial(amqpURL)
	if err != nil {
		t.Fatalf("connect to clean up integration queue: %v", err)
	}
	defer cleanupConnection.Close()
	cleanupChannel, err := cleanupConnection.Channel()
	if err != nil {
		t.Fatalf("open channel to clean up integration queue: %v", err)
	}
	defer cleanupChannel.Close()
	if _, err := cleanupChannel.QueueDelete(queueName, false, false, false); err != nil {
		t.Errorf("delete integration queue: %v", err)
	}
}

func waitForLog(t *testing.T, output *synchronizedBuffer, text string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(output.String(), text) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for log %q; got %q", text, output.String())
}

func TestRunReturnsOnCancellation(t *testing.T) {
	alertConsumer := New(os.Getenv("RABBITMQ_TEST_URL"), log.New(&bytes.Buffer{}, "", 0))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := alertConsumer.Run(ctx); err != nil {
		t.Fatalf("Run should return cleanly when already canceled, got %v", err)
	}
}
