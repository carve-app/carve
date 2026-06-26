package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/carve-app/carve/services/api/internal/mailer"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// isUniqueViolation reports whether err is a PostgreSQL unique-constraint
// violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

const refreshTokenCookie = "carve_refresh"

type Handler struct {
	db     *pgxpool.Pool
	mailer mailer.Sender
	clock  Clock
}

type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func NewHandler(db *pgxpool.Pool) *Handler {
	return NewHandlerWithMailer(db, mailer.NewFromEnv())
}

func NewHandlerWithMailer(db *pgxpool.Pool, sender mailer.Sender) *Handler {
	return NewHandlerWithDependencies(db, sender, systemClock{})
}

func NewHandlerWithDependencies(db *pgxpool.Pool, sender mailer.Sender, clock Clock) *Handler {
	return &Handler{db: db, mailer: sender, clock: clock}
}

func (h *Handler) now() time.Time {
	if h.clock == nil {
		return time.Now()
	}
	return h.clock.Now()
}

// ── Request / response types ──────────────────────────────────────────────────

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"` // seconds
	// RefreshToken is also returned in the body (in addition to the HttpOnly
	// cookie) for clients that can't rely on cookies — notably the browser
	// extension, whose service-worker fetches to the API origin are cross-site
	// and don't carry the SameSite cookie. The web app ignores this and uses
	// the cookie. Treat it like a credential: store it in extension-local
	// storage, never expose it to page context.
	RefreshToken string      `json:"refresh_token"`
	User         userPayload `json:"user"`
}

type userPayload struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// cookieSecure reports whether the refresh cookie should carry the Secure flag.
// A Secure cookie is dropped by browsers over plain http, which silently breaks
// refresh in local dev (http://localhost). Default to secure; disable it only
// when COOKIE_INSECURE=1 (set in dev/compose) so localhost works while prod
// stays Secure.
func cookieSecure() bool {
	return os.Getenv("COOKIE_INSECURE") != "1"
}

func setRefreshCookie(w http.ResponseWriter, raw string, exp time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshTokenCookie,
		Value:    raw,
		Expires:  exp,
		HttpOnly: true,
		Secure:   cookieSecure(),
		// Lax (not Strict): the web app and API are different localhost ports
		// (and different subdomains in prod), so the refresh POST is
		// cross-site; Strict would withhold the cookie. Lax still sends it on
		// top-level + same-site requests, which covers /v1/auth/refresh.
		SameSite: http.SameSiteLaxMode,
		Path:     "/v1/auth",
	})
}

func (h *Handler) issueTokenPair(
	ctx context.Context,
	w http.ResponseWriter,
	userID, email, displayName string,
) error {
	access, _, err := issueAccessTokenAt(userID, email, h.now())
	if err != nil {
		return fmt.Errorf("IssueAccessToken: %w", err)
	}

	rawRefresh, hashedRefresh, err := NewRefreshToken()
	if err != nil {
		return fmt.Errorf("NewRefreshToken: %w", err)
	}
	refreshExp := h.now().Add(RefreshTokenTTL)

	_, err = h.db.Exec(ctx,
		`INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3, $4)`,
		NewID(), userID, hashedRefresh, refreshExp,
	)
	if err != nil {
		return fmt.Errorf("store refresh token: %w", err)
	}

	setRefreshCookie(w, rawRefresh, refreshExp)
	writeJSON(w, http.StatusOK, authResponse{
		AccessToken:  access,
		ExpiresIn:    int(AccessTokenTTL.Seconds()),
		RefreshToken: rawRefresh,
		User:         userPayload{ID: userID, Email: email, DisplayName: displayName},
	})
	return nil
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// POST /auth/register
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Email == "" || req.Password == "" || req.DisplayName == "" {
		writeError(w, http.StatusBadRequest, "email, password, and display_name are required")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if len([]byte(req.Password)) > 72 {
		writeError(w, http.StatusBadRequest, "password must be at most 72 bytes")
		return
	}
	if len(req.Email) > 254 || len(req.DisplayName) > 80 || strings.ContainsRune(req.Email, '\x00') || strings.ContainsRune(req.DisplayName, '\x00') {
		writeError(w, http.StatusBadRequest, "invalid registration fields")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	userID := NewID()
	ctx := r.Context()

	// Create the user and its password row atomically so a failure of the
	// second insert can't leave an orphaned users row that permanently locks
	// the email out of both registration and login.
	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx,
		`INSERT INTO users (id, email, display_name) VALUES ($1, $2, $3)`,
		userID, req.Email, req.DisplayName,
	)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "email already in use")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO user_auth (id, user_id, provider, password_hash)
		 VALUES ($1, $2, 'email', $3)`,
		NewID(), userID, string(hash),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Verification delivery is best-effort; resend remains available if SMTP is
	// temporarily unavailable. No session is issued until ownership is proven.
	rawTokenForTest := ""
	if rawToken, err := h.IssueVerificationToken(ctx, userID); err != nil {
		slog.Error("issue registration verification", "error", err, "user_id", userID)
	} else {
		rawTokenForTest = rawToken
		if err := h.sendVerification(ctx, req.Email, rawToken); err != nil {
			slog.Error("send registration verification", "error", err, "user_id", userID)
		}
	}

	response := map[string]any{
		"verification_required": true,
		"email":                 req.Email,
	}
	if os.Getenv("EXPOSE_VERIFY_TOKENS") == "1" && rawTokenForTest != "" {
		response["verification_token_test"] = rawTokenForTest
	}
	writeJSON(w, http.StatusCreated, response)
}

// POST /auth/login
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	ctx := r.Context()
	var (
		userID      string
		displayName string
		pwHash      string
		verifiedAt  *time.Time
	)
	err := h.db.QueryRow(ctx,
		`SELECT u.id, u.display_name, a.password_hash, u.email_verified_at
		 FROM users u
		 JOIN user_auth a ON a.user_id = u.id AND a.provider = 'email'
		 WHERE u.email = $1 AND u.deleted_at IS NULL`,
		req.Email,
	).Scan(&userID, &displayName, &pwHash, &verifiedAt)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(pwHash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	if verifiedAt == nil {
		writeError(w, http.StatusForbidden, "email verification required")
		return
	}

	if err := h.issueTokenPair(ctx, w, userID, req.Email, displayName); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
}

// POST /auth/refresh
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	// Accept the refresh token from the HttpOnly cookie (web app) OR a JSON
	// body {"refresh_token": "..."} (browser extension, which can't rely on
	// cross-site cookies). Cookie takes precedence when both are present.
	raw := ""
	if cookie, err := r.Cookie(refreshTokenCookie); err == nil {
		raw = cookie.Value
	}
	if raw == "" && r.Body != nil {
		var body struct {
			RefreshToken string `json:"refresh_token"`
		}
		if json.NewDecoder(r.Body).Decode(&body) == nil {
			raw = body.RefreshToken
		}
	}
	if raw == "" {
		writeError(w, http.StatusUnauthorized, "no refresh token")
		return
	}

	hashed := HashRefreshToken(raw)
	ctx := r.Context()

	var (
		userID      string
		email       string
		displayName string
		expiresAt   time.Time
		revokedAt   *time.Time
	)
	err := h.db.QueryRow(ctx,
		`SELECT u.id, u.email, u.display_name, rt.expires_at, rt.revoked_at
		 FROM refresh_tokens rt
		 JOIN users u ON u.id = rt.user_id
		 WHERE rt.token_hash = $1`,
		hashed,
	).Scan(&userID, &email, &displayName, &expiresAt, &revokedAt)
	if err != nil || revokedAt != nil || h.now().After(expiresAt) {
		writeError(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}

	// Rotate atomically: only the request that flips revoked_at from NULL wins.
	// If RowsAffected()==0 the token was already revoked — treat it as a reuse
	// event, revoke the whole token family, and reject.
	tag, err := h.db.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now()
		 WHERE token_hash = $1 AND revoked_at IS NULL`,
		hashed,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if tag.RowsAffected() == 0 {
		// Another request may have rotated this token concurrently. Reject this
		// reuse, but do not revoke the winner's newly-issued token: without a
		// persisted token-family identifier that would turn ordinary concurrent
		// browser requests into a logout of the valid session.
		writeError(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}

	if err := h.issueTokenPair(ctx, w, userID, email, displayName); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
}

// POST /auth/logout
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	raw := ""
	if cookie, err := r.Cookie(refreshTokenCookie); err == nil {
		raw = cookie.Value
	}
	if raw == "" && r.Body != nil {
		var body struct {
			RefreshToken string `json:"refresh_token"`
		}
		if json.NewDecoder(r.Body).Decode(&body) == nil {
			raw = body.RefreshToken
		}
	}
	if raw != "" {
		hashed := HashRefreshToken(raw)
		h.db.Exec(r.Context(),
			`UPDATE refresh_tokens SET revoked_at = now() WHERE token_hash = $1`,
			hashed,
		)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     refreshTokenCookie,
		Value:    "",
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
		Path:     "/v1/auth",
		Secure:   cookieSecure(),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}
