package authn

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// The jwks tests live in package authn (internal) to control the client's clock.
// authntest imports authn, so it cannot be imported here; build documents by hand.

func jwksFixture(t *testing.T, kids ...string) (doc func() []byte, signers map[string]*testSigner) {
	t.Helper()
	signers = map[string]*testSigner{}
	for _, kid := range kids {
		signers[kid] = newTestSigner(t, kid)
	}
	return func() []byte {
		var list []*testSigner
		for _, kid := range kids {
			list = append(list, signers[kid])
		}
		return jwksDocument(t, list...)
	}, signers
}

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

func newTestClient(url string, clock *fakeClock) *JWKSClient {
	client := NewJWKSClient(url)
	client.now = clock.Now
	return client
}

func TestJWKSClientFetchesCachesAndRefreshesOnTTL(t *testing.T) {
	doc, signers := jwksFixture(t, "kid-1")
	var fetches atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetches.Add(1)
		_, _ = w.Write(doc())
	}))
	defer server.Close()
	clock := &fakeClock{now: time.Unix(10_000, 0)}
	client := newTestClient(server.URL, clock)

	for i := 0; i < 3; i++ {
		key, err := client.Key("kid-1")
		if err != nil || key.N.Cmp(signers["kid-1"].key.N) != 0 {
			t.Fatalf("lookup %d: %v", i, err)
		}
	}
	if fetches.Load() != 1 {
		t.Fatalf("expected one fetch while the cache is fresh, got %d", fetches.Load())
	}
	clock.now = clock.now.Add(6 * time.Minute) // past the TTL
	if _, err := client.Key("kid-1"); err != nil {
		t.Fatalf("lookup after ttl: %v", err)
	}
	if fetches.Load() != 2 {
		t.Fatalf("expected a refresh after the TTL, got %d fetches", fetches.Load())
	}
}

func TestJWKSClientPicksUpRotatedKeyOnUnknownKid(t *testing.T) {
	doc, signers := jwksFixture(t, "old")
	var current atomic.Value
	current.Store(doc())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(current.Load().([]byte))
	}))
	defer server.Close()
	clock := &fakeClock{now: time.Unix(10_000, 0)}
	client := newTestClient(server.URL, clock)
	if _, err := client.Key("old"); err != nil {
		t.Fatalf("old key: %v", err)
	}

	// auth-service rotates: publishes new + old.
	signers["new"] = newTestSigner(t, "new")
	current.Store(jwksDocument(t, signers["new"], signers["old"]))
	clock.now = clock.now.Add(11 * time.Second) // beyond the min-refresh spacing
	if _, err := client.Key("new"); err != nil {
		t.Fatalf("rotated key must be discovered without a restart: %v", err)
	}
	if _, err := client.Key("old"); err != nil {
		t.Fatalf("old key must still verify during the overlap: %v", err)
	}

	// Later the old key is retired from the JWKS and disappears after refresh.
	current.Store(jwksDocument(t, signers["new"]))
	clock.now = clock.now.Add(6 * time.Minute)
	if _, err := client.Key("new"); err != nil {
		t.Fatalf("new key: %v", err)
	}
	if _, err := client.Key("old"); err == nil {
		t.Fatal("a retired key must stop verifying once the JWKS drops it")
	}
}

func TestJWKSClientThrottlesRefreshesForForgedKids(t *testing.T) {
	doc, _ := jwksFixture(t, "kid-1")
	var fetches atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fetches.Add(1)
		_, _ = w.Write(doc())
	}))
	defer server.Close()
	clock := &fakeClock{now: time.Unix(10_000, 0)}
	client := newTestClient(server.URL, clock)
	if _, err := client.Key("kid-1"); err != nil {
		t.Fatalf("initial: %v", err)
	}
	before := fetches.Load()
	for i := 0; i < 50; i++ {
		if _, err := client.Key("forged-kid"); err == nil {
			t.Fatal("forged kid accepted")
		}
	}
	if extra := fetches.Load() - before; extra > 1 {
		t.Fatalf("forged kids caused %d upstream fetches, want at most 1 per window", extra)
	}
}

func TestJWKSClientKeepsServingCachedKeysWhenAuthServiceIsDown(t *testing.T) {
	doc, _ := jwksFixture(t, "kid-1")
	var down atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if down.Load() {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(doc())
	}))
	defer server.Close()
	clock := &fakeClock{now: time.Unix(10_000, 0)}
	client := newTestClient(server.URL, clock)
	if _, err := client.Key("kid-1"); err != nil {
		t.Fatalf("initial: %v", err)
	}
	down.Store(true)
	clock.now = clock.now.Add(time.Hour)
	if _, err := client.Key("kid-1"); err != nil {
		t.Fatalf("stale cached key must keep working during an outage: %v", err)
	}
}

func TestJWKSClientFailsClosedWithoutAnyKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client := newTestClient(server.URL, &fakeClock{now: time.Unix(10_000, 0)})
	if _, err := client.Key("kid-1"); err == nil {
		t.Fatal("expected an error when no key was ever loaded")
	}
	if _, err := client.Key(""); err == nil {
		t.Fatal("expected an error for a token without kid")
	}
	if err := client.Prefetch(context.Background()); err == nil {
		t.Fatal("prefetch should report the failure")
	}
}

func TestParseJWKSIgnoresUnusableEntries(t *testing.T) {
	good := newTestSigner(t, "good")
	raw := []byte(`{"keys":[
		{"kty":"EC","kid":"ec","use":"sig","alg":"ES256","n":"","e":""},
		{"kty":"RSA","kid":"enc","use":"enc","alg":"RS256","n":"AQAB","e":"AQAB"},
		{"kty":"RSA","kid":"hs","use":"sig","alg":"HS256","n":"AQAB","e":"AQAB"},
		{"kty":"RSA","kid":"tiny","use":"sig","alg":"RS256","n":"AQAB","e":"AQAB"},
		{"kty":"RSA","use":"sig","alg":"RS256","n":"AQAB","e":"AQAB"}
	]}`)
	if _, err := ParseJWKS(raw); err == nil {
		t.Fatal("a JWKS with no usable keys must be an error")
	}
	keys, err := ParseJWKS(jwksDocument(t, good))
	if err != nil || len(keys) != 1 || keys["good"] == nil {
		t.Fatalf("expected the good key, got %v (%v)", keys, err)
	}
	if _, err := ParseJWKS([]byte("not json")); err == nil {
		t.Fatal("invalid JSON accepted")
	}
}

func TestMiddlewareVerifiesTokensFromJWKSClientEndToEnd(t *testing.T) {
	doc, signers := jwksFixture(t, "kid-1")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(doc())
	}))
	defer server.Close()
	middleware, err := NewMiddleware(NewJWKSClient(server.URL), "test-issuer")
	if err != nil {
		t.Fatalf("middleware: %v", err)
	}
	token := signers["kid-1"].token(t, Claims{
		UserID: "u1",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "test-issuer", Subject: "u1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	})
	claims, err := middleware.parse(token)
	if err != nil || claims.UserID != "u1" {
		t.Fatalf("token signed by the published key must verify: %v", err)
	}
}
