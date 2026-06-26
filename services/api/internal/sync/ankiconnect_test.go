package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/carve-app/carve/services/api/internal/auth"
)

// Stub for the local AnkiConnect daemon. Reads the action and replies
// with a canned response shape.
func stubAnki(actions map[string]any) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ankiRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if resp, ok := actions[req.Action]; ok {
			body, _ := json.Marshal(map[string]any{"result": resp})
			_, _ = w.Write(body)
			return
		}
		// Unknown action: success with null result.
		_, _ = w.Write([]byte(`{"result": null}`))
	}))
}

func TestTest_UnauthorizedWhenNoClaims(t *testing.T) {
	h := NewHandler(nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/sync/anki-connect/test", strings.NewReader(`{"url":"x"}`))
	w := httptest.NewRecorder()
	h.Test(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestTest_BadRequestOnMissingURL(t *testing.T) {
	h := NewHandler(nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/sync/anki-connect/test", strings.NewReader(`{}`))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{UserID: "u"}))
	w := httptest.NewRecorder()
	h.Test(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestValidateAnkiConnectURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "localhost", url: "http://localhost:8765"},
		{name: "ipv4 loopback", url: "http://127.0.0.1:8765"},
		{name: "ipv6 loopback", url: "http://[::1]:8765"},
		{name: "remote host", url: "https://example.com", wantErr: true},
		{name: "metadata endpoint", url: "http://169.254.169.254/latest/meta-data", wantErr: true},
		{name: "loopback https", url: "https://localhost:8765", wantErr: true},
		{name: "credentials", url: "http://user:pass@localhost:8765", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAnkiConnectURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateAnkiConnectURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestTest_RejectsRemoteURL(t *testing.T) {
	h := NewHandler(nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/sync/anki-connect/test", strings.NewReader(`{"url":"http://169.254.169.254/latest/meta-data"}`))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{UserID: "u"}))
	w := httptest.NewRecorder()
	h.Test(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTest_ReportsDeckNames(t *testing.T) {
	stub := stubAnki(map[string]any{"deckNames": []string{"Default", "JLPT N5"}})
	defer stub.Close()
	h := NewHandler(nil)
	body, _ := json.Marshal(map[string]string{"url": stub.URL})
	req := httptest.NewRequest(http.MethodPost, "/v1/sync/anki-connect/test", bytes.NewReader(body))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{UserID: "u"}))
	w := httptest.NewRecorder()
	h.Test(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Errorf("expected ok=true, got %+v", resp)
	}
	decks, _ := resp["decks"].([]any)
	if len(decks) != 2 {
		t.Errorf("expected 2 decks, got %v", decks)
	}
}

// TestAnkiRequest_VersionAlwaysSix asserts the protocol version we send
// matches what AnkiConnect 25.x expects.
func TestAnkiRequest_VersionAlwaysSix(t *testing.T) {
	h := NewHandler(nil)
	called := false
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ankiRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Version != 6 {
			t.Errorf("expected version=6, got %d", req.Version)
		}
		called = true
		_, _ = w.Write([]byte(`{"result": []}`))
	}))
	defer stub.Close()

	_, err := h.anki(context.Background(), stub.URL, "deckNames", nil)
	if err != nil {
		t.Fatalf("anki: %v", err)
	}
	if !called {
		t.Error("stub never reached")
	}
}
