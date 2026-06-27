package onboarding

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/carve-app/carve/services/api/internal/auth"
)

type placementTestPayload struct {
	Language         string          `json:"language"`
	Version          string          `json:"version"`
	EstimatedMinutes int             `json:"estimated_minutes"`
	Items            []PlacementItem `json:"items"`
}

type submitPlacementRequest struct {
	Language string            `json:"language"`
	Version  string            `json:"version"`
	Answers  []PlacementAnswer `json:"answers"`
}

type placementResultPayload struct {
	AttemptID      string               `json:"attempt_id"`
	Language       string               `json:"language"`
	Version        string               `json:"version"`
	Correct        int                  `json:"correct"`
	Total          int                  `json:"total"`
	VerifiedKnown  int                  `json:"verified_known"`
	EstimatedKnown int                  `json:"estimated_known"`
	EstimateLower  int                  `json:"estimate_lower"`
	EstimateUpper  int                  `json:"estimate_upper"`
	ResultLabel    string               `json:"result_label"`
	BandScores     []PlacementBandScore `json:"band_scores"`
}

// PlacementTest returns the current English receptive-vocabulary item set
// without its answer key. Authentication keeps this onboarding endpoint
// consistent with the rest of the user's vocabulary state.
func (h *Handler) PlacementTest(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.ClaimsFromContext(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	language := r.URL.Query().Get("language")
	if language == "" {
		language = "en"
	}
	if language != "en" {
		writeError(w, http.StatusBadRequest, "placement test is currently available for English only")
		return
	}

	writeJSON(w, http.StatusOK, placementTestPayload{
		Language: "en", Version: englishPlacementVersion, EstimatedMinutes: 4,
		Items: publicEnglishPlacementItems(),
	})
}

// SubmitPlacementTest scores and persists an English placement attempt, then
// seeds only the individually verified items into user_word_knowledge. The
// extrapolated estimate is deliberately not inserted as thousands of assumed
// known words; later reading interactions refine the user's exact word model.
func (h *Handler) SubmitPlacementTest(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req submitPlacementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Language != "en" {
		writeError(w, http.StatusBadRequest, "placement test is currently available for English only")
		return
	}
	if req.Version != englishPlacementVersion {
		writeError(w, http.StatusBadRequest, "placement test version is missing or no longer current")
		return
	}

	score, err := scoreEnglishPlacement(req.Answers)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	answersJSON, err := json.Marshal(req.Answers)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid placement answers")
		return
	}

	ctx := r.Context()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		slog.Error("begin placement result", "error", err, "user_id", claims.UserID)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer tx.Rollback(ctx)

	var attemptID string
	if err := tx.QueryRow(ctx,
		`INSERT INTO placement_test_attempts
		    (user_id, language_code, test_version, answers, correct_count,
		     total_count, estimated_known_words, estimate_lower, estimate_upper, result_label)
		 VALUES ($1, 'en', $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id`,
		claims.UserID, englishPlacementVersion, answersJSON, score.Correct, score.Total,
		score.EstimatedKnown, score.EstimateLower, score.EstimateUpper, score.ResultLabel,
	).Scan(&attemptID); err != nil {
		slog.Error("persist placement result", "error", err, "user_id", claims.UserID)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if _, err := tx.Exec(ctx,
		`UPDATE users SET target_language = 'en' WHERE id = $1`, claims.UserID); err != nil {
		slog.Error("set placement target language", "error", err, "user_id", claims.UserID)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO user_languages (user_id, language_code, is_active)
		 VALUES ($1, 'en', TRUE)
		 ON CONFLICT (user_id, language_code) DO UPDATE SET is_active = TRUE`,
		claims.UserID,
	); err != nil {
		slog.Error("activate placement target language", "error", err, "user_id", claims.UserID)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	marked := 0
	for _, item := range score.CorrectItems {
		var wordID string
		if err := tx.QueryRow(ctx,
			`INSERT INTO words (language_code, lemma, reading, frequency_rank)
			 VALUES ('en', $1, $1, $2)
			 ON CONFLICT (language_code, lemma, reading) DO UPDATE
			   SET frequency_rank = COALESCE(words.frequency_rank, EXCLUDED.frequency_rank)
			 RETURNING id`,
			item.Word, item.FrequencyRank,
		).Scan(&wordID); err != nil {
			slog.Error("upsert placement word", "error", err, "user_id", claims.UserID, "word", item.Word)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO user_word_knowledge (user_id, word_id, status, known_since)
			 VALUES ($1, $2, 'known', now())
			 ON CONFLICT (user_id, word_id) DO UPDATE
			   SET status = 'known', known_since = COALESCE(user_word_knowledge.known_since, now()), updated_at = now()`,
			claims.UserID, wordID,
		); err != nil {
			slog.Error("seed placement knowledge", "error", err, "user_id", claims.UserID, "word", item.Word)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		marked++
	}

	if err := tx.Commit(ctx); err != nil {
		slog.Error("commit placement result", "error", err, "user_id", claims.UserID)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, placementResultPayload{
		AttemptID: attemptID, Language: "en", Version: englishPlacementVersion,
		Correct: score.Correct, Total: score.Total, VerifiedKnown: marked,
		EstimatedKnown: score.EstimatedKnown, EstimateLower: score.EstimateLower,
		EstimateUpper: score.EstimateUpper, ResultLabel: score.ResultLabel,
		BandScores: score.BandScores,
	})
}
