package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// CreateSession creates a new terminal session with project association and comprehensive documentation
func (t *TerminalTools) CreateSession(ctx context.Context, req *mcp.CallToolRequest, args CreateSessionArgs) (*mcp.CallToolResult, CreateSessionResult, error) {
	// Validate session name
	if err := validateSessionName(args.Name); err != nil {
		return createErrorResult(fmt.Sprintf("Invalid session name: %v. Tip: Session names should be 3-100 characters, alphanumeric with underscores and hyphens only. Examples: 'my-project', 'dev_server', 'testing_session'", err)), CreateSessionResult{}, nil
	}

	// Create session with simplified API - let session manager handle workspace detection and project ID generation
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

// DeleteSession deletes terminal sessions (individual or project-wide) with confirmation
func (t *TerminalTools) DeleteSession(ctx context.Context, req *mcp.CallToolRequest, args DeleteSessionArgs) (*mcp.CallToolResult, DeleteSessionResult, error) {
	// Require confirmation
	if !args.Confirm {
		return createErrorResult("Deletion requires confirmation. Set 'confirm' to true. Tip: This prevents accidental deletion of sessions and command history."), DeleteSessionResult{}, nil
	}

	// Validate arguments - must specify either session_id or project_id, but not both
	if args.SessionID == "" && args.ProjectID == "" {
		return createErrorResult("Must specify either session_id or project_id. Tip: Use session_id to delete a single session, or project_id to delete all sessions in a project."), DeleteSessionResult{}, nil
	}

	if args.SessionID != "" && args.ProjectID != "" {
		return createErrorResult("Cannot specify both session_id and project_id. Choose one. Tip: Use session_id to delete a single session, or project_id to delete all sessions in a project."), DeleteSessionResult{}, nil
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
