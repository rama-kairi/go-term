package logger

import (
	"testing"

	"github.com/rama-kairi/go-term/internal/config"
)

// TestNewLogger tests logger creation
func TestNewLogger(t *testing.T) {
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
}

// TestLogLevels tests different log levels
func TestLogLevels(t *testing.T) {
	cfg := &config.LoggingConfig{
		Level:  "debug",
		Format: "text",
		Output: "stderr",
	}

	logger, err := NewLogger(cfg, "test")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Test different log levels
	logger.Debug("Debug message", map[string]interface{}{"key": "value"})
	logger.Info("Info message", map[string]interface{}{"key": "value"})
	logger.Warn("Warning message", map[string]interface{}{"key": "value"})
	logger.Error("Error message", nil, map[string]interface{}{"key": "value"})
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
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.level.String() != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, tt.level.String())
			}
		})
	}
}
