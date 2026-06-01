package reports

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRenderWeeklyEmail_IncludesAllFields(t *testing.T) {
	r := &WeeklyReport{
		Language:         "ja",
		WeekStart:        time.Date(2024, 2, 12, 0, 0, 0, 0, time.UTC),
		WeekEnd:          time.Date(2024, 2, 19, 0, 0, 0, 0, time.UTC),
		CardsMined:       12,
		ReviewsCompleted: 240,
		ImmersionMinutes: 95,
		NewKnownWords:    8,
		RetentionRate:    0.91,
		StreakDays:       14,
	}
	body := RenderWeeklyEmail("Alex", r)
	wants := []string{"Alex", "12", "240", "95", "8", "91%", "14", "Feb 12", "Feb 19"}
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("missing %q in body:\n%s", w, body)
		}
	}
}

func TestRenderWeeklyEmail_HandlesZeroReviews(t *testing.T) {
	r := &WeeklyReport{
		CardsMined:       0,
		ReviewsCompleted: 0,
		RetentionRate:    0,
		StreakDays:       1,
	}
	body := RenderWeeklyEmail("", r)
	if !strings.Contains(body, "Hi there,") {
		t.Errorf("missing default greeting: %s", body)
	}
	if !strings.Contains(body, "—") {
		t.Errorf("expected dash placeholder for retention with zero reviews:\n%s", body)
	}
}

func TestRenderWeeklyEmail_StreakSingularPlural(t *testing.T) {
	one := RenderWeeklyEmail("x", &WeeklyReport{StreakDays: 1})
	if !strings.Contains(one, "1 day") || strings.Contains(one, "1 days") {
		t.Errorf("streak=1 should say 'day' not 'days':\n%s", one)
	}
	many := RenderWeeklyEmail("x", &WeeklyReport{StreakDays: 30})
	if !strings.Contains(many, "30 days") {
		t.Errorf("streak=30 should say 'days':\n%s", many)
	}
}

func TestLoadSMTPConfig_NoHostReturnsFalse(t *testing.T) {
	os.Unsetenv("SMTP_HOST")
	cfg, ok := LoadSMTPConfig()
	if ok || cfg != nil {
		t.Errorf("expected (nil, false), got (%v, %v)", cfg, ok)
	}
}

func TestLoadSMTPConfig_DefaultsPort587(t *testing.T) {
	os.Setenv("SMTP_HOST", "smtp.example.com")
	defer os.Unsetenv("SMTP_HOST")
	os.Unsetenv("SMTP_PORT")
	cfg, ok := LoadSMTPConfig()
	if !ok {
		t.Fatal("expected ok")
	}
	if cfg.Port != "587" {
		t.Errorf("expected port=587, got %q", cfg.Port)
	}
}

func TestLoadSMTPConfig_FromFallsBackToUser(t *testing.T) {
	os.Setenv("SMTP_HOST", "smtp.example.com")
	os.Setenv("SMTP_USER", "user@example.com")
	defer func() {
		os.Unsetenv("SMTP_HOST")
		os.Unsetenv("SMTP_USER")
	}()
	os.Unsetenv("SMTP_FROM")
	cfg, _ := LoadSMTPConfig()
	if cfg.From != "user@example.com" {
		t.Errorf("expected From to fall back to SMTP_USER, got %q", cfg.From)
	}
}

func TestSendWeekly_NilConfigIsNoOp(t *testing.T) {
	if err := SendWeekly(nil, "user@example.com", "User", &WeeklyReport{}); err != nil {
		t.Errorf("nil cfg should be no-op, got %v", err)
	}
}

func TestWeeklyHandler_Unauthorized(t *testing.T) {
	h := &Handler{db: nil}
	r := httptest.NewRequest(http.MethodGet, "/v1/reports/weekly", nil)
	w := httptest.NewRecorder()
	h.Weekly(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "  ", "a"); got != "a" {
		t.Errorf("got %q", got)
	}
}
