package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/omer-asraf/shaka-beach-service/internal/domain"
)

type stubBeachReader struct {
	beaches []domain.Beach
	beach   *domain.Beach
	nearby  []domain.Beach
	created *domain.Beach
	err     error
}

func (s stubBeachReader) GetAll(context.Context) ([]domain.Beach, error) {
	return s.beaches, s.err
}

func (s stubBeachReader) GetByID(context.Context, string) (*domain.Beach, error) {
	return s.beach, s.err
}

func (s stubBeachReader) FindNearby(context.Context, float64, float64, float64) ([]domain.Beach, error) {
	return s.nearby, s.err
}

func (s *stubBeachReader) Create(_ context.Context, beach *domain.Beach) error {
	if s.err != nil {
		return s.err
	}
	s.created = beach
	return nil
}

func TestHealth(t *testing.T) {
	handler := New(&stubBeachReader{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("unexpected content type %q", got)
	}
	if got, want := response.Body.String(), "{\"status\":\"ok\"}\n"; got != want {
		t.Fatalf("unexpected response body %q, want %q", got, want)
	}
}

func TestGetAllBeaches(t *testing.T) {
	handler := New(&stubBeachReader{
		beaches: []domain.Beach{{ID: "b1", Name: "Hilton Beach"}},
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/beaches", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	if got, want := response.Header().Get("Content-Type"), "application/json; charset=utf-8"; got != want {
		t.Fatalf("unexpected content type %q, want %q", got, want)
	}
	if got, want := response.Body.String(), "[{\"id\":\"b1\",\"name\":\"Hilton Beach\",\"latitude\":0,\"longitude\":0,\"wave_height\":0,\"wind_speed\":0,\"description\":\"\",\"created_at\":\"0001-01-01T00:00:00Z\",\"updated_at\":\"0001-01-01T00:00:00Z\"}]\n"; got != want {
		t.Fatalf("unexpected response body %q, want %q", got, want)
	}
}

func TestGetAllBeachesReturnsEmptyArray(t *testing.T) {
	handler := New(&stubBeachReader{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/beaches", nil))

	if got, want := response.Body.String(), "[]\n"; got != want {
		t.Fatalf("unexpected response body %q, want %q", got, want)
	}
}

func TestGetAllBeachesReturnsInternalServerError(t *testing.T) {
	handler := New(&stubBeachReader{err: errors.New("database unavailable")})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/beaches", nil))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, response.Code)
	}
}

func TestGetBeachByID(t *testing.T) {
	handler := New(&stubBeachReader{beach: &domain.Beach{ID: "b1", Name: "Hilton Beach"}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/beaches/b1", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	var beach domain.Beach
	if err := json.Unmarshal(response.Body.Bytes(), &beach); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if beach.ID != "b1" {
		t.Fatalf("expected beach ID b1, got %q", beach.ID)
	}
}

func TestGetBeachByIDReturnsNotFound(t *testing.T) {
	handler := New(&stubBeachReader{err: fmt.Errorf("%w: b1", domain.ErrBeachNotFound)})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/beaches/b1", nil))

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, response.Code)
	}
}

func TestFindNearbyBeaches(t *testing.T) {
	handler := New(&stubBeachReader{nearby: []domain.Beach{{ID: "b1"}}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(
		http.MethodGet,
		"/api/v1/beaches/nearby?lat=32.08&lon=34.77&radius_km=5",
		nil,
	))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
}

func TestFindNearbyBeachesRejectsInvalidCoordinates(t *testing.T) {
	handler := New(&stubBeachReader{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(
		http.MethodGet,
		"/api/v1/beaches/nearby?lat=91&lon=34.77&radius_km=5",
		nil,
	))

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}
}

func TestCreateBeach(t *testing.T) {
	repository := &stubBeachReader{}
	handler := New(repository)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(
		http.MethodPost,
		"/api/v1/beaches",
		strings.NewReader(`{"name":"  Test Beach ","latitude":32.08,"longitude":34.77,"description":"A break"}`),
	))

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, response.Code, response.Body.String())
	}
	if repository.created == nil {
		t.Fatal("expected beach to be created")
	}
	if repository.created.Name != "Test Beach" {
		t.Fatalf("expected trimmed beach name, got %q", repository.created.Name)
	}
	if len(repository.created.ID) != 36 {
		t.Fatalf("expected UUID beach ID, got %q", repository.created.ID)
	}
	if got, want := response.Header().Get("Location"), "/api/v1/beaches/"+repository.created.ID; got != want {
		t.Fatalf("unexpected Location header %q, want %q", got, want)
	}
}

func TestCreateBeachRejectsInvalidRequest(t *testing.T) {
	handler := New(&stubBeachReader{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(
		http.MethodPost,
		"/api/v1/beaches",
		strings.NewReader(`{"name":"Test Beach","latitude":91,"longitude":34.77}`),
	))

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}
}
