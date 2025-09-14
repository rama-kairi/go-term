package tools

import (
	"fmt"
	"strings"

	"github.com/rama-kairi/go-term/internal/config"
	"github.com/rama-kairi/go-term/internal/database"
	"github.com/rama-kairi/go-term/internal/logger"
	"github.com/rama-kairi/go-term/internal/terminal"
	"github.com/rama-kairi/go-term/internal/utils"
)

// TerminalTools contains all MCP tools for terminal management with enhanced features
type TerminalTools struct {
	manager        *terminal.Manager
	config         *config.Config
	logger         *logger.Logger
	database       *database.DB
	security       *SecurityValidator
	projectGen     *utils.ProjectIDGenerator
	packageManager *utils.PackageManagerDetector
}

// NewTerminalTools creates a new instance of terminal tools with enhanced features
func NewTerminalTools(manager *terminal.Manager, cfg *config.Config, logger *logger.Logger, db *database.DB) *TerminalTools {
	return &TerminalTools{
		manager:        manager,
		config:         cfg,
		logger:         logger,
		database:       db,
		security:       NewSecurityValidator(cfg),
		projectGen:     utils.NewProjectIDGenerator(),
		packageManager: utils.NewPackageManagerDetector(),
	}
}

// SecurityValidator provides command security validation
type SecurityValidator struct {
	config *config.Config
}

// NewSecurityValidator creates a new security validator
func NewSecurityValidator(cfg *config.Config) *SecurityValidator {
	return &SecurityValidator{config: cfg}
}

// ValidateCommand validates a command against security policies
func (s *SecurityValidator) ValidateCommand(command string) error {
	if command == "" {
		return fmt.Errorf("command cannot be empty")
	}

	if len(command) > s.config.Session.MaxCommandLength {
		return fmt.Errorf("command cannot exceed %d characters", s.config.Session.MaxCommandLength)
	}

	// Check for blocked commands using word boundaries to avoid false positives
	lowerCommand := strings.ToLower(strings.TrimSpace(command))

	// Split command into words for more precise validation
	commandWords := strings.Fields(lowerCommand)

	for _, blocked := range s.config.Security.BlockedCommands {
		blockedLower := strings.ToLower(blocked)

		// Check if any word in the command matches the blocked command exactly
		for _, word := range commandWords {
			// Remove common shell operators to get the actual command
			cleanWord := strings.Trim(word, ";&|(){}[]<>\"'`")

			if cleanWord == blockedLower {
				return fmt.Errorf("command contains blocked operation: %s", blocked)
			}
		}

		// Also check for blocked patterns that might contain spaces or special operators
		// using regex word boundaries for patterns like "rm -rf /"
		if strings.Contains(blockedLower, " ") || strings.ContainsAny(blockedLower, "-/") {
			if strings.Contains(lowerCommand, blockedLower) {
				return fmt.Errorf("command contains blocked operation: %s", blocked)
			}
		}
	}

	// Additional security checks
	if s.config.Security.EnableSandbox {
		// Check for potentially dangerous patterns
		dangerousPatterns := []string{
			"rm -rf /",
			"dd if=/dev",
			"mkfs",
			"fdisk",
			":(){ :|:& };:",
			"> /dev/",
			"chmod 777",
			"chown root",
		}

		for _, pattern := range dangerousPatterns {
			if strings.Contains(lowerCommand, pattern) {
				return fmt.Errorf("command contains potentially dangerous pattern: %s", pattern)
			}
		}

		// Check for network access if not allowed
		if !s.config.Security.AllowNetworkAccess {
			networkCommands := []string{"wget", "curl", "ssh", "scp", "rsync", "nc", "netcat", "telnet"}
			for _, netCmd := range networkCommands {
				if strings.Contains(lowerCommand, netCmd) {
					return fmt.Errorf("network access not allowed: %s", netCmd)
				}
			}
		}

		// Check for file system write operations if not allowed
		if !s.config.Security.AllowFileSystemWrite {
			writeCommands := []string{"rm ", "mv ", "cp ", "touch ", "mkdir ", "rmdir "}
			for _, writeCmd := range writeCommands {
				if strings.Contains(lowerCommand, writeCmd) {
					return fmt.Errorf("file system write operations not allowed: %s", strings.TrimSpace(writeCmd))
				}
			}
		}
	}

	return nil
}

// ===== TYPE DEFINITIONS =====

// CreateSessionArgs represents arguments for creating a terminal session (simplified)
type CreateSessionArgs struct {
	Name string `json:"name" jsonschema:"required,description=Simple descriptive name for the terminal session"`
}

// CreateSessionResult represents the result of creating a terminal session with project info
type CreateSessionResult struct {
	SessionID    string                      `json:"session_id"`
	Name         string                      `json:"name"`
	ProjectID    string                      `json:"project_id"`
	WorkingDir   string                      `json:"working_dir"`
	Message      string                      `json:"message"`
	ProjectInfo  utils.ProjectIDInfo         `json:"project_info"`
	Instructions utils.ProjectIDInstructions `json:"instructions"`
}

// ListSessionsArgs represents arguments for listing terminal sessions (no args needed)
type ListSessionsArgs struct{}

// SessionInfo represents comprehensive session information for listing
type SessionInfo struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	ProjectID     string            `json:"project_id"`
	WorkingDir    string            `json:"working_dir"`
	Environment   map[string]string `json:"environment,omitempty"`
	CreatedAt     string            `json:"created_at"`
	LastUsedAt    string            `json:"last_used_at"`
	IsActive      bool              `json:"is_active"`
	CommandCount  int               `json:"command_count"`
	SuccessCount  int               `json:"success_count"`
	SuccessRate   float64           `json:"success_rate"`
	TotalDuration string            `json:"total_duration"`
}

// ListSessionsResult represents the enhanced result of listing terminal sessions
type ListSessionsResult struct {
	Sessions     []SessionInfo             `json:"sessions"`
	Count        int                       `json:"count"`
	Statistics   terminal.SessionStats     `json:"statistics"`
	ProjectStats map[string]ProjectSummary `json:"project_stats"`
}

// ProjectSummary provides a summary of sessions per project
type ProjectSummary struct {
	ProjectID     string `json:"project_id"`
	SessionCount  int    `json:"session_count"`
	TotalCommands int    `json:"total_commands"`
}

// DeleteSessionArgs represents arguments for deleting sessions
type DeleteSessionArgs struct {
	SessionID string `json:"session_id,omitempty" jsonschema:"description,The UUID4 identifier of the session to delete. Leave empty to delete by project."`
	ProjectID string `json:"project_id,omitempty" jsonschema:"description,Delete all sessions for this project ID. Leave empty to delete by session ID."`
	Confirm   bool   `json:"confirm" jsonschema:"required,description,Confirmation flag to prevent accidental deletion. Must be set to true."`
}

// DeleteSessionResult represents the result of session deletion
type DeleteSessionResult struct {
	Success         bool   `json:"success"`
	SessionsDeleted int    `json:"sessions_deleted"`
	Message         string `json:"message"`
	ProjectID       string `json:"project_id,omitempty"`
	SessionID       string `json:"session_id,omitempty"`
}

// RunCommandArgs represents arguments for running a foreground command
type RunCommandArgs struct {
	SessionID string `json:"session_id" jsonschema:"required,description=The UUID4 identifier of the terminal session to run the command in. Use list_terminal_sessions to see available sessions."`
	Command   string `json:"command" jsonschema:"required,description=The command to execute in the terminal session. Will be validated for security before execution. Directory changes (cd) persist across commands. This tool only runs foreground commands - use run_background_process for long-running processes."`
}

// RunCommandResult represents the result of running a foreground command
type RunCommandResult struct {
	SessionID      string `json:"session_id"`                // Session identifier
	ProjectID      string `json:"project_id"`                // Project identifier
	Command        string `json:"command"`                   // The executed command
	Output         string `json:"output"`                    // Standard output
	ErrorOutput    string `json:"error_output,omitempty"`    // Error output if any
	Success        bool   `json:"success"`                   // Whether command succeeded
	ExitCode       int    `json:"exit_code"`                 // Exit code from command
	Duration       string `json:"duration"`                  // Time taken to execute
	WorkingDir     string `json:"working_dir"`               // Working directory during execution
	CommandCount   int    `json:"command_count"`             // Total commands run in session
	HistoryID      string `json:"history_id"`                // ID for this command in history
	StreamingUsed  bool   `json:"streaming_used"`            // Whether real-time streaming was used
	TotalChunks    int    `json:"total_chunks,omitempty"`    // Number of stream chunks if streaming was used
	PackageManager string `json:"package_manager,omitempty"` // Detected package manager used
	ProjectType    string `json:"project_type,omitempty"`    // Detected project type
}

// CheckBackgroundProcessArgs represents arguments for checking background process status
type CheckBackgroundProcessArgs struct {
	SessionID string `json:"session_id" jsonschema:"required,description,The UUID4 identifier of the session running the background process."`
	ProcessID string `json:"process_id,omitempty" jsonschema:"description,Optional background process ID. If not provided will check the latest background process for the session."`
}

// CheckBackgroundProcessResult represents the result of checking a background process
type CheckBackgroundProcessResult struct {
	SessionID   string `json:"session_id"`
	ProcessID   string `json:"process_id"`
	IsRunning   bool   `json:"is_running"`
	Output      string `json:"output"`
	ErrorOutput string `json:"error_output"`
	StartTime   string `json:"start_time"`
	Duration    string `json:"duration"`
	Command     string `json:"command"`
	PID         int    `json:"pid,omitempty"`
	Status      string `json:"status"` // "running", "completed", "failed", "not_found"
	LastChecked string `json:"last_checked"`
}

// RunBackgroundProcessArgs represents arguments for running a background process
type RunBackgroundProcessArgs struct {
	SessionID string `json:"session_id" jsonschema:"required,description=The UUID4 identifier of the terminal session to run the background process in. Use list_terminal_sessions to see available sessions."`
	Command   string `json:"command" jsonschema:"required,description=The command to execute as a background process. No validation is performed - the agent decides what to run."`
}

// RunBackgroundProcessResult represents the result of starting a background process
type RunBackgroundProcessResult struct {
	SessionID         string `json:"session_id"`
	ProjectID         string `json:"project_id"`
	ProcessID         string `json:"process_id"`
	Command           string `json:"command"`
	StartTime         string `json:"start_time"`
	WorkingDir        string `json:"working_dir"`
	Success           bool   `json:"success"`
	Message           string `json:"message"`
	BackgroundCount   int    `json:"background_count"`
	MaxBackgroundProc int    `json:"max_background_processes"`
}

// ListBackgroundProcessesArgs represents arguments for listing background processes
type ListBackgroundProcessesArgs struct {
	SessionID string `json:"session_id,omitempty" jsonschema:"description=Optional: Filter by specific session ID. Leave empty to list all background processes across all sessions."`
	ProjectID string `json:"project_id,omitempty" jsonschema:"description=Optional: Filter by specific project ID. Leave empty to list all background processes across all projects."`
}

// BackgroundProcessInfo represents information about a background process
type BackgroundProcessInfo struct {
	ProcessID   string `json:"process_id"`
	SessionID   string `json:"session_id"`
	SessionName string `json:"session_name"`
	ProjectID   string `json:"project_id"`
	Command     string `json:"command"`
	PID         int    `json:"pid"`
	StartTime   string `json:"start_time"`
	Duration    string `json:"duration"`
	IsRunning   bool   `json:"is_running"`
	ExitCode    int    `json:"exit_code,omitempty"`
	WorkingDir  string `json:"working_dir"`
	OutputSize  int    `json:"output_size"`
	ErrorSize   int    `json:"error_size"`
}

// ListBackgroundProcessesResult represents the result of listing background processes
type ListBackgroundProcessesResult struct {
	Processes      []BackgroundProcessInfo `json:"processes"`
	TotalCount     int                     `json:"total_count"`
	RunningCount   int                     `json:"running_count"`
	CompletedCount int                     `json:"completed_count"`
	SessionStats   map[string]int          `json:"session_stats"`
	ProjectStats   map[string]int          `json:"project_stats"`
	Summary        string                  `json:"summary"`
}

// TerminateBackgroundProcessArgs represents arguments for terminating a background process
type TerminateBackgroundProcessArgs struct {
	SessionID string `json:"session_id" jsonschema:"required,description=The UUID4 identifier of the session containing the background process."`
	ProcessID string `json:"process_id" jsonschema:"required,description=The UUID4 identifier of the background process to terminate."`
	Force     bool   `json:"force,omitempty" jsonschema:"description=Whether to force kill the process (SIGKILL) instead of graceful termination (SIGTERM). Default: false."`
}

// TerminateBackgroundProcessResult represents the result of terminating a background process
type TerminateBackgroundProcessResult struct {
	SessionID   string `json:"session_id"`
	ProcessID   string `json:"process_id"`
	Command     string `json:"command"`
	PID         int    `json:"pid"`
	WasRunning  bool   `json:"was_running"`
	Terminated  bool   `json:"terminated"`
	Force       bool   `json:"force"`
	Message     string `json:"message"`
	FinalOutput string `json:"final_output,omitempty"`
	FinalError  string `json:"final_error,omitempty"`
}

// SearchHistoryArgs represents arguments for searching command history
type SearchHistoryArgs struct {
	SessionID     string   `json:"session_id,omitempty" jsonschema:"description,Filter by specific session ID. Leave empty to search all sessions."`
	ProjectID     string   `json:"project_id,omitempty" jsonschema:"description,Filter by specific project ID. Leave empty to search all projects."`
	Command       string   `json:"command,omitempty" jsonschema:"description,Search for commands containing this text (case-insensitive partial match)."`
	Output        string   `json:"output,omitempty" jsonschema:"description,Search for commands with output containing this text (case-insensitive partial match)."`
	Success       *bool    `json:"success,omitempty" jsonschema:"description,Filter by success status: true for successful commands false for failed commands omit for all."`
	StartTime     string   `json:"start_time,omitempty" jsonschema:"description,Find commands executed after this time (ISO 8601 format: 2006-01-02T15:04:05Z)."`
	EndTime       string   `json:"end_time,omitempty" jsonschema:"description,Find commands executed before this time (ISO 8601 format: 2006-01-02T15:04:05Z)."`
	WorkingDir    string   `json:"working_dir,omitempty" jsonschema:"description,Filter by working directory path (partial match)."`
	Tags          []string `json:"tags,omitempty" jsonschema:"description,Filter by tags (commands must have all specified tags)."`
	Limit         int      `json:"limit,omitempty" jsonschema:"description,Maximum number of results to return (default: 100 max: 1000)."`
	SortBy        string   `json:"sort_by,omitempty" jsonschema:"description,Sort results by: 'time' (default) 'duration' or 'command'."`
	SortDesc      bool     `json:"sort_desc,omitempty" jsonschema:"description,Sort in descending order (default: true for time-based sorting)."`
	IncludeOutput bool     `json:"include_output,omitempty" jsonschema:"description,Include command output in results (default: false to reduce response size)."`
}

// SearchHistoryResult represents the result of searching command history
type SearchHistoryResult struct {
	TotalFound   int                       `json:"total_found"`
	Results      []*database.CommandResult `json:"results"`
	Query        SearchHistoryArgs         `json:"query"`
	SearchTime   string                    `json:"search_time"`
	ProjectStats map[string]int            `json:"project_stats"` // project_id -> command_count in results
	SessionStats map[string]int            `json:"session_stats"` // session_id -> command_count in results
	Instructions SearchInstructions        `json:"instructions"`
}

// SearchInstructions provides guidance on how to use the search functionality
type SearchInstructions struct {
	Description string          `json:"description"`
	Examples    []SearchExample `json:"examples"`
	Tips        []string        `json:"tips"`
	Limits      SearchLimits    `json:"limits"`
}

// SearchExample shows example search queries
type SearchExample struct {
	Description string            `json:"description"`
	Query       SearchHistoryArgs `json:"query"`
}

// SearchLimits defines the limits for search queries
type SearchLimits struct {
	MaxResults     int    `json:"max_results"`
	DefaultResults int    `json:"default_results"`
	TimeFormat     string `json:"time_format"`
}
