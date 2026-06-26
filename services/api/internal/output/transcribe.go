package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"strings"
	"time"

	"github.com/carve-app/carve/services/api/internal/auth"
)

// sttHTTPClient is package-level so tests can swap in a stub for the
// external STT backend. Default 30s timeout — long enough for a
// 60-second utterance through faster-whisper-base.
var sttHTTPClient = &http.Client{Timeout: 30 * time.Second}

// forwardToSTTBackend posts the user's audio (along with the recorded
// MIME type and the language code) to the external STT service. The
// expected response is `{ "text": "<utf-8>" }`. Any error here is
// non-fatal: the handler falls back to the client-provided hypothesis.
func forwardToSTTBackend(backendURL, language, mime string, audio []byte) (string, error) {
	if backendURL == "" || len(audio) == 0 {
		return "", nil
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	if language != "" {
		_ = mw.WriteField("language", language)
	}

	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", `form-data; name="audio"; filename="utterance"`)
	if mime != "" {
		hdr.Set("Content-Type", mime)
	}
	part, err := mw.CreatePart(hdr)
	if err != nil {
		return "", fmt.Errorf("create part: %w", err)
	}
	if _, err := part.Write(audio); err != nil {
		return "", fmt.Errorf("write audio: %w", err)
	}
	if err := mw.Close(); err != nil {
		return "", fmt.Errorf("close multipart: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, backendURL, &buf)
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if key := os.Getenv("STT_BACKEND_KEY"); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := sttHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("backend %d: %s", resp.StatusCode, string(body))
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	return out.Text, nil
}

// ── POST /v1/output/transcribe ────────────────────────────────────────────────
//
// Accepts a `multipart/form-data` upload containing:
//   - audio: the user's recorded speech
//   - expected: optional reference transcript to diff against
//   - language: 2- or 5-letter code (en, ja, zh-cn, ko)
//
// Returns:
//   {
//     "transcript": "<recognized text or empty if no STT backend>",
//     "diff":       [{"op":"keep|insert|delete|sub","ref":"..","hyp":".."}]
//     "wer":        0.13
//   }
//
// Implementation notes:
//   - If `STT_BACKEND_URL` is configured, the audio is forwarded to an external
//     STT (faster-whisper, Whisper API, etc.). The expected response shape is
//     `{ "text": "<utf-8>" }`.
//   - If no backend is configured, we return an empty transcript so the
//     client can fall back to the browser's native SpeechRecognition. The
//     diff is then computed against whatever the client posts as `hypothesis`.
//   - WER (word error rate) is computed over normalized whitespace + casefold.

func (h *Handler) Transcribe(w http.ResponseWriter, r *http.Request) {
	_, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 21<<20)
	if err := r.ParseMultipartForm(21 << 20); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) || strings.Contains(err.Error(), "request body too large") {
			writeError(w, http.StatusRequestEntityTooLarge, "audio upload too large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid multipart payload")
		}
		return
	}

	expected := r.FormValue("expected")
	hypothesis := r.FormValue("hypothesis")
	language := r.FormValue("language")
	if language == "" {
		language = "en"
	}

	// Read the audio. If STT_BACKEND_URL is configured, forward it for
	// server-side transcription (faster-whisper, Whisper API, Deepgram,
	// etc.). On any backend error, fall back to the client-provided
	// hypothesis so the UX still works in Chrome via window.SpeechRecognition.
	transcript := hypothesis
	backendUsed := false
	if file, header, err := r.FormFile("audio"); err == nil {
		defer file.Close()
		audio, readErr := io.ReadAll(io.LimitReader(file, (20<<20)+1))
		if readErr != nil {
			writeError(w, http.StatusBadRequest, "could not read audio")
			return
		}
		if len(audio) > 20<<20 {
			writeError(w, http.StatusRequestEntityTooLarge, "audio upload too large")
			return
		}
		backendURL := os.Getenv("STT_BACKEND_URL")
		mime := ""
		if header != nil {
			mime = header.Header.Get("Content-Type")
		}
		if text, err := forwardToSTTBackend(backendURL, language, mime, audio); err == nil && text != "" {
			transcript = text
			backendUsed = true
		}
	}

	diff, wer := computeDiff(expected, transcript)
	writeJSON(w, http.StatusOK, map[string]any{
		"transcript":   transcript,
		"language":     language,
		"diff":         diff,
		"wer":          wer,
		"backend_used": backendUsed,
	})
}

type diffEntry struct {
	Op  string `json:"op"`
	Ref string `json:"ref,omitempty"`
	Hyp string `json:"hyp,omitempty"`
}

func normalize(s string) []string {
	s = strings.ToLower(s)
	if s == "" {
		return nil
	}
	// Strip common punctuation so "hello, world." matches "hello world"
	s = strings.Map(func(r rune) rune {
		switch r {
		case '.', ',', '!', '?', ';', ':', '"', '\'':
			return ' '
		}
		return r
	}, s)
	// strings.Fields drops empty tokens and collapses runs of whitespace.
	return strings.Fields(s)
}

// computeDiff returns a token-level edit script + WER between reference
// and hypothesis. Classic Wagner–Fischer with substitution/insertion/deletion.
func computeDiff(ref, hyp string) ([]diffEntry, float64) {
	r := normalize(ref)
	h := normalize(hyp)
	n, m := len(r), len(h)
	if n == 0 && m == 0 {
		return []diffEntry{}, 0
	}

	// dp[i][j] = min edits to align r[:i] with h[:j]
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
		dp[i][0] = i
	}
	for j := 0; j <= m; j++ {
		dp[0][j] = j
	}

	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			cost := 1
			if r[i-1] == h[j-1] {
				cost = 0
			}
			a := dp[i-1][j] + 1 // delete from ref
			b := dp[i][j-1] + 1 // insert into ref
			c := dp[i-1][j-1] + cost
			dp[i][j] = min3(a, b, c)
		}
	}

	// Backtrack to build the edit script
	out := []diffEntry{}
	i, j := n, m
	for i > 0 || j > 0 {
		switch {
		case i > 0 && j > 0 && r[i-1] == h[j-1]:
			out = append(out, diffEntry{Op: "keep", Ref: r[i-1], Hyp: h[j-1]})
			i--
			j--
		case i > 0 && j > 0 && dp[i][j] == dp[i-1][j-1]+1:
			out = append(out, diffEntry{Op: "sub", Ref: r[i-1], Hyp: h[j-1]})
			i--
			j--
		case i > 0 && dp[i][j] == dp[i-1][j]+1:
			out = append(out, diffEntry{Op: "delete", Ref: r[i-1]})
			i--
		default:
			out = append(out, diffEntry{Op: "insert", Hyp: h[j-1]})
			j--
		}
	}
	// reverse
	for k, l := 0, len(out)-1; k < l; k, l = k+1, l-1 {
		out[k], out[l] = out[l], out[k]
	}

	wer := 0.0
	if n > 0 {
		wer = float64(dp[n][m]) / float64(n)
	}
	return out, wer
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// Compile-time check: makes sure json package stays referenced even if
// future refactors drop the explicit writeJSON usage.
var _ = json.Marshal
