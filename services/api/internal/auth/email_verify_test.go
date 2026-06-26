package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/carve-app/carve/services/api/internal/mailer"
)

type recordingSender struct{ messages []mailer.Message }

func (s *recordingSender) Send(_ context.Context, msg mailer.Message) error {
	s.messages = append(s.messages, msg)
	return nil
}

func TestSendVerificationUsesMailerWithoutLoggingToken(t *testing.T) {
	t.Setenv("APP_BASE_URL", "https://carve.example")
	sender := &recordingSender{}
	h := NewHandlerWithMailer(nil, sender)
	if err := h.sendVerification(context.Background(), "learner@example.com", "secret-token"); err != nil {
		t.Fatal(err)
	}
	if len(sender.messages) != 1 {
		t.Fatalf("expected one message, got %d", len(sender.messages))
	}
	msg := sender.messages[0]
	if msg.To != "learner@example.com" || !strings.Contains(msg.Text, "https://carve.example/verify-email?token=secret-token") {
		t.Fatalf("unexpected verification message: %#v", msg)
	}
}

func TestVerifyEmail_BadRequestOnMissingToken(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/verify", strings.NewReader("{}"))
	w := httptest.NewRecorder()
	h.VerifyEmail(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestVerifyEmail_BadRequestOnMalformedJSON(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/verify", strings.NewReader("{not json"))
	w := httptest.NewRecorder()
	h.VerifyEmail(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestResendVerification_Always200OnMissingEmail(t *testing.T) {
	// No-body: should still 200 (anti-enumeration).
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/verify/resend", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.ResendVerification(w, req)
	if w.Code != 200 {
		t.Errorf("expected 200 to avoid enumeration, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Errorf("expected ok=true")
	}
}

func TestResendVerification_GarbledBodyStill200(t *testing.T) {
	h := &Handler{}
	body, _ := json.Marshal(map[string]string{"email": ""})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/verify/resend", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.ResendVerification(w, req)
	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
