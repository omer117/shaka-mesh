package authn

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

// Internal-test copies of the authntest helpers (authntest imports authn, so
// package-internal tests cannot import it without a cycle).

type testSigner struct {
	key *rsa.PrivateKey
	kid string
}

func newTestSigner(t testing.TB, kid string) *testSigner {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return &testSigner{key: key, kid: kid}
}

func (s *testSigner) token(t testing.TB, claims jwt.Claims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = s.kid
	signed, err := token.SignedString(s.key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signed
}

func jwksDocument(t testing.TB, signers ...*testSigner) []byte {
	t.Helper()
	type entry struct {
		KeyType   string `json:"kty"`
		Use       string `json:"use"`
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
		N         string `json:"n"`
		E         string `json:"e"`
	}
	var document struct {
		Keys []entry `json:"keys"`
	}
	for _, s := range signers {
		document.Keys = append(document.Keys, entry{
			KeyType: "RSA", Use: "sig", Algorithm: "RS256", KeyID: s.kid,
			N: base64.RawURLEncoding.EncodeToString(s.key.N.Bytes()),
			E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(s.key.E)).Bytes()),
		})
	}
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}
