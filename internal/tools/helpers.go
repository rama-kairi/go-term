package tools

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Helper functions for validation and result creation

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
func createErrorResult(message string) *mcp.CallToolResult {
	content := []mcp.Content{
		&mcp.TextContent{
			Text: fmt.Sprintf("Error: %s", message),
		},
	}

	return &mcp.CallToolResult{
		Content: content,
		IsError: true,
	}
}

// enhanceCommandWithPackageManager enhances commands with appropriate package manager
func (t *TerminalTools) enhanceCommandWithPackageManager(command, workingDir string) string {
	// Simple enhancement - in production this would be more sophisticated
	if strings.Contains(command, "npm run") {
		// Check if bun is available and preferred
		if pm, err := t.packageManager.DetectPackageManager(workingDir); err == nil && pm != nil {
			if pm.Name == "bun" {
				return strings.Replace(command, "npm run", "bun run", 1)
			}
		}
	}

	// For Python scripts, prefer uv if available
	if strings.HasSuffix(command, ".py") && !strings.Contains(command, "uv run") {
		if pm, err := t.packageManager.DetectPackageManager(workingDir); err == nil && pm != nil {
			if pm.Name == "uv" {
				return "uv run " + command
			}
		}
	}

	return command
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
