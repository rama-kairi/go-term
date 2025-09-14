package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Server.Name != "github.com/rama-kairi/go-term" {
		t.Errorf("Expected server name 'github.com/rama-kairi/go-term', got '%s'", cfg.Server.Name)
	}

	if cfg.Session.MaxSessions != 10 {
		t.Errorf("Expected max sessions 10, got %d", cfg.Session.MaxSessions)
	}

	if !cfg.Database.Enable {
		t.Errorf("Expected database to be enabled")
	}
}

func TestLoadConfig(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "config_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	testConfig := DefaultConfig()
	testConfig.Server.Debug = true
	testConfig.Session.MaxSessions = 20

	testConfigFile := filepath.Join(tempDir, "test_config.json")
	err = testConfig.SaveToFile(testConfigFile)
	if err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	loadedConfig, err := LoadConfig(testConfigFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if !loadedConfig.Server.Debug {
		t.Error("Expected debug to be true")
	}

	if loadedConfig.Session.MaxSessions != 20 {
		t.Errorf("Expected max sessions 20, got %d", loadedConfig.Session.MaxSessions)
	}
}

func TestEnvironmentVariables(t *testing.T) {
	envVars := map[string]string{
		"TERMINAL_MCP_DEBUG":        "true",
		"TERMINAL_MCP_MAX_SESSIONS": "15",
		"TERMINAL_MCP_LOG_LEVEL":    "debug",
	}

	origEnv := make(map[string]string)
	for key, value := range envVars {
		origEnv[key] = os.Getenv(key)
		os.Setenv(key, value)
	}
	defer func() {
		for key, origValue := range origEnv {
			if origValue == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, origValue)
			}
		}
	}()

	config, err := LoadConfig("")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if !config.Server.Debug {
		t.Error("Expected debug to be true from environment")
	}

	if config.Session.MaxSessions != 15 {
		t.Errorf("Expected max sessions 15 from environment, got %d", config.Session.MaxSessions)
	}
}

func TestValidation(t *testing.T) {
	config := DefaultConfig()
	config.Session.MaxSessions = 0

	err := validateConfig(config)
	if err == nil {
		t.Error("Expected error for zero max sessions")
	}
}

func TestSaveToFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "config_save_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	config := DefaultConfig()
	configFile := filepath.Join(tempDir, "save_test.json")
	err = config.SaveToFile(configFile)
	if err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}
}

func TestGetConfigDir(t *testing.T) {
	configDir, err := GetConfigDir()
	if err != nil {
		t.Fatalf("Failed to get config dir: %v", err)
	}

	if !strings.Contains(configDir, ".config") {
		t.Error("Config dir should contain '.config'")
	}
}

func TestGetDefaultConfigPath(t *testing.T) {
	configPath, err := GetDefaultConfigPath()
	if err != nil {
		t.Fatalf("Failed to get default config path: %v", err)
	}

	if !strings.HasSuffix(configPath, "config.json") {
		t.Error("Config path should end with 'config.json'")
	}
}

func TestHelperFunctions(t *testing.T) {
	if !parseBool("true") {
		t.Error("Expected parseBool('true') to return true")
	}

	if parseBool("false") {
		t.Error("Expected parseBool('false') to return false")
	}

	if parseInt("123", 0) != 123 {
		t.Error("Expected parseInt('123', 0) to return 123")
	}

	if parseInt("invalid", 50) != 50 {
		t.Error("Expected parseInt('invalid', 50) to return default 50")
	}
}

func TestFileExists(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "file_exists_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	existingFile := filepath.Join(tempDir, "exists.txt")
	err = os.WriteFile(existingFile, []byte("test"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	if !fileExists(existingFile) {
		t.Error("Expected fileExists to return true for existing file")
	}

	nonExistentFile := filepath.Join(tempDir, "does_not_exist.txt")
	if fileExists(nonExistentFile) {
		t.Error("Expected fileExists to return false for non-existent file")
	}
}
