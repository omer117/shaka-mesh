package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func newTestKey(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	return key, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func clearKeyEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"JWT_PRIVATE_KEY", "JWT_PRIVATE_KEY_FILE", "JWT_PREVIOUS_PUBLIC_KEYS", "JWT_PREVIOUS_PUBLIC_KEYS_FILE",
	} {
		t.Setenv(name, "")
	}
}

func TestLoadKeySetGeneratesDevKeyWhenUnconfigured(t *testing.T) {
	clearKeyEnv(t)
	keys, err := loadKeySet()
	if err != nil {
		t.Fatalf("expected a generated dev key, got %v", err)
	}
	if _, kid := keys.SigningKey(); kid == "" {
		t.Fatal("generated key has no kid")
	}
}

func TestLoadKeySetFromEnvAndFile(t *testing.T) {
	key, text := newTestKey(t)

	clearKeyEnv(t)
	t.Setenv("JWT_PRIVATE_KEY", text)
	fromEnv, err := loadKeySet()
	if err != nil {
		t.Fatalf("env key: %v", err)
	}

	clearKeyEnv(t)
	path := filepath.Join(t.TempDir(), "jwt.pem")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}
	t.Setenv("JWT_PRIVATE_KEY_FILE", path)
	fromFile, err := loadKeySet()
	if err != nil {
		t.Fatalf("file key: %v", err)
	}

	got, _ := fromEnv.SigningKey()
	gotFile, _ := fromFile.SigningKey()
	if got.N.Cmp(key.N) != 0 || gotFile.N.Cmp(key.N) != 0 {
		t.Fatal("loaded key does not match the configured one")
	}
}

func TestLoadKeySetPublishesPreviousKeysAfterRotation(t *testing.T) {
	_, current := newTestKey(t)
	old, _ := newTestKey(t)
	pkix, _ := x509.MarshalPKIXPublicKey(&old.PublicKey)

	clearKeyEnv(t)
	t.Setenv("JWT_PRIVATE_KEY", current)
	t.Setenv("JWT_PREVIOUS_PUBLIC_KEYS", string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pkix})))
	keys, err := loadKeySet()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := len(keys.JWKS().Keys); got != 2 {
		t.Fatalf("expected current and previous keys published, got %d", got)
	}
}

func TestLoadKeySetRejectsBadConfiguration(t *testing.T) {
	_, text := newTestKey(t)

	clearKeyEnv(t)
	t.Setenv("JWT_PRIVATE_KEY", "garbage")
	if _, err := loadKeySet(); err == nil {
		t.Error("garbage private key accepted")
	}

	clearKeyEnv(t)
	t.Setenv("JWT_PRIVATE_KEY_FILE", filepath.Join(t.TempDir(), "missing.pem"))
	if _, err := loadKeySet(); err == nil {
		t.Error("missing key file accepted")
	}

	clearKeyEnv(t)
	t.Setenv("JWT_PREVIOUS_PUBLIC_KEYS", "irrelevant")
	if _, err := loadKeySet(); err == nil {
		t.Error("previous keys without a signing key accepted")
	}

	clearKeyEnv(t)
	t.Setenv("JWT_PRIVATE_KEY", text)
	t.Setenv("JWT_PREVIOUS_PUBLIC_KEYS", "garbage")
	if _, err := loadKeySet(); err == nil {
		t.Error("garbage previous keys accepted")
	}
}
