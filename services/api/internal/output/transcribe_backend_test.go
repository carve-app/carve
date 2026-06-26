package output

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/carve-app/carve/services/api/internal/auth"
)

// TestTranscribe_UsesSTTBackend stands up a stub STT backend that
// echoes a hardcoded transcript, points the handler at it via
// STT_BACKEND_URL, and asserts the response carries the backend
// transcript + `backend_used=true`.
func TestTranscribe_UsesSTTBackend(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(20 << 20); err != nil {
			t.Errorf("stub: parse multipart: %v", err)
		}
		// Verify we received the audio part and the language field.
		if r.FormValue("language") != "en" {
			t.Errorf("stub: expected language=en, got %q", r.FormValue("language"))
		}
		if _, _, err := r.FormFile("audio"); err != nil {
			t.Errorf("stub: missing audio: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"text":"hello from backend"}`)
	}))
	defer stub.Close()

	t.Setenv("STT_BACKEND_URL", stub.URL)

	h := &Handler{}
	req := buildTranscribeRequest(t, "expected text", "client hypothesis", "en", []byte("\x00fake audio\xff"))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{UserID: "user-001"}))

	w := httptest.NewRecorder()
	h.Transcribe(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["transcript"] != "hello from backend" {
		t.Errorf("expected transcript from backend, got %v", resp["transcript"])
	}
	if resp["backend_used"] != true {
		t.Errorf("expected backend_used=true, got %v", resp["backend_used"])
	}
}

// TestTranscribe_FallsBackToHypothesisOnBackendError ensures a 500 from
// the backend doesn't break the handler — it falls through to the
// client's hypothesis so the diff/WER pipeline still runs.
func TestTranscribe_FallsBackToHypothesisOnBackendError(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "intentional failure", 500)
	}))
	defer stub.Close()

	t.Setenv("STT_BACKEND_URL", stub.URL)

	h := &Handler{}
	req := buildTranscribeRequest(t, "expected", "client hyp", "en", []byte("fake"))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{UserID: "user-002"}))

	w := httptest.NewRecorder()
	h.Transcribe(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200 even on backend error, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["transcript"] != "client hyp" {
		t.Errorf("expected client hypothesis, got %v", resp["transcript"])
	}
	if resp["backend_used"] != false {
		t.Errorf("expected backend_used=false, got %v", resp["backend_used"])
	}
}

// TestTranscribe_NoBackendConfigured uses the bare client hypothesis as
// the transcript (matches the pre-backend behavior).
func TestTranscribe_NoBackendConfigured(t *testing.T) {
	t.Setenv("STT_BACKEND_URL", "")

	h := &Handler{}
	req := buildTranscribeRequest(t, "expected", "hyp only", "en", nil)
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{UserID: "user-003"}))

	w := httptest.NewRecorder()
	h.Transcribe(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["transcript"] != "hyp only" {
		t.Errorf("expected 'hyp only', got %v", resp["transcript"])
	}
}

func TestTranscribe_RejectsOversizedAudio(t *testing.T) {
	h := &Handler{}
	req := buildTranscribeRequest(t, "expected", "hypothesis", "en", bytes.Repeat([]byte("x"), 21<<20))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{UserID: "user-004"}))
	w := httptest.NewRecorder()
	h.Transcribe(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", w.Code, w.Body.String())
	}
}

// ── helpers ────────────────────────────────────────────────────────────────

func buildTranscribeRequest(t *testing.T, expected, hypothesis, language string, audio []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("expected", expected)
	_ = mw.WriteField("hypothesis", hypothesis)
	_ = mw.WriteField("language", language)
	if len(audio) > 0 {
		part, err := mw.CreateFormFile("audio", "utterance.webm")
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := part.Write(audio); err != nil {
			t.Fatalf("write audio: %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close mw: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/output/transcribe", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

// quick sanity that the helper builds a parseable payload
func TestBuildTranscribeRequest_Roundtrip(t *testing.T) {
	r := buildTranscribeRequest(t, "ref", "hyp", "en", []byte("x"))
	if err := r.ParseMultipartForm(20 << 20); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if r.FormValue("expected") != "ref" || r.FormValue("hypothesis") != "hyp" || r.FormValue("language") != "en" {
		t.Errorf("fields lost in roundtrip: %+v", r.Form)
	}
	if !strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
		t.Errorf("wrong content type: %s", r.Header.Get("Content-Type"))
	}
}
