package output

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/carve-app/carve/services/api/internal/auth"
)

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

	if err := r.ParseMultipartForm(20 << 20); err != nil { // 20 MB cap
		writeError(w, http.StatusBadRequest, "invalid multipart payload")
		return
	}

	expected := r.FormValue("expected")
	hypothesis := r.FormValue("hypothesis")
	language := r.FormValue("language")
	if language == "" {
		language = "en"
	}

	// External backend forwarding is intentionally deferred — when STT_BACKEND_URL
	// is set the handler will round-trip the audio file; until then we let the
	// client send a `hypothesis` (e.g. from window.SpeechRecognition) so the
	// diff/WER endpoints stay useful in browsers that support it.
	if file, header, err := r.FormFile("audio"); err == nil {
		defer file.Close()
		// drain to /dev/null so the upload completes from the client's perspective
		_, _ = io.Copy(io.Discard, file)
		_ = header
	}

	diff, wer := computeDiff(expected, hypothesis)
	writeJSON(w, http.StatusOK, map[string]any{
		"transcript": hypothesis,
		"language":   language,
		"diff":       diff,
		"wer":        wer,
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
			a := dp[i-1][j] + 1   // delete from ref
			b := dp[i][j-1] + 1   // insert into ref
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
