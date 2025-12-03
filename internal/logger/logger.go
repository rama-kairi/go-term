package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/rama-kairi/go-term/internal/config"
)

// LogLevel represents the severity level of a log entry
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Component string                 `json:"component,omitempty"`
	SessionID string                 `json:"session_id,omitempty"`
	UserID    string                 `json:"user_id,omitempty"`
	Command   string                 `json:"command,omitempty"`
	Duration  string                 `json:"duration,omitempty"`
	Error     string                 `json:"error,omitempty"`
	File      string                 `json:"file,omitempty"`
	Line      int                    `json:"line,omitempty"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// Logger provides structured logging capabilities
type Logger struct {
	level      LogLevel
	format     string
	output     io.Writer
	mu         sync.RWMutex
	component  string
	baseFields map[string]interface{}
	fileHandle *os.File // H7: Track file handle for cleanup
}

// NewLogger creates a new logger instance
func NewLogger(cfg *config.LoggingConfig, component string) (*Logger, error) {
	level := parseLogLevel(cfg.Level)

	var output io.Writer
	var fileHandle *os.File // H7: Track if we opened a file
	switch cfg.Output {
	case "stderr":
		output = os.Stderr
	case "stdout":
		output = os.Stdout
	case "file":
		// Default log file
		file, err := os.OpenFile("github.com/rama-kairi/go-term.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
		output = file
		fileHandle = file
	default:
		// Treat as file path
		if strings.HasPrefix(cfg.Output, "/") || strings.Contains(cfg.Output, ".log") {
			file, err := os.OpenFile(cfg.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			if err != nil {
				return nil, fmt.Errorf("failed to open log file %s: %w", cfg.Output, err)
			}
			output = file
			fileHandle = file
		} else {
			output = os.Stderr
		}
	}

	return &Logger{
		level:      level,
		format:     cfg.Format,
		output:     output,
		component:  component,
		baseFields: make(map[string]interface{}),
		fileHandle: fileHandle,
	}, nil
}

// H7: Close closes any open file handles
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.fileHandle != nil {
		err := l.fileHandle.Close()
		l.fileHandle = nil
		return err
	}
	return nil
}

// SetLevel sets the logging level
func (l *Logger) SetLevel(level string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = parseLogLevel(level)
}

// SetBaseField sets a base field that will be included in all log entries
func (l *Logger) SetBaseField(key string, value interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.baseFields[key] = value
}

// WithFields returns a new logger instance with additional fields
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	l.mu.RLock()
	defer l.mu.RUnlock()

	newLogger := &Logger{
		level:      l.level,
		format:     l.format,
		output:     l.output,
		component:  l.component,
		baseFields: make(map[string]interface{}),
	}

	// Copy base fields
	for k, v := range l.baseFields {
		newLogger.baseFields[k] = v
	}

	// Add new fields
	for k, v := range fields {
		newLogger.baseFields[k] = v
	}

	return newLogger
}

// WithSession returns a logger with session ID
func (l *Logger) WithSession(sessionID string) *Logger {
	return l.WithFields(map[string]interface{}{
		"session_id": sessionID,
	})
}

// WithComponent returns a logger with component name
func (l *Logger) WithComponent(component string) *Logger {
	newLogger := l.WithFields(nil)
	newLogger.component = component
	return newLogger
}

// Debug logs a debug message
func (l *Logger) Debug(message string, fields ...map[string]interface{}) {
	l.log(DEBUG, message, "", fields...)
}

// Info logs an info message
func (l *Logger) Info(message string, fields ...map[string]interface{}) {
	l.log(INFO, message, "", fields...)
}

// Warn logs a warning message
func (l *Logger) Warn(message string, fields ...map[string]interface{}) {
	l.log(WARN, message, "", fields...)
}

// Error logs an error message
func (l *Logger) Error(message string, err error, fields ...map[string]interface{}) {
	errorStr := ""
	if err != nil {
		errorStr = err.Error()
	}
	l.log(ERROR, message, errorStr, fields...)
}

// LogCommand logs a command execution
func (l *Logger) LogCommand(sessionID, command string, duration time.Duration, success bool, output string, err error) {
	fields := map[string]interface{}{
		"session_id": sessionID,
		"command":    command,
		"duration":   duration.String(),
		"success":    success,
		"output_len": len(output),
	}

	if err != nil {
		l.Error("Command execution completed with error", err, fields)
	} else {
		l.Info("Command execution completed successfully", fields)
	}
}

// LogSessionEvent logs session-related events
func (l *Logger) LogSessionEvent(event, sessionID, sessionName string, fields ...map[string]interface{}) {
	eventFields := map[string]interface{}{
		"event":        event,
		"session_id":   sessionID,
		"session_name": sessionName,
	}

	if len(fields) > 0 {
		for k, v := range fields[0] {
			eventFields[k] = v
		}
	}

	l.Info(fmt.Sprintf("Session %s", event), eventFields)
}

// LogSecurityEvent logs security-related events
func (l *Logger) LogSecurityEvent(event, details string, severity string, fields ...map[string]interface{}) {
	securityFields := map[string]interface{}{
		"security_event": event,
		"details":        details,
		"severity":       severity,
	}

	if len(fields) > 0 {
		for k, v := range fields[0] {
			securityFields[k] = v
		}
	}

	switch severity {
	case "critical", "high":
		l.Error(fmt.Sprintf("Security event: %s", event), nil, securityFields)
	case "medium":
		l.Warn(fmt.Sprintf("Security event: %s", event), securityFields)
	default:
		l.Info(fmt.Sprintf("Security event: %s", event), securityFields)
	}
}

// log is the internal logging method
func (l *Logger) log(level LogLevel, message, errorStr string, fields ...map[string]interface{}) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	// Check if we should log this level
	if level < l.level {
		return
	}

	// Get caller information
	_, file, line, ok := runtime.Caller(3)
	if ok {
		// Get just the filename, not the full path
		parts := strings.Split(file, "/")
		file = parts[len(parts)-1]
	}

	// Create log entry
	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     level.String(),
		Message:   message,
		Component: l.component,
		Error:     errorStr,
		File:      file,
		Line:      line,
		Fields:    make(map[string]interface{}),
	}

	// Add base fields
	for k, v := range l.baseFields {
		switch k {
		case "session_id":
			entry.SessionID = fmt.Sprintf("%v", v)
		case "user_id":
			entry.UserID = fmt.Sprintf("%v", v)
		case "command":
			entry.Command = fmt.Sprintf("%v", v)
		case "duration":
			entry.Duration = fmt.Sprintf("%v", v)
		default:
			entry.Fields[k] = v
		}
	}

	// Add additional fields
	if len(fields) > 0 {
		for k, v := range fields[0] {
			switch k {
			case "session_id":
				entry.SessionID = fmt.Sprintf("%v", v)
			case "user_id":
				entry.UserID = fmt.Sprintf("%v", v)
			case "command":
				entry.Command = fmt.Sprintf("%v", v)
			case "duration":
				entry.Duration = fmt.Sprintf("%v", v)
			default:
				entry.Fields[k] = v
			}
		}
	}

	// Remove empty fields map if not used
	if len(entry.Fields) == 0 {
		entry.Fields = nil
	}

	// Format and write the log entry
	var output string
	if l.format == "json" {
		data, _ := json.Marshal(entry)
		output = string(data) + "\n"
	} else {
		// Text format
		output = l.formatTextEntry(entry)
	}

	l.output.Write([]byte(output))
}

// formatTextEntry formats a log entry as human-readable text
func (l *Logger) formatTextEntry(entry LogEntry) string {
	var parts []string

	// Timestamp and level
	parts = append(parts, fmt.Sprintf("[%s] %s", entry.Timestamp[:19], entry.Level))

	// Component
	if entry.Component != "" {
		parts = append(parts, fmt.Sprintf("[%s]", entry.Component))
	}

	// Session ID
	if entry.SessionID != "" {
		parts = append(parts, fmt.Sprintf("[session:%s]", entry.SessionID[:8]))
	}

	// Message
	parts = append(parts, entry.Message)

	// Error
	if entry.Error != "" {
		parts = append(parts, fmt.Sprintf("error=%s", entry.Error))
	}

	// Command
	if entry.Command != "" {
		parts = append(parts, fmt.Sprintf("cmd=%q", entry.Command))
	}

	// Duration
	if entry.Duration != "" {
		parts = append(parts, fmt.Sprintf("duration=%s", entry.Duration))
	}

	// Additional fields
	if entry.Fields != nil {
		for k, v := range entry.Fields {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
	}

	// File and line (only in debug mode)
	if l.level == DEBUG && entry.File != "" {
		parts = append(parts, fmt.Sprintf("(%s:%d)", entry.File, entry.Line))
	}

	return strings.Join(parts, " ") + "\n"
}

// parseLogLevel converts a string to LogLevel
func parseLogLevel(level string) LogLevel {
	switch strings.ToLower(level) {
	case "debug":
		return DEBUG
	case "info":
		return INFO
	case "warn", "warning":
		return WARN
	case "error":
		return ERROR
	default:
		return INFO
	}
}

// GetDefaultLogger creates a default logger for the application
func GetDefaultLogger() *Logger {
	cfg := &config.LoggingConfig{
		Level:  "info",
		Format: "json",
		Output: "stderr",
	}

	logger, _ := NewLogger(cfg, "github.com/rama-kairi/go-term")
	return logger
}
