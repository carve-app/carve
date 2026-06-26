package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/carve-app/carve/services/api/internal/db"
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

func TestWithAdvisoryLockExcludesConcurrentReplica(t *testing.T) {
	pool := db.SetupPostgres(t)
	ctx := context.Background()
	const lockKey int64 = 0x43415256ff

	holder, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()
	if _, err := holder.Exec(ctx, "SELECT pg_advisory_lock($1)", lockKey); err != nil {
		t.Fatal(err)
	}

	var runs atomic.Int32
	withAdvisoryLock(ctx, pool, lockKey, func(context.Context) { runs.Add(1) })
	if got := runs.Load(); got != 0 {
		t.Fatalf("second replica ran the job while lock was held: %d", got)
	}
	if _, err := holder.Exec(ctx, "SELECT pg_advisory_unlock($1)", lockKey); err != nil {
		t.Fatal(err)
	}

	withAdvisoryLock(ctx, pool, lockKey, func(context.Context) { runs.Add(1) })
	if got := runs.Load(); got != 1 {
		t.Fatalf("job did not run after lock release: %d", got)
	}
}
