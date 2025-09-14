package history

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestNewHistoryManager tests creation of history manager
func TestNewHistoryManager(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "history-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	hm := NewHistoryManager(tempDir)

	if hm == nil {
		t.Fatalf("Expected non-nil history manager")
	}

	// Check that history directory was created
	historyDir := filepath.Join(tempDir, "history")
	if _, err := os.Stat(historyDir); os.IsNotExist(err) {
		t.Errorf("Expected history directory to be created")
	}
}

// TestSessionHistory tests session history functionality
func TestSessionHistory(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "history-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	hm := NewHistoryManager(tempDir)

	// Create session history
	sessionID := "test-session"
	projectID := "test-project"
	sessionName := "Test Session"

	err = hm.CreateSessionHistory(sessionID, projectID, sessionName)
	if err != nil {
		t.Errorf("Failed to create session history: %v", err)
	}

	// Try to create duplicate - should fail
	err = hm.CreateSessionHistory(sessionID, projectID, sessionName)
	if err == nil {
		t.Errorf("Expected error when creating duplicate session history")
	}

	// Add command to history
	cmd := CommandEntry{
		ID:         "cmd-1",
		SessionID:  sessionID,
		ProjectID:  projectID,
		Command:    "echo hello",
		Output:     "hello\\n",
		Success:    true,
		StartTime:  time.Now(),
		EndTime:    time.Now().Add(time.Second),
		Duration:   time.Second,
		WorkingDir: "/tmp",
	}

	err = hm.AddCommand(cmd)
	if err != nil {
		t.Errorf("Failed to add command: %v", err)
	}

	// Get session history
	history, err := hm.GetSessionHistory(sessionID)
	if err != nil {
		t.Errorf("Failed to get session history: %v", err)
	}

	if len(history.Commands) != 1 {
		t.Errorf("Expected 1 command in history, got %d", len(history.Commands))
	}

	if history.Commands[0].Command != "echo hello" {
		t.Errorf("Expected command \"echo hello\", got %s", history.Commands[0].Command)
	}
}

// TestHistorySearch tests history search functionality
func TestHistorySearch(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "history-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	hm := NewHistoryManager(tempDir)

	// Create session and add commands
	sessionID := "search-session"
	projectID := "search-project"

	err = hm.CreateSessionHistory(sessionID, projectID, "Search Test")
	if err != nil {
		t.Errorf("Failed to create session history: %v", err)
	}

	// Add multiple commands
	commands := []CommandEntry{
		{
			ID:        "cmd-1",
			SessionID: sessionID,
			ProjectID: projectID,
			Command:   "echo hello",
			Output:    "hello\\n",
			Success:   true,
			StartTime: time.Now().Add(-time.Hour),
			EndTime:   time.Now().Add(-time.Hour).Add(time.Second),
			Duration:  time.Second,
		},
		{
			ID:        "cmd-2",
			SessionID: sessionID,
			ProjectID: projectID,
			Command:   "ls -la",
			Output:    "total 0\\n",
			Success:   true,
			StartTime: time.Now().Add(-time.Minute),
			EndTime:   time.Now().Add(-time.Minute).Add(500 * time.Millisecond),
			Duration:  500 * time.Millisecond,
		},
		{
			ID:        "cmd-3",
			SessionID: sessionID,
			ProjectID: projectID,
			Command:   "invalid_command",
			Output:    "",
			Success:   false,
			StartTime: time.Now(),
			EndTime:   time.Now().Add(100 * time.Millisecond),
			Duration:  100 * time.Millisecond,
		},
	}

	for _, cmd := range commands {
		err = hm.AddCommand(cmd)
		if err != nil {
			t.Errorf("Failed to add command: %v", err)
		}
	}

	// Test search by command
	opts := SearchOptions{
		Command: "echo",
		Limit:   10,
	}

	results, err := hm.SearchHistory(opts)
	if err != nil {
		t.Errorf("Failed to search: %v", err)
	}

	if len(results.Results) != 1 {
		t.Errorf("Expected 1 result for echo command, got %d", len(results.Results))
	}

	// Test search by success status
	success := true
	opts = SearchOptions{
		Success: &success,
		Limit:   10,
	}

	results, err = hm.SearchHistory(opts)
	if err != nil {
		t.Errorf("Failed to search by success: %v", err)
	}

	if len(results.Results) != 2 {
		t.Errorf("Expected 2 successful commands, got %d", len(results.Results))
	}

	// Test search by project
	opts = SearchOptions{
		ProjectID: projectID,
		Limit:     10,
	}

	results, err = hm.SearchHistory(opts)
	if err != nil {
		t.Errorf("Failed to search by project: %v", err)
	}

	if len(results.Results) != 3 {
		t.Errorf("Expected 3 commands for project, got %d", len(results.Results))
	}
}

// TestCommandStats tests command statistics functionality
func TestCommandStats(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "history-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	hm := NewHistoryManager(tempDir)

	// Create session and add commands
	sessionID := "stats-session"
	projectID := "stats-project"

	err = hm.CreateSessionHistory(sessionID, projectID, "Stats Test")
	if err != nil {
		t.Errorf("Failed to create session history: %v", err)
	}

	// Add commands for statistics
	for i := 0; i < 5; i++ {
		cmd := CommandEntry{
			ID:        fmt.Sprintf("cmd-%d", i),
			SessionID: sessionID,
			ProjectID: projectID,
			Command:   fmt.Sprintf("echo test%d", i),
			Success:   i%2 == 0, // Every other command succeeds
			StartTime: time.Now().Add(-time.Duration(i) * time.Minute),
			EndTime:   time.Now().Add(-time.Duration(i) * time.Minute).Add(time.Second),
			Duration:  time.Second,
		}

		err = hm.AddCommand(cmd)
		if err != nil {
			t.Errorf("Failed to add command: %v", err)
		}
	}

	// Get overall stats
	stats := hm.GetStats()

	if stats.TotalCommands < 5 {
		t.Errorf("Expected at least 5 total commands, got %d", stats.TotalCommands)
	}

	// Get session history to verify commands were added
	history, err := hm.GetSessionHistory(sessionID)
	if err != nil {
		t.Errorf("Failed to get session history: %v", err)
	}

	if len(history.Commands) != 5 {
		t.Errorf("Expected 5 commands in session history, got %d", len(history.Commands))
	}
}

// TestHistoryBasics tests basic history functionality without persistence
func TestHistoryBasics(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "history-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	hm := NewHistoryManager(tempDir)
	sessionID := "basic-session"
	projectID := "basic-project"

	err = hm.CreateSessionHistory(sessionID, projectID, "Basic Test")
	if err != nil {
		t.Errorf("Failed to create session history: %v", err)
	}

	cmd := CommandEntry{
		ID:        "basic-cmd",
		SessionID: sessionID,
		ProjectID: projectID,
		Command:   "echo basic test",
		Success:   true,
		StartTime: time.Now(),
		EndTime:   time.Now().Add(time.Second),
		Duration:  time.Second,
	}

	err = hm.AddCommand(cmd)
	if err != nil {
		t.Errorf("Failed to add command: %v", err)
	}

	// Verify command was added
	history, err := hm.GetSessionHistory(sessionID)
	if err != nil {
		t.Errorf("Failed to get session history: %v", err)
	}

	if len(history.Commands) != 1 {
		t.Errorf("Expected 1 command, got %d", len(history.Commands))
	}

	if history.Commands[0].Command != "echo basic test" {
		t.Errorf("Expected basic test command, got %s", history.Commands[0].Command)
	}

	// Test deletion
	err = hm.DeleteSessionHistory(sessionID)
	if err != nil {
		t.Errorf("Failed to delete session history: %v", err)
	}

	// Verify deletion
	_, err = hm.GetSessionHistory(sessionID)
	if err == nil {
		t.Errorf("Expected error when getting deleted session history")
	}
}
