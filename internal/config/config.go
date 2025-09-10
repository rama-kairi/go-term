package config

import (
	"encoding/json"
	"fmt"
	"os"
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
	MaxSessions      int           `json:"max_sessions"`
	DefaultTimeout   time.Duration `json:"default_timeout"`
	CleanupInterval  time.Duration `json:"cleanup_interval"`
	MaxCommandLength int           `json:"max_command_length"`
	MaxOutputSize    int           `json:"max_output_size"`
	WorkingDir       string        `json:"working_dir"`
	Shell            string        `json:"shell"`
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
	EnableMetrics    bool   `json:"enable_metrics"`
	MetricsPort      int    `json:"metrics_port"`
	HealthCheckPort  int    `json:"health_check_port"`
	StatsInterval    time.Duration `json:"stats_interval"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Name:    "terminal-mcp",
			Version: "2.0.0",
			Debug:   false,
		},
		Session: SessionConfig{
			MaxSessions:      10,
			DefaultTimeout:   30 * time.Minute,
			CleanupInterval:  5 * time.Minute,
			MaxCommandLength: 10000,
			MaxOutputSize:    1024 * 1024, // 1MB
			WorkingDir:       "",          // Use current directory
			Shell:            "",          // Use system default
		},
		Security: SecurityConfig{
			EnableSandbox:        true,
			AllowedCommands:      []string{}, // Empty means all allowed (subject to blocked)
			BlockedCommands:      []string{
				"rm -rf /", "format", "mkfs", "dd if=/dev/zero", ":(){ :|:& };:",
				"sudo", "su", "passwd", "useradd", "userdel", "groupadd", "groupdel",
				"chmod 777", "chown", "mount", "umount", "fdisk", "parted",
				"iptables", "ufw", "firewall-cmd", "systemctl", "service",
				"reboot", "shutdown", "halt", "poweroff", "init",
			},
			AllowNetworkAccess:   false,
			AllowFileSystemWrite: true,
			MaxProcesses:         5,
			MaxMemoryMB:          512,
			MaxCPUPercent:        50,
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

	// Load from config file if provided
	if configFile != "" {
		if err := loadFromFile(config, configFile); err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
	}

	// Override with environment variables
	loadFromEnvironment(config)

	// Validate configuration
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
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

	return os.WriteFile(filename, data, 0644)
}
