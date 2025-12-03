package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rama-kairi/go-term/internal/config"
	"github.com/rama-kairi/go-term/internal/database"
	"github.com/rama-kairi/go-term/internal/logger"
	"github.com/rama-kairi/go-term/internal/monitoring"
	"github.com/rama-kairi/go-term/internal/terminal"
	"github.com/rama-kairi/go-term/internal/tools"
)

// boolPtr returns a pointer to a boolean value (used for MCP tool annotations)
func boolPtr(b bool) *bool {
	return &b
}

func main() {
	// Parse command line flags
	configFile := flag.String("config", "", "Path to configuration file")
	debugMode := flag.Bool("debug", false, "Enable debug mode")
	flag.Parse()

	// Load configuration
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Override debug mode if specified via flag
	if *debugMode {
		cfg.Server.Debug = true
		cfg.Logging.Level = "debug"
	}

	// Set log output to stderr to avoid interfering with JSON-RPC communication
	log.SetOutput(os.Stderr)

	// Initialize logger
	appLogger, err := logger.NewLogger(&cfg.Logging, "github.com/rama-kairi/go-term")
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	appLogger.Info("Starting Enhanced Terminal MCP Server", map[string]interface{}{
		"version":    cfg.Server.Version,
		"debug":      cfg.Server.Debug,
		"config_dir": cfg.Database.DataDir,
	})

	// Initialize database if enabled
	var db *database.DB
	if cfg.Database.Enable {
		var err error
		db, err = database.NewDB(cfg.Database.Path)
		if err != nil {
			log.Fatalf("Failed to initialize database: %v", err)
		}
		defer db.Close()

		appLogger.Info("Database initialized successfully", map[string]interface{}{
			"driver": cfg.Database.Driver,
			"path":   cfg.Database.Path,
		})
	}

	// Initialize streaming if enabled
	streamingEnabled := cfg.Streaming.Enable
	if streamingEnabled {
		appLogger.Info("Command streaming enabled", map[string]interface{}{
			"buffer_size": cfg.Streaming.BufferSize,
			"timeout":     cfg.Streaming.Timeout,
		})
	}

	// Create terminal session manager with enhanced features
	terminalManager := terminal.NewManager(cfg, appLogger, db)

	// M8: Initialize health endpoint if enabled
	if cfg.Monitoring.EnableMetrics {
		healthEndpoint := monitoring.NewHealthEndpoint(cfg.Monitoring.HealthCheckPort, nil)
		if db != nil {
			healthEndpoint.RegisterHealthCheck("database", db)
		}
		if err := healthEndpoint.Start(); err != nil {
			appLogger.Warn("Failed to start health endpoint", map[string]interface{}{
				"error": err.Error(),
				"port":  cfg.Monitoring.HealthCheckPort,
			})
		} else {
			appLogger.Info("Health endpoint started", map[string]interface{}{
				"port": cfg.Monitoring.HealthCheckPort,
			})
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				healthEndpoint.Stop(ctx)
			}()
		}
	}

	// Create terminal tools with enhanced features
	terminalTools := tools.NewTerminalTools(terminalManager, cfg, appLogger, db)

	// Create MCP server
	server := mcp.NewServer(&mcp.Implementation{
		Name:    cfg.Server.Name,
		Version: cfg.Server.Version,
	}, nil)

	// Register create terminal session tool with enhanced features
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_terminal_session",
		Description: "Create isolated terminal sessions for executing commands with persistent environment state. Each session maintains its own working directory, command history, and can run up to 3 background processes independently. Project IDs automatically organize sessions by directory. Essential for organized development workflow and resource management.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"name": {
					Type:        "string",
					Description: "Descriptive name for the terminal session (e.g., 'main-dev', 'testing', 'build-process'). 3-100 characters, alphanumeric with underscores and hyphens.",
				},
				"project_id": {
					Type:        "string",
					Description: "Optional: Custom project ID to group related sessions. Auto-generated from directory name if not provided.",
				},
				"working_dir": {
					Type:        "string",
					Description: "Optional: Starting directory for the session. Uses current directory if not specified.",
				},
			},
			Required: []string{"name"},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:        "Create Terminal Session",
			ReadOnlyHint: false,
		},
	}, terminalTools.CreateSession)

	// Register list terminal sessions tool with enhanced information
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_terminal_sessions",
		Description: "List all active terminal sessions with comprehensive status information including command statistics, background process counts, and project grouping. Essential for session management - use this to find available sessions for commands, check which sessions have running background processes, and monitor resource usage across projects.",
		InputSchema: &jsonschema.Schema{
			Type:       "object",
			Properties: map[string]*jsonschema.Schema{},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:        "List Terminal Sessions",
			ReadOnlyHint: true,
		},
	}, terminalTools.ListSessions)

	// Register run command tool for foreground commands only
	mcp.AddTool(server, &mcp.Tool{
		Name:        "run_command",
		Description: "Execute foreground commands in terminal sessions with immediate output. This tool waits for command completion and returns output. Use for: npm install, pip install, git commands, build tasks, tests, file operations, single-execution commands. DO NOT use for: dev servers (npm start, python manage.py runserver), file watchers (webpack --watch), or any long-running processes that don't exit automatically - use run_background_process instead. Includes intelligent package manager detection and security validation.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"session_id": {
					Type:        "string",
					Description: "Session ID to run the command in. Use list_terminal_sessions to see available sessions.",
				},
				"command": {
					Type:        "string",
					Description: "Command to execute in foreground that will complete and exit. Examples: 'npm install', 'git commit -m \"message\"', 'go build', 'pytest', 'ls -la'. This tool blocks until command finishes.",
				},
				"timeout": {
					Type:        "integer",
					Description: "Optional: Command timeout in seconds. Default: 60 seconds. Maximum: 300 seconds (5 minutes). Set to 0 to use default timeout.",
				},
			},
			Required: []string{"session_id", "command"},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Run Command",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			OpenWorldHint:   boolPtr(true),
		},
	}, terminalTools.RunCommand)

	// Register run background process tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "run_background_process",
		Description: "Start long-running processes in the background without blocking. Use ONLY for processes that run continuously and don't exit automatically: development servers (npm start, python manage.py runserver, go run main.go), file watchers (webpack --watch, npm run dev), background services, or monitoring processes. This tool returns immediately with a process ID for tracking. Maximum 3 background processes per session. Use run_command for commands that complete and exit (builds, installs, tests).",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"session_id": {
					Type:        "string",
					Description: "Session ID to run the background process in. Use list_terminal_sessions to see available sessions.",
				},
				"command": {
					Type:        "string",
					Description: "Long-running command to execute in background. Examples: 'npm start', 'python manage.py runserver', 'webpack --watch --mode development'. Command starts immediately and runs until manually terminated.",
				},
			},
			Required: []string{"session_id", "command"},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:         "Run Background Process",
			ReadOnlyHint:  false,
			OpenWorldHint: boolPtr(true),
		},
	}, terminalTools.RunBackgroundProcess)

	// Register list background processes tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_background_processes",
		Description: "List all running background processes across sessions and projects with comprehensive status information. Essential for monitoring active development servers, build watchers, and long-running tasks. Shows process IDs, running status, resource usage, and allows filtering by session or project. Use to identify processes that need termination or monitoring.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"session_id": {
					Type:        "string",
					Description: "Optional: Filter by specific session ID. Leave empty to list all background processes.",
				},
				"project_id": {
					Type:        "string",
					Description: "Optional: Filter by specific project ID. Leave empty to list all background processes.",
				},
			},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:        "List Background Processes",
			ReadOnlyHint: true,
		},
	}, terminalTools.ListBackgroundProcesses)

	// Register terminate background process tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "terminate_background_process",
		Description: "Stop and remove specific background processes by their process ID. Essential for resource management - use to terminate dev servers, build watchers, or stuck processes. Supports graceful termination (SIGTERM) or force kill (SIGKILL). Always terminate background processes when switching tasks or completing development work to free resources.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"session_id": {
					Type:        "string",
					Description: "Session ID containing the background process to terminate. Get from list_background_processes.",
				},
				"process_id": {
					Type:        "string",
					Description: "Process ID of the background process to terminate. Get from list_background_processes.",
				},
				"force": {
					Type:        "boolean",
					Description: "Whether to force kill the process (SIGKILL) instead of graceful termination (SIGTERM). Use true for stuck processes. Default: false.",
				},
			},
			Required: []string{"session_id", "process_id"},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Terminate Background Process",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
		},
	}, terminalTools.TerminateBackgroundProcess)

	// Register search history tool for command discovery
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_terminal_history",
		Description: "Search command history across all sessions and projects to find previously executed commands, analyze outputs, and troubleshoot issues. Essential for debugging, finding command patterns, and learning from past executions. Supports comprehensive filtering and time-based analysis.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"session_id": {
					Type:        "string",
					Description: "Filter by session ID. Leave empty to search all sessions. Get session IDs from list_terminal_sessions.",
				},
				"project_id": {
					Type:        "string",
					Description: "Filter by project ID. Leave empty to search all projects. Useful for project-specific command analysis.",
				},
				"command": {
					Type:        "string",
					Description: "Search for commands containing this text (case-insensitive). Example: 'npm' to find all npm commands.",
				},
				"output": {
					Type:        "string",
					Description: "Search for commands with output containing this text (case-insensitive). Useful for finding errors or specific output patterns.",
				},
				"success": {
					Type:        "boolean",
					Description: "Filter by command success status: true for successful commands, false for failed commands. Useful for debugging.",
				},
				"start_time": {
					Type:        "string",
					Description: "Find commands executed after this time (ISO 8601 format: 2006-01-02T15:04:05Z). Useful for time-based analysis.",
				},
				"end_time": {
					Type:        "string",
					Description: "Find commands executed before this time (ISO 8601 format: 2006-01-02T15:04:05Z). Combine with start_time for time ranges.",
				},
				"working_dir": {
					Type:        "string",
					Description: "Filter by working directory (partial match). Useful for finding commands executed in specific paths.",
				},
				"tags": {
					Type:        "array",
					Items:       &jsonschema.Schema{Type: "string"},
					Description: "Filter by tags (commands must have all specified tags). Used for categorizing and filtering commands.",
				},
				"limit": {
					Type:        "integer",
					Description: "Maximum results to return (default: 100, max: 1000). Use smaller values for focused results.",
				},
				"sort_by": {
					Type:        "string",
					Description: "Sort results by: 'time' (default), 'duration', or 'command'. Choose based on analysis needs.",
				},
				"sort_desc": {
					Type:        "boolean",
					Description: "Sort in descending order (default: true). Use false for chronological order.",
				},
				"include_output": {
					Type:        "boolean",
					Description: "Include full command output in results (default: false). Warning: may return large amounts of data.",
				},
			},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:        "Search Terminal History",
			ReadOnlyHint: true,
		},
	}, terminalTools.SearchHistory)

	// Register delete session tool for session management
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_session",
		Description: "Delete terminal sessions individually or by project with confirmation requirement. Essential for resource cleanup - removes session history, terminates background processes, and frees system resources. Use after completing work to maintain clean development environment. Requires explicit confirmation to prevent accidental deletion.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"session_id": {
					Type:        "string",
					Description: "Session ID to delete. Leave empty to delete by project_id instead. Get session IDs from list_terminal_sessions.",
				},
				"project_id": {
					Type:        "string",
					Description: "Delete all sessions for this project. Leave empty to delete by session_id instead. Useful for cleaning up entire project workspaces.",
				},
				"confirm": {
					Type:        "boolean",
					Description: "Must be true to confirm deletion and prevent accidents. Required safety measure.",
				},
			},
			Required: []string{"confirm"},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Delete Session",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
		},
	}, terminalTools.DeleteSession)

	// Register background process monitoring tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "check_background_process",
		Description: "Monitor specific background processes to check their status, output, and health. Use to track development servers, build processes, and other long-running tasks started with run_background_process. Returns real-time status, output logs, error messages, and resource usage. Essential for debugging background processes and monitoring their health.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"session_id": {
					Type:        "string",
					Description: "Session ID where the background process is running. Get from list_terminal_sessions.",
				},
				"process_id": {
					Type:        "string",
					Description: "Optional: Specific process ID to check. If not provided, checks the latest background process in the session. Get process IDs from list_background_processes.",
				},
			},
			Required: []string{"session_id"},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:        "Check Background Process",
			ReadOnlyHint: true,
		},
	}, terminalTools.CheckBackgroundProcess)

	// Register resource monitoring tools
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_resource_status",
		Description: "Get comprehensive resource usage and monitoring status including memory consumption, goroutine counts, and potential leak detection. Essential for monitoring MCP server health, tracking resource usage patterns, and identifying performance issues. Use regularly during heavy workloads or when experiencing performance degradation.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"force_gc": {
					Type:        "boolean",
					Description: "Force garbage collection before retrieving metrics to get accurate memory usage. Useful for detecting memory leaks. Default: false.",
				},
			},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:        "Get Resource Status",
			ReadOnlyHint: true,
		},
	}, terminalTools.GetResourceStatus)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "check_resource_leaks",
		Description: "Analyze current resource usage to detect potential memory or goroutine leaks with detailed diagnostic analysis. Provides leak detection, resource growth analysis, and specific recommendations for addressing resource issues. Use when experiencing performance problems or after long-running operations.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"threshold": {
					Type:        "integer",
					Description: "Custom threshold for goroutine leak detection (number of goroutines increase to consider suspicious). Default: 50 goroutines.",
				},
			},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:        "Check Resource Leaks",
			ReadOnlyHint: true,
		},
	}, terminalTools.CheckResourceLeaks)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "force_resource_cleanup",
		Description: "Perform aggressive resource cleanup to address potential leaks and free system resources. Includes garbage collection, inactive session cleanup, and background process termination. Use when resource leaks are detected or system performance is degraded. Requires confirmation to prevent accidental cleanup.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"cleanup_type": {
					Type:        "string",
					Description: "Type of cleanup: 'gc' (garbage collection only), 'sessions' (cleanup inactive sessions), 'processes' (terminate background processes), or 'all' (comprehensive cleanup). Default: 'gc'.",
				},
				"confirm": {
					Type:        "boolean",
					Description: "Must be true to confirm cleanup and prevent accidental resource cleanup. Required safety measure.",
				},
			},
			Required: []string{"confirm"},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Force Resource Cleanup",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
		},
	}, terminalTools.ForceCleanup)

	// F1: Register command template tools
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_command_template",
		Description: "Create a reusable command template with variable placeholders. Templates can include variables like {{name}} that get replaced when the template is used. Useful for frequently used commands with slight variations.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"name": {
					Type:        "string",
					Description: "Unique name for the template (e.g., 'build-docker', 'deploy-staging')",
				},
				"command": {
					Type:        "string",
					Description: "Command template with optional {{variable}} placeholders (e.g., 'docker build -t {{image_name}} .')",
				},
				"description": {
					Type:        "string",
					Description: "Description of what the template does",
				},
				"category": {
					Type:        "string",
					Description: "Optional category for organizing templates (e.g., 'docker', 'git', 'deployment')",
				},
			},
			Required: []string{"name", "command"},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:        "Create Command Template",
			ReadOnlyHint: false,
		},
	}, terminalTools.CreateCommandTemplate)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_command_templates",
		Description: "List all saved command templates, optionally filtered by category.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"category": {
					Type:        "string",
					Description: "Optional category to filter templates",
				},
			},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:        "List Command Templates",
			ReadOnlyHint: true,
		},
	}, terminalTools.ListCommandTemplates)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "expand_command_template",
		Description: "Expand a command template by replacing variable placeholders with actual values.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"template_name": {
					Type:        "string",
					Description: "Name of the template to expand",
				},
				"variables": {
					Type:        "object",
					Description: "Map of variable names to values (e.g., {\"image_name\": \"myapp:latest\"})",
				},
			},
			Required: []string{"template_name"},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:        "Expand Command Template",
			ReadOnlyHint: true,
		},
	}, terminalTools.ExpandCommandTemplate)

	// F6: Register output search tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_command_output",
		Description: "Search through command outputs for specific patterns or text. Supports regex patterns and case-insensitive matching.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"session_id": {
					Type:        "string",
					Description: "Session ID to search in",
				},
				"pattern": {
					Type:        "string",
					Description: "Search pattern (text or regex)",
				},
				"is_regex": {
					Type:        "boolean",
					Description: "Whether the pattern is a regular expression (default: false)",
				},
				"case_sensitive": {
					Type:        "boolean",
					Description: "Whether the search is case-sensitive (default: false)",
				},
				"context_lines": {
					Type:        "integer",
					Description: "Number of lines to include before and after matches (default: 2)",
				},
				"max_results": {
					Type:        "integer",
					Description: "Maximum number of results to return (default: 50)",
				},
			},
			Required: []string{"session_id", "pattern"},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:        "Search Command Output",
			ReadOnlyHint: true,
		},
	}, terminalTools.SearchCommandOutput)

	// F2: Register session snapshot tools
	mcp.AddTool(server, &mcp.Tool{
		Name:        "save_session_snapshot",
		Description: "Save a snapshot of the current session state including environment, working directory, and command history.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"session_id": {
					Type:        "string",
					Description: "Session ID to snapshot",
				},
				"name": {
					Type:        "string",
					Description: "Name for the snapshot",
				},
				"description": {
					Type:        "string",
					Description: "Optional description of what this snapshot represents",
				},
			},
			Required: []string{"session_id", "name"},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:        "Save Session Snapshot",
			ReadOnlyHint: false,
		},
	}, terminalTools.SaveSessionSnapshot)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_session_snapshots",
		Description: "List all saved session snapshots.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"session_id": {
					Type:        "string",
					Description: "Optional: filter by session ID",
				},
			},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:        "List Session Snapshots",
			ReadOnlyHint: true,
		},
	}, terminalTools.ListSessionSnapshots)

	// F7: Register process chain tools
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_process_chain",
		Description: "Create a chain of background processes that run in sequence with dependency management. Processes in the chain start one after another, optionally waiting for readiness signals.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"session_id": {
					Type:        "string",
					Description: "Session ID to run processes in",
				},
				"name": {
					Type:        "string",
					Description: "Name for the process chain",
				},
				"description": {
					Type:        "string",
					Description: "Description of what this chain does",
				},
				"processes": {
					Type:        "array",
					Description: "List of processes to run in order. Each has: name, command, ready_pattern (optional), wait_seconds (optional)",
					Items: &jsonschema.Schema{
						Type: "object",
						Properties: map[string]*jsonschema.Schema{
							"name": {
								Type:        "string",
								Description: "Name of this process in the chain",
							},
							"command": {
								Type:        "string",
								Description: "Command to execute",
							},
							"ready_pattern": {
								Type:        "string",
								Description: "Pattern in output indicating process is ready (optional)",
							},
							"wait_seconds": {
								Type:        "integer",
								Description: "Seconds to wait after starting before proceeding to next process (optional)",
							},
						},
						Required: []string{"name", "command"},
					},
				},
			},
			Required: []string{"session_id", "name", "processes"},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:        "Create Process Chain",
			ReadOnlyHint: false,
		},
	}, terminalTools.CreateProcessChain)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "start_process_chain",
		Description: "Start executing a previously created process chain.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"chain_id": {
					Type:        "string",
					Description: "ID of the chain to start",
				},
			},
			Required: []string{"chain_id"},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:        "Start Process Chain",
			ReadOnlyHint: false,
		},
	}, terminalTools.StartProcessChain)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_process_chain_status",
		Description: "Get the current status of a process chain including status of each process in the chain.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"chain_id": {
					Type:        "string",
					Description: "ID of the chain to check",
				},
			},
			Required: []string{"chain_id"},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:        "Get Process Chain Status",
			ReadOnlyHint: true,
		},
	}, terminalTools.GetProcessChainStatus)

	// Environment variable management tools (M4)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_session_environment",
		Description: "Set or update environment variables for a terminal session. These variables will be available to all commands executed in the session.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"session_id": {
					Type:        "string",
					Description: "The session ID to set environment variables for",
				},
				"variables": {
					Type:        "object",
					Description: "Map of environment variable names to values",
					AdditionalProperties: &jsonschema.Schema{
						Type: "string",
					},
				},
			},
			Required: []string{"session_id", "variables"},
		},
		Annotations: &mcp.ToolAnnotations{
			Title: "Set Session Environment Variables",
		},
	}, terminalTools.SetSessionEnvironment)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_session_environment",
		Description: "Get environment variables for a terminal session. Can retrieve all variables or a specific one.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"session_id": {
					Type:        "string",
					Description: "The session ID to get environment variables from",
				},
				"key": {
					Type:        "string",
					Description: "Specific environment variable key to retrieve. If not provided, returns all variables",
				},
			},
			Required: []string{"session_id"},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:        "Get Session Environment Variables",
			ReadOnlyHint: true,
		},
	}, terminalTools.GetSessionEnvironment)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "unset_session_environment",
		Description: "Remove environment variables from a terminal session.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"session_id": {
					Type:        "string",
					Description: "The session ID to unset environment variables from",
				},
				"keys": {
					Type:        "array",
					Description: "List of environment variable keys to remove",
					Items: &jsonschema.Schema{
						Type: "string",
					},
				},
			},
			Required: []string{"session_id", "keys"},
		},
		Annotations: &mcp.ToolAnnotations{
			Title: "Unset Session Environment Variables",
		},
	}, terminalTools.UnsetSessionEnvironment)

	// M9: Session Activity Metrics tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_session_activity_metrics",
		Description: "Get detailed activity metrics for terminal sessions including command counts, success rates, execution times, command type distribution, and error categories.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"session_id": {
					Type:        "string",
					Description: "Session ID to get metrics for. If not provided, returns metrics for all sessions.",
				},
			},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:        "Get Session Activity Metrics",
			ReadOnlyHint: true,
		},
	}, terminalTools.GetSessionActivityMetrics)

	// M10: Command Execution Tracing tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_traces",
		Description: "Get OpenTelemetry-compatible trace spans for command execution. Useful for debugging and performance analysis.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"limit": {
					Type:        "integer",
					Description: "Maximum number of spans to return (default: 100, max: 1000)",
				},
				"trace_id": {
					Type:        "string",
					Description: "Filter by specific trace ID",
				},
			},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:        "Get Command Traces",
			ReadOnlyHint: true,
		},
	}, terminalTools.GetTraces)

	appLogger.Info("Terminal MCP Server registered all tools successfully", map[string]interface{}{
		"tools_count": 26,
	})
	appLogger.Info("Available tools:")
	appLogger.Info("  - create_terminal_session: Create isolated terminal sessions for organized project work")
	appLogger.Info("  - list_terminal_sessions: View all sessions with status and statistics")
	appLogger.Info("  - run_command: Execute foreground commands with immediate output")
	appLogger.Info("  - run_background_process: Start long-running processes in background")
	appLogger.Info("  - list_background_processes: List all running background processes")
	appLogger.Info("  - terminate_background_process: Stop specific background processes")
	appLogger.Info("  - search_terminal_history: Find and analyze previous commands across projects")
	appLogger.Info("  - delete_session: Clean up sessions individually or by project")
	appLogger.Info("  - check_background_process: Monitor specific background processes")
	appLogger.Info("  - get_resource_status: Monitor server resource usage and health")
	appLogger.Info("  - check_resource_leaks: Detect and analyze potential resource leaks")
	appLogger.Info("  - force_resource_cleanup: Perform aggressive resource cleanup when needed")

	// Set up graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		appLogger.Info("Received shutdown signal, cleaning up...")

		// Shutdown terminal manager (this will close all sessions)
		terminalManager.Shutdown()

		cancel()
	}()

	// Start the MCP server using stdio transport
	appLogger.Info("Enhanced Terminal MCP Server is now running and waiting for requests...")
	appLogger.Info("Features: Project-based sessions, Command history tracking, Advanced search, Security validation")
	appLogger.Info("Configuration:", map[string]interface{}{
		"config_directory": cfg.Database.DataDir,
		"database_path":    cfg.Database.Path,
		"max_sessions":     cfg.Session.MaxSessions,
		"sandbox_enabled":  cfg.Security.EnableSandbox,
		"network_access":   cfg.Security.AllowNetworkAccess,
	})
	appLogger.Info("Use stdio transport to communicate with this server")

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		appLogger.Error("Server error", err)
		os.Exit(1)
	}

	appLogger.Info("Terminal MCP Server shutdown completed")
}
