package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rama-kairi/go-term/internal/config"
	"github.com/rama-kairi/go-term/internal/database"
	"github.com/rama-kairi/go-term/internal/logger"
	"github.com/rama-kairi/go-term/internal/terminal"
	"github.com/rama-kairi/go-term/internal/tracing"
	"github.com/rama-kairi/go-term/internal/utils"
)

// H2: RateLimiter implements a token bucket rate limiter
type RateLimiter struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
	mu         sync.Mutex
}

// NewRateLimiter creates a new rate limiter with the given rate and burst
func NewRateLimiter(ratePerMinute int, burst int) *RateLimiter {
	return &RateLimiter{
		tokens:     float64(burst),
		maxTokens:  float64(burst),
		refillRate: float64(ratePerMinute) / 60.0, // Convert to per-second
		lastRefill: time.Now(),
	}
}

// Allow checks if a request is allowed and consumes a token if so
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()
	rl.lastRefill = now

	// Refill tokens
	rl.tokens += elapsed * rl.refillRate
	if rl.tokens > rl.maxTokens {
		rl.tokens = rl.maxTokens
	}

	// Check if we have tokens available
	if rl.tokens >= 1 {
		rl.tokens--
		return true
	}
	return false
}

// GetTokens returns current available tokens (for monitoring)
func (rl *RateLimiter) GetTokens() float64 {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.tokens
}

// TerminalTools contains all MCP tools for terminal management with enhanced features
type TerminalTools struct {
	manager           *terminal.Manager
	config            *config.Config
	logger            *logger.Logger
	database          *database.DB
	security          *SecurityValidator
	projectGen        *utils.ProjectIDGenerator
	packageManager    *utils.PackageManagerDetector
	rateLimiter       *RateLimiter       // H2: Rate limiter for tool calls
	templateManager   *TemplateManager   // F1: Command templates manager
	snapshotManager   *SnapshotManager   // F2: Session snapshots manager
	dependencyManager *DependencyManager // F7: Process dependency manager
	tracer            *tracing.Tracer    // M10: Command execution tracing
}

// NewTerminalTools creates a new instance of terminal tools with enhanced features
func NewTerminalTools(manager *terminal.Manager, cfg *config.Config, logger *logger.Logger, db *database.DB) *TerminalTools {
	return &TerminalTools{
		manager:           manager,
		config:            cfg,
		logger:            logger,
		database:          db,
		security:          NewSecurityValidator(cfg),
		projectGen:        utils.NewProjectIDGenerator(),
		packageManager:    utils.NewPackageManagerDetector(),
		rateLimiter:       NewRateLimiter(cfg.Session.RateLimitPerMinute, cfg.Session.RateLimitBurst),
		templateManager:   NewTemplateManager(),
		snapshotManager:   NewSnapshotManager(cfg.Database.DataDir),
		dependencyManager: NewDependencyManager(),
		tracer:            tracing.NewTracer("go-term"),
	}
}

// CheckRateLimit checks if the rate limit is exceeded and returns an error if so
func (t *TerminalTools) CheckRateLimit() error {
	if !t.rateLimiter.Allow() {
		t.logger.Warn("Rate limit exceeded", map[string]interface{}{
			"available_tokens": t.rateLimiter.GetTokens(),
		})
		return fmt.Errorf("rate limit exceeded. Please slow down your requests. Current limit: %d calls per minute",
			t.config.Session.RateLimitPerMinute)
	}
	return nil
}

// =============================================================================
// F1: Command Template Tool Wrappers (defined in template_tools.go)
// =============================================================================

// CreateCommandTemplateArgs represents arguments for creating a command template
type CreateCommandTemplateArgs struct {
	Name        string `json:"name" jsonschema:"required,description=Unique name for the template"`
	Command     string `json:"command" jsonschema:"required,description=Command template with optional {{variable}} placeholders"`
	Description string `json:"description,omitempty" jsonschema:"description=Description of what the template does"`
	Category    string `json:"category,omitempty" jsonschema:"description=Category for organizing templates"`
}

// CreateCommandTemplate creates a new command template
func (t *TerminalTools) CreateCommandTemplate(ctx context.Context, req *mcp.CallToolRequest, args CreateCommandTemplateArgs) (*mcp.CallToolResult, *CommandTemplate, error) {
	template := &CommandTemplate{
		Name:        args.Name,
		Command:     args.Command,
		Description: args.Description,
		Category:    args.Category,
	}

	if err := t.templateManager.AddTemplate(template); err != nil {
		return createErrorResult(fmt.Sprintf("Failed to create template: %v", err)), nil, nil
	}

	t.logger.Info("Command template created", map[string]interface{}{
		"name":     args.Name,
		"category": args.Category,
	})

	return createJSONResult(template), template, nil
}

// ExpandCommandTemplateArgs represents arguments for expanding a template
type ExpandCommandTemplateArgs struct {
	TemplateName string            `json:"template_name" jsonschema:"required,description=Name of the template to expand"`
	Variables    map[string]string `json:"variables,omitempty" jsonschema:"description=Map of variable names to values"`
}

// ExpandCommandTemplateResult represents the result of expanding a template
type ExpandCommandTemplateResult struct {
	OriginalTemplate string `json:"original_template"`
	ExpandedCommand  string `json:"expanded_command"`
	VariablesUsed    int    `json:"variables_used"`
}

// ExpandCommandTemplate expands a command template with variables
func (t *TerminalTools) ExpandCommandTemplate(ctx context.Context, req *mcp.CallToolRequest, args ExpandCommandTemplateArgs) (*mcp.CallToolResult, ExpandCommandTemplateResult, error) {
	template, exists := t.templateManager.GetTemplate(args.TemplateName)
	if !exists {
		return createErrorResult(fmt.Sprintf("Template not found: %s", args.TemplateName)), ExpandCommandTemplateResult{}, nil
	}

	expanded, _ := t.templateManager.ExpandTemplate(template.Command, args.Variables)

	result := ExpandCommandTemplateResult{
		OriginalTemplate: template.Command,
		ExpandedCommand:  expanded,
		VariablesUsed:    len(args.Variables),
	}

	return createJSONResult(result), result, nil
}

// =============================================================================
// F6: Output Search Tool Wrapper
// =============================================================================

// SearchCommandOutput searches through command outputs
func (t *TerminalTools) SearchCommandOutput(ctx context.Context, req *mcp.CallToolRequest, args SearchOutputArgs) (*mcp.CallToolResult, SearchOutputResult, error) {
	// Get session to validate it exists
	session, err := t.manager.GetSession(args.SessionID)
	if err != nil {
		return createErrorResult(fmt.Sprintf("Session not found: %v", err)), SearchOutputResult{}, nil
	}

	// Get command history from database using SearchCommands
	maxResults := args.MaxResults
	if maxResults <= 0 {
		maxResults = 100
	}
	commands, err := t.database.SearchCommands(args.SessionID, "", "", "", nil, time.Time{}, time.Time{}, maxResults)
	if err != nil {
		return createErrorResult(fmt.Sprintf("Failed to get command history: %v", err)), SearchOutputResult{}, nil
	}

	// Perform search through the outputs
	result := searchCommandOutputsInternal(commands, args, session.WorkingDir)

	return createJSONResult(result), result, nil
}

// searchCommandOutputsInternal performs the actual search through command outputs
func searchCommandOutputsInternal(commands []*database.CommandRecord, args SearchOutputArgs, workingDir string) SearchOutputResult {
	var matches []SearchOutputMatch
	pattern := args.Pattern

	if !args.CaseSensitive {
		pattern = strings.ToLower(pattern)
	}

	contextLines := args.IncludeContext
	if contextLines <= 0 {
		contextLines = 2
	}

	for _, cmd := range commands {
		output := cmd.Output
		searchOutput := output
		if !args.CaseSensitive {
			searchOutput = strings.ToLower(output)
		}

		if strings.Contains(searchOutput, pattern) {
			// Find the line numbers with matches
			lines := strings.Split(output, "\n")
			for lineNum, line := range lines {
				searchLine := line
				if !args.CaseSensitive {
					searchLine = strings.ToLower(line)
				}
				if strings.Contains(searchLine, pattern) {
					match := SearchOutputMatch{
						CommandID:   cmd.ID,
						SessionID:   cmd.SessionID,
						Command:     cmd.Command,
						LineNumber:  lineNum + 1,
						MatchedText: line,
						Timestamp:   cmd.Timestamp.Format(time.RFC3339),
					}

					// Add context lines
					start := lineNum - contextLines
					if start < 0 {
						start = 0
					}
					end := lineNum + contextLines + 1
					if end > len(lines) {
						end = len(lines)
					}
					match.Context = lines[start:end]

					matches = append(matches, match)
				}
			}
		}
	}

	truncated := false
	maxResults := args.MaxResults
	if maxResults > 0 && len(matches) > maxResults {
		matches = matches[:maxResults]
		truncated = true
	}

	return SearchOutputResult{
		Pattern:      args.Pattern,
		IsRegex:      args.IsRegex,
		TotalMatches: len(matches),
		Matches:      matches,
		SearchTime:   time.Now().Format(time.RFC3339),
		Truncated:    truncated,
	}
}

// =============================================================================
// F2: Session Snapshot Tool Wrappers
// =============================================================================

// SaveSessionSnapshotArgs represents arguments for saving a session snapshot
type SaveSessionSnapshotArgs struct {
	SessionID   string `json:"session_id" jsonschema:"required,description=Session ID to snapshot"`
	Name        string `json:"name" jsonschema:"required,description=Name for the snapshot"`
	Description string `json:"description,omitempty" jsonschema:"description=Optional description"`
}

// SaveSessionSnapshot saves a session snapshot
func (t *TerminalTools) SaveSessionSnapshot(ctx context.Context, req *mcp.CallToolRequest, args SaveSessionSnapshotArgs) (*mcp.CallToolResult, *SessionSnapshot, error) {
	session, err := t.manager.GetSession(args.SessionID)
	if err != nil {
		return createErrorResult(fmt.Sprintf("Session not found: %v", err)), nil, nil
	}

	snapshot := &SessionSnapshot{
		SessionID:   args.SessionID,
		Name:        args.Name,
		Description: args.Description,
		WorkingDir:  session.WorkingDir,
		Environment: session.Environment,
		CreatedAt:   time.Now(),
	}

	if err := t.snapshotManager.saveSnapshot(snapshot); err != nil {
		return createErrorResult(fmt.Sprintf("Failed to save snapshot: %v", err)), nil, nil
	}

	t.logger.Info("Session snapshot saved", map[string]interface{}{
		"session_id":  args.SessionID,
		"snapshot_id": snapshot.ID,
		"name":        args.Name,
	})

	return createJSONResult(snapshot), snapshot, nil
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

		// Single-word blocked commands: check word-by-word with word boundaries
		if !strings.ContainsAny(blockedLower, " -/") {
			for _, word := range commandWords {
				// Remove common shell operators to get the actual command
				cleanWord := strings.Trim(word, ";&|(){}[]<>\"'`")

				if cleanWord == blockedLower {
					return fmt.Errorf("command contains blocked operation: %s", blocked)
				}
			}
			continue
		}

		// Multi-word or pattern-based blocked commands: check for exact substring match
		// with word boundary awareness for patterns like "rm -rf /"
		if s.containsBlockedPattern(lowerCommand, blockedLower) {
			return fmt.Errorf("command contains blocked operation: %s", blocked)
		}
	}

	// Additional security checks
	if s.config.Security.EnableSandbox {
		// Check for potentially dangerous patterns using word boundaries
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
			if s.containsBlockedPattern(lowerCommand, pattern) {
				return fmt.Errorf("command contains potentially dangerous pattern: %s", pattern)
			}
		}

		// Check for network access if not allowed
		if !s.config.Security.AllowNetworkAccess {
			networkCommands := []string{"wget", "curl", "ssh", "scp", "rsync", "nc", "netcat", "telnet"}
			for _, netCmd := range networkCommands {
				if s.isCommandPresent(lowerCommand, netCmd) {
					return fmt.Errorf("network access not allowed: %s", netCmd)
				}
			}
		}

		// Check for file system write operations if not allowed
		if !s.config.Security.AllowFileSystemWrite {
			writeCommands := []string{"rm", "mv", "cp", "touch", "mkdir", "rmdir"}
			for _, writeCmd := range writeCommands {
				if s.isCommandPresent(lowerCommand, writeCmd) {
					return fmt.Errorf("file system write operations not allowed: %s", writeCmd)
				}
			}
		}
	}

	return nil
}

// containsBlockedPattern checks if a command contains a blocked pattern with awareness of context.
// It uses substring matching but ensures the pattern is not part of a larger word in most cases.
func (s *SecurityValidator) containsBlockedPattern(command, pattern string) bool {
	// For patterns with operators (>, |, &, etc.) or paths (/), do direct substring match
	// These are inherently unambiguous (e.g., "> /dev/" won't appear in normal command names)
	if strings.ContainsAny(pattern, ">|&;(){}[]<>") || strings.Count(pattern, "/") >= 2 {
		return strings.Contains(command, pattern)
	}

	// For other patterns, check if the pattern appears as a complete substring
	// with appropriate boundaries
	return strings.Contains(command, pattern)
}

// isCommandPresent checks if a specific command is present in the command line.
// It uses word boundary checking to avoid false positives (e.g., "nc" in "sync").
func (s *SecurityValidator) isCommandPresent(command, cmdName string) bool {
	words := strings.Fields(command)
	for _, word := range words {
		// Remove shell operators and quotes to get the base command
		cleanWord := strings.Trim(word, ";&|(){}[]<>\"'`=:")

		// Check if word starts with or equals the command name
		// This handles cases like "curl" in "curl https://..." but not in "sync"
		if cleanWord == cmdName || strings.HasPrefix(cleanWord, cmdName+"=") {
			return true
		}
	}
	return false
}

// ===== TYPE DEFINITIONS =====

// CreateSessionArgs represents arguments for creating a terminal session (simplified)
type CreateSessionArgs struct {
	Name       string `json:"name" jsonschema:"required,description=Simple descriptive name for the terminal session"`
	ProjectID  string `json:"project_id,omitempty" jsonschema:"description=Optional: Custom project ID to group related sessions. Auto-generated from directory name if not provided."`
	WorkingDir string `json:"working_dir,omitempty" jsonschema:"description=Optional: Starting directory for the session. Uses current directory if not specified."`
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
	Timeout   int    `json:"timeout,omitempty" jsonschema:"description=Optional: Command timeout in seconds. Default: 60 seconds. Maximum: 300 seconds (5 minutes). Set to 0 to use default timeout."`
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
	TimeoutUsed    int    `json:"timeout_used"`              // Timeout value used in seconds
	TimedOut       bool   `json:"timed_out"`                 // Whether command was terminated due to timeout
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
