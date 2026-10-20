package service

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/omer-asraf/shaka-auth-service/internal/domain"
)

var (
	testKeyOnce sync.Once
	testKeyA    *rsa.PrivateKey
	testKeyB    *rsa.PrivateKey
)

func testKeys(t testing.TB) (*rsa.PrivateKey, *rsa.PrivateKey) {
	t.Helper()
	testKeyOnce.Do(func() {
		var err error
		if testKeyA, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
			panic(err)
		}
		if testKeyB, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
			panic(err)
		}
	})
	return testKeyA, testKeyB
}

func testKeySet(t testing.TB) *KeySet {
	t.Helper()
	a, _ := testKeys(t)
	set, err := NewKeySet(a)
	if err != nil {
		t.Fatalf("key set: %v", err)
	}
	return set
}

func issueFor(t *testing.T, manager *JWTManager) string {
	t.Helper()
	token, _, err := manager.Issue(&domain.User{ID: "user-1", Email: "surfer@example.com"})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	return token
}

func TestIssuedTokenIsRS256WithKidHeader(t *testing.T) {
	keys := testKeySet(t)
	manager, _ := NewJWTManager(keys, "test-issuer", time.Minute)
	raw := issueFor(t, manager)

	token, _, err := jwt.NewParser().ParseUnverified(raw, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, kid := keys.SigningKey()
	if token.Header["alg"] != "RS256" || token.Header["kid"] != kid {
		t.Fatalf("unexpected header: %v", token.Header)
	}
	if _, err := manager.Parse(raw); err != nil {
		t.Fatalf("own token must verify: %v", err)
	}
}

func TestParseRejectsWrongAlgorithmUnknownKidAndForeignKey(t *testing.T) {
	keys := testKeySet(t)
	manager, _ := NewJWTManager(keys, "test-issuer", time.Minute)
	_, otherKey := testKeys(t)
	claims := UserClaims{UserID: "u", RegisteredClaims: jwt.RegisteredClaims{
		Issuer: "test-issuer", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	}}
	_, kid := keys.SigningKey()

	foreign := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	foreign.Header["kid"] = kid // claims our kid but is signed by someone else
	foreignSigned, _ := foreign.SignedString(otherKey)

	unknown := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	unknown.Header["kid"] = "no-such-key"
	unknownSigned, _ := unknown.SignedString(otherKey)

	// Algorithm confusion: HS256 using the public modulus as the HMAC secret.
	hs := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	hs.Header["kid"] = kid
	hsSigned, _ := hs.SignedString(keys.signing.N.Bytes())

	none := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	none.Header["kid"] = kid
	noneSigned, _ := none.SignedString(jwt.UnsafeAllowNoneSignatureType)

	for name, raw := range map[string]string{
		"foreign signature": foreignSigned, "unknown kid": unknownSigned,
		"hs256 confusion": hsSigned, "alg none": noneSigned,
	} {
		if _, err := manager.Parse(raw); err == nil {
			t.Errorf("%s: token was accepted", name)
		}
	}
}

func TestRotationKeepsOldTokensValidAndPublishesBothKeys(t *testing.T) {
	a, b := testKeys(t)
	oldSet, _ := NewKeySet(a)
	oldManager, _ := NewJWTManager(oldSet, "test-issuer", time.Minute)
	oldToken := issueFor(t, oldManager)

	// Rotation: B signs from now on, A's public key stays published.
	newSet, err := NewKeySet(b, &a.PublicKey)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	newManager, _ := NewJWTManager(newSet, "test-issuer", time.Minute)
	if _, err := newManager.Parse(oldToken); err != nil {
		t.Fatalf("token signed before rotation must stay valid: %v", err)
	}
	newToken := issueFor(t, newManager)
	if _, err := oldManager.Parse(newToken); err == nil {
		t.Fatal("old key set should not know the new key")
	}

	jwks := newSet.JWKS()
	if len(jwks.Keys) != 2 {
		t.Fatalf("expected both keys published, got %d", len(jwks.Keys))
	}
	_, newKid := newSet.SigningKey()
	if jwks.Keys[0].KeyID != newKid {
		t.Fatal("the signing key must be listed first")
	}
	// Retiring A: once its tokens have expired, drop it from the set.
	retired, _ := NewKeySet(b)
	retiredManager, _ := NewJWTManager(retired, "test-issuer", time.Minute)
	if _, err := retiredManager.Parse(oldToken); err == nil {
		t.Fatal("a retired key must no longer verify tokens")
	}
}

func TestJWKSContainsOnlyPublicParameters(t *testing.T) {
	raw, err := json.Marshal(testKeySet(t).JWKS())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded struct {
		Keys []map[string]any `json:"keys"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil || len(decoded.Keys) != 1 {
		t.Fatalf("decode: %v %s", err, raw)
	}
	for field := range decoded.Keys[0] {
		switch field {
		case "kty", "use", "alg", "kid", "n", "e":
		default:
			t.Errorf("unexpected JWK field %q", field)
		}
	}
	if decoded.Keys[0]["alg"] != "RS256" || decoded.Keys[0]["e"] != "AQAB" {
		t.Fatalf("unexpected JWK: %v", decoded.Keys[0])
	}
}

func TestKeyIDIsStableAndKeySpecific(t *testing.T) {
	a, b := testKeys(t)
	if KeyID(&a.PublicKey) != KeyID(&a.PublicKey) || KeyID(&a.PublicKey) == KeyID(&b.PublicKey) {
		t.Fatal("kid must be deterministic and differ between keys")
	}
}

func pemOf(t *testing.T, blockType string, der []byte) string {
	t.Helper()
	return string(pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}))
}

func TestPEMParsingAcceptsCommonFormats(t *testing.T) {
	a, _ := testKeys(t)
	pkcs8, err := x509.MarshalPKCS8PrivateKey(a)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}
	pkix, _ := x509.MarshalPKIXPublicKey(&a.PublicKey)

	for name, text := range map[string]string{
		"pkcs8":              pemOf(t, "PRIVATE KEY", pkcs8),
		"pkcs1":              pemOf(t, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(a)),
		"single-line escape": strings.ReplaceAll(pemOf(t, "PRIVATE KEY", pkcs8), "\n", `\n`),
	} {
		key, err := ParsePrivateKeyPEM(text)
		if err != nil || key.N.Cmp(a.N) != 0 {
			t.Errorf("%s: %v", name, err)
		}
	}
	keys, err := ParsePublicKeysPEM(pemOf(t, "PUBLIC KEY", pkix) + pemOf(t, "RSA PUBLIC KEY", x509.MarshalPKCS1PublicKey(&a.PublicKey)))
	if err != nil || len(keys) != 2 {
		t.Fatalf("public keys: %v (%d)", err, len(keys))
	}
}

func TestPEMParsingRejectsGarbage(t *testing.T) {
	if _, err := ParsePrivateKeyPEM("not a key"); err == nil {
		t.Error("garbage private key accepted")
	}
	if _, err := ParsePrivateKeyPEM(pemOf(t, "CERTIFICATE", []byte("x"))); err == nil {
		t.Error("wrong PEM type accepted")
	}
	if _, err := ParsePublicKeysPEM(""); err == nil {
		t.Error("empty public key PEM accepted")
	}
}

func TestKeySetRejectsSmallKeys(t *testing.T) {
	small, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := NewKeySet(small); err == nil {
		t.Fatal("1024-bit signing key must be rejected")
	}
	a, _ := testKeys(t)
	if _, err := NewKeySet(a, &small.PublicKey); err == nil {
		t.Fatal("1024-bit previous key must be rejected")
	}
	if _, err := NewKeySet(nil); err == nil {
		t.Fatal("nil key must be rejected")
	}
}

func TestGenerateKeySetProducesUsableKey(t *testing.T) {
	set, err := GenerateKeySet()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	manager, _ := NewJWTManager(set, "test-issuer", time.Minute)
	if _, err := manager.Parse(issueFor(t, manager)); err != nil {
		t.Fatalf("generated key must work: %v", err)
	}
}
