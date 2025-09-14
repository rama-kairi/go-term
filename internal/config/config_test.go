package config

import (
	"encoding/json"
	"testing"
	"time"
)

// TestDefaultConfig tests the default configuration
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Test server config
	if cfg.Server.Name != "github.com/rama-kairi/go-term" {
		t.Errorf("Expected server name 'github.com/rama-kairi/go-term', got '%s'", cfg.Server.Name)
	}

	if cfg.Server.Version != "2.0.0" {
		t.Errorf("Expected version '2.0.0', got '%s'", cfg.Server.Version)
	}

	// Test session config
	if cfg.Session.MaxSessions != 10 {
		t.Errorf("Expected max sessions 10, got %d", cfg.Session.MaxSessions)
	}

	if cfg.Session.MaxCommandsPerSession != 30 {
		t.Errorf("Expected max commands per session 30, got %d", cfg.Session.MaxCommandsPerSession)
	}

	if cfg.Session.MaxBackgroundProcesses != 3 {
		t.Errorf("Expected max background processes 3, got %d", cfg.Session.MaxBackgroundProcesses)
	}

	// Test database config
	if !cfg.Database.Enable {
		t.Errorf("Expected database to be enabled")
	}

	if cfg.Database.Driver != "sqlite3" {
		t.Errorf("Expected database driver 'sqlite3', got '%s'", cfg.Database.Driver)
	}

	// Test streaming config
	if !cfg.Streaming.Enable {
		t.Errorf("Expected streaming to be enabled")
	}

	if cfg.Streaming.BufferSize != 4096 {
		t.Errorf("Expected streaming buffer size 4096, got %d", cfg.Streaming.BufferSize)
	}

	// Test security config
	if cfg.Security.EnableSandbox {
		t.Errorf("Expected sandbox to be disabled by default")
	}

	if !cfg.Security.AllowNetworkAccess {
		t.Errorf("Expected network access to be allowed by default")
	}

	if !cfg.Security.AllowFileSystemWrite {
		t.Errorf("Expected filesystem write to be allowed by default")
	}

	// Test logging config
	if cfg.Logging.Level != "info" {
		t.Errorf("Expected logging level 'info', got '%s'", cfg.Logging.Level)
	}

	if cfg.Logging.Format != "json" {
		t.Errorf("Expected logging format 'json', got '%s'", cfg.Logging.Format)
	}
}

// TestConfigSerialization tests JSON serialization and deserialization
func TestConfigSerialization(t *testing.T) {
	original := DefaultConfig()

	// Serialize to JSON
	jsonData, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	// Deserialize from JSON
	var restored Config
	err = json.Unmarshal(jsonData, &restored)
	if err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	// Compare key values
	if restored.Server.Name != original.Server.Name {
		t.Errorf("Server name mismatch after serialization")
	}

	if restored.Session.MaxSessions != original.Session.MaxSessions {
		t.Errorf("Max sessions mismatch after serialization")
	}

	if restored.Database.Enable != original.Database.Enable {
		t.Errorf("Database enable mismatch after serialization")
	}
}

// TestTimeValues tests that time durations are properly handled
func TestTimeValues(t *testing.T) {
	cfg := DefaultConfig()

	// Test that time values are reasonable
	if cfg.Session.DefaultTimeout < time.Minute {
		t.Errorf("Default timeout seems too short: %v", cfg.Session.DefaultTimeout)
	}

	if cfg.Session.CleanupInterval < time.Second {
		t.Errorf("Cleanup interval seems too short: %v", cfg.Session.CleanupInterval)
	}

	if cfg.Database.ConnectionTimeout < time.Second {
		t.Errorf("Connection timeout seems too short: %v", cfg.Database.ConnectionTimeout)
	}

	if cfg.Streaming.Timeout < time.Second {
		t.Errorf("Streaming timeout seems too short: %v", cfg.Streaming.Timeout)
	}
}

// TestSecurityDefaults tests that security defaults are reasonable
func TestSecurityDefaults(t *testing.T) {
	cfg := DefaultConfig()

	// Test that dangerous commands are blocked by default
	dangerous := []string{"rm -rf /", "format", "mkfs"}
	for _, cmd := range dangerous {
		found := false
		for _, blocked := range cfg.Security.BlockedCommands {
			if blocked == cmd {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Dangerous command '%s' should be blocked by default", cmd)
		}
	}

	// Test resource limits are reasonable
	if cfg.Security.MaxProcesses < 1 {
		t.Errorf("Max processes should be at least 1, got %d", cfg.Security.MaxProcesses)
	}

	if cfg.Security.MaxMemoryMB < 100 {
		t.Errorf("Max memory should be at least 100MB, got %d", cfg.Security.MaxMemoryMB)
	}

	if cfg.Security.MaxCPUPercent < 10 || cfg.Security.MaxCPUPercent > 100 {
		t.Errorf("Max CPU percent should be between 10-100, got %d", cfg.Security.MaxCPUPercent)
	}
}
