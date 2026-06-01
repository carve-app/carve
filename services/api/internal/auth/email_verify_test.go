package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
