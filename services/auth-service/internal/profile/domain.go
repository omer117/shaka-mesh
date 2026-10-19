// Package profile implements user profiles (display name, bio, home beach and
// avatar). It is layered like the rest of the service: domain.go (types and
// errors), repository.go (PostgreSQL), service.go (rules), handler.go (HTTP).
package profile

import (
	"errors"
	"time"
)

// Profile is the owner's view of a user. Email comes from the account itself;
// everything else is editable through PATCH /me.
type Profile struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Bio         string    `json:"bio"`
	HomeBeachID string    `json:"home_beach_id"`
	AvatarURL   string    `json:"avatar_url"`
	CreatedAt   time.Time `json:"created_at"`
}

// UpdateInput is a partial update: nil fields are left untouched, an empty
// string clears the field. There is intentionally no ID, email or avatar field.
type UpdateInput struct {
	DisplayName *string `json:"display_name"`
	Bio         *string `json:"bio"`
	HomeBeachID *string `json:"home_beach_id"`
}

const (
	MaxDisplayNameRunes = 50
	MaxBioRunes         = 500
	MaxHomeBeachIDBytes = 36
)

var (
	ErrNotFound         = errors.New("profile not found")
	ErrBeachNotFound    = errors.New("home beach does not exist")
	ErrUnsupportedImage = errors.New("avatar must be a JPEG, PNG or WebP image")
	ErrImageTooLarge    = errors.New("avatar must be at most 2 MB")
	ErrInvalidImage     = errors.New("avatar image could not be decoded")
)

// ValidationError is a client mistake that maps to HTTP 400.
type ValidationError struct{ Message string }

func (e ValidationError) Error() string { return e.Message }
