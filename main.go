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
		Description: "Create a new terminal session with project association and comprehensive tracking. Project IDs are auto-generated based on current directory (format: folder_name_with_underscores_RANDOM). Use this to start organized terminal work within projects. SOLVES VSCode Copilot Issues: (1) Session isolation prevents interference between concurrent tasks, (2) Reliable session management without state loss, (3) Project-based organization for better workflow management. Each session maintains independent working directory and environment state.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"name": {
					Type:        "string",
					Description: "The name of the terminal session to create. Should be descriptive and meaningful for your project work.",
				},
				"project_id": {
					Type:        "string",
					Description: "Optional project ID to associate with this session. If not provided, will be auto-generated based on current directory. Format: folder_name_with_underscores_RANDOM (e.g., my_project_a7b3c9)",
				},
				"working_dir": {
					Type:        "string",
					Description: "Optional working directory for the session. If not provided, uses current directory. This affects project ID generation.",
				},
			},
			Required: []string{"name"},
		},
	}, terminalTools.CreateSession)

	// Register list terminal sessions tool with enhanced information
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_terminal_sessions",
		Description: "List all existing terminal sessions with comprehensive information including project association, command statistics, and session health. Use this to see all active sessions and their status. SOLVES VSCode Copilot Issues: (1) Clear session visibility and management, (2) Prevents confusion about which terminals are running what processes, (3) Shows command execution statistics and success rates for troubleshooting, (4) Project-based grouping for better organization. Helps avoid the common VSCode Copilot issue of running commands in terminals with active background processes.",
		InputSchema: &jsonschema.Schema{
			Type:       "object",
			Properties: map[string]*jsonschema.Schema{},
		},
	}, terminalTools.ListSessions)

	// Register run command tool with enhanced tracking
	mcp.AddTool(server, &mcp.Tool{
		Name:        "run_command",
		Description: "Execute a command in a specific terminal session with comprehensive tracking, intelligent package management, and automatic background execution for long-running processes. KEY FEATURES: (1) AUTOMATIC BACKGROUND DETECTION - Long-running processes (dev servers, HTTP servers, file watchers) are automatically detected and executed in isolated background sessions to prevent blocking, (2) Intelligent package manager optimization (prefers bun for Node.js, uv for Python), (3) Security validation and command enhancement, (4) Complete command history tracking. AUTOMATIC BACKGROUND TRIGGERS: Python scripts containing 'server', 'http_server', 'app.py', Flask/Django/FastAPI apps; Node.js scripts with 'server.js', 'app.js', Express/Fastify apps; Commands with 'dev', 'serve', 'start', 'watch'. SOLVES VSCode Copilot Issues: (1) Prevents hanging on long-running processes, (2) Reliable output capture, (3) Proper session isolation, (4) Background process management. Use 'is_background: false' to force foreground execution if needed.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"session_id": {
					Type:        "string",
					Description: "The UUID4 identifier of the terminal session to run the command in. Use list_terminal_sessions to see available sessions.",
				},
				"command": {
					Type:        "string",
					Description: "The command to execute. AUTOMATIC BACKGROUND DETECTION: Commands containing server scripts (http_server.py, server.js, app.py, dev servers) are automatically executed in background sessions to prevent blocking. Enhanced with intelligent package manager detection (prefers bun for Node.js, uv for Python). Directory changes (cd) persist across commands.",
				},
				"is_background": {
					Type:        "boolean",
					Description: "Optional: Override automatic background detection. Set to 'false' to force foreground execution of detected long-running processes, or 'true' to force background execution of regular commands.",
				},
				"timeout_test": {
					Type:        "boolean",
					Description: "Optional: Test command with 10-second timeout to verify responsiveness before full execution. Useful for potentially hanging commands.",
				},
			},
			Required: []string{"session_id", "command"},
		},
	}, terminalTools.RunCommand)

	// Register search history tool for command discovery
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_terminal_history",
		Description: "Search through command history across all terminal sessions and projects. Use this to quickly find previously executed commands, check command outputs, analyze patterns, or troubleshoot issues. Supports filtering by project, session, command text, output text, success status, time range, and more. SOLVES VSCode Copilot Issues: (1) Persistent command history that doesn't get lost between sessions, (2) Advanced search capabilities to find specific commands or outputs, (3) Project-based filtering for organized workflow management, (4) Success/failure analysis for debugging. Unlike VSCode's limited terminal history, this provides comprehensive searchable records.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"session_id": {
					Type:        "string",
					Description: "Filter by specific session ID. Leave empty to search all sessions.",
				},
				"project_id": {
					Type:        "string",
					Description: "Filter by specific project ID. Leave empty to search all projects. Use this to focus on commands from a particular project.",
				},
				"command": {
					Type:        "string",
					Description: "Search for commands containing this text (case-insensitive partial match). Use this to find specific commands or command patterns.",
				},
				"output": {
					Type:        "string",
					Description: "Search for commands with output containing this text (case-insensitive partial match). Useful for finding commands that produced specific results or errors.",
				},
				"success": {
					Type:        "boolean",
					Description: "Filter by success status: true for successful commands, false for failed commands, omit for all commands.",
				},
				"start_time": {
					Type:        "string",
					Description: "Find commands executed after this time (ISO 8601 format: 2006-01-02T15:04:05Z). Use this to focus on recent activity.",
				},
				"end_time": {
					Type:        "string",
					Description: "Find commands executed before this time (ISO 8601 format: 2006-01-02T15:04:05Z). Use this to limit search to a specific time period.",
				},
				"working_dir": {
					Type:        "string",
					Description: "Filter by working directory path (partial match). Use this to find commands executed in specific directories.",
				},
				"tags": {
					Type:        "array",
					Items:       &jsonschema.Schema{Type: "string"},
					Description: "Filter by tags (commands must have all specified tags). Tags can be added to commands for better organization.",
				},
				"limit": {
					Type:        "integer",
					Description: "Maximum number of results to return (default: 100, max: 1000). Use lower values for faster responses.",
				},
				"sort_by": {
					Type:        "string",
					Description: "Sort results by: 'time' (default), 'duration', or 'command'. Time sorts by execution time, duration by how long commands took.",
				},
				"sort_desc": {
					Type:        "boolean",
					Description: "Sort in descending order (default: true for time-based sorting). Use false for ascending order.",
				},
				"include_output": {
					Type:        "boolean",
					Description: "Include command output in results (default: false to reduce response size). Set to true when searching by output content or when you need to see command results.",
				},
			},
		},
	}, terminalTools.SearchHistory)

	// Register delete session tool for session management
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_session",
		Description: "Delete terminal sessions (individual or all sessions for a project) with confirmation requirement. Use this to clean up old sessions or remove all sessions for a completed project. Requires explicit confirmation to prevent accidental deletion.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"session_id": {
					Type:        "string",
					Description: "The UUID4 identifier of the session to delete. Leave empty to delete by project. Cannot be used together with project_id.",
				},
				"project_id": {
					Type:        "string",
					Description: "Delete all sessions for this project ID. Leave empty to delete by session ID. Cannot be used together with session_id.",
				},
				"confirm": {
					Type:        "boolean",
					Description: "Confirmation flag to prevent accidental deletion. Must be set to true to proceed with deletion.",
				},
			},
			Required: []string{"confirm"},
		},
	}, terminalTools.DeleteSession)

	// Register background process monitoring tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "check_background_process",
		Description: "Check the output and status of background processes for agents. This allows agents to monitor long-running processes like development servers, HTTP servers, and other background tasks that were started with automatic background detection. SOLVES VSCode Copilot Issues: (1) Agents can check output of background processes without losing context, (2) Monitor status of dev servers and long-running tasks, (3) Retrieve output from processes that were started in background sessions.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"session_id": {
					Type:        "string",
					Description: "The UUID4 identifier of the session running the background process.",
				},
				"process_id": {
					Type:        "string",
					Description: "Optional background process ID. If not provided will check the latest background process for the session.",
				},
			},
			Required: []string{"session_id"},
		},
	}, terminalTools.CheckBackgroundProcess)

	appLogger.Info("Terminal MCP Server registered all tools successfully", map[string]interface{}{
		"tools_count": 6,
	})
	appLogger.Info("Available tools:")
	appLogger.Info("  - create_terminal_session: Create a new terminal session with project association and comprehensive tracking")
	appLogger.Info("  - list_terminal_sessions: List all existing terminal sessions with detailed information and statistics")
	appLogger.Info("  - run_command: Execute a command in a specific terminal session with full history tracking")
	appLogger.Info("  - search_terminal_history: Search through command history across all sessions and projects")
	appLogger.Info("  - delete_session: Delete terminal sessions (individual or project-wide) with confirmation")
	appLogger.Info("  - check_background_process: Check output and status of background processes for agents")

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
