// Package authntest provides RS256 signing helpers for tests of code that
// verifies auth-service tokens. It is only imported from _test files.
package authntest

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"sync"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/omer-asraf/shaka-api-gateway/internal/authn"
)

// Signer signs tokens the way auth-service does.
type Signer struct {
	Key *rsa.PrivateKey
	KID string
}

var (
	sharedOnce sync.Once
	shared     *rsa.PrivateKey
)

// NewSigner returns a signer with a process-wide cached 2048-bit key (key
// generation is slow) under the given kid.
func NewSigner(t testing.TB, kid string) *Signer {
	t.Helper()
	sharedOnce.Do(func() {
		var err error
		if shared, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
			panic(err)
		}
	})
	return &Signer{Key: shared, KID: kid}
}

// Generate returns a signer with a brand-new key, for "attacker" or rotation tests.
func Generate(t testing.TB, kid string) *Signer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return &Signer{Key: key, KID: kid}
}

// Keys returns a KeySource that trusts this signer.
func (s *Signer) Keys() authn.StaticKeys {
	return authn.StaticKeys{s.KID: &s.Key.PublicKey}
}

// Token signs claims with RS256 and the signer's kid.
func (s *Signer) Token(t testing.TB, claims jwt.Claims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = s.KID
	signed, err := token.SignedString(s.Key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

// JWKS renders the signer's public key as a JWKS document, like auth-service.
func JWKS(t testing.TB, signers ...*Signer) []byte {
	t.Helper()
	type jwk struct {
		KeyType   string `json:"kty"`
		Use       string `json:"use"`
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
		N         string `json:"n"`
		E         string `json:"e"`
	}
	var document struct {
		Keys []jwk `json:"keys"`
	}
	for _, s := range signers {
		document.Keys = append(document.Keys, jwk{
			KeyType: "RSA", Use: "sig", Algorithm: "RS256", KeyID: s.KID,
			N: base64.RawURLEncoding.EncodeToString(s.Key.N.Bytes()),
			E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(s.Key.E)).Bytes()),
		})
	}
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal JWKS: %v", err)
	}
	return raw
}
