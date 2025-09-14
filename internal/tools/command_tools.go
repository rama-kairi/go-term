package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RunCommand executes a foreground command in the specified terminal session
func (t *TerminalTools) RunCommand(ctx context.Context, req *mcp.CallToolRequest, args RunCommandArgs) (*mcp.CallToolResult, RunCommandResult, error) {
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

	// Execute the command in foreground only
	startTime := time.Now()
	var output, errorOutput string
	var success bool
	var exitCode int
	var totalChunks int
	streamingUsed := false

	// Execute with streaming if enabled, otherwise use traditional execution
	if t.config.Streaming.Enable {
		streamingUsed = true
		streamOutput, streamErr := t.manager.ExecuteCommandWithStreaming(args.SessionID, enhancedCommand)

		success = streamErr == nil
		exitCode = 0
		output = streamOutput
		totalChunks = 1

		if streamErr != nil {
			errorOutput = streamErr.Error()
			exitCode = 1
			success = false
		}
	} else {
		// Fall back to traditional execution
		output, err = t.manager.ExecuteCommand(args.SessionID, enhancedCommand)
		success = err == nil
		exitCode = 0

		if err != nil {
			errorOutput = err.Error()
			exitCode = 1
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
	})

	return &mcp.CallToolResult{
		Content: content,
		IsError: false,
	}, result, nil
}
