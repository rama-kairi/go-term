package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rama-kairi/go-term/internal/config"
	"github.com/rama-kairi/go-term/internal/database"
	"github.com/rama-kairi/go-term/internal/logger"
	"github.com/rama-kairi/go-term/internal/terminal"
)

// setupTestEnvironment creates a test environment for terminal tools
func setupTestEnvironment(t *testing.T) (*TerminalTools, *terminal.Manager, string) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "go-term-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Create test config using DefaultConfig as base
	cfg := config.DefaultConfig()

	// Override specific settings for testing
	cfg.Database.Path = filepath.Join(tempDir, "test.db")
	cfg.Server.Debug = true
	cfg.Session.MaxSessions = 10
	cfg.Session.MaxCommandsPerSession = 30
	cfg.Session.MaxBackgroundProcesses = 3
	cfg.Session.BackgroundOutputLimit = 2000
	cfg.Session.ResourceCleanupInterval = time.Minute
	cfg.Session.MaxCommandLength = 10000
	cfg.Streaming.Enable = false // Disable streaming for tests
	cfg.Streaming.BufferSize = 4096
	cfg.Streaming.Timeout = 30 * time.Second
	cfg.Security.EnableSandbox = false
	cfg.Security.BlockedCommands = []string{}
	cfg.Security.AllowNetworkAccess = true
	cfg.Security.AllowFileSystemWrite = true

	// Create logger with minimal output for tests
	cfg.Logging.Level = "error"
	testLogger, err := logger.NewLogger(&cfg.Logging, "test")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create database
	db, err := database.NewDB(cfg.Database.Path)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Create terminal manager
	manager := terminal.NewManager(cfg, testLogger, db)

	// Create terminal tools
	tools := NewTerminalTools(manager, cfg, testLogger, db)

	return tools, manager, tempDir
}

// TestCreateSession tests session creation functionality
func TestCreateSession(t *testing.T) {
	tools, _, tempDir := setupTestEnvironment(t)
	defer os.RemoveAll(tempDir)

	ctx := context.Background()

	tests := []struct {
		name        string
		sessionName string
		expectError bool
	}{
		{
			name:        "valid session name",
			sessionName: "test-session",
			expectError: false,
		},
		{
			name:        "session with underscores",
			sessionName: "test_session_123",
			expectError: false,
		},
		{
			name:        "empty session name",
			sessionName: "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := CreateSessionArgs{
				Name: tt.sessionName,
			}

			result, sessionResult, err := tools.CreateSession(ctx, nil, args)

			if tt.expectError {
				if err == nil && (result == nil || !result.IsError) {
					t.Errorf("Expected error for session name '%s' but got none", tt.sessionName)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result != nil && result.IsError {
					t.Errorf("Result indicates error but none expected")
				}
				if sessionResult.SessionID == "" {
					t.Errorf("Expected session ID but got empty string")
				}
				if sessionResult.Name != tt.sessionName {
					t.Errorf("Expected session name '%s', got '%s'", tt.sessionName, sessionResult.Name)
				}
			}
		})
	}
}

// TestRunCommand tests foreground command execution
func TestRunCommand(t *testing.T) {
	tools, _, tempDir := setupTestEnvironment(t)
	defer os.RemoveAll(tempDir)

	ctx := context.Background()

	// Create a test session
	createArgs := CreateSessionArgs{Name: "test-session"}
	_, sessionResult, err := tools.CreateSession(ctx, nil, createArgs)
	if err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	tests := []struct {
		name        string
		command     string
		expectError bool
	}{
		{
			name:        "simple echo command",
			command:     "echo hello",
			expectError: false,
		},
		{
			name:        "empty command",
			command:     "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := RunCommandArgs{
				SessionID: sessionResult.SessionID,
				Command:   tt.command,
			}

			result, cmdResult, err := tools.RunCommand(ctx, nil, args)

			if tt.expectError {
				if err == nil && (result == nil || !result.IsError) {
					t.Errorf("Expected error for command '%s' but got none", tt.command)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result != nil && result.IsError {
					t.Errorf("Result indicates error but none expected")
				}
				if cmdResult.SessionID != sessionResult.SessionID {
					t.Errorf("Expected session ID '%s', got '%s'", sessionResult.SessionID, cmdResult.SessionID)
				}
			}
		})
	}
}

// TestRunBackgroundProcess tests background process execution
func TestRunBackgroundProcess(t *testing.T) {
	tools, _, tempDir := setupTestEnvironment(t)
	defer os.RemoveAll(tempDir)

	ctx := context.Background()

	// Create a test session
	createArgs := CreateSessionArgs{Name: "test-session"}
	_, sessionResult, err := tools.CreateSession(ctx, nil, createArgs)
	if err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	// Test running a background process
	args := RunBackgroundProcessArgs{
		SessionID: sessionResult.SessionID,
		Command:   "sleep 2", // Short sleep for testing
	}

	result, bgResult, err := tools.RunBackgroundProcess(ctx, nil, args)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatalf("Result indicates error")
	}

	if bgResult.SessionID != sessionResult.SessionID {
		t.Errorf("Expected session ID '%s', got '%s'", sessionResult.SessionID, bgResult.SessionID)
	}

	if bgResult.ProcessID == "" {
		t.Errorf("Expected process ID but got empty string")
	}

	if !bgResult.Success {
		t.Errorf("Expected success but got failure")
	}
}

// TestSecurityValidator tests command security validation
func TestSecurityValidator(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Security.EnableSandbox = true
	cfg.Security.BlockedCommands = []string{"rm", "sudo"}

	validator := NewSecurityValidator(cfg)

	tests := []struct {
		name        string
		command     string
		expectError bool
	}{
		{
			name:        "safe command",
			command:     "echo hello",
			expectError: false,
		},
		{
			name:        "blocked command",
			command:     "rm file.txt",
			expectError: true,
		},
		{
			name:        "empty command",
			command:     "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateCommand(tt.command)

			if tt.expectError && err == nil {
				t.Errorf("Expected error for command '%s' but got none", tt.command)
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error for command '%s': %v", tt.command, err)
			}
		})
	}
}
