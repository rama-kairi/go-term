package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SearchHistory searches through command history across all sessions and projects
func (t *TerminalTools) SearchHistory(ctx context.Context, req *mcp.CallToolRequest, args SearchHistoryArgs) (*mcp.CallToolResult, SearchHistoryResult, error) {
	startTime := time.Now()

	// Parse time filters if provided
	var startTimeFilter, endTimeFilter time.Time
	if args.StartTime != "" {
		if t, err := time.Parse(time.RFC3339, args.StartTime); err == nil {
			startTimeFilter = t
		} else {
			return createErrorResult(fmt.Sprintf("Invalid start_time format. Use ISO 8601 format: %s. Example: %s", time.RFC3339, time.Now().Add(-24*time.Hour).Format(time.RFC3339))), SearchHistoryResult{}, nil
		}
	}

	if args.EndTime != "" {
		if t, err := time.Parse(time.RFC3339, args.EndTime); err == nil {
			endTimeFilter = t
		} else {
			return createErrorResult(fmt.Sprintf("Invalid end_time format. Use ISO 8601 format: %s. Example: %s", time.RFC3339, time.Now().Format(time.RFC3339))), SearchHistoryResult{}, nil
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
	commands, err := t.database.SearchCommandsFormatted(
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
