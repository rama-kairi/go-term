package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	termerr "github.com/rama-kairi/go-term/internal/errors"
)

// CommandEntry represents a single command execution in history
type CommandEntry struct {
	ID          string            `json:"id"`
	SessionID   string            `json:"session_id"`
	ProjectID   string            `json:"project_id"`
	Command     string            `json:"command"`
	Output      string            `json:"output"`
	ErrorOutput string            `json:"error_output,omitempty"`
	ExitCode    int               `json:"exit_code"`
	Success     bool              `json:"success"`
	StartTime   time.Time         `json:"start_time"`
	EndTime     time.Time         `json:"end_time"`
	Duration    time.Duration     `json:"duration"`
	WorkingDir  string            `json:"working_dir"`
	Environment map[string]string `json:"environment,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
}

// SessionHistory contains all command history for a session
type SessionHistory struct {
	SessionID   string         `json:"session_id"`
	ProjectID   string         `json:"project_id"`
	SessionName string         `json:"session_name"`
	CreatedAt   time.Time      `json:"created_at"`
	Commands    []CommandEntry `json:"commands"`
	mutex       sync.RWMutex
}

// HistoryManager manages command history across all sessions
type HistoryManager struct {
	sessions    map[string]*SessionHistory
	projectsDir string
	mutex       sync.RWMutex
}

// SearchOptions defines parameters for searching command history
type SearchOptions struct {
	SessionID     string    `json:"session_id,omitempty"`
	ProjectID     string    `json:"project_id,omitempty"`
	Command       string    `json:"command,omitempty"`        // Partial command match
	Output        string    `json:"output,omitempty"`         // Partial output match
	Success       *bool     `json:"success,omitempty"`        // Filter by success status
	StartTime     time.Time `json:"start_time,omitempty"`     // Commands after this time
	EndTime       time.Time `json:"end_time,omitempty"`       // Commands before this time
	WorkingDir    string    `json:"working_dir,omitempty"`    // Filter by working directory
	Tags          []string  `json:"tags,omitempty"`           // Commands with all these tags
	Limit         int       `json:"limit,omitempty"`          // Max results (default 100)
	SortBy        string    `json:"sort_by,omitempty"`        // "time", "duration", "command" (default "time")
	SortDesc      bool      `json:"sort_desc,omitempty"`      // Sort descending (default true)
	IncludeOutput bool      `json:"include_output,omitempty"` // Include command output in results
}

// SearchResult contains the results of a history search
type SearchResult struct {
	TotalFound int            `json:"total_found"`
	Results    []CommandEntry `json:"results"`
	Query      SearchOptions  `json:"query"`
	SearchTime time.Duration  `json:"search_time"`
}

// NewHistoryManager creates a new history manager
func NewHistoryManager(dataDir string) *HistoryManager {
	if dataDir == "" {
		dataDir = ".github.com/rama-kairi/go-term"
	}

	projectsDir := filepath.Join(dataDir, "history")
	os.MkdirAll(projectsDir, 0o755)

	hm := &HistoryManager{
		sessions:    make(map[string]*SessionHistory),
		projectsDir: projectsDir,
	}

	// Load existing history
	hm.loadExistingHistory()

	return hm
}

// CreateSessionHistory creates a new session history
func (hm *HistoryManager) CreateSessionHistory(sessionID, projectID, sessionName string) error {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	if _, exists := hm.sessions[sessionID]; exists {
		return termerr.SessionExists(sessionID)
	}

	history := &SessionHistory{
		SessionID:   sessionID,
		ProjectID:   projectID,
		SessionName: sessionName,
		CreatedAt:   time.Now(),
		Commands:    make([]CommandEntry, 0),
	}

	hm.sessions[sessionID] = history
	return nil
}

// AddCommand adds a command execution to session history
func (hm *HistoryManager) AddCommand(entry CommandEntry) error {
	hm.mutex.RLock()
	session, exists := hm.sessions[entry.SessionID]
	hm.mutex.RUnlock()

	if !exists {
		return termerr.HistoryNotFound(entry.SessionID)
	}

	session.mutex.Lock()
	defer session.mutex.Unlock()

	// Generate unique ID if not provided
	if entry.ID == "" {
		entry.ID = fmt.Sprintf("%s_%d", entry.SessionID[:8], len(session.Commands)+1)
	}

	// Set project ID from session if not provided
	if entry.ProjectID == "" {
		entry.ProjectID = session.ProjectID
	}

	session.Commands = append(session.Commands, entry)

	// Save to disk asynchronously
	go hm.saveSessionHistory(session)

	return nil
}

// SearchHistory searches through command history with the given options
func (hm *HistoryManager) SearchHistory(options SearchOptions) (*SearchResult, error) {
	startTime := time.Now()

	hm.mutex.RLock()
	defer hm.mutex.RUnlock()

	var allResults []CommandEntry

	// Search through all sessions
	for _, session := range hm.sessions {
		session.mutex.RLock()

		// Filter by session ID if specified
		if options.SessionID != "" && session.SessionID != options.SessionID {
			session.mutex.RUnlock()
			continue
		}

		// Filter by project ID if specified
		if options.ProjectID != "" && session.ProjectID != options.ProjectID {
			session.mutex.RUnlock()
			continue
		}

		// Search commands in this session
		for _, cmd := range session.Commands {
			if hm.matchesSearchCriteria(cmd, options) {
				// Create a copy to avoid concurrent access issues
				cmdCopy := cmd

				// Optionally exclude output to reduce response size
				if !options.IncludeOutput {
					cmdCopy.Output = ""
					cmdCopy.ErrorOutput = ""
				}

				allResults = append(allResults, cmdCopy)
			}
		}

		session.mutex.RUnlock()
	}

	// Sort results
	hm.sortResults(allResults, options)

	// Apply limit
	limit := options.Limit
	if limit <= 0 {
		limit = 100
	}

	if len(allResults) > limit {
		allResults = allResults[:limit]
	}

	return &SearchResult{
		TotalFound: len(allResults),
		Results:    allResults,
		Query:      options,
		SearchTime: time.Since(startTime),
	}, nil
}

// GetSessionHistory returns the complete history for a session
func (hm *HistoryManager) GetSessionHistory(sessionID string) (*SessionHistory, error) {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()

	session, exists := hm.sessions[sessionID]
	if !exists {
		return nil, termerr.HistoryNotFound(sessionID)
	}

	session.mutex.RLock()
	defer session.mutex.RUnlock()

	// Create a deep copy
	historyCopy := &SessionHistory{
		SessionID:   session.SessionID,
		ProjectID:   session.ProjectID,
		SessionName: session.SessionName,
		CreatedAt:   session.CreatedAt,
		Commands:    make([]CommandEntry, len(session.Commands)),
	}

	copy(historyCopy.Commands, session.Commands)
	return historyCopy, nil
}

// ListProjects returns all unique project IDs with statistics
func (hm *HistoryManager) ListProjects() map[string]ProjectStats {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()

	projects := make(map[string]ProjectStats)

	for _, session := range hm.sessions {
		session.mutex.RLock()

		stats, exists := projects[session.ProjectID]
		if !exists {
			stats = ProjectStats{
				ProjectID: session.ProjectID,
				Sessions:  make([]string, 0),
			}
		}

		stats.Sessions = append(stats.Sessions, session.SessionID)
		stats.TotalCommands += len(session.Commands)

		if session.CreatedAt.Before(stats.FirstSession) || stats.FirstSession.IsZero() {
			stats.FirstSession = session.CreatedAt
		}

		if session.CreatedAt.After(stats.LastSession) {
			stats.LastSession = session.CreatedAt
		}

		projects[session.ProjectID] = stats
		session.mutex.RUnlock()
	}

	return projects
}

// ProjectStats contains statistics for a project
type ProjectStats struct {
	ProjectID     string    `json:"project_id"`
	Sessions      []string  `json:"sessions"`
	TotalCommands int       `json:"total_commands"`
	FirstSession  time.Time `json:"first_session"`
	LastSession   time.Time `json:"last_session"`
}

// DeleteSessionHistory removes session history
func (hm *HistoryManager) DeleteSessionHistory(sessionID string) error {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	session, exists := hm.sessions[sessionID]
	if !exists {
		return termerr.HistoryNotFound(sessionID)
	}

	// Delete from memory
	delete(hm.sessions, sessionID)

	// Delete from disk
	filename := hm.getSessionHistoryFile(session.ProjectID, sessionID)
	if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
		return termerr.FileSystemError(err, filename).
			WithDetails("failed to delete session history file")
	}

	return nil
}

// matchesSearchCriteria checks if a command entry matches the search criteria
func (hm *HistoryManager) matchesSearchCriteria(cmd CommandEntry, options SearchOptions) bool {
	// Command text match
	if options.Command != "" {
		if !strings.Contains(strings.ToLower(cmd.Command), strings.ToLower(options.Command)) {
			return false
		}
	}

	// Output match
	if options.Output != "" {
		outputText := cmd.Output + " " + cmd.ErrorOutput
		if !strings.Contains(strings.ToLower(outputText), strings.ToLower(options.Output)) {
			return false
		}
	}

	// Success status filter
	if options.Success != nil && cmd.Success != *options.Success {
		return false
	}

	// Time range filter
	if !options.StartTime.IsZero() && cmd.StartTime.Before(options.StartTime) {
		return false
	}

	if !options.EndTime.IsZero() && cmd.StartTime.After(options.EndTime) {
		return false
	}

	// Working directory filter
	if options.WorkingDir != "" {
		if !strings.Contains(cmd.WorkingDir, options.WorkingDir) {
			return false
		}
	}

	// Tags filter (command must have all specified tags)
	if len(options.Tags) > 0 {
		for _, requiredTag := range options.Tags {
			found := false
			for _, cmdTag := range cmd.Tags {
				if cmdTag == requiredTag {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}

	return true
}

// sortResults sorts the search results based on the specified criteria
func (hm *HistoryManager) sortResults(results []CommandEntry, options SearchOptions) {
	sortBy := options.SortBy
	if sortBy == "" {
		sortBy = "time"
	}

	sort.Slice(results, func(i, j int) bool {
		var less bool

		switch sortBy {
		case "duration":
			less = results[i].Duration < results[j].Duration
		case "command":
			less = results[i].Command < results[j].Command
		case "time":
			fallthrough
		default:
			less = results[i].StartTime.Before(results[j].StartTime)
		}

		if options.SortDesc {
			return !less
		}
		return less
	})
}

// saveSessionHistory saves session history to disk
func (hm *HistoryManager) saveSessionHistory(session *SessionHistory) error {
	session.mutex.RLock()
	defer session.mutex.RUnlock()

	// Create project directory if it doesn't exist
	projectDir := filepath.Join(hm.projectsDir, session.ProjectID)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		return termerr.FileSystemError(err, projectDir).
			WithDetails("failed to create project directory")
	}

	filename := hm.getSessionHistoryFile(session.ProjectID, session.SessionID)

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return termerr.Wrap(err, termerr.ErrCodeHistoryCorrupted, "failed to marshal session history").
			WithContext("session_id", session.SessionID)
	}

	if err := os.WriteFile(filename, data, 0o644); err != nil {
		return termerr.HistoryWriteFailed(err, session.SessionID)
	}

	return nil
}

// loadExistingHistory loads all existing history from disk
func (hm *HistoryManager) loadExistingHistory() {
	// Walk through the history directory
	filepath.Walk(hm.projectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if !info.IsDir() && strings.HasSuffix(path, ".json") {
			hm.loadSessionHistoryFile(path)
		}

		return nil
	})
}

// loadSessionHistoryFile loads a single session history file
func (hm *HistoryManager) loadSessionHistoryFile(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return termerr.HistoryReadFailed(err, filename)
	}

	var session SessionHistory
	if err := json.Unmarshal(data, &session); err != nil {
		return termerr.Wrap(err, termerr.ErrCodeHistoryCorrupted, "failed to unmarshal session history").
			WithContext("file", filename).
			WithSuggestion("The history file may be corrupted. Consider deleting it.")
	}

	hm.sessions[session.SessionID] = &session
	return nil
}

// getSessionHistoryFile returns the filename for a session history file
func (hm *HistoryManager) getSessionHistoryFile(projectID, sessionID string) string {
	return filepath.Join(hm.projectsDir, projectID, fmt.Sprintf("%s.json", sessionID))
}

// GetStats returns overall statistics about the history
func (hm *HistoryManager) GetStats() HistoryStats {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()

	stats := HistoryStats{
		TotalSessions: len(hm.sessions),
		Projects:      make(map[string]int),
	}

	for _, session := range hm.sessions {
		session.mutex.RLock()
		stats.TotalCommands += len(session.Commands)
		stats.Projects[session.ProjectID]++
		session.mutex.RUnlock()
	}

	stats.TotalProjects = len(stats.Projects)
	return stats
}

// HistoryStats contains overall statistics about command history
type HistoryStats struct {
	TotalSessions int            `json:"total_sessions"`
	TotalCommands int            `json:"total_commands"`
	TotalProjects int            `json:"total_projects"`
	Projects      map[string]int `json:"projects"` // project_id -> session_count
}
