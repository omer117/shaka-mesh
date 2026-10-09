package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/omer-asraf/shaka-beach-service/internal/domain"
)

const maxCreateBodyBytes = 1 << 20

type beachRepository interface {
	GetAll(ctx context.Context) ([]domain.Beach, error)
	GetByID(ctx context.Context, id string) (*domain.Beach, error)
	FindNearby(ctx context.Context, lat, lon float64, radiusKm float64) ([]domain.Beach, error)
	Create(ctx context.Context, beach *domain.Beach) error
}

type beachHandler struct {
	beaches beachRepository
}

func New(beaches beachRepository) http.Handler {
	h := beachHandler{beaches: beaches}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("GET /api/v1/beaches", h.getAll)
	mux.HandleFunc("GET /api/v1/beaches/nearby", h.findNearby)
	mux.HandleFunc("GET /api/v1/beaches/{id}", h.getByID)
	mux.HandleFunc("POST /api/v1/beaches", h.create)
	return mux
}

func (h beachHandler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, struct {
		Status string `json:"status"`
	}{Status: "ok"})
}

func (h beachHandler) getAll(w http.ResponseWriter, r *http.Request) {
	beaches, err := h.beaches.GetAll(r.Context())
	if err != nil {
		log.Printf("failed to get beaches: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve beaches")
		return
	}
	if beaches == nil {
		beaches = []domain.Beach{}
	}
	writeJSON(w, http.StatusOK, beaches)
}

func (h beachHandler) getByID(w http.ResponseWriter, r *http.Request) {
	beach, err := h.beaches.GetByID(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, domain.ErrBeachNotFound) {
			writeError(w, http.StatusNotFound, "beach not found")
			return
		}
		log.Printf("failed to get beach: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to retrieve beach")
		return
	}
	writeJSON(w, http.StatusOK, beach)
}

func (h beachHandler) findNearby(w http.ResponseWriter, r *http.Request) {
	lat, err := parseCoordinate(r, "lat", -90, 90)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	lon, err := parseCoordinate(r, "lon", -180, 180)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	radiusKm, err := parseQueryFloat(r, "radius_km")
	if err != nil || radiusKm <= 0 {
		writeError(w, http.StatusBadRequest, "radius_km must be a positive number")
		return
	}

	beaches, err := h.beaches.FindNearby(r.Context(), lat, lon, radiusKm)
	if err != nil {
		log.Printf("failed to find nearby beaches: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to find nearby beaches")
		return
	}
	if beaches == nil {
		beaches = []domain.Beach{}
	}
	writeJSON(w, http.StatusOK, beaches)
}

func (h beachHandler) create(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        string   `json:"name"`
		Latitude    *float64 `json:"latitude"`
		Longitude   *float64 `json:"longitude"`
		Description string   `json:"description"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body is too large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid JSON request body")
		}
		return
	}

	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || utf8.RuneCountInString(input.Name) > 100 {
		writeError(w, http.StatusBadRequest, "name is required and must be at most 100 characters")
		return
	}
	if input.Latitude == nil || input.Longitude == nil ||
		math.IsNaN(*input.Latitude) || math.IsInf(*input.Latitude, 0) ||
		math.IsNaN(*input.Longitude) || math.IsInf(*input.Longitude, 0) ||
		*input.Latitude < -90 || *input.Latitude > 90 ||
		*input.Longitude < -180 || *input.Longitude > 180 {
		writeError(w, http.StatusBadRequest, "latitude and longitude must be valid coordinates")
		return
	}

	id, err := newBeachID()
	if err != nil {
		log.Printf("failed to generate beach ID: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create beach")
		return
	}
	beach := &domain.Beach{
		ID:          id,
		Name:        input.Name,
		Latitude:    *input.Latitude,
		Longitude:   *input.Longitude,
		Description: input.Description,
	}
	if err := h.beaches.Create(r.Context(), beach); err != nil {
		log.Printf("failed to create beach: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create beach")
		return
	}
	w.Header().Set("Location", "/api/v1/beaches/"+beach.ID)
	writeJSON(w, http.StatusCreated, beach)
}

func parseCoordinate(r *http.Request, name string, min, max float64) (float64, error) {
	value, err := parseQueryFloat(r, name)
	if err != nil || value < min || value > max {
		return 0, fmt.Errorf("%s must be a number between %v and %v", name, min, max)
	}
	return value, nil
}

func parseQueryFloat(r *http.Request, name string) (float64, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, fmt.Errorf("%s is required", name)
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("%s must be a finite number", name)
	}
	return value, nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxCreateBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain a single JSON value")
		}
		return err
	}
	return nil
}

func newBeachID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	var encoded [36]byte
	hex.Encode(encoded[0:8], raw[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], raw[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], raw[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], raw[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], raw[10:16])
	return string(encoded[:]), nil
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, struct {
		Error string `json:"error"`
	}{Error: message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("failed to encode HTTP response: %v", err)
	}
}
