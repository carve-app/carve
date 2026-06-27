package onboarding

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

func TestSubmitPlacementPersistsAttemptAndOnlyVerifiedWords(t *testing.T) {
	pool := db.SetupPostgres(t)
	ctx := context.Background()
	userID := auth.NewID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, display_name) VALUES ($1, 'placement@example.com', 'Placement Test')`,
		userID,
	); err != nil {
		t.Fatal(err)
	}

	answers := placementAnswers(func(item placementItem) int {
		if item.Band < 4 {
			return item.CorrectIndex
		}
		return -1
	})
	body, err := json.Marshal(submitPlacementRequest{
		Language: "en", Version: englishPlacementVersion, Answers: answers,
	})
	if err != nil {
		t.Fatal(err)
	}

	h := NewHandler(pool)
	req := httptest.NewRequest(http.MethodPost, "/v1/onboarding/placement-test", bytes.NewReader(body))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{UserID: userID}))
	w := httptest.NewRecorder()
	h.SubmitPlacementTest(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}

	var result placementResultPayload
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Correct != 20 || result.VerifiedKnown != 20 || result.EstimatedKnown != 5000 {
		t.Fatalf("unexpected result: %+v", result)
	}

	var attempts, known, activeLanguages int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM placement_test_attempts WHERE user_id = $1 AND language_code = 'en'`,
		userID,
	).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*)
		 FROM user_word_knowledge uwk
		 JOIN words w ON w.id = uwk.word_id
		 WHERE uwk.user_id = $1 AND w.language_code = 'en' AND uwk.status = 'known'`,
		userID,
	).Scan(&known); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM user_languages WHERE user_id = $1 AND language_code = 'en' AND is_active`,
		userID,
	).Scan(&activeLanguages); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || known != 20 || activeLanguages != 1 {
		t.Fatalf("attempts=%d known=%d active_languages=%d", attempts, known, activeLanguages)
	}

	var targetLanguage string
	if err := pool.QueryRow(ctx, `SELECT target_language FROM users WHERE id = $1`, userID).Scan(&targetLanguage); err != nil {
		t.Fatal(err)
	}
	if targetLanguage != "en" {
		t.Fatalf("target_language=%q, want en", targetLanguage)
	}

	var systemCards int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM cards c
		 WHERE c.user_id = '00000000-0000-0000-0000-000000000000'::uuid
		   AND c.language_code = 'en' AND c.deleted_at IS NULL`,
	).Scan(&systemCards); err != nil {
		t.Fatal(err)
	}
	deckReq := httptest.NewRequest(http.MethodPost, "/v1/onboarding/starter-deck", bytes.NewBufferString(`{"language":"en"}`))
	deckReq = deckReq.WithContext(auth.ContextWithClaims(deckReq.Context(), &auth.Claims{UserID: userID}))
	deckW := httptest.NewRecorder()
	h.StarterDeck(deckW, deckReq)
	if deckW.Code != http.StatusOK {
		t.Fatalf("starter deck got %d: %s", deckW.Code, deckW.Body.String())
	}

	var userCards int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM cards WHERE user_id = $1 AND language_code = 'en' AND deleted_at IS NULL`,
		userID,
	).Scan(&userCards); err != nil {
		t.Fatal(err)
	}
	// "respond", "establish", and "subsequent" are both correctly answered
	// items and English starter cards. They must not be copied into the queue.
	if userCards != systemCards-3 {
		t.Fatalf("user cards=%d, want %d after skipping verified words", userCards, systemCards-3)
	}
}
