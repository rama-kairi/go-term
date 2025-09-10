package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/google/jsonschema-go/jsonschema"
	"terminal-mcp/internal/config"
	"terminal-mcp/internal/logger"
	"terminal-mcp/internal/terminal"
	"terminal-mcp/internal/tools"
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
	appLogger, err := logger.NewLogger(&cfg.Logging, "terminal-mcp")
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	appLogger.Info("Starting Enhanced Terminal MCP Server", map[string]interface{}{
		"version": cfg.Server.Version,
		"debug":   cfg.Server.Debug,
	})

	// Create terminal session manager with enhanced features
	terminalManager := terminal.NewManager(cfg, appLogger)

	// Create terminal tools with enhanced features
	terminalTools := tools.NewTerminalTools(terminalManager, cfg, appLogger)

	// Create MCP server
	server := mcp.NewServer(&mcp.Implementation{
		Name:    cfg.Server.Name,
		Version: cfg.Server.Version,
	}, nil)

	// Register create terminal session tool with enhanced features
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_terminal_session",
		Description: "Create a new terminal session with project association and comprehensive tracking. Project IDs are auto-generated based on current directory (format: folder_name_with_underscores_RANDOM). Use this to start organized terminal work within projects.",
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
		Description: "List all existing terminal sessions with comprehensive information including project association, command statistics, and session health. Use this to see all active sessions and their status.",
		InputSchema: &jsonschema.Schema{
			Type:       "object",
			Properties: map[string]*jsonschema.Schema{},
		},
	}, terminalTools.ListSessions)

	// Register run command tool with enhanced tracking
	mcp.AddTool(server, &mcp.Tool{
		Name:        "run_command",
		Description: "Execute a command in a specific terminal session with comprehensive tracking and security validation. All commands are logged to session history for later search and analysis. Working directory changes persist across commands.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"session_id": {
					Type:        "string",
					Description: "The UUID4 identifier of the terminal session to run the command in. Use list_terminal_sessions to see available sessions.",
				},
				"command": {
					Type:        "string",
					Description: "The command to execute in the terminal session. Will be validated for security before execution. Directory changes (cd) persist across commands.",
				},
			},
			Required: []string{"session_id", "command"},
		},
	}, terminalTools.RunCommand)

	// Register search history tool for command discovery
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_terminal_history",
		Description: "Search through command history across all terminal sessions and projects. Use this to quickly find previously executed commands, check command outputs, analyze patterns, or troubleshoot issues. Supports filtering by project, session, command text, output text, success status, time range, and more.",
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

	appLogger.Info("Terminal MCP Server registered all tools successfully", map[string]interface{}{
		"tools_count": 4,
	})
	appLogger.Info("Available tools:")
	appLogger.Info("  - create_terminal_session: Create a new terminal session with project association and comprehensive tracking")
	appLogger.Info("  - list_terminal_sessions: List all existing terminal sessions with detailed information and statistics")
	appLogger.Info("  - run_command: Execute a command in a specific terminal session with full history tracking")
	appLogger.Info("  - search_terminal_history: Search through command history across all sessions and projects")

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
	appLogger.Info("Use stdio transport to communicate with this server")

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		appLogger.Error("Server error", err)
		os.Exit(1)
	}

	appLogger.Info("Terminal MCP Server shutdown completed")
}
