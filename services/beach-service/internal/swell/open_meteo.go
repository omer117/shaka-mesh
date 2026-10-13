package swell

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	marineAPIURL   = "https://marine-api.open-meteo.com/v1/marine"
	forecastAPIURL = "https://api.open-meteo.com/v1/forecast"
)

type Conditions struct {
	WaveHeight float64
	WindSpeed  float64
}

type HeightProvider interface {
	SwellHeight(ctx context.Context, latitude, longitude float64) (float64, error)
}

type ConditionsProvider interface {
	CurrentConditions(ctx context.Context, latitude, longitude float64) (Conditions, error)
}

type OpenMeteoClient struct {
	httpClient      *http.Client
	baseURL         string
	forecastBaseURL string
}

func NewOpenMeteoClient() *OpenMeteoClient {
	return &OpenMeteoClient{
		httpClient:      &http.Client{Timeout: 10 * time.Second},
		baseURL:         marineAPIURL,
		forecastBaseURL: forecastAPIURL,
	}
}

func (c *OpenMeteoClient) SwellHeight(ctx context.Context, latitude, longitude float64) (float64, error) {
	endpoint, err := url.Parse(c.baseURL)
	if err != nil {
		return 0, fmt.Errorf("parse Open-Meteo URL: %w", err)
	}
	query := endpoint.Query()
	query.Set("latitude", strconv.FormatFloat(latitude, 'f', -1, 64))
	query.Set("longitude", strconv.FormatFloat(longitude, 'f', -1, 64))
	query.Set("current", "swell_wave_height")
	query.Set("timezone", "UTC")
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return 0, fmt.Errorf("create Open-Meteo request: %w", err)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return 0, fmt.Errorf("request Open-Meteo swell height: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("Open-Meteo returned status %d", response.StatusCode)
	}

	var payload struct {
		Current struct {
			SwellWaveHeight *float64 `json:"swell_wave_height"`
		} `json:"current"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return 0, fmt.Errorf("decode Open-Meteo response: %w", err)
	}
	if payload.Current.SwellWaveHeight == nil {
		return 0, errors.New("Open-Meteo response did not include current swell height")
	}
	height := *payload.Current.SwellWaveHeight
	if math.IsNaN(height) || math.IsInf(height, 0) || height < 0 {
		return 0, fmt.Errorf("Open-Meteo returned invalid swell height %v", height)
	}
	return height, nil
}

func (c *OpenMeteoClient) CurrentConditions(ctx context.Context, latitude, longitude float64) (Conditions, error) {
	height, err := c.SwellHeight(ctx, latitude, longitude)
	if err != nil {
		return Conditions{}, err
	}
	windSpeed, err := c.windSpeed(ctx, latitude, longitude)
	if err != nil {
		return Conditions{}, err
	}
	return Conditions{WaveHeight: height, WindSpeed: windSpeed}, nil
}

func (c *OpenMeteoClient) windSpeed(ctx context.Context, latitude, longitude float64) (float64, error) {
	endpoint, err := url.Parse(c.forecastBaseURL)
	if err != nil {
		return 0, fmt.Errorf("parse Open-Meteo forecast URL: %w", err)
	}
	query := endpoint.Query()
	query.Set("latitude", strconv.FormatFloat(latitude, 'f', -1, 64))
	query.Set("longitude", strconv.FormatFloat(longitude, 'f', -1, 64))
	query.Set("current", "wind_speed_10m")
	query.Set("wind_speed_unit", "kn")
	query.Set("timezone", "UTC")
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return 0, fmt.Errorf("create Open-Meteo wind request: %w", err)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return 0, fmt.Errorf("request Open-Meteo wind speed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("Open-Meteo forecast returned status %d", response.StatusCode)
	}

	var payload struct {
		Current struct {
			WindSpeed *float64 `json:"wind_speed_10m"`
		} `json:"current"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return 0, fmt.Errorf("decode Open-Meteo wind response: %w", err)
	}
	if payload.Current.WindSpeed == nil {
		return 0, errors.New("Open-Meteo response did not include current wind speed")
	}
	windSpeed := *payload.Current.WindSpeed
	if math.IsNaN(windSpeed) || math.IsInf(windSpeed, 0) || windSpeed < 0 {
		return 0, fmt.Errorf("Open-Meteo returned invalid wind speed %v", windSpeed)
	}
	return windSpeed, nil
}
