// Package reports computes user-facing learning reports (weekly digest,
// soon also monthly) and sends them via SMTP when configured.
package reports

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"time"

	"github.com/carve-app/carve/services/api/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler exposes /v1/reports/* — the data is also used by the SMTP sender.
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

// WeeklyReport is the data shipped to the UI and rendered into email body.
type WeeklyReport struct {
	Language          string    `json:"language"`
	WeekStart         time.Time `json:"week_start"`
	WeekEnd           time.Time `json:"week_end"`
	CardsMined        int       `json:"cards_mined"`
	ReviewsCompleted  int       `json:"reviews_completed"`
	ImmersionMinutes  int       `json:"immersion_minutes"`
	NewKnownWords     int       `json:"new_known_words"`
	RetentionRate     float64   `json:"retention_rate"`
	StreakDays        int       `json:"streak_days"`
}

// Weekly handles GET /v1/reports/weekly?language=ja
func (h *Handler) Weekly(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	language := r.URL.Query().Get("language")
	if language == "" {
		language = "ja"
	}
	now := time.Now().UTC()
	weekStart := now.AddDate(0, 0, -7)

	report, err := h.ComputeWeekly(r.Context(), claims.UserID, language, weekStart, now)
	if err != nil {
		slog.Error("reports: compute weekly", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// ComputeWeekly is exported so the email cron can call it without going
// through HTTP.
func (h *Handler) ComputeWeekly(
	ctx context.Context, userID, language string, weekStart, weekEnd time.Time,
) (*WeeklyReport, error) {
	r := &WeeklyReport{
		Language:  language,
		WeekStart: weekStart,
		WeekEnd:   weekEnd,
	}

	if err := h.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM cards
		  WHERE user_id = $1 AND language_code = $2
		    AND created_at BETWEEN $3 AND $4
		    AND deleted_at IS NULL`,
		userID, language, weekStart, weekEnd,
	).Scan(&r.CardsMined); err != nil {
		return nil, fmt.Errorf("cards_mined: %w", err)
	}

	if err := h.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM review_events re
		   JOIN cards c ON c.id = re.card_id
		  WHERE re.user_id = $1 AND c.language_code = $2
		    AND re.reviewed_at BETWEEN $3 AND $4`,
		userID, language, weekStart, weekEnd,
	).Scan(&r.ReviewsCompleted); err != nil {
		return nil, fmt.Errorf("reviews_completed: %w", err)
	}

	var totalSec int
	if err := h.db.QueryRow(ctx,
		`SELECT COALESCE(SUM(duration_sec), 0) FROM immersion_sessions
		  WHERE user_id = $1 AND language_code = $2
		    AND started_at BETWEEN $3 AND $4`,
		userID, language, weekStart, weekEnd,
	).Scan(&totalSec); err != nil {
		return nil, fmt.Errorf("immersion: %w", err)
	}
	r.ImmersionMinutes = totalSec / 60

	// Retention proxy: % of reviews where rating >= 3 (Good or Easy).
	var totalReviews, goodReviews int
	if err := h.db.QueryRow(ctx,
		`SELECT COUNT(*),
		        COUNT(*) FILTER (WHERE rating >= 3)
		   FROM review_events re
		   JOIN cards c ON c.id = re.card_id
		  WHERE re.user_id = $1 AND c.language_code = $2
		    AND re.reviewed_at BETWEEN $3 AND $4`,
		userID, language, weekStart, weekEnd,
	).Scan(&totalReviews, &goodReviews); err != nil {
		return nil, fmt.Errorf("retention: %w", err)
	}
	if totalReviews > 0 {
		r.RetentionRate = float64(goodReviews) / float64(totalReviews)
	}

	// New "known" words: cards that transitioned into review state this week.
	// Approximated by first review event with stability_after >= 21 days
	// (standard FSRS "mature" threshold).
	if err := h.db.QueryRow(ctx,
		`SELECT COUNT(DISTINCT c.id)
		   FROM cards c
		   JOIN review_events re ON re.card_id = c.id
		  WHERE c.user_id = $1 AND c.language_code = $2
		    AND re.reviewed_at BETWEEN $3 AND $4
		    AND re.stability_after >= 21.0`,
		userID, language, weekStart, weekEnd,
	).Scan(&r.NewKnownWords); err != nil {
		return nil, fmt.Errorf("new_known: %w", err)
	}

	// Streak: consecutive UTC days with at least one review_event, looking back
	// from weekEnd.
	rows, err := h.db.Query(ctx,
		`SELECT DISTINCT DATE_TRUNC('day', reviewed_at)::date
		   FROM review_events
		  WHERE user_id = $1
		    AND reviewed_at >= $2
		  ORDER BY 1 DESC`,
		userID, weekEnd.AddDate(0, 0, -60),
	)
	if err == nil {
		defer rows.Close()
		var prev time.Time
		first := true
		for rows.Next() {
			var d time.Time
			if rows.Scan(&d) != nil {
				continue
			}
			if first {
				prev = d
				r.StreakDays = 1
				first = false
				continue
			}
			if prev.Sub(d) == 24*time.Hour {
				r.StreakDays++
				prev = d
			} else {
				break
			}
		}
	}

	return r, nil
}

// ── SMTP sender ──────────────────────────────────────────────────────────────

type SMTPConfig struct {
	Host string
	Port string
	User string
	Pass string
	From string
}

func LoadSMTPConfig() (*SMTPConfig, bool) {
	host := os.Getenv("SMTP_HOST")
	if host == "" {
		return nil, false
	}
	port := os.Getenv("SMTP_PORT")
	if port == "" {
		port = "587"
	}
	return &SMTPConfig{
		Host: host, Port: port,
		User: os.Getenv("SMTP_USER"),
		Pass: os.Getenv("SMTP_PASS"),
		From: firstNonEmpty(os.Getenv("SMTP_FROM"), os.Getenv("SMTP_USER")),
	}, true
}

// SendWeekly delivers a rendered HTML+text email. Returns nil if SMTP is not
// configured (caller treats this as a successful no-op).
func SendWeekly(cfg *SMTPConfig, to, displayName string, r *WeeklyReport) error {
	if cfg == nil {
		return nil
	}
	subject := fmt.Sprintf("Your Carve week: %d cards, %d reviews", r.CardsMined, r.ReviewsCompleted)
	body := RenderWeeklyEmail(displayName, r)

	headers := map[string]string{
		"From":         cfg.From,
		"To":           to,
		"Subject":      subject,
		"MIME-Version": "1.0",
		"Content-Type": "text/plain; charset=utf-8",
	}
	var msg strings.Builder
	for k, v := range headers {
		msg.WriteString(k + ": " + v + "\r\n")
	}
	msg.WriteString("\r\n")
	msg.WriteString(body)

	addr := cfg.Host + ":" + cfg.Port
	var auth_ smtp.Auth
	if cfg.User != "" {
		auth_ = smtp.PlainAuth("", cfg.User, cfg.Pass, cfg.Host)
	}
	return smtp.SendMail(addr, auth_, cfg.From, []string{to}, []byte(msg.String()))
}

// RenderWeeklyEmail produces the plain-text email body. Public so the
// /reports/weekly endpoint can optionally return a preview, and so tests can
// assert against the rendered output without going through SMTP.
func RenderWeeklyEmail(name string, r *WeeklyReport) string {
	if name == "" {
		name = "there"
	}
	hi := fmt.Sprintf("Hi %s,", name)
	retention := "—"
	if r.ReviewsCompleted > 0 {
		retention = fmt.Sprintf("%.0f%%", r.RetentionRate*100)
	}
	streakWord := "day"
	if r.StreakDays != 1 {
		streakWord = "days"
	}
	return strings.Join([]string{
		hi, "",
		"Here's your Carve week (" + r.WeekStart.Format("Jan 2") + " – " + r.WeekEnd.Format("Jan 2") + "):",
		"",
		fmt.Sprintf("  • Cards mined:        %d", r.CardsMined),
		fmt.Sprintf("  • Reviews completed:  %d", r.ReviewsCompleted),
		fmt.Sprintf("  • Immersion:          %d min", r.ImmersionMinutes),
		fmt.Sprintf("  • New known words:    %d", r.NewKnownWords),
		fmt.Sprintf("  • Retention:          %s", retention),
		fmt.Sprintf("  • Streak:             %d %s", r.StreakDays, streakWord),
		"",
		"Keep going!",
		"— Carve",
	}, "\n")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
