package users

import (
	"encoding/json"
	"net/http"

	"github.com/carve-app/carve/services/api/internal/auth"
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

// GET /users/me
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var user struct {
		ID          string  `json:"id"`
		Email       string  `json:"email"`
		DisplayName string  `json:"display_name"`
		AvatarURL   *string `json:"avatar_url"`
		CreatedAt   string  `json:"created_at"`
	}

	err := h.db.QueryRow(r.Context(),
		`SELECT id, email, display_name, avatar_url, created_at
		 FROM users WHERE id = $1 AND deleted_at IS NULL`,
		claims.UserID,
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.AvatarURL, &user.CreatedAt)
	if err != nil {
		http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, user)
}

// PATCH /users/me
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req struct {
		DisplayName *string `json:"display_name"`
		AvatarURL   *string `json:"avatar_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
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
