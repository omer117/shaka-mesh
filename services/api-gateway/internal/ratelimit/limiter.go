// Package ratelimit provides brute-force protection for the auth endpoints.
//
// Limiter is an interface so the in-memory token bucket can later be replaced
// by a shared store (for example Redis) when the gateway runs as several
// replicas.
package ratelimit

import (
	"math"
	"sync"
	"time"
)

// Limiter decides whether an action identified by key may proceed now.
type Limiter interface {
	// Allow consumes one token for key. When it returns false, retryAfter is how
	// long the caller should wait before a token is available again.
	Allow(key string) (allowed bool, retryAfter time.Duration)
}

// Config describes a token bucket: it holds up to Burst tokens and refills
// Rate tokens per second.
type Config struct {
	Burst float64
	Rate  float64
}

// PerMinute builds a Config that allows burst attempts at once and then
// perMinute attempts per minute.
func PerMinute(burst int, perMinute float64) Config {
	return Config{Burst: float64(burst), Rate: perMinute / 60}
}

const (
	defaultMaxKeys      = 100_000
	defaultPurgeEvery   = time.Minute
	denyWhenFullRetry   = time.Second
	minimumRetryAfterNs = int64(time.Millisecond)
)

type bucket struct {
	tokens float64
	last   time.Time
}

// TokenBucket is an in-memory Limiter. It is safe for concurrent use.
type TokenBucket struct {
	cfg Config
	now func() time.Time

	mu        sync.Mutex
	buckets   map[string]*bucket
	maxKeys   int
	lastPurge time.Time
}

// NewTokenBucket returns a limiter with the given configuration.
func NewTokenBucket(cfg Config) *TokenBucket {
	return newTokenBucket(cfg, time.Now)
}

func newTokenBucket(cfg Config, now func() time.Time) *TokenBucket {
	if cfg.Burst < 1 {
		cfg.Burst = 1
	}
	if cfg.Rate <= 0 {
		cfg.Rate = 1.0 / 60
	}
	return &TokenBucket{
		cfg:       cfg,
		now:       now,
		buckets:   make(map[string]*bucket),
		maxKeys:   defaultMaxKeys,
		lastPurge: now(),
	}
}

func (l *TokenBucket) Allow(key string) (bool, time.Duration) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	if now.Sub(l.lastPurge) >= defaultPurgeEvery {
		l.purgeLocked(now)
	}
	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) >= l.maxKeys {
			l.purgeLocked(now)
			if len(l.buckets) >= l.maxKeys {
				// Fail closed: refusing is safer than letting an attacker who
				// floods unique keys bypass the limit. Memory stays bounded.
				return false, denyWhenFullRetry
			}
		}
		b = &bucket{tokens: l.cfg.Burst, last: now}
		l.buckets[key] = b
	}

	if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens = math.Min(l.cfg.Burst, b.tokens+elapsed*l.cfg.Rate)
	}
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := time.Duration((1 - b.tokens) / l.cfg.Rate * float64(time.Second))
	if int64(wait) < minimumRetryAfterNs {
		wait = time.Duration(minimumRetryAfterNs)
	}
	return false, wait
}

// purgeLocked drops buckets that have fully refilled; they are equivalent to
// a brand-new bucket, so forgetting them changes nothing.
func (l *TokenBucket) purgeLocked(now time.Time) {
	l.lastPurge = now
	for key, b := range l.buckets {
		refilled := b.tokens + now.Sub(b.last).Seconds()*l.cfg.Rate
		if refilled >= l.cfg.Burst {
			delete(l.buckets, key)
		}
	}
}
