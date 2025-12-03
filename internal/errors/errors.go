// Package errors provides standardized error types for the go-term application.
// This enables consistent error handling, categorization, and user-friendly messages.
package errors

import (
	"errors"
	"fmt"
)

// ErrorCode represents standardized error categories
type ErrorCode string

const (
	// Session errors
	ErrCodeSessionNotFound     ErrorCode = "SESSION_NOT_FOUND"
	ErrCodeSessionExists       ErrorCode = "SESSION_ALREADY_EXISTS"
	ErrCodeSessionInvalid      ErrorCode = "SESSION_INVALID"
	ErrCodeSessionLimitReached ErrorCode = "SESSION_LIMIT_REACHED"

	// History errors
	ErrCodeHistoryNotFound    ErrorCode = "HISTORY_NOT_FOUND"
	ErrCodeHistoryEmpty       ErrorCode = "HISTORY_EMPTY"
	ErrCodeHistoryCorrupted   ErrorCode = "HISTORY_CORRUPTED"
	ErrCodeHistoryWriteFailed ErrorCode = "HISTORY_WRITE_FAILED"
	ErrCodeHistoryReadFailed  ErrorCode = "HISTORY_READ_FAILED"

	// Command errors
	ErrCodeCommandNotFound   ErrorCode = "COMMAND_NOT_FOUND"
	ErrCodeCommandBlocked    ErrorCode = "COMMAND_BLOCKED"
	ErrCodeCommandTimeout    ErrorCode = "COMMAND_TIMEOUT"
	ErrCodeCommandFailed     ErrorCode = "COMMAND_FAILED"
	ErrCodeCommandValidation ErrorCode = "COMMAND_VALIDATION_FAILED"

	// Process errors
	ErrCodeProcessNotFound     ErrorCode = "PROCESS_NOT_FOUND"
	ErrCodeProcessLimitReached ErrorCode = "PROCESS_LIMIT_REACHED"
	ErrCodeProcessStartFailed  ErrorCode = "PROCESS_START_FAILED"
	ErrCodeProcessTerminated   ErrorCode = "PROCESS_TERMINATED"

	// Resource errors
	ErrCodeRateLimited       ErrorCode = "RATE_LIMITED"
	ErrCodeResourceExhausted ErrorCode = "RESOURCE_EXHAUSTED"
	ErrCodeDatabaseError     ErrorCode = "DATABASE_ERROR"
	ErrCodeFileSystemError   ErrorCode = "FILESYSTEM_ERROR"

	// Validation errors
	ErrCodeInvalidInput    ErrorCode = "INVALID_INPUT"
	ErrCodeMissingRequired ErrorCode = "MISSING_REQUIRED_FIELD"
	ErrCodeInvalidFormat   ErrorCode = "INVALID_FORMAT"

	// Internal errors
	ErrCodeInternal       ErrorCode = "INTERNAL_ERROR"
	ErrCodeNotImplemented ErrorCode = "NOT_IMPLEMENTED"
)

// TerminalError is the standardized error type for the application
type TerminalError struct {
	Code       ErrorCode      `json:"code"`
	Message    string         `json:"message"`
	Details    string         `json:"details,omitempty"`
	Context    map[string]any `json:"context,omitempty"`
	Cause      error          `json:"-"`
	Retryable  bool           `json:"retryable"`
	Suggestion string         `json:"suggestion,omitempty"`
}

// Error implements the error interface
func (e *TerminalError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap allows errors.Is and errors.As to work with the underlying cause
func (e *TerminalError) Unwrap() error {
	return e.Cause
}

// WithContext adds context information to the error
func (e *TerminalError) WithContext(key string, value any) *TerminalError {
	if e.Context == nil {
		e.Context = make(map[string]any)
	}
	e.Context[key] = value
	return e
}

// WithSuggestion adds a suggestion for the user
func (e *TerminalError) WithSuggestion(suggestion string) *TerminalError {
	e.Suggestion = suggestion
	return e
}

// WithDetails adds detailed information
func (e *TerminalError) WithDetails(details string) *TerminalError {
	e.Details = details
	return e
}

// New creates a new TerminalError
func New(code ErrorCode, message string) *TerminalError {
	return &TerminalError{
		Code:    code,
		Message: message,
	}
}

// Wrap wraps an existing error with additional context
func Wrap(cause error, code ErrorCode, message string) *TerminalError {
	return &TerminalError{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

// Is checks if the error matches the given error code
func Is(err error, code ErrorCode) bool {
	var termErr *TerminalError
	if errors.As(err, &termErr) {
		return termErr.Code == code
	}
	return false
}

// GetCode extracts the error code from an error
func GetCode(err error) ErrorCode {
	var termErr *TerminalError
	if errors.As(err, &termErr) {
		return termErr.Code
	}
	return ErrCodeInternal
}

// IsRetryable checks if the error is retryable
func IsRetryable(err error) bool {
	var termErr *TerminalError
	if errors.As(err, &termErr) {
		return termErr.Retryable
	}
	return false
}

// --- Convenience constructors for common errors ---

// SessionNotFound creates a session not found error
func SessionNotFound(sessionID string) *TerminalError {
	return New(ErrCodeSessionNotFound, fmt.Sprintf("session not found: %s", sessionID)).
		WithContext("session_id", sessionID).
		WithSuggestion("Use list_terminal_sessions to see available sessions")
}

// SessionExists creates a session already exists error
func SessionExists(sessionID string) *TerminalError {
	return New(ErrCodeSessionExists, fmt.Sprintf("session already exists: %s", sessionID)).
		WithContext("session_id", sessionID).
		WithSuggestion("Use a different session name or delete the existing session first")
}

// HistoryNotFound creates a history not found error
func HistoryNotFound(sessionID string) *TerminalError {
	return New(ErrCodeHistoryNotFound, fmt.Sprintf("no history found for session: %s", sessionID)).
		WithContext("session_id", sessionID).
		WithSuggestion("Run some commands first to create history")
}

// HistoryWriteFailed creates a history write failure error
func HistoryWriteFailed(cause error, sessionID string) *TerminalError {
	return Wrap(cause, ErrCodeHistoryWriteFailed, "failed to save command history").
		WithContext("session_id", sessionID).
		WithDetails("Check disk space and file permissions").
		WithSuggestion("Ensure the data directory is writable")
}

// HistoryReadFailed creates a history read failure error
func HistoryReadFailed(cause error, path string) *TerminalError {
	return Wrap(cause, ErrCodeHistoryReadFailed, "failed to read history file").
		WithContext("path", path).
		WithSuggestion("The history file may be corrupted. Consider deleting it.")
}

// CommandBlocked creates a blocked command error
func CommandBlocked(command, reason string) *TerminalError {
	return New(ErrCodeCommandBlocked, fmt.Sprintf("command blocked for security: %s", reason)).
		WithContext("command", command).
		WithSuggestion("Use safer alternative commands or request admin override")
}

// CommandTimeout creates a command timeout error
func CommandTimeout(command string, timeout int) *TerminalError {
	return New(ErrCodeCommandTimeout, fmt.Sprintf("command timed out after %d seconds", timeout)).
		WithContext("command", command).
		WithContext("timeout_seconds", timeout).
		WithSuggestion("Use run_background_process for long-running commands")
}

// RateLimited creates a rate limit error
func RateLimited(retryAfter int) *TerminalError {
	err := New(ErrCodeRateLimited, "rate limit exceeded")
	err.Retryable = true
	err.Context = map[string]any{"retry_after_seconds": retryAfter}
	err.Suggestion = fmt.Sprintf("Wait %d seconds before retrying", retryAfter)
	return err
}

// ProcessNotFound creates a process not found error
func ProcessNotFound(processID string) *TerminalError {
	return New(ErrCodeProcessNotFound, fmt.Sprintf("background process not found: %s", processID)).
		WithContext("process_id", processID).
		WithSuggestion("Use list_background_processes to see running processes")
}

// ProcessLimitReached creates a process limit error
func ProcessLimitReached(current, max int) *TerminalError {
	return New(ErrCodeProcessLimitReached, fmt.Sprintf("maximum background processes reached: %d/%d", current, max)).
		WithContext("current", current).
		WithContext("max", max).
		WithSuggestion("Terminate some background processes before starting new ones")
}

// InvalidInput creates an invalid input error
func InvalidInput(field, reason string) *TerminalError {
	return New(ErrCodeInvalidInput, fmt.Sprintf("invalid input for %s: %s", field, reason)).
		WithContext("field", field)
}

// MissingRequired creates a missing required field error
func MissingRequired(field string) *TerminalError {
	return New(ErrCodeMissingRequired, fmt.Sprintf("required field missing: %s", field)).
		WithContext("field", field)
}

// DatabaseError creates a database error
func DatabaseError(cause error, operation string) *TerminalError {
	return Wrap(cause, ErrCodeDatabaseError, fmt.Sprintf("database operation failed: %s", operation)).
		WithContext("operation", operation).
		WithSuggestion("Check database connection and try again")
}

// FileSystemError creates a filesystem error
func FileSystemError(cause error, path string) *TerminalError {
	return Wrap(cause, ErrCodeFileSystemError, "filesystem operation failed").
		WithContext("path", path).
		WithSuggestion("Check file permissions and disk space")
}

// InternalError creates an internal error
func InternalError(cause error, details string) *TerminalError {
	return Wrap(cause, ErrCodeInternal, "internal error occurred").
		WithDetails(details).
		WithSuggestion("Please report this issue if it persists")
}
