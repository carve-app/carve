package discover

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/carve-app/carve/services/api/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler exposes the /v1/discover endpoints.
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

// FeedItem is one article in the discover response.
type FeedItem struct {
	ID               string    `json:"id"`
	Source           string    `json:"source"`
	Title            string    `json:"title"`
	Summary          string    `json:"summary,omitempty"`
	URL              string    `json:"url"`
	PublishedAt      time.Time `json:"published_at,omitempty"`
	ComprehensionPct float64   `json:"comprehension_pct"`
	UnknownCount     int       `json:"unknown_count"`
	RecommendedMode  string    `json:"recommended_mode"`
	FitScore         float64   `json:"fit_score"`
}

// Feed handles GET /v1/discover/feed?language=ja&limit=20.
//
// Returns articles sorted by i+1 fit (sweet spot 90-95% comprehension first),
// then by recency. Articles outside [70%, 100%] are excluded — too easy or
// too hard wastes the discovery slot.
func (h *Handler) Feed(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	language := r.URL.Query().Get("language")
	if language == "" {
		language = "ja"
	}
	limit := 20
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 50 {
		limit = l
	}

	known, learning := h.fetchUserVocab(r.Context(), claims.UserID, language)
	knownSet := make(map[string]struct{}, len(known)+len(learning))
	learningSet := make(map[string]struct{}, len(learning))
	for _, l := range known {
		knownSet[l] = struct{}{}
	}
	for _, l := range learning {
		knownSet[l] = struct{}{}
		learningSet[l] = struct{}{}
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT id, source, title, summary, url, published_at,
		        content_lemmas, total_content_words
		   FROM discover_articles
		  WHERE language_code = $1
		    AND total_content_words >= 30
		  ORDER BY published_at DESC NULLS LAST
		  LIMIT 100`,
		language,
	)
	if err != nil {
		slog.Error("discover feed query", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()

	items := make([]FeedItem, 0, 100)
	for rows.Next() {
		var (
			id, source, title, url string
			summary                *string
			publishedAt            *time.Time
			lemmas                 []string
			totalContent           int
		)
		if err := rows.Scan(&id, &source, &title, &summary, &url, &publishedAt, &lemmas, &totalContent); err != nil {
			slog.Warn("discover feed scan", "error", err)
			continue
		}
		if totalContent == 0 {
			continue
		}

		// Compute per-user comprehension. content_lemmas is a deduped set, so
		// we can't recover the with-duplicates content-word count. We use the
		// distinct fraction as a proxy, which approximates pct well enough for
		// ranking; the absolute number is published alongside for transparency.
		knownInArticle := 0
		learningInArticle := 0
		for _, l := range lemmas {
			if _, ok := knownSet[l]; ok {
				knownInArticle++
			}
			if _, ok := learningSet[l]; ok {
				learningInArticle++
			}
		}
		distinct := len(lemmas)
		if distinct == 0 {
			continue
		}
		// Effective-known counts learning words as 50% — matches the scorer.
		effective := float64(knownInArticle) - float64(learningInArticle)*0.5
		comp := (effective / float64(distinct)) * 100.0
		if comp < 0 {
			comp = 0
		}

		mode := classifyMode(comp)
		// Only surface articles in the discovery sweet spot.
		if comp < 70.0 {
			continue
		}

		summaryStr := ""
		if summary != nil {
			summaryStr = *summary
		}
		pub := time.Time{}
		if publishedAt != nil {
			pub = *publishedAt
		}

		items = append(items, FeedItem{
			ID:               id,
			Source:           source,
			Title:            title,
			Summary:          summaryStr,
			URL:              url,
			PublishedAt:      pub,
			ComprehensionPct: round1(comp),
			UnknownCount:     distinct - knownInArticle,
			RecommendedMode:  mode,
			FitScore:         round4(fitScore(comp)),
		})
	}
	if err := rows.Err(); err != nil {
		slog.Warn("discover feed rows.Err", "error", err)
	}

	// Sort by fit score desc, then published_at desc.
	sort.SliceStable(items, func(a, b int) bool {
		if items[a].FitScore != items[b].FitScore {
			return items[a].FitScore > items[b].FitScore
		}
		return items[a].PublishedAt.After(items[b].PublishedAt)
	})
	if len(items) > limit {
		items = items[:limit]
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"language": language,
		"items":    items,
	})
}

// fitScore mirrors the i+1 picker's curve: triangular peak at 93%, zero at
// 70% and 100%. Used both to filter (>=70%) and to rank.
func fitScore(comprehensionPct float64) float64 {
	if comprehensionPct < 70.0 || comprehensionPct > 100.0 {
		return 0.0
	}
	if comprehensionPct <= 93.0 {
		return (comprehensionPct - 70.0) / 23.0
	}
	return math.Max(0.0, 1.0-(comprehensionPct-93.0)/7.0)
}

func classifyMode(comp float64) string {
	switch {
	case comp >= 98:
		return "flow_read"
	case comp >= 90:
		return "mining_read"
	case comp >= 80:
		return "study_read"
	default:
		return "too_hard"
	}
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
func round4(v float64) float64 { return math.Round(v*10000) / 10000 }

func (h *Handler) fetchUserVocab(ctx context.Context, userID, language string) (known, learning []string) {
	rows, err := h.db.Query(ctx,
		`SELECT front_text, fsrs_state
		   FROM cards
		  WHERE user_id = $1 AND language_code = $2 AND deleted_at IS NULL`,
		userID, language,
	)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()
	for rows.Next() {
		var lemma, state string
		if rows.Scan(&lemma, &state) == nil {
			switch state {
			case "review":
				known = append(known, lemma)
			case "learning", "relearning":
				learning = append(learning, lemma)
			}
		}
	}
	return known, learning
}
