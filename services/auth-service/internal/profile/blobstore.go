package profile

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"time"
)

// ErrBlobNotFound is returned by Open when the key does not exist.
var ErrBlobNotFound = errors.New("blob not found")

// BlobFile is an opened blob that supports range requests.
type BlobFile interface {
	io.ReadSeekCloser
}

// BlobStore stores immutable binary objects under server-generated keys.
//
// TODO: add an OCI Object Storage implementation (Always Free tier) and select
// it with an env var; the interface is deliberately small so it maps onto
// PutObject / GetObject / DeleteObject.
type BlobStore interface {
	Put(ctx context.Context, key string, data []byte) error
	Open(ctx context.Context, key string) (BlobFile, time.Time, error)
	Delete(ctx context.Context, key string) error
}

// avatarKeyPattern is the only shape of key we ever create or serve: 32 random
// hex characters plus the fixed extension. Anything else is rejected, which
// makes user-controlled paths and traversal impossible.
var avatarKeyPattern = regexp.MustCompile(`^[a-f0-9]{32}\.jpg$`)

// ValidAvatarKey reports whether key has the exact shape NewAvatarKey produces.
func ValidAvatarKey(key string) bool { return avatarKeyPattern.MatchString(key) }

// NewAvatarKey returns a fresh random key such as "3f2a...c9.jpg".
func NewAvatarKey() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate avatar key: %w", err)
	}
	return hex.EncodeToString(buf) + ".jpg", nil
}

// LocalBlobStore keeps blobs in one directory, confined with os.Root so even a
// buggy key can never escape it.
type LocalBlobStore struct{ root *os.Root }

func NewLocalBlobStore(dir string) (*LocalBlobStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create blob directory: %w", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open blob directory: %w", err)
	}
	return &LocalBlobStore{root: root}, nil
}

// Close releases the directory handle.
func (s *LocalBlobStore) Close() error { return s.root.Close() }

func (s *LocalBlobStore) Put(_ context.Context, key string, data []byte) error {
	if !ValidAvatarKey(key) {
		return fmt.Errorf("invalid blob key %q", key)
	}
	f, err := s.root.OpenFile(key, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create blob: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		_ = s.root.Remove(key)
		return fmt.Errorf("write blob: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = s.root.Remove(key)
		return fmt.Errorf("close blob: %w", err)
	}
	return nil
}

func (s *LocalBlobStore) Open(_ context.Context, key string) (BlobFile, time.Time, error) {
	if !ValidAvatarKey(key) {
		return nil, time.Time{}, ErrBlobNotFound
	}
	f, err := s.root.Open(key)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, time.Time{}, ErrBlobNotFound
		}
		return nil, time.Time{}, fmt.Errorf("open blob: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, time.Time{}, fmt.Errorf("stat blob: %w", err)
	}
	return f, info.ModTime(), nil
}

func (s *LocalBlobStore) Delete(_ context.Context, key string) error {
	if !ValidAvatarKey(key) {
		return nil
	}
	if err := s.root.Remove(key); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("delete blob: %w", err)
	}
	return nil
}
