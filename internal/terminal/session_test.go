package terminal

import (
	"fmt"
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
	t.Run("BasicBackgroundProcess", func(t *testing.T) {
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
	})

	t.Run("TruncateOutput", func(t *testing.T) {
		bp := &BackgroundProcess{
			ID:      "truncate-test",
			Command: "test",
		}

		// Test large output truncation
		longOutput := strings.Repeat("A", 500)
		bp.Output = longOutput
		bp.TruncateOutput(100)

		if len(bp.Output) > 103 {
			t.Errorf("Expected output to be truncated, got length %d", len(bp.Output))
		}

		if !strings.HasPrefix(bp.Output, "...") {
			t.Error("Expected truncated output to start with '...'")
		}

		// Test large error output truncation
		longErrorOutput := strings.Repeat("E", 500)
		bp.ErrorOutput = longErrorOutput
		bp.TruncateOutput(100)

		if len(bp.ErrorOutput) > 103 {
			t.Errorf("Expected error output to be truncated, got length %d", len(bp.ErrorOutput))
		}

		// Test no truncation needed
		bp.Output = "short"
		bp.ErrorOutput = "short error"
		bp.TruncateOutput(100)

		if bp.Output != "short" {
			t.Error("Expected short output to remain unchanged")
		}

		if bp.ErrorOutput != "short error" {
			t.Error("Expected short error output to remain unchanged")
		}
	})

	t.Run("UpdateOutputWithLimits", func(t *testing.T) {
		bp := &BackgroundProcess{
			ID:      "limit-test",
			Command: "test",
		}

		// Test normal update
		bp.UpdateOutput("first line\n", 0) // no limit
		bp.UpdateOutput("second line\n", 0)

		expected := "first line\nsecond line\n"
		if bp.Output != expected {
			t.Errorf("Expected output '%s', got '%s'", expected, bp.Output)
		}

		// Test with limit
		bp = &BackgroundProcess{ID: "limit-test2", Command: "test"}
		longLine := strings.Repeat("X", 100)
		bp.UpdateOutput(longLine, 50)

		if len(bp.Output) > 53 { // 50 + "..."
			t.Errorf("Expected output to be limited to around 50 chars, got %d", len(bp.Output))
		}

		// Test error output with limits
		bp.UpdateErrorOutput("error line 1\n", 0)
		bp.UpdateErrorOutput("error line 2\n", 0)

		if !strings.Contains(bp.ErrorOutput, "error line 1") {
			t.Error("Expected error output to contain both lines")
		}

		// Test error output limit
		bp = &BackgroundProcess{ID: "limit-test3", Command: "test"}
		longError := strings.Repeat("E", 100)
		bp.UpdateErrorOutput(longError, 50)

		if len(bp.ErrorOutput) > 53 {
			t.Errorf("Expected error output to be limited, got %d", len(bp.ErrorOutput))
		}
	})
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
	t.Run("BasicSessionManagement", func(t *testing.T) {
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
	})

	t.Run("CreateMultipleSessions", func(t *testing.T) {
		_, manager, cleanup := setupTestSession(t)
		defer cleanup()

		// Create additional sessions
		session2, err := manager.CreateSession("session-2", "project_2", "/home")
		if err != nil {
			t.Fatalf("Failed to create second session: %v", err)
		}

		session3, err := manager.CreateSession("session-3", "project_3", "/var")
		if err != nil {
			t.Fatalf("Failed to create third session: %v", err)
		}

		// Test that all sessions exist
		sessions := manager.ListSessions()
		if len(sessions) < 3 {
			t.Errorf("Expected at least 3 sessions, got %d", len(sessions))
		}

		// Test retrieving specific sessions
		retrieved2, err := manager.GetSession(session2.ID)
		if err != nil || retrieved2 == nil {
			t.Error("Failed to retrieve session 2")
		}

		retrieved3, err := manager.GetSession(session3.ID)
		if err != nil || retrieved3 == nil {
			t.Error("Failed to retrieve session 3")
		}

		// Test session properties
		if retrieved2.Name != "session-2" {
			t.Errorf("Expected session name session-2, got %s", retrieved2.Name)
		}

		if retrieved3.ProjectID != "project_3" {
			t.Errorf("Expected project ID project-3, got %s", retrieved3.ProjectID)
		}
	})

	t.Run("SessionInvalidOperations", func(t *testing.T) {
		_, manager, cleanup := setupTestSession(t)
		defer cleanup()

		// Test getting non-existent session
		nonExistent, err := manager.GetSession("non-existent-id")
		if err == nil || nonExistent != nil {
			t.Error("Expected error when getting non-existent session")
		}

		// Test deleting non-existent session
		err = manager.DeleteSession("non-existent-id")
		if err == nil {
			t.Error("Expected error when deleting non-existent session")
		}

		// Test creating session with empty name (should succeed as name validation is not enforced)
		emptyNameSession, err := manager.CreateSession("", "invalid_project", "/tmp")
		if err != nil {
			t.Logf("Session creation with empty name failed as expected: %v", err)
		} else if emptyNameSession != nil {
			t.Log("Session creation with empty name succeeded (no validation enforced)")
			manager.DeleteSession(emptyNameSession.ID)
		}
	})

	t.Run("SessionOperations", func(t *testing.T) {
		session, manager, cleanup := setupTestSession(t)
		defer cleanup()

		// Execute some commands to generate activity
		_, err := manager.ExecuteCommand(session.ID, "echo test1")
		if err != nil {
			t.Errorf("Failed to execute command: %v", err)
		}

		_, err = manager.ExecuteCommand(session.ID, "echo test2")
		if err != nil {
			t.Errorf("Failed to execute command: %v", err)
		}

		// Test SessionExists
		if !manager.SessionExists(session.ID) {
			t.Error("Expected session to exist")
		}

		if manager.SessionExists("non-existent-id") {
			t.Error("Expected non-existent session to not exist")
		}

		// Test CloseSession
		err = manager.CloseSession(session.ID)
		if err != nil {
			t.Errorf("Failed to close session: %v", err)
		}

		// Test DeleteProjectSessions
		session2, err := manager.CreateSession("session-in-project", "same_project", "/tmp")
		if err != nil {
			t.Fatalf("Failed to create session in same project: %v", err)
		}

		session3, err := manager.CreateSession("session-in-project-2", "same_project", "/tmp")
		if err != nil {
			t.Fatalf("Failed to create second session in same project: %v", err)
		}

		deletedSessions, err := manager.DeleteProjectSessions("same_project")
		if err != nil {
			t.Errorf("Failed to delete project sessions: %v", err)
		}

		if len(deletedSessions) < 2 {
			t.Errorf("Expected at least 2 deleted sessions, got %d", len(deletedSessions))
		}

		// Verify sessions were deleted
		if manager.SessionExists(session2.ID) {
			t.Error("Expected project session to be deleted")
		}

		if manager.SessionExists(session3.ID) {
			t.Error("Expected second project session to be deleted")
		}

		// Test GetProjectIDGenerator
		generator := manager.GetProjectIDGenerator()
		if generator == nil {
			t.Error("Expected non-nil project ID generator")
		}
	})
}

// TestCommandExecution tests command execution in sessions
func TestCommandExecution(t *testing.T) {
	t.Run("BasicCommandExecution", func(t *testing.T) {
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
	})

	t.Run("ExecuteCommandWithStreaming", func(t *testing.T) {
		session, manager, cleanup := setupTestSession(t)
		defer cleanup()

		// Test streaming command execution
		output, err := manager.ExecuteCommandWithStreaming(session.ID, "echo streaming test")
		if err != nil {
			t.Errorf("Failed to execute streaming command: %v", err)
		}

		if !strings.Contains(output, "streaming test") {
			t.Errorf("Expected streaming output to contain test text, got: %s", output)
		}
	})

	t.Run("CommandExecutionInvalidSession", func(t *testing.T) {
		_, manager, cleanup := setupTestSession(t)
		defer cleanup()

		// Test command execution with invalid session
		_, err := manager.ExecuteCommand("invalid-session-id", "echo test")
		if err == nil {
			t.Error("Expected error when executing command in invalid session")
		}

		_, err = manager.ExecuteCommandWithStreaming("invalid-session-id", "echo test")
		if err == nil {
			t.Error("Expected error when executing streaming command in invalid session")
		}
	})

	t.Run("DirectoryChangeCommands", func(t *testing.T) {
		session, manager, cleanup := setupTestSession(t)
		defer cleanup()

		// Test directory change detection (private method tested indirectly)
		// This will exercise the isDirectoryChangeCommand method
		originalDir := session.WorkingDir

		// Execute a command that simulates directory change behavior
		_, err := manager.ExecuteCommand(session.ID, "pwd")
		if err != nil {
			t.Errorf("Failed to execute pwd command: %v", err)
		}

		// Verify session still exists after directory commands
		updatedSession, err := manager.GetSession(session.ID)
		if err != nil {
			t.Errorf("Failed to get session after directory command: %v", err)
		}

		// Working directory should still be accessible
		if updatedSession.WorkingDir == "" {
			t.Error("Expected working directory to be maintained")
		}

		t.Logf("Original dir: %s, Updated dir: %s", originalDir, updatedSession.WorkingDir)
	})

	t.Run("MultipleCommandsInSession", func(t *testing.T) {
		session, manager, cleanup := setupTestSession(t)
		defer cleanup()

		commands := []string{
			"echo first command",
			"echo second command",
			"pwd",
			"echo third command",
		}

		for i, cmd := range commands {
			output, err := manager.ExecuteCommand(session.ID, cmd)
			if err != nil {
				t.Errorf("Failed to execute command %d (%s): %v", i+1, cmd, err)
				continue
			}

			t.Logf("Command %d output: %s", i+1, strings.TrimSpace(output))
		}

		// Verify session is still active after multiple commands
		finalSession, err := manager.GetSession(session.ID)
		if err != nil {
			t.Errorf("Failed to get session after multiple commands: %v", err)
		}

		if !finalSession.IsActive {
			t.Error("Expected session to still be active after multiple commands")
		}
	})
}

// TestNewManager tests manager creation
func TestNewManager(t *testing.T) {
	t.Run("BasicManagerCreation", func(t *testing.T) {
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
	})

	t.Run("ManagerWithDifferentConfigs", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "manager-config-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Test with different configurations
		configs := []*config.Config{
			{
				Database: config.DatabaseConfig{
					Path: filepath.Join(tempDir, "test1.db"),
				},
				Session: config.SessionConfig{
					MaxSessions:             5,
					CleanupInterval:         30 * time.Second,
					ResourceCleanupInterval: 30 * time.Second,
					DefaultTimeout:          15 * time.Second,
				},
				Logging: config.LoggingConfig{
					Level:  "debug",
					Format: "json",
					Output: "stderr",
				},
			},
			{
				Database: config.DatabaseConfig{
					Path: filepath.Join(tempDir, "test2.db"),
				},
				Session: config.SessionConfig{
					MaxSessions:             20,
					CleanupInterval:         2 * time.Minute,
					ResourceCleanupInterval: 2 * time.Minute,
					DefaultTimeout:          60 * time.Second,
				},
				Logging: config.LoggingConfig{
					Level:  "error",
					Format: "text",
					Output: "stderr",
				},
			},
		}

		for i, cfg := range configs {
			logger, err := logger.NewLogger(&cfg.Logging, "test")
			if err != nil {
				t.Fatalf("Failed to create logger for config %d: %v", i, err)
			}

			db, err := database.NewDB(cfg.Database.Path)
			if err != nil {
				t.Fatalf("Failed to create database for config %d: %v", i, err)
			}

			manager := NewManager(cfg, logger, db)
			if manager == nil {
				t.Errorf("Expected non-nil manager for config %d", i)
			}

			// Test creating a session with this manager
			session, err := manager.CreateSession(fmt.Sprintf("test-session-%d", i), fmt.Sprintf("project_%d", i), "/tmp")
			if err != nil {
				t.Errorf("Failed to create session with config %d: %v", i, err)
			} else if session == nil {
				t.Errorf("Expected non-nil session for config %d", i)
			}

			db.Close()
		}
	})
}

// TestWorkingDirectoryDetection tests working directory functionality
func TestWorkingDirectoryDetection(t *testing.T) {
	t.Run("CreateSessionWithWorkingDir", func(t *testing.T) {
		_, manager, cleanup := setupTestSession(t)
		defer cleanup()

		// Test creating session with specific working directory
		session, err := manager.CreateSession("dir-test", "project_dir", "/usr/local")
		if err != nil {
			t.Fatalf("Failed to create session with specific dir: %v", err)
		}

		if session.WorkingDir != "/usr/local" {
			t.Errorf("Expected working dir /usr/local, got %s", session.WorkingDir)
		}

		// Test creating session with empty working directory (should use default)
		session2, err := manager.CreateSession("dir-test-2", "project_dir_2", "")
		if err != nil {
			t.Fatalf("Failed to create session with empty dir: %v", err)
		}

		if session2.WorkingDir == "" {
			t.Error("Expected working directory to be set to default")
		}

		t.Logf("Default working dir: %s", session2.WorkingDir)
	})

	t.Run("SessionEnvironment", func(t *testing.T) {
		session, manager, cleanup := setupTestSession(t)
		defer cleanup()

		// Check that session has environment variables
		if session.Environment == nil {
			t.Error("Expected session to have environment variables")
		}

		// Test that environment is preserved
		retrievedSession, err := manager.GetSession(session.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve session: %v", err)
		}

		if retrievedSession.Environment == nil {
			t.Error("Expected retrieved session to have environment variables")
		}

		// Environment should have at least some basic variables
		t.Logf("Session environment has %d variables", len(retrievedSession.Environment))
	})
}
