package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rama-kairi/go-term/internal/config"
	"github.com/rama-kairi/go-term/internal/database"
	"github.com/rama-kairi/go-term/internal/logger"
	"github.com/rama-kairi/go-term/internal/terminal"
	"github.com/rama-kairi/go-term/internal/utils"
)

// TerminalTools contains all MCP tools for terminal management with enhanced features
type TerminalTools struct {
	manager    *terminal.Manager
	config     *config.Config
	logger     *logger.Logger
	database   *database.DB
	security   *SecurityValidator
	projectGen *utils.ProjectIDGenerator
}

// NewTerminalTools creates a new instance of terminal tools with enhanced features
func NewTerminalTools(manager *terminal.Manager, cfg *config.Config, logger *logger.Logger, db *database.DB) *TerminalTools {
	return &TerminalTools{
		manager:    manager,
		config:     cfg,
		logger:     logger,
		database:   db,
		security:   NewSecurityValidator(cfg),
		projectGen: utils.NewProjectIDGenerator(),
	}
}

// CreateSessionArgs represents arguments for creating a terminal session with project support
type CreateSessionArgs struct {
	Name       string `json:"name" jsonschema:"required,description,The name of the terminal session to create. Should be descriptive and meaningful for your project."`
	ProjectID  string `json:"project_id,omitempty" jsonschema:"description,Optional project ID to associate with this session. If not provided will be auto-generated based on current directory. Format: folder_name_with_underscores_RANDOM (e.g. my_project_a7b3c9)"`
	WorkingDir string `json:"working_dir,omitempty" jsonschema:"description,Optional working directory for the session. If not provided uses current directory. This affects project ID generation."`
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

// CreateSession creates a new terminal session with project association and comprehensive documentation
func (t *TerminalTools) CreateSession(ctx context.Context, req *mcp.CallToolRequest, args CreateSessionArgs) (*mcp.CallToolResult, CreateSessionResult, error) {
	// Validate session name
	if err := validateSessionName(args.Name); err != nil {
		return createErrorResult(fmt.Sprintf("Invalid session name: %v", err)), CreateSessionResult{}, nil
	}

	// Validate project ID if provided
	if args.ProjectID != "" {
		if err := t.projectGen.ValidateProjectID(args.ProjectID); err != nil {
			return createErrorResult(fmt.Sprintf("Invalid project ID: %v", err)), CreateSessionResult{}, nil
		}
	}

	session, err := t.manager.CreateSession(args.Name, args.ProjectID, args.WorkingDir)
	if err != nil {
		t.logger.Error("Failed to create session", err, map[string]interface{}{
			"session_name": args.Name,
			"project_id":   args.ProjectID,
			"working_dir":  args.WorkingDir,
		})
		return createErrorResult(fmt.Sprintf("Failed to create session: %v", err)), CreateSessionResult{}, nil
	}

	// Parse project ID for detailed information
	projectInfo := t.projectGen.ParseProjectID(session.ProjectID)
	instructions := t.projectGen.GetProjectIDInstructions()

	result := CreateSessionResult{
		SessionID:    session.ID,
		Name:         session.Name,
		ProjectID:    session.ProjectID,
		WorkingDir:   session.WorkingDir,
		Message:      fmt.Sprintf("Terminal session '%s' created successfully with ID: %s in project: %s", session.Name, session.ID, session.ProjectID),
		ProjectInfo:  projectInfo,
		Instructions: instructions,
	}

	// Create comprehensive response with usage instructions
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	content := []mcp.Content{
		&mcp.TextContent{
			Text: string(resultJSON),
		},
	}

	t.logger.Info("Session created successfully", map[string]interface{}{
		"session_id":  session.ID,
		"project_id":  session.ProjectID,
		"working_dir": session.WorkingDir,
	})

	return &mcp.CallToolResult{
		Content: content,
		IsError: false,
	}, result, nil
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

// ListSessions lists all terminal sessions with enhanced information and statistics
func (t *TerminalTools) ListSessions(ctx context.Context, req *mcp.CallToolRequest, args ListSessionsArgs) (*mcp.CallToolResult, ListSessionsResult, error) {
	sessions := t.manager.ListSessions()
	stats := t.manager.GetSessionStats()

	sessionInfos := make([]SessionInfo, len(sessions))
	projectStats := make(map[string]ProjectSummary)

	for i, session := range sessions {
		successRate := 0.0
		if session.CommandCount > 0 {
			successRate = float64(session.SuccessCount) / float64(session.CommandCount)
		}

		sessionInfos[i] = SessionInfo{
			ID:            session.ID,
			Name:          session.Name,
			ProjectID:     session.ProjectID,
			WorkingDir:    session.WorkingDir,
			CreatedAt:     session.CreatedAt.Format("2006-01-02 15:04:05"),
			LastUsedAt:    session.LastUsedAt.Format("2006-01-02 15:04:05"),
			IsActive:      session.IsActive,
			CommandCount:  session.CommandCount,
			SuccessCount:  session.SuccessCount,
			SuccessRate:   successRate,
			TotalDuration: session.TotalDuration.String(),
		}

		// Update project statistics
		if summary, exists := projectStats[session.ProjectID]; exists {
			summary.SessionCount++
			summary.TotalCommands += session.CommandCount
			projectStats[session.ProjectID] = summary
		} else {
			projectStats[session.ProjectID] = ProjectSummary{
				ProjectID:     session.ProjectID,
				SessionCount:  1,
				TotalCommands: session.CommandCount,
			}
		}
	}

	result := ListSessionsResult{
		Sessions:     sessionInfos,
		Count:        len(sessionInfos),
		Statistics:   stats,
		ProjectStats: projectStats,
	}

	// Create comprehensive response
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	content := []mcp.Content{
		&mcp.TextContent{
			Text: string(resultJSON),
		},
	}

	return &mcp.CallToolResult{
		Content: content,
		IsError: false,
	}, result, nil
}

// RunCommandArgs represents arguments for running a command with enhanced validation
type RunCommandArgs struct {
	SessionID string `json:"session_id" jsonschema:"required,description,The UUID4 identifier of the terminal session to run the command in. Use list_terminal_sessions to see available sessions."`
	Command   string `json:"command" jsonschema:"required,description,The command to execute in the terminal session. Will be validated for security before execution."`
}

// RunCommandResult represents the enhanced result of running a command with detailed information
type RunCommandResult struct {
	SessionID     string `json:"session_id"`             // Session identifier
	ProjectID     string `json:"project_id"`             // Project identifier
	Command       string `json:"command"`                // The executed command
	Output        string `json:"output"`                 // Standard output
	ErrorOutput   string `json:"error_output,omitempty"` // Error output if any
	Success       bool   `json:"success"`                // Whether command succeeded
	ExitCode      int    `json:"exit_code"`              // Exit code from command
	Duration      string `json:"duration"`               // Time taken to execute
	WorkingDir    string `json:"working_dir"`            // Working directory during execution
	CommandCount  int    `json:"command_count"`          // Total commands run in session
	HistoryID     string `json:"history_id"`             // ID for this command in history
	StreamingUsed bool   `json:"streaming_used"`         // Whether real-time streaming was used
	TotalChunks   int    `json:"total_chunks,omitempty"` // Number of stream chunks if streaming was used
}

// RunCommand executes a command in the specified terminal session with comprehensive tracking
func (t *TerminalTools) RunCommand(ctx context.Context, req *mcp.CallToolRequest, args RunCommandArgs) (*mcp.CallToolResult, RunCommandResult, error) {
	// Validate input
	if err := validateSessionID(args.SessionID); err != nil {
		return createErrorResult(fmt.Sprintf("Invalid session ID: %v", err)), RunCommandResult{}, nil
	}

	if err := t.security.ValidateCommand(args.Command); err != nil {
		t.logger.LogSecurityEvent("command_blocked", fmt.Sprintf("Command blocked: %s", args.Command), "medium", map[string]interface{}{
			"session_id": args.SessionID,
			"command":    args.Command,
			"reason":     err.Error(),
		})
		return createErrorResult(fmt.Sprintf("Command blocked for security reasons: %v", err)), RunCommandResult{}, nil
	}

	// Verify session exists
	session, err := t.manager.GetSession(args.SessionID)
	if err != nil {
		return createErrorResult(fmt.Sprintf("Session not found: %v", err)), RunCommandResult{}, nil
	}

	// Execute the command with streaming support if enabled
	startTime := time.Now()
	var output, errorOutput string
	var success bool
	var exitCode int
	var totalChunks int
	streamingUsed := false

	// Check if streaming is enabled and use it for better real-time experience
	if t.config.Streaming.Enable {
		streamingUsed = true

		// Use the manager's streaming command execution to maintain session state
		streamOutput, streamErr := t.manager.ExecuteCommandWithStreaming(args.SessionID, args.Command)

		success = streamErr == nil
		exitCode = 0
		output = streamOutput
		totalChunks = 1 // Basic implementation, will be enhanced when full streaming is integrated

		if streamErr != nil {
			errorOutput = streamErr.Error()
			exitCode = 1
			success = false
		}

		t.logger.Info("Command executed with streaming", map[string]interface{}{
			"session_id": args.SessionID,
			"command":    args.Command,
			"success":    success,
			"streaming":  true,
			"duration":   time.Since(startTime).String(),
		})
	} else {
		// Fall back to traditional execution
		output, err = t.manager.ExecuteCommand(args.SessionID, args.Command)
		success = err == nil
		exitCode = 0

		if err != nil {
			errorOutput = err.Error()
			exitCode = 1
		}

		t.logger.Info("Command executed traditionally", map[string]interface{}{
			"session_id": args.SessionID,
			"command":    args.Command,
			"success":    success,
			"duration":   time.Since(startTime).String(),
		})
	}

	duration := time.Since(startTime)

	// Get updated session info
	updatedSession, _ := t.manager.GetSession(args.SessionID)
	commandCount := 0
	if updatedSession != nil {
		commandCount = updatedSession.CommandCount
	}

	result := RunCommandResult{
		SessionID:     args.SessionID,
		ProjectID:     session.ProjectID,
		Command:       args.Command,
		Output:        output,
		ErrorOutput:   errorOutput,
		Success:       success,
		ExitCode:      exitCode,
		Duration:      duration.String(),
		WorkingDir:    session.WorkingDir,
		CommandCount:  commandCount,
		HistoryID:     fmt.Sprintf("%s_%d", args.SessionID[:8], commandCount),
		StreamingUsed: streamingUsed,
		TotalChunks:   totalChunks,
	}

	// Create response
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	content := []mcp.Content{
		&mcp.TextContent{
			Text: string(resultJSON),
		},
	}

	t.logger.Info("Command executed", map[string]interface{}{
		"session_id": args.SessionID,
		"project_id": session.ProjectID,
		"success":    success,
		"duration":   duration.String(),
	})

	return &mcp.CallToolResult{
		Content: content,
		IsError: false,
	}, result, nil
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
	Results      []*database.CommandRecord `json:"results"`
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

// SearchHistory searches through command history across all sessions and projects
func (t *TerminalTools) SearchHistory(ctx context.Context, req *mcp.CallToolRequest, args SearchHistoryArgs) (*mcp.CallToolResult, SearchHistoryResult, error) {
	startTime := time.Now()

	// Parse time filters if provided
	var startTimeFilter, endTimeFilter time.Time
	if args.StartTime != "" {
		if t, err := time.Parse(time.RFC3339, args.StartTime); err == nil {
			startTimeFilter = t
		} else {
			return createErrorResult(fmt.Sprintf("Invalid start_time format. Use ISO 8601 format: %s", time.RFC3339)), SearchHistoryResult{}, nil
		}
	}

	if args.EndTime != "" {
		if t, err := time.Parse(time.RFC3339, args.EndTime); err == nil {
			endTimeFilter = t
		} else {
			return createErrorResult(fmt.Sprintf("Invalid end_time format. Use ISO 8601 format: %s", time.RFC3339)), SearchHistoryResult{}, nil
		}
	}

	// Apply default limits
	limit := args.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	// Execute database search
	commands, err := t.database.SearchCommands(
		args.SessionID,
		args.ProjectID,
		args.Command,
		args.Output,
		args.Success,
		startTimeFilter,
		endTimeFilter,
		limit,
	)
	if err != nil {
		t.logger.Error("Failed to search command history", err, map[string]interface{}{
			"query": args,
		})
		return createErrorResult(fmt.Sprintf("Search failed: %v", err)), SearchHistoryResult{}, nil
	}

	// Calculate stats
	projectStats := make(map[string]int)
	sessionStats := make(map[string]int)
	for _, cmd := range commands {
		projectStats[cmd.ProjectID]++
		sessionStats[cmd.SessionID]++
	}

	result := SearchHistoryResult{
		TotalFound:   len(commands),
		Results:      commands,
		Query:        args,
		SearchTime:   time.Since(startTime).String(),
		ProjectStats: projectStats,
		SessionStats: sessionStats,
		Instructions: getSearchInstructions(),
	}

	t.logger.Info("Command history search completed", map[string]interface{}{
		"results_count": len(commands),
		"search_time":   time.Since(startTime).String(),
		"session_id":    args.SessionID,
		"project_id":    args.ProjectID,
	})

	return createJSONResult(result), result, nil
}

// getSearchInstructions returns comprehensive search instructions and examples
func getSearchInstructions() SearchInstructions {
	return SearchInstructions{
		Description: "Search through command history across all terminal sessions and projects. Use filters to narrow down results and find specific commands or outputs.",
		Examples: []SearchExample{
			{
				Description: "Find all failed commands in the last day",
				Query: SearchHistoryArgs{
					Success:   boolPtr(false),
					StartTime: time.Now().Add(-24 * time.Hour).Format(time.RFC3339),
					Limit:     50,
				},
			},
			{
				Description: "Search for Docker commands in a specific project",
				Query: SearchHistoryArgs{
					Command:   "docker",
					ProjectID: "my_project_a7b3c9",
					Limit:     20,
				},
			},
			{
				Description: "Find commands that produced error output containing 'permission denied'",
				Query: SearchHistoryArgs{
					Output:        "permission denied",
					IncludeOutput: true,
					Success:       boolPtr(false),
				},
			},
		},
		Tips: []string{
			"Use partial text matching for both commands and output",
			"Combine multiple filters to narrow down results",
			"Use time filters to focus on recent activity",
			"Set include_output=true when searching by output content",
			"Use project_id to focus on specific projects",
			"Sort by duration to find long-running commands",
		},
		Limits: SearchLimits{
			MaxResults:     1000,
			DefaultResults: 100,
			TimeFormat:     time.RFC3339,
		},
	}
}

// boolPtr returns a pointer to a boolean value
func boolPtr(b bool) *bool {
	return &b
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

// Helper functions

// validateSessionName validates a session name
func validateSessionName(name string) error {
	if name == "" {
		return fmt.Errorf("session name cannot be empty")
	}

	if len(name) > 100 {
		return fmt.Errorf("session name cannot exceed 100 characters")
	}

	// Allow alphanumeric, spaces, hyphens, underscores
	validName := regexp.MustCompile(`^[a-zA-Z0-9 _-]+$`)
	if !validName.MatchString(name) {
		return fmt.Errorf("session name can only contain letters, numbers, spaces, hyphens, and underscores")
	}

	return nil
}

// validateSessionID validates a session ID format
func validateSessionID(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session ID cannot be empty")
	}

	// Basic UUID format validation
	uuidPattern := regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	if !uuidPattern.MatchString(sessionID) {
		return fmt.Errorf("session ID must be a valid UUID format")
	}

	return nil
}

// createJSONResult creates a JSON result for tool responses
func createJSONResult(data interface{}) *mcp.CallToolResult {
	resultJSON, _ := json.MarshalIndent(data, "", "  ")
	content := []mcp.Content{
		&mcp.TextContent{
			Text: string(resultJSON),
		},
	}

	return &mcp.CallToolResult{
		Content: content,
		IsError: false,
	}
}

// createErrorResult creates an error result for tool responses
func createErrorResult(errorMessage string) *mcp.CallToolResult {
	content := []mcp.Content{
		&mcp.TextContent{
			Text: fmt.Sprintf(`{"error": "%s"}`, errorMessage),
		},
	}

	return &mcp.CallToolResult{
		Content: content,
		IsError: true,
	}
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

// DeleteSession deletes terminal sessions (individual or project-wide) with confirmation
func (t *TerminalTools) DeleteSession(ctx context.Context, req *mcp.CallToolRequest, args DeleteSessionArgs) (*mcp.CallToolResult, DeleteSessionResult, error) {
	// Require confirmation
	if !args.Confirm {
		return createErrorResult("Deletion requires confirmation. Set 'confirm' to true."), DeleteSessionResult{}, nil
	}

	// Validate arguments - must specify either session_id or project_id, but not both
	if args.SessionID == "" && args.ProjectID == "" {
		return createErrorResult("Must specify either session_id or project_id"), DeleteSessionResult{}, nil
	}

	if args.SessionID != "" && args.ProjectID != "" {
		return createErrorResult("Cannot specify both session_id and project_id. Choose one."), DeleteSessionResult{}, nil
	}

	var deletedCount int
	var message string
	var err error

	if args.SessionID != "" {
		// Delete specific session
		if err := validateSessionID(args.SessionID); err != nil {
			return createErrorResult(fmt.Sprintf("Invalid session ID: %v", err)), DeleteSessionResult{}, nil
		}

		// Check if session exists before attempting to delete
		if !t.manager.SessionExists(args.SessionID) {
			return createErrorResult(fmt.Sprintf("Session not found: %s", args.SessionID)), DeleteSessionResult{}, nil
		}

		err = t.manager.DeleteSession(args.SessionID)
		if err != nil {
			t.logger.Error("Failed to delete session", err, map[string]interface{}{
				"session_id": args.SessionID,
			})
			return createErrorResult(fmt.Sprintf("Failed to delete session: %v", err)), DeleteSessionResult{}, nil
		}

		deletedCount = 1
		message = fmt.Sprintf("Successfully deleted session %s", args.SessionID)

		t.logger.LogSessionEvent("session_deleted", args.SessionID, "", map[string]interface{}{
			"deleted_by": "mcp_tool",
		})

	} else {
		// Delete all sessions for project
		if err := t.projectGen.ValidateProjectID(args.ProjectID); err != nil {
			return createErrorResult(fmt.Sprintf("Invalid project ID: %v", err)), DeleteSessionResult{}, nil
		}

		deletedSessions, err := t.manager.DeleteProjectSessions(args.ProjectID)
		if err != nil {
			t.logger.Error("Failed to delete project sessions", err, map[string]interface{}{
				"project_id": args.ProjectID,
			})
			return createErrorResult(fmt.Sprintf("Failed to delete project sessions: %v", err)), DeleteSessionResult{}, nil
		}

		deletedCount = len(deletedSessions)
		if deletedCount == 0 {
			message = fmt.Sprintf("No sessions found for project %s", args.ProjectID)
		} else {
			message = fmt.Sprintf("Successfully deleted %d sessions for project %s", deletedCount, args.ProjectID)
		}

		t.logger.LogSessionEvent("project_sessions_deleted", "", args.ProjectID, map[string]interface{}{
			"deleted_count": deletedCount,
			"deleted_by":    "mcp_tool",
		})
	}

	result := DeleteSessionResult{
		Success:         true,
		SessionsDeleted: deletedCount,
		Message:         message,
		ProjectID:       args.ProjectID,
		SessionID:       args.SessionID,
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return createErrorResult("Failed to marshal result"), DeleteSessionResult{}, nil
	}

	content := []mcp.Content{
		&mcp.TextContent{
			Text: string(resultJSON),
		},
	}

	return &mcp.CallToolResult{
		Content: content,
		IsError: false,
	}, result, nil
}
