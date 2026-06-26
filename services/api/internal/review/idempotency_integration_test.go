package review

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/carve-app/carve/services/api/internal/auth"
	"github.com/carve-app/carve/services/api/internal/db"
)

func TestSubmitEvent_IdempotentReplayDoesNotReschedule(t *testing.T) {
	pool := db.SetupPostgres(t)
	ctx := context.Background()
	userID := auth.NewID()
	cardID := auth.NewID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, display_name) VALUES ($1, $2, 'Replay Tester')`,
		userID, userID+"@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO cards (id, user_id, language_code, front_text)
		 VALUES ($1, $2, 'en', 'idempotent')`, cardID, userID); err != nil {
		t.Fatal(err)
	}

	payload := map[string]any{
		"event_id":      "9aa88ff7-f1da-4ff1-84ee-3650fcb2ba6e",
		"card_id":       cardID,
		"rating":        3,
		"time_taken_ms": 1200,
		"reviewed_at":   "2026-06-26T12:00:00Z",
	}
	body, _ := json.Marshal(payload)
	h := NewHandler(pool)
	responses := make([]map[string]any, 0, 2)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/review/events", bytes.NewReader(body))
		req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{UserID: userID}))
		w := httptest.NewRecorder()
		h.SubmitEvent(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("attempt %d: expected 200, got %d: %s", i+1, w.Code, w.Body.String())
		}
		var response map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		responses = append(responses, response)
	}
	if responses[0]["reps"] != responses[1]["reps"] || responses[0]["due"] != responses[1]["due"] {
		t.Fatalf("replay response changed: first=%v second=%v", responses[0], responses[1])
	}

	var reps, events int
	if err := pool.QueryRow(ctx, `SELECT fsrs_reps FROM cards WHERE id = $1`, cardID).Scan(&reps); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM review_events WHERE user_id = $1`, userID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if reps != 1 || events != 1 {
		t.Fatalf("expected one scheduling transition and event, got reps=%d events=%d", reps, events)
	}
}
