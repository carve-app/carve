// migrate runs SQL migration files against the database.
// Usage: migrate up [--db <dsn>] [--dir <migrations-dir>]
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dir := flag.String("dir", "migrations", "directory containing *.sql migration files")
	flag.Parse()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://carve:carve@localhost:5432/carve?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		slog.Error("connect failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := runMigrations(ctx, pool, *dir); err != nil {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}
	slog.Info("migrations complete")
}

func runMigrations(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	// Create migrations tracking table
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version     TEXT PRIMARY KEY,
			applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	// Get already-applied migrations
	rows, err := pool.Query(ctx, "SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		return fmt.Errorf("query migrations: %w", err)
	}
	applied := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return fmt.Errorf("scan version: %w", err)
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate migrations: %w", err)
	}
	rows.Close()

	// Collect migration files
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return fmt.Errorf("glob migrations: %w", err)
	}
	sort.Strings(files)

	for _, f := range files {
		version := strings.TrimSuffix(filepath.Base(f), ".sql")
		if applied[version] {
			slog.Info("skip (already applied)", "migration", version)
			continue
		}

		sql, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}

		slog.Info("applying migration", "migration", version)
		t0 := time.Now()

		tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return fmt.Errorf("begin tx: %w", err)
		}

		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("exec %s: %w", version, err)
		}

		if _, err := tx.Exec(ctx,
			"INSERT INTO schema_migrations (version) VALUES ($1)", version,
		); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("record %s: %w", version, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit %s: %w", version, err)
		}

		slog.Info("applied", "migration", version, "duration", time.Since(t0).String())
	}
	return nil
}
