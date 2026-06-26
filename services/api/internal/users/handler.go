package users

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/carve-app/carve/services/api/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	db *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// GET /users/me
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var user struct {
		ID          string    `json:"id"`
		Email       string    `json:"email"`
		DisplayName string    `json:"display_name"`
		AvatarURL   *string   `json:"avatar_url"`
		CreatedAt   time.Time `json:"created_at"`
	}

	// created_at is timestamptz — it MUST be scanned into time.Time, not string,
	// or pgx returns a scan error. The old code scanned into a string and then
	// reported ANY error as 404, so a valid logged-in user got "user not found"
	// and the web app bounced them to /login. Distinguish no-rows (404) from a
	// real query/scan error (500) so this can't masquerade again.
	err := h.db.QueryRow(r.Context(),
		`SELECT id, email, display_name, avatar_url, created_at
		 FROM users WHERE id = $1 AND deleted_at IS NULL`,
		claims.UserID,
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.AvatarURL, &user.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		slog.Error("users.Me query", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, user)
}

// DELETE /users/me — soft-deletes the account and revokes all refresh tokens (GDPR).
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	ctx := r.Context()
	_, err := h.db.Exec(ctx,
		`UPDATE users SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`,
		claims.UserID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.db.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`,
		claims.UserID,
	)

	w.WriteHeader(http.StatusNoContent)
}

// PATCH /users/me
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		DisplayName *string `json:"display_name"`
		AvatarURL   *string `json:"avatar_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.DisplayName != nil {
		h.db.Exec(r.Context(),
			`UPDATE users SET display_name = $1, updated_at = now() WHERE id = $2`,
			*req.DisplayName, claims.UserID,
		)
	}
	if req.AvatarURL != nil {
		h.db.Exec(r.Context(),
			`UPDATE users SET avatar_url = $1, updated_at = now() WHERE id = $2`,
			*req.AvatarURL, claims.UserID,
		)
	}

	h.Me(w, r) // return updated user
}
