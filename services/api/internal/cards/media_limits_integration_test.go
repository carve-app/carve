package cards

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/carve-app/carve/services/api/internal/auth"
	"github.com/carve-app/carve/services/api/internal/db"
	"github.com/go-chi/chi/v5"
)

func TestAttachMediaRejectsOversizedTotalBody(t *testing.T) {
	pool := db.SetupPostgres(t)
	ctx := context.Background()
	userID := auth.NewID()
	cardID := auth.NewID()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, display_name) VALUES ($1, $2, 'Media Limit Proof')`,
		userID, "media-limit-"+userID+"@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO cards (id, user_id, language_code, front_text, fsrs_state) VALUES ($1, $2, 'en', 'oversized', 'new')`,
		cardID, userID); err != nil {
		t.Fatal(err)
	}

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("image", "oversized.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(bytes.Repeat([]byte{'x'}, (26<<20)+1)); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/cards/"+cardID+"/media", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", cardID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{UserID: userID}))
	w := httptest.NewRecorder()
	NewHandler(pool).AttachMedia(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", w.Code, w.Body.String())
	}
}
