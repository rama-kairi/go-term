// Package tools provides MCP tool handlers for tracing (M10)
package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rama-kairi/go-term/internal/tracing"
)

// --- Tracing Types ---

// GetTracesArgs represents arguments for getting traces
type GetTracesArgs struct {
	Limit   int    `json:"limit,omitempty" jsonschema:"description=Maximum number of spans to return (default: 100)"`
	TraceID string `json:"trace_id,omitempty" jsonschema:"description=Filter by specific trace ID"`
}

// TracesResult represents the result of getting traces
type TracesResult struct {
	Success bool            `json:"success"`
	Spans   []*tracing.Span `json:"spans"`
	Count   int             `json:"count"`
	Message string          `json:"message,omitempty"`
}

// GetTraces retrieves collected trace spans
func (t *TerminalTools) GetTraces(ctx context.Context, req *mcp.CallToolRequest, args GetTracesArgs) (*mcp.CallToolResult, TracesResult, error) {
	if t.tracer == nil {
		result := TracesResult{
			Success: false,
			Message: "Tracing is not enabled",
		}
		return createErrorResult(result.Message), result, nil
	}

	limit := args.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	spans := t.tracer.GetRecentSpans(limit)

	// Filter by trace ID if specified
	if args.TraceID != "" {
		filtered := make([]*tracing.Span, 0)
		for _, span := range spans {
			if span.TraceID() == args.TraceID {
				filtered = append(filtered, span)
			}
		}
		spans = filtered
	}

	result := TracesResult{
		Success: true,
		Spans:   spans,
		Count:   len(spans),
		Message: fmt.Sprintf("Retrieved %d trace spans", len(spans)),
	}

	return createJSONResult(result), result, nil
}
