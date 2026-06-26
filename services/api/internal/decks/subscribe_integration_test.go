package decks

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/carve-app/carve/services/api/internal/auth"
	"github.com/carve-app/carve/services/api/internal/db"
	"github.com/go-chi/chi/v5"
)

func TestSubscribeCopiesOnlyTemplateCardsForMultipleUsers(t *testing.T) {
	pool := db.SetupPostgres(t)
	ctx := context.Background()
	h := NewHandler(pool)

	var deckID string
	var expected int
	if err := pool.QueryRow(ctx, `
		SELECT d.id, count(c.id)
		FROM decks d
		JOIN cards c ON c.deck_id = d.id
		WHERE d.is_official AND d.is_public AND d.deleted_at IS NULL
		  AND c.user_id = '00000000-0000-0000-0000-000000000000'::uuid
		  AND c.deleted_at IS NULL
		GROUP BY d.id
		ORDER BY d.name
		LIMIT 1
	`).Scan(&deckID, &expected); err != nil {
		t.Fatal(err)
	}
	if expected == 0 {
		t.Fatal("official template deck has no cards")
	}

	for i := 1; i <= 2; i++ {
		userID := auth.NewID()
		if _, err := pool.Exec(ctx,
			`INSERT INTO users (id, email, display_name) VALUES ($1, $2, 'Subscribe Proof')`,
			userID, fmt.Sprintf("subscribe-%d@example.com", i)); err != nil {
			t.Fatal(err)
		}

		for attempt := 0; attempt < 2; attempt++ {
			req := httptest.NewRequest(http.MethodPost, "/v1/decks/"+deckID+"/subscribe", nil)
			routeCtx := chi.NewRouteContext()
			routeCtx.URLParams.Add("id", deckID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
			req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{UserID: userID}))
			w := httptest.NewRecorder()
			h.Subscribe(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("user %d attempt %d: got %d: %s", i, attempt+1, w.Code, w.Body.String())
			}
		}

		var got int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM cards WHERE user_id = $1 AND deck_id = $2 AND deleted_at IS NULL`,
			userID, deckID).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != expected {
			t.Fatalf("user %d has %d cards after repeated subscribe, want %d", i, got, expected)
		}
	}
}
