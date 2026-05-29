package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const refreshTokenCookie = "carve_refresh"

type Handler struct {
	db *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
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
	AccessToken string      `json:"access_token"`
	ExpiresIn   int         `json:"expires_in"` // seconds
	User        userPayload `json:"user"`
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

func setRefreshCookie(w http.ResponseWriter, raw string, exp time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshTokenCookie,
		Value:    raw,
		Expires:  exp,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		Path:     "/v1/auth",
	})
}

func (h *Handler) issueTokenPair(
	ctx context.Context,
	w http.ResponseWriter,
	userID, email, displayName string,
) error {
	access, _, err := IssueAccessToken(userID, email)
	if err != nil {
		return fmt.Errorf("IssueAccessToken: %w", err)
	}

	rawRefresh, hashedRefresh, err := NewRefreshToken()
	if err != nil {
		return fmt.Errorf("NewRefreshToken: %w", err)
	}
	refreshExp := time.Now().Add(RefreshTokenTTL)

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
		AccessToken: access,
		ExpiresIn:   int(AccessTokenTTL.Seconds()),
		User:        userPayload{ID: userID, Email: email, DisplayName: displayName},
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

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	userID := NewID()
	ctx := r.Context()

	_, err = h.db.Exec(ctx,
		`INSERT INTO users (id, email, display_name) VALUES ($1, $2, $3)`,
		userID, req.Email, req.DisplayName,
	)
	if err != nil {
		// Duplicate email (unique constraint)
		writeError(w, http.StatusConflict, "email already in use")
		return
	}

	_, err = h.db.Exec(ctx,
		`INSERT INTO user_auth (id, user_id, provider, password_hash)
		 VALUES ($1, $2, 'email', $3)`,
		NewID(), userID, string(hash),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := h.issueTokenPair(ctx, w, userID, req.Email, req.DisplayName); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
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
	)
	err := h.db.QueryRow(ctx,
		`SELECT u.id, u.display_name, a.password_hash
		 FROM users u
		 JOIN user_auth a ON a.user_id = u.id AND a.provider = 'email'
		 WHERE u.email = $1 AND u.deleted_at IS NULL`,
		req.Email,
	).Scan(&userID, &displayName, &pwHash)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(pwHash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	if err := h.issueTokenPair(ctx, w, userID, req.Email, displayName); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
}

// POST /auth/refresh
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshTokenCookie)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "no refresh token")
		return
	}

	hashed := HashRefreshToken(cookie.Value)
	ctx := r.Context()

	var (
		userID      string
		email       string
		displayName string
		expiresAt   time.Time
		revokedAt   *time.Time
	)
	err = h.db.QueryRow(ctx,
		`SELECT u.id, u.email, u.display_name, rt.expires_at, rt.revoked_at
		 FROM refresh_tokens rt
		 JOIN users u ON u.id = rt.user_id
		 WHERE rt.token_hash = $1`,
		hashed,
	).Scan(&userID, &email, &displayName, &expiresAt, &revokedAt)
	if err != nil || revokedAt != nil || time.Now().After(expiresAt) {
		writeError(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}

	// Rotate: revoke old token, issue new pair
	h.db.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE token_hash = $1`,
		hashed,
	)
	if err := h.issueTokenPair(ctx, w, userID, email, displayName); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
}

// POST /auth/logout
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshTokenCookie)
	if err == nil {
		hashed := HashRefreshToken(cookie.Value)
		h.db.Exec(r.Context(),
			`UPDATE refresh_tokens SET revoked_at = now() WHERE token_hash = $1`,
			hashed,
		)
	}
	http.SetCookie(w, &http.Cookie{
		Name:    refreshTokenCookie,
		Value:   "",
		MaxAge:  -1,
		Path:    "/v1/auth",
		Secure:  true,
		HttpOnly: true,
	})
	w.WriteHeader(http.StatusNoContent)
}
