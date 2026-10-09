package stream

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/omer-asraf/shaka-notification-service/internal/domain"
)

type eventSignalRecorder struct {
	*httptest.ResponseRecorder
	eventWritten chan struct{}
}

func (r *eventSignalRecorder) Write(data []byte) (int, error) {
	n, err := r.ResponseRecorder.Write(data)
	if strings.Contains(string(data), "event: swell-alert\n") {
		close(r.eventWritten)
	}
	return n, err
}

func TestEventsStreamsPublishedAlerts(t *testing.T) {
	hub := NewHub()
	handler, err := NewHandler(hub, "http://localhost:3000")
	if err != nil {
		t.Fatalf("create handler: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)
	request.Header.Set("Origin", "http://localhost:3000")
	response := &eventSignalRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		eventWritten:     make(chan struct{}),
	}
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(response, request)
		close(done)
	}()

	deadline := time.Now().Add(time.Second)
	for hub.subscriberCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if hub.subscriberCount() != 1 {
		cancel()
		t.Fatal("SSE handler did not subscribe to the event hub")
	}

	event := domain.SwellAlertEvent{
		BeachID:    "beach-1",
		BeachName:  "Hilton Beach",
		WaveHeight: 1.8,
		Timestamp:  time.Date(2026, time.October, 9, 7, 0, 0, 0, time.UTC),
	}
	hub.Publish(event)
	select {
	case <-response.eventWritten:
		cancel()
	case <-time.After(time.Second):
		cancel()
		t.Fatal("SSE handler did not write the published alert")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SSE handler did not stop after cancellation")
	}

	if got := response.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("got content type %q", got)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("got CORS origin %q", got)
	}
	if body := response.Body.String(); !strings.Contains(body, "event: swell-alert\n") ||
		!strings.Contains(body, `"beach_name":"Hilton Beach"`) {
		t.Fatalf("unexpected SSE body %q", body)
	}
}

func TestEventsRejectsUnconfiguredOrigin(t *testing.T) {
	handler, err := NewHandler(NewHub(), "http://localhost:3000")
	if err != nil {
		t.Fatalf("create handler: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/events", nil)
	request.Header.Set("Origin", "https://untrusted.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("got status %d, want %d", response.Code, http.StatusForbidden)
	}
}
