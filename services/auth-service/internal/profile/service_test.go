package profile

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeRepo struct {
	profiles map[string]*Profile
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{profiles: map[string]*Profile{
		"alice": {ID: "alice", Email: "alice@example.com", DisplayName: "Alice"},
		"bob":   {ID: "bob", Email: "bob@example.com", DisplayName: "Bob"},
	}}
}

func (f *fakeRepo) Get(_ context.Context, id string) (*Profile, error) {
	p, ok := f.profiles[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *p
	return &cp, nil
}

func (f *fakeRepo) Update(_ context.Context, id string, p *Profile) error {
	cur, ok := f.profiles[id]
	if !ok {
		return ErrNotFound
	}
	cur.DisplayName, cur.Bio, cur.HomeBeachID = p.DisplayName, p.Bio, p.HomeBeachID
	return nil
}

func (f *fakeRepo) SetAvatarURL(_ context.Context, id, url string) (string, error) {
	cur, ok := f.profiles[id]
	if !ok {
		return "", ErrNotFound
	}
	prev := cur.AvatarURL
	cur.AvatarURL = url
	return prev, nil
}

type fakeBeaches map[string]bool

func (f fakeBeaches) Exists(_ context.Context, id string) (bool, error) { return f[id], nil }

func str(s string) *string { return &s }

func newTestService(t *testing.T) (*Service, *fakeRepo, *LocalBlobStore) {
	t.Helper()
	repo := newFakeRepo()
	blobs, err := NewLocalBlobStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { blobs.Close() })
	return NewService(repo, blobs, fakeBeaches{"b1": true}), repo, blobs
}

func TestUpdateValidation(t *testing.T) {
	tests := []struct {
		name    string
		in      UpdateInput
		wantErr string // substring; "" means success
	}{
		{"valid", UpdateInput{DisplayName: str("  Alice S  "), Bio: str("surfer"), HomeBeachID: str("b1")}, ""},
		{"clear home beach", UpdateInput{HomeBeachID: str("")}, ""},
		{"display name too long", UpdateInput{DisplayName: str(strings.Repeat("x", 51))}, "display_name"},
		{"display name multibyte ok at 50 runes", UpdateInput{DisplayName: str(strings.Repeat("ש", 50))}, ""},
		{"display name control char", UpdateInput{DisplayName: str("a\x00b")}, "control"},
		{"bio too long", UpdateInput{Bio: str(strings.Repeat("x", 501))}, "bio"},
		{"unknown beach", UpdateInput{HomeBeachID: str("nope")}, ErrBeachNotFound.Error()},
		{"beach id too long", UpdateInput{HomeBeachID: str(strings.Repeat("a", 37))}, "home_beach_id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, _ := newTestService(t)
			_, err := svc.Update(context.Background(), "alice", tt.in)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("got %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestUpdateOnlyTouchesOwnProfile(t *testing.T) {
	svc, repo, _ := newTestService(t)
	if _, err := svc.Update(context.Background(), "alice", UpdateInput{Bio: str("alice bio")}); err != nil {
		t.Fatal(err)
	}
	if repo.profiles["bob"].Bio != "" || repo.profiles["bob"].DisplayName != "Bob" {
		t.Fatal("updating alice changed bob")
	}
	if repo.profiles["alice"].Bio != "alice bio" || repo.profiles["alice"].DisplayName != "Alice" {
		t.Fatal("partial update should keep untouched fields")
	}
	if _, err := svc.Update(context.Background(), "ghost", UpdateInput{Bio: str("x")}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestSetAvatarStoresAndReplacesPreviousFile(t *testing.T) {
	svc, repo, blobs := newTestService(t)
	img := encodePNG(t, twoToneImage(100, 100))

	first, err := svc.SetAvatar(context.Background(), "alice", img)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first.AvatarURL, AvatarURLPrefix) || !strings.HasSuffix(first.AvatarURL, ".jpg") {
		t.Fatalf("unexpected avatar url %q", first.AvatarURL)
	}
	firstKey := strings.TrimPrefix(first.AvatarURL, AvatarURLPrefix)
	f, _, err := blobs.Open(context.Background(), firstKey)
	if err != nil {
		t.Fatalf("avatar not stored: %v", err)
	}
	f.Close()

	second, err := svc.SetAvatar(context.Background(), "alice", img)
	if err != nil {
		t.Fatal(err)
	}
	if second.AvatarURL == first.AvatarURL {
		t.Fatal("each upload must get a fresh random name")
	}
	if _, _, err := blobs.Open(context.Background(), firstKey); !errors.Is(err, ErrBlobNotFound) {
		t.Fatal("previous avatar should have been deleted")
	}
	if repo.profiles["bob"].AvatarURL != "" {
		t.Fatal("bob's avatar changed")
	}
}

func TestSetAvatarRejectsBadInputWithoutStoring(t *testing.T) {
	svc, repo, _ := newTestService(t)
	_, err := svc.SetAvatar(context.Background(), "alice", []byte("not an image"))
	if !errors.Is(err, ErrUnsupportedImage) {
		t.Fatalf("got %v", err)
	}
	if repo.profiles["alice"].AvatarURL != "" {
		t.Fatal("avatar url must not change on failure")
	}
}
