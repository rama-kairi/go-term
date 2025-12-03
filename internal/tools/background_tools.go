package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// CheckBackgroundProcess checks the output and status of background processes for agents
func (t *TerminalTools) CheckBackgroundProcess(ctx context.Context, req *mcp.CallToolRequest, args CheckBackgroundProcessArgs) (*mcp.CallToolResult, CheckBackgroundProcessResult, error) {
	t.logger.Info("Checking background process status", map[string]interface{}{
		"session_id": args.SessionID,
		"process_id": args.ProcessID,
	})

	// Get the background process directly from session tracking
	bgProcess, err := t.manager.GetBackgroundProcess(args.SessionID, args.ProcessID)
	if err != nil {
		return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{
						Text: fmt.Sprintf("Error: %v", err),
					},
				},
				IsError: true,
			}, CheckBackgroundProcessResult{
				SessionID:   args.SessionID,
				Status:      "not_found",
				LastChecked: time.Now().Format("2006-01-02 15:04:05"),
			}, nil // Don't return error to allow graceful handling
	}

	// Thread-safe access to background process data
	bgProcess.Mutex.RLock()
	processID := bgProcess.ID
	command := bgProcess.Command
	pid := bgProcess.PID
	startTime := bgProcess.StartTime
	isRunning := bgProcess.IsRunning
	exitCode := bgProcess.ExitCode
	output := bgProcess.Output
	errorOutput := bgProcess.ErrorOutput
	bgProcess.Mutex.RUnlock()

	// Calculate duration
	var duration string
	if isRunning {
		duration = time.Since(startTime).String()
	} else {
		// For completed processes, we could calculate from stored data or estimate
		duration = time.Since(startTime).String()
	}

	// Determine status
	status := "running"
	if !isRunning {
		if exitCode == 0 {
			status = "completed"
		} else {
			status = "failed"
		}
	}

	result := CheckBackgroundProcessResult{
		SessionID:   args.SessionID,
		ProcessID:   processID,
		IsRunning:   isRunning,
		Output:      output,
		ErrorOutput: errorOutput,
		StartTime:   startTime.Format(time.RFC3339),
		Duration:    duration,
		Command:     command,
		PID:         pid,
		Status:      status,
		LastChecked: time.Now().Format("2006-01-02 15:04:05"),
	}

	// Create response message
	var statusMsg string
	if isRunning {
		statusMsg = fmt.Sprintf("Background process %s is running (PID: %d). Command: %s", processID[:8], pid, command)
	} else {
		statusMsg = fmt.Sprintf("Background process %s has %s with exit code %d. Command: %s", processID[:8], status, exitCode, command)
	}

	if output != "" {
		statusMsg += fmt.Sprintf("\n\nOutput:\n%s", output)
	}
	if errorOutput != "" {
		statusMsg += fmt.Sprintf("\n\nError Output:\n%s", errorOutput)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: statusMsg,
			},
		},
		IsError: false,
	}, result, nil
}

// RunBackgroundProcess starts a command as a background process with security validation
func (t *TerminalTools) RunBackgroundProcess(ctx context.Context, req *mcp.CallToolRequest, args RunBackgroundProcessArgs) (*mcp.CallToolResult, RunBackgroundProcessResult, error) {
	// H2: Check rate limit first
	if err := t.CheckRateLimit(); err != nil {
		return createErrorResult(err.Error()), RunBackgroundProcessResult{}, nil
	}

	// Validate session ID
	if err := validateSessionID(args.SessionID); err != nil {
		return createErrorResult(fmt.Sprintf("Invalid session ID: %v. Use 'list_terminal_sessions' to find valid session IDs.", err)), RunBackgroundProcessResult{}, nil
	}

	// Verify session exists
	session, err := t.manager.GetSession(args.SessionID)
	if err != nil {
		return createErrorResult(fmt.Sprintf("Session not found: %v. Use 'list_terminal_sessions' to see all available sessions.", err)), RunBackgroundProcessResult{}, nil
	}

	// SECURITY: Validate command before starting background process (C1 fix)
	if err := t.security.ValidateCommand(args.Command); err != nil {
		t.logger.LogSecurityEvent("blocked_background_command", args.Command, "high", map[string]interface{}{
			"session_id": args.SessionID,
			"reason":     err.Error(),
		})
		return createErrorResult(fmt.Sprintf("Command blocked by security policy: %v", err)), RunBackgroundProcessResult{}, nil
	}

	// Start the background process
	processID, err := t.manager.ExecuteCommandInBackground(args.SessionID, args.Command)
	if err != nil {
		return createErrorResult(fmt.Sprintf("Failed to start background process: %v", err)), RunBackgroundProcessResult{}, nil
	}

	// Get updated session info
	updatedSession, _ := t.manager.GetSession(args.SessionID)
	backgroundCount := 0
	if updatedSession != nil {
		backgroundCount = len(updatedSession.BackgroundProcesses)
	}

	result := RunBackgroundProcessResult{
		SessionID:         args.SessionID,
		ProjectID:         session.ProjectID,
		ProcessID:         processID,
		Command:           args.Command,
		StartTime:         time.Now().Format(time.RFC3339),
		WorkingDir:        session.WorkingDir,
		Success:           true,
		Message:           fmt.Sprintf("Background process started successfully. Process ID: %s", processID),
		BackgroundCount:   backgroundCount,
		MaxBackgroundProc: t.config.Session.MaxBackgroundProcesses,
	}

	t.logger.Info("Background process started", map[string]interface{}{
		"session_id":       args.SessionID,
		"process_id":       processID,
		"command":          args.Command,
		"background_count": backgroundCount,
		"max_background":   t.config.Session.MaxBackgroundProcesses,
	})

	return createJSONResult(result), result, nil
}

// ListBackgroundProcesses lists all background processes with filtering options
func (t *TerminalTools) ListBackgroundProcesses(ctx context.Context, req *mcp.CallToolRequest, args ListBackgroundProcessesArgs) (*mcp.CallToolResult, ListBackgroundProcessesResult, error) {
	// Get all background processes using the manager method
	allSessionProcesses, err := t.manager.GetAllBackgroundProcesses(args.SessionID, args.ProjectID)
	if err != nil {
		return createErrorResult(fmt.Sprintf("Failed to get background processes: %v", err)), ListBackgroundProcessesResult{}, nil
	}

	var allProcesses []BackgroundProcessInfo
	sessionStats := make(map[string]int)
	projectStats := make(map[string]int)
	runningCount := 0

	// Get session details for each session with background processes
	for sessionID, processes := range allSessionProcesses {
		session, err := t.manager.GetSession(sessionID)
		if err != nil {
			continue // Skip if session not found
		}

		for processID, bgProcess := range processes {
			bgProcess.Mutex.RLock()

			processInfo := BackgroundProcessInfo{
				ProcessID:   processID,
				SessionID:   session.ID,
				SessionName: session.Name,
				ProjectID:   session.ProjectID,
				Command:     bgProcess.Command,
				PID:         bgProcess.PID,
				StartTime:   bgProcess.StartTime.Format(time.RFC3339),
				Duration:    time.Since(bgProcess.StartTime).String(),
				IsRunning:   bgProcess.IsRunning,
				ExitCode:    bgProcess.ExitCode,
				WorkingDir:  session.WorkingDir,
				OutputSize:  len(bgProcess.Output),
				ErrorSize:   len(bgProcess.ErrorOutput),
			}

			allProcesses = append(allProcesses, processInfo)
			sessionStats[session.ID]++
			projectStats[session.ProjectID]++

			if bgProcess.IsRunning {
				runningCount++
			}

			bgProcess.Mutex.RUnlock()
		}
	}

	completedCount := len(allProcesses) - runningCount
	summary := fmt.Sprintf("Total: %d processes (%d running, %d completed) across %d sessions and %d projects",
		len(allProcesses), runningCount, completedCount, len(sessionStats), len(projectStats))

	result := ListBackgroundProcessesResult{
		Processes:      allProcesses,
		TotalCount:     len(allProcesses),
		RunningCount:   runningCount,
		CompletedCount: completedCount,
		SessionStats:   sessionStats,
		ProjectStats:   projectStats,
		Summary:        summary,
	}

	t.logger.Info("Listed background processes", map[string]interface{}{
		"total_count":    len(allProcesses),
		"running_count":  runningCount,
		"session_filter": args.SessionID,
		"project_filter": args.ProjectID,
	})

	return createJSONResult(result), result, nil
}

// TerminateBackgroundProcess stops a specific background process
func (t *TerminalTools) TerminateBackgroundProcess(ctx context.Context, req *mcp.CallToolRequest, args TerminateBackgroundProcessArgs) (*mcp.CallToolResult, TerminateBackgroundProcessResult, error) {
	// Validate input
	if err := validateSessionID(args.SessionID); err != nil {
		return createErrorResult(fmt.Sprintf("Invalid session ID: %v", err)), TerminateBackgroundProcessResult{}, nil
	}

	if err := validateSessionID(args.ProcessID); err != nil {
		return createErrorResult(fmt.Sprintf("Invalid process ID: %v", err)), TerminateBackgroundProcessResult{}, nil
	}

	// Get the background process info before termination
	bgProcess, err := t.manager.GetBackgroundProcess(args.SessionID, args.ProcessID)
	if err != nil {
		return createErrorResult(fmt.Sprintf("Background process not found: %v", err)), TerminateBackgroundProcessResult{}, nil
	}

	// Capture process info before termination
	bgProcess.Mutex.RLock()
	wasRunning := bgProcess.IsRunning
	command := bgProcess.Command
	pid := bgProcess.PID
	finalOutput := bgProcess.Output
	finalError := bgProcess.ErrorOutput
	bgProcess.Mutex.RUnlock()

	// Attempt to terminate the process using the manager method
	err = t.manager.TerminateBackgroundProcess(args.SessionID, args.ProcessID, args.Force)
	terminated := err == nil

	message := ""
	if terminated {
		if args.Force {
			message = fmt.Sprintf("Force killed background process %s (PID: %d)", args.ProcessID[:8], pid)
		} else {
			message = fmt.Sprintf("Gracefully terminated background process %s (PID: %d)", args.ProcessID[:8], pid)
		}
	} else {
		message = fmt.Sprintf("Failed to terminate background process %s: %v", args.ProcessID[:8], err)
	}

	result := TerminateBackgroundProcessResult{
		SessionID:   args.SessionID,
		ProcessID:   args.ProcessID,
		Command:     command,
		PID:         pid,
		WasRunning:  wasRunning,
		Terminated:  terminated,
		Force:       args.Force,
		Message:     message,
		FinalOutput: finalOutput,
		FinalError:  finalError,
	}

	t.logger.Info("Background process termination", map[string]interface{}{
		"session_id":  args.SessionID,
		"process_id":  args.ProcessID,
		"was_running": wasRunning,
		"terminated":  terminated,
		"force":       args.Force,
	})

	return createJSONResult(result), result, nil
}
