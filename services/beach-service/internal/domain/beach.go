package domain

import (
	"context"
	"errors"
	"time"
)

var ErrBeachNotFound = errors.New("beach not found")

// Beach מייצג חוף גלישה במערכת
type Beach struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Latitude    float64   `json:"latitude"`
	Longitude   float64   `json:"longitude"`
	WaveHeight  float64   `json:"wave_height"` // גובה גל במטרים
	WindSpeed   float64   `json:"wind_speed"`  // מהירות רוח בקשר
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// BeachRepository מגדיר את החוזה (Interface) מול מסד הנתונים
// כל מי שמממש את הממשק הזה חייב לספק את הפונקציות האלו.
type BeachRepository interface {
	GetAll(ctx context.Context) ([]Beach, error)
	GetByID(ctx context.Context, id string) (*Beach, error)
	FindNearby(ctx context.Context, lat, lon float64, radiusKm float64) ([]Beach, error)
	Create(ctx context.Context, beach *Beach) error
	UpdateWaveHeight(ctx context.Context, id string, height float64) error
}
