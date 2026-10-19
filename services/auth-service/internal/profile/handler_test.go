package profile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
)

// recordingAPI records which user ID the handler passed down.
type recordingAPI struct {
	gotUserID string
	updateIn  UpdateInput
	avatarLen int
	err       error
}

func (r *recordingAPI) Get(_ context.Context, id string) (*Profile, error) {
	r.gotUserID = id
	return &Profile{ID: id}, r.err
}

func (r *recordingAPI) Update(_ context.Context, id string, in UpdateInput) (*Profile, error) {
	r.gotUserID, r.updateIn = id, in
	return &Profile{ID: id}, r.err
}

func (r *recordingAPI) SetAvatar(_ context.Context, id string, data []byte) (*Profile, error) {
	r.gotUserID, r.avatarLen = id, len(data)
	return &Profile{ID: id, AvatarURL: "/media/avatars/x.jpg"}, r.err
}

func newMux(t *testing.T, api API) (*http.ServeMux, *LocalBlobStore) {
	t.Helper()
	blobs, err := NewLocalBlobStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { blobs.Close() })
	mux := http.NewServeMux()
	NewHandler(api, blobs).Register(mux)
	return mux, blobs
}

func do(mux http.Handler, method, path, userID, contentType string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if userID != "" {
		req.Header.Set(UserIDHeader, userID)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func multipartBody(t *testing.T, field, partContentType string, data []byte) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="`+field+`"; filename="me.png"`)
	h.Set("Content-Type", partContentType)
	part, err := w.CreatePart(h)
	if err != nil {
		t.Fatal(err)
	}
	part.Write(data)
	w.Close()
	return buf.Bytes(), w.FormDataContentType()
}

func TestEndpointsRequireIdentity(t *testing.T) {
	mux, _ := newMux(t, &recordingAPI{})
	for _, tc := range []struct{ method, path string }{
		{"GET", "/me"}, {"PATCH", "/me"}, {"POST", "/me/avatar"},
	} {
		if rec := do(mux, tc.method, tc.path, "", "application/json", []byte(`{}`)); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without identity: got %d, want 401", tc.method, tc.path, rec.Code)
		}
	}
}

func TestIdentityComesFromHeaderNeverFromBody(t *testing.T) {
	api := &recordingAPI{}
	mux, _ := newMux(t, api)

	rec := do(mux, "PATCH", "/me", "alice", "application/json", []byte(`{"bio":"hi"}`))
	if rec.Code != http.StatusOK || api.gotUserID != "alice" {
		t.Fatalf("code=%d user=%q", rec.Code, api.gotUserID)
	}

	// Attempts to name another user (or touch protected fields) are rejected
	// outright, and nothing reaches the service.
	for _, body := range []string{
		`{"id":"bob","bio":"pwned"}`,
		`{"user_id":"bob"}`,
		`{"email":"evil@example.com"}`,
		`{"avatar_url":"http://evil.example/x.png"}`,
	} {
		api := &recordingAPI{}
		mux, _ := newMux(t, api)
		rec := do(mux, "PATCH", "/me", "alice", "application/json", []byte(body))
		if rec.Code != http.StatusBadRequest || api.gotUserID != "" {
			t.Errorf("body %s: code=%d, service called=%v", body, rec.Code, api.gotUserID != "")
		}
	}

	// There is no per-user URL to abuse.
	for _, path := range []string{"/me/bob", "/users/bob", "/users/bob/profile"} {
		if rec := do(mux, "PATCH", path, "alice", "application/json", []byte(`{"bio":"x"}`)); rec.Code != http.StatusNotFound {
			t.Errorf("PATCH %s: got %d, want 404", path, rec.Code)
		}
	}
}

func TestPatchErrorMapping(t *testing.T) {
	tests := []struct {
		name string
		err  error
		body string
		want int
	}{
		{"validation", ValidationError{"bio too long"}, `{"bio":"x"}`, http.StatusBadRequest},
		{"unknown beach", ErrBeachNotFound, `{"home_beach_id":"z"}`, http.StatusBadRequest},
		{"not found", ErrNotFound, `{"bio":"x"}`, http.StatusNotFound},
		{"internal", errors.New("db down"), `{"bio":"x"}`, http.StatusInternalServerError},
		{"malformed json", nil, `{"bio":`, http.StatusBadRequest},
		{"trailing data", nil, `{"bio":"x"} {"bio":"y"}`, http.StatusBadRequest},
		{"ok", nil, `{"display_name":"A"}`, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux, _ := newMux(t, &recordingAPI{err: tt.err})
			if rec := do(mux, "PATCH", "/me", "alice", "application/json", []byte(tt.body)); rec.Code != tt.want {
				t.Fatalf("got %d, want %d: %s", rec.Code, tt.want, rec.Body)
			}
		})
	}
}

func TestInternalErrorsDoNotLeak(t *testing.T) {
	mux, _ := newMux(t, &recordingAPI{err: errors.New("pq: password authentication failed")})
	rec := do(mux, "GET", "/me", "alice", "", nil)
	if strings.Contains(rec.Body.String(), "password authentication") {
		t.Fatal("internal error leaked to client")
	}
}

func TestUploadAvatar(t *testing.T) {
	png := encodePNG(t, twoToneImage(10, 10))
	big := append(append([]byte{}, png...), make([]byte, MaxAvatarBytes)...)

	tests := []struct {
		name        string
		field       string
		partType    string
		data        []byte
		serviceErr  error
		wantStatus  int
		wantService bool
	}{
		{"valid png", "avatar", "image/png", png, nil, http.StatusOK, true},
		{"claimed type is ignored", "avatar", "text/plain", png, nil, http.StatusOK, true},
		{"wrong field name", "file", "image/png", png, nil, http.StatusBadRequest, false},
		{"service says unsupported (spoofed header)", "avatar", "image/png", []byte("MZ exe"), ErrUnsupportedImage, http.StatusUnsupportedMediaType, true},
		{"service says invalid image", "avatar", "image/png", png, ErrInvalidImage, http.StatusBadRequest, true},
		{"too large", "avatar", "image/png", big, ErrImageTooLarge, http.StatusRequestEntityTooLarge, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &recordingAPI{err: tt.serviceErr}
			mux, _ := newMux(t, api)
			body, ct := multipartBody(t, tt.field, tt.partType, tt.data)
			rec := do(mux, "POST", "/me/avatar", "alice", ct, body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("got %d, want %d: %s", rec.Code, tt.wantStatus, rec.Body)
			}
			if called := api.gotUserID != ""; called != tt.wantService {
				t.Fatalf("service called=%v, want %v", called, tt.wantService)
			}
			if tt.wantService && api.gotUserID != "alice" {
				t.Fatalf("service got user %q", api.gotUserID)
			}
		})
	}
}

func TestUploadAvatarRequiresMultipart(t *testing.T) {
	mux, _ := newMux(t, &recordingAPI{})
	if rec := do(mux, "POST", "/me/avatar", "alice", "image/png", []byte("raw")); rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestUploadRejectsHugeBodyBeforeService(t *testing.T) {
	api := &recordingAPI{}
	mux, _ := newMux(t, api)
	body, ct := multipartBody(t, "avatar", "image/png", make([]byte, 3*MaxAvatarBytes))
	rec := do(mux, "POST", "/me/avatar", "alice", ct, body)
	if rec.Code != http.StatusRequestEntityTooLarge || api.gotUserID != "" {
		t.Fatalf("got %d, called=%v", rec.Code, api.gotUserID != "")
	}
}

func TestServeAvatar(t *testing.T) {
	mux, blobs := newMux(t, &recordingAPI{})
	key, _ := NewAvatarKey()
	if err := blobs.Put(context.Background(), key, []byte("jpegdata")); err != nil {
		t.Fatal(err)
	}

	rec := do(mux, "GET", "/media/avatars/"+key, "", "", nil) // public: no identity
	if rec.Code != http.StatusOK || rec.Body.String() != "jpegdata" {
		t.Fatalf("got %d %q", rec.Code, rec.Body)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") || !strings.Contains(cc, "max-age=31536000") {
		t.Fatalf("Cache-Control = %q", cc)
	}
	if rec.Header().Get("Content-Type") != "image/jpeg" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("bad headers: %v", rec.Header())
	}

	for _, name := range []string{
		"missing.jpg", "0123456789abcdef0123456789abcdef.jpg", "..%2fsecret.jpg",
		"%2e%2e%2f%2e%2e%2fetc%2fpasswd", "0123456789abcdef0123456789abcdef.png",
	} {
		if rec := do(mux, "GET", "/media/avatars/"+name, "", "", nil); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: got %d, want 404", name, rec.Code)
		}
	}
}

func TestGetReturnsProfileJSON(t *testing.T) {
	mux, _ := newMux(t, &recordingAPI{})
	rec := do(mux, "GET", "/me", "alice", "", nil)
	var p Profile
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil || p.ID != "alice" {
		t.Fatalf("err=%v profile=%+v", err, p)
	}
}
