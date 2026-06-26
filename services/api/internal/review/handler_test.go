package review

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/carve-app/carve/services/api/internal/auth"
)

func newReviewHandler() *Handler {
	return &Handler{db: nil}
}

func authedCtx(r *http.Request) *http.Request {
	return r.WithContext(auth.ContextWithClaims(r.Context(), &auth.Claims{UserID: "user-test-001"}))
}

// ── Auth guards ───────────────────────────────────────────────────────────────

func TestDueCount_NoAuth(t *testing.T) {
	h := newReviewHandler()
	req := httptest.NewRequest(http.MethodGet, "/v1/review/due-count", nil)
	w := httptest.NewRecorder()
	h.DueCount(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSession_NoAuth(t *testing.T) {
	h := newReviewHandler()
	req := httptest.NewRequest(http.MethodGet, "/v1/review/session", nil)
	w := httptest.NewRecorder()
	h.Session(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSubmitEvent_NoAuth(t *testing.T) {
	h := newReviewHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/review/events", nil)
	w := httptest.NewRecorder()
	h.SubmitEvent(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestIntervals_NoAuth(t *testing.T) {
	h := newReviewHandler()
	req := httptest.NewRequest(http.MethodGet, "/v1/review/intervals", nil)
	w := httptest.NewRecorder()
	h.Intervals(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestForecast_NoAuth(t *testing.T) {
	h := newReviewHandler()
	req := httptest.NewRequest(http.MethodGet, "/v1/review/forecast", nil)
	w := httptest.NewRecorder()
	h.Forecast(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestNotifications_NoAuth(t *testing.T) {
	h := newReviewHandler()
	req := httptest.NewRequest(http.MethodGet, "/v1/review/notifications", nil)
	w := httptest.NewRecorder()
	h.Notifications(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestMarkNotificationRead_NoAuth(t *testing.T) {
	h := newReviewHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/review/notifications/abc/read", nil)
	w := httptest.NewRecorder()
	h.MarkNotificationRead(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ── SubmitEvent validation ────────────────────────────────────────────────────

func TestSubmitEvent_InvalidJSON(t *testing.T) {
	h := newReviewHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/review/events", strings.NewReader("{bad json"))
	req = authedCtx(req)
	w := httptest.NewRecorder()
	h.SubmitEvent(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSubmitEvent_MissingCardID(t *testing.T) {
	h := newReviewHandler()
	body, _ := json.Marshal(map[string]any{"rating": 3})
	req := httptest.NewRequest(http.MethodPost, "/v1/review/events", bytes.NewReader(body))
	req = authedCtx(req)
	w := httptest.NewRecorder()
	h.SubmitEvent(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing card_id, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "card_id") {
		t.Errorf("expected 'card_id' in error, got: %s", w.Body.String())
	}
}

func TestSubmitEvent_InvalidEventID(t *testing.T) {
	h := newReviewHandler()
	body, _ := json.Marshal(map[string]any{
		"event_id": "not-a-uuid", "card_id": "some-id", "rating": 3,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/review/events", bytes.NewReader(body))
	req = authedCtx(req)
	w := httptest.NewRecorder()
	h.SubmitEvent(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid event_id, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "event_id") {
		t.Fatalf("expected event_id error, got %s", w.Body.String())
	}
}

func TestSubmitEvent_RatingZero(t *testing.T) {
	h := newReviewHandler()
	body, _ := json.Marshal(map[string]any{"card_id": "some-id", "rating": 0})
	req := httptest.NewRequest(http.MethodPost, "/v1/review/events", bytes.NewReader(body))
	req = authedCtx(req)
	w := httptest.NewRecorder()
	h.SubmitEvent(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for rating=0, got %d", w.Code)
	}
}

func TestSubmitEvent_RatingFive(t *testing.T) {
	h := newReviewHandler()
	body, _ := json.Marshal(map[string]any{"card_id": "some-id", "rating": 5})
	req := httptest.NewRequest(http.MethodPost, "/v1/review/events", bytes.NewReader(body))
	req = authedCtx(req)
	w := httptest.NewRecorder()
	h.SubmitEvent(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for rating=5, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "1") && !strings.Contains(w.Body.String(), "rating") {
		t.Errorf("expected rating error, got: %s", w.Body.String())
	}
}

func TestSubmitEvent_RatingNegative(t *testing.T) {
	h := newReviewHandler()
	body, _ := json.Marshal(map[string]any{"card_id": "some-id", "rating": -1})
	req := httptest.NewRequest(http.MethodPost, "/v1/review/events", bytes.NewReader(body))
	req = authedCtx(req)
	w := httptest.NewRecorder()
	h.SubmitEvent(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for rating=-1, got %d", w.Code)
	}
}

func TestSubmitEvent_RatingErrorMessage(t *testing.T) {
	h := newReviewHandler()
	body, _ := json.Marshal(map[string]any{"card_id": "some-id", "rating": 10})
	req := httptest.NewRequest(http.MethodPost, "/v1/review/events", bytes.NewReader(body))
	req = authedCtx(req)
	w := httptest.NewRecorder()
	h.SubmitEvent(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for rating=10, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "rating") {
		t.Errorf("expected 'rating' in error message, got: %s", w.Body.String())
	}
}
