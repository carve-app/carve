package reports

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SendWeeklyDigestToAll iterates active users and emails each their weekly
// digest. Designed to be called from a Monday-morning cron in main.go.
// Best-effort: per-user failures are logged but do not stop the run.
func SendWeeklyDigestToAll(ctx context.Context, db *pgxpool.Pool) {
	cfg, ok := LoadSMTPConfig()
	if !ok {
		slog.Info("reports: SMTP not configured; skipping weekly digest")
		return
	}

	rows, err := db.Query(ctx,
		`SELECT u.id, u.email, COALESCE(u.display_name, ''), u.target_language
		   FROM users u
		  WHERE u.email IS NOT NULL
		    AND u.email <> ''
		    AND u.deleted_at IS NULL
		    AND COALESCE(u.weekly_email_opt_out, FALSE) = FALSE`,
	)
	if err != nil {
		slog.Error("reports: scan users", "error", err)
		return
	}
	defer rows.Close()

	h := NewHandler(db)
	now := time.Now().UTC()
	weekStart := now.AddDate(0, 0, -7)
	sent := 0
	for rows.Next() {
		var userID, email, name, language string
		if err := rows.Scan(&userID, &email, &name, &language); err != nil {
			continue
		}
		if language == "" {
			language = "ja"
		}
		report, err := h.ComputeWeekly(ctx, userID, language, weekStart, now)
		if err != nil {
			slog.Warn("reports: compute failed", "user_id", userID, "error", err)
			continue
		}
		// Skip silent weeks — sending "you did nothing!" emails just churns.
		if report.CardsMined == 0 && report.ReviewsCompleted == 0 && report.ImmersionMinutes == 0 {
			continue
		}
		if err := SendWeekly(cfg, email, name, report); err != nil {
			slog.Warn("reports: send failed", "user_id", userID, "error", err)
			continue
		}
		sent++
	}
	slog.Info("reports: weekly digest run complete", "sent", sent)
}
