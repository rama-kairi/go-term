package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rama-kairi/go-term/internal/tracing"
)

// RunCommand executes a foreground command in the specified terminal session
func (t *TerminalTools) RunCommand(ctx context.Context, req *mcp.CallToolRequest, args RunCommandArgs) (*mcp.CallToolResult, RunCommandResult, error) {
	// M10: Start tracing span for command execution
	ctx, span := t.tracer.StartSpanWithKind(ctx, "run_command", tracing.SpanKindServer)
	defer span.End()
	span.SetAttribute(tracing.AttrSessionID, args.SessionID)
	span.SetAttribute(tracing.AttrCommand, args.Command)

	// H2: Check rate limit first
	if err := t.CheckRateLimit(); err != nil {
		span.SetStatus(tracing.StatusError, "rate limited")
		return createErrorResult(err.Error()), RunCommandResult{}, nil
	}

	// Validate input
	if err := validateSessionID(args.SessionID); err != nil {
		return createErrorResult(fmt.Sprintf("Invalid session ID: %v. Tip: Session ID must be a valid UUID4. Use 'list_terminal_sessions' to find valid session IDs, or create a new session with 'create_terminal_session'.", err)), RunCommandResult{}, nil
	}

	if err := t.security.ValidateCommand(args.Command); err != nil {
		t.logger.LogSecurityEvent("command_blocked", fmt.Sprintf("Command blocked: %s", args.Command), "medium", map[string]interface{}{
			"session_id": args.SessionID,
			"command":    args.Command,
			"reason":     err.Error(),
		})
		return createErrorResult(fmt.Sprintf("Command blocked for security reasons: %v. Tip: Check if the command contains restricted characters or operations. Review security settings or use a different approach.", err)), RunCommandResult{}, nil
	}

	// Determine timeout value
	timeoutSeconds := args.Timeout
	if timeoutSeconds <= 0 {
		timeoutSeconds = 60 // Default 60 seconds
	}
	if timeoutSeconds > 300 {
		timeoutSeconds = 300 // Maximum 5 minutes
	}
	timeout := time.Duration(timeoutSeconds) * time.Second

	// Verify session exists
	session, err := t.manager.GetSession(args.SessionID)
	if err != nil {
		return createErrorResult(fmt.Sprintf("Session not found: %v. Tip: Use 'list_terminal_sessions' to see all available sessions and their IDs. Make sure to create a session first with 'create_terminal_session'.", err)), RunCommandResult{}, nil
	}

	// Detect package manager and project type using current directory
	packageManager := ""
	currentWorkingDir := session.GetCurrentDir()
	projectType := t.packageManager.DetectProjectType(currentWorkingDir)
	if pm, err := t.packageManager.DetectPackageManager(currentWorkingDir); err == nil && pm != nil {
		packageManager = pm.Name
	}

	// Enhance command with package manager intelligence
	enhancedCommand := t.enhanceCommandWithPackageManager(args.Command, currentWorkingDir)

	// Execute the command in foreground with timeout
	startTime := time.Now()
	var output, errorOutput string
	var success bool
	var exitCode int
	var totalChunks int
	streamingUsed := false
	timedOut := false

	// Use timeout for command execution
	output, err = t.manager.ExecuteCommandWithTimeout(args.SessionID, enhancedCommand, timeout)
	success = err == nil
	exitCode = 0

	if err != nil {
		errorOutput = err.Error()
		exitCode = 1

		// Check if error is due to timeout
		if strings.Contains(err.Error(), "context deadline exceeded") ||
			strings.Contains(err.Error(), "timeout") ||
			strings.Contains(err.Error(), "signal: killed") {
			timedOut = true
			errorOutput = fmt.Sprintf("Command timed out after %d seconds: %v", timeoutSeconds, err)
			exitCode = 124 // Standard timeout exit code
		}
	}

	duration := time.Since(startTime)

	// Get updated session info
	updatedSession, _ := t.manager.GetSession(args.SessionID)
	commandCount := 0
	if updatedSession != nil {
		commandCount = updatedSession.CommandCount
	}

	result := RunCommandResult{
		SessionID:      args.SessionID,
		ProjectID:      session.ProjectID,
		Command:        enhancedCommand,
		Output:         output,
		ErrorOutput:    errorOutput,
		Success:        success,
		ExitCode:       exitCode,
		Duration:       duration.String(),
		WorkingDir:     session.WorkingDir,
		CommandCount:   commandCount,
		HistoryID:      fmt.Sprintf("%s_%d", args.SessionID[:8], commandCount),
		StreamingUsed:  streamingUsed,
		TotalChunks:    totalChunks,
		PackageManager: packageManager,
		ProjectType:    projectType,
		TimeoutUsed:    timeoutSeconds,
		TimedOut:       timedOut,
	}

	// Create response
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	content := []mcp.Content{
		&mcp.TextContent{
			Text: string(resultJSON),
		},
	}

	t.logger.Info("Foreground command executed", map[string]interface{}{
		"session_id":      args.SessionID,
		"project_id":      session.ProjectID,
		"success":         success,
		"duration":        duration.String(),
		"package_manager": packageManager,
		"project_type":    projectType,
		"timeout_used":    timeoutSeconds,
		"timed_out":       timedOut,
	})

	// M10: Update span with execution details
	span.SetAttributes(map[string]interface{}{
		tracing.AttrExitCode:     exitCode,
		tracing.AttrOutputSize:   len(output),
		tracing.AttrWorkingDir:   session.WorkingDir,
		tracing.AttrProjectID:    session.ProjectID,
		tracing.AttrIsBackground: false,
	})
	if success {
		span.SetStatus(tracing.StatusOK, "command completed successfully")
	} else {
		span.SetStatus(tracing.StatusError, errorOutput)
		span.SetAttribute(tracing.AttrErrorMessage, errorOutput)
	}
	if timedOut {
		span.AddEvent("command_timeout", tracing.Attribute{Key: "timeout_seconds", Value: timeoutSeconds})
	}

	return &mcp.CallToolResult{
		Content: content,
		IsError: false,
	}, result, nil
}
