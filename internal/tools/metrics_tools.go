// Package tools provides MCP tool handlers for session activity metrics (M9)
package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rama-kairi/go-term/internal/terminal"
)

// --- Session Metrics Types ---

// GetSessionMetricsArgs represents arguments for getting session metrics
type GetSessionMetricsArgs struct {
	SessionID string `json:"session_id,omitempty" jsonschema:"description=Session ID to get metrics for. If not provided, returns metrics for all sessions."`
}

// SessionMetricsResult represents the result of getting session metrics
type SessionMetricsResult struct {
	Success bool                               `json:"success"`
	Metrics []*terminal.SessionActivityMetrics `json:"metrics,omitempty"`
	Summary *MetricsSummary                    `json:"summary,omitempty"`
	Message string                             `json:"message,omitempty"`
}

// MetricsSummary provides an aggregated summary across all sessions
type MetricsSummary struct {
	TotalSessions             int            `json:"total_sessions"`
	TotalCommands             int            `json:"total_commands"`
	TotalSuccessful           int            `json:"total_successful"`
	TotalFailed               int            `json:"total_failed"`
	OverallSuccessRate        float64        `json:"overall_success_rate"`
	TotalExecutionTime        int64          `json:"total_execution_time"`
	AverageCommandsPerSession float64        `json:"average_commands_per_session"`
	MostActiveSession         string         `json:"most_active_session"`
	MostActiveSessionCommands int            `json:"most_active_session_commands"`
	CommonCommandTypes        map[string]int `json:"common_command_types"`
	CommonErrorCategories     map[string]int `json:"common_error_categories"`
}

// GetSessionActivityMetrics retrieves detailed activity metrics for sessions
func (t *TerminalTools) GetSessionActivityMetrics(ctx context.Context, req *mcp.CallToolRequest, args GetSessionMetricsArgs) (*mcp.CallToolResult, SessionMetricsResult, error) {
	var metrics []*terminal.SessionActivityMetrics

	if args.SessionID != "" {
		// Get metrics for specific session
		metric, err := t.manager.GetSessionActivityMetrics(args.SessionID)
		if err != nil {
			result := SessionMetricsResult{
				Success: false,
				Message: fmt.Sprintf("Failed to get metrics for session %s: %v", args.SessionID, err),
			}
			return createErrorResult(result.Message), result, nil
		}
		metrics = []*terminal.SessionActivityMetrics{metric}
	} else {
		// Get metrics for all sessions
		metrics = t.manager.GetAllSessionActivityMetrics()
	}

	// Calculate summary
	summary := calculateMetricsSummary(metrics)

	result := SessionMetricsResult{
		Success: true,
		Metrics: metrics,
		Summary: summary,
		Message: fmt.Sprintf("Retrieved activity metrics for %d session(s)", len(metrics)),
	}

	t.logger.Info("Retrieved session activity metrics", map[string]interface{}{
		"session_count":   len(metrics),
		"total_commands":  summary.TotalCommands,
		"overall_success": summary.OverallSuccessRate,
	})

	return createJSONResult(result), result, nil
}

// calculateMetricsSummary aggregates metrics across all sessions
func calculateMetricsSummary(metrics []*terminal.SessionActivityMetrics) *MetricsSummary {
	summary := &MetricsSummary{
		TotalSessions:         len(metrics),
		CommonCommandTypes:    make(map[string]int),
		CommonErrorCategories: make(map[string]int),
	}

	if len(metrics) == 0 {
		return summary
	}

	var mostActiveSession string
	var mostActiveCommands int

	for _, m := range metrics {
		summary.TotalCommands += m.TotalCommands
		summary.TotalSuccessful += m.SuccessfulCommands
		summary.TotalFailed += m.FailedCommands
		summary.TotalExecutionTime += int64(m.TotalExecutionTime)

		// Track most active session
		if m.TotalCommands > mostActiveCommands {
			mostActiveCommands = m.TotalCommands
			mostActiveSession = m.SessionID
		}

		// Aggregate command types
		for cmdType, count := range m.CommandTypeDistribution {
			summary.CommonCommandTypes[cmdType] += count
		}

		// Aggregate error categories
		for errCat, count := range m.ErrorCategories {
			summary.CommonErrorCategories[errCat] += count
		}
	}

	// Calculate rates and averages
	if summary.TotalCommands > 0 {
		summary.OverallSuccessRate = float64(summary.TotalSuccessful) / float64(summary.TotalCommands)
	}

	if summary.TotalSessions > 0 {
		summary.AverageCommandsPerSession = float64(summary.TotalCommands) / float64(summary.TotalSessions)
	}

	summary.MostActiveSession = mostActiveSession
	summary.MostActiveSessionCommands = mostActiveCommands

	return summary
}
