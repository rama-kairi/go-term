# Go Terminal MCP Server. (go-term)

A robust, feature-rich Model Context Protocol (MCP) server for comprehensive terminal session management with advanced project tracking, command history, and security features.

## 🚀 Features

- **🎯 Project-based Session Management**: Auto-generated project IDs based on working directory with intelligent naming
- **📚 Command History Tracking**: Persistent command history with comprehensive metadata, timing, and success tracking
- **🔍 Advanced Search**: Powerful search across command history with filtering by project, session, time, output, and more
- **🔒 Security Validation**: Comprehensive input validation, command blocking, and security monitoring
- **📊 Real-time Streaming**: Live command output streaming with configurable buffer sizes
- **🗂️ Structured Logging**: JSON-formatted logging with configurable levels and outputs
- **⚙️ Flexible Configuration**: Environment variable configuration with automatic config file generation
- **🏠 User-specific Paths**: Automatic user directory detection for cross-platform compatibility
- **💾 SQLite Database**: Persistent storage with WAL mode and automatic cleanup
- **🧹 Session Management**: Automatic cleanup, timeout handling, and graceful shutdown

## 📦 Installation

### Method 1: Go Install (Recommended)

```bash
go install github.com/rama-kairi/go-term@latest
```

### Method 2: Build from Source

```bash
git clone https://github.com/rama-kairi/go-term.git
cd go-term
go build -o go-term .
go install .
```

### Requirements

- **Go 1.19 or later**
- **MCP-compatible client** (VS Code with MCP extension, Claude Desktop, etc.)

## 🔧 MCP Client Configuration

### VS Code Configuration

Add to your VS Code `mcp.json` file:

```jsonc
{
  "servers": {
    "go-terminal": {
      "command": "go-term"
    }
  }
}
```

### Claude Desktop Configuration

Add to your Claude Desktop `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "go-terminal": {
      "command": "go-term"
    }
  }
}
```

### Custom Binary Path Configuration

If you have the binary in a specific location:

```jsonc
{
  "servers": {
    "go-terminal": {
      "command": "/path/to/your/go-term"
    }
  }
}
```

## 🛠️ Available MCP Tools

### 1. `create_terminal_session`
Create a new terminal session with project association and comprehensive tracking.

**Parameters:**
- `name` (required): Descriptive name for the terminal session
- `project_id` (optional): Project ID to associate (auto-generated if not provided)
- `working_dir` (optional): Working directory (defaults to current directory)

**Example:**
```json
{
  "name": "web-development",
  "project_id": "my_website_a7b3c9",
  "working_dir": "/Users/username/projects/website"
}
```

### 2. `list_terminal_sessions`
List all existing terminal sessions with comprehensive information.

**Returns:**
- Session details (ID, name, project, working directory)
- Command statistics (total commands, success rate, last activity)
- Project information and health status

### 3. `run_command`
Execute a command in a specific terminal session with full tracking.

**Parameters:**
- `session_id` (required): UUID4 identifier of the terminal session
- `command` (required): Command to execute (validated for security)

**Features:**
- Directory changes persist across commands
- Comprehensive output capture
- Execution time tracking
- Security validation
- Command history logging

### 4. `search_terminal_history`
Search through command history across all sessions and projects.

**Parameters:**
- `session_id` (optional): Filter by specific session
- `project_id` (optional): Filter by specific project
- `command` (optional): Search command text (case-insensitive)
- `output` (optional): Search output text (case-insensitive)
- `success` (optional): Filter by success status (true/false)
- `start_time` (optional): Filter by start time (ISO 8601 format)
- `end_time` (optional): Filter by end time (ISO 8601 format)
- `working_dir` (optional): Filter by working directory
- `tags` (optional): Filter by tags array
- `limit` (optional): Maximum results (default: 100, max: 1000)
- `sort_by` (optional): Sort by 'time', 'duration', or 'command'
- `sort_desc` (optional): Sort in descending order (default: true)
- `include_output` (optional): Include command output (default: false)

### 5. `delete_session`
Delete terminal sessions with confirmation requirement.

**Parameters:**
- `session_id` (optional): Delete specific session by ID
- `project_id` (optional): Delete all sessions for a project
- `confirm` (required): Must be `true` to proceed with deletion

## ⚙️ Configuration

The server automatically creates a configuration file at `~/.config/go-term/config.json` with sensible defaults. You can customize behavior using environment variables or by editing the config file directly.

### Environment Variables

#### Server Configuration
```bash
export TERMINAL_MCP_DEBUG=true|false              # Enable debug mode
```

#### Session Configuration
```bash
export TERMINAL_MCP_MAX_SESSIONS=50               # Maximum concurrent sessions
export TERMINAL_MCP_SESSION_TIMEOUT=60m          # Default session timeout
export TERMINAL_MCP_CLEANUP_INTERVAL=5m          # Cleanup interval
export TERMINAL_MCP_MAX_COMMAND_LENGTH=50000     # Maximum command length
export TERMINAL_MCP_MAX_OUTPUT_SIZE=10485760     # Maximum output size (10MB)
export TERMINAL_MCP_WORKING_DIR=/custom/path     # Default working directory
export TERMINAL_MCP_SHELL=/bin/bash              # Default shell
export TERMINAL_MCP_ENABLE_STREAMING=true        # Enable real-time streaming
```

#### Database Configuration
```bash
export TERMINAL_MCP_DATA_DIR=/custom/data/dir    # Custom data directory
export TERMINAL_MCP_MAX_CONNECTIONS=10           # Database max connections
export TERMINAL_MCP_CONNECTION_TIMEOUT=5s        # Database connection timeout
export TERMINAL_MCP_ENABLE_WAL=true              # Enable SQLite WAL mode
```

#### Security Configuration
```bash
export TERMINAL_MCP_ENABLE_SANDBOX=false         # Enable command sandboxing
export TERMINAL_MCP_BLOCKED_COMMANDS="rm -rf /,format"  # Comma-separated blocked commands
export TERMINAL_MCP_ALLOW_NETWORK=true           # Allow network access
export TERMINAL_MCP_ALLOW_FILESYSTEM_WRITE=true  # Allow filesystem writes
export TERMINAL_MCP_MAX_PROCESSES=20             # Maximum concurrent processes
export TERMINAL_MCP_MAX_MEMORY_MB=2048           # Maximum memory usage (MB)
export TERMINAL_MCP_MAX_CPU_PERCENT=80           # Maximum CPU usage (%)
```

#### Logging Configuration
```bash
export TERMINAL_MCP_LOG_LEVEL=info               # Log level: debug, info, warn, error
export TERMINAL_MCP_LOG_FORMAT=json              # Log format: json, text
export TERMINAL_MCP_LOG_OUTPUT=stderr            # Log output: stderr, file, or file path
```

#### Monitoring Configuration
```bash
export TERMINAL_MCP_ENABLE_METRICS=false         # Enable metrics endpoint
export TERMINAL_MCP_METRICS_PORT=9090            # Metrics port
export TERMINAL_MCP_HEALTH_PORT=8080             # Health check port
```

### Configuration File Location

The configuration file is automatically created at:
- **macOS/Linux**: `~/.config/go-term/config.json`
- **Windows**: `%USERPROFILE%\.config\go-term\config.json`

### Custom Configuration File

You can specify a custom configuration file:

```bash
go-term -config /path/to/custom/config.json
```

## 🗄️ Data Storage

### Database Location
- **Database file**: `~/.config/go-term/sessions.db`
- **Data directory**: `~/.config/go-term/`

### What's Stored
- Terminal session metadata
- Command history with timestamps
- Project associations
- Execution results and timing
- Working directory changes

### Database Features
- **SQLite with WAL mode** for better performance
- **Automatic cleanup** of old sessions and commands
- **Vacuum operations** for database optimization
- **Connection pooling** for concurrent access

## 🔒 Security Features

### Command Validation
- **Blocked commands**: Dangerous commands are automatically blocked
- **Command length limits**: Prevents excessively long commands
- **Output size limits**: Prevents memory exhaustion

### Default Blocked Commands
```bash
rm -rf /
format
mkfs
dd if=/dev/zero
:(){ :|:& };:
```

### Resource Limits
- **Maximum processes**: 20 concurrent processes
- **Memory limit**: 2048 MB maximum usage
- **CPU limit**: 80% maximum CPU usage
- **Network access**: Configurable network restrictions

### Sandbox Mode
When enabled, provides additional isolation:
- Restricted file system access
- Limited network capabilities
- Process monitoring and limits

## 🚀 Usage Examples

### Basic Session Management

```bash
# The server runs automatically when called by MCP client
# No manual startup required - it communicates via stdio
```

### Creating a Session via MCP Client

```json
{
  "tool": "create_terminal_session",
  "arguments": {
    "name": "web-dev-session",
    "working_dir": "/Users/username/projects/website"
  }
}
```

### Running Commands

```json
{
  "tool": "run_command",
  "arguments": {
    "session_id": "550e8400-e29b-41d4-a716-446655440000",
    "command": "npm install"
  }
}
```

### Searching Command History

```json
{
  "tool": "search_terminal_history",
  "arguments": {
    "command": "npm",
    "success": true,
    "limit": 50,
    "include_output": true
  }
}
```

## 📁 Project Structure

```
github.com/rama-kairi/go-term/
├── main.go                      # MCP server entry point
├── go.mod                       # Go module definition
├── go.sum                       # Go module checksums
├── config-schema.json           # JSON schema for configuration
├── README.md                    # This file
└── internal/
    ├── config/
    │   └── config.go           # Configuration management
    ├── database/
    │   └── database.go         # SQLite database operations
    ├── history/
    │   └── history.go          # Command history tracking
    ├── logger/
    │   └── logger.go           # Structured logging
    ├── streaming/
    │   └── streaming.go        # Real-time command streaming
    ├── terminal/
    │   └── session.go          # Terminal session management
    ├── tools/
    │   └── terminal_tools.go   # MCP tool implementations
    └── utils/
        └── project.go          # Project ID utilities
```

## 🐛 Debugging

### Enable Debug Mode

```bash
export TERMINAL_MCP_DEBUG=true
export TERMINAL_MCP_LOG_LEVEL=debug
go-term
```

### Check Configuration

The server logs its configuration on startup. Check the logs to verify settings:

```bash
go-term 2>&1 | jq '.config_directory'
```

### Common Issues

1. **Permission Denied**: Ensure the binary is executable
   ```bash
   chmod +x $(which go-term)
   ```

2. **Config Directory Issues**: Manually create the config directory
   ```bash
   mkdir -p ~/.config/go-term
   ```

3. **Database Lock Issues**: Check for zombie processes
   ```bash
   lsof ~/.config/go-term/sessions.db
   ```

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📋 Requirements

- **Go 1.25+**: For building and running
- **SQLite3**: Embedded database (automatically included)
- **MCP Client**: VS Code with MCP extension, Claude Desktop, etc.

## 📄 License

MIT License - see the [LICENSE](LICENSE) file for details.

## 🔗 Links

- **Repository**: [github.com/rama-kairi/go-term](https://github.com/rama-kairi/go-term)
- **Model Context Protocol**: [MCP Documentation](https://modelcontextprotocol.io/)
- **Issues**: [GitHub Issues](https://github.com/rama-kairi/go-term/issues)

---

**Note**: This MCP server uses dynamic user path generation, making it fully portable across different users and systems without any hardcoded paths.
