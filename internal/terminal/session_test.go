package terminal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rama-kairi/go-term/internal/config"
	"github.com/rama-kairi/go-term/internal/database"
	"github.com/rama-kairi/go-term/internal/logger"
)

// TestBackgroundProcess tests background process functionality
func TestBackgroundProcess(t *testing.T) {
	bp := &BackgroundProcess{
		ID:        "test-bg-process",
		Command:   "echo hello",
		IsRunning: true,
		StartTime: time.Now(),
	}

	// Test output update
	bp.UpdateOutput("line 1\n", 1000)
	bp.UpdateOutput("line 2\n", 1000)

	if !strings.Contains(bp.Output, "line 1") {
		t.Errorf("Expected output to contain line 1")
	}

	if !strings.Contains(bp.Output, "line 2") {
		t.Errorf("Expected output to contain line 2")
	}

	// Test error output update
	bp.UpdateErrorOutput("error 1\n", 1000)
	if !strings.Contains(bp.ErrorOutput, "error 1") {
		t.Errorf("Expected error output to contain error 1")
	}

	// Test output truncation
	longOutput := strings.Repeat("x", 1000)
	bp.UpdateOutput(longOutput, 100)

	if len(bp.Output) > 103 { // 100 + "..." prefix
		t.Errorf("Expected output to be truncated to around 100 characters, got %d", len(bp.Output))
	}
}

// setupTestSession creates a test session for testing
func setupTestSession(t *testing.T) (*Session, *Manager, func()) {
	// Create temp directory for test database
	tempDir, err := os.MkdirTemp("", "terminal-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Create test config
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			Path: filepath.Join(tempDir, "test.db"),
		},
		Session: config.SessionConfig{
			MaxSessions:             10,
			CleanupInterval:         time.Minute,
			ResourceCleanupInterval: time.Minute,
			DefaultTimeout:          30 * time.Second,
		},
		Security: config.SecurityConfig{
			AllowedCommands: []string{},
			BlockedCommands: []string{},
		},
		Logging: config.LoggingConfig{
			Level:  "debug",
			Format: "text",
			Output: "stderr",
		},
	}

	// Create components
	logger, err := logger.NewLogger(&cfg.Logging, "test")
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to create logger: %v", err)
	}

	db, err := database.NewDB(cfg.Database.Path)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to create database: %v", err)
	}

	manager := NewManager(cfg, logger, db)

	// Create test session
	session, err := manager.CreateSession("test-session", "test_project", "/tmp")
	if err != nil {
		db.Close()
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to create session: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.RemoveAll(tempDir)
	}

	return session, manager, cleanup
}

// TestSession tests basic session functionality
func TestSession(t *testing.T) {
	session, manager, cleanup := setupTestSession(t)
	defer cleanup()

	// Test session properties
	if session.Name != "test-session" {
		t.Errorf("Expected session name test-session, got %s", session.Name)
	}

	if session.ProjectID != "test_project" {
		t.Errorf("Expected project ID test_project, got %s", session.ProjectID)
	}

	if session.WorkingDir != "/tmp" {
		t.Errorf("Expected working dir /tmp, got %s", session.WorkingDir)
	}

	if !session.IsActive {
		t.Errorf("Expected session to be active")
	}

	// Test session retrieval
	retrieved, err := manager.GetSession(session.ID)
	if err != nil {
		t.Fatalf("Failed to retrieve session: %v", err)
	}

	if retrieved == nil {
		t.Fatalf("Expected to retrieve created session")
	}

	if retrieved.ID != session.ID {
		t.Errorf("Expected session ID %s, got %s", session.ID, retrieved.ID)
	}
}

// TestSessionManager tests session manager functionality
func TestSessionManager(t *testing.T) {
	session, manager, cleanup := setupTestSession(t)
	defer cleanup()

	// Test listing sessions
	sessions := manager.ListSessions()
	if len(sessions) == 0 {
		t.Errorf("Expected at least one session")
	}

	found := false
	for _, s := range sessions {
		if s.ID == session.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected to find created session in list")
	}

	// Test session deletion
	err := manager.DeleteSession(session.ID)
	if err != nil {
		t.Errorf("Failed to delete session: %v", err)
	}

	// Verify deletion
	retrieved, err := manager.GetSession(session.ID)
	if err == nil && retrieved != nil {
		t.Errorf("Expected session to be deleted")
	}
}

// TestCommandExecution tests command execution in sessions
func TestCommandExecution(t *testing.T) {
	session, manager, cleanup := setupTestSession(t)
	defer cleanup()

	// Test simple command execution
	output, err := manager.ExecuteCommand(session.ID, "echo hello world")
	if err != nil {
		t.Errorf("Failed to execute command: %v", err)
	}

	if !strings.Contains(output, "hello world") {
		t.Errorf("Expected output to contain hello world, got: %s", output)
	}

	// Basic test that session still exists and is accessible
	_, err = manager.GetSession(session.ID)
	if err != nil {
		t.Errorf("Failed to get session after command execution: %v", err)
	}
}

// TestNewManager tests manager creation
func TestNewManager(t *testing.T) {
	// Create temp directory for test database
	tempDir, err := os.MkdirTemp("", "manager-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test config
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			Path: filepath.Join(tempDir, "test.db"),
		},
		Session: config.SessionConfig{
			MaxSessions:             10,
			CleanupInterval:         time.Minute,
			ResourceCleanupInterval: time.Minute,
			DefaultTimeout:          30 * time.Second,
		},
		Logging: config.LoggingConfig{
			Level:  "info",
			Format: "text",
			Output: "stderr",
		},
	}

	// Create components
	logger, err := logger.NewLogger(&cfg.Logging, "test")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	db, err := database.NewDB(cfg.Database.Path)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	manager := NewManager(cfg, logger, db)

	if manager == nil {
		t.Fatalf("Expected non-nil manager")
	}

	// Test that initial sessions list is empty
	sessions := manager.ListSessions()
	if len(sessions) != 0 {
		t.Errorf("Expected empty sessions list, got %d sessions", len(sessions))
	}
}
