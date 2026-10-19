package messaging

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/omer-asraf/shaka-beach-service/internal/domain"
)

type fakeConfirmation struct {
	acked bool
	err   error
	block bool
}

func (c fakeConfirmation) WaitContext(ctx context.Context) (bool, error) {
	if c.block {
		<-ctx.Done()
		return false, ctx.Err()
	}
	return c.acked, c.err
}

type fakeSession struct {
	mu         sync.Mutex
	conf       fakeConfirmation
	publishErr error
	lost       chan struct{}
	bodies     [][]byte
	closed     bool
}

func newFakeSession(conf fakeConfirmation) *fakeSession {
	return &fakeSession{conf: conf, lost: make(chan struct{})}
}

func (s *fakeSession) publish(_ context.Context, body []byte) (confirmation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.publishErr != nil {
		return nil, s.publishErr
	}
	s.bodies = append(s.bodies, body)
	return s.conf, nil
}

func (s *fakeSession) done() <-chan struct{} { return s.lost }

func (s *fakeSession) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *fakeSession) drop() { close(s.lost) }

func testEvent() domain.SwellAlertEvent {
	return domain.SwellAlertEvent{BeachID: "b1", BeachName: "Hilton", WaveHeight: 1.5, Timestamp: time.Now().UTC()}
}

func TestPublishSwellAlertConfirmHandling(t *testing.T) {
	boom := errors.New("boom")
	failingPublish := newFakeSession(fakeConfirmation{})
	failingPublish.publishErr = boom
	tests := []struct {
		name    string
		session *fakeSession
		wantErr error
	}{
		{"acked", newFakeSession(fakeConfirmation{acked: true}), nil},
		{"nacked", newFakeSession(fakeConfirmation{acked: false}), ErrNotConfirmed},
		{"confirm timeout", newFakeSession(fakeConfirmation{block: true}), context.DeadlineExceeded},
		{"confirm error", newFakeSession(fakeConfirmation{err: boom}), boom},
		{"publish error", failingPublish, boom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := newPublisher(func() (session, error) { return tt.session, nil })
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			p.confirmTimeout = 20 * time.Millisecond
			err = p.PublishSwellAlert(context.Background(), testEvent())
			if tt.wantErr == nil && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("got %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestPublishFailsWhileDisconnected(t *testing.T) {
	first := newFakeSession(fakeConfirmation{acked: true})
	var mu sync.Mutex
	dials := 0
	p, err := newPublisher(func() (session, error) {
		mu.Lock()
		defer mu.Unlock()
		dials++
		if dials == 1 {
			return first, nil
		}
		return nil, errors.New("broker down")
	})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	p.baseDelay, p.maxDelay = time.Millisecond, 5*time.Millisecond
	first.drop()
	if err := p.PublishSwellAlert(context.Background(), testEvent()); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("got %v, want ErrNotConnected", err)
	}
}

func TestReconnectsAfterConnectionLoss(t *testing.T) {
	first := newFakeSession(fakeConfirmation{acked: true})
	second := newFakeSession(fakeConfirmation{acked: true})
	var mu sync.Mutex
	dials := 0
	p, err := newPublisher(func() (session, error) {
		mu.Lock()
		defer mu.Unlock()
		dials++
		switch dials {
		case 1:
			return first, nil
		case 2, 3:
			return nil, errors.New("broker down") // two failed retries first
		default:
			return second, nil
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	p.baseDelay, p.maxDelay = time.Millisecond, 5*time.Millisecond

	first.drop()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := p.PublishSwellAlert(context.Background(), testEvent()); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("publisher never recovered")
		}
		time.Sleep(time.Millisecond)
	}
	mu.Lock()
	gotDials := dials
	mu.Unlock()
	if gotDials != 4 {
		t.Fatalf("expected 4 dials (1 initial + 2 failed + 1 ok), got %d", gotDials)
	}
	first.mu.Lock()
	closed := first.closed
	first.mu.Unlock()
	second.mu.Lock()
	published := len(second.bodies)
	second.mu.Unlock()
	if published != 1 || !closed {
		t.Fatalf("expected one publish on second session and first closed; published=%d closed=%v", published, closed)
	}
}

func TestBackoffDelay(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{0, time.Second}, {1, 2 * time.Second}, {2, 4 * time.Second},
		{4, 16 * time.Second}, {5, 30 * time.Second}, {50, 30 * time.Second},
	}
	for _, tt := range tests {
		if got := backoffDelay(tt.attempt, time.Second, 30*time.Second); got != tt.want {
			t.Errorf("attempt %d: got %s, want %s", tt.attempt, got, tt.want)
		}
	}
}
