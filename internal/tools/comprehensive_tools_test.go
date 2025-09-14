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

// setupTestToolsEnvironment creates a comprehensive test environment
func setupTestToolsEnvironment(t *testing.T) (*TerminalTools, *terminal.Manager, string) {
	tempDir, err := os.MkdirTemp("", "tools_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Database.Path = filepath.Join(tempDir, "test.db")
	cfg.Logging.Level = "error" // Reduce noise in tests

	testLogger, err := logger.NewLogger(&cfg.Logging, "test")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	db, err := database.NewDB(cfg.Database.Path)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	manager := terminal.NewManager(cfg, testLogger, db)
	tools := NewTerminalTools(manager, cfg, testLogger, db)

	return tools, manager, tempDir
}

func TestCreateSessionTool(t *testing.T) {
	tools, _, tempDir := setupTestToolsEnvironment(t)
	defer os.RemoveAll(tempDir)

	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	args := CreateSessionArgs{
		Name: "test-session",
	}

	result, response, err := tools.CreateSession(ctx, req, args)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if result.IsError {
		t.Errorf("Expected success, got error: %v", result.Content)
	}

	if response.SessionID == "" {
		t.Error("Expected session ID to be set")
	}

	if response.Name != "test-session" {
		t.Errorf("Expected name 'test-session', got %s", response.Name)
	}

	if response.ProjectID == "" {
		t.Error("Expected project ID to be set")
	}
}

func TestRunCommandTool(t *testing.T) {
	tools, manager, tempDir := setupTestToolsEnvironment(t)
	defer os.RemoveAll(tempDir)

	// Create a test session first
	session, err := manager.CreateSession("cmd-test", "", "")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	args := RunCommandArgs{
		SessionID: session.ID,
		Command:   "echo 'test command'",
	}

	result, response, err := tools.RunCommand(ctx, req, args)
	if err != nil {
		t.Fatalf("RunCommand failed: %v", err)
	}

	if result.IsError {
		t.Errorf("Expected success, got error: %v", result.Content)
	}

	if !response.Success {
		t.Errorf("Expected command to succeed")
	}

	if response.SessionID != session.ID {
		t.Errorf("Expected session ID %s, got %s", session.ID, response.SessionID)
	}
}

func TestRunBackgroundProcessTool(t *testing.T) {
	tools, manager, tempDir := setupTestToolsEnvironment(t)
	defer os.RemoveAll(tempDir)

	// Create a test session first
	session, err := manager.CreateSession("bg-test", "", "")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	args := RunBackgroundProcessArgs{
		SessionID: session.ID,
		Command:   "sleep 1",
	}

	result, response, err := tools.RunBackgroundProcess(ctx, req, args)
	if err != nil {
		t.Fatalf("RunBackgroundProcess failed: %v", err)
	}

	if result.IsError {
		t.Errorf("Expected success, got error: %v", result.Content)
	}

	if response.ProcessID == "" {
		t.Error("Expected process ID to be set")
	}

	if response.SessionID != session.ID {
		t.Errorf("Expected session ID %s, got %s", session.ID, response.SessionID)
	}

	// Wait a bit for the background process to finish
	time.Sleep(1500 * time.Millisecond)
}

func TestCheckBackgroundProcessTool(t *testing.T) {
	tools, manager, tempDir := setupTestToolsEnvironment(t)
	defer os.RemoveAll(tempDir)

	// Create a test session first
	session, err := manager.CreateSession("check-bg-test", "", "")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Start a background process
	processID, err := manager.ExecuteCommandInBackground(session.ID, "echo 'background test'")
	if err != nil {
		t.Fatalf("Failed to start background process: %v", err)
	}

	// Wait a bit for the process to complete
	time.Sleep(500 * time.Millisecond)

	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	args := CheckBackgroundProcessArgs{
		SessionID: session.ID,
		ProcessID: processID,
	}

	result, response, err := tools.CheckBackgroundProcess(ctx, req, args)
	if err != nil {
		t.Fatalf("CheckBackgroundProcess failed: %v", err)
	}

	if result.IsError {
		t.Errorf("Expected success, got error: %v", result.Content)
	}

	if response.SessionID != session.ID {
		t.Errorf("Expected session ID %s, got %s", session.ID, response.SessionID)
	}

	if response.ProcessID != processID {
		t.Errorf("Expected process ID %s, got %s", processID, response.ProcessID)
	}
}

func TestListBackgroundProcessesTool(t *testing.T) {
	tools, manager, tempDir := setupTestToolsEnvironment(t)
	defer os.RemoveAll(tempDir)

	// Create a test session first
	session, err := manager.CreateSession("list-bg-test", "", "")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	args := ListBackgroundProcessesArgs{
		SessionID: session.ID,
	}

	result, response, err := tools.ListBackgroundProcesses(ctx, req, args)
	if err != nil {
		t.Fatalf("ListBackgroundProcesses failed: %v", err)
	}

	if result.IsError {
		t.Errorf("Expected success, got error: %v", result.Content)
	}

	// Should have no background processes initially
	if len(response.Processes) == 0 {
		t.Log("No background processes found (expected for initial test)")
	}
}

func TestListSessionsTool(t *testing.T) {
	tools, manager, tempDir := setupTestToolsEnvironment(t)
	defer os.RemoveAll(tempDir)

	// Create test sessions
	session1, err := manager.CreateSession("list-test-1", "", "")
	if err != nil {
		t.Fatalf("Failed to create session 1: %v", err)
	}

	session2, err := manager.CreateSession("list-test-2", "", "")
	if err != nil {
		t.Fatalf("Failed to create session 2: %v", err)
	}

	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	args := ListSessionsArgs{} // No filters

	result, response, err := tools.ListSessions(ctx, req, args)
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}

	if result.IsError {
		t.Errorf("Expected success, got error: %v", result.Content)
	}

	if len(response.Sessions) < 2 {
		t.Errorf("Expected at least 2 sessions, got %d", len(response.Sessions))
	}

	// Check that our sessions are in the list
	foundSession1 := false
	foundSession2 := false
	for _, s := range response.Sessions {
		if s.ID == session1.ID {
			foundSession1 = true
		}
		if s.ID == session2.ID {
			foundSession2 = true
		}
	}

	if !foundSession1 {
		t.Error("Expected to find session 1 in list")
	}

	if !foundSession2 {
		t.Error("Expected to find session 2 in list")
	}
}

func TestDeleteSessionTool(t *testing.T) {
	tools, manager, tempDir := setupTestToolsEnvironment(t)
	defer os.RemoveAll(tempDir)

	// Create a test session
	session, err := manager.CreateSession("delete-test", "", "")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	args := DeleteSessionArgs{
		SessionID: session.ID,
		Confirm:   true,
	}

	result, response, err := tools.DeleteSession(ctx, req, args)
	if err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}

	if result.IsError {
		t.Errorf("Expected success, got error: %v", result.Content)
	}

	if response.SessionID != session.ID {
		t.Errorf("Expected session ID %s, got %s", session.ID, response.SessionID)
	}

	if !response.Success {
		t.Error("Expected deletion to succeed")
	}

	// Verify session is actually deleted
	_, err = manager.GetSession(session.ID)
	if err == nil {
		t.Error("Expected session to be deleted")
	}
}

func TestSearchHistoryTool(t *testing.T) {
	tools, manager, tempDir := setupTestToolsEnvironment(t)
	defer os.RemoveAll(tempDir)

	// Create a test session and run some commands
	session, err := manager.CreateSession("history-test", "", "")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Execute some commands to create history
	_, err = manager.ExecuteCommand(session.ID, "echo 'first command'")
	if err != nil {
		t.Fatalf("Failed to execute first command: %v", err)
	}

	_, err = manager.ExecuteCommand(session.ID, "echo 'second command'")
	if err != nil {
		t.Fatalf("Failed to execute second command: %v", err)
	}

	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	args := SearchHistoryArgs{
		SessionID: session.ID,
		Command:   "echo",
		Limit:     10,
	}

	result, response, err := tools.SearchHistory(ctx, req, args)
	if err != nil {
		t.Fatalf("SearchHistory failed: %v", err)
	}

	if result.IsError {
		t.Errorf("Expected success, got error: %v", result.Content)
	}

	if len(response.Results) == 0 {
		t.Error("Expected to find commands in history")
	}
}

func TestTerminateBackgroundProcessTool(t *testing.T) {
	tools, manager, tempDir := setupTestToolsEnvironment(t)
	defer os.RemoveAll(tempDir)

	// Create a test session
	session, err := manager.CreateSession("terminate-test", "", "")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Start a long-running background process
	processID, err := manager.ExecuteCommandInBackground(session.ID, "sleep 30")
	if err != nil {
		t.Fatalf("Failed to start background process: %v", err)
	}

	// Wait a bit for the process to start
	time.Sleep(100 * time.Millisecond)

	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	args := TerminateBackgroundProcessArgs{
		SessionID: session.ID,
		ProcessID: processID,
		Force:     false,
	}

	result, response, err := tools.TerminateBackgroundProcess(ctx, req, args)
	if err != nil {
		t.Fatalf("TerminateBackgroundProcess failed: %v", err)
	}

	if result.IsError {
		t.Errorf("Expected success, got error: %v", result.Content)
	}

	if response.ProcessID != processID {
		t.Errorf("Expected process ID %s, got %s", processID, response.ProcessID)
	}

	if !response.Terminated {
		t.Error("Expected termination to succeed")
	}
}

func TestValidateHelpers(t *testing.T) {
	// Test validateSessionName
	tests := []struct {
		name        string
		sessionName string
		expectError bool
	}{
		{"valid name", "test-session", false},
		{"valid with underscores", "test_session_123", false},
		{"empty name", "", true},
		{"too short", "ab", false},
		{"too long", string(make([]byte, 200)), true},
		{"invalid characters", "test session!", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSessionName(tt.sessionName)
			if tt.expectError && err == nil {
				t.Errorf("Expected error for session name '%s'", tt.sessionName)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error for session name '%s', got: %v", tt.sessionName, err)
			}
		})
	}

	// Test validateSessionID
	validUUID := "123e4567-e89b-12d3-a456-426614174000"
	invalidUUID := "not-a-uuid"

	if err := validateSessionID(validUUID); err != nil {
		t.Errorf("Expected valid UUID to pass validation, got: %v", err)
	}

	if err := validateSessionID(invalidUUID); err == nil {
		t.Error("Expected invalid UUID to fail validation")
	}
}

func TestCreateJSONResult(t *testing.T) {
	testData := map[string]interface{}{
		"test": "value",
		"num":  42,
	}

	result := createJSONResult(testData)

	if result.IsError {
		t.Error("Expected result to not be an error")
	}

	if len(result.Content) != 1 {
		t.Errorf("Expected 1 content item, got %d", len(result.Content))
	}
}

func TestCreateErrorResult(t *testing.T) {
	errorMsg := "test error message"
	result := createErrorResult(errorMsg)

	if !result.IsError {
		t.Error("Expected result to be an error")
	}

	if len(result.Content) != 1 {
		t.Errorf("Expected 1 content item, got %d", len(result.Content))
	}

	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Error("Expected text content")
	}

	if textContent.Text != "Error: "+errorMsg {
		t.Errorf("Expected error message 'Error: %s', got '%s'", errorMsg, textContent.Text)
	}
}
