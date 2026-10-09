package domain

import "time"

type SwellAlertEvent struct {
	BeachID    string    `json:"beach_id"`
	BeachName  string    `json:"beach_name"`
	WaveHeight float64   `json:"wave_height"`
	Timestamp  time.Time `json:"timestamp"`
}
