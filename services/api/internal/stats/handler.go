package stats

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

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

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// GET /v1/stats?language=ja
func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	language := r.URL.Query().Get("language")
	if language == "" {
		language = "ja"
	}

	ctx := r.Context()
	now := time.Now().UTC()
	thirtyDaysAgo := now.AddDate(0, 0, -30)

	// ── Known & learning card counts ──────────────────────────────────────────

	var knownCount, learningCount int
	h.db.QueryRow(ctx,
		`SELECT
		    COUNT(*) FILTER (WHERE fsrs_state IN ('review') AND fsrs_lapses < 8),
		    COUNT(*) FILTER (WHERE fsrs_state IN ('learning','relearning'))
		 FROM cards
		 WHERE user_id = $1 AND language_code = $2 AND deleted_at IS NULL AND suspended = FALSE`,
		claims.UserID, language,
	).Scan(&knownCount, &learningCount)

	// ── Retention rate (last 30 days) ─────────────────────────────────────────

	var totalReviews, successReviews int
	h.db.QueryRow(ctx,
		`SELECT COUNT(*), COUNT(*) FILTER (WHERE rating >= 2)
		 FROM review_events re
		 JOIN cards c ON c.id = re.card_id
		 WHERE re.user_id = $1
		   AND c.language_code = $2
		   AND re.reviewed_at >= $3`,
		claims.UserID, language, thirtyDaysAgo,
	).Scan(&totalReviews, &successReviews)

	retention := 0.0
	if totalReviews > 0 {
		retention = float64(successReviews) / float64(totalReviews)
	}

	// ── Immersion time (last 30 days, reading + listening seconds) ────────────

	var readingSec, listeningSec int
	h.db.QueryRow(ctx,
		`SELECT
		    COALESCE(SUM(duration_sec) FILTER (WHERE session_type = 'reading'), 0),
		    COALESCE(SUM(duration_sec) FILTER (WHERE session_type = 'listening'), 0)
		 FROM immersion_sessions
		 WHERE user_id = $1 AND language_code = $2 AND started_at >= $3`,
		claims.UserID, language, thirtyDaysAgo,
	).Scan(&readingSec, &listeningSec)

	// ── Streak: consecutive calendar days with at least one review ────────────

	rows, err := h.db.Query(ctx,
		`SELECT DISTINCT date_trunc('day', reviewed_at AT TIME ZONE 'UTC')::date AS day
		 FROM review_events re
		 JOIN cards c ON c.id = re.card_id
		 WHERE re.user_id = $1 AND c.language_code = $2
		 ORDER BY day DESC
		 LIMIT 365`,
		claims.UserID, language,
	)
	streak := 0
	if err == nil {
		defer rows.Close()
		today := now.Truncate(24 * time.Hour)
		expected := today
		for rows.Next() {
			var day time.Time
			if rows.Scan(&day) != nil {
				break
			}
			day = day.UTC()
			if day.Equal(expected) || day.Equal(expected.AddDate(0, 0, -1)) {
				streak++
				expected = day
			} else if day.Before(expected.AddDate(0, 0, -1)) {
				break
			}
		}
		// If no review today, the streak may have started yesterday.
	}

	// ── Word count snapshots (last 90 days for growth chart) ─────────────────

	snapRows, snapErr := h.db.Query(ctx,
		`SELECT snapshotted_at, known_count, mature_count, learning_count
		 FROM word_count_snapshots
		 WHERE user_id = $1 AND language_code = $2
		   AND snapshotted_at >= $3
		 ORDER BY snapshotted_at ASC`,
		claims.UserID, language, now.AddDate(0, 0, -90),
	)

	type snapEntry struct {
		Date          string `json:"date"`
		KnownCount    int    `json:"known_count"`
		MatureCount   int    `json:"mature_count"`
		LearningCount int    `json:"learning_count"`
	}
	var snapshots []snapEntry
	if snapErr == nil {
		defer snapRows.Close()
		for snapRows.Next() {
			var e snapEntry
			var d time.Time
			if snapRows.Scan(&d, &e.KnownCount, &e.MatureCount, &e.LearningCount) == nil {
				e.Date = d.Format("2006-01-02")
				snapshots = append(snapshots, e)
			}
		}
	}

	// ── Total reviews ever ────────────────────────────────────────────────────

	var totalEverReviews int
	h.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM review_events re
		 JOIN cards c ON c.id = re.card_id
		 WHERE re.user_id = $1 AND c.language_code = $2`,
		claims.UserID, language,
	).Scan(&totalEverReviews)

	writeJSON(w, http.StatusOK, map[string]any{
		"language_code":      language,
		"known_cards":        knownCount,
		"learning_cards":     learningCount,
		"retention_30d":      retention,
		"total_reviews":      totalReviews,
		"streak_days":        streak,
		"reading_minutes":    readingSec / 60,
		"listening_minutes":  listeningSec / 60,
		"total_ever_reviews": totalEverReviews,
		"word_growth":        snapshots,
	})
}

// TakeWordCountSnapshot records a word-count snapshot for a user+language today.
// Called by the daily background job.
func TakeWordCountSnapshot(ctx context.Context, db *pgxpool.Pool, userID, language string) error {
	today := time.Now().UTC().Truncate(24 * time.Hour)

	var known, mature, learning int
	err := db.QueryRow(ctx,
		`SELECT
		    COUNT(*) FILTER (WHERE fsrs_state = 'review'),
		    COUNT(*) FILTER (WHERE fsrs_state = 'review' AND fsrs_lapses = 0 AND fsrs_reps >= 3),
		    COUNT(*) FILTER (WHERE fsrs_state IN ('learning','relearning'))
		 FROM cards
		 WHERE user_id = $1 AND language_code = $2 AND deleted_at IS NULL AND suspended = FALSE`,
		userID, language,
	).Scan(&known, &mature, &learning)
	if err != nil {
		return err
	}

	_, err = db.Exec(ctx,
		`INSERT INTO word_count_snapshots
		    (id, user_id, language_code, snapshotted_at, known_count, mature_count, learning_count)
		 VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6)
		 ON CONFLICT (user_id, language_code, snapshotted_at) DO UPDATE SET
		   known_count    = EXCLUDED.known_count,
		   mature_count   = EXCLUDED.mature_count,
		   learning_count = EXCLUDED.learning_count`,
		userID, language, today, known, mature, learning,
	)
	return err
}

// SnapshotAllUsers writes a daily word-count snapshot for every active user.
// Designed to be called once per day from a background goroutine.
func SnapshotAllUsers(ctx context.Context, db *pgxpool.Pool) {
	rows, err := db.Query(ctx,
		`SELECT DISTINCT re.user_id, c.language_code
		 FROM review_events re
		 JOIN cards c ON c.id = re.card_id
		 WHERE re.reviewed_at >= now() - interval '30 days'`,
	)
	if err != nil {
		slog.Error("snapshot: query active users", "error", err)
		return
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var userID, lang string
		if err := rows.Scan(&userID, &lang); err != nil {
			continue
		}
		if err := TakeWordCountSnapshot(ctx, db, userID, lang); err != nil {
			slog.Warn("snapshot: failed for user", "user_id", userID, "lang", lang, "error", err)
		} else {
			count++
		}
	}
	slog.Info("word count snapshots done", "users_snapshotted", count)
}
