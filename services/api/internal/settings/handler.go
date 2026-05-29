package settings

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/carve-app/carve/services/api/internal/auth"
	"github.com/carve-app/carve/services/api/internal/fsrs"
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

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ── GET /v1/settings/fsrs ─────────────────────────────────────────────────────

func (h *Handler) GetFSRS(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	language := r.URL.Query().Get("language")
	if language == "" {
		language = "ja"
	}

	defaults := fsrs.DefaultParams()
	type fsrsSettings struct {
		Language        string    `json:"language_code"`
		Weights         []float64 `json:"weights"`
		TargetRetention float64   `json:"target_retention"`
		LeechThreshold  int       `json:"leech_threshold"`
		DailyNewLimit   int       `json:"daily_new_limit"`
		UpdatedAt       time.Time `json:"updated_at"`
		IsCustomized    bool      `json:"is_customized"`
	}

	var s fsrsSettings
	var weightsJSON []byte
	err := h.db.QueryRow(r.Context(),
		`SELECT weights, target_retention, leech_threshold, daily_new_limit, updated_at
		 FROM user_fsrs_params
		 WHERE user_id = $1 AND language_code = $2`,
		claims.UserID, language,
	).Scan(&weightsJSON, &s.TargetRetention, &s.LeechThreshold, &s.DailyNewLimit, &s.UpdatedAt)

	if err != nil {
		// Return defaults
		s.Language = language
		s.Weights = defaults.W[:]
		s.TargetRetention = defaults.TargetRetention
		s.LeechThreshold = fsrs.DefaultLeechThreshold
		s.DailyNewLimit = 20
		s.IsCustomized = false
		writeJSON(w, http.StatusOK, s)
		return
	}

	var w2 []float64
	if json.Unmarshal(weightsJSON, &w2) == nil && len(w2) == 19 {
		s.Weights = w2
	} else {
		s.Weights = defaults.W[:]
	}
	s.Language = language
	s.IsCustomized = true

	writeJSON(w, http.StatusOK, s)
}

// ── PUT /v1/settings/fsrs ─────────────────────────────────────────────────────

func (h *Handler) PutFSRS(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Language        string    `json:"language_code"`
		Weights         []float64 `json:"weights"`
		TargetRetention *float64  `json:"target_retention"`
		LeechThreshold  *int      `json:"leech_threshold"`
		DailyNewLimit   *int      `json:"daily_new_limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Language == "" {
		req.Language = "ja"
	}

	defaults := fsrs.DefaultParams()

	weights := defaults.W[:]
	if len(req.Weights) == 19 {
		weights = req.Weights
	}
	weightsJSON, _ := json.Marshal(weights)

	targetRetention := defaults.TargetRetention
	if req.TargetRetention != nil && *req.TargetRetention > 0.7 && *req.TargetRetention < 0.99 {
		targetRetention = *req.TargetRetention
	}

	leechThreshold := fsrs.DefaultLeechThreshold
	if req.LeechThreshold != nil && *req.LeechThreshold >= 1 && *req.LeechThreshold <= 20 {
		leechThreshold = *req.LeechThreshold
	}

	dailyNewLimit := 20
	if req.DailyNewLimit != nil && *req.DailyNewLimit >= 0 && *req.DailyNewLimit <= 9999 {
		dailyNewLimit = *req.DailyNewLimit
	}

	ctx := r.Context()
	id := auth.NewID()
	_, err := h.db.Exec(ctx,
		`INSERT INTO user_fsrs_params
		    (id, user_id, language_code, weights, target_retention, leech_threshold, daily_new_limit, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,now())
		 ON CONFLICT (user_id, language_code) DO UPDATE SET
		   weights          = EXCLUDED.weights,
		   target_retention = EXCLUDED.target_retention,
		   leech_threshold  = EXCLUDED.leech_threshold,
		   daily_new_limit  = EXCLUDED.daily_new_limit,
		   updated_at       = now()`,
		id, claims.UserID, req.Language, weightsJSON,
		targetRetention, leechThreshold, dailyNewLimit,
	)
	if err != nil {
		slog.Error("upsert fsrs params", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Update target_retention in user_languages too
	h.db.Exec(ctx,
		`UPDATE user_languages SET target_retention = $1
		 WHERE user_id = $2 AND language_code = $3`,
		targetRetention, claims.UserID, req.Language,
	)

	writeJSON(w, http.StatusOK, map[string]any{
		"updated":          true,
		"target_retention": targetRetention,
		"leech_threshold":  leechThreshold,
		"daily_new_limit":  dailyNewLimit,
	})
}

// ── GET /v1/settings/workload-preview ────────────────────────────────────────
// Returns projected workload if target retention changes.

func (h *Handler) WorkloadPreview(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	language := r.URL.Query().Get("language")
	if language == "" {
		language = "ja"
	}
	retentionStr := r.URL.Query().Get("target_retention")
	newRetention := 0.90
	if retentionStr != "" {
		var f float64
		if err := json.Unmarshal([]byte(retentionStr), &f); err == nil {
			if f > 0.7 && f < 0.99 {
				newRetention = f
			}
		}
	}

	// Fetch all review cards' stabilities
	rows, err := h.db.Query(r.Context(),
		`SELECT fsrs_stability FROM cards
		 WHERE user_id = $1 AND language_code = $2
		   AND deleted_at IS NULL AND suspended = FALSE
		   AND fsrs_state = 'review'`,
		claims.UserID, language,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()

	type preview struct {
		TargetRetention  float64 `json:"target_retention"`
		AvgIntervalDays  float64 `json:"avg_interval_days"`
		TotalReviewCards int     `json:"total_review_cards"`
	}

	var stabilities []float64
	for rows.Next() {
		var s float64
		if rows.Scan(&s) == nil && s > 0 {
			stabilities = append(stabilities, s)
		}
	}

	// Calculate average projected interval
	var sumIntervals float64
	for _, s := range stabilities {
		// interval = S when r=0.9; scale by ratio
		interval := s * (1 - newRetention) / (1 - 0.9)
		if interval < 1 {
			interval = 1
		}
		sumIntervals += interval
	}
	avgInterval := 0.0
	if len(stabilities) > 0 {
		avgInterval = sumIntervals / float64(len(stabilities))
	}

	writeJSON(w, http.StatusOK, preview{
		TargetRetention:  newRetention,
		AvgIntervalDays:  avgInterval,
		TotalReviewCards: len(stabilities),
	})
}
