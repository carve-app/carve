package auth

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/carve-app/carve/services/api/internal/mailer"
	"github.com/jackc/pgx/v5"
)

const verifyTokenTTL = 7 * 24 * time.Hour

func appBaseURL() string {
	base := strings.TrimRight(os.Getenv("APP_BASE_URL"), "/")
	if base == "" {
		base = "http://localhost:5173"
	}
	return base
}

func (h *Handler) sendVerification(ctx context.Context, email, rawToken string) error {
	if h.mailer == nil {
		return nil
	}
	url := appBaseURL() + "/verify-email?token=" + rawToken
	return h.mailer.Send(ctx, mailer.Message{
		To: email, Subject: "Verify your Carve email",
		Text: "Verify your Carve email by opening this link:\n\n" + url + "\n\nThis link expires in 7 days.",
	})
}

// IssueVerificationToken creates a fresh single-use email-verification
// token for the given user. It returns the *unhashed* raw token (which
// the caller emails to the user) — only the SHA-256 hash is stored.
//
// Callers should send the verification link as something like:
//
//	https://carve.app/verify-email?token=<raw>
func (h *Handler) IssueVerificationToken(ctx context.Context, userID string) (string, error) {
	raw, _, err := NewRefreshToken()
	if err != nil {
		return "", err
	}
	hash := HashRefreshToken(raw)
	exp := h.now().Add(verifyTokenTTL)

	if _, err := h.db.Exec(ctx,
		`INSERT INTO email_verification_tokens (user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (token_hash) DO NOTHING`,
		userID, hash, exp,
	); err != nil {
		return "", err
	}
	return raw, nil
}

// POST /v1/auth/verify
// Body: {"token": "<raw>"}
// Marks the corresponding user as verified. Idempotent: re-verifying is
// a no-op success so users who click the email twice don't see an error.
func (h *Handler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}

	tokenHash := HashRefreshToken(req.Token)
	ctx := r.Context()

	var (
		id        string
		userID    string
		expiresAt time.Time
		usedAt    *time.Time
	)
	err := h.db.QueryRow(ctx,
		`SELECT id, user_id, expires_at, used_at
		 FROM email_verification_tokens
		 WHERE token_hash = $1`,
		tokenHash,
	).Scan(&id, &userID, &expiresAt, &usedAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusBadRequest, "invalid or expired token")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if h.now().After(expiresAt) {
		writeError(w, http.StatusBadRequest, "token expired")
		return
	}

	// Mark token used (idempotent) and flip the user's verified-at column.
	if _, err := h.db.Exec(ctx,
		`UPDATE email_verification_tokens SET used_at = COALESCE(used_at, now()) WHERE id = $1`, id,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if _, err := h.db.Exec(ctx,
		`UPDATE users SET email_verified_at = COALESCE(email_verified_at, now()) WHERE id = $1`, userID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"verified": true})
}

// POST /v1/auth/verify/resend
// Body: {"email": "<address>"}
// Issues a new verification token for an unverified address. Always
// returns 200 so attackers can't enumerate registered emails.
func (h *Handler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Email == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	ctx := r.Context()
	var userID string
	var verifiedAt *time.Time
	if err := h.db.QueryRow(ctx,
		`SELECT id, email_verified_at FROM users WHERE email = $1 AND deleted_at IS NULL`,
		req.Email,
	).Scan(&userID, &verifiedAt); err != nil || verifiedAt != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	rawToken, err := h.IssueVerificationToken(ctx, userID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	verifyURL := appBaseURL() + "/verify-email?token=" + rawToken

	// Returning the raw token in the HTTP body would let anyone who knows a
	// victim's email bypass the email-ownership check entirely. Only expose it
	// when an explicit test flag is set (CI/dev) — never in production, where
	// APP_BASE_URL is always set. Production and local full-stack development
	// deliver through the configured mailer.
	if os.Getenv("EXPOSE_VERIFY_TOKENS") == "1" {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":              true,
			"verify_url_test": verifyURL,
		})
		return
	}

	if err := h.sendVerification(ctx, req.Email, rawToken); err != nil {
		slog.Error("send email verification", "error", err, "user_id", userID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
