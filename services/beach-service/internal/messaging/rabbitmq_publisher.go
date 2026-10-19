package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/omer-asraf/shaka-beach-service/internal/domain"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	SurfEventsExchange   = "surf_events"
	SwellAlertRoutingKey = "surf.swell.alert"

	defaultConfirmTimeout = 5 * time.Second
	defaultBaseDelay      = time.Second
	defaultMaxDelay       = 30 * time.Second
)

var (
	// ErrNotConnected is returned while the publisher is waiting to reconnect.
	ErrNotConnected = errors.New("RabbitMQ publisher is not connected")
	// ErrNotConfirmed is returned when the broker nacks a message.
	ErrNotConfirmed = errors.New("RabbitMQ broker did not confirm the message")
)

// confirmation is the broker's answer for one published message.
type confirmation interface {
	WaitContext(ctx context.Context) (bool, error)
}

// session is one live connection+channel with publisher confirms enabled.
type session interface {
	publish(ctx context.Context, body []byte) (confirmation, error)
	// done is closed when the connection or channel is lost.
	done() <-chan struct{}
	close() error
}

type dialFunc func() (session, error)

type RabbitMQPublisher struct {
	dial           dialFunc
	confirmTimeout time.Duration
	baseDelay      time.Duration
	maxDelay       time.Duration

	mu      sync.RWMutex
	current session

	cancel  context.CancelFunc
	stopped chan struct{}
}

func NewRabbitMQPublisher(amqpURL string) (*RabbitMQPublisher, error) {
	return newPublisher(func() (session, error) { return dialRabbit(amqpURL) })
}

func newPublisher(dial dialFunc) (*RabbitMQPublisher, error) {
	first, err := dial()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &RabbitMQPublisher{
		dial:           dial,
		confirmTimeout: defaultConfirmTimeout,
		baseDelay:      defaultBaseDelay,
		maxDelay:       defaultMaxDelay,
		current:        first,
		cancel:         cancel,
		stopped:        make(chan struct{}),
	}
	go p.supervise(ctx)
	return p, nil
}

// supervise waits for the current session to drop and then reconnects with
// exponential backoff until it succeeds or the publisher is closed.
func (p *RabbitMQPublisher) supervise(ctx context.Context) {
	defer close(p.stopped)
	for {
		p.mu.RLock()
		lost := p.current.done()
		p.mu.RUnlock()
		select {
		case <-ctx.Done():
			return
		case <-lost:
		}
		log.Printf("RabbitMQ publisher connection lost; reconnecting")
		p.mu.RLock()
		_ = p.current.close()
		p.mu.RUnlock()

		next, ok := p.reconnect(ctx)
		if !ok {
			return
		}
		p.mu.Lock()
		p.current = next
		p.mu.Unlock()
		log.Printf("RabbitMQ publisher reconnected")
	}
}

func (p *RabbitMQPublisher) reconnect(ctx context.Context) (session, bool) {
	for attempt := 0; ; attempt++ {
		s, err := p.dial()
		if err == nil {
			return s, true
		}
		delay := backoffDelay(attempt, p.baseDelay, p.maxDelay)
		log.Printf("RabbitMQ publisher reconnect failed: %v; retrying in %s", err, delay)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, false
		case <-timer.C:
		}
	}
}

// backoffDelay doubles base for every attempt, capped at max.
func backoffDelay(attempt int, base, max time.Duration) time.Duration {
	delay := base
	for i := 0; i < attempt; i++ {
		delay *= 2
		if delay >= max {
			return max
		}
	}
	if delay > max {
		return max
	}
	return delay
}

// PublishSwellAlert returns nil only after the broker has confirmed the
// message (or ErrNotConnected / a confirm error / timeout otherwise).
func (p *RabbitMQPublisher) PublishSwellAlert(ctx context.Context, event domain.SwellAlertEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode swell alert: %w", err)
	}
	p.mu.RLock()
	s := p.current
	p.mu.RUnlock()
	select {
	case <-s.done():
		return ErrNotConnected
	default:
	}

	conf, err := s.publish(ctx, body)
	if err != nil {
		return fmt.Errorf("publish swell alert: %w", err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, p.confirmTimeout)
	defer cancel()
	acked, err := conf.WaitContext(waitCtx)
	if err != nil {
		return fmt.Errorf("wait for publisher confirm: %w", err)
	}
	if !acked {
		return ErrNotConfirmed
	}
	return nil
}

func (p *RabbitMQPublisher) Close() error {
	p.cancel()
	<-p.stopped
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.current.close()
}

// rabbitSession is the real amqp091-go implementation of session.
type rabbitSession struct {
	connection *amqp.Connection
	channel    *amqp.Channel
	lost       chan struct{}
}

func dialRabbit(amqpURL string) (session, error) {
	connection, err := amqp.Dial(amqpURL)
	if err != nil {
		return nil, fmt.Errorf("connect to RabbitMQ: %w", err)
	}
	channel, err := connection.Channel()
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("open RabbitMQ channel: %w", err)
	}
	if err := channel.ExchangeDeclare(SurfEventsExchange, amqp.ExchangeTopic, true, false, false, false, nil); err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("declare RabbitMQ exchange %q: %w", SurfEventsExchange, err)
	}
	if err := channel.Confirm(false); err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("enable publisher confirms: %w", err)
	}
	s := &rabbitSession{connection: connection, channel: channel, lost: make(chan struct{})}
	connClosed := connection.NotifyClose(make(chan *amqp.Error, 1))
	chanClosed := channel.NotifyClose(make(chan *amqp.Error, 1))
	go func() {
		defer close(s.lost)
		select {
		case <-connClosed:
		case <-chanClosed:
		}
	}()
	return s, nil
}

func (s *rabbitSession) publish(ctx context.Context, body []byte) (confirmation, error) {
	return s.channel.PublishWithDeferredConfirmWithContext(
		ctx, SurfEventsExchange, SwellAlertRoutingKey, false, false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now().UTC(),
			Body:         body,
		},
	)
}

func (s *rabbitSession) done() <-chan struct{} { return s.lost }

func (s *rabbitSession) close() error {
	if err := s.connection.Close(); err != nil && !errors.Is(err, amqp.ErrClosed) {
		return fmt.Errorf("close RabbitMQ connection: %w", err)
	}
	return nil
}
