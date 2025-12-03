package terminal

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/rama-kairi/go-term/internal/config"
	"github.com/rama-kairi/go-term/internal/database"
	"github.com/rama-kairi/go-term/internal/logger"
	"github.com/rama-kairi/go-term/internal/monitoring"
	"github.com/rama-kairi/go-term/internal/utils"
)

// H4: shellEscape escapes a string for safe use in shell commands
// This prevents shell injection attacks by escaping special characters
func shellEscape(s string) string {
	// If string is empty, return empty quotes
	if s == "" {
		return "''"
	}

	// Check if string needs escaping
	needsEscape := false
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '-' ||
			c == '.' || c == '/' || c == ':') {
			needsEscape = true
			break
		}
	}

	if !needsEscape {
		return s
	}

	// Use single quotes for escaping, escape any existing single quotes
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// BackgroundProcess represents a running background process
type BackgroundProcess struct {
	ID           string    `json:"id"`
	Command      string    `json:"command"`
	PID          int       `json:"pid"`
	StartTime    time.Time `json:"start_time"`
	IsRunning    bool      `json:"is_running"`
	ExitCode     int       `json:"exit_code,omitempty"`
	Output       string    `json:"output"`
	ErrorOutput  string    `json:"error_output"`
	cmd          *exec.Cmd
	outputBuffer strings.Builder
	errorBuffer  strings.Builder
	Mutex        sync.RWMutex `json:"-"` // Exported for access
}

// TruncateOutput limits the output to the specified maximum length, keeping the latest content
func (bp *BackgroundProcess) TruncateOutput(maxLength int) {
	bp.Mutex.Lock()
	defer bp.Mutex.Unlock()

	if len(bp.Output) > maxLength {
		// Keep the latest content
		bp.Output = "..." + bp.Output[len(bp.Output)-maxLength+3:]
	}

	if len(bp.ErrorOutput) > maxLength {
		// Keep the latest content
		bp.ErrorOutput = "..." + bp.ErrorOutput[len(bp.ErrorOutput)-maxLength+3:]
	}
}

// UpdateOutput safely updates the output and applies length limits
func (bp *BackgroundProcess) UpdateOutput(newOutput string, maxLength int) {
	bp.Mutex.Lock()
	defer bp.Mutex.Unlock()

	bp.outputBuffer.WriteString(newOutput)
	bp.Output = bp.outputBuffer.String()

	// Apply length limit if specified
	if maxLength > 0 && len(bp.Output) > maxLength {
		bp.Output = "..." + bp.Output[len(bp.Output)-maxLength+3:]
		// Reset buffer with truncated content
		bp.outputBuffer.Reset()
		bp.outputBuffer.WriteString(bp.Output)
	}
}

// UpdateErrorOutput safely updates the error output and applies length limits
func (bp *BackgroundProcess) UpdateErrorOutput(newOutput string, maxLength int) {
	bp.Mutex.Lock()
	defer bp.Mutex.Unlock()

	bp.errorBuffer.WriteString(newOutput)
	bp.ErrorOutput = bp.errorBuffer.String()

	// Apply length limit if specified
	if maxLength > 0 && len(bp.ErrorOutput) > maxLength {
		bp.ErrorOutput = "..." + bp.ErrorOutput[len(bp.ErrorOutput)-maxLength+3:]
		// Reset buffer with truncated content
		bp.errorBuffer.Reset()
		bp.errorBuffer.WriteString(bp.ErrorOutput)
	}
}

// Session represents a terminal session with project association and command history
type Session struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	ProjectID     string            `json:"project_id"`
	WorkingDir    string            `json:"working_dir"`
	Environment   map[string]string `json:"environment"`
	CreatedAt     time.Time         `json:"created_at"`
	LastUsedAt    time.Time         `json:"last_used_at"`
	IsActive      bool              `json:"is_active"`
	CommandCount  int               `json:"command_count"`
	SuccessCount  int               `json:"success_count"`
	TotalDuration time.Duration     `json:"total_duration"`

	// Background process tracking
	BackgroundProcesses map[string]*BackgroundProcess `json:"background_processes,omitempty"`

	// M9: Activity tracking
	activityTracker *SessionActivityTracker `json:"-"`

	// Internal fields for session management
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
	mutex  sync.RWMutex

	// Context for cancellation support
	ctx    context.Context
	cancel context.CancelFunc

	// Persistent shell state tracking
	currentDir string
	shellPid   int
	shellEnv   map[string]string
}

// GetCurrentDir returns the current working directory of the session
func (s *Session) GetCurrentDir() string {
	return s.currentDir
}

// SetEnvironment sets or updates an environment variable for this session
func (s *Session) SetEnvironment(key, value string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.Environment == nil {
		s.Environment = make(map[string]string)
	}
	if s.shellEnv == nil {
		s.shellEnv = make(map[string]string)
	}

	s.Environment[key] = value
	s.shellEnv[key] = value
}

// SetEnvironmentBatch sets multiple environment variables at once
func (s *Session) SetEnvironmentBatch(envVars map[string]string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.Environment == nil {
		s.Environment = make(map[string]string)
	}
	if s.shellEnv == nil {
		s.shellEnv = make(map[string]string)
	}

	for key, value := range envVars {
		s.Environment[key] = value
		s.shellEnv[key] = value
	}
}

// GetEnvironment returns the value of an environment variable
func (s *Session) GetEnvironment(key string) (string, bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if s.Environment == nil {
		return "", false
	}
	value, exists := s.Environment[key]
	return value, exists
}

// GetAllEnvironment returns a copy of all environment variables
func (s *Session) GetAllEnvironment() map[string]string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	env := make(map[string]string, len(s.Environment))
	for k, v := range s.Environment {
		env[k] = v
	}
	return env
}

// UnsetEnvironment removes an environment variable from the session
func (s *Session) UnsetEnvironment(key string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	delete(s.Environment, key)
	delete(s.shellEnv, key)
}

// ClearEnvironment removes all session-specific environment variables
// and resets to system defaults
func (s *Session) ClearEnvironment() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.Environment = make(map[string]string)
	s.shellEnv = make(map[string]string)

	// Restore system environment
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			s.Environment[parts[0]] = parts[1]
			s.shellEnv[parts[0]] = parts[1]
		}
	}
}

// Manager manages terminal sessions with project organization and command history
type Manager struct {
	sessions            map[string]*Session
	config              *config.Config
	logger              *logger.Logger
	database            *database.DB
	projectIDGen        *utils.ProjectIDGenerator
	mutex               sync.RWMutex
	cleanupTicker       *time.Ticker
	resourceTicker      *time.Ticker
	stopCleanup         chan bool
	stopResourceCleanup chan bool
	resourceMonitor     *monitoring.ResourceMonitor

	// Context for manager-wide cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

// NewManager creates a new terminal session manager with enhanced features
func NewManager(cfg *config.Config, logger *logger.Logger, db *database.DB) *Manager {
	projectIDGen := utils.NewProjectIDGenerator()

	// Create manager context for cancellation support
	ctx, cancel := context.WithCancel(context.Background())

	manager := &Manager{
		sessions:            make(map[string]*Session),
		config:              cfg,
		logger:              logger,
		database:            db,
		projectIDGen:        projectIDGen,
		stopCleanup:         make(chan bool),
		stopResourceCleanup: make(chan bool),
		ctx:                 ctx,
		cancel:              cancel,
	}

	// Initialize resource monitor
	manager.resourceMonitor = monitoring.NewResourceMonitor(logger, 30*time.Second)
	manager.resourceMonitor.SetCounters(
		func() int { return len(manager.sessions) },
		func() int { return manager.getTotalBackgroundProcesses() },
	)

	// Start cleanup routines
	manager.startCleanupRoutine()
	manager.startResourceCleanupRoutine()

	// Start resource monitoring
	manager.resourceMonitor.Start(manager.ctx)

	return manager
}

// determineWorkingDirectory implements hierarchical working directory detection
// Priority: 1) VS Code environment, 2) Directory tree walking, 3) Server CWD, 4) User home
func (m *Manager) determineWorkingDirectory() (string, error) {
	// Method 1: VS Code environment variables (most reliable)
	if envWorkspace, err := m.detectFromEnvironment(); err == nil {
		m.logger.Info("Using environment workspace detection", map[string]interface{}{
			"workspace_root": envWorkspace,
			"method":         "environment_variables",
		})
		return envWorkspace, nil
	}

	// Method 2: Directory tree walking from MCP server location
	if currentDir, err := os.Getwd(); err == nil {
		if workspaceRoot := m.findWorkspaceRoot(currentDir); workspaceRoot != "" {
			m.logger.Info("Using directory tree workspace detection", map[string]interface{}{
				"workspace_root": workspaceRoot,
				"method":         "directory_walking",
			})
			return workspaceRoot, nil
		}
	}

	// Method 3: MCP server's current directory
	if currentDir, err := os.Getwd(); err == nil {
		m.logger.Info("Using MCP server current directory", map[string]interface{}{
			"working_dir": currentDir,
			"method":      "server_cwd",
		})
		return currentDir, nil
	}

	// Method 4: User home directory (final fallback)
	if homeDir, err := os.UserHomeDir(); err == nil {
		m.logger.Info("Using user home directory fallback", map[string]interface{}{
			"working_dir": homeDir,
			"method":      "home_fallback",
		})
		return homeDir, nil
	}

	return "", fmt.Errorf("unable to determine working directory from any method")
}

// detectFromEnvironment detects workspace from VS Code environment variables
func (m *Manager) detectFromEnvironment() (string, error) {
	// Method 1: VSCODE_CWD (most reliable according to community research)
	if vscodeCwd := os.Getenv("VSCODE_CWD"); vscodeCwd != "" {
		if info, err := os.Stat(vscodeCwd); err == nil && info.IsDir() {
			m.logger.Debug("Found VSCODE_CWD environment variable", map[string]interface{}{
				"path": vscodeCwd,
			})
			return vscodeCwd, nil
		}
	}

	// Method 2: WORKSPACE_FOLDER_PATHS (less reliable but worth trying)
	if workspacePaths := os.Getenv("WORKSPACE_FOLDER_PATHS"); workspacePaths != "" {
		// May contain multiple paths separated by delimiter
		paths := strings.Split(workspacePaths, string(os.PathListSeparator))
		if len(paths) > 0 && paths[0] != "" {
			if info, err := os.Stat(paths[0]); err == nil && info.IsDir() {
				m.logger.Debug("Found WORKSPACE_FOLDER_PATHS environment variable", map[string]interface{}{
					"path": paths[0],
				})
				return paths[0], nil
			}
		}
	}

	// Method 3: Check for VS Code specific environment variables
	if vscodeWorkspace := os.Getenv("VSCODE_WORKSPACE"); vscodeWorkspace != "" {
		workspaceDir := filepath.Dir(vscodeWorkspace)
		if info, err := os.Stat(workspaceDir); err == nil && info.IsDir() {
			m.logger.Debug("Found VSCODE_WORKSPACE environment variable", map[string]interface{}{
				"path": workspaceDir,
			})
			return workspaceDir, nil
		}
	}

	return "", fmt.Errorf("no workspace environment variables found")
}

// findWorkspaceRoot walks up the directory tree looking for workspace indicators
func (m *Manager) findWorkspaceRoot(startDir string) string {
	currentDir := startDir
	maxDepth := 10 // Prevent infinite loop

	for i := 0; i < maxDepth; i++ {
		// Check for workspace indicators in order of priority
		workspaceIndicators := []string{
			".vscode",            // VS Code workspace
			".git",               // Git repository
			"package.json",       // Node.js project
			"go.mod",             // Go project
			"requirements.txt",   // Python project
			"Cargo.toml",         // Rust project
			"pom.xml",            // Maven project
			"build.gradle",       // Gradle project
			"composer.json",      // PHP project
			"Gemfile",            // Ruby project
			"tsconfig.json",      // TypeScript project
			".project",           // Eclipse project
			"pyproject.toml",     // Modern Python project
			"Dockerfile",         // Docker project
			"docker-compose.yml", // Docker Compose
		}

		for _, indicator := range workspaceIndicators {
			indicatorPath := filepath.Join(currentDir, indicator)
			if _, err := os.Stat(indicatorPath); err == nil {
				m.logger.Debug("Found workspace indicator", map[string]interface{}{
					"indicator": indicator,
					"path":      currentDir,
				})
				return currentDir
			}
		}

		// Move up one directory
		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			// Reached filesystem root
			break
		}
		currentDir = parentDir
	}

	return ""
}

// CreateSession creates a new terminal session with project association
func (m *Manager) CreateSession(name string, projectID string, workingDir string) (*Session, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Check session limit before creating new session
	if len(m.sessions) >= m.config.Session.MaxSessions {
		// Attempt to cleanup excess sessions
		m.cleanupExcessSessions()

		// Check again after cleanup
		if len(m.sessions) >= m.config.Session.MaxSessions {
			return nil, fmt.Errorf("maximum number of sessions (%d) reached, cannot create new session", m.config.Session.MaxSessions)
		}
	}

	// Ensure database connection is available (auto-recovery)
	if m.database != nil {
		if err := m.database.HealthCheck(); err != nil {
			m.logger.Warn("Database health check failed, will continue without database persistence", map[string]interface{}{
				"error": err.Error(),
			})
		}
	}

	sessionID := uuid.New().String()

	// Generate project ID if not provided
	if projectID == "" {
		var err error
		if workingDir != "" {
			projectID = m.projectIDGen.GenerateProjectIDFromPath(workingDir)
		} else {
			projectID, err = m.projectIDGen.GenerateProjectID()
			if err != nil {
				m.logger.Error("Failed to generate project ID", err)
				projectID = "default_project_" + sessionID[:8]
			}
		}
	}

	// Validate project ID
	if err := m.projectIDGen.ValidateProjectID(projectID); err != nil {
		return nil, fmt.Errorf("invalid project ID: %w", err)
	}

	// Set working directory using enhanced detection
	if workingDir == "" {
		var err error
		workingDir, err = m.determineWorkingDirectory()
		if err != nil {
			m.logger.Error("Failed to determine working directory", err)
			// Final fallback to home directory
			if homeDir, homeErr := os.UserHomeDir(); homeErr == nil {
				workingDir = homeDir
			} else {
				return nil, fmt.Errorf("unable to determine working directory: %w", err)
			}
		}
	}

	// Ensure working directory exists
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create working directory: %w", err)
	}

	// Create session context for cancellation support
	sessionCtx, sessionCancel := context.WithCancel(m.ctx)

	session := &Session{
		ID:                  sessionID,
		Name:                name,
		ProjectID:           projectID,
		WorkingDir:          workingDir,
		Environment:         make(map[string]string),
		CreatedAt:           time.Now(),
		LastUsedAt:          time.Now(),
		IsActive:            true,
		CommandCount:        0,
		SuccessCount:        0,
		TotalDuration:       0,
		BackgroundProcesses: make(map[string]*BackgroundProcess),
		activityTracker:     NewSessionActivityTracker(), // M9: Initialize activity tracker
		currentDir:          workingDir,
		shellEnv:            make(map[string]string),
		ctx:                 sessionCtx,
		cancel:              sessionCancel,
	}

	// Copy environment variables
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			session.Environment[parts[0]] = parts[1]
			session.shellEnv[parts[0]] = parts[1]
		}
	}

	// Initialize the persistent shell
	shell := m.config.Session.Shell
	if shell == "" {
		shell = os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/bash"
		}
	}

	// Create shell command with proper working directory
	cmd := exec.Command(shell)
	cmd.Dir = workingDir
	cmd.Env = os.Environ()

	// Set up pipes for persistent shell interaction
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		stdout.Close()
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	session.cmd = cmd
	session.stdin = stdin
	session.stdout = stdout
	session.stderr = stderr

	// Start the shell
	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		stderr.Close()
		return nil, fmt.Errorf("failed to start shell: %w", err)
	}

	session.shellPid = cmd.Process.Pid

	// Session initialized successfully
	m.logger.Info("Session created successfully", map[string]interface{}{
		"session_id": sessionID,
		"project_id": projectID,
		"name":       name,
	})

	m.sessions[sessionID] = session

	// Persist session to database if available
	if m.database != nil {
		envJSON, _ := json.Marshal(session.Environment)
		sessionRecord := &database.SessionRecord{
			ID:           sessionID,
			Name:         name,
			ProjectID:    projectID,
			WorkingDir:   workingDir,
			Environment:  string(envJSON),
			CreatedAt:    session.CreatedAt,
			LastUsedAt:   session.LastUsedAt,
			IsActive:     session.IsActive,
			CommandCount: session.CommandCount,
		}
		err := m.database.CreateSession(sessionRecord)
		if err != nil {
			m.logger.Warn("Failed to persist session to database", map[string]interface{}{
				"session_id": sessionID,
				"error":      err.Error(),
			})
		} else {
			m.logger.Info("Session persisted to database", map[string]interface{}{
				"session_id": sessionID,
			})
		}
	}

	m.logger.LogSessionEvent("created", sessionID, name, map[string]interface{}{
		"project_id":  projectID,
		"working_dir": workingDir,
		"shell":       shell,
	})

	return session, nil
}

// GetSession retrieves a session by ID
func (m *Manager) GetSession(sessionID string) (*Session, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session with ID %s not found", sessionID)
	}

	return session, nil
}

// SetSessionEnvironment sets or updates environment variable(s) for a session
func (m *Manager) SetSessionEnvironment(sessionID string, envVars map[string]string) error {
	m.mutex.RLock()
	session, exists := m.sessions[sessionID]
	m.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("session with ID %s not found", sessionID)
	}

	session.SetEnvironmentBatch(envVars)

	m.logger.Info("Updated session environment variables", map[string]interface{}{
		"session_id": sessionID,
		"variables":  len(envVars),
	})

	return nil
}

// GetSessionEnvironment returns all environment variables for a session
func (m *Manager) GetSessionEnvironment(sessionID string) (map[string]string, error) {
	m.mutex.RLock()
	session, exists := m.sessions[sessionID]
	m.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("session with ID %s not found", sessionID)
	}

	return session.GetAllEnvironment(), nil
}

// UnsetSessionEnvironment removes environment variable(s) from a session
func (m *Manager) UnsetSessionEnvironment(sessionID string, keys []string) error {
	m.mutex.RLock()
	session, exists := m.sessions[sessionID]
	m.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("session with ID %s not found", sessionID)
	}

	for _, key := range keys {
		session.UnsetEnvironment(key)
	}

	m.logger.Info("Removed session environment variables", map[string]interface{}{
		"session_id": sessionID,
		"variables":  len(keys),
	})

	return nil
}

// ListSessions returns all sessions with dynamically calculated statistics
func (m *Manager) ListSessions() []*Session {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// If database is available, use it for accurate statistics
	if m.database != nil {
		dbSessions, err := m.database.GetSessionsWithStats()
		if err == nil {
			sessions := make([]*Session, 0, len(dbSessions))
			for _, dbSession := range dbSessions {
				// Get in-memory session for current state if exists
				inMemorySession := m.sessions[dbSession.ID]

				session := &Session{
					ID:                  dbSession.ID,
					Name:                dbSession.Name,
					ProjectID:           dbSession.ProjectID,
					WorkingDir:          dbSession.WorkingDir,
					CreatedAt:           dbSession.CreatedAt,
					LastUsedAt:          dbSession.LastUsedAt,
					IsActive:            dbSession.IsActive,
					CommandCount:        dbSession.CommandCount,  // From database (accurate)
					SuccessCount:        dbSession.SuccessCount,  // From database (accurate)
					TotalDuration:       dbSession.TotalDuration, // From database (accurate)
					BackgroundProcesses: make(map[string]*BackgroundProcess),
				}

				// Use current working directory from in-memory session if available
				if inMemorySession != nil {
					session.currentDir = inMemorySession.currentDir
				} else {
					session.currentDir = dbSession.WorkingDir
				}

				sessions = append(sessions, session)
			}
			return sessions
		}
		// Fall back to in-memory if database query fails
	}

	// Fallback to in-memory sessions (original behavior)
	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		// Create a copy to avoid data races
		sessionCopy := &Session{
			ID:            session.ID,
			Name:          session.Name,
			ProjectID:     session.ProjectID,
			WorkingDir:    session.WorkingDir,
			CreatedAt:     session.CreatedAt,
			LastUsedAt:    session.LastUsedAt,
			IsActive:      session.IsActive,
			CommandCount:  session.CommandCount,
			SuccessCount:  session.SuccessCount,
			TotalDuration: session.TotalDuration,
			currentDir:    session.currentDir,
		}
		sessions = append(sessions, sessionCopy)
	}

	return sessions
}

// ExecuteCommand executes a command in the specified session with full history tracking
func (m *Manager) ExecuteCommand(sessionID, command string) (string, error) {
	session, err := m.GetSession(sessionID)
	if err != nil {
		return "", err
	}

	session.mutex.Lock()
	defer session.mutex.Unlock()

	if !session.IsActive {
		return "", fmt.Errorf("session %s is not active", sessionID)
	}

	startTime := time.Now()
	session.LastUsedAt = startTime

	m.logger.Debug("Executing command", map[string]interface{}{
		"session_id":  sessionID,
		"command":     command,
		"working_dir": session.currentDir,
	})

	// Execute the command with timeout
	ctx, cancel := context.WithTimeout(context.Background(), m.config.Session.DefaultTimeout)
	defer cancel()

	output, exitCode, err := m.executeCommandInSession(ctx, session, command)

	endTime := time.Now()
	duration := endTime.Sub(startTime)
	success := err == nil && exitCode == 0

	// Update session last used time
	session.LastUsedAt = endTime

	// Log command execution
	m.logger.LogCommand(sessionID, command, duration, success, output, err)

	// Store command in database if available
	if m.database != nil {
		// Check database health before using it
		if dbHealthErr := m.database.HealthCheck(); dbHealthErr == nil {
			dbErr := m.database.StoreCommand(
				sessionID,
				session.ProjectID,
				command,
				output,
				exitCode,
				success,
				startTime,
				endTime,
				duration,
				session.currentDir,
			)

			if dbErr != nil {
				m.logger.Error("Failed to store command in database", dbErr, map[string]interface{}{
					"session_id": sessionID,
					"command":    command,
				})
			}
		} else {
			m.logger.Debug("Database not available for storing command", map[string]interface{}{
				"session_id": sessionID,
				"error":      dbHealthErr.Error(),
			})
		}
	}

	// Update session working directory if command changed it
	if success && m.isDirectoryChangeCommand(command) {
		if newDir := m.extractDirectoryFromCommand(command); newDir != "" {
			session.currentDir = m.resolveDirectoryPath(session.currentDir, newDir)
		}
	}

	// Return output and error
	if err != nil {
		return output, fmt.Errorf("command execution failed: %w", err)
	}

	return output, nil
}

// ExecuteCommandWithStreaming executes a command with streaming output (enhanced version of ExecuteCommand)
func (m *Manager) ExecuteCommandWithStreaming(sessionID, command string) (string, error) {
	m.mutex.RLock()
	session, exists := m.sessions[sessionID]
	m.mutex.RUnlock()

	if !exists {
		return "", fmt.Errorf("session %s not found", sessionID)
	}

	session.mutex.Lock()
	defer session.mutex.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), m.config.Session.DefaultTimeout)
	defer cancel()

	// Record start time for accurate duration tracking
	startTime := time.Now()

	// Add a small delay to simulate streaming behavior while maintaining session state
	// This is a transitional implementation that maintains session continuity
	// while providing the streaming experience

	// Use the existing session-aware execution but with simulated streaming timing
	output, exitCode, err := m.executeCommandInSessionWithStreaming(ctx, session, command)

	// Record end time for accurate duration tracking
	endTime := time.Now()
	duration := endTime.Sub(startTime)

	// Update session last used time
	session.LastUsedAt = endTime

	// Update working directory if this was a directory change command
	if m.isDirectoryChangeCommand(command) {
		targetDir := m.extractDirectoryFromCommand(command)
		if targetDir != "" {
			resolved := m.resolveDirectoryPath(session.currentDir, targetDir)
			if info, err := os.Stat(resolved); err == nil && info.IsDir() {
				session.currentDir = resolved
			}
		}
	}

	// Store command in database if available
	if m.database != nil {
		// Check database health before using it
		if dbHealthErr := m.database.HealthCheck(); dbHealthErr == nil {
			dbErr := m.database.StoreCommand(
				sessionID,
				session.ProjectID,
				command,
				output,
				exitCode,
				err == nil,
				startTime,
				endTime,
				duration,
				session.currentDir,
			)

			if dbErr != nil {
				m.logger.Error("Failed to store streaming command in database", dbErr, map[string]interface{}{
					"session_id": sessionID,
					"command":    command,
				})
			}
		} else {
			m.logger.Debug("Database not available for storing streaming command", map[string]interface{}{
				"session_id": sessionID,
				"error":      dbHealthErr.Error(),
			})
		}
	}

	m.logger.Info("Streaming command executed", map[string]interface{}{
		"session_id":    sessionID,
		"command":       command,
		"working_dir":   session.currentDir,
		"command_count": session.CommandCount,
		"success":       err == nil,
		"streaming":     true,
	})

	if err != nil {
		return output, fmt.Errorf("command execution failed: %w", err)
	}

	return output, nil
}

// executeCommandInSessionWithStreaming executes a command with enhanced streaming support
func (m *Manager) executeCommandInSessionWithStreaming(ctx context.Context, session *Session, command string) (string, int, error) {
	// For true session persistence with streaming simulation
	shell := m.config.Session.Shell
	if shell == "" {
		// Always use bash for consistent behavior, especially for loop commands
		shell = "/bin/bash"
	}

	// H4: Escape the current directory to prevent shell injection
	escapedDir := shellEscape(session.currentDir)
	fullCommand := fmt.Sprintf("cd %s && %s", escapedDir, command)

	cmd := exec.CommandContext(ctx, shell, "-c", fullCommand)
	cmd.Dir = session.WorkingDir

	// Set environment from session
	env := make([]string, 0, len(session.shellEnv))
	for k, v := range session.shellEnv {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env

	// Execute command - this will take the actual time the command needs
	// For sleep or loop commands, this will naturally take the expected time
	output, err := cmd.CombinedOutput()
	exitCode := 0

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = 1
		}
	}

	return string(output), exitCode, err
}

// executeCommandInSession executes a command in the session's persistent shell
func (m *Manager) executeCommandInSession(ctx context.Context, session *Session, command string) (string, int, error) {
	// For true session persistence, we need to use the persistent shell
	// For now, we'll use a simpler approach that maintains working directory

	shell := m.config.Session.Shell
	if shell == "" {
		// Always use bash for consistent behavior
		shell = "/bin/bash"
	}

	// H4: Escape the current directory to prevent shell injection
	escapedDir := shellEscape(session.currentDir)
	fullCommand := fmt.Sprintf("cd %s && %s", escapedDir, command)

	cmd := exec.CommandContext(ctx, shell, "-c", fullCommand)
	cmd.Dir = session.WorkingDir

	// Set environment from session
	env := make([]string, 0, len(session.shellEnv))
	for k, v := range session.shellEnv {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env

	// CRITICAL FIX: Set up proper process group handling for timeout support
	// This ensures that when the context is cancelled, all child processes are terminated
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // Create a new process group
	}

	// Capture output using pipes
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", 1, fmt.Errorf("failed to create stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", 1, fmt.Errorf("failed to create stderr pipe: %v", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return "", 1, fmt.Errorf("failed to start command: %v", err)
	}

	// Read output in goroutines
	var outputBuilder strings.Builder
	outputDone := make(chan bool, 2)

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			outputBuilder.WriteString(scanner.Text() + "\n")
		}
		outputDone <- true
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			outputBuilder.WriteString(scanner.Text() + "\n")
		}
		outputDone <- true
	}()

	// Set up a goroutine to handle command completion
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	// Wait for either completion or context cancellation
	select {
	case <-ctx.Done():
		// Context was cancelled (timeout or manual cancellation)
		// Kill the entire process group to ensure all child processes are terminated
		if cmd.Process != nil {
			pgid := cmd.Process.Pid
			// Send SIGTERM to the entire process group
			if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil {
				// If SIGTERM fails, try SIGKILL
				syscall.Kill(-pgid, syscall.SIGKILL)
			}
		}

		// Wait a short time for the process to terminate gracefully
		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
			// Force kill if still running
			if cmd.Process != nil {
				syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
		}

		// Wait for output goroutines to finish
		go func() {
			<-outputDone
			<-outputDone
		}()

		return outputBuilder.String(), 124, ctx.Err() // Exit code 124 indicates timeout
	case err := <-done:
		// Command completed normally, wait for output to be read
		<-outputDone
		<-outputDone

		exitCode := 0
		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				exitCode = exitError.ExitCode()
			} else {
				exitCode = 1
			}
		}

		return outputBuilder.String(), exitCode, err
	}
} // isDirectoryChangeCommand checks if the command is a directory change command
func (m *Manager) isDirectoryChangeCommand(command string) bool {
	trimmed := strings.TrimSpace(command)
	return strings.HasPrefix(trimmed, "cd ") || trimmed == "cd"
}

// extractDirectoryFromCommand extracts the directory path from a cd command
func (m *Manager) extractDirectoryFromCommand(command string) string {
	parts := strings.Fields(strings.TrimSpace(command))
	if len(parts) >= 2 && parts[0] == "cd" {
		return parts[1]
	}
	return ""
}

// resolveDirectoryPath resolves a directory path relative to the current directory
func (m *Manager) resolveDirectoryPath(currentDir, targetDir string) string {
	if filepath.IsAbs(targetDir) {
		return targetDir
	}

	resolved := filepath.Join(currentDir, targetDir)
	if abs, err := filepath.Abs(resolved); err == nil {
		return abs
	}

	return resolved
}

// CloseSession closes a terminal session and cleans up resources
func (m *Manager) CloseSession(sessionID string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session with ID %s not found", sessionID)
	}

	session.mutex.Lock()
	defer session.mutex.Unlock()

	session.IsActive = false

	// Cancel session context to stop all background processes and operations
	if session.cancel != nil {
		session.cancel()
	}

	// Close pipes
	if session.stdin != nil {
		session.stdin.Close()
	}
	if session.stdout != nil {
		session.stdout.Close()
	}
	if session.stderr != nil {
		session.stderr.Close()
	}

	// Kill the process
	if session.cmd != nil && session.cmd.Process != nil {
		session.cmd.Process.Kill()
		session.cmd.Wait()
	}

	// Clean up background processes
	for processID, bgProcess := range session.BackgroundProcesses {
		if bgProcess.cmd != nil && bgProcess.cmd.Process != nil && bgProcess.IsRunning {
			bgProcess.cmd.Process.Kill()
			bgProcess.cmd.Wait()
			m.logger.Info("Killed background process", map[string]interface{}{
				"session_id": sessionID,
				"process_id": processID,
				"command":    bgProcess.Command,
			})
		}
	}

	// Clean up database records
	if m.database != nil {
		// Check if database is still available before trying to delete
		if dbHealthErr := m.database.HealthCheck(); dbHealthErr == nil {
			if err := m.database.DeleteSession(sessionID); err != nil {
				m.logger.Error("Failed to delete session from database", err, map[string]interface{}{
					"session_id": sessionID,
				})
				// Don't return error here as we still want to clean up the in-memory session
			}
		} else {
			m.logger.Debug("Database not available for session deletion", map[string]interface{}{
				"session_id": sessionID,
				"error":      dbHealthErr.Error(),
			})
		}
	}

	// Log session closure with statistics
	successRate := 0.0
	if session.CommandCount > 0 {
		successRate = float64(session.SuccessCount) / float64(session.CommandCount)
	}

	m.logger.LogSessionEvent("closed", sessionID, session.Name, map[string]interface{}{
		"project_id":       session.ProjectID,
		"command_count":    session.CommandCount,
		"success_count":    session.SuccessCount,
		"success_rate":     successRate,
		"total_duration":   session.TotalDuration.String(),
		"session_duration": time.Since(session.CreatedAt).String(),
	})

	delete(m.sessions, sessionID)
	return nil
}

// SessionExists checks if a session with the given ID exists
func (m *Manager) SessionExists(sessionID string) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	_, exists := m.sessions[sessionID]
	return exists
}

// DeleteSession deletes a specific session
func (m *Manager) DeleteSession(sessionID string) error {
	return m.CloseSession(sessionID)
}

// DeleteProjectSessions deletes all sessions for a specific project
func (m *Manager) DeleteProjectSessions(projectID string) ([]string, error) {
	m.mutex.RLock()
	// Collect session IDs to delete
	var sessionIDs []string
	for id, session := range m.sessions {
		if session.ProjectID == projectID {
			sessionIDs = append(sessionIDs, id)
		}
	}
	m.mutex.RUnlock()

	// Delete each session
	var deletedSessions []string
	for _, sessionID := range sessionIDs {
		if err := m.CloseSession(sessionID); err != nil {
			m.logger.Error("Failed to delete session", err, map[string]interface{}{
				"session_id": sessionID,
				"project_id": projectID,
			})
			continue
		}
		deletedSessions = append(deletedSessions, sessionID)
	}

	return deletedSessions, nil
}

// GetProjectIDGenerator returns the project ID generator
func (m *Manager) GetProjectIDGenerator() *utils.ProjectIDGenerator {
	return m.projectIDGen
}

// ListSessionsByProject returns all sessions for a specific project
func (m *Manager) ListSessionsByProject(projectID string) []*Session {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var projectSessions []*Session
	for _, session := range m.sessions {
		if session.ProjectID == projectID {
			sessionCopy := m.copySession(session)
			projectSessions = append(projectSessions, sessionCopy)
		}
	}

	return projectSessions
}

// GetSessionStats returns statistics for all sessions
func (m *Manager) GetSessionStats() SessionStats {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	stats := SessionStats{
		TotalSessions: len(m.sessions),
		Projects:      make(map[string]int),
	}

	for _, session := range m.sessions {
		session.mutex.RLock()
		stats.TotalCommands += session.CommandCount
		stats.TotalSuccessful += session.SuccessCount
		stats.Projects[session.ProjectID]++

		if session.IsActive {
			stats.ActiveSessions++
		}
		session.mutex.RUnlock()
	}

	if stats.TotalCommands > 0 {
		stats.OverallSuccessRate = float64(stats.TotalSuccessful) / float64(stats.TotalCommands)
	}

	return stats
}

// M9: GetSessionActivityMetrics returns detailed activity metrics for a session
func (m *Manager) GetSessionActivityMetrics(sessionID string) (*SessionActivityMetrics, error) {
	session, err := m.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	session.mutex.RLock()
	defer session.mutex.RUnlock()

	metrics := &SessionActivityMetrics{
		SessionID:          session.ID,
		SessionName:        session.Name,
		ProjectID:          session.ProjectID,
		TotalCommands:      session.CommandCount,
		SuccessfulCommands: session.SuccessCount,
		FailedCommands:     session.CommandCount - session.SuccessCount,
		TotalExecutionTime: session.TotalDuration,
		SessionDuration:    time.Since(session.CreatedAt),
		LastCommandTime:    session.LastUsedAt,
		IdleTime:           time.Since(session.LastUsedAt),
	}

	// Calculate success rate
	if metrics.TotalCommands > 0 {
		metrics.SuccessRate = float64(metrics.SuccessfulCommands) / float64(metrics.TotalCommands)
		metrics.AverageExecutionTime = metrics.TotalExecutionTime / time.Duration(metrics.TotalCommands)
	}

	// Calculate commands per minute
	if metrics.SessionDuration > 0 {
		minutes := metrics.SessionDuration.Minutes()
		if minutes > 0 {
			metrics.CommandsPerMinute = float64(metrics.TotalCommands) / minutes
		}
	}

	// Get background process stats
	metrics.TotalBackgroundProcs = len(session.BackgroundProcesses)
	for _, proc := range session.BackgroundProcesses {
		proc.Mutex.RLock()
		if proc.IsRunning {
			metrics.ActiveBackgroundProcs++
		}
		proc.Mutex.RUnlock()
	}

	// Get detailed metrics from activity tracker if available
	if session.activityTracker != nil {
		cmdTypes, errorCats, maxTime, minTime, peakHour := session.activityTracker.GetMetrics()
		metrics.CommandTypeDistribution = cmdTypes
		metrics.ErrorCategories = errorCats
		metrics.MaxExecutionTime = maxTime
		metrics.MinExecutionTime = minTime
		metrics.PeakActivityHour = peakHour
	}

	return metrics, nil
}

// M9: GetAllSessionActivityMetrics returns activity metrics for all sessions
func (m *Manager) GetAllSessionActivityMetrics() []*SessionActivityMetrics {
	m.mutex.RLock()
	sessionIDs := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		sessionIDs = append(sessionIDs, id)
	}
	m.mutex.RUnlock()

	metrics := make([]*SessionActivityMetrics, 0, len(sessionIDs))
	for _, id := range sessionIDs {
		if m, err := m.GetSessionActivityMetrics(id); err == nil {
			metrics = append(metrics, m)
		}
	}

	return metrics
}

// getTotalBackgroundProcesses returns the total number of background processes across all sessions
func (m *Manager) getTotalBackgroundProcesses() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	total := 0
	for _, session := range m.sessions {
		session.mutex.RLock()
		total += len(session.BackgroundProcesses)
		session.mutex.RUnlock()
	}
	return total
}

// GetResourceMonitor returns the resource monitor instance
func (m *Manager) GetResourceMonitor() *monitoring.ResourceMonitor {
	return m.resourceMonitor
}

// startCleanupRoutine starts the automatic cleanup routine for inactive sessions
func (m *Manager) startCleanupRoutine() {
	m.cleanupTicker = time.NewTicker(m.config.Session.CleanupInterval)

	go func() {
		// Panic recovery to prevent server crashes
		defer func() {
			if r := recover(); r != nil {
				m.logger.Error("Panic in cleanup routine", fmt.Errorf("panic: %v", r), map[string]interface{}{
					"routine": "session_cleanup",
				})
				// Attempt to restart the cleanup routine after a delay
				time.Sleep(5 * time.Second)
				m.startCleanupRoutine()
			}
		}()

		for {
			select {
			case <-m.cleanupTicker.C:
				m.cleanupInactiveSessions()
			case <-m.stopCleanup:
				m.cleanupTicker.Stop()
				return
			case <-m.ctx.Done():
				m.cleanupTicker.Stop()
				return
			}
		}
	}()
}

// startResourceCleanupRoutine starts the automatic resource cleanup routine
func (m *Manager) startResourceCleanupRoutine() {
	m.resourceTicker = time.NewTicker(m.config.Session.ResourceCleanupInterval)

	go func() {
		// Panic recovery to prevent server crashes
		defer func() {
			if r := recover(); r != nil {
				m.logger.Error("Panic in resource cleanup routine", fmt.Errorf("panic: %v", r), map[string]interface{}{
					"routine": "resource_cleanup",
				})
				// Attempt to restart the resource cleanup routine after a delay
				time.Sleep(5 * time.Second)
				m.startResourceCleanupRoutine()
			}
		}()

		for {
			select {
			case <-m.resourceTicker.C:
				m.cleanupResources()
			case <-m.stopResourceCleanup:
				m.resourceTicker.Stop()
				return
			case <-m.ctx.Done():
				m.resourceTicker.Stop()
				return
			}
		}
	}()
}

// cleanupInactiveSessions removes sessions that have been inactive for too long
func (m *Manager) cleanupInactiveSessions() {
	m.mutex.RLock()
	var sessionsToCleanup []string
	cutoffTime := time.Now().Add(-m.config.Session.DefaultTimeout)

	for sessionID, session := range m.sessions {
		session.mutex.RLock()
		if session.IsActive && session.LastUsedAt.Before(cutoffTime) {
			sessionsToCleanup = append(sessionsToCleanup, sessionID)
		}
		session.mutex.RUnlock()
	}
	m.mutex.RUnlock()

	// Close inactive sessions
	for _, sessionID := range sessionsToCleanup {
		m.logger.Info("Cleaning up inactive session", map[string]interface{}{
			"session_id": sessionID,
			"reason":     "inactive_timeout",
		})

		if err := m.CloseSession(sessionID); err != nil {
			m.logger.Error("Failed to cleanup session", err, map[string]interface{}{
				"session_id": sessionID,
			})
		}
	}
}

// cleanupResources performs automatic resource cleanup based on configuration limits
func (m *Manager) cleanupResources() {
	// C4 FIX: Collect information while holding read lock, then release before operations

	// 1. Check if we need to cleanup excess sessions
	m.mutex.RLock()
	needSessionCleanup := len(m.sessions) > m.config.Session.MaxSessions

	// 2. Collect sessions that need background process cleanup
	type sessionCleanupInfo struct {
		sessionID           string
		needsBgCleanup      bool
		processesToTruncate []string
	}
	var cleanupInfos []sessionCleanupInfo

	for sessionID, session := range m.sessions {
		session.mutex.RLock()
		info := sessionCleanupInfo{
			sessionID:      sessionID,
			needsBgCleanup: len(session.BackgroundProcesses) > m.config.Session.MaxBackgroundProcesses,
		}
		for procID := range session.BackgroundProcesses {
			info.processesToTruncate = append(info.processesToTruncate, procID)
		}
		session.mutex.RUnlock()
		cleanupInfos = append(cleanupInfos, info)
	}
	m.mutex.RUnlock()

	// 3. Enforce maximum sessions limit (no locks held)
	if needSessionCleanup {
		m.cleanupExcessSessions()
	}

	// 4. Cleanup background processes and truncate output (acquire locks individually)
	for _, info := range cleanupInfos {
		m.mutex.RLock()
		session, exists := m.sessions[info.sessionID]
		m.mutex.RUnlock()

		if !exists {
			continue
		}

		session.mutex.Lock()

		// Enforce maximum background processes per session
		if info.needsBgCleanup && len(session.BackgroundProcesses) > m.config.Session.MaxBackgroundProcesses {
			m.cleanupExcessBackgroundProcesses(session)
		}

		// Truncate background process output to limit
		for _, proc := range session.BackgroundProcesses {
			proc.TruncateOutput(m.config.Session.BackgroundOutputLimit)
		}

		session.mutex.Unlock()
	}

	// 5. Cleanup command history if database is available
	if m.database != nil {
		m.cleanupExcessCommands()
	}

	m.logger.Debug("Resource cleanup completed", map[string]interface{}{
		"active_sessions":      len(m.sessions),
		"max_sessions":         m.config.Session.MaxSessions,
		"background_limit":     m.config.Session.MaxBackgroundProcesses,
		"output_limit":         m.config.Session.BackgroundOutputLimit,
		"commands_per_session": m.config.Session.MaxCommandsPerSession,
	})
}

// cleanupExcessSessions removes oldest sessions when over limit
func (m *Manager) cleanupExcessSessions() {
	type sessionAge struct {
		id       string
		lastUsed time.Time
	}

	// Collect sessions with their last used times
	var sessions []sessionAge
	for id, session := range m.sessions {
		sessions = append(sessions, sessionAge{
			id:       id,
			lastUsed: session.LastUsedAt,
		})
	}

	// Sort by last used time (oldest first) using efficient sort.Slice
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].lastUsed.Before(sessions[j].lastUsed)
	})

	// Remove excess sessions (oldest first)
	excessCount := len(sessions) - m.config.Session.MaxSessions
	for i := 0; i < excessCount; i++ {
		sessionID := sessions[i].id
		m.logger.Info("Cleaning up excess session", map[string]interface{}{
			"session_id": sessionID,
			"reason":     "max_sessions_exceeded",
			"max_limit":  m.config.Session.MaxSessions,
		})

		// Note: We need to release the read lock before calling CloseSession
		go func(id string) {
			if err := m.CloseSession(id); err != nil {
				m.logger.Error("Failed to cleanup excess session", err, map[string]interface{}{
					"session_id": id,
				})
			}
		}(sessionID)
	}
}

// cleanupExcessBackgroundProcesses removes oldest background processes when over limit
func (m *Manager) cleanupExcessBackgroundProcesses(session *Session) {
	type processAge struct {
		id        string
		startTime time.Time
	}

	// Collect background processes with their start times
	var processes []processAge
	for id, proc := range session.BackgroundProcesses {
		processes = append(processes, processAge{
			id:        id,
			startTime: proc.StartTime,
		})
	}

	// Sort by start time (oldest first) using efficient sort.Slice
	sort.Slice(processes, func(i, j int) bool {
		return processes[i].startTime.Before(processes[j].startTime)
	})

	// Remove excess background processes (oldest first)
	excessCount := len(processes) - m.config.Session.MaxBackgroundProcesses
	for i := 0; i < excessCount; i++ {
		processID := processes[i].id
		if proc, exists := session.BackgroundProcesses[processID]; exists {
			// Kill the process if it's still running
			if proc.IsRunning && proc.cmd != nil && proc.cmd.Process != nil {
				proc.cmd.Process.Kill()
			}
			delete(session.BackgroundProcesses, processID)

			m.logger.Info("Cleaned up excess background process", map[string]interface{}{
				"session_id": session.ID,
				"process_id": processID,
				"reason":     "max_background_processes_exceeded",
				"max_limit":  m.config.Session.MaxBackgroundProcesses,
			})
		}
	}
}

// cleanupExcessCommands removes old commands from database when over limit
func (m *Manager) cleanupExcessCommands() {
	// M1: Use the database method to cleanup excess commands
	if m.database == nil {
		return
	}

	// Check database health before cleanup
	if err := m.database.HealthCheck(); err != nil {
		m.logger.Debug("Database not available for command cleanup", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	// Cleanup excess commands per session
	deleted, err := m.database.CleanupExcessCommands(m.config.Session.MaxCommandsPerSession)
	if err != nil {
		m.logger.Error("Failed to cleanup excess commands", err, map[string]interface{}{
			"max_commands_per_session": m.config.Session.MaxCommandsPerSession,
		})
		return
	}

	if deleted > 0 {
		m.logger.Info("Cleaned up excess commands", map[string]interface{}{
			"deleted_count":            deleted,
			"max_commands_per_session": m.config.Session.MaxCommandsPerSession,
		})
	}

	// Also cleanup old stream chunks (older than 24 hours)
	chunksDeleted, err := m.database.CleanupOldStreamChunks(24 * time.Hour)
	if err != nil {
		m.logger.Error("Failed to cleanup old stream chunks", err, nil)
	} else if chunksDeleted > 0 {
		m.logger.Debug("Cleaned up old stream chunks", map[string]interface{}{
			"deleted_count": chunksDeleted,
		})
	}
}

// Shutdown gracefully shuts down the manager
func (m *Manager) Shutdown() {
	// Cancel manager context to signal all operations to stop
	if m.cancel != nil {
		m.cancel()
	}

	close(m.stopCleanup)
	close(m.stopResourceCleanup)

	// Stop resource monitor
	if m.resourceMonitor != nil {
		m.resourceMonitor.Stop()
	}

	// Close all active sessions
	m.mutex.RLock()
	sessionIDs := make([]string, 0, len(m.sessions))
	for sessionID := range m.sessions {
		sessionIDs = append(sessionIDs, sessionID)
	}
	m.mutex.RUnlock()

	for _, sessionID := range sessionIDs {
		if err := m.CloseSession(sessionID); err != nil {
			m.logger.Error("Failed to close session during shutdown", err, map[string]interface{}{
				"session_id": sessionID,
			})
		}
	}
}

// GetGoroutineCount returns the current number of goroutines (for testing)
func GetGoroutineCount() int {
	return runtime.NumGoroutine()
}

// copySession creates a safe copy of a session for external use
func (m *Manager) copySession(session *Session) *Session {
	session.mutex.RLock()
	defer session.mutex.RUnlock()

	envCopy := make(map[string]string)
	for k, v := range session.Environment {
		envCopy[k] = v
	}

	return &Session{
		ID:            session.ID,
		Name:          session.Name,
		ProjectID:     session.ProjectID,
		WorkingDir:    session.WorkingDir,
		Environment:   envCopy,
		CreatedAt:     session.CreatedAt,
		LastUsedAt:    session.LastUsedAt,
		IsActive:      session.IsActive,
		CommandCount:  session.CommandCount,
		SuccessCount:  session.SuccessCount,
		TotalDuration: session.TotalDuration,
		currentDir:    session.currentDir,
	}
}

// SessionStats contains statistics about all sessions
type SessionStats struct {
	TotalSessions      int            `json:"total_sessions"`
	ActiveSessions     int            `json:"active_sessions"`
	TotalCommands      int            `json:"total_commands"`
	TotalSuccessful    int            `json:"total_successful"`
	OverallSuccessRate float64        `json:"overall_success_rate"`
	Projects           map[string]int `json:"projects"` // project_id -> session_count
}

// M9: SessionActivityMetrics provides detailed activity metrics for a session
type SessionActivityMetrics struct {
	SessionID   string `json:"session_id"`
	SessionName string `json:"session_name"`
	ProjectID   string `json:"project_id"`

	// Command statistics
	TotalCommands      int     `json:"total_commands"`
	SuccessfulCommands int     `json:"successful_commands"`
	FailedCommands     int     `json:"failed_commands"`
	SuccessRate        float64 `json:"success_rate"`

	// Timing statistics
	TotalExecutionTime   time.Duration `json:"-"`
	AverageExecutionTime time.Duration `json:"-"`
	MaxExecutionTime     time.Duration `json:"-"`
	MinExecutionTime     time.Duration `json:"-"`

	// Activity patterns
	CommandsPerMinute float64       `json:"commands_per_minute"`
	LastCommandTime   time.Time     `json:"-"`
	SessionDuration   time.Duration `json:"-"`
	IdleTime          time.Duration `json:"-"`

	// Background process stats
	TotalBackgroundProcs  int `json:"total_background_procs"`
	ActiveBackgroundProcs int `json:"active_background_procs"`

	// Command type distribution
	CommandTypeDistribution map[string]int `json:"command_type_distribution"`

	// Peak activity
	PeakActivityHour int `json:"peak_activity_hour"` // Hour of day with most activity

	// Error categories
	ErrorCategories map[string]int `json:"error_categories"`
}

// sessionActivityMetricsJSON is used for custom JSON marshaling
type sessionActivityMetricsJSON struct {
	SessionID               string         `json:"session_id"`
	SessionName             string         `json:"session_name"`
	ProjectID               string         `json:"project_id"`
	TotalCommands           int            `json:"total_commands"`
	SuccessfulCommands      int            `json:"successful_commands"`
	FailedCommands          int            `json:"failed_commands"`
	SuccessRate             float64        `json:"success_rate"`
	TotalExecutionTime      int64          `json:"total_execution_time"`
	AverageExecutionTime    int64          `json:"average_execution_time"`
	MaxExecutionTime        int64          `json:"max_execution_time"`
	MinExecutionTime        int64          `json:"min_execution_time"`
	CommandsPerMinute       float64        `json:"commands_per_minute"`
	LastCommandTime         string         `json:"last_command_time"`
	SessionDuration         int64          `json:"session_duration"`
	IdleTime                int64          `json:"idle_time"`
	TotalBackgroundProcs    int            `json:"total_background_procs"`
	ActiveBackgroundProcs   int            `json:"active_background_procs"`
	CommandTypeDistribution map[string]int `json:"command_type_distribution"`
	PeakActivityHour        int            `json:"peak_activity_hour"`
	ErrorCategories         map[string]int `json:"error_categories"`
}

// MarshalJSON implements custom JSON marshaling for SessionActivityMetrics
func (m *SessionActivityMetrics) MarshalJSON() ([]byte, error) {
	return json.Marshal(sessionActivityMetricsJSON{
		SessionID:               m.SessionID,
		SessionName:             m.SessionName,
		ProjectID:               m.ProjectID,
		TotalCommands:           m.TotalCommands,
		SuccessfulCommands:      m.SuccessfulCommands,
		FailedCommands:          m.FailedCommands,
		SuccessRate:             m.SuccessRate,
		TotalExecutionTime:      int64(m.TotalExecutionTime),
		AverageExecutionTime:    int64(m.AverageExecutionTime),
		MaxExecutionTime:        int64(m.MaxExecutionTime),
		MinExecutionTime:        int64(m.MinExecutionTime),
		CommandsPerMinute:       m.CommandsPerMinute,
		LastCommandTime:         m.LastCommandTime.Format(time.RFC3339),
		SessionDuration:         int64(m.SessionDuration),
		IdleTime:                int64(m.IdleTime),
		TotalBackgroundProcs:    m.TotalBackgroundProcs,
		ActiveBackgroundProcs:   m.ActiveBackgroundProcs,
		CommandTypeDistribution: m.CommandTypeDistribution,
		PeakActivityHour:        m.PeakActivityHour,
		ErrorCategories:         m.ErrorCategories,
	})
}

// SessionActivityTracker tracks detailed activity for a session
type SessionActivityTracker struct {
	commandTimes      []time.Duration
	commandTimestamps []time.Time
	commandTypes      map[string]int
	errorCategories   map[string]int
	maxExecutionTime  time.Duration
	minExecutionTime  time.Duration
	hourlyActivity    [24]int // Commands per hour of day
	mutex             sync.RWMutex
}

// NewSessionActivityTracker creates a new activity tracker
func NewSessionActivityTracker() *SessionActivityTracker {
	return &SessionActivityTracker{
		commandTimes:      make([]time.Duration, 0),
		commandTimestamps: make([]time.Time, 0),
		commandTypes:      make(map[string]int),
		errorCategories:   make(map[string]int),
		minExecutionTime:  time.Duration(1<<63 - 1), // Max duration as initial min
	}
}

// RecordCommand records command execution metrics
func (sat *SessionActivityTracker) RecordCommand(duration time.Duration, command string, success bool, errorMsg string) {
	sat.mutex.Lock()
	defer sat.mutex.Unlock()

	now := time.Now()
	sat.commandTimes = append(sat.commandTimes, duration)
	sat.commandTimestamps = append(sat.commandTimestamps, now)

	// Track max/min execution times
	if duration > sat.maxExecutionTime {
		sat.maxExecutionTime = duration
	}
	if duration < sat.minExecutionTime {
		sat.minExecutionTime = duration
	}

	// Track hourly activity
	sat.hourlyActivity[now.Hour()]++

	// Categorize command type (extract first word)
	cmdType := extractCommandType(command)
	sat.commandTypes[cmdType]++

	// Track error categories
	if !success && errorMsg != "" {
		category := categorizeError(errorMsg)
		sat.errorCategories[category]++
	}

	// Keep only last 1000 command times to prevent memory bloat
	if len(sat.commandTimes) > 1000 {
		sat.commandTimes = sat.commandTimes[len(sat.commandTimes)-1000:]
		sat.commandTimestamps = sat.commandTimestamps[len(sat.commandTimestamps)-1000:]
	}
}

// GetMetrics returns the current activity metrics
func (sat *SessionActivityTracker) GetMetrics() (commandTypes map[string]int, errorCats map[string]int, maxTime, minTime time.Duration, peakHour int) {
	sat.mutex.RLock()
	defer sat.mutex.RUnlock()

	// Copy maps to avoid race conditions
	commandTypes = make(map[string]int, len(sat.commandTypes))
	for k, v := range sat.commandTypes {
		commandTypes[k] = v
	}

	errorCats = make(map[string]int, len(sat.errorCategories))
	for k, v := range sat.errorCategories {
		errorCats[k] = v
	}

	maxTime = sat.maxExecutionTime
	minTime = sat.minExecutionTime
	if minTime == time.Duration(1<<63-1) {
		minTime = 0
	}

	// Find peak activity hour
	maxActivity := 0
	for hour, count := range sat.hourlyActivity {
		if count > maxActivity {
			maxActivity = count
			peakHour = hour
		}
	}

	return
}

// extractCommandType extracts the command type from a command string
func extractCommandType(command string) string {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "empty"
	}
	// Get just the base command name without path
	cmd := parts[0]
	if idx := strings.LastIndex(cmd, "/"); idx != -1 {
		cmd = cmd[idx+1:]
	}
	return cmd
}

// categorizeError categorizes an error message into a category
func categorizeError(errorMsg string) string {
	lowerErr := strings.ToLower(errorMsg)

	switch {
	case strings.Contains(lowerErr, "timeout"):
		return "timeout"
	case strings.Contains(lowerErr, "permission") || strings.Contains(lowerErr, "denied"):
		return "permission"
	case strings.Contains(lowerErr, "not found") || strings.Contains(lowerErr, "no such"):
		return "not_found"
	case strings.Contains(lowerErr, "connection") || strings.Contains(lowerErr, "network"):
		return "network"
	case strings.Contains(lowerErr, "memory") || strings.Contains(lowerErr, "oom"):
		return "memory"
	case strings.Contains(lowerErr, "syntax") || strings.Contains(lowerErr, "parse"):
		return "syntax"
	case strings.Contains(lowerErr, "signal") || strings.Contains(lowerErr, "killed"):
		return "signal"
	default:
		return "other"
	}
}

// ExecuteCommandWithTimeout executes a command with a timeout
func (m *Manager) ExecuteCommandWithTimeout(sessionID, command string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	session, err := m.GetSession(sessionID)
	if err != nil {
		return "", fmt.Errorf("session not found: %v", err)
	}

	// Use the existing executeCommandInSession method with timeout context
	output, _, err := m.executeCommandInSession(ctx, session, command)
	return output, err
}

// ExecuteCommandInBackground executes a command in background mode with proper process tracking
func (m *Manager) ExecuteCommandInBackground(sessionID, command string) (string, error) {
	session, err := m.GetSession(sessionID)
	if err != nil {
		return "", fmt.Errorf("session not found: %v", err)
	}

	// Check if session context is cancelled before starting
	select {
	case <-session.ctx.Done():
		return "", fmt.Errorf("session is shutting down: %v", session.ctx.Err())
	default:
		// Continue with background process creation
	}

	// Check background process limit
	session.mutex.Lock()
	if len(session.BackgroundProcesses) >= m.config.Session.MaxBackgroundProcesses {
		// Cleanup excess background processes first
		m.cleanupExcessBackgroundProcesses(session)

		// Check again after cleanup
		if len(session.BackgroundProcesses) >= m.config.Session.MaxBackgroundProcesses {
			session.mutex.Unlock()
			return "", fmt.Errorf("maximum number of background processes (%d) reached for session %s", m.config.Session.MaxBackgroundProcesses, sessionID)
		}
	}
	session.mutex.Unlock()

	// Generate unique process ID
	processID := uuid.New().String()

	// Create background process tracking
	bgProcess := &BackgroundProcess{
		ID:        processID,
		Command:   command,
		StartTime: time.Now(),
		IsRunning: true,
	}

	// Store background process in session immediately
	session.mutex.Lock()
	session.BackgroundProcesses[processID] = bgProcess
	session.mutex.Unlock()

	// Start the command in the background with proper process tracking
	go func() {
		// Check context again at start of goroutine
		select {
		case <-session.ctx.Done():
			bgProcess.Mutex.Lock()
			bgProcess.IsRunning = false
			bgProcess.ExitCode = -1
			bgProcess.ErrorOutput = fmt.Sprintf("Session context cancelled: %v", session.ctx.Err())
			bgProcess.Mutex.Unlock()
			return
		default:
			// Continue with command execution
		}

		// H1: Use configurable timeout from config instead of hardcoded 24 hours
		bgTimeout := m.config.Session.BackgroundProcessTimeout
		if bgTimeout <= 0 {
			bgTimeout = 4 * time.Hour // Fallback to 4 hours if not configured
		}
		ctx, cancel := context.WithTimeout(session.ctx, bgTimeout)
		defer cancel()

		startTime := time.Now()

		// Prepare command for execution
		parts := strings.Fields(command)
		if len(parts) == 0 {
			m.logger.Error("Empty command provided", nil)
			bgProcess.Mutex.Lock()
			bgProcess.IsRunning = false
			bgProcess.ExitCode = -1
			bgProcess.ErrorOutput = "Empty command provided"
			bgProcess.Mutex.Unlock()
			return
		}

		// Create the command with proper working directory and environment
		cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
		cmd.Dir = session.currentDir

		// Set environment variables
		cmd.Env = make([]string, 0, len(session.Environment))
		for key, value := range session.Environment {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
		}

		// M6: Apply resource limits if enabled
		if m.config.Session.EnableResourceLimits {
			limits := ResourceLimits{
				MaxMemoryMB:   m.config.Session.MaxProcessMemoryMB,
				MaxFileSizeMB: m.config.Session.MaxProcessFilesMB,
				Nice:          m.config.Session.ProcessNice,
				Enabled:       true,
			}
			if err := applyResourceLimits(cmd, limits); err != nil {
				m.logger.Warn("Failed to apply resource limits (continuing anyway)", map[string]interface{}{
					"error":      err.Error(),
					"process_id": processID,
				})
			}
		}

		// Create pipes for output capture with proper cleanup
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			m.logger.Error("Failed to create stdout pipe", err)
			bgProcess.Mutex.Lock()
			bgProcess.IsRunning = false
			bgProcess.ExitCode = -1
			bgProcess.ErrorOutput = fmt.Sprintf("Failed to create stdout pipe: %v", err)
			bgProcess.Mutex.Unlock()
			return
		}
		defer func() {
			if stdout != nil {
				stdout.Close()
			}
		}()

		stderr, err := cmd.StderrPipe()
		if err != nil {
			m.logger.Error("Failed to create stderr pipe", err)
			bgProcess.Mutex.Lock()
			bgProcess.IsRunning = false
			bgProcess.ExitCode = -1
			bgProcess.ErrorOutput = fmt.Sprintf("Failed to create stderr pipe: %v", err)
			bgProcess.Mutex.Unlock()
			return
		}
		defer func() {
			if stderr != nil {
				stderr.Close()
			}
		}()

		// Update background process with cmd reference
		bgProcess.Mutex.Lock()
		bgProcess.cmd = cmd
		bgProcess.Mutex.Unlock()

		// Start the command
		if err := cmd.Start(); err != nil {
			m.logger.Error("Failed to start background command", err)
			bgProcess.Mutex.Lock()
			bgProcess.IsRunning = false
			bgProcess.ExitCode = -1
			bgProcess.ErrorOutput = fmt.Sprintf("Failed to start command: %v", err)
			bgProcess.Mutex.Unlock()
			return
		}

		// Update PID
		bgProcess.Mutex.Lock()
		bgProcess.PID = cmd.Process.Pid
		bgProcess.cmd = cmd
		bgProcess.Mutex.Unlock()

		// M6: Apply runtime resource limits (like nice value) after process starts
		if m.config.Session.EnableResourceLimits && cmd.Process.Pid > 0 {
			limits := ResourceLimits{
				MaxMemoryMB:   m.config.Session.MaxProcessMemoryMB,
				MaxFileSizeMB: m.config.Session.MaxProcessFilesMB,
				Nice:          m.config.Session.ProcessNice,
				Enabled:       true,
			}
			if err := setResourceLimits(cmd.Process.Pid, limits); err != nil {
				m.logger.Warn("Failed to apply runtime resource limits", map[string]interface{}{
					"error":      err.Error(),
					"process_id": processID,
					"pid":        cmd.Process.Pid,
				})
			} else {
				m.logger.Debug("Applied resource limits to background process", map[string]interface{}{
					"process_id":    processID,
					"pid":           cmd.Process.Pid,
					"nice":          limits.Nice,
					"max_memory_mb": limits.MaxMemoryMB,
					"max_file_mb":   limits.MaxFileSizeMB,
				})
			}
		}

		// Use WaitGroup to wait for output capture goroutines with timeout protection
		var outputWg sync.WaitGroup
		outputWg.Add(2)

		// C2 FIX: Use buffered channels and proper synchronization to prevent race conditions
		// Create done channel to signal all goroutines to stop
		done := make(chan struct{})

		// Stdout capture goroutine with proper synchronization
		go func() {
			defer outputWg.Done()
			defer func() {
				if r := recover(); r != nil {
					m.logger.Error("Panic in stdout capture goroutine", fmt.Errorf("panic: %v", r))
				}
			}()

			scanner := bufio.NewScanner(stdout)
			scanner.Split(bufio.ScanLines)

			// C2 FIX: Use buffered channel to prevent blocking
			lineChan := make(chan string, 100)

			// Scanner goroutine
			go func() {
				defer close(lineChan)
				for scanner.Scan() {
					select {
					case lineChan <- scanner.Text():
					case <-done:
						return
					case <-ctx.Done():
						return
					}
				}
			}()

			// C2 FIX: Drain channel properly until closed or done
			for {
				select {
				case line, ok := <-lineChan:
					if !ok {
						return // Channel closed, scanner finished
					}
					bgProcess.UpdateOutput(line+"\n", m.config.Session.BackgroundOutputLimit)
				case <-done:
					return
				case <-ctx.Done():
					return
				}
			}
		}()

		// Stderr capture goroutine with proper synchronization
		go func() {
			defer outputWg.Done()
			defer func() {
				if r := recover(); r != nil {
					m.logger.Error("Panic in stderr capture goroutine", fmt.Errorf("panic: %v", r))
				}
			}()

			scanner := bufio.NewScanner(stderr)
			scanner.Split(bufio.ScanLines)

			// C2 FIX: Use buffered channel to prevent blocking
			lineChan := make(chan string, 100)

			// Scanner goroutine
			go func() {
				defer close(lineChan)
				for scanner.Scan() {
					select {
					case lineChan <- scanner.Text():
					case <-done:
						return
					case <-ctx.Done():
						return
					}
				}
			}()

			// C2 FIX: Drain channel properly until closed or done
			for {
				select {
				case line, ok := <-lineChan:
					if !ok {
						return // Channel closed, scanner finished
					}
					bgProcess.UpdateErrorOutput(line+"\n", m.config.Session.BackgroundOutputLimit)
				case <-done:
					return
				case <-ctx.Done():
					return
				}
			}
		}()

		// Wait for command completion with timeout protection
		execErr := cmd.Wait()

		// C2 FIX: Signal done to all goroutines after command completes
		close(done)

		// Wait for output capture goroutines to complete with timeout
		outputDone := make(chan struct{})
		go func() {
			outputWg.Wait()
			close(outputDone)
		}()

		select {
		case <-outputDone:
			// Output capture completed normally
		case <-time.After(30 * time.Second):
			// Force timeout for output capture
			m.logger.Warn("Output capture timeout, forcing completion", map[string]interface{}{
				"process_id": processID,
				"command":    command,
			})
		}

		endTime := time.Now()
		duration := endTime.Sub(startTime)
		exitCode := 0

		if execErr != nil {
			if exitError, ok := execErr.(*exec.ExitError); ok {
				exitCode = exitError.ExitCode()
			} else {
				exitCode = -1
			}
		}

		// Update background process status
		bgProcess.Mutex.Lock()
		bgProcess.IsRunning = false
		bgProcess.ExitCode = exitCode
		bgProcess.Mutex.Unlock()

		// Store the command result in history
		success := execErr == nil && exitCode == 0

		// Store in database (check if database is still available)
		if m.database != nil {
			// Check database health before using it
			if dbHealthErr := m.database.HealthCheck(); dbHealthErr == nil {
				if storeErr := m.database.StoreCommand(
					sessionID,
					session.ProjectID,
					command,
					bgProcess.Output,
					exitCode,
					success,
					startTime,
					endTime,
					duration,
					session.WorkingDir,
				); storeErr != nil {
					m.logger.Error("Failed to store background command", storeErr)
				}
			} else {
				m.logger.Debug("Database not available for storing background command", map[string]interface{}{
					"session_id": sessionID,
					"error":      dbHealthErr.Error(),
				})
			}
		}

		m.logger.Info("Background command completed", map[string]interface{}{
			"session_id": sessionID,
			"process_id": processID,
			"command":    command,
			"success":    success,
			"duration":   duration.String(),
		})
	}()

	// Return immediately for background execution with process ID
	return processID, nil
}

// GetBackgroundProcess returns a background process by ID
func (m *Manager) GetBackgroundProcess(sessionID, processID string) (*BackgroundProcess, error) {
	session, err := m.GetSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %v", err)
	}

	session.mutex.RLock()
	defer session.mutex.RUnlock()

	if processID == "" {
		// Return the most recent background process
		var latest *BackgroundProcess
		var latestTime time.Time
		for _, proc := range session.BackgroundProcesses {
			if proc.StartTime.After(latestTime) {
				latest = proc
				latestTime = proc.StartTime
			}
		}
		if latest == nil {
			return nil, fmt.Errorf("no background processes found")
		}
		return latest, nil
	}

	proc, exists := session.BackgroundProcesses[processID]
	if !exists {
		return nil, fmt.Errorf("background process not found: %s", processID)
	}

	return proc, nil
}

// GetAllBackgroundProcesses returns all background processes across all sessions with optional filtering
func (m *Manager) GetAllBackgroundProcesses(sessionID, projectID string) (map[string]map[string]*BackgroundProcess, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	result := make(map[string]map[string]*BackgroundProcess)

	for _, session := range m.sessions {
		// Apply session filter if specified
		if sessionID != "" && session.ID != sessionID {
			continue
		}

		// Apply project filter if specified
		if projectID != "" && session.ProjectID != projectID {
			continue
		}

		session.mutex.RLock()
		if len(session.BackgroundProcesses) > 0 {
			sessionProcesses := make(map[string]*BackgroundProcess)
			for procID, proc := range session.BackgroundProcesses {
				sessionProcesses[procID] = proc
			}
			result[session.ID] = sessionProcesses
		}
		session.mutex.RUnlock()
	}

	return result, nil
}

// GracefulTerminationConfig holds configuration for graceful process termination
type GracefulTerminationConfig struct {
	GracePeriod     time.Duration // Time to wait after SIGTERM before SIGKILL
	UseProcessGroup bool          // Kill entire process group
	LogProgress     bool          // Log termination progress
}

// DefaultGracefulTerminationConfig returns default graceful termination settings
func DefaultGracefulTerminationConfig() GracefulTerminationConfig {
	return GracefulTerminationConfig{
		GracePeriod:     5 * time.Second,
		UseProcessGroup: true,
		LogProgress:     true,
	}
}

// TerminateBackgroundProcess terminates a specific background process
// M7: Implements graceful termination with SIGTERM -> grace period -> SIGKILL
func (m *Manager) TerminateBackgroundProcess(sessionID, processID string, force bool) error {
	return m.TerminateBackgroundProcessWithConfig(sessionID, processID, force, DefaultGracefulTerminationConfig())
}

// TerminateBackgroundProcessWithConfig terminates a background process with custom config
func (m *Manager) TerminateBackgroundProcessWithConfig(sessionID, processID string, force bool, config GracefulTerminationConfig) error {
	session, err := m.GetSession(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %v", err)
	}

	session.mutex.Lock()
	bgProcess, exists := session.BackgroundProcesses[processID]
	if !exists {
		session.mutex.Unlock()
		return fmt.Errorf("background process %s not found in session %s", processID, sessionID)
	}

	// Get process info while holding the lock
	isRunning := bgProcess.IsRunning
	cmd := bgProcess.cmd
	pid := 0
	if cmd != nil && cmd.Process != nil {
		pid = cmd.Process.Pid
	}
	session.mutex.Unlock()

	// Terminate the process if it's running
	if isRunning && cmd != nil && cmd.Process != nil {
		if force {
			// Force kill immediately
			if config.LogProgress {
				m.logger.Info("Force killing background process", map[string]interface{}{
					"session_id": sessionID,
					"process_id": processID,
					"pid":        pid,
				})
			}

			if config.UseProcessGroup {
				// Try to kill the entire process group
				pgid, err := syscall.Getpgid(pid)
				if err == nil {
					syscall.Kill(-pgid, syscall.SIGKILL)
				} else {
					cmd.Process.Kill()
				}
			} else {
				cmd.Process.Kill()
			}
		} else {
			// M7: Graceful termination: SIGTERM -> wait -> SIGKILL
			if config.LogProgress {
				m.logger.Info("Sending SIGTERM for graceful shutdown", map[string]interface{}{
					"session_id":   sessionID,
					"process_id":   processID,
					"pid":          pid,
					"grace_period": config.GracePeriod.String(),
				})
			}

			// Send SIGTERM first
			var termErr error
			if config.UseProcessGroup {
				pgid, err := syscall.Getpgid(pid)
				if err == nil {
					termErr = syscall.Kill(-pgid, syscall.SIGTERM)
				} else {
					termErr = cmd.Process.Signal(syscall.SIGTERM)
				}
			} else {
				termErr = cmd.Process.Signal(syscall.SIGTERM)
			}

			if termErr != nil {
				// If SIGTERM fails, go straight to kill
				if config.LogProgress {
					m.logger.Warn("SIGTERM failed, force killing", map[string]interface{}{
						"session_id": sessionID,
						"process_id": processID,
						"error":      termErr.Error(),
					})
				}
				cmd.Process.Kill()
			} else {
				// Wait for grace period, checking if process exited
				exited := m.waitForProcessExit(cmd, config.GracePeriod)

				if !exited {
					// Process didn't exit gracefully, send SIGKILL
					if config.LogProgress {
						m.logger.Info("Grace period expired, sending SIGKILL", map[string]interface{}{
							"session_id":   sessionID,
							"process_id":   processID,
							"pid":          pid,
							"grace_period": config.GracePeriod.String(),
						})
					}

					if config.UseProcessGroup {
						pgid, err := syscall.Getpgid(pid)
						if err == nil {
							syscall.Kill(-pgid, syscall.SIGKILL)
						} else {
							cmd.Process.Kill()
						}
					} else {
						cmd.Process.Kill()
					}
				} else if config.LogProgress {
					m.logger.Info("Process exited gracefully after SIGTERM", map[string]interface{}{
						"session_id": sessionID,
						"process_id": processID,
						"pid":        pid,
					})
				}
			}
		}

		// Update process status
		bgProcess.Mutex.Lock()
		bgProcess.IsRunning = false
		bgProcess.Mutex.Unlock()
	}

	// Remove from session background processes
	session.mutex.Lock()
	delete(session.BackgroundProcesses, processID)
	session.mutex.Unlock()

	return nil
}

// waitForProcessExit waits for a process to exit with a timeout
func (m *Manager) waitForProcessExit(cmd *exec.Cmd, timeout time.Duration) bool {
	done := make(chan struct{})

	go func() {
		cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// TerminateAllBackgroundProcesses gracefully terminates all background processes in a session
func (m *Manager) TerminateAllBackgroundProcesses(sessionID string, force bool, gracePeriod time.Duration) error {
	session, err := m.GetSession(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %v", err)
	}

	config := DefaultGracefulTerminationConfig()
	if gracePeriod > 0 {
		config.GracePeriod = gracePeriod
	}

	session.mutex.RLock()
	processIDs := make([]string, 0, len(session.BackgroundProcesses))
	for processID := range session.BackgroundProcesses {
		processIDs = append(processIDs, processID)
	}
	session.mutex.RUnlock()

	var lastErr error
	for _, processID := range processIDs {
		if err := m.TerminateBackgroundProcessWithConfig(sessionID, processID, force, config); err != nil {
			m.logger.Error("Failed to terminate background process", err, map[string]interface{}{
				"session_id": sessionID,
				"process_id": processID,
			})
			lastErr = err
		}
	}

	return lastErr
}
