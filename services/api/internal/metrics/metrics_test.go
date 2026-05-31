package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandler_EmitsExpectedFamilies(t *testing.T) {
	IncRequest("GET", "/v1/cards", 200, 12*time.Millisecond)
	IncRequest("POST", "/v1/cards", 500, 800*time.Millisecond)
	IncCounter("fsrs_event_total")
	IncCounter("fsrs_event_total")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	Handler(w, req)

	body := w.Body.String()
	required := []string{
		"carve_http_requests_total",
		"carve_http_errors_total",
		"carve_http_request_duration_seconds_bucket",
		"carve_http_request_duration_seconds_sum",
		"carve_http_request_duration_seconds_count",
		"carve_app_counter",
		`name="fsrs_event_total"`,
		`path="/v1/cards"`,
	}
	for _, want := range required {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in metrics output:\n%s", want, body)
		}
	}
}
