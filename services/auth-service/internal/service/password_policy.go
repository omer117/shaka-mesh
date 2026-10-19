package service

import (
	_ "embed"
	"fmt"
	"strings"
)

const (
	// MinPasswordBytes is the minimum length for NEW passwords (registration).
	// Existing accounts created under the old 8-byte rule can still log in.
	MinPasswordBytes = 10
	// MaxPasswordBytes is bcrypt's input limit; longer values are silently
	// truncated by bcrypt (or rejected), so we refuse them up front.
	MaxPasswordBytes = 72
)

//go:embed common_passwords.txt
var commonPasswordsRaw string

var commonPasswords = func() map[string]struct{} {
	set := make(map[string]struct{})
	for _, line := range strings.Split(commonPasswordsRaw, "\n") {
		if word := strings.ToLower(strings.TrimSpace(line)); word != "" {
			set[word] = struct{}{}
		}
	}
	return set
}()

// PasswordPolicyError lists every rule a rejected password broke, so the UI can
// show all of them at once instead of one per attempt.
type PasswordPolicyError struct {
	Problems []string
}

func (e *PasswordPolicyError) Error() string {
	return "password does not meet requirements: " + strings.Join(e.Problems, "; ")
}

// ValidateNewPassword checks a password being set for the first time. email may
// be empty; when given, passwords that are the email (or its local part) are
// rejected.
func ValidateNewPassword(password, email string) error {
	var problems []string
	if len(password) < MinPasswordBytes {
		problems = append(problems, fmt.Sprintf("must be at least %d characters long", MinPasswordBytes))
	}
	if len(password) > MaxPasswordBytes {
		problems = append(problems, fmt.Sprintf("must be at most %d bytes long", MaxPasswordBytes))
	}
	if strings.TrimSpace(password) == "" {
		problems = append(problems, "must not be blank")
	} else {
		if isSingleRepeatedCharacter(password) {
			problems = append(problems, "must not be a single repeated character")
		}
		if isCommonPassword(password) {
			problems = append(problems, "is too common; choose something less guessable")
		}
		if matchesEmail(password, email) {
			problems = append(problems, "must not be your email address")
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return &PasswordPolicyError{Problems: problems}
}

func isCommonPassword(password string) bool {
	_, found := commonPasswords[strings.ToLower(strings.TrimSpace(password))]
	return found
}

func isSingleRepeatedCharacter(password string) bool {
	runes := []rune(password)
	for _, r := range runes[1:] {
		if r != runes[0] {
			return false
		}
	}
	return len(runes) > 1
}

func matchesEmail(password, email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false
	}
	candidate := strings.ToLower(password)
	local, _, _ := strings.Cut(email, "@")
	return candidate == email || (len(local) >= 6 && candidate == local)
}
