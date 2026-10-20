package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
	"github.com/omer-asraf/shaka-auth-service/internal/domain"
	"github.com/omer-asraf/shaka-auth-service/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidCredentials = errors.New("invalid email or password")

type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetByID(ctx context.Context, id string) (*domain.User, error)
}

type TokenIssuer interface {
	Issue(user *domain.User) (string, time.Time, error)
}

type AuthService struct {
	users      UserRepository
	tokens     TokenIssuer
	refresh    RefreshTokenRepository
	refreshTTL time.Duration
	now        func() time.Time
}

func NewAuthService(users UserRepository, tokens TokenIssuer, refresh RefreshTokenRepository, refreshTTL time.Duration) *AuthService {
	return &AuthService{users: users, tokens: tokens, refresh: refresh, refreshTTL: refreshTTL, now: time.Now}
}

func (s *AuthService) Register(ctx context.Context, email, password string) (*domain.User, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return nil, err
	}
	if err := ValidateNewPassword(password, email); err != nil {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	id, err := newUserID()
	if err != nil {
		return nil, fmt.Errorf("generate user ID: %w", err)
	}
	user := &domain.User{ID: id, Email: email, PasswordHash: string(hash)}
	if err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (*Session, error) {
	email, err := normalizeEmail(email)
	if err != nil || !validLoginPassword(password) {
		return nil, ErrInvalidCredentials
	}
	user, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("load user: %w", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	familyID, err := newUserID()
	if err != nil {
		return nil, fmt.Errorf("generate token family ID: %w", err)
	}
	return s.newSession(ctx, user, familyID)
}

func normalizeEmail(email string) (string, error) {
	email = strings.TrimSpace(email)
	if email == "" || utf8.RuneCountInString(email) > 320 {
		return "", errors.New("email is required and must be at most 320 characters")
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || !strings.Contains(email, "@") {
		return "", errors.New("email must be a valid address")
	}
	return strings.ToLower(email), nil
}

// validLoginPassword is deliberately looser than the registration policy so
// accounts created under older, weaker rules can still sign in.
func validLoginPassword(password string) bool {
	return len(password) > 0 && len(password) <= MaxPasswordBytes
}

func newUserID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	var encoded [36]byte
	hex.Encode(encoded[0:8], raw[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], raw[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], raw[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], raw[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], raw[10:16])
	return string(encoded[:]), nil
}

type UserClaims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

type JWTManager struct {
	keys   *KeySet
	issuer string
	ttl    time.Duration
	now    func() time.Time
}

func NewJWTManager(keys *KeySet, issuer string, ttl time.Duration) (*JWTManager, error) {
	if keys == nil {
		return nil, errors.New("JWT key set is required")
	}
	if strings.TrimSpace(issuer) == "" {
		return nil, errors.New("JWT_ISSUER must not be empty")
	}
	if ttl <= 0 {
		return nil, errors.New("JWT_TTL must be a positive duration")
	}
	return &JWTManager{keys: keys, issuer: issuer, ttl: ttl, now: time.Now}, nil
}

// JWKS exposes the public verification keys for /.well-known/jwks.json.
func (m *JWTManager) JWKS() JWKS { return m.keys.JWKS() }

func (m *JWTManager) Issue(user *domain.User) (string, time.Time, error) {
	now := m.now().UTC().Truncate(time.Second)
	expiresAt := now.Add(m.ttl)
	claims := UserClaims{
		UserID: user.ID,
		Email:  user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	key, kid := m.keys.SigningKey()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(key)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}
	return signed, expiresAt, nil
}

// Parse verifies a token against the published keys (selected by its kid).
func (m *JWTManager) Parse(rawToken string) (*UserClaims, error) {
	claims := &UserClaims{}
	token, err := jwt.ParseWithClaims(rawToken, claims, func(token *jwt.Token) (any, error) {
		kid, _ := token.Header["kid"].(string)
		key, ok := m.keys.PublicKey(kid)
		if !ok {
			return nil, fmt.Errorf("unknown signing key %q", kid)
		}
		return key, nil
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer(m.issuer), jwt.WithExpirationRequired())
	if err != nil {
		return nil, fmt.Errorf("parse access token: %w", err)
	}
	if !token.Valid {
		return nil, errors.New("invalid access token")
	}
	return claims, nil
}
