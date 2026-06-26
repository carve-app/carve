package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/carve-app/carve/services/api/internal/mailer"
	"golang.org/x/crypto/bcrypt"
)

const resetTokenTTL = 1 * time.Hour

// POST /v1/auth/forgot
// Accepts {"email": "..."}.
// Always returns 204 (no information leakage about whether the email exists).
// Delivery uses the configured SMTP sender and never exposes the token in logs.
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

	exp := h.now().Add(resetTokenTTL)
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

	resetURL := appBaseURL() + "/reset-password?token=" + rawToken
	if h.mailer != nil {
		if err := h.mailer.Send(ctx, mailer.Message{
			To: req.Email, Subject: "Reset your Carve password",
			Text: "Reset your Carve password by opening this link:\n\n" + resetURL + "\n\nThis link expires in 1 hour.",
		}); err != nil {
			// Preserve the endpoint's anti-enumeration response while recording a
			// token-free operational error.
			slog.Error("send password reset", "error", err, "user_id", userID)
		}
	}

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
	if req.Token == "" || len(req.Password) < 8 || len([]byte(req.Password)) > 72 {
		writeError(w, http.StatusBadRequest, "token and password (8-72 bytes) are required")
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
	if err != nil || usedAt != nil || h.now().After(expiresAt) {
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
