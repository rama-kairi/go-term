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
	cfg.Streaming.Enable = true
	cfg.Streaming.BufferSize = 4096
	cfg.Streaming.Timeout = 30 * time.Second
	cfg.Security.EnableSandbox = false
	cfg.Security.BlockedCommands = []string{}
	cfg.Security.AllowNetworkAccess = true
	cfg.Security.AllowFileSystemWrite = true

	// Create logger
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

// TestBackgroundProcessDetection tests if background detection patterns work
func TestBackgroundProcessDetection(t *testing.T) {
	tools, _, tempDir := setupTestEnvironment(t)
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name         string
		command      string
		shouldDetect bool
		description  string
	}{
		{
			name:         "ping_should_be_background",
			command:      "ping google.com",
			shouldDetect: true,
			description:  "Ping commands should be detected as background",
		},
		{
			name:         "sleep_long_should_be_background",
			command:      "sleep 60",
			shouldDetect: true,
			description:  "Long sleep commands should be detected as background",
		},
		{
			name:         "python_background_test",
			command:      "python3 background_test.py",
			shouldDetect: true,
			description:  "Python background test script should be detected",
		},
		{
			name:         "node_setinterval",
			command:      "node -e \"setInterval(() => console.log('test'), 1000)\"",
			shouldDetect: true,
			description:  "Node.js setInterval should be detected as background",
		},
		{
			name:         "echo_should_not_be_background",
			command:      "echo 'hello'",
			shouldDetect: false,
			description:  "Simple echo should not be background",
		},
		{
			name:         "ls_should_not_be_background",
			command:      "ls -la",
			shouldDetect: false,
			description:  "Directory listing should not be background",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detected := tools.shouldAutoDetectBackground(tt.command)
			if detected != tt.shouldDetect {
				t.Errorf("%s: expected %v, got %v for command: %s",
					tt.description, tt.shouldDetect, detected, tt.command)
			}
		})
	}
}

// TestBackgroundProcessExecution tests actual background process execution
func TestBackgroundProcessExecution(t *testing.T) {
	tools, manager, tempDir := setupTestEnvironment(t)
	defer os.RemoveAll(tempDir)

	// Create a test session
	session, err := manager.CreateSession("test-session", "", "")
	if err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	tests := []struct {
		name         string
		command      string
		isBackground bool
		expectError  bool
		description  string
	}{
		{
			name:         "force_background_sleep",
			command:      "sleep 5",
			isBackground: true,
			expectError:  false,
			description:  "Force background execution of sleep should work",
		},
		{
			name:         "auto_detect_ping",
			command:      "ping -c 3 127.0.0.1",
			isBackground: false, // Let auto-detection handle it
			expectError:  false,
			description:  "Auto-detection should make ping background",
		},
		{
			name:         "foreground_echo",
			command:      "echo 'test output'",
			isBackground: false,
			expectError:  false,
			description:  "Simple echo should run in foreground",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			args := RunCommandArgs{
				SessionID:    session.ID,
				Command:      tt.command,
				IsBackground: tt.isBackground,
			}

			// Set a timeout for the test to prevent hanging
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			// Run the command
			start := time.Now()
			result, cmdResult, err := tools.RunCommand(ctx, nil, args)
			duration := time.Since(start)

			// Check for errors
			if tt.expectError && err == nil {
				t.Errorf("%s: expected error but got none", tt.description)
			}
			if !tt.expectError && err != nil {
				t.Errorf("%s: unexpected error: %v", tt.description, err)
			}

			// Check if result indicates error
			if result != nil && result.IsError && !tt.expectError {
				t.Errorf("%s: result indicates error but none expected", tt.description)
			}

			// Check execution time - background commands should return quickly
			shouldRunInBackground := tt.isBackground || tools.shouldAutoDetectBackground(tt.command)
			if shouldRunInBackground && duration > 2*time.Second {
				t.Errorf("%s: background command took too long: %v", tt.description, duration)
			}

			// Log results for debugging
			t.Logf("%s: duration=%v, isBackground=%v, success=%v",
				tt.description, duration, cmdResult.IsBackground, cmdResult.Success)
		})
	}
}

// TestBackgroundProcessMonitoring tests the check_background_process functionality
func TestBackgroundProcessMonitoring(t *testing.T) {
	tools, manager, tempDir := setupTestEnvironment(t)
	defer os.RemoveAll(tempDir)

	// Create a test session
	session, err := manager.CreateSession("monitor-test", "", "")
	if err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	// Start a background process
	ctx := context.Background()
	args := RunCommandArgs{
		SessionID:    session.ID,
		Command:      "sleep 30",
		IsBackground: true, // Force background
	}

	_, cmdResult, err := tools.RunCommand(ctx, nil, args)
	if err != nil {
		t.Fatalf("Failed to start background process: %v", err)
	}

	if !cmdResult.IsBackground {
		t.Fatalf("Command should have run in background but didn't")
	}

	// Wait a moment for the process to start
	time.Sleep(100 * time.Millisecond)

	// Now check the background process
	checkArgs := CheckBackgroundProcessArgs{
		SessionID: session.ID,
	}

	_, checkResult, err := tools.CheckBackgroundProcess(ctx, nil, checkArgs)
	if err != nil {
		t.Fatalf("Failed to check background process: %v", err)
	}

	// Verify the check result
	if checkResult.SessionID != session.ID {
		t.Errorf("Expected session ID %s, got %s", session.ID, checkResult.SessionID)
	}

	t.Logf("Background process check result: running=%v, command=%s",
		checkResult.IsRunning, checkResult.Command)
}

// TestSimpleBackgroundExecution tests the most basic case
func TestSimpleBackgroundExecution(t *testing.T) {
	tools, manager, tempDir := setupTestEnvironment(t)
	defer os.RemoveAll(tempDir)

	// Create a test session
	session, err := manager.CreateSession("simple-test", "", "")
	if err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	// Test the simplest possible background command
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	args := RunCommandArgs{
		SessionID:    session.ID,
		Command:      "sleep 10",
		IsBackground: true,
	}

	start := time.Now()
	result, cmdResult, err := tools.RunCommand(ctx, nil, args)
	duration := time.Since(start)

	// This should complete quickly since it's background
	if duration > 2*time.Second {
		t.Errorf("Background command took too long: %v", duration)
	}

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if result.IsError {
		t.Errorf("Result indicates error: %+v", result)
	}

	if !cmdResult.IsBackground {
		t.Errorf("Command should have run in background")
	}

	t.Logf("Simple background test: duration=%v, success=%v, isBackground=%v",
		duration, cmdResult.Success, cmdResult.IsBackground)
}
