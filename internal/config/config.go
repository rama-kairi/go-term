package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config holds all configuration for the Terminal MCP server
type Config struct {
	// Server configuration
	Server ServerConfig `json:"server"`

	// Session configuration
	Session SessionConfig `json:"session"`

	// Database configuration
	Database DatabaseConfig `json:"database"`

	// Streaming configuration
	Streaming StreamingConfig `json:"streaming"`

	// Security configuration
	Security SecurityConfig `json:"security"`

	// Logging configuration
	Logging LoggingConfig `json:"logging"`

	// Monitoring configuration
	Monitoring MonitoringConfig `json:"monitoring"`
}

// ServerConfig holds server-specific configuration
type ServerConfig struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Debug   bool   `json:"debug"`
}

// SessionConfig holds session management configuration
type SessionConfig struct {
	MaxSessions             int           `json:"max_sessions"`
	DefaultTimeout          time.Duration `json:"default_timeout"`
	CleanupInterval         time.Duration `json:"cleanup_interval"`
	MaxCommandLength        int           `json:"max_command_length"`
	MaxOutputSize           int           `json:"max_output_size"`
	WorkingDir              string        `json:"working_dir"`
	Shell                   string        `json:"shell"`
	EnableStreaming         bool          `json:"enable_streaming"`
	MaxCommandsPerSession   int           `json:"max_commands_per_session"`
	MaxBackgroundProcesses  int           `json:"max_background_processes"`
	BackgroundOutputLimit   int           `json:"background_output_limit"`
	ResourceCleanupInterval time.Duration `json:"resource_cleanup_interval"`
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Enable            bool          `json:"enable"`
	Driver            string        `json:"driver"`
	Path              string        `json:"path"`
	DataDir           string        `json:"data_dir"`
	MaxConnections    int           `json:"max_connections"`
	ConnectionTimeout time.Duration `json:"connection_timeout"`
	EnableWAL         bool          `json:"enable_wal"`
	VacuumInterval    time.Duration `json:"vacuum_interval"`
}

// StreamingConfig holds streaming configuration
type StreamingConfig struct {
	Enable     bool          `json:"enable"`
	BufferSize int           `json:"buffer_size"`
	Timeout    time.Duration `json:"timeout"`
}

// SecurityConfig holds security-related configuration
type SecurityConfig struct {
	EnableSandbox        bool     `json:"enable_sandbox"`
	AllowedCommands      []string `json:"allowed_commands"`
	BlockedCommands      []string `json:"blocked_commands"`
	AllowNetworkAccess   bool     `json:"allow_network_access"`
	AllowFileSystemWrite bool     `json:"allow_filesystem_write"`
	MaxProcesses         int      `json:"max_processes"`
	MaxMemoryMB          int      `json:"max_memory_mb"`
	MaxCPUPercent        int      `json:"max_cpu_percent"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level      string `json:"level"`
	Format     string `json:"format"` // "json" or "text"
	Output     string `json:"output"` // "stderr", "file", or file path
	MaxSizeMB  int    `json:"max_size_mb"`
	MaxBackups int    `json:"max_backups"`
	MaxAgeDays int    `json:"max_age_days"`
}

// MonitoringConfig holds monitoring configuration
type MonitoringConfig struct {
	EnableMetrics   bool          `json:"enable_metrics"`
	MetricsPort     int           `json:"metrics_port"`
	HealthCheckPort int           `json:"health_check_port"`
	StatsInterval   time.Duration `json:"stats_interval"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	// Get user's home directory
	homeDir, _ := os.UserHomeDir()
	configDir := filepath.Join(homeDir, ".config", "go-term")

	return &Config{
		Server: ServerConfig{
			Name:    "github.com/rama-kairi/go-term",
			Version: "2.0.0",
			Debug:   false,
		},
		Session: SessionConfig{
			MaxSessions:             10,               // User requested: max 10 sessions
			DefaultTimeout:          60 * time.Minute, // Increased from 30 minutes
			CleanupInterval:         5 * time.Minute,
			MaxCommandLength:        50000,            // Increased from 10000
			MaxOutputSize:           10 * 1024 * 1024, // 10MB, increased from 1MB
			WorkingDir:              "",               // Use current directory
			Shell:                   "",               // Use system default
			EnableStreaming:         true,             // Enable real-time streaming
			MaxCommandsPerSession:   30,               // User requested: max 30 commands per session
			MaxBackgroundProcesses:  3,                // User requested: max 3 background processes
			BackgroundOutputLimit:   2000,             // Keep only latest 2000 characters of background output
			ResourceCleanupInterval: 1 * time.Minute,  // Cleanup every minute
		},
		Database: DatabaseConfig{
			Enable:            true,
			Driver:            "sqlite3",
			Path:              filepath.Join(configDir, "sessions.db"),
			DataDir:           configDir,
			MaxConnections:    10,
			ConnectionTimeout: 5 * time.Second,
			EnableWAL:         true,
			VacuumInterval:    24 * time.Hour,
		},
		Streaming: StreamingConfig{
			Enable:     true,
			BufferSize: 4096,
			Timeout:    30 * time.Second,
		},
		Security: SecurityConfig{
			EnableSandbox:   false,      // Disabled for better usability
			AllowedCommands: []string{}, // Empty means all allowed (subject to blocked)
			BlockedCommands: []string{
				// Only block truly dangerous commands
				"rm -rf /", "format", "mkfs", "dd if=/dev/zero", ":(){ :|:& };:",
			},
			AllowNetworkAccess:   true, // Allow network access
			AllowFileSystemWrite: true,
			MaxProcesses:         20,   // Increased from 5
			MaxMemoryMB:          2048, // Increased from 512
			MaxCPUPercent:        80,   // Increased from 50
		},
		Logging: LoggingConfig{
			Level:      "info",
			Format:     "json",
			Output:     "stderr",
			MaxSizeMB:  100,
			MaxBackups: 3,
			MaxAgeDays: 30,
		},
		Monitoring: MonitoringConfig{
			EnableMetrics:   false,
			MetricsPort:     9090,
			HealthCheckPort: 8080,
			StatsInterval:   30 * time.Second,
		},
	}
}

// LoadConfig loads configuration from environment variables and optional config file
func LoadConfig(configFile string) (*Config, error) {
	config := DefaultConfig()

	// Get user's home directory for default config
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".config", "go-term")
	defaultConfigFile := filepath.Join(configDir, "config.json")

	// Ensure config directory exists
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	// Determine which config file to use
	configFileToUse := defaultConfigFile
	if configFile != "" {
		configFileToUse = configFile
	}

	// Create default config file if it doesn't exist and no custom config file was specified
	if configFile == "" && !fileExists(defaultConfigFile) {
		if err := config.SaveToFile(defaultConfigFile); err != nil {
			return nil, fmt.Errorf("failed to create default config file: %w", err)
		}
	}

	// Load from config file if it exists
	if fileExists(configFileToUse) {
		if err := loadFromFile(config, configFileToUse); err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
	}

	// Override with environment variables
	loadFromEnvironment(config)

	// Update paths to use the proper config directory
	if config.Database.DataDir == "" || strings.Contains(config.Database.DataDir, ".github.com") {
		config.Database.DataDir = configDir
		config.Database.Path = filepath.Join(configDir, "sessions.db")
	}

	// Validate configuration
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// fileExists checks if a file exists
func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

// loadFromFile loads configuration from a JSON file
func loadFromFile(config *Config, filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, config)
}

// loadFromEnvironment loads configuration from environment variables
func loadFromEnvironment(config *Config) {
	// Server configuration
	if val := os.Getenv("TERMINAL_MCP_DEBUG"); val != "" {
		config.Server.Debug = parseBool(val)
	}

	// Session configuration
	if val := os.Getenv("TERMINAL_MCP_MAX_SESSIONS"); val != "" {
		config.Session.MaxSessions = parseInt(val, config.Session.MaxSessions)
	}
	if val := os.Getenv("TERMINAL_MCP_SESSION_TIMEOUT"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			config.Session.DefaultTimeout = duration
		}
	}
	if val := os.Getenv("TERMINAL_MCP_CLEANUP_INTERVAL"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			config.Session.CleanupInterval = duration
		}
	}
	if val := os.Getenv("TERMINAL_MCP_MAX_COMMAND_LENGTH"); val != "" {
		config.Session.MaxCommandLength = parseInt(val, config.Session.MaxCommandLength)
	}
	if val := os.Getenv("TERMINAL_MCP_MAX_OUTPUT_SIZE"); val != "" {
		config.Session.MaxOutputSize = parseInt(val, config.Session.MaxOutputSize)
	}
	if val := os.Getenv("TERMINAL_MCP_WORKING_DIR"); val != "" {
		config.Session.WorkingDir = val
	}
	if val := os.Getenv("TERMINAL_MCP_SHELL"); val != "" {
		config.Session.Shell = val
	}
	if val := os.Getenv("TERMINAL_MCP_ENABLE_STREAMING"); val != "" {
		config.Session.EnableStreaming = parseBool(val)
	}
	if val := os.Getenv("TERMINAL_MCP_MAX_COMMANDS_PER_SESSION"); val != "" {
		config.Session.MaxCommandsPerSession = parseInt(val, config.Session.MaxCommandsPerSession)
	}
	if val := os.Getenv("TERMINAL_MCP_MAX_BACKGROUND_PROCESSES"); val != "" {
		config.Session.MaxBackgroundProcesses = parseInt(val, config.Session.MaxBackgroundProcesses)
	}
	if val := os.Getenv("TERMINAL_MCP_BACKGROUND_OUTPUT_LIMIT"); val != "" {
		config.Session.BackgroundOutputLimit = parseInt(val, config.Session.BackgroundOutputLimit)
	}
	if val := os.Getenv("TERMINAL_MCP_RESOURCE_CLEANUP_INTERVAL"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			config.Session.ResourceCleanupInterval = duration
		}
	}

	// Database configuration
	if val := os.Getenv("TERMINAL_MCP_DATA_DIR"); val != "" {
		config.Database.DataDir = val
	}
	if val := os.Getenv("TERMINAL_MCP_MAX_CONNECTIONS"); val != "" {
		config.Database.MaxConnections = parseInt(val, config.Database.MaxConnections)
	}
	if val := os.Getenv("TERMINAL_MCP_CONNECTION_TIMEOUT"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			config.Database.ConnectionTimeout = duration
		}
	}
	if val := os.Getenv("TERMINAL_MCP_ENABLE_WAL"); val != "" {
		config.Database.EnableWAL = parseBool(val)
	}

	// Security configuration
	if val := os.Getenv("TERMINAL_MCP_ENABLE_SANDBOX"); val != "" {
		config.Security.EnableSandbox = parseBool(val)
	}
	if val := os.Getenv("TERMINAL_MCP_BLOCKED_COMMANDS"); val != "" {
		config.Security.BlockedCommands = strings.Split(val, ",")
		for i := range config.Security.BlockedCommands {
			config.Security.BlockedCommands[i] = strings.TrimSpace(config.Security.BlockedCommands[i])
		}
	}
	if val := os.Getenv("TERMINAL_MCP_ALLOW_NETWORK"); val != "" {
		config.Security.AllowNetworkAccess = parseBool(val)
	}
	if val := os.Getenv("TERMINAL_MCP_ALLOW_FILESYSTEM_WRITE"); val != "" {
		config.Security.AllowFileSystemWrite = parseBool(val)
	}
	if val := os.Getenv("TERMINAL_MCP_MAX_PROCESSES"); val != "" {
		config.Security.MaxProcesses = parseInt(val, config.Security.MaxProcesses)
	}
	if val := os.Getenv("TERMINAL_MCP_MAX_MEMORY_MB"); val != "" {
		config.Security.MaxMemoryMB = parseInt(val, config.Security.MaxMemoryMB)
	}
	if val := os.Getenv("TERMINAL_MCP_MAX_CPU_PERCENT"); val != "" {
		config.Security.MaxCPUPercent = parseInt(val, config.Security.MaxCPUPercent)
	}

	// Logging configuration
	if val := os.Getenv("TERMINAL_MCP_LOG_LEVEL"); val != "" {
		config.Logging.Level = val
	}
	if val := os.Getenv("TERMINAL_MCP_LOG_FORMAT"); val != "" {
		config.Logging.Format = val
	}
	if val := os.Getenv("TERMINAL_MCP_LOG_OUTPUT"); val != "" {
		config.Logging.Output = val
	}

	// Monitoring configuration
	if val := os.Getenv("TERMINAL_MCP_ENABLE_METRICS"); val != "" {
		config.Monitoring.EnableMetrics = parseBool(val)
	}
	if val := os.Getenv("TERMINAL_MCP_METRICS_PORT"); val != "" {
		config.Monitoring.MetricsPort = parseInt(val, config.Monitoring.MetricsPort)
	}
	if val := os.Getenv("TERMINAL_MCP_HEALTH_PORT"); val != "" {
		config.Monitoring.HealthCheckPort = parseInt(val, config.Monitoring.HealthCheckPort)
	}
}

// validateConfig validates the configuration values
func validateConfig(config *Config) error {
	if config.Session.MaxSessions <= 0 {
		return fmt.Errorf("max_sessions must be greater than 0")
	}

	if config.Session.DefaultTimeout <= 0 {
		return fmt.Errorf("default_timeout must be greater than 0")
	}

	if config.Session.MaxCommandLength <= 0 {
		return fmt.Errorf("max_command_length must be greater than 0")
	}

	if config.Session.MaxOutputSize <= 0 {
		return fmt.Errorf("max_output_size must be greater than 0")
	}

	if config.Session.MaxCommandsPerSession <= 0 {
		return fmt.Errorf("max_commands_per_session must be greater than 0")
	}

	if config.Session.MaxBackgroundProcesses <= 0 {
		return fmt.Errorf("max_background_processes must be greater than 0")
	}

	if config.Session.BackgroundOutputLimit <= 0 {
		return fmt.Errorf("background_output_limit must be greater than 0")
	}

	if config.Session.ResourceCleanupInterval <= 0 {
		return fmt.Errorf("resource_cleanup_interval must be greater than 0")
	}

	if config.Security.MaxProcesses <= 0 {
		return fmt.Errorf("max_processes must be greater than 0")
	}

	if config.Security.MaxMemoryMB <= 0 {
		return fmt.Errorf("max_memory_mb must be greater than 0")
	}

	if config.Security.MaxCPUPercent <= 0 || config.Security.MaxCPUPercent > 100 {
		return fmt.Errorf("max_cpu_percent must be between 1 and 100")
	}

	validLogLevels := map[string]bool{
		"debug": true, "info": true, "warn": true, "error": true,
	}
	if !validLogLevels[strings.ToLower(config.Logging.Level)] {
		return fmt.Errorf("invalid log level: %s", config.Logging.Level)
	}

	validLogFormats := map[string]bool{
		"json": true, "text": true,
	}
	if !validLogFormats[strings.ToLower(config.Logging.Format)] {
		return fmt.Errorf("invalid log format: %s", config.Logging.Format)
	}

	return nil
}

// Helper functions for parsing environment variables
func parseBool(s string) bool {
	val, _ := strconv.ParseBool(s)
	return val
}

func parseInt(s string, defaultVal int) int {
	if val, err := strconv.Atoi(s); err == nil {
		return val
	}
	return defaultVal
}

// SaveConfig saves the current configuration to a file
func (c *Config) SaveToFile(filename string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0o644)
}

// GetConfigDir returns the default configuration directory path
func GetConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	return filepath.Join(homeDir, ".config", "go-term"), nil
}

// GetDefaultConfigPath returns the default configuration file path
func GetDefaultConfigPath() (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "config.json"), nil
}
