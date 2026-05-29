package auth_test

import (
	"testing"
	"time"

	"github.com/carve-app/carve/services/api/internal/auth"
)

func TestIssueAndParseAccessToken(t *testing.T) {
	token, _, err := auth.IssueAccessToken("user-123", "test@example.com")
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}

	claims, err := auth.ParseAccessToken(token)
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}
	if claims.UserID != "user-123" {
		t.Errorf("UserID: got %q, want %q", claims.UserID, "user-123")
	}
	if claims.Email != "test@example.com" {
		t.Errorf("Email: got %q, want %q", claims.Email, "test@example.com")
	}
}

func TestParseInvalidToken(t *testing.T) {
	_, err := auth.ParseAccessToken("not.a.valid.jwt")
	if err != auth.ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestParseEmptyToken(t *testing.T) {
	_, err := auth.ParseAccessToken("")
	if err != auth.ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken for empty string, got %v", err)
	}
}

func TestRefreshTokenRoundtrip(t *testing.T) {
	raw, hashed, err := auth.NewRefreshToken()
	if err != nil {
		t.Fatalf("NewRefreshToken: %v", err)
	}
	if raw == "" || hashed == "" {
		t.Fatal("expected non-empty raw and hashed tokens")
	}
	if raw == hashed {
		t.Fatal("raw and hashed should differ")
	}

	// Hashing the same raw token must produce the same hash
	hashed2 := auth.HashRefreshToken(raw)
	if hashed != hashed2 {
		t.Errorf("hash mismatch: %q vs %q", hashed, hashed2)
	}
}

func TestRefreshTokensAreUnique(t *testing.T) {
	raw1, _, _ := auth.NewRefreshToken()
	raw2, _, _ := auth.NewRefreshToken()
	if raw1 == raw2 {
		t.Fatal("two refresh tokens should not be equal")
	}
}

func TestAccessTokenTTL(t *testing.T) {
	_, exp, err := auth.IssueAccessToken("u", "e@e.com")
	if err != nil {
		t.Fatal(err)
	}
	// TTL should be ~15 minutes from now
	ttl := time.Until(exp)
	if ttl < 14*time.Minute || ttl > 16*time.Minute {
		t.Errorf("unexpected TTL: %v (expected ~15min)", ttl)
	}
}

func TestNewID(t *testing.T) {
	id1 := auth.NewID()
	id2 := auth.NewID()
	if id1 == id2 {
		t.Fatal("NewID should produce unique values")
	}
	if len(id1) != 36 {
		t.Errorf("expected UUID length 36, got %d", len(id1))
	}
}
