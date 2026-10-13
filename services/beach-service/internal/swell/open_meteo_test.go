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

func TestOpenMeteoClientCurrentConditions(t *testing.T) {
	marineServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("current"); got != "swell_wave_height" {
			t.Errorf("unexpected marine current parameter %q", got)
		}
		fmt.Fprint(w, `{"current":{"swell_wave_height":1.4}}`)
	}))
	defer marineServer.Close()

	forecastServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("current"); got != "wind_speed_10m" {
			t.Errorf("unexpected forecast current parameter %q", got)
		}
		if got := r.URL.Query().Get("wind_speed_unit"); got != "kn" {
			t.Errorf("expected wind speed in knots, got %q", got)
		}
		fmt.Fprint(w, `{"current":{"wind_speed_10m":9.3}}`)
	}))
	defer forecastServer.Close()

	client := NewOpenMeteoClient()
	client.httpClient = marineServer.Client()
	client.baseURL = marineServer.URL
	client.forecastBaseURL = forecastServer.URL

	conditions, err := client.CurrentConditions(context.Background(), 32.08, 34.77)
	if err != nil {
		t.Fatalf("fetch current conditions: %v", err)
	}
	if conditions != (Conditions{WaveHeight: 1.4, WindSpeed: 9.3}) {
		t.Fatalf("unexpected current conditions: %+v", conditions)
	}
}

func TestOpenMeteoClientCurrentConditionsFailsWhenWindIsUnavailable(t *testing.T) {
	marineServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"current":{"swell_wave_height":1.4}}`)
	}))
	defer marineServer.Close()

	forecastServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"current":{"wind_speed_10m":null}}`)
	}))
	defer forecastServer.Close()

	client := NewOpenMeteoClient()
	client.httpClient = marineServer.Client()
	client.baseURL = marineServer.URL
	client.forecastBaseURL = forecastServer.URL

	if _, err := client.CurrentConditions(context.Background(), 32.08, 34.77); err == nil {
		t.Fatal("expected an error when wind speed is unavailable")
	}
}
