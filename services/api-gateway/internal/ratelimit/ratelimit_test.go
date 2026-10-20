package ratelimit

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func TestTokenBucketBurstThenRefill(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_000, 0)}
	limiter := newTokenBucket(PerMinute(3, 6), clock.now) // 3 burst, 1 token / 10 s

	for i := 0; i < 3; i++ {
		if ok, _ := limiter.Allow("k"); !ok {
			t.Fatalf("attempt %d inside the burst was denied", i+1)
		}
	}
	ok, wait := limiter.Allow("k")
	if ok {
		t.Fatal("fourth attempt should be denied")
	}
	if wait < 9*time.Second || wait > 10*time.Second {
		t.Fatalf("expected ~10s retry-after, got %v", wait)
	}

	clock.t = clock.t.Add(10 * time.Second)
	if ok, _ := limiter.Allow("k"); !ok {
		t.Fatal("one token should have refilled after 10s")
	}
	if ok, _ := limiter.Allow("k"); ok {
		t.Fatal("only one token should have refilled")
	}
}

func TestTokenBucketKeysAreIndependent(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_000, 0)}
	limiter := newTokenBucket(PerMinute(1, 1), clock.now)
	if ok, _ := limiter.Allow("a"); !ok {
		t.Fatal("a should be allowed")
	}
	if ok, _ := limiter.Allow("a"); ok {
		t.Fatal("a should be limited")
	}
	if ok, _ := limiter.Allow("b"); !ok {
		t.Fatal("b must not be affected by a")
	}
}

func TestTokenBucketNeverExceedsBurstAfterLongIdle(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_000, 0)}
	limiter := newTokenBucket(PerMinute(2, 60), clock.now)
	limiter.Allow("k")
	clock.t = clock.t.Add(24 * time.Hour)
	allowed := 0
	for i := 0; i < 5; i++ {
		if ok, _ := limiter.Allow("k"); ok {
			allowed++
		}
	}
	if allowed != 2 {
		t.Fatalf("expected burst cap of 2 after idling, got %d", allowed)
	}
}

func TestTokenBucketPurgesIdleKeysAndBoundsMemory(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_000, 0)}
	limiter := newTokenBucket(PerMinute(1, 60), clock.now)
	limiter.maxKeys = 3
	for _, key := range []string{"a", "b", "c"} {
		limiter.Allow(key)
	}
	// Full: a new key is refused while the others are still depleted (fail closed).
	if ok, _ := limiter.Allow("d"); ok {
		t.Fatal("new key should be refused when the table is full of active buckets")
	}
	clock.t = clock.t.Add(2 * time.Second) // everything has refilled and can be purged
	if ok, _ := limiter.Allow("d"); !ok {
		t.Fatal("new key should be accepted after idle buckets are purged")
	}
	if len(limiter.buckets) > 3 {
		t.Fatalf("table grew past its cap: %d", len(limiter.buckets))
	}
}

func TestClientIP(t *testing.T) {
	cases := []struct {
		name   string
		remote string
		xff    []string
		hops   int
		want   string
	}{
		{"direct connection", "203.0.113.9:5555", nil, 0, "203.0.113.9"},
		{"forged header ignored without trusted proxy", "203.0.113.9:5555", []string{"1.2.3.4"}, 0, "203.0.113.9"},
		{"one trusted proxy uses last entry", "172.18.0.5:4000", []string{"198.51.100.7"}, 1, "198.51.100.7"},
		{"client-forged prefix is skipped", "172.18.0.5:4000", []string{"6.6.6.6, 198.51.100.7"}, 1, "198.51.100.7"},
		{"two hops", "172.18.0.5:4000", []string{"198.51.100.7, 10.0.0.2"}, 2, "198.51.100.7"},
		{"header missing falls back", "172.18.0.5:4000", nil, 1, "172.18.0.5"},
		{"too few entries falls back", "172.18.0.5:4000", []string{"198.51.100.7"}, 2, "172.18.0.5"},
		{"garbage falls back", "172.18.0.5:4000", []string{"not-an-ip"}, 1, "172.18.0.5"},
		{"ipv6", "[2001:db8::1]:80", nil, 0, "2001:db8::1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", nil)
			r.RemoteAddr = tc.remote
			for _, v := range tc.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			if got := ClientIP(r, tc.hops); got != tc.want {
				t.Fatalf("ClientIP = %q, want %q", got, tc.want)
			}
		})
	}
}

func newAuthChain(ipBurst, emailBurst, hops int) (http.Handler, *int) {
	clock := &fakeClock{t: time.Unix(1_000, 0)}
	calls := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Body", string(body))
		w.WriteHeader(http.StatusOK)
	})
	mw := AuthMiddleware(AuthConfig{
		IP:               newTokenBucket(PerMinute(ipBurst, 1), clock.now),
		Email:            newTokenBucket(PerMinute(emailBurst, 1), clock.now),
		TrustedProxyHops: hops,
	})
	return mw(next), &calls
}

func post(handler http.Handler, path, body, remote string, xff string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.RemoteAddr = remote
	if xff != "" {
		r.Header.Set("X-Forwarded-For", xff)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestMiddlewareLimitsPerIPWith429AndRetryAfter(t *testing.T) {
	handler, calls := newAuthChain(2, 100, 0)
	for i := 0; i < 2; i++ {
		body := `{"email":"u` + strconv.Itoa(i) + `@x.co","password":"p"}`
		if w := post(handler, "/api/v1/auth/login", body, "9.9.9.9:1", ""); w.Code != http.StatusOK {
			t.Fatalf("request %d: %d", i, w.Code)
		}
	}
	w := post(handler, "/api/v1/auth/login", `{"email":"u9@x.co"}`, "9.9.9.9:1", "")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}
	if seconds, err := strconv.Atoi(w.Header().Get("Retry-After")); err != nil || seconds < 1 {
		t.Fatalf("invalid Retry-After %q", w.Header().Get("Retry-After"))
	}
	if *calls != 2 {
		t.Fatalf("limited request must not reach the upstream, calls=%d", *calls)
	}
	if w := post(handler, "/api/v1/auth/login", `{"email":"u9@x.co"}`, "8.8.8.8:1", ""); w.Code != http.StatusOK {
		t.Fatalf("another IP must not be limited, got %d", w.Code)
	}
}

func TestMiddlewareLimitsPerEmailAcrossIPsCaseInsensitively(t *testing.T) {
	handler, _ := newAuthChain(100, 2, 0)
	emails := []string{"Victim@x.co", "victim@X.co"}
	for i, email := range emails {
		remote := "7.7.7." + strconv.Itoa(i+1) + ":1"
		if w := post(handler, "/api/v1/auth/login", `{"email":"`+email+`"}`, remote, ""); w.Code != http.StatusOK {
			t.Fatalf("attempt %d: %d", i, w.Code)
		}
	}
	w := post(handler, "/api/v1/auth/login", ` {"email":" VICTIM@x.co "}`, "7.7.7.3:1", "")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("distributed guessing against one email must be limited, got %d", w.Code)
	}
}

func TestMiddlewareForwardsBodyUnchanged(t *testing.T) {
	handler, _ := newAuthChain(5, 5, 0)
	body := `{"email":"a@b.co","password":"correct horse battery"}`
	w := post(handler, "/api/v1/auth/register", body, "1.1.1.1:1", "")
	if got := w.Header().Get("X-Body"); got != body {
		t.Fatalf("upstream saw %q", got)
	}
}

func TestMiddlewareLoginAndRegisterHaveSeparateBuckets(t *testing.T) {
	handler, _ := newAuthChain(1, 1, 0)
	body := `{"email":"a@b.co"}`
	if w := post(handler, "/api/v1/auth/login", body, "1.1.1.1:1", ""); w.Code != http.StatusOK {
		t.Fatal("login should pass")
	}
	if w := post(handler, "/api/v1/auth/register", body, "1.1.1.1:1", ""); w.Code != http.StatusOK {
		t.Fatal("register has its own bucket")
	}
}

func TestMiddlewareIgnoresOtherRoutesAndMethods(t *testing.T) {
	handler, calls := newAuthChain(1, 1, 0)
	for i := 0; i < 5; i++ {
		post(handler, "/api/v1/auth/refresh", `{}`, "1.1.1.1:1", "")
		post(handler, "/api/v1/beaches", `{}`, "1.1.1.1:1", "")
		r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil)
		handler.ServeHTTP(httptest.NewRecorder(), r)
	}
	if *calls != 15 {
		t.Fatalf("unlimited routes must all pass, calls=%d", *calls)
	}
}

func TestMiddlewareUnparseableBodyStillIPLimited(t *testing.T) {
	handler, _ := newAuthChain(1, 1, 0)
	if w := post(handler, "/api/v1/auth/login", `not json`, "1.1.1.1:1", ""); w.Code != http.StatusOK {
		t.Fatal("first request should pass through to the upstream validator")
	}
	if w := post(handler, "/api/v1/auth/login", `not json`, "1.1.1.1:1", ""); w.Code != http.StatusTooManyRequests {
		t.Fatalf("garbage must still count against the IP, got %d", w.Code)
	}
}

func TestMiddlewareSpoofedForwardedForDoesNotEvadeLimitWithoutTrustedProxy(t *testing.T) {
	handler, _ := newAuthChain(1, 100, 0)
	post(handler, "/api/v1/auth/login", `{"email":"a@b.co"}`, "5.5.5.5:1", "1.1.1.1")
	w := post(handler, "/api/v1/auth/login", `{"email":"c@d.co"}`, "5.5.5.5:1", "2.2.2.2")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("rotating X-Forwarded-For must not evade the limit, got %d", w.Code)
	}
}

func TestMiddlewareUsesForwardedForBehindTrustedProxy(t *testing.T) {
	handler, _ := newAuthChain(1, 100, 1)
	// Both requests arrive from the proxy (172.18.0.5) but for different clients.
	if w := post(handler, "/api/v1/auth/login", `{"email":"a@b.co"}`, "172.18.0.5:1", "1.1.1.1"); w.Code != http.StatusOK {
		t.Fatal("client 1 should pass")
	}
	if w := post(handler, "/api/v1/auth/login", `{"email":"a@b.co"}`, "172.18.0.5:2", "2.2.2.2"); w.Code != http.StatusOK {
		t.Fatal("client 2 must have its own bucket")
	}
	if w := post(handler, "/api/v1/auth/login", `{"email":"a@b.co"}`, "172.18.0.5:3", "9.9.9.9, 1.1.1.1"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("client 1 again (with a forged prefix) must be limited, got %d", w.Code)
	}
}
