package swell

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenMeteoClientSwellHeight(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("current"); got != "swell_wave_height" {
			t.Errorf("unexpected current parameter %q", got)
		}
		if got := r.URL.Query().Get("latitude"); got != "32.08" {
			t.Errorf("unexpected latitude %q", got)
		}
		if got := r.URL.Query().Get("longitude"); got != "34.77" {
			t.Errorf("unexpected longitude %q", got)
		}
		fmt.Fprint(w, `{"current":{"swell_wave_height":1.25}}`)
	}))
	defer server.Close()

	client := NewOpenMeteoClient()
	client.httpClient = server.Client()
	client.baseURL = server.URL
	height, err := client.SwellHeight(context.Background(), 32.08, 34.77)
	if err != nil {
		t.Fatalf("fetch swell height: %v", err)
	}
	if height != 1.25 {
		t.Fatalf("expected swell height 1.25, got %v", height)
	}
}

func TestOpenMeteoClientRejectsMissingSwellHeight(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"current":{"swell_wave_height":null}}`)
	}))
	defer server.Close()

	client := NewOpenMeteoClient()
	client.httpClient = server.Client()
	client.baseURL = server.URL
	if _, err := client.SwellHeight(context.Background(), 32.08, 34.77); err == nil {
		t.Fatal("expected an error for a missing swell height")
	}
}

func TestOpenMeteoClientRejectsNonSuccessStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewOpenMeteoClient()
	client.httpClient = server.Client()
	client.baseURL = server.URL
	if _, err := client.SwellHeight(context.Background(), 32.08, 34.77); err == nil {
		t.Fatal("expected an error for a non-success status")
	}
}
