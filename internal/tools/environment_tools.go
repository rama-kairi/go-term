// Package tools provides MCP tool handlers for environment variable management
package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- Environment Variable Types ---

// SetEnvironmentArgs represents arguments for setting environment variables
type SetEnvironmentArgs struct {
	SessionID string            `json:"session_id" jsonschema:"description=The session ID to set environment variables for"`
	Variables map[string]string `json:"variables" jsonschema:"description=Map of environment variable names to values"`
}

// GetEnvironmentArgs represents arguments for getting environment variables
type GetEnvironmentArgs struct {
	SessionID string `json:"session_id" jsonschema:"description=The session ID to get environment variables from"`
	Key       string `json:"key,omitempty" jsonschema:"description=Specific environment variable key to retrieve. If not provided, returns all variables"`
}

// UnsetEnvironmentArgs represents arguments for removing environment variables
type UnsetEnvironmentArgs struct {
	SessionID string   `json:"session_id" jsonschema:"description=The session ID to unset environment variables from"`
	Keys      []string `json:"keys" jsonschema:"description=List of environment variable keys to remove"`
}

// EnvironmentResult represents the result of environment operations
type EnvironmentResult struct {
	Success   bool              `json:"success"`
	SessionID string            `json:"session_id"`
	Operation string            `json:"operation"`
	Variables map[string]string `json:"variables,omitempty"`
	Message   string            `json:"message,omitempty"`
	Count     int               `json:"count,omitempty"`
}

// --- MCP Tool Handlers ---

// SetSessionEnvironment sets or updates environment variables for a session
func (t *TerminalTools) SetSessionEnvironment(ctx context.Context, req *mcp.CallToolRequest, args SetEnvironmentArgs) (*mcp.CallToolResult, EnvironmentResult, error) {
	// Rate limit check
	if !t.rateLimiter.Allow() {
		result := EnvironmentResult{
			Success:   false,
			SessionID: args.SessionID,
			Operation: "set",
			Message:   "rate limit exceeded, please try again later",
		}
		return createErrorResult("rate limit exceeded"), result, nil
	}

	// Validate input
	if args.SessionID == "" {
		result := EnvironmentResult{
			Success:   false,
			Operation: "set",
			Message:   "session_id is required",
		}
		return createErrorResult("session_id is required"), result, nil
	}

	if len(args.Variables) == 0 {
		result := EnvironmentResult{
			Success:   false,
			SessionID: args.SessionID,
			Operation: "set",
			Message:   "at least one variable is required",
		}
		return createErrorResult("at least one variable is required"), result, nil
	}

	// Validate variable names (security check)
	for key := range args.Variables {
		if key == "" {
			result := EnvironmentResult{
				Success:   false,
				SessionID: args.SessionID,
				Operation: "set",
				Message:   "empty variable name is not allowed",
			}
			return createErrorResult("empty variable name is not allowed"), result, nil
		}

		// Check for potentially dangerous variables
		dangerousVars := []string{"PATH", "LD_PRELOAD", "LD_LIBRARY_PATH", "DYLD_LIBRARY_PATH", "DYLD_INSERT_LIBRARIES"}
		for _, dangerous := range dangerousVars {
			if key == dangerous {
				t.logger.Warn("Attempt to set potentially dangerous environment variable", map[string]interface{}{
					"session_id": args.SessionID,
					"variable":   key,
				})
				// Allow but log - user may have valid reasons
			}
		}
	}

	// Set environment variables
	if err := t.manager.SetSessionEnvironment(args.SessionID, args.Variables); err != nil {
		t.logger.Error("Failed to set environment variables", err, map[string]interface{}{
			"session_id": args.SessionID,
			"count":      len(args.Variables),
		})
		result := EnvironmentResult{
			Success:   false,
			SessionID: args.SessionID,
			Operation: "set",
			Message:   err.Error(),
		}
		return createErrorResult(err.Error()), result, nil
	}

	result := EnvironmentResult{
		Success:   true,
		SessionID: args.SessionID,
		Operation: "set",
		Variables: args.Variables,
		Count:     len(args.Variables),
		Message:   fmt.Sprintf("Successfully set %d environment variable(s)", len(args.Variables)),
	}

	t.logger.Info("Environment variables set successfully", map[string]interface{}{
		"session_id": args.SessionID,
		"count":      len(args.Variables),
	})

	return createJSONResult(result), result, nil
}

// GetSessionEnvironment retrieves environment variables for a session
func (t *TerminalTools) GetSessionEnvironment(ctx context.Context, req *mcp.CallToolRequest, args GetEnvironmentArgs) (*mcp.CallToolResult, EnvironmentResult, error) {
	// Validate input
	if args.SessionID == "" {
		result := EnvironmentResult{
			Success:   false,
			Operation: "get",
			Message:   "session_id is required",
		}
		return createErrorResult("session_id is required"), result, nil
	}

	// Get environment variables
	envVars, err := t.manager.GetSessionEnvironment(args.SessionID)
	if err != nil {
		t.logger.Error("Failed to get environment variables", err, map[string]interface{}{
			"session_id": args.SessionID,
		})
		result := EnvironmentResult{
			Success:   false,
			SessionID: args.SessionID,
			Operation: "get",
			Message:   err.Error(),
		}
		return createErrorResult(err.Error()), result, nil
	}

	// If specific key requested, filter
	if args.Key != "" {
		if value, exists := envVars[args.Key]; exists {
			envVars = map[string]string{args.Key: value}
		} else {
			result := EnvironmentResult{
				Success:   false,
				SessionID: args.SessionID,
				Operation: "get",
				Message:   fmt.Sprintf("environment variable '%s' not found", args.Key),
			}
			return createErrorResult(result.Message), result, nil
		}
	}

	result := EnvironmentResult{
		Success:   true,
		SessionID: args.SessionID,
		Operation: "get",
		Variables: envVars,
		Count:     len(envVars),
		Message:   fmt.Sprintf("Retrieved %d environment variable(s)", len(envVars)),
	}

	return createJSONResult(result), result, nil
}

// UnsetSessionEnvironment removes environment variables from a session
func (t *TerminalTools) UnsetSessionEnvironment(ctx context.Context, req *mcp.CallToolRequest, args UnsetEnvironmentArgs) (*mcp.CallToolResult, EnvironmentResult, error) {
	// Rate limit check
	if !t.rateLimiter.Allow() {
		result := EnvironmentResult{
			Success:   false,
			SessionID: args.SessionID,
			Operation: "unset",
			Message:   "rate limit exceeded, please try again later",
		}
		return createErrorResult("rate limit exceeded"), result, nil
	}

	// Validate input
	if args.SessionID == "" {
		result := EnvironmentResult{
			Success:   false,
			Operation: "unset",
			Message:   "session_id is required",
		}
		return createErrorResult("session_id is required"), result, nil
	}

	if len(args.Keys) == 0 {
		result := EnvironmentResult{
			Success:   false,
			SessionID: args.SessionID,
			Operation: "unset",
			Message:   "at least one key is required",
		}
		return createErrorResult("at least one key is required"), result, nil
	}

	// Unset environment variables
	if err := t.manager.UnsetSessionEnvironment(args.SessionID, args.Keys); err != nil {
		t.logger.Error("Failed to unset environment variables", err, map[string]interface{}{
			"session_id": args.SessionID,
			"keys":       args.Keys,
		})
		result := EnvironmentResult{
			Success:   false,
			SessionID: args.SessionID,
			Operation: "unset",
			Message:   err.Error(),
		}
		return createErrorResult(err.Error()), result, nil
	}

	result := EnvironmentResult{
		Success:   true,
		SessionID: args.SessionID,
		Operation: "unset",
		Count:     len(args.Keys),
		Message:   fmt.Sprintf("Successfully removed %d environment variable(s)", len(args.Keys)),
	}

	t.logger.Info("Environment variables removed successfully", map[string]interface{}{
		"session_id": args.SessionID,
		"keys":       args.Keys,
	})

	return createJSONResult(result), result, nil
}
