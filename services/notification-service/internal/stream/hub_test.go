package stream

import (
	"testing"
	"time"

	"github.com/omer-asraf/shaka-notification-service/internal/domain"
)

func TestHubPublishesAndUnsubscribes(t *testing.T) {
	hub := NewHub()
	events, unsubscribe := hub.Subscribe()
	event := domain.SwellAlertEvent{BeachID: "beach-1", BeachName: "Hilton"}

	if dropped := hub.Publish(event); dropped != 0 {
		t.Fatalf("expected no dropped events, got %d", dropped)
	}
	select {
	case got := <-events:
		if got != event {
			t.Fatalf("got %+v, want %+v", got, event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for published event")
	}

	unsubscribe()
	unsubscribe()
	if got := hub.subscriberCount(); got != 0 {
		t.Fatalf("expected no subscribers after unsubscribe, got %d", got)
	}
}

func TestHubKeepsLatestEventForSlowSubscriber(t *testing.T) {
	hub := NewHub()
	events, unsubscribe := hub.Subscribe()
	defer unsubscribe()

	for i := 0; i <= subscriberBufferSize; i++ {
		hub.Publish(domain.SwellAlertEvent{BeachID: string(rune('a' + i))})
	}
	if got := hub.Publish(domain.SwellAlertEvent{BeachID: "latest"}); got != 1 {
		t.Fatalf("expected to count a slow subscriber, got %d", got)
	}
	var last domain.SwellAlertEvent
	for i := 0; i < subscriberBufferSize; i++ {
		last = <-events
	}
	if last.BeachID != "latest" {
		t.Fatalf("got final event %q, want latest", last.BeachID)
	}
}
