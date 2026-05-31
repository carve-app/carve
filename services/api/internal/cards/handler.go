package cards

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/carve-app/carve/services/api/internal/audio"
	"github.com/carve-app/carve/services/api/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

// isUniqueViolation reports whether err is a PostgreSQL unique-constraint
// violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// POST /v1/cards
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		LanguageCode        string   `json:"language_code"`
		Lemma               string   `json:"lemma"`
		Reading             string   `json:"reading"`
		BackText            *string  `json:"back_text"`
		SubtitleTranslation *string  `json:"subtitle_translation"`
		Sentence            *string  `json:"sentence"`
		SourceURL           *string  `json:"source_url"`
		SourceTimestamp     *float64 `json:"source_timestamp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.LanguageCode == "" || req.Lemma == "" {
		writeError(w, http.StatusBadRequest, "language_code and lemma are required")
		return
	}

	id := auth.NewID()
	ctx := r.Context()

	// Check for an existing non-deleted card with the same lemma+language.
	var exists bool
	if err := h.db.QueryRow(ctx,
		`SELECT EXISTS(
		     SELECT 1 FROM cards
		     WHERE user_id = $1 AND language_code = $2 AND front_text = $3
		       AND deleted_at IS NULL
		 )`,
		claims.UserID, req.LanguageCode, req.Lemma,
	).Scan(&exists); err != nil {
		slog.Error("check duplicate card", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if exists {
		writeError(w, http.StatusConflict, "card for this lemma and language already exists")
		return
	}

	var readingVal *string
	if req.Reading != "" {
		readingVal = &req.Reading
	}

	var createdAt time.Time
	err := h.db.QueryRow(ctx,
		`INSERT INTO cards
		    (id, user_id, language_code, front_text, front_reading, back_text, subtitle_translation, sentence, source_url, source_timestamp, fsrs_state)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'new')
		 RETURNING created_at`,
		id, claims.UserID, req.LanguageCode, req.Lemma, readingVal,
		req.BackText, req.SubtitleTranslation, req.Sentence, req.SourceURL, req.SourceTimestamp,
	).Scan(&createdAt)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "card for this lemma and language already exists")
			return
		}
		slog.Error("create card", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Populate audio asynchronously — non-blocking, best-effort.
	if req.Reading != "" {
		go audio.PopulateCard(h.db, id, req.LanguageCode, req.Lemma, req.Reading)
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":            id,
		"lemma":         req.Lemma,
		"language_code": req.LanguageCode,
		"created_at":    createdAt,
	})
}

// GET /v1/cards?language=ja&limit=50&offset=0
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	q := r.URL.Query()

	language := q.Get("language")
	if language == "" {
		language = "ja"
	}

	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > 200 {
				n = 200
			}
			limit = n
		}
	}

	offset := 0
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	ctx := r.Context()

	var total int
	if err := h.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM cards
		 WHERE user_id = $1 AND language_code = $2 AND deleted_at IS NULL`,
		claims.UserID, language,
	).Scan(&total); err != nil {
		slog.Error("count cards", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rows, err := h.db.Query(ctx,
		`SELECT id, front_text, COALESCE(back_text,''), sentence, source_url,
		        fsrs_state, fsrs_due, created_at
		 FROM cards
		 WHERE user_id = $1 AND language_code = $2 AND deleted_at IS NULL
		 ORDER BY created_at DESC
		 LIMIT $3 OFFSET $4`,
		claims.UserID, language, limit, offset,
	)
	if err != nil {
		slog.Error("list cards query", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()

	type cardRow struct {
		ID        string     `json:"id"`
		Lemma     string     `json:"lemma"`
		FrontText string     `json:"front_text"`
		BackText  string     `json:"back_text"`
		Sentence  *string    `json:"sentence"`
		SourceURL *string    `json:"source_url"`
		FsrsState string     `json:"fsrs_state"`
		FsrsDue   *time.Time `json:"fsrs_due"`
		CreatedAt time.Time  `json:"created_at"`
	}

	cards := []cardRow{}
	for rows.Next() {
		var c cardRow
		if err := rows.Scan(
			&c.ID, &c.FrontText, &c.BackText, &c.Sentence,
			&c.SourceURL, &c.FsrsState, &c.FsrsDue, &c.CreatedAt,
		); err != nil {
			slog.Error("scan card row", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		c.Lemma = c.FrontText
		cards = append(cards, c)
	}
	if err := rows.Err(); err != nil {
		slog.Error("rows error listing cards", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"cards":  cards,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GET /v1/cards/{id}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	cardID := chi.URLParam(r, "id")

	var c struct {
		ID                  string     `json:"id"`
		LanguageCode        string     `json:"language_code"`
		FrontText           string     `json:"front_text"`
		BackText            *string    `json:"back_text"`
		Sentence            *string    `json:"sentence"`
		Translation         *string    `json:"subtitle_translation"`
		SourceURL           *string    `json:"source_url"`
		VideoSourceURL      *string    `json:"video_source_url"`
		FrontAudioURL       *string    `json:"audio_url"`
		FrontImageURL       *string    `json:"image_url"`
		SubtitleStartMs     *int       `json:"subtitle_start_ms"`
		SubtitleEndMs       *int       `json:"subtitle_end_ms"`
		FsrsState           string     `json:"fsrs_state"`
		FsrsDue             *time.Time `json:"fsrs_due"`
		FsrsStability       *float64   `json:"stability"`
		FsrsDifficulty      *float64   `json:"difficulty"`
		FsrsReps            int        `json:"reps"`
		FsrsLapses          int        `json:"lapses"`
		CreatedAt           time.Time  `json:"created_at"`
	}

	err := h.db.QueryRow(r.Context(),
		`SELECT id, language_code, front_text, back_text, sentence, subtitle_translation,
		        source_url, video_source_url, front_audio_url, front_image_url,
		        subtitle_start_ms, subtitle_end_ms,
		        fsrs_state, fsrs_due, fsrs_stability, fsrs_difficulty, fsrs_reps, fsrs_lapses,
		        created_at
		 FROM cards
		 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		cardID, claims.UserID,
	).Scan(
		&c.ID, &c.LanguageCode, &c.FrontText, &c.BackText, &c.Sentence, &c.Translation,
		&c.SourceURL, &c.VideoSourceURL, &c.FrontAudioURL, &c.FrontImageURL,
		&c.SubtitleStartMs, &c.SubtitleEndMs,
		&c.FsrsState, &c.FsrsDue, &c.FsrsStability, &c.FsrsDifficulty, &c.FsrsReps, &c.FsrsLapses,
		&c.CreatedAt,
	)
	if err != nil {
		writeError(w, http.StatusNotFound, "card not found")
		return
	}

	writeJSON(w, http.StatusOK, c)
}

// POST /v1/cards/{id}/media
// Accepts multipart/form-data with optional 'image' (JPEG) and 'audio' (webm) parts,
// uploads them to the media service, and stores the resulting URLs on the card.
func (h *Handler) AttachMedia(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	cardID := chi.URLParam(r, "id")

	// Verify ownership
	var exists bool
	if err := h.db.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM cards WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL)`,
		cardID, claims.UserID,
	).Scan(&exists); err != nil || !exists {
		writeError(w, http.StatusNotFound, "card not found")
		return
	}

	if err := r.ParseMultipartForm(20 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	mediaBase := os.Getenv("MEDIA_SERVICE_URL")
	if mediaBase == "" {
		mediaBase = "http://localhost:8002"
	}

	var imageURL, audioURL *string

	if f, _, err := r.FormFile("image"); err == nil {
		defer f.Close()
		if url, err := uploadToMediaService(mediaBase+"/screenshots", f, "image/jpeg"); err == nil {
			imageURL = &url
		} else {
			slog.Warn("card media: upload image", "error", err)
		}
	}

	if f, _, err := r.FormFile("audio"); err == nil {
		defer f.Close()
		if url, err := uploadToMediaService(mediaBase+"/audio", f, "audio/webm"); err == nil {
			audioURL = &url
		} else {
			slog.Warn("card media: upload audio", "error", err)
		}
	}

	startMs := formInt(r, "subtitle_start_ms")
	endMs := formInt(r, "subtitle_end_ms")
	videoSrcURL := r.FormValue("video_source_url")
	translation := r.FormValue("subtitle_translation")

	_, err := h.db.Exec(r.Context(),
		`UPDATE cards SET
			front_image_url       = COALESCE($1, front_image_url),
			front_audio_url       = COALESCE($2, front_audio_url),
			video_source_url      = COALESCE(NULLIF($3,''), video_source_url),
			subtitle_start_ms     = COALESCE($4, subtitle_start_ms),
			subtitle_end_ms       = COALESCE($5, subtitle_end_ms),
			subtitle_translation  = COALESCE(NULLIF($6,''), subtitle_translation)
		 WHERE id = $7 AND user_id = $8`,
		imageURL, audioURL, videoSrcURL, startMs, endMs, translation, cardID, claims.UserID,
	)
	if err != nil {
		slog.Error("card media: update card", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"image_url": imageURL,
		"audio_url": audioURL,
	})
}

func uploadToMediaService(url string, body io.Reader, contentType string) (string, error) {
	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", contentType)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("media service %s returned %d", url, resp.StatusCode)
	}

	var result struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	mediaBase := os.Getenv("MEDIA_SERVICE_URL")
	if mediaBase == "" {
		mediaBase = "http://localhost:8002"
	}
	return mediaBase + result.URL, nil
}

func formInt(r *http.Request, field string) *int {
	v := r.FormValue(field)
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return nil
	}
	return &n
}

// DELETE /v1/cards/:id
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	cardID := chi.URLParam(r, "id")

	tag, err := h.db.Exec(r.Context(),
		`UPDATE cards SET deleted_at = now()
		 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		cardID, claims.UserID,
	)
	if err != nil {
		slog.Error("delete card", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "card not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
