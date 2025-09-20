package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rama-kairi/go-term/internal/config"
	"github.com/rama-kairi/go-term/internal/database"
	"github.com/rama-kairi/go-term/internal/logger"
	"github.com/rama-kairi/go-term/internal/terminal"
	"github.com/rama-kairi/go-term/internal/tools"
)

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
	}, terminalTools.CreateSession)

	// Register list terminal sessions tool with enhanced information
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_terminal_sessions",
		Description: "List all active terminal sessions with comprehensive status information including command statistics, background process counts, and project grouping. Essential for session management - use this to find available sessions for commands, check which sessions have running background processes, and monitor resource usage across projects.",
		InputSchema: &jsonschema.Schema{
			Type:       "object",
			Properties: map[string]*jsonschema.Schema{},
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
	}, terminalTools.ForceCleanup)

	appLogger.Info("Terminal MCP Server registered all tools successfully", map[string]interface{}{
		"tools_count": 12,
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
