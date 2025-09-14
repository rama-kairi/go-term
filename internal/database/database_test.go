package database

import (
	"os"
	"path/filepath"
	"testing"
)

// setupTestDB creates a temporary database for testing
func setupTestDB(t *testing.T) (*DB, string) {
	tempDir, err := os.MkdirTemp("", "db-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	return db, tempDir
}

// TestNewDB tests database creation and initialization
func TestNewDB(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer os.RemoveAll(tempDir)
	defer db.Close()

	// Test that database connection is working
	err := db.HealthCheck()
	if err != nil {
		t.Errorf("Database health check failed: %v", err)
	}
}
