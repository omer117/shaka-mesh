package domain

import "time"

// RefreshToken is the server-side record of an opaque refresh token. Only the
// SHA-256 hash of the token is ever stored; the raw value lives in the
// client's HttpOnly cookie. Tokens issued from the same login share a FamilyID
// so the whole chain can be revoked when a rotated token is replayed.
type RefreshToken struct {
	ID        string
	UserID    string
	FamilyID  string
	TokenHash string
	ExpiresAt time.Time
	Revoked   bool
	CreatedAt time.Time
}
