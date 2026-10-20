package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestValidateNewPassword(t *testing.T) {
	cases := []struct {
		name         string
		password     string
		email        string
		wantProblems []string // substrings, one per expected problem
	}{
		{"strong passphrase", "correct horse battery", "a@b.co", nil},
		{"exactly ten characters", "x7!kQ2m9Zp", "a@b.co", nil},
		{"nine characters is too short", "x7!kQ2m9Z", "a@b.co", []string{"at least 10"}},
		{"old minimum of eight is no longer enough", "abcd1234", "a@b.co", []string{"at least 10"}},
		{"empty", "", "a@b.co", []string{"at least 10", "blank"}},
		{"common password", "password123", "a@b.co", []string{"too common"}},
		{"common password is case-insensitive", "PassWord123", "a@b.co", []string{"too common"}},
		{"common keyboard walk", "1qaz2wsx3edc", "a@b.co", []string{"too common"}},
		{"repeated character", "aaaaaaaaaaaa", "a@b.co", []string{"repeated"}},
		{"equals the email", "surfer@example.com", "surfer@example.com", []string{"email"}},
		{"equals the email local part", "surferdude", "surferdude@example.com", []string{"email"}},
		{"too long for bcrypt", strings.Repeat("ab", 37), "a@b.co", []string{"at most 72"}},
		{"blank spaces", "          ", "a@b.co", []string{"blank"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateNewPassword(tc.password, tc.email)
			if len(tc.wantProblems) == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var policyErr *PasswordPolicyError
			if !errors.As(err, &policyErr) {
				t.Fatalf("expected PasswordPolicyError, got %v", err)
			}
			for _, want := range tc.wantProblems {
				found := false
				for _, problem := range policyErr.Problems {
					if strings.Contains(problem, want) {
						found = true
					}
				}
				if !found {
					t.Errorf("problems %q missing %q", policyErr.Problems, want)
				}
			}
		})
	}
}

func TestEveryCommonPasswordMeetsLengthSoTheListIsUseful(t *testing.T) {
	if len(commonPasswords) < 100 {
		t.Fatalf("embedded list looks truncated: %d entries", len(commonPasswords))
	}
	for word := range commonPasswords {
		if word != strings.ToLower(word) {
			t.Errorf("entry %q must be lower case", word)
		}
	}
}

func TestRegisterReportsAllPolicyProblems(t *testing.T) {
	auth, _, _ := newTestAuth(t, &memoryUsers{})
	_, err := auth.Register(context.Background(), "surfer@example.com", "pass")
	var policyErr *PasswordPolicyError
	if !errors.As(err, &policyErr) || len(policyErr.Problems) == 0 {
		t.Fatalf("expected a policy error, got %v", err)
	}
}

func TestLoginStillAcceptsLegacyShortPasswords(t *testing.T) {
	// An account created under the old 8-byte rule must still be able to sign in.
	users := &memoryUsers{}
	auth, _, _ := newTestAuth(t, users)
	if _, err := auth.Register(context.Background(), "legacy@example.com", "a-long-enough-pass"); err != nil {
		t.Fatalf("register: %v", err)
	}
	// Swap in a hash of an 8-byte password, as an older account would have.
	hash, err := bcrypt.GenerateFromPassword([]byte("short8pw"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	users.user.PasswordHash = string(hash)
	if _, err := auth.Login(context.Background(), "legacy@example.com", "short8pw"); err != nil {
		t.Fatalf("legacy password must still log in: %v", err)
	}
}
