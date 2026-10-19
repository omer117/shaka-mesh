package profile

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"strings"
)

// UserIDHeader is set by the api-gateway from the verified JWT. Services sit
// on the internal network only, so the header is trusted (same as
// community-service). It is the sole source of identity in this package.
const UserIDHeader = "X-Authenticated-User-ID"

const maxMultipartOverhead = 64 << 10

type API interface {
	Get(ctx context.Context, userID string) (*Profile, error)
	Update(ctx context.Context, userID string, in UpdateInput) (*Profile, error)
	SetAvatar(ctx context.Context, userID string, data []byte) (*Profile, error)
}

type Handler struct {
	api   API
	blobs BlobStore
}

func NewHandler(api API, blobs BlobStore) *Handler { return &Handler{api: api, blobs: blobs} }

// Register mounts the profile routes on mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /me", h.get)
	mux.HandleFunc("PATCH /me", h.patch)
	mux.HandleFunc("POST /me/avatar", h.uploadAvatar)
	mux.HandleFunc("GET /media/avatars/{name}", h.serveAvatar)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity(w, r)
	if !ok {
		return
	}
	p, err := h.api.Get(r.Context(), userID)
	h.respond(w, p, err)
}

func (h *Handler) patch(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity(w, r)
	if !ok {
		return
	}
	var in UpdateInput
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields() // id, email, avatar_url etc. are rejected
	if err := dec.Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request body")
		return
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "request body must contain a single JSON value")
		return
	}
	p, err := h.api.Update(r.Context(), userID, in)
	h.respond(w, p, err)
}

func (h *Handler) uploadAvatar(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity(w, r)
	if !ok {
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		writeError(w, http.StatusBadRequest, "request must be multipart/form-data")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxAvatarBytes+maxMultipartOverhead)
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart request")
		return
	}
	var data []byte
	found := false
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			h.respond(w, nil, uploadReadError(err))
			return
		}
		if part.FormName() != "avatar" || found {
			part.Close()
			continue
		}
		// Read one byte past the limit so oversize files are detected exactly.
		data, err = io.ReadAll(io.LimitReader(part, MaxAvatarBytes+1))
		part.Close()
		if err != nil {
			h.respond(w, nil, uploadReadError(err))
			return
		}
		found = true
	}
	if !found {
		writeError(w, http.StatusBadRequest, `multipart field "avatar" is required`)
		return
	}
	// The part's own Content-Type header is deliberately ignored; the service
	// sniffs the real type from the bytes.
	p, err := h.api.SetAvatar(r.Context(), userID, data)
	h.respond(w, p, err)
}

func uploadReadError(err error) error {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		return ErrImageTooLarge
	}
	return ValidationError{"invalid multipart request"}
}

func (h *Handler) serveAvatar(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !ValidAvatarKey(name) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	f, modTime, err := h.blobs.Open(r.Context(), name)
	if err != nil {
		if errors.Is(err, ErrBlobNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		log.Printf("open avatar: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to read avatar")
		return
	}
	defer f.Close()
	// Names are random and never reused, so the content is immutable.
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("ETag", `"`+strings.TrimSuffix(name, ".jpg")+`"`)
	http.ServeContent(w, r, name, modTime, f)
}

func identity(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := strings.TrimSpace(r.Header.Get(UserIDHeader))
	if id == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return "", false
	}
	return id, true
}

func (h *Handler) respond(w http.ResponseWriter, p *Profile, err error) {
	var validation ValidationError
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, p)
	case errors.As(err, &validation):
		writeError(w, http.StatusBadRequest, validation.Message)
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "profile not found")
	case errors.Is(err, ErrBeachNotFound):
		writeError(w, http.StatusBadRequest, ErrBeachNotFound.Error())
	case errors.Is(err, ErrImageTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, ErrImageTooLarge.Error())
	case errors.Is(err, ErrUnsupportedImage):
		writeError(w, http.StatusUnsupportedMediaType, ErrUnsupportedImage.Error())
	case errors.Is(err, ErrInvalidImage):
		writeError(w, http.StatusBadRequest, ErrInvalidImage.Error())
	default:
		log.Printf("profile request failed: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode HTTP response: %v", err)
	}
}
