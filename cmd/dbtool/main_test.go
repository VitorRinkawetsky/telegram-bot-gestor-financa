package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckDatabasePath(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.db")
	err := checkDatabasePath(missing)
	if !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), "DATABASE_PATH") {
		t.Fatalf("missing database should explain path mismatch: %v", err)
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("path validation created a database")
	}
	if err := checkDatabasePath(dir); err == nil || !strings.Contains(err.Error(), "arquivo SQLite regular") {
		t.Fatalf("directory accepted: %v", err)
	}
	if err := checkDatabasePath(""); err == nil {
		t.Fatal("empty path accepted")
	}
	existing := filepath.Join(dir, "existing.db")
	if err := os.WriteFile(existing, []byte("existing file"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkDatabasePath(existing); err != nil {
		t.Fatalf("existing regular file rejected: %v", err)
	}
	content, err := os.ReadFile(existing)
	if err != nil || string(content) != "existing file" {
		t.Fatal("path validation altered the existing file")
	}
}
