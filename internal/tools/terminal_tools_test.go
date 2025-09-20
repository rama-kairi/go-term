package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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

// TestCreateSessionWithWorkingDir tests creating sessions with working directory parameter
func TestCreateSessionWithWorkingDir(t *testing.T) {
	tools, manager, tempDir := setupTestEnvironment(t)
	defer os.RemoveAll(tempDir)
	defer manager.Shutdown()

	ctx := context.Background()

	// Test creating a session with working directory
	args := CreateSessionArgs{
		Name:       "test-workdir-session",
		WorkingDir: "/tmp",
	}

	result, createResult, err := tools.CreateSession(ctx, nil, args)
	if err != nil {
		t.Fatalf("Failed to create session with working dir: %v", err)
	}

	if result.IsError {
		t.Fatalf("CreateSession returned error: %s", string(result.Content[0].(*mcp.TextContent).Text))
	}

	// Verify the working directory was set correctly
	if createResult.WorkingDir != "/tmp" {
		t.Errorf("Expected working dir '/tmp', got '%s'", createResult.WorkingDir)
	}

	// Test creating a session with custom project ID
	args2 := CreateSessionArgs{
		Name:      "test-project-session",
		ProjectID: "custom_project_123",
	}

	result2, createResult2, err := tools.CreateSession(ctx, nil, args2)
	if err != nil {
		t.Fatalf("Failed to create session with project ID: %v", err)
	}

	if result2.IsError {
		t.Fatalf("CreateSession returned error: %s", string(result2.Content[0].(*mcp.TextContent).Text))
	}

	// Verify the project ID was set correctly
	if createResult2.ProjectID != "custom_project_123" {
		t.Errorf("Expected project ID 'custom_project_123', got '%s'", createResult2.ProjectID)
	}
}

// TestRunCommandTimeout tests the timeout functionality for run_command
func TestRunCommandTimeout(t *testing.T) {
	tools, manager, tempDir := setupTestEnvironment(t)
	defer func() {
		manager.Shutdown()
		os.RemoveAll(tempDir)
	}()

	ctx := context.Background()

	// Create a test session
	createArgs := CreateSessionArgs{Name: "timeout-test-session"}
	_, createResult, err := tools.CreateSession(ctx, nil, createArgs)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Test 1: Command that should complete within timeout
	t.Run("CommandWithinTimeout", func(t *testing.T) {
		args := RunCommandArgs{
			SessionID: createResult.SessionID,
			Command:   "echo 'quick command'",
			Timeout:   5, // 5 seconds should be plenty
		}

		result, runResult, err := tools.RunCommand(ctx, nil, args)
		if err != nil {
			t.Fatalf("Failed to run command: %v", err)
		}

		if result.IsError {
			t.Fatalf("RunCommand returned error: %s", string(result.Content[0].(*mcp.TextContent).Text))
		}

		if !runResult.Success {
			t.Errorf("Expected command to succeed, but it failed: %s", runResult.ErrorOutput)
		}

		if runResult.TimedOut {
			t.Errorf("Expected command not to timeout, but it did")
		}

		if runResult.TimeoutUsed != 5 {
			t.Errorf("Expected timeout used to be 5, got %d", runResult.TimeoutUsed)
		}
	})

	// Test 2: Command that should timeout
	t.Run("CommandTimeout", func(t *testing.T) {
		args := RunCommandArgs{
			SessionID: createResult.SessionID,
			Command:   "sleep 10", // This should timeout with 2 second limit
			Timeout:   2,          // 2 seconds timeout
		}

		result, runResult, err := tools.RunCommand(ctx, nil, args)
		if err != nil {
			t.Fatalf("Failed to run command: %v", err)
		}

		if result.IsError {
			t.Fatalf("RunCommand returned error: %s", string(result.Content[0].(*mcp.TextContent).Text))
		}

		if runResult.Success {
			t.Errorf("Expected command to fail due to timeout, but it succeeded")
		}

		if !runResult.TimedOut {
			t.Errorf("Expected command to timeout, but it didn't")
		}

		if runResult.TimeoutUsed != 2 {
			t.Errorf("Expected timeout used to be 2, got %d", runResult.TimeoutUsed)
		}

		if runResult.ExitCode != 124 { // Standard timeout exit code
			t.Errorf("Expected exit code 124 for timeout, got %d", runResult.ExitCode)
		}
	})

	// Test 3: Default timeout (should be 60 seconds)
	t.Run("DefaultTimeout", func(t *testing.T) {
		args := RunCommandArgs{
			SessionID: createResult.SessionID,
			Command:   "echo 'default timeout test'",
			Timeout:   0, // Use default
		}

		result, runResult, err := tools.RunCommand(ctx, nil, args)
		if err != nil {
			t.Fatalf("Failed to run command: %v", err)
		}

		if result.IsError {
			t.Fatalf("RunCommand returned error: %s", string(result.Content[0].(*mcp.TextContent).Text))
		}

		if runResult.TimeoutUsed != 60 {
			t.Errorf("Expected default timeout to be 60, got %d", runResult.TimeoutUsed)
		}
	})

	// Test 4: Maximum timeout limit (should be capped at 300 seconds)
	t.Run("MaximumTimeoutLimit", func(t *testing.T) {
		args := RunCommandArgs{
			SessionID: createResult.SessionID,
			Command:   "echo 'max timeout test'",
			Timeout:   500, // Request 500 seconds, should be capped at 300
		}

		result, runResult, err := tools.RunCommand(ctx, nil, args)
		if err != nil {
			t.Fatalf("Failed to run command: %v", err)
		}

		if result.IsError {
			t.Fatalf("RunCommand returned error: %s", string(result.Content[0].(*mcp.TextContent).Text))
		}

		if runResult.TimeoutUsed != 300 {
			t.Errorf("Expected timeout to be capped at 300, got %d", runResult.TimeoutUsed)
		}
	})
}
