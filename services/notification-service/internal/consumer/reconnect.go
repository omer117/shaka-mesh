package consumer

import (
	"context"
	"time"
)

const (
	reconnectBaseDelay = time.Second
	reconnectMaxDelay  = 30 * time.Second
	// A session that stayed up this long counts as healthy and resets the backoff.
	healthySessionAfter = time.Minute
)

// RunWithReconnect runs the consumer until ctx is canceled. When the broker
// connection is lost (Run returns an error) it waits with exponential backoff
// and starts again, re-declaring the topology each time.
func (c *Consumer) RunWithReconnect(ctx context.Context) error {
	return runWithReconnect(ctx, c.Run, c.logger.Printf, reconnectBaseDelay, reconnectMaxDelay, time.Now, sleepCtx)
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func runWithReconnect(
	ctx context.Context,
	run func(context.Context) error,
	logf func(string, ...any),
	base, max time.Duration,
	now func() time.Time,
	sleep func(context.Context, time.Duration) bool,
) error {
	delay := base
	for {
		started := now()
		err := run(ctx)
		if ctx.Err() != nil || err == nil {
			return nil
		}
		if now().Sub(started) >= healthySessionAfter {
			delay = base
		}
		logf("notification consumer stopped unexpectedly: %v; reconnecting in %s", err, delay)
		if !sleep(ctx, delay) {
			return nil
		}
		delay *= 2
		if delay > max {
			delay = max
		}
	}
}
