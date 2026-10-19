package consumer

import (
	"context"
	"errors"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestRunWithReconnectBackoff(t *testing.T) {
	boom := errors.New("connection lost")
	tests := []struct {
		name       string
		results    []error // result of each run; ctx is canceled during the last one
		runtimes   []time.Duration
		wantDelays []time.Duration
	}{
		{
			name:       "doubles then caps",
			results:    []error{boom, boom, boom, boom, boom},
			runtimes:   []time.Duration{0, 0, 0, 0, 0},
			wantDelays: []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 4 * time.Second},
		},
		{
			name:       "healthy session resets backoff",
			results:    []error{boom, boom, boom, boom},
			runtimes:   []time.Duration{0, 0, 2 * time.Minute, 0},
			wantDelays: []time.Duration{time.Second, 2 * time.Second, time.Second},
		},
		{
			name:       "clean return stops without retry",
			results:    []error{nil},
			runtimes:   []time.Duration{0},
			wantDelays: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var clock time.Time
			calls := 0
			run := func(context.Context) error {
				i := calls
				calls++
				clock = clock.Add(tt.runtimes[i])
				if i == len(tt.results)-1 {
					cancel()
				}
				return tt.results[i]
			}
			var delays []time.Duration
			sleep := func(_ context.Context, d time.Duration) bool {
				delays = append(delays, d)
				return true
			}
			err := runWithReconnect(ctx, run, func(string, ...any) {}, time.Second, 4*time.Second,
				func() time.Time { return clock }, sleep)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if calls != len(tt.results) {
				t.Fatalf("run called %d times, want %d", calls, len(tt.results))
			}
			if len(delays) != len(tt.wantDelays) {
				t.Fatalf("delays %v, want %v", delays, tt.wantDelays)
			}
			for i := range delays {
				if delays[i] != tt.wantDelays[i] {
					t.Fatalf("delays %v, want %v", delays, tt.wantDelays)
				}
			}
		})
	}
}

func TestRunWithReconnectStopsDuringSleep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	err := runWithReconnect(ctx,
		func(context.Context) error { calls++; return errors.New("lost") },
		func(string, ...any) {}, time.Second, time.Second, time.Now,
		func(context.Context, time.Duration) bool { cancel(); return false })
	if err != nil || calls != 1 {
		t.Fatalf("err=%v calls=%d, want nil and 1", err, calls)
	}
}

type fakeDeclarer struct {
	exchanges []string
	queues    []string
	binds     [][2]string // queue, exchange
	failOn    string
}

func (f *fakeDeclarer) ExchangeDeclare(name, _ string, _, _, _, _ bool, _ amqp.Table) error {
	if f.failOn == "exchange" {
		return errors.New("fail")
	}
	f.exchanges = append(f.exchanges, name)
	return nil
}

func (f *fakeDeclarer) QueueDeclare(name string, _, _, _, _ bool, _ amqp.Table) (amqp.Queue, error) {
	if f.failOn == "queue" {
		return amqp.Queue{}, errors.New("fail")
	}
	f.queues = append(f.queues, name)
	return amqp.Queue{Name: name}, nil
}

func (f *fakeDeclarer) QueueBind(name, _, exchange string, _ bool, _ amqp.Table) error {
	if f.failOn == "bind" {
		return errors.New("fail")
	}
	f.binds = append(f.binds, [2]string{name, exchange})
	return nil
}

func TestDeclareDeadLetterTopology(t *testing.T) {
	f := &fakeDeclarer{}
	if err := declareDeadLetterTopology(f); err != nil {
		t.Fatal(err)
	}
	if len(f.exchanges) != 1 || f.exchanges[0] != DeadLetterExchange ||
		len(f.queues) != 1 || f.queues[0] != DeadLetterQueue ||
		len(f.binds) != 1 || f.binds[0] != [2]string{DeadLetterQueue, DeadLetterExchange} {
		t.Fatalf("unexpected topology: %+v", f)
	}
	for _, step := range []string{"exchange", "queue", "bind"} {
		if err := declareDeadLetterTopology(&fakeDeclarer{failOn: step}); err == nil {
			t.Errorf("expected error when %s declaration fails", step)
		}
	}
}
