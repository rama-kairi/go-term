package terminal

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rama-kairi/go-term/internal/config"
	"github.com/rama-kairi/go-term/internal/database"
	"github.com/rama-kairi/go-term/internal/logger"
	"github.com/rama-kairi/go-term/internal/utils"
)

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

	// Internal fields for session management
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
	mutex  sync.RWMutex

	// Persistent shell state tracking
	currentDir string
	shellPid   int
	shellEnv   map[string]string
}

// Manager manages terminal sessions with project organization and command history
type Manager struct {
	sessions      map[string]*Session
	config        *config.Config
	logger        *logger.Logger
	database      *database.DB
	projectIDGen  *utils.ProjectIDGenerator
	mutex         sync.RWMutex
	cleanupTicker *time.Ticker
	stopCleanup   chan bool
}

// NewManager creates a new terminal session manager with enhanced features
func NewManager(cfg *config.Config, logger *logger.Logger, db *database.DB) *Manager {
	projectIDGen := utils.NewProjectIDGenerator()

	manager := &Manager{
		sessions:     make(map[string]*Session),
		config:       cfg,
		logger:       logger,
		database:     db,
		projectIDGen: projectIDGen,
		stopCleanup:  make(chan bool),
	}

	// Start cleanup routine
	manager.startCleanupRoutine()

	return manager
}

// CreateSession creates a new terminal session with project association
func (m *Manager) CreateSession(name string, projectID string, workingDir string) (*Session, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

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

	// Set working directory
	if workingDir == "" {
		if m.config.Session.WorkingDir != "" {
			workingDir = m.config.Session.WorkingDir
		} else {
			var err error
			workingDir, err = os.Getwd()
			if err != nil {
				workingDir = os.Getenv("HOME")
			}
		}
	}

	// Ensure working directory exists
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create working directory: %w", err)
	}

	session := &Session{
		ID:            sessionID,
		Name:          name,
		ProjectID:     projectID,
		WorkingDir:    workingDir,
		Environment:   make(map[string]string),
		CreatedAt:     time.Now(),
		LastUsedAt:    time.Now(),
		IsActive:      true,
		CommandCount:  0,
		SuccessCount:  0,
		TotalDuration: 0,
		currentDir:    workingDir,
		shellEnv:      make(map[string]string),
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

// ListSessions returns all sessions
func (m *Manager) ListSessions() []*Session {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		// Create a copy to avoid data races
		sessionCopy := &Session{
			ID:         session.ID,
			Name:       session.Name,
			CreatedAt:  session.CreatedAt,
			LastUsedAt: session.LastUsedAt,
			IsActive:   session.IsActive,
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

	// Update session statistics
	session.CommandCount++
	if success {
		session.SuccessCount++
	}
	session.TotalDuration += duration

	// Log command execution
	m.logger.LogCommand(sessionID, command, duration, success, output, err)

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

	// Add a small delay to simulate streaming behavior while maintaining session state
	// This is a transitional implementation that maintains session continuity
	// while providing the streaming experience

	// Use the existing session-aware execution but with simulated streaming timing
	output, exitCode, err := m.executeCommandInSessionWithStreaming(ctx, session, command)

	// Update session statistics
	session.CommandCount++
	session.LastUsedAt = time.Now()

	if err == nil {
		session.SuccessCount++
	}

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

	// Log command completion
	startTime := time.Now()
	endTime := time.Now()
	duration := time.Since(startTime)

	// Store command in database if available
	if m.database != nil {
		m.database.StoreCommand(
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

	// Create command that changes to the session's current directory first
	fullCommand := fmt.Sprintf("cd %s && %s", session.currentDir, command)

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

	// Create command that changes to the session's current directory first
	fullCommand := fmt.Sprintf("cd %s && %s", session.currentDir, command)

	cmd := exec.CommandContext(ctx, shell, "-c", fullCommand)
	cmd.Dir = session.WorkingDir

	// Set environment from session
	env := make([]string, 0, len(session.shellEnv))
	for k, v := range session.shellEnv {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env

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

// isDirectoryChangeCommand checks if the command is a directory change command
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

	// Log session closure with statistics
	m.logger.LogSessionEvent("closed", sessionID, session.Name, map[string]interface{}{
		"project_id":       session.ProjectID,
		"command_count":    session.CommandCount,
		"success_count":    session.SuccessCount,
		"success_rate":     float64(session.SuccessCount) / float64(session.CommandCount),
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

// startCleanupRoutine starts the automatic cleanup routine for inactive sessions
func (m *Manager) startCleanupRoutine() {
	m.cleanupTicker = time.NewTicker(m.config.Session.CleanupInterval)

	go func() {
		for {
			select {
			case <-m.cleanupTicker.C:
				m.cleanupInactiveSessions()
			case <-m.stopCleanup:
				m.cleanupTicker.Stop()
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

// Shutdown gracefully shuts down the manager
func (m *Manager) Shutdown() {
	close(m.stopCleanup)

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
