package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const resetTokenTTL = 1 * time.Hour

// POST /v1/auth/forgot
// Accepts {"email": "..."}.
// Always returns 204 (no information leakage about whether the email exists).
// In production, SMTP/transactional email sends the reset link; here we log it.
func (h *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		w.WriteHeader(http.StatusNoContent) // never reveal missing email
		return
	}

	ctx := r.Context()

	var userID string
	err := h.db.QueryRow(ctx,
		`SELECT id FROM users WHERE email = $1 AND deleted_at IS NULL`,
		req.Email,
	).Scan(&userID)
	if err != nil {
		// User not found — still return 204 to avoid enumeration.
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Generate a random token.
	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	rawToken := hex.EncodeToString(rawBytes)
	tokenHash := HashRefreshToken(rawToken)

	exp := time.Now().Add(resetTokenTTL)
	_, err = h.db.Exec(ctx,
		`INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (token_hash) DO NOTHING`,
		NewID(), userID, tokenHash, exp,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	appBase := os.Getenv("APP_BASE_URL")
	if appBase == "" {
		appBase = "http://localhost:5173"
	}
	resetURL := appBase + "/reset-password?token=" + rawToken

	// In production: send email. Here we log so local testing works.
	slog.Info("password reset link", "email", req.Email, "url", resetURL)

	w.WriteHeader(http.StatusNoContent)
}

// POST /v1/auth/reset
// Accepts {"token": "...", "password": "..."}.
func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Token == "" || len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "token and password (min 8 chars) are required")
		return
	}

	tokenHash := HashRefreshToken(req.Token)
	ctx := r.Context()

	var (
		tokenID   string
		userID    string
		expiresAt time.Time
		usedAt    *time.Time
	)
	err := h.db.QueryRow(ctx,
		`SELECT id, user_id, expires_at, used_at
		 FROM password_reset_tokens
		 WHERE token_hash = $1`,
		tokenHash,
	).Scan(&tokenID, &userID, &expiresAt, &usedAt)
	if err != nil || usedAt != nil || time.Now().After(expiresAt) {
		writeError(w, http.StatusUnauthorized, "invalid or expired reset token")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Mark token used and update password in one transaction.
	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx,
		`UPDATE password_reset_tokens SET used_at = now() WHERE id = $1`, tokenID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	_, err = tx.Exec(ctx,
		`UPDATE user_auth SET password_hash = $1
		 WHERE user_id = $2 AND provider = 'email'`,
		string(hash), userID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
