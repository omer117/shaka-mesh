package stream

import (
	"sync"

	"github.com/omer-asraf/shaka-notification-service/internal/domain"
)

const subscriberBufferSize = 16

type Hub struct {
	mu          sync.Mutex
	subscribers map[chan domain.SwellAlertEvent]struct{}
}

func NewHub() *Hub {
	return &Hub{subscribers: make(map[chan domain.SwellAlertEvent]struct{})}
}

func (h *Hub) Subscribe() (<-chan domain.SwellAlertEvent, func()) {
	channel := make(chan domain.SwellAlertEvent, subscriberBufferSize)
	h.mu.Lock()
	h.subscribers[channel] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subscribers, channel)
			close(channel)
			h.mu.Unlock()
		})
	}
	return channel, unsubscribe
}

func (h *Hub) Publish(event domain.SwellAlertEvent) int {
	h.mu.Lock()
	defer h.mu.Unlock()

	dropped := 0
	for subscriber := range h.subscribers {
		select {
		case subscriber <- event:
		default:
			select {
			case <-subscriber:
				dropped++
			default:
			}
			subscriber <- event
		}
	}
	return dropped
}

func (h *Hub) subscriberCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subscribers)
}
