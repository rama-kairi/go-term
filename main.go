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
		Description: "Create a new terminal session for executing commands. Sessions isolate work by project and maintain persistent environment state. Use this to start organized terminal work within projects - project IDs are automatically generated from the current directory. Each session tracks command history and maintains independent working directories.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"name": {
					Type:        "string",
					Description: "Descriptive name for the terminal session (e.g., 'main-dev', 'testing', 'build-process').",
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
		Description: "List all active terminal sessions with status information, command statistics, and project grouping. Use this to see which sessions are available for running commands, check session health, and avoid conflicts with busy terminals running background processes.",
		InputSchema: &jsonschema.Schema{
			Type:       "object",
			Properties: map[string]*jsonschema.Schema{},
		},
	}, terminalTools.ListSessions)

	// Register run command tool for foreground commands only
	mcp.AddTool(server, &mcp.Tool{
		Name:        "run_command",
		Description: "Execute foreground commands in terminal sessions with immediate output. This tool only runs blocking/foreground commands that complete and return output. For long-running processes like development servers, use run_background_process instead. Includes intelligent package manager detection and security validation.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"session_id": {
					Type:        "string",
					Description: "Session ID to run the command in. Use list_terminal_sessions to see available sessions.",
				},
				"command": {
					Type:        "string",
					Description: "Command to execute in foreground. This tool waits for command completion and returns output. Use run_background_process for long-running commands.",
				},
			},
			Required: []string{"session_id", "command"},
		},
	}, terminalTools.RunCommand)

	// Register run background process tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "run_background_process",
		Description: "Start long-running processes in the background without blocking. Use this for development servers, build watchers, and other processes that need to run continuously. No command validation is performed - the agent decides what to run. Maximum 3 background processes per session.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"session_id": {
					Type:        "string",
					Description: "Session ID to run the background process in. Use list_terminal_sessions to see available sessions.",
				},
				"command": {
					Type:        "string",
					Description: "Command to execute as a background process. No validation is performed.",
				},
			},
			Required: []string{"session_id", "command"},
		},
	}, terminalTools.RunBackgroundProcess)

	// Register list background processes tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_background_processes",
		Description: "List all running background processes across sessions and projects with comprehensive status information. Use this to monitor which background processes are active, check their status, and avoid resource conflicts.",
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
		Description: "Stop and remove specific background processes by their process ID. Use this to clean up background processes that are no longer needed or are consuming too many resources.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"session_id": {
					Type:        "string",
					Description: "Session ID containing the background process to terminate.",
				},
				"process_id": {
					Type:        "string",
					Description: "Process ID of the background process to terminate.",
				},
				"force": {
					Type:        "boolean",
					Description: "Whether to force kill the process (SIGKILL) instead of graceful termination (SIGTERM). Default: false.",
				},
			},
			Required: []string{"session_id", "process_id"},
		},
	}, terminalTools.TerminateBackgroundProcess)

	// Register search history tool for command discovery
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_terminal_history",
		Description: "Search command history across all sessions and projects to find previously executed commands, analyze outputs, and troubleshoot issues. Supports filtering by project, session, command text, output content, success status, and time ranges. Essential for debugging and finding patterns in command execution.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"session_id": {
					Type:        "string",
					Description: "Filter by session ID. Leave empty to search all sessions.",
				},
				"project_id": {
					Type:        "string",
					Description: "Filter by project ID. Leave empty to search all projects.",
				},
				"command": {
					Type:        "string",
					Description: "Search for commands containing this text (case-insensitive).",
				},
				"output": {
					Type:        "string",
					Description: "Search for commands with output containing this text (case-insensitive).",
				},
				"success": {
					Type:        "boolean",
					Description: "Filter by success status: true for successful, false for failed commands.",
				},
				"start_time": {
					Type:        "string",
					Description: "Find commands after this time (ISO 8601: 2006-01-02T15:04:05Z).",
				},
				"end_time": {
					Type:        "string",
					Description: "Find commands before this time (ISO 8601: 2006-01-02T15:04:05Z).",
				},
				"working_dir": {
					Type:        "string",
					Description: "Filter by working directory (partial match).",
				},
				"tags": {
					Type:        "array",
					Items:       &jsonschema.Schema{Type: "string"},
					Description: "Filter by tags (commands must have all specified tags).",
				},
				"limit": {
					Type:        "integer",
					Description: "Maximum results to return (default: 100, max: 1000).",
				},
				"sort_by": {
					Type:        "string",
					Description: "Sort by: 'time' (default), 'duration', or 'command'.",
				},
				"sort_desc": {
					Type:        "boolean",
					Description: "Sort in descending order (default: true).",
				},
				"include_output": {
					Type:        "boolean",
					Description: "Include command output in results (default: false).",
				},
			},
		},
	}, terminalTools.SearchHistory)

	// Register delete session tool for session management
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_session",
		Description: "Delete terminal sessions individually or by project with confirmation requirement. Use this to clean up completed work and free resources. Requires explicit confirmation to prevent accidental deletion of active sessions.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"session_id": {
					Type:        "string",
					Description: "Session ID to delete. Leave empty to delete by project_id instead.",
				},
				"project_id": {
					Type:        "string",
					Description: "Delete all sessions for this project. Leave empty to delete by session_id instead.",
				},
				"confirm": {
					Type:        "boolean",
					Description: "Must be true to confirm deletion and prevent accidents.",
				},
			},
			Required: []string{"confirm"},
		},
	}, terminalTools.DeleteSession)

	// Register background process monitoring tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "check_background_process",
		Description: "Monitor specific background processes to check their status, output, and health. Use this to track development servers, build processes, and other long-running tasks started with run_background_process.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"session_id": {
					Type:        "string",
					Description: "Session ID where the background process is running.",
				},
				"process_id": {
					Type:        "string",
					Description: "Optional: Specific process ID. If not provided, checks the latest background process.",
				},
			},
			Required: []string{"session_id"},
		},
	}, terminalTools.CheckBackgroundProcess)

	appLogger.Info("Terminal MCP Server registered all tools successfully", map[string]interface{}{
		"tools_count": 9,
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
