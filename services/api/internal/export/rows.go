package export

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// loadCardRows pulls the non-deleted cards for a user, optionally filtered by
// language, flattened into exportCardRow. Shared by the CSV and .apkg
// exporters so both formats see identical data.
//
// A blank `language` exports every language; a non-blank one filters to it.
func (h *Handler) loadCardRows(ctx context.Context, userID, language string) ([]exportCardRow, error) {
	const cols = `
		front_text, COALESCE(front_reading,''), COALESCE(back_text,''),
		COALESCE(sentence,''), COALESCE(subtitle_translation,''),
		COALESCE(source_url,''), COALESCE(front_audio_url,''),
		COALESCE(front_image_url,''), COALESCE(back_audio_url,''),
		COALESCE(sentence_audio_url,''),
		fsrs_state, fsrs_stability, fsrs_difficulty, fsrs_reps, fsrs_lapses`

	var (
		rows pgx.Rows
		err  error
	)
	if language == "" {
		rows, err = h.db.Query(ctx,
			`SELECT `+cols+`
			 FROM cards
			 WHERE user_id = $1 AND deleted_at IS NULL
			 ORDER BY created_at`,
			userID,
		)
	} else {
		rows, err = h.db.Query(ctx,
			`SELECT `+cols+`
			 FROM cards
			 WHERE user_id = $1 AND language_code = $2 AND deleted_at IS NULL
			 ORDER BY created_at`,
			userID, language,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []exportCardRow
	for rows.Next() {
		var c exportCardRow
		var front *string // front_text is nullable in the schema
		if err := rows.Scan(
			&front, &c.Reading, &c.BackText,
			&c.Sentence, &c.SubtitleTranslation, &c.SourceURL,
			&c.FrontAudioURL, &c.FrontImageURL, &c.BackAudioURL, &c.SentenceAudioURL,
			&c.FsrsState, &c.Stability, &c.Difficulty, &c.Reps, &c.Lapses,
		); err != nil {
			return nil, err
		}
		if front != nil {
			c.FrontText = *front
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
