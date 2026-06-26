package onboarding

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/carve-app/carve/services/api/internal/auth"
	"github.com/carve-app/carve/services/api/internal/db"
)

func TestStarterDeckCopiesOnlySystemCardsForMultipleUsers(t *testing.T) {
	pool := db.SetupPostgres(t)
	ctx := context.Background()
	h := NewHandler(pool)

	var expected int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM cards c
		WHERE c.deck_id = (
		  SELECT id FROM decks
		  WHERE language_code = 'ja' AND is_official AND deleted_at IS NULL
		  ORDER BY name LIMIT 1
		)
		  AND c.user_id = '00000000-0000-0000-0000-000000000000'::uuid
		  AND c.deleted_at IS NULL
	`).Scan(&expected); err != nil {
		t.Fatal(err)
	}
	if expected == 0 {
		t.Fatal("official Japanese starter cards are missing")
	}

	for i := 1; i <= 2; i++ {
		userID := auth.NewID()
		if _, err := pool.Exec(ctx,
			`INSERT INTO users (id, email, display_name) VALUES ($1, $2, 'Starter Proof')`,
			userID, fmt.Sprintf("starter-%d@example.com", i)); err != nil {
			t.Fatal(err)
		}

		for attempt := 0; attempt < 2; attempt++ {
			req := httptest.NewRequest(http.MethodPost, "/v1/onboarding/starter-deck", bytes.NewBufferString(`{"language":"ja"}`))
			req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{UserID: userID}))
			w := httptest.NewRecorder()
			h.StarterDeck(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("user %d attempt %d: got %d: %s", i, attempt+1, w.Code, w.Body.String())
			}
		}

		var got int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM cards WHERE user_id = $1 AND language_code = 'ja' AND deleted_at IS NULL`,
			userID).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != expected {
			t.Fatalf("user %d has %d cards after repeated subscribe, want %d", i, got, expected)
		}
	}
}
