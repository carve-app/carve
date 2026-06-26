package cards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/carve-app/carve/services/api/internal/audio"
	"github.com/carve-app/carve/services/api/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	db    *pgxpool.Pool
	media MediaUploader
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return NewHandlerWithMedia(db, newHTTPMediaUploader())
}

// MediaUploader isolates provider transport from card persistence. Fault tests
// can inject interrupted uploads and malformed responses without a live media
// service.
type MediaUploader interface {
	Upload(context.Context, string, io.Reader, string) (string, error)
}

type httpMediaUploader struct {
	baseURL   string
	publicURL string
	token     string
	client    interface {
		Do(*http.Request) (*http.Response, error)
	}
}

func newHTTPMediaUploader() MediaUploader {
	baseURL := strings.TrimRight(os.Getenv("MEDIA_SERVICE_URL"), "/")
	if baseURL == "" {
		baseURL = "http://localhost:8002"
	}
	publicURL := strings.TrimRight(os.Getenv("MEDIA_PUBLIC_BASE"), "/")
	if publicURL == "" {
		publicURL = baseURL
	}
	return &httpMediaUploader{
		baseURL:   baseURL,
		publicURL: publicURL,
		token:     os.Getenv("MEDIA_INTERNAL_TOKEN"),
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

func NewHandlerWithMedia(db *pgxpool.Pool, media MediaUploader) *Handler {
	return &Handler{db: db, media: media}
}

func (h *Handler) mediaUploader() MediaUploader {
	if h.media == nil {
		return newHTTPMediaUploader()
	}
	return h.media
}

func (u *httpMediaUploader) Upload(ctx context.Context, path string, body io.Reader, contentType string) (string, error) {
	endpoint := u.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", contentType)
	if u.token != "" {
		req.Header.Set("Authorization", "Bearer "+u.token)
	}
	resp, err := u.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("media service %s returned %d", endpoint, resp.StatusCode)
	}
	var result struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&result); err != nil {
		return "", err
	}
	if result.URL == "" {
		return "", errors.New("media service returned empty URL")
	}
	if strings.HasPrefix(result.URL, "http://") || strings.HasPrefix(result.URL, "https://") {
		return result.URL, nil
	}
	publicURL := u.publicURL
	if publicURL == "" {
		publicURL = u.baseURL
	}
	return publicURL + "/" + strings.TrimLeft(result.URL, "/"), nil
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

var supportedLanguages = map[string]bool{
	"ja": true, "zh-cn": true, "zh-tw": true, "ko": true, "en": true,
	"es": true, "de": true, "fr": true, "it": true, "pt": true, "vi": true,
}

func validLanguage(language string) bool { return supportedLanguages[language] }

func validUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

func unsafeText(value string) bool { return strings.ContainsRune(value, '\x00') }

func unsafeOptionalText(value *string) bool { return value != nil && unsafeText(*value) }

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
	if !validLanguage(req.LanguageCode) || len(req.Lemma) > 500 || unsafeText(req.Lemma) || unsafeText(req.Reading) ||
		unsafeOptionalText(req.BackText) || unsafeOptionalText(req.SubtitleTranslation) || unsafeOptionalText(req.Sentence) || unsafeOptionalText(req.SourceURL) {
		writeError(w, http.StatusBadRequest, "invalid card fields")
		return
	}

	id := auth.NewID()
	ctx := r.Context()

	// Mining is idempotent on (user, language, lemma): if a non-deleted card
	// for this word already exists, return it (200) instead of rejecting (409).
	// This lets the extension attach a screenshot/audio to the existing card
	// when a user re-mines the same word — previously the media was orphaned
	// because the client treated the 409 as a terminal "already mined".
	var existingID string
	var existingCreatedAt time.Time
	switch err := h.db.QueryRow(ctx,
		`SELECT id, created_at FROM cards
		   WHERE user_id = $1 AND language_code = $2 AND front_text = $3
		     AND deleted_at IS NULL
		   LIMIT 1`,
		claims.UserID, req.LanguageCode, req.Lemma,
	).Scan(&existingID, &existingCreatedAt); {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]any{
			"id":            existingID,
			"lemma":         req.Lemma,
			"language_code": req.LanguageCode,
			"created_at":    existingCreatedAt,
			"existing":      true,
		})
		return
	case errors.Is(err, pgx.ErrNoRows):
		// fall through to INSERT
	default:
		slog.Error("check duplicate card", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
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
		// Lost a race with a concurrent insert of the same word — resolve it to
		// the existing card so the caller can still attach media.
		if isUniqueViolation(err) {
			var raceID string
			var raceCreatedAt time.Time
			if qErr := h.db.QueryRow(ctx,
				`SELECT id, created_at FROM cards
				   WHERE user_id = $1 AND language_code = $2 AND front_text = $3
				     AND deleted_at IS NULL
				   LIMIT 1`,
				claims.UserID, req.LanguageCode, req.Lemma,
			).Scan(&raceID, &raceCreatedAt); qErr == nil {
				writeJSON(w, http.StatusOK, map[string]any{
					"id":            raceID,
					"lemma":         req.Lemma,
					"language_code": req.LanguageCode,
					"created_at":    raceCreatedAt,
					"existing":      true,
				})
				return
			}
			writeError(w, http.StatusConflict, "card for this lemma and language already exists")
			return
		}
		slog.Error("create card", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Populate word + sentence audio asynchronously — non-blocking, best-effort.
	// Runs for every card: word audio resolves per-language (JapanesePod101 for
	// JA, TTS fallback elsewhere when enabled), and sentence audio is synthesized
	// via TTS. Reading may be empty for non-Japanese languages.
	var sentence string
	if req.Sentence != nil {
		sentence = *req.Sentence
	}
	go audio.PopulateCard(h.db, id, req.LanguageCode, req.Lemma, req.Reading, sentence)

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":            id,
		"lemma":         req.Lemma,
		"language_code": req.LanguageCode,
		"created_at":    createdAt,
	})
}

// GET /v1/cards?language=ja&limit=50&offset=0&search=...&state=review&suspended=false&is_leech=false&sort=created
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
	if !validLanguage(language) {
		writeError(w, http.StatusBadRequest, "unsupported language")
		return
	}

	limit := 50
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 200")
			return
		}
		limit = n
	}

	offset := 0
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	search := q.Get("search")
	if search == "" {
		search = q.Get("q")
	}
	if unsafeText(search) {
		writeError(w, http.StatusBadRequest, "invalid search")
		return
	}
	stateFilter := q.Get("state")
	if stateFilter != "" && stateFilter != "new" && stateFilter != "learning" && stateFilter != "review" && stateFilter != "relearning" && stateFilter != "suspended" {
		writeError(w, http.StatusBadRequest, "invalid state")
		return
	}
	suspendedFilter := q.Get("suspended") // "true" | "false" | ""
	isLeechFilter := q.Get("is_leech")    // "true" | "false" | ""
	sortBy := q.Get("sort")               // created | due | lapses | alpha

	// Build WHERE conditions incrementally.
	args := []any{claims.UserID, language}
	where := "user_id = $1 AND language_code = $2 AND deleted_at IS NULL"

	if search != "" {
		args = append(args, search)
		where += fmt.Sprintf(` AND to_tsvector('simple',
			coalesce(front_text,'') || ' ' || coalesce(front_reading,'') || ' ' ||
			coalesce(back_text,'') || ' ' || coalesce(sentence,'')
		) @@ plainto_tsquery('simple', $%d)`, len(args))
	}
	if stateFilter != "" {
		args = append(args, stateFilter)
		where += fmt.Sprintf(` AND fsrs_state = $%d`, len(args))
	}
	switch suspendedFilter {
	case "true":
		where += " AND suspended = TRUE"
	case "false":
		where += " AND suspended = FALSE"
	}
	switch isLeechFilter {
	case "true":
		where += " AND is_leech = TRUE"
	case "false":
		where += " AND is_leech = FALSE"
	}

	orderBy := "created_at DESC"
	switch sortBy {
	case "due":
		orderBy = "COALESCE(fsrs_due, '2099-01-01') ASC"
	case "lapses":
		orderBy = "fsrs_lapses DESC, created_at DESC"
	case "alpha":
		orderBy = "front_text ASC"
	case "last_reviewed":
		orderBy = "COALESCE(fsrs_last_review, created_at) DESC"
	}

	ctx := r.Context()

	countArgs := make([]any, len(args))
	copy(countArgs, args)
	var total int
	if err := h.db.QueryRow(ctx,
		fmt.Sprintf(`SELECT COUNT(*) FROM cards WHERE %s`, where),
		countArgs...,
	).Scan(&total); err != nil {
		slog.Error("count cards", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	args = append(args, limit, offset)
	rows, err := h.db.Query(ctx,
		fmt.Sprintf(`SELECT id, front_text, COALESCE(back_text,''), sentence, source_url,
		        fsrs_state, fsrs_due, created_at, suspended, is_leech, tags
		 FROM cards
		 WHERE %s
		 ORDER BY %s
		 LIMIT $%d OFFSET $%d`, where, orderBy, len(args)-1, len(args)),
		args...,
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
		Suspended bool       `json:"suspended"`
		IsLeech   bool       `json:"is_leech"`
		Tags      []string   `json:"tags"`
	}

	cardList := []cardRow{}
	for rows.Next() {
		var c cardRow
		if err := rows.Scan(
			&c.ID, &c.FrontText, &c.BackText, &c.Sentence,
			&c.SourceURL, &c.FsrsState, &c.FsrsDue, &c.CreatedAt,
			&c.Suspended, &c.IsLeech, &c.Tags,
		); err != nil {
			slog.Error("scan card row", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if c.Tags == nil {
			c.Tags = []string{}
		}
		c.Lemma = c.FrontText
		cardList = append(cardList, c)
	}
	if err := rows.Err(); err != nil {
		slog.Error("rows error listing cards", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"cards":  cardList,
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
	if !validUUID(cardID) {
		writeError(w, http.StatusBadRequest, "card id must be a UUID")
		return
	}

	var c struct {
		ID              string     `json:"id"`
		LanguageCode    string     `json:"language_code"`
		FrontText       string     `json:"front_text"`
		BackText        *string    `json:"back_text"`
		Sentence        *string    `json:"sentence"`
		Translation     *string    `json:"subtitle_translation"`
		SourceURL       *string    `json:"source_url"`
		VideoSourceURL  *string    `json:"video_source_url"`
		FrontAudioURL   *string    `json:"audio_url"`
		FrontImageURL   *string    `json:"image_url"`
		SubtitleStartMs *int       `json:"subtitle_start_ms"`
		SubtitleEndMs   *int       `json:"subtitle_end_ms"`
		FsrsState       string     `json:"fsrs_state"`
		FsrsDue         *time.Time `json:"fsrs_due"`
		FsrsStability   *float64   `json:"stability"`
		FsrsDifficulty  *float64   `json:"difficulty"`
		FsrsReps        int        `json:"reps"`
		FsrsLapses      int        `json:"lapses"`
		Suspended       bool       `json:"suspended"`
		IsLeech         bool       `json:"is_leech"`
		Notes           *string    `json:"notes"`
		Tags            []string   `json:"tags"`
		CreatedAt       time.Time  `json:"created_at"`
	}

	err := h.db.QueryRow(r.Context(),
		`SELECT id, language_code, front_text, back_text, sentence, subtitle_translation,
		        source_url, video_source_url, front_audio_url, front_image_url,
		        subtitle_start_ms, subtitle_end_ms,
		        fsrs_state, fsrs_due, fsrs_stability, fsrs_difficulty, fsrs_reps, fsrs_lapses,
		        suspended, is_leech, notes, COALESCE(tags, '{}'),
		        created_at
		 FROM cards
		 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		cardID, claims.UserID,
	).Scan(
		&c.ID, &c.LanguageCode, &c.FrontText, &c.BackText, &c.Sentence, &c.Translation,
		&c.SourceURL, &c.VideoSourceURL, &c.FrontAudioURL, &c.FrontImageURL,
		&c.SubtitleStartMs, &c.SubtitleEndMs,
		&c.FsrsState, &c.FsrsDue, &c.FsrsStability, &c.FsrsDifficulty, &c.FsrsReps, &c.FsrsLapses,
		&c.Suspended, &c.IsLeech, &c.Notes, &c.Tags,
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
	if !validUUID(cardID) {
		writeError(w, http.StatusBadRequest, "card id must be a UUID")
		return
	}

	// Verify ownership
	var exists bool
	if err := h.db.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM cards WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL)`,
		cardID, claims.UserID,
	).Scan(&exists); err != nil || !exists {
		writeError(w, http.StatusNotFound, "card not found")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 26<<20)
	if err := r.ParseMultipartForm(26 << 20); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) || strings.Contains(err.Error(), "request body too large") {
			writeError(w, http.StatusRequestEntityTooLarge, "media upload too large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid multipart form")
		}
		return
	}

	var imageURL, audioURL *string
	// Track parts that were SENT but failed to store, so we can report an honest
	// error instead of a 200 with null URLs (which the client would read as
	// success while the card silently has no media).
	var uploadFailures []string
	uploadsAttempted := 0

	if f, hdr, err := r.FormFile("image"); err == nil {
		defer f.Close()
		uploadsAttempted++
		ct := contentTypeOf(hdr, "image/jpeg")
		if url, err := h.mediaUploader().Upload(r.Context(), "/screenshots", f, ct); err == nil {
			imageURL = &url
		} else {
			slog.Warn("card media: upload image", "error", err)
			uploadFailures = append(uploadFailures, "image")
		}
	}

	if f, hdr, err := r.FormFile("audio"); err == nil {
		defer f.Close()
		uploadsAttempted++
		ct := contentTypeOf(hdr, "audio/webm")
		if url, err := h.mediaUploader().Upload(r.Context(), "/audio", f, ct); err == nil {
			audioURL = &url
		} else {
			slog.Warn("card media: upload audio", "error", err)
			uploadFailures = append(uploadFailures, "audio")
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

	// Decide the response from what actually persisted:
	//   - every supplied part failed  → 502 (nothing landed; honest failure).
	//   - some succeeded, some failed → 200 with partial_failure listed, so the
	//     client keeps the URLs that DID persist and can still report honestly.
	//   - all succeeded (or none sent) → 200.
	if len(uploadFailures) > 0 {
		if uploadsAttempted > 0 && len(uploadFailures) == uploadsAttempted {
			writeJSON(w, http.StatusBadGateway, map[string]any{
				"error":     "media upload failed: " + strings.Join(uploadFailures, ", "),
				"image_url": imageURL,
				"audio_url": audioURL,
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"image_url":       imageURL,
			"audio_url":       audioURL,
			"partial_failure": uploadFailures,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"image_url": imageURL,
		"audio_url": audioURL,
	})
}

// contentTypeOf returns the multipart part's declared Content-Type, falling
// back to def when absent. This preserves the real recorded codec (e.g.
// audio/webm;codecs=opus) end-to-end instead of flattening it.
func contentTypeOf(hdr *multipart.FileHeader, def string) string {
	if hdr != nil {
		if ct := hdr.Header.Get("Content-Type"); ct != "" {
			return ct
		}
	}
	return def
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

// PATCH /v1/cards/{id}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	cardID := chi.URLParam(r, "id")
	if !validUUID(cardID) {
		writeError(w, http.StatusBadRequest, "card id must be a UUID")
		return
	}

	var req struct {
		BackText     *string  `json:"back_text"`
		Sentence     *string  `json:"sentence"`
		Translation  *string  `json:"subtitle_translation"`
		FrontText    *string  `json:"front_text"`
		FrontReading *string  `json:"front_reading"`
		Notes        *string  `json:"notes"`
		Tags         []string `json:"tags"`
		DeckID       *string  `json:"deck_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if unsafeOptionalText(req.BackText) || unsafeOptionalText(req.Sentence) || unsafeOptionalText(req.Translation) ||
		unsafeOptionalText(req.FrontText) || unsafeOptionalText(req.FrontReading) || unsafeOptionalText(req.Notes) {
		writeError(w, http.StatusBadRequest, "invalid card fields")
		return
	}
	for _, tag := range req.Tags {
		if unsafeText(tag) {
			writeError(w, http.StatusBadRequest, "invalid tag")
			return
		}
	}
	if req.DeckID != nil {
		if _, err := uuid.Parse(*req.DeckID); err != nil {
			writeError(w, http.StatusBadRequest, "deck_id must be a UUID")
			return
		}
	}

	tag, err := h.db.Exec(r.Context(),
		`UPDATE cards SET
			back_text             = COALESCE($1, back_text),
			sentence              = COALESCE($2, sentence),
			subtitle_translation  = COALESCE($3, subtitle_translation),
			front_text            = COALESCE($4, front_text),
			front_reading         = COALESCE($5, front_reading),
			notes                 = COALESCE($6, notes),
			tags                  = COALESCE($7::text[], tags),
			deck_id               = COALESCE($8::uuid, deck_id),
			updated_at            = now()
		 WHERE id = $9 AND user_id = $10 AND deleted_at IS NULL`,
		req.BackText, req.Sentence, req.Translation,
		req.FrontText, req.FrontReading, req.Notes,
		tagsArg(req.Tags), req.DeckID,
		cardID, claims.UserID,
	)
	if err != nil {
		slog.Error("update card", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "card not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// tagsArg converts a Go string slice to a pointer for COALESCE logic:
// nil slice → nil (no update), empty or populated → pointer to the value.
func tagsArg(tags []string) interface{} {
	if tags == nil {
		return nil
	}
	return tags
}

// POST /v1/cards/{id}/suspend
func (h *Handler) Suspend(w http.ResponseWriter, r *http.Request) {
	h.setLifecycleFlag(w, r, "suspended", true)
}

// POST /v1/cards/{id}/unsuspend
func (h *Handler) Unsuspend(w http.ResponseWriter, r *http.Request) {
	h.setLifecycleFlag(w, r, "suspended", false)
}

// POST /v1/cards/{id}/bury
func (h *Handler) Bury(w http.ResponseWriter, r *http.Request) {
	h.setLifecycleFlag(w, r, "buried", true)
}

func (h *Handler) Unbury(w http.ResponseWriter, r *http.Request) {
	h.setLifecycleFlag(w, r, "buried", false)
}

func (h *Handler) setLifecycleFlag(w http.ResponseWriter, r *http.Request, col string, val bool) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	cardID := chi.URLParam(r, "id")
	if !validUUID(cardID) {
		writeError(w, http.StatusBadRequest, "card id must be a UUID")
		return
	}

	var query string
	switch col {
	case "suspended":
		if val {
			query = `UPDATE cards SET suspended = TRUE, updated_at = now() WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`
		} else {
			query = `UPDATE cards SET suspended = FALSE, updated_at = now() WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`
		}
	case "buried":
		if val {
			query = `UPDATE cards SET buried = TRUE, buried_until = CURRENT_DATE + 1, updated_at = now() WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`
		} else {
			query = `UPDATE cards SET buried = FALSE, buried_until = NULL, updated_at = now() WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`
		}
	default:
		writeError(w, http.StatusBadRequest, "unknown lifecycle flag")
		return
	}

	tag, err := h.db.Exec(r.Context(), query, cardID, claims.UserID)
	if err != nil {
		slog.Error("lifecycle update", "col", col, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "card not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{col: val})
}

// POST /v1/cards/bulk
// Body: {"action": "suspend"|"unsuspend"|"bury"|"unbury"|"delete", "ids": [...]}
func (h *Handler) Bulk(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Action string   `json:"action"`
		IDs    []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if len(req.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "ids required")
		return
	}
	if len(req.IDs) > 500 {
		writeError(w, http.StatusBadRequest, "too many ids (max 500)")
		return
	}
	var query string
	switch req.Action {
	case "suspend":
		query = `UPDATE cards SET suspended = TRUE, updated_at = now() WHERE user_id = $1 AND id = ANY($2::uuid[]) AND deleted_at IS NULL`
	case "unsuspend":
		query = `UPDATE cards SET suspended = FALSE, updated_at = now() WHERE user_id = $1 AND id = ANY($2::uuid[]) AND deleted_at IS NULL`
	case "bury":
		query = `UPDATE cards SET buried = TRUE, buried_until = CURRENT_DATE + 1, updated_at = now() WHERE user_id = $1 AND id = ANY($2::uuid[]) AND deleted_at IS NULL`
	case "unbury":
		query = `UPDATE cards SET buried = FALSE, buried_until = NULL, updated_at = now() WHERE user_id = $1 AND id = ANY($2::uuid[]) AND deleted_at IS NULL`
	case "delete":
		query = `UPDATE cards SET deleted_at = now() WHERE user_id = $1 AND id = ANY($2::uuid[]) AND deleted_at IS NULL`
	default:
		writeError(w, http.StatusBadRequest, "unknown action")
		return
	}
	for _, id := range req.IDs {
		if !validUUID(id) {
			writeError(w, http.StatusBadRequest, "every id must be a UUID")
			return
		}
	}

	tag, err := h.db.Exec(r.Context(), query, claims.UserID, req.IDs)
	if err != nil {
		slog.Error("bulk card action", "action", req.Action, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"affected": tag.RowsAffected()})
}

// DELETE /v1/cards/:id
// POST /v1/cards/find-similar
//
// Given a candidate sentence, return the user's existing cards whose sentence
// is near-duplicate. Used by the extension's mine form to warn before saving
// yet another card with the same context, which is the dominant source of
// review-queue waste (subtitle loops, re-runs, mining many words from one cue).
func (h *Handler) FindSimilar(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		LanguageCode string  `json:"language_code"`
		Sentence     string  `json:"sentence"`
		Threshold    float64 `json:"threshold"`
		Limit        int     `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.LanguageCode == "" || req.Sentence == "" {
		writeError(w, http.StatusBadRequest, "language_code and sentence are required")
		return
	}
	if req.Threshold <= 0 {
		req.Threshold = 0.5
	}
	if req.Limit <= 0 || req.Limit > 10 {
		req.Limit = 3
	}

	candidateGrams := charTrigrams(req.Sentence)
	if len(candidateGrams) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"matches": []any{}})
		return
	}

	// Scan a recent window of cards — enough to catch dupes, small enough to
	// keep the request cheap. 200 covers a few months of heavy mining.
	rows, err := h.db.Query(r.Context(),
		`SELECT id, front_text, sentence
		   FROM cards
		  WHERE user_id = $1
		    AND language_code = $2
		    AND deleted_at IS NULL
		    AND sentence IS NOT NULL
		    AND length(sentence) > 0
		  ORDER BY created_at DESC
		  LIMIT 200`,
		claims.UserID, req.LanguageCode,
	)
	if err != nil {
		slog.Error("find-similar query", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()

	type match struct {
		ID         string  `json:"id"`
		FrontText  string  `json:"front_text"`
		Sentence   string  `json:"sentence"`
		Similarity float64 `json:"similarity"`
	}
	matches := make([]match, 0, req.Limit+1)

	for rows.Next() {
		var id, front string
		var sentence *string
		if err := rows.Scan(&id, &front, &sentence); err != nil {
			slog.Warn("find-similar scan", "error", err)
			continue
		}
		if sentence == nil {
			continue
		}
		sim := jaccardTrigrams(candidateGrams, charTrigrams(*sentence))
		if sim < req.Threshold {
			continue
		}
		matches = append(matches, match{
			ID:         id,
			FrontText:  front,
			Sentence:   *sentence,
			Similarity: sim,
		})
	}
	if err := rows.Err(); err != nil {
		slog.Warn("find-similar rows.Err", "error", err)
	}

	// Sort by similarity descending and cap to limit.
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0 && matches[j].Similarity > matches[j-1].Similarity; j-- {
			matches[j], matches[j-1] = matches[j-1], matches[j]
		}
	}
	if len(matches) > req.Limit {
		matches = matches[:req.Limit]
	}

	writeJSON(w, http.StatusOK, map[string]any{"matches": matches})
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	cardID := chi.URLParam(r, "id")
	if !validUUID(cardID) {
		writeError(w, http.StatusBadRequest, "card id must be a UUID")
		return
	}

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
