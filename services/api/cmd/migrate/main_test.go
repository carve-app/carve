package main

import (
	"path/filepath"
	"testing"
)

func TestValidateMigrationFilesAllowsHistoricalDuplicates(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateMigrationFiles(files); err != nil {
		t.Fatalf("repository migrations should validate: %v", err)
	}
}

func TestValidateMigrationFilesRejectsNewDuplicatePrefix(t *testing.T) {
	err := validateMigrationFiles([]string{"015_one.sql", "015_two.sql"})
	if err == nil {
		t.Fatal("expected duplicate prefix to fail")
	}
}

func TestValidateMigrationFilesRejectsInvalidName(t *testing.T) {
	if err := validateMigrationFiles([]string{"next.sql"}); err == nil {
		t.Fatal("expected invalid filename to fail")
	}
}
