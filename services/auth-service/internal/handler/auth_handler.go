package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/omer-asraf/shaka-auth-service/internal/domain"
	"github.com/omer-asraf/shaka-auth-service/internal/repository"
	"github.com/omer-asraf/shaka-auth-service/internal/service"
)

const maxRequestBodyBytes = 1 << 20

type Authenticator interface {
	Register(ctx context.Context, email, password string) (*domain.User, error)
	Login(ctx context.Context, email, password string) (*service.Session, error)
	Refresh(ctx context.Context, refreshToken string) (*service.Session, error)
	Logout(ctx context.Context, refreshToken string) error
}

// CookieConfig describes the refresh-token cookie. Path must be the path the
// browser sees (the gateway prefix, e.g. /api/v1/auth), not the path this
// service is mounted at behind the gateway.
type CookieConfig struct {
	Name     string
	Path     string
	Secure   bool
	SameSite http.SameSite
}

// DefaultCookieConfig returns the production-safe defaults.
func DefaultCookieConfig() CookieConfig {
	return CookieConfig{
		Name:     "shaka_refresh",
		Path:     "/api/v1/auth",
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
}

type authHandler struct {
	auth    Authenticator
	cookies CookieConfig
}

func New(auth Authenticator, cookies CookieConfig) http.Handler {
	h := authHandler{auth: auth, cookies: cookies}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/register", h.register)
	mux.HandleFunc("POST /auth/login", h.login)
	mux.HandleFunc("POST /auth/refresh", h.refresh)
	mux.HandleFunc("POST /auth/logout", h.logout)
	mux.HandleFunc("GET /health", h.health)
	return mux
}

func (h authHandler) register(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request body")
		return
	}
	if err := validateCredentials(input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	user, err := h.auth.Register(r.Context(), input.Email, input.Password)
	if err != nil {
		if errors.Is(err, repository.ErrEmailInUse) {
			writeError(w, http.StatusConflict, "email is already registered")
			return
		}
		if strings.HasPrefix(err.Error(), "email ") || strings.HasPrefix(err.Error(), "password ") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Printf("register user: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to register user")
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

func (h authHandler) login(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request body")
		return
	}
	if err := validateCredentials(input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	session, err := h.auth.Login(r.Context(), input.Email, input.Password)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		log.Printf("login user: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to log in")
		return
	}
	h.writeSession(w, session)
}

func (h authHandler) refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(h.cookies.Name)
	if err != nil || cookie.Value == "" {
		writeError(w, http.StatusUnauthorized, "refresh token required")
		return
	}
	session, err := h.auth.Refresh(r.Context(), cookie.Value)
	if err != nil {
		if errors.Is(err, service.ErrInvalidRefreshToken) {
			h.clearCookie(w)
			writeError(w, http.StatusUnauthorized, "invalid refresh token")
			return
		}
		log.Printf("refresh session: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to refresh session")
		return
	}
	h.writeSession(w, session)
}

func (h authHandler) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(h.cookies.Name); err == nil {
		if err := h.auth.Logout(r.Context(), cookie.Value); err != nil {
			log.Printf("logout: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to log out")
			return
		}
	}
	h.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h authHandler) writeSession(w http.ResponseWriter, session *service.Session) {
	h.setCookie(w, session.RefreshToken, session.RefreshExpiresAt)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, struct {
		AccessToken string    `json:"access_token"`
		TokenType   string    `json:"token_type"`
		ExpiresAt   time.Time `json:"expires_at"`
		Email       string    `json:"email"`
	}{
		AccessToken: session.AccessToken,
		TokenType:   "Bearer",
		ExpiresAt:   session.ExpiresAt,
		Email:       session.Email,
	})
}

func (h authHandler) setCookie(w http.ResponseWriter, value string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.cookies.Name,
		Value:    value,
		Path:     h.cookies.Path,
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: true,
		Secure:   h.cookies.Secure,
		SameSite: h.cookies.SameSite,
	})
}

func (h authHandler) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.cookies.Name,
		Value:    "",
		Path:     h.cookies.Path,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.cookies.Secure,
		SameSite: h.cookies.SameSite,
	})
}

func (h authHandler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, struct {
		Status string `json:"status"`
	}{Status: "ok"})
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func validateCredentials(input credentials) error {
	email := strings.TrimSpace(input.Email)
	if email == "" || utf8.RuneCountInString(email) > 320 {
		return errors.New("email is required and must be at most 320 characters")
	}
	if strings.TrimSpace(input.Password) == "" {
		return errors.New("password is required")
	}
	if len(input.Password) < service.MinPasswordBytes || len(input.Password) > service.MaxPasswordBytes {
		return fmt.Errorf("password must be between %d and %d bytes", service.MinPasswordBytes, service.MaxPasswordBytes)
	}
	return nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain a single JSON value")
		}
		return err
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, struct {
		Error string `json:"error"`
	}{Error: message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode HTTP response: %v", err)
	}
}
