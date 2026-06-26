package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/carve-app/carve/services/api/internal/discover"
	"github.com/carve-app/carve/services/api/internal/reports"
	"github.com/carve-app/carve/services/api/internal/stats"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	snapshotJobLock int64 = 0x4341525601
	discoverJobLock int64 = 0x4341525602
	reportsJobLock  int64 = 0x4341525603
)

// ScheduledJobs is the background-work boundary. Production delegates to the
// concrete packages; tests can inject deterministic failures without public
// providers or changing process-global state.
type ScheduledJobs interface {
	Snapshot(context.Context)
	Ingest(context.Context)
	WeeklyReports(context.Context)
}

type productionJobs struct {
	pool     *pgxpool.Pool
	ingester *discover.Ingester
}

func (j productionJobs) Snapshot(ctx context.Context) { stats.SnapshotAllUsers(ctx, j.pool) }
func (j productionJobs) Ingest(ctx context.Context)   { j.ingester.IngestAll(ctx, 30) }
func (j productionJobs) WeeklyReports(ctx context.Context) {
	reports.SendWeeklyDigestToAll(ctx, j.pool)
}

func startBackgroundJobs(parent context.Context, pool *pgxpool.Pool, ingester *discover.Ingester) context.CancelFunc {
	return startBackgroundJobsWithScheduler(parent, pool, productionJobs{pool: pool, ingester: ingester})
}

func startBackgroundJobsWithScheduler(parent context.Context, pool *pgxpool.Pool, jobs ScheduledJobs) context.CancelFunc {
	ctx, cancel := context.WithCancel(parent)
	var wg sync.WaitGroup
	start := func(run func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			run()
		}()
	}
	start(func() { runDailySnapshots(ctx, pool, jobs) })
	start(func() { runDiscoverIngest(ctx, pool, jobs) })
	start(func() { runWeeklyReports(ctx, pool, jobs) })
	var once sync.Once
	return func() {
		once.Do(cancel)
		wg.Wait()
	}
}

func withAdvisoryLock(ctx context.Context, pool *pgxpool.Pool, key int64, run func(context.Context)) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("background job: acquire connection", "error", err, "lock", key)
		}
		return
	}
	defer conn.Release()

	var locked bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&locked); err != nil || !locked {
		if err != nil && ctx.Err() == nil {
			slog.Warn("background job: acquire advisory lock", "error", err, "lock", key)
		}
		return
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = conn.Exec(unlockCtx, "SELECT pg_advisory_unlock($1)", key)
	}()
	run(ctx)
}

func waitUntil(ctx context.Context, when time.Time) bool {
	timer := time.NewTimer(time.Until(when))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func runDailySnapshots(ctx context.Context, pool *pgxpool.Pool, jobs ScheduledJobs) {
	for {
		now := time.Now().UTC()
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 5, 0, 0, time.UTC)
		if !waitUntil(ctx, next) {
			return
		}
		withAdvisoryLock(ctx, pool, snapshotJobLock, func(jobCtx context.Context) {
			jobs.Snapshot(jobCtx)
		})
	}
}

func runDiscoverIngest(ctx context.Context, pool *pgxpool.Pool, jobs ScheduledJobs) {
	run := func() {
		withAdvisoryLock(ctx, pool, discoverJobLock, func(jobCtx context.Context) {
			bounded, cancel := context.WithTimeout(jobCtx, 10*time.Minute)
			defer cancel()
			jobs.Ingest(bounded)
		})
	}
	run()
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func nextWeeklyReport(now time.Time) time.Time {
	now = now.UTC()
	daysUntilMon := (8 - int(now.Weekday())) % 7
	if daysUntilMon == 0 && now.Hour() >= 8 {
		daysUntilMon = 7
	}
	return time.Date(now.Year(), now.Month(), now.Day()+daysUntilMon, 8, 0, 0, 0, time.UTC)
}

func runWeeklyReports(ctx context.Context, pool *pgxpool.Pool, jobs ScheduledJobs) {
	for {
		if !waitUntil(ctx, nextWeeklyReport(time.Now())) {
			return
		}
		withAdvisoryLock(ctx, pool, reportsJobLock, func(jobCtx context.Context) {
			bounded, cancel := context.WithTimeout(jobCtx, 30*time.Minute)
			defer cancel()
			jobs.WeeklyReports(bounded)
		})
	}
}
