# Go-Term Copilot Instructions

## Project Overview
go-term is a Model Context Protocol (MCP) server for terminal session management. It enables AI agents to execute shell commands, manage background processes, and track command history with security validation.

## Architecture

### Core Components
```
main.go                     # MCP server setup, tool registration via mcp.AddTool()
internal/
├── config/config.go        # Configuration with defaults (Config, SessionConfig, SecurityConfig)
├── terminal/session.go     # Session/BackgroundProcess management, command execution
├── tools/                   # MCP tool implementations (one file per tool category)
├── database/database.go    # SQLite persistence for sessions and command history
├── monitoring/             # Resource monitoring, health endpoints
└── errors/errors.go        # Standardized ErrorCode constants and TerminalError type
```

### Data Flow
1. MCP client calls tool → `main.go` routes to `TerminalTools` method
2. `TerminalTools` validates input, checks rate limits, delegates to `terminal.Manager`
3. `Manager` executes command in `Session`, tracks background processes
4. Results stored in SQLite via `database.DB`, returned as JSON to client

## Code Patterns

### Adding New MCP Tools
1. Define `Args` and `Result` structs in appropriate `internal/tools/*_tools.go`
2. Implement handler: `func (t *TerminalTools) ToolName(ctx context.Context, req *mcp.CallToolRequest, args ArgsType) (*mcp.CallToolResult, ResultType, error)`
3. Register in `main.go` using `mcp.AddTool()` with jsonschema input validation
4. Always call `t.CheckRateLimit()` at handler start

### Error Handling Pattern
```go
// Use createErrorResult for user-facing errors with actionable tips
return createErrorResult(fmt.Sprintf("Session not found: %v. Tip: Use 'list_terminal_sessions' to see available sessions.", err)), ResultType{}, nil

// Use errors.TerminalError for structured internal errors
import terrors "github.com/rama-kairi/go-term/internal/errors"
return nil, ResultType{}, terrors.NewSessionError(terrors.ErrCodeSessionNotFound, "session not found", sessionID)
```

### Thread Safety
- Use `sync.RWMutex` for shared state (see `BackgroundProcess.Mutex`, `Session.mutex`)
- Lock order: Session mutex → BackgroundProcess mutex
- Use defer for unlock: `defer s.mutex.RUnlock()`

### Constants (internal/tools/constants.go)
Use defined constants instead of magic numbers:
- `DefaultTimeout`, `MaxTimeout` for command timeouts
- `MaxBackgroundProcesses = 3` per session
- `DefaultRateLimitPerMinute = 60`

## Build & Test Commands

```bash
# Build
go build .

# Run all tests with verbose output
go test -v ./...

# Run tests with coverage
go test -v -cover ./...

# Format code (requires gofumpt)
gofumpt -w .

# Lint (requires staticcheck)
staticcheck ./...

# Run the server
go run . --debug
```

## Testing Patterns

Test files follow `*_test.go` naming. Key test utilities:
```go
// internal/tools/terminal_tools_test.go
tools, manager, tempDir := setupTestEnvironment(t)
defer os.RemoveAll(tempDir)
```

Tests use real temp directories and SQLite databases - no extensive mocking.

## Configuration

Configuration loaded via `config.LoadConfig()` with these layers:
1. `config.DefaultConfig()` base values
2. JSON config file (optional `-config` flag)
3. Command-line flags (`-debug`)

Key config sections: `Server`, `Session`, `Database`, `Security`, `Monitoring`

## Security Validation

`SecurityValidator` in `internal/tools/terminal_tools.go` validates commands before execution:
- Blocked commands list in `SecurityConfig.BlockedCommands`
- Command length limits
- Shell injection prevention via `shellEscape()` in session.go

## Tracing & Logging

- Use `t.tracer.StartSpanWithKind()` for operation tracing
- Logger methods: `Info()`, `Warn()`, `Error()` with structured fields
- Security events: `t.logger.LogSecurityEvent()`
