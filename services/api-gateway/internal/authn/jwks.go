package authn

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// KeySource resolves the public key that verifies a token, by its kid header.
type KeySource interface {
	Key(kid string) (*rsa.PublicKey, error)
}

// StaticKeys is a fixed KeySource, useful for tests and single-key setups.
type StaticKeys map[string]*rsa.PublicKey

func (s StaticKeys) Key(kid string) (*rsa.PublicKey, error) {
	if key, ok := s[kid]; ok {
		return key, nil
	}
	return nil, fmt.Errorf("unknown signing key %q", kid)
}

const (
	defaultJWKSTTL        = 5 * time.Minute
	defaultMinRefresh     = 10 * time.Second
	defaultFetchTimeout   = 5 * time.Second
	maxJWKSResponseBytes  = 1 << 20
	jwkAlgorithm          = "RS256"
	maxPublishedKeysAllow = 32
)

// JWKSClient fetches auth-service's public keys and caches them.
//
//   - Keys are refreshed after ttl.
//   - A token with an unknown kid triggers an immediate refresh (that is how a
//     key rotation is picked up without a restart), but never more often than
//     once per minRefresh, so a flood of forged kids cannot turn the gateway
//     into a request amplifier against auth-service.
//   - If a refresh fails, keys already cached keep working: a brief auth-service
//     outage must not log everyone out.
type JWKSClient struct {
	url        string
	httpClient *http.Client
	ttl        time.Duration
	minRefresh time.Duration
	now        func() time.Time

	mu          sync.Mutex
	keys        map[string]*rsa.PublicKey
	fetchedAt   time.Time
	lastAttempt time.Time
}

// NewJWKSClient creates a client for the given JWKS URL.
func NewJWKSClient(url string) *JWKSClient {
	return &JWKSClient{
		url:        url,
		httpClient: &http.Client{Timeout: defaultFetchTimeout},
		ttl:        defaultJWKSTTL,
		minRefresh: defaultMinRefresh,
		now:        time.Now,
		keys:       map[string]*rsa.PublicKey{},
	}
}

// Prefetch loads the keys once at startup. Failure is not fatal (auth-service
// may still be starting); keys are then fetched on first use.
func (c *JWKSClient) Prefetch(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.refreshLocked(ctx)
}

func (c *JWKSClient) Key(kid string) (*rsa.PublicKey, error) {
	if kid == "" {
		return nil, errors.New("token has no kid header")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	key, found := c.keys[kid]
	if found && now.Sub(c.fetchedAt) < c.ttl {
		return key, nil
	}
	if now.Sub(c.lastAttempt) >= c.minRefresh {
		ctx, cancel := context.WithTimeout(context.Background(), defaultFetchTimeout)
		defer cancel()
		if err := c.refreshLocked(ctx); err != nil {
			log.Printf("refresh JWKS: %v", err)
		}
	}
	if key, found = c.keys[kid]; found {
		return key, nil
	}
	return nil, fmt.Errorf("unknown signing key %q", kid)
}

func (c *JWKSClient) refreshLocked(ctx context.Context) error {
	c.lastAttempt = c.now()
	keys, err := c.fetch(ctx)
	if err != nil {
		return err
	}
	c.keys = keys
	c.fetchedAt = c.lastAttempt
	return nil
}

func (c *JWKSClient) fetch(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return nil, err
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", c.url, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: status %d", c.url, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxJWKSResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read JWKS: %w", err)
	}
	return ParseJWKS(body)
}

type jwkDocument struct {
	Keys []struct {
		KeyType   string `json:"kty"`
		Use       string `json:"use"`
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
		N         string `json:"n"`
		E         string `json:"e"`
	} `json:"keys"`
}

// ParseJWKS extracts the RS256 signing keys from a JWKS document. Entries of
// other types, algorithms or uses are skipped; an empty result is an error so a
// broken response never wipes the cache.
func ParseJWKS(raw []byte) (map[string]*rsa.PublicKey, error) {
	var document jwkDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("decode JWKS: %w", err)
	}
	keys := make(map[string]*rsa.PublicKey)
	for _, entry := range document.Keys {
		if entry.KeyType != "RSA" || entry.KeyID == "" ||
			(entry.Use != "" && entry.Use != "sig") ||
			(entry.Algorithm != "" && entry.Algorithm != jwkAlgorithm) {
			continue
		}
		n, err := base64.RawURLEncoding.DecodeString(entry.N)
		if err != nil {
			continue
		}
		e, err := base64.RawURLEncoding.DecodeString(entry.E)
		if err != nil || len(e) == 0 || len(e) > 4 {
			continue
		}
		exponent := int(new(big.Int).SetBytes(e).Int64())
		modulus := new(big.Int).SetBytes(n)
		if exponent < 3 || modulus.BitLen() < 2048 {
			continue
		}
		keys[entry.KeyID] = &rsa.PublicKey{N: modulus, E: exponent}
		if len(keys) >= maxPublishedKeysAllow {
			break
		}
	}
	if len(keys) == 0 {
		return nil, errors.New("JWKS contains no usable RS256 keys")
	}
	return keys, nil
}
