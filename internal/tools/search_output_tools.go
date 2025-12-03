package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// F6: SearchOutputArgs represents arguments for searching command output
type SearchOutputArgs struct {
	SessionID      string `json:"session_id,omitempty" jsonschema:"description=Search outputs in a specific session"`
	ProjectID      string `json:"project_id,omitempty" jsonschema:"description=Search outputs in a specific project"`
	Pattern        string `json:"pattern" jsonschema:"required,description=Text or regex pattern to search for in command outputs"`
	IsRegex        bool   `json:"is_regex,omitempty" jsonschema:"description=Treat pattern as regular expression"`
	CaseSensitive  bool   `json:"case_sensitive,omitempty" jsonschema:"description=Case sensitive search (default: false)"`
	MaxResults     int    `json:"max_results,omitempty" jsonschema:"description=Maximum number of results to return (default: 50)"`
	IncludeContext int    `json:"include_context,omitempty" jsonschema:"description=Number of lines of context around match (default: 2)"`
}

// SearchOutputMatch represents a single match in the output
type SearchOutputMatch struct {
	CommandID   string   `json:"command_id"`
	SessionID   string   `json:"session_id"`
	Command     string   `json:"command"`
	LineNumber  int      `json:"line_number"`
	MatchedText string   `json:"matched_text"`
	Context     []string `json:"context,omitempty"`
	Timestamp   string   `json:"timestamp"`
}

// SearchOutputResult represents the result of searching outputs
type SearchOutputResult struct {
	Pattern      string              `json:"pattern"`
	IsRegex      bool                `json:"is_regex"`
	TotalMatches int                 `json:"total_matches"`
	Matches      []SearchOutputMatch `json:"matches"`
	SearchTime   string              `json:"search_time"`
	Truncated    bool                `json:"truncated"`
}

// SearchOutput searches through command outputs for a pattern
func (t *TerminalTools) SearchOutput(ctx context.Context, req *mcp.CallToolRequest, args SearchOutputArgs) (*mcp.CallToolResult, SearchOutputResult, error) {
	startTime := time.Now()

	// Set defaults
	if args.MaxResults <= 0 {
		args.MaxResults = 50
	}
	if args.MaxResults > 200 {
		args.MaxResults = 200
	}
	if args.IncludeContext <= 0 {
		args.IncludeContext = 2
	}

	// Validate pattern
	if args.Pattern == "" {
		return createErrorResult("Search pattern cannot be empty"), SearchOutputResult{}, nil
	}

	// Prepare the search pattern
	var searchFunc func(string) [][]int
	if args.IsRegex {
		flags := ""
		if !args.CaseSensitive {
			flags = "(?i)"
		}
		re, err := regexp.Compile(flags + args.Pattern)
		if err != nil {
			return createErrorResult(fmt.Sprintf("Invalid regex pattern: %v", err)), SearchOutputResult{}, nil
		}
		searchFunc = func(text string) [][]int {
			return re.FindAllStringIndex(text, -1)
		}
	} else {
		pattern := args.Pattern
		if !args.CaseSensitive {
			pattern = strings.ToLower(pattern)
		}
		searchFunc = func(text string) [][]int {
			searchText := text
			if !args.CaseSensitive {
				searchText = strings.ToLower(text)
			}
			var matches [][]int
			offset := 0
			for {
				idx := strings.Index(searchText[offset:], pattern)
				if idx == -1 {
					break
				}
				absIdx := offset + idx
				matches = append(matches, []int{absIdx, absIdx + len(pattern)})
				offset = absIdx + 1
			}
			return matches
		}
	}

	// Get commands from database
	commands, err := t.database.SearchCommandsFormatted(
		args.SessionID,
		args.ProjectID,
		"",          // no command filter
		"",          // no output filter (we'll search manually)
		nil,         // any success status
		time.Time{}, // no start time
		time.Time{}, // no end time
		500,         // get more commands to search through
	)
	if err != nil {
		return createErrorResult(fmt.Sprintf("Failed to get commands: %v", err)), SearchOutputResult{}, nil
	}

	var matches []SearchOutputMatch

	for _, cmd := range commands {
		// Search in output
		output := cmd.Output
		if output == "" {
			continue
		}

		lines := strings.Split(output, "\n")
		matchIndices := make(map[int]bool)

		// Find all matching lines
		for lineNum, line := range lines {
			if found := searchFunc(line); len(found) > 0 {
				matchIndices[lineNum] = true
			}
		}

		// Create matches with context
		for lineNum := range matchIndices {
			if len(matches) >= args.MaxResults {
				break
			}

			// Build context
			var contextLines []string
			startLine := lineNum - args.IncludeContext
			if startLine < 0 {
				startLine = 0
			}
			endLine := lineNum + args.IncludeContext
			if endLine >= len(lines) {
				endLine = len(lines) - 1
			}

			for i := startLine; i <= endLine; i++ {
				prefix := "  "
				if i == lineNum {
					prefix = "> "
				}
				contextLines = append(contextLines, fmt.Sprintf("%s%d: %s", prefix, i+1, lines[i]))
			}

			match := SearchOutputMatch{
				CommandID:   cmd.ID,
				SessionID:   cmd.SessionID,
				Command:     cmd.Command,
				LineNumber:  lineNum + 1,
				MatchedText: lines[lineNum],
				Context:     contextLines,
				Timestamp:   cmd.Timestamp,
			}
			matches = append(matches, match)
		}

		if len(matches) >= args.MaxResults {
			break
		}
	}

	result := SearchOutputResult{
		Pattern:      args.Pattern,
		IsRegex:      args.IsRegex,
		TotalMatches: len(matches),
		Matches:      matches,
		SearchTime:   time.Since(startTime).String(),
		Truncated:    len(matches) >= args.MaxResults,
	}

	t.logger.Info("Output search completed", map[string]interface{}{
		"pattern":     args.Pattern,
		"matches":     len(matches),
		"search_time": result.SearchTime,
	})

	return createJSONResult(result), result, nil
}
