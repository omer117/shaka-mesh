package profile

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalBlobStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	store, err := NewLocalBlobStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	key, err := NewAvatarKey()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, key, []byte("data")); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, key, []byte("again")); err == nil {
		t.Fatal("overwriting an existing key must fail")
	}
	f, _, err := store.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(f)
	f.Close()
	if string(got) != "data" {
		t.Fatalf("got %q", got)
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Open(ctx, key); !errors.Is(err, ErrBlobNotFound) {
		t.Fatalf("got %v, want ErrBlobNotFound", err)
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("deleting a missing blob should be a no-op: %v", err)
	}
}

func TestLocalBlobStoreRejectsUnsafeKeys(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "avatars")
	store, err := NewLocalBlobStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	secret := filepath.Join(parent, "secret.jpg")
	if err := os.WriteFile(secret, []byte("top secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	keys := []string{
		"../secret.jpg", "..\\secret.jpg", "/etc/passwd", "a/b.jpg", "",
		"%2e%2e%2fsecret.jpg", "avatar.jpg", "0123456789abcdef0123456789abcdef.png",
		"0123456789ABCDEF0123456789ABCDEF.jpg", "0123456789abcdef0123456789abcdef.jpg/",
	}
	for _, key := range keys {
		if err := store.Put(context.Background(), key, []byte("x")); err == nil {
			t.Errorf("Put(%q) should fail", key)
		}
		if _, _, err := store.Open(context.Background(), key); !errors.Is(err, ErrBlobNotFound) {
			t.Errorf("Open(%q) = %v, want ErrBlobNotFound", key, err)
		}
	}
	if b, _ := os.ReadFile(secret); string(b) != "top secret" {
		t.Fatal("file outside the store was modified")
	}
}

func TestNewAvatarKeyIsRandomAndValid(t *testing.T) {
	a, _ := NewAvatarKey()
	b, _ := NewAvatarKey()
	if a == b || !ValidAvatarKey(a) || !ValidAvatarKey(b) {
		t.Fatalf("bad keys %q %q", a, b)
	}
}
