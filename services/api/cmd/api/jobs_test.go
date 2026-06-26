package main

import (
	"context"
	"testing"
	"time"
)

func TestWaitUntilCancelsPromptly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if waitUntil(ctx, time.Now().Add(time.Hour)) {
		t.Fatal("cancelled wait should return false")
	}
}

func TestNextWeeklyReport(t *testing.T) {
	mondayMorning := time.Date(2026, 6, 22, 7, 30, 0, 0, time.UTC)
	if got := nextWeeklyReport(mondayMorning); !got.Equal(time.Date(2026, 6, 22, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("before 08:00 should schedule same Monday, got %s", got)
	}
	mondayAfter := time.Date(2026, 6, 22, 9, 0, 0, 0, time.UTC)
	if got := nextWeeklyReport(mondayAfter); !got.Equal(time.Date(2026, 6, 29, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("after 08:00 should schedule next Monday, got %s", got)
	}
}
