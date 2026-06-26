package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	// Access tokens are short-lived bearer JWTs. 15 minutes was far too short
	// for a study app — users hit "session expired" mid-review. 4 hours covers
	// a long immersion/review session; the rotating 30-day refresh token (used
	// transparently by the clients on 401) keeps the session alive beyond that
	// without re-login, so a leaked access token is still only valuable briefly.
	AccessTokenTTL  = 4 * time.Hour
	RefreshTokenTTL = 30 * 24 * time.Hour
)

var ErrInvalidToken = errors.New("invalid token")

type Claims struct {
	UserID string `json:"uid"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// MinJWTSecretLen is the minimum acceptable length for JWT_SECRET. The server
// refuses to start (see RequireJWTSecret) with a shorter or empty secret, so
// the dev fallback below is only ever reachable under `go test`.
const MinJWTSecretLen = 32

func jwtSecret() []byte {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		// Dev/test-only fallback. Production cannot reach this branch:
		// cmd/api/main.go calls RequireJWTSecret at startup and exits if
		// JWT_SECRET is unset or too short.
		secret = "dev-secret-change-in-production-at-least-32-chars"
	}
	return []byte(secret)
}

// RequireJWTSecret returns an error if JWT_SECRET is unset or shorter than
// MinJWTSecretLen. The server's entrypoint calls this before serving traffic so
// a misconfigured deployment fails loudly instead of silently signing tokens
// with the dev fallback (which would allow trivial token forgery).
func RequireJWTSecret() error {
	secret := os.Getenv("JWT_SECRET")
	if len(secret) < MinJWTSecretLen {
		return fmt.Errorf("JWT_SECRET must be set and at least %d bytes (got %d)", MinJWTSecretLen, len(secret))
	}
	return nil
}

// IssueAccessToken creates a signed JWT for the given user.
func IssueAccessToken(userID, email string) (string, time.Time, error) {
	return issueAccessTokenAt(userID, email, time.Now())
}

func issueAccessTokenAt(userID, email string, now time.Time) (string, time.Time, error) {
	exp := now.Add(AccessTokenTTL)
	claims := Claims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			Issuer:    "carve-api",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(jwtSecret())
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return signed, exp, nil
}

// ParseAccessToken validates and parses a JWT, returning its claims.
func ParseAccessToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return jwtSecret(), nil
	})
	if err != nil {
		return nil, ErrInvalidToken
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// NewRefreshToken generates a cryptographically random opaque token.
// Returns the raw token (for the cookie) and its SHA-256 hash (for storage).
func NewRefreshToken() (raw string, hashed string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", fmt.Errorf("rand.Read: %w", err)
	}
	raw = hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(raw))
	hashed = hex.EncodeToString(sum[:])
	return raw, hashed, nil
}

// HashRefreshToken returns the SHA-256 hex hash of a raw refresh token.
func HashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// NewID returns a new random UUID string.
func NewID() string {
	return uuid.New().String()
}
