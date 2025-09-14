package logger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rama-kairi/go-term/internal/config"
)

// TestNewLogger tests logger creation
func TestNewLogger(t *testing.T) {
	t.Run("BasicLoggerCreation", func(t *testing.T) {
		cfg := &config.LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stderr",
		}

		logger, err := NewLogger(cfg, "test")
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}

		if logger == nil {
			t.Errorf("Expected logger instance, got nil")
		}
	})

	t.Run("FileOutputLogger", func(t *testing.T) {
		// Create temporary file
		tmpDir := t.TempDir()
		logFile := filepath.Join(tmpDir, "test.log")

		cfg := &config.LoggingConfig{
			Level:  "debug",
			Format: "text",
			Output: logFile,
		}

		logger, err := NewLogger(cfg, "test")
		if err != nil {
			t.Fatalf("Failed to create file logger: %v", err)
		}

		// Log something
		logger.Info("Test file output")

		// Check if file was created and has content
		if _, err := os.Stat(logFile); os.IsNotExist(err) {
			t.Error("Log file was not created")
		}
	})

	t.Run("DefaultFileLogger", func(t *testing.T) {
		// Skip this test as it creates files in the working directory
		t.Skip("Skipping default file logger test to avoid file creation in working directory")
	})

	t.Run("StdoutLogger", func(t *testing.T) {
		cfg := &config.LoggingConfig{
			Level:  "warn",
			Format: "text",
			Output: "stdout",
		}

		logger, err := NewLogger(cfg, "test")
		if err != nil {
			t.Fatalf("Failed to create stdout logger: %v", err)
		}

		if logger == nil {
			t.Error("Expected logger instance, got nil")
		}
	})

	t.Run("InvalidFilePathFallback", func(t *testing.T) {
		cfg := &config.LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "unknown_output",
		}

		logger, err := NewLogger(cfg, "test")
		if err != nil {
			t.Fatalf("Failed to create logger with unknown output: %v", err)
		}

		if logger == nil {
			t.Error("Expected logger instance, got nil")
		}
	})
}

// TestLogLevels tests different log levels
func TestLogLevels(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.LoggingConfig{
		Level:  "debug",
		Format: "text",
		Output: "stderr",
	}

	logger, err := NewLogger(cfg, "test")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Redirect output to buffer for testing
	logger.output = &buf

	// Test different log levels
	logger.Debug("Debug message", map[string]interface{}{"key": "value"})
	logger.Info("Info message", map[string]interface{}{"key": "value"})
	logger.Warn("Warning message", map[string]interface{}{"key": "value"})
	logger.Error("Error message", nil, map[string]interface{}{"key": "value"})
	logger.Error("Error with error", fmt.Errorf("test error"), map[string]interface{}{"key": "value"})

	output := buf.String()
	if !strings.Contains(output, "Debug message") {
		t.Error("Debug message not found in output")
	}
	if !strings.Contains(output, "Info message") {
		t.Error("Info message not found in output")
	}
	if !strings.Contains(output, "Warning message") {
		t.Error("Warning message not found in output")
	}
	if !strings.Contains(output, "Error message") {
		t.Error("Error message not found in output")
	}
	if !strings.Contains(output, "test error") {
		t.Error("Error text not found in output")
	}
}

// TestLogLevelString tests log level string conversion
func TestLogLevelString(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected string
	}{
		{DEBUG, "DEBUG"},
		{INFO, "INFO"},
		{WARN, "WARN"},
		{ERROR, "ERROR"},
		{LogLevel(999), "UNKNOWN"}, // Test unknown level
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.level.String() != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, tt.level.String())
			}
		})
	}
}

// TestLogLevelFiltering tests that log levels are properly filtered
func TestLogLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.LoggingConfig{
		Level:  "warn",
		Format: "text",
		Output: "stderr",
	}

	logger, err := NewLogger(cfg, "test")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	logger.output = &buf

	// These should not appear in output due to level filtering
	logger.Debug("Debug message")
	logger.Info("Info message")

	// These should appear
	logger.Warn("Warning message")
	logger.Error("Error message", nil)

	output := buf.String()
	if strings.Contains(output, "Debug message") {
		t.Error("Debug message should not appear with WARN level")
	}
	if strings.Contains(output, "Info message") {
		t.Error("Info message should not appear with WARN level")
	}
	if !strings.Contains(output, "Warning message") {
		t.Error("Warning message should appear with WARN level")
	}
	if !strings.Contains(output, "Error message") {
		t.Error("Error message should appear with WARN level")
	}
}

// TestSetLevel tests changing log level
func TestSetLevel(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "stderr",
	}

	logger, err := NewLogger(cfg, "test")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	logger.output = &buf

	// Change level to error
	logger.SetLevel("error")

	// This should not appear
	logger.Info("Info message")
	// This should appear
	logger.Error("Error message", nil)

	output := buf.String()
	if strings.Contains(output, "Info message") {
		t.Error("Info message should not appear after setting level to ERROR")
	}
	if !strings.Contains(output, "Error message") {
		t.Error("Error message should appear after setting level to ERROR")
	}
}

// TestSetBaseField tests setting base fields
func TestSetBaseField(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "stderr",
	}

	logger, err := NewLogger(cfg, "test")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	logger.output = &buf

	// Set base field
	logger.SetBaseField("user_id", "test_user")
	logger.SetBaseField("environment", "testing")

	logger.Info("Test message with base fields")

	output := buf.String()
	if !strings.Contains(output, "environment=testing") {
		t.Error("Base field environment not found in output")
	}
	// Note: user_id appears as a special field in the log format, not as key=value
}

// TestWithFields tests creating logger with additional fields
func TestWithFields(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "stderr",
	}

	logger, err := NewLogger(cfg, "test")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Set some base fields
	logger.SetBaseField("base_field", "base_value")

	// Create logger with additional fields
	newLogger := logger.WithFields(map[string]interface{}{
		"session_id": "test_session",
		"command":    "test_command",
		"duration":   "100ms",
		"extra":      "extra_value",
	})

	newLogger.output = &buf
	newLogger.Info("Test message with fields")

	output := buf.String()
	if !strings.Contains(output, "session:test_ses") { // Truncated session ID
		t.Error("Session ID not found in output")
	}
	if !strings.Contains(output, "cmd=\"test_command\"") {
		t.Error("Command not found in output")
	}
	if !strings.Contains(output, "duration=100ms") {
		t.Error("Duration not found in output")
	}
	if !strings.Contains(output, "base_field=base_value") {
		t.Error("Base field not found in output")
	}
	if !strings.Contains(output, "extra=extra_value") {
		t.Error("Extra field not found in output")
	}
}

// TestWithSession tests creating logger with session ID
func TestWithSession(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "stderr",
	}

	logger, err := NewLogger(cfg, "test")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	sessionLogger := logger.WithSession("test_session_id_12345")
	sessionLogger.output = &buf
	sessionLogger.Info("Test session message")

	output := buf.String()
	if !strings.Contains(output, "session:test_ses") { // Truncated to 8 chars
		t.Error("Session ID not found in output")
	}
}

// TestWithComponent tests creating logger with component name
func TestWithComponent(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "stderr",
	}

	logger, err := NewLogger(cfg, "test")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	componentLogger := logger.WithComponent("new_component")
	componentLogger.output = &buf
	componentLogger.Info("Test component message")

	output := buf.String()
	if !strings.Contains(output, "[new_component]") {
		t.Error("Component name not found in output")
	}
}

// TestLogCommand tests command logging
func TestLogCommand(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "stderr",
	}

	logger, err := NewLogger(cfg, "test")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	logger.output = &buf

	// Test successful command
	logger.LogCommand("session123", "echo hello", 100*time.Millisecond, true, "hello", nil)

	// Test failed command
	logger.LogCommand("session123", "failed_command", 50*time.Millisecond, false, "", fmt.Errorf("command failed"))

	output := buf.String()
	if !strings.Contains(output, "Command execution completed successfully") {
		t.Error("Successful command log not found")
	}
	if !strings.Contains(output, "Command execution completed with error") {
		t.Error("Failed command log not found")
	}
	if !strings.Contains(output, "cmd=\"echo hello\"") {
		t.Error("Command not found in log")
	}
	if !strings.Contains(output, "command failed") {
		t.Error("Error message not found in failed command log")
	}
	// Note: session_id appears as a special field in the log format
}

// TestLogSessionEvent tests session event logging
func TestLogSessionEvent(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "stderr",
	}

	logger, err := NewLogger(cfg, "test")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	logger.output = &buf

	// Test session event without additional fields
	logger.LogSessionEvent("created", "session123", "test-session")

	// Test session event with additional fields
	logger.LogSessionEvent("terminated", "session456", "another-session", map[string]interface{}{
		"reason":   "timeout",
		"duration": "5m",
	})

	output := buf.String()
	if !strings.Contains(output, "Session created") {
		t.Error("Session created event not found")
	}
	if !strings.Contains(output, "Session terminated") {
		t.Error("Session terminated event not found")
	}
	if !strings.Contains(output, "event=created") {
		t.Error("Event field not found")
	}
	if !strings.Contains(output, "session_name=test-session") {
		t.Error("Session name not found in event log")
	}
	if !strings.Contains(output, "reason=timeout") {
		t.Error("Additional field not found in event log")
	}
	// Note: session_id appears as a special field in the log format
}

// TestLogSecurityEvent tests security event logging
func TestLogSecurityEvent(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "stderr",
	}

	logger, err := NewLogger(cfg, "test")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	logger.output = &buf

	// Test different severity levels
	logger.LogSecurityEvent("suspicious_command", "Attempted to run rm -rf", "critical")
	logger.LogSecurityEvent("failed_login", "Invalid credentials", "medium")
	logger.LogSecurityEvent("session_timeout", "Session expired", "low")

	// Test with additional fields
	logger.LogSecurityEvent("privilege_escalation", "Sudo attempt", "high", map[string]interface{}{
		"user": "testuser",
		"ip":   "192.168.1.1",
	})

	output := buf.String()
	if !strings.Contains(output, "Security event: suspicious_command") {
		t.Error("Critical security event not found")
	}
	if !strings.Contains(output, "Security event: failed_login") {
		t.Error("Medium security event not found")
	}
	if !strings.Contains(output, "Security event: session_timeout") {
		t.Error("Low security event not found")
	}
	if !strings.Contains(output, "security_event=suspicious_command") {
		t.Error("Security event field not found")
	}
	if !strings.Contains(output, "severity=critical") {
		t.Error("Severity field not found")
	}
	if !strings.Contains(output, "user=testuser") {
		t.Error("Additional security field not found")
	}
}

// TestJSONFormat tests JSON formatting
func TestJSONFormat(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.LoggingConfig{
		Level:  "info",
		Format: "json",
		Output: "stderr",
	}

	logger, err := NewLogger(cfg, "test")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	logger.output = &buf
	logger.Info("Test JSON message", map[string]interface{}{
		"test_field": "test_value",
		"number":     42,
	})

	output := buf.String()
	var entry LogEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &entry); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	if entry.Level != "INFO" {
		t.Errorf("Expected level INFO, got %s", entry.Level)
	}
	if entry.Message != "Test JSON message" {
		t.Errorf("Expected message 'Test JSON message', got %s", entry.Message)
	}
	if entry.Component != "test" {
		t.Errorf("Expected component 'test', got %s", entry.Component)
	}
	if entry.Fields["test_field"] != "test_value" {
		t.Error("Expected test_field in JSON fields")
	}
	if entry.Fields["number"] != float64(42) { // JSON numbers are float64
		t.Error("Expected number field in JSON")
	}
}

// TestParseLogLevel tests log level parsing
func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected LogLevel
	}{
		{"debug", DEBUG},
		{"DEBUG", DEBUG},
		{"info", INFO},
		{"INFO", INFO},
		{"warn", WARN},
		{"WARN", WARN},
		{"warning", WARN},
		{"error", ERROR},
		{"ERROR", ERROR},
		{"invalid", INFO}, // Default to INFO
		{"", INFO},        // Default to INFO
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseLogLevel(tt.input)
			if result != tt.expected {
				t.Errorf("parseLogLevel(%s): expected %v, got %v", tt.input, tt.expected, result)
			}
		})
	}
}

// TestGetDefaultLogger tests default logger creation
func TestGetDefaultLogger(t *testing.T) {
	logger := GetDefaultLogger()
	if logger == nil {
		t.Error("Expected default logger instance, got nil")
	}
	if logger.component != "github.com/rama-kairi/go-term" {
		t.Errorf("Expected component 'github.com/rama-kairi/go-term', got %s", logger.component)
	}
	if logger.level != INFO {
		t.Errorf("Expected INFO level, got %v", logger.level)
	}
}

// TestConcurrentLogging tests concurrent logging safety
func TestConcurrentLogging(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "stderr",
	}

	logger, err := NewLogger(cfg, "test")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	logger.output = &buf

	// Test concurrent logging
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			logger.Info(fmt.Sprintf("Concurrent message %d", id))
			logger.SetLevel("debug")
			logger.SetBaseField(fmt.Sprintf("field_%d", id), fmt.Sprintf("value_%d", id))
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	output := buf.String()
	// Should have at least some messages
	if len(output) == 0 {
		t.Error("Expected some output from concurrent logging")
	}
}
