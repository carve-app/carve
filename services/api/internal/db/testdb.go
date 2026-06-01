// Package db's testdb.go provides a testcontainers-backed Postgres pool
// for integration tests. Every call to SetupPostgres boots an ephemeral
// container, applies every migration in services/api/migrations/, and
// returns a connected pgxpool plus a teardown function.
//
// Tests that need real SQL should use this instead of the `db = nil`
// mock pattern. The fast unit-style tests still run; integration tests
// run alongside them in the same `go test ./...` invocation and are
// only skipped when SKIP_INTEGRATION=1 is set (useful for very fast
// local iteration).

package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// SetupPostgres boots a Postgres container, applies every migration, and
// returns a connected pool. Cleanup is registered via t.Cleanup, so the
// container disappears when the test ends.
//
// If SKIP_INTEGRATION=1 is set in the environment, the test is skipped
// immediately. Use this knob for fast inner-loop iteration when you know
// you're only touching pure-logic code.
func SetupPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if os.Getenv("SKIP_INTEGRATION") == "1" {
		t.Skip("SKIP_INTEGRATION=1 — skipping testcontainers integration")
	}

	ctx := context.Background()

	c, err := tcpg.Run(ctx,
		"postgres:16-alpine",
		tcpg.WithDatabase("carve_test"),
		tcpg.WithUsername("test"),
		tcpg.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("postgres testcontainer failed to start: %v", err)
	}
	t.Cleanup(func() { _ = c.Terminate(context.Background()) })

	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgx connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := applyMigrations(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	return pool
}

// applyMigrations runs every .sql file in services/api/migrations/ in
// numeric order. They are written with `IF NOT EXISTS` guards so the
// container's empty database starts as a fresh, fully-schema'd test DB.
func applyMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	migrationsDir := locateMigrationsDir()
	if migrationsDir == "" {
		return fmt.Errorf("could not locate services/api/migrations/")
	}

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		data, err := os.ReadFile(filepath.Join(migrationsDir, name))
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx, string(data)); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
	}
	return nil
}

// locateMigrationsDir walks up from the test's working directory looking
// for services/api/migrations/. Tests run from internal/<pkg>/, so we
// need to climb a few levels.
func locateMigrationsDir() string {
	cwd, _ := os.Getwd()
	for i := 0; i < 6; i++ {
		candidate := filepath.Join(cwd, "migrations")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		cwd = filepath.Dir(cwd)
	}
	return ""
}
