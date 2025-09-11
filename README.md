# Terminal MCP

A robust Model Context Protocol (MCP) server for terminal session management with enhanced features.

## Features

- **Project-based Session Management**: Auto-generated project IDs based on working directory
- **Command History Tracking**: Persistent command history with comprehensive metadata
- **Advanced Search**: Search command history across sessions and projects
- **Security Validation**: Comprehensive input validation and security monitoring
- **Structured Logging**: JSON-formatted logging with configurable output
- **Configuration Management**: Environment variable configuration support

## Installation

### Build from Source

```bash
go build -o github.com/rama-kairi/go-term .
go install .
```

### Requirements

- Go 1.19 or later
- MCP-compatible client

## Usage

The Terminal MCP server communicates via stdio using the Model Context Protocol:

```bash
github.com/rama-kairi/go-term
```

### MCP Tools Available

1. **`create_terminal_session`** - Create a new terminal session with project association
2. **`list_terminal_sessions`** - List all active terminal sessions
3. **`run_command`** - Execute commands with history tracking
4. **`search_terminal_history`** - Search command history across sessions

### Configuration

Configure the server using environment variables:

```bash
export TERMINAL_MCP_LOG_LEVEL=info
export TERMINAL_MCP_MAX_SESSIONS=10
export TERMINAL_MCP_LOG_OUTPUT=stderr
```

## Project Structure

```
github.com/rama-kairi/go-term/
├── main.go                      # MCP server entry point
├── internal/
│   ├── config/                  # Configuration management
│   ├── logger/                  # Structured logging
│   ├── history/                 # Command history tracking
│   ├── utils/                   # Project ID utilities
│   ├── terminal/                # Session management
│   └── tools/                   # MCP tool implementations
└── README.md
```

## License

MIT License
