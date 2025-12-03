package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// F2: SessionSnapshot represents a saved session state
type SessionSnapshot struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	SessionID    string            `json:"session_id"`
	ProjectID    string            `json:"project_id"`
	WorkingDir   string            `json:"working_dir"`
	CurrentDir   string            `json:"current_dir"`
	Environment  map[string]string `json:"environment"`
	CommandCount int               `json:"command_count"`
	CreatedAt    time.Time         `json:"created_at"`
	Description  string            `json:"description,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
}

// F2: SnapshotManager manages session snapshots
type SnapshotManager struct {
	snapshots   map[string]*SessionSnapshot
	snapshotDir string
	mu          sync.RWMutex
}

// NewSnapshotManager creates a new snapshot manager
func NewSnapshotManager(dataDir string) *SnapshotManager {
	snapshotDir := filepath.Join(dataDir, "snapshots")
	os.MkdirAll(snapshotDir, 0o755)

	sm := &SnapshotManager{
		snapshots:   make(map[string]*SessionSnapshot),
		snapshotDir: snapshotDir,
	}

	// Load existing snapshots
	sm.loadSnapshots()

	return sm
}

// loadSnapshots loads all snapshots from disk
func (sm *SnapshotManager) loadSnapshots() {
	files, err := os.ReadDir(sm.snapshotDir)
	if err != nil {
		return
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(sm.snapshotDir, file.Name()))
		if err != nil {
			continue
		}

		var snapshot SessionSnapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			continue
		}

		sm.snapshots[snapshot.ID] = &snapshot
	}
}

// CreateSnapshot creates a new session snapshot
func (sm *SnapshotManager) CreateSnapshot(snapshot *SessionSnapshot) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	snapshot.CreatedAt = time.Now()
	sm.snapshots[snapshot.ID] = snapshot

	// Save to disk
	return sm.saveSnapshot(snapshot)
}

// saveSnapshot saves a snapshot to disk
func (sm *SnapshotManager) saveSnapshot(snapshot *SessionSnapshot) error {
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}

	filename := filepath.Join(sm.snapshotDir, snapshot.ID+".json")
	return os.WriteFile(filename, data, 0o644)
}

// GetSnapshot retrieves a snapshot by ID or name
func (sm *SnapshotManager) GetSnapshot(idOrName string) (*SessionSnapshot, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// Try by ID first
	if snapshot, exists := sm.snapshots[idOrName]; exists {
		return snapshot, true
	}

	// Try by name
	for _, snapshot := range sm.snapshots {
		if snapshot.Name == idOrName {
			return snapshot, true
		}
	}

	return nil, false
}

// ListSnapshots returns all snapshots
func (sm *SnapshotManager) ListSnapshots() []*SessionSnapshot {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make([]*SessionSnapshot, 0, len(sm.snapshots))
	for _, s := range sm.snapshots {
		result = append(result, s)
	}
	return result
}

// DeleteSnapshot removes a snapshot
func (sm *SnapshotManager) DeleteSnapshot(id string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.snapshots[id]; !exists {
		return fmt.Errorf("snapshot not found: %s", id)
	}

	delete(sm.snapshots, id)

	// Remove from disk
	filename := filepath.Join(sm.snapshotDir, id+".json")
	return os.Remove(filename)
}

// =============================================================================
// F2: Snapshot Tool Handlers
// =============================================================================

// CreateSnapshotArgs represents arguments for creating a snapshot
type CreateSnapshotArgs struct {
	SessionID   string   `json:"session_id" jsonschema:"required,description=Session ID to snapshot"`
	Name        string   `json:"name" jsonschema:"required,description=Name for the snapshot"`
	Description string   `json:"description,omitempty" jsonschema:"description=Description of the snapshot"`
	Tags        []string `json:"tags,omitempty" jsonschema:"description=Tags for categorizing the snapshot"`
}

// CreateSnapshotResult represents the result of creating a snapshot
type CreateSnapshotResult struct {
	SnapshotID string    `json:"snapshot_id"`
	Name       string    `json:"name"`
	SessionID  string    `json:"session_id"`
	CreatedAt  time.Time `json:"created_at"`
	Message    string    `json:"message"`
}

// ListSnapshotsArgs represents arguments for listing snapshots
type ListSnapshotsArgs struct{}

// ListSnapshotsResult represents the result of listing snapshots
type ListSnapshotsResult struct {
	Snapshots []*SessionSnapshot `json:"snapshots"`
	Count     int                `json:"count"`
}

// RestoreSnapshotArgs represents arguments for restoring a snapshot
type RestoreSnapshotArgs struct {
	SnapshotID string `json:"snapshot_id" jsonschema:"required,description=Snapshot ID or name to restore"`
	NewName    string `json:"new_name,omitempty" jsonschema:"description=Name for the restored session (optional)"`
}

// RestoreSnapshotResult represents the result of restoring a snapshot
type RestoreSnapshotResult struct {
	NewSessionID string `json:"new_session_id"`
	SnapshotID   string `json:"snapshot_id"`
	RestoredName string `json:"restored_name"`
	WorkingDir   string `json:"working_dir"`
	Message      string `json:"message"`
}

// CreateSessionSnapshot creates a snapshot of the current session state
func (t *TerminalTools) CreateSessionSnapshot(ctx context.Context, req *mcp.CallToolRequest, args CreateSnapshotArgs) (*mcp.CallToolResult, CreateSnapshotResult, error) {
	// Get the session
	session, err := t.manager.GetSession(args.SessionID)
	if err != nil {
		return createErrorResult(fmt.Sprintf("Session not found: %v", err)), CreateSnapshotResult{}, nil
	}

	// Create snapshot
	snapshot := &SessionSnapshot{
		ID:           fmt.Sprintf("snap-%s", time.Now().Format("20060102-150405")),
		Name:         args.Name,
		SessionID:    session.ID,
		ProjectID:    session.ProjectID,
		WorkingDir:   session.WorkingDir,
		CurrentDir:   session.GetCurrentDir(),
		Environment:  session.Environment,
		CommandCount: session.CommandCount,
		Description:  args.Description,
		Tags:         args.Tags,
	}

	if err := t.snapshotManager.CreateSnapshot(snapshot); err != nil {
		return createErrorResult(fmt.Sprintf("Failed to create snapshot: %v", err)), CreateSnapshotResult{}, nil
	}

	result := CreateSnapshotResult{
		SnapshotID: snapshot.ID,
		Name:       snapshot.Name,
		SessionID:  snapshot.SessionID,
		CreatedAt:  snapshot.CreatedAt,
		Message:    fmt.Sprintf("Snapshot '%s' created successfully", snapshot.Name),
	}

	t.logger.Info("Session snapshot created", map[string]interface{}{
		"snapshot_id": snapshot.ID,
		"session_id":  args.SessionID,
		"name":        args.Name,
	})

	return createJSONResult(result), result, nil
}

// ListSessionSnapshots lists all available snapshots
func (t *TerminalTools) ListSessionSnapshots(ctx context.Context, req *mcp.CallToolRequest, args ListSnapshotsArgs) (*mcp.CallToolResult, ListSnapshotsResult, error) {
	snapshots := t.snapshotManager.ListSnapshots()

	result := ListSnapshotsResult{
		Snapshots: snapshots,
		Count:     len(snapshots),
	}

	return createJSONResult(result), result, nil
}

// RestoreSessionSnapshot restores a session from a snapshot
func (t *TerminalTools) RestoreSessionSnapshot(ctx context.Context, req *mcp.CallToolRequest, args RestoreSnapshotArgs) (*mcp.CallToolResult, RestoreSnapshotResult, error) {
	// Get the snapshot
	snapshot, exists := t.snapshotManager.GetSnapshot(args.SnapshotID)
	if !exists {
		return createErrorResult(fmt.Sprintf("Snapshot not found: %s", args.SnapshotID)), RestoreSnapshotResult{}, nil
	}

	// Determine session name
	sessionName := args.NewName
	if sessionName == "" {
		sessionName = fmt.Sprintf("%s-restored", snapshot.Name)
	}

	// Create new session from snapshot
	session, err := t.manager.CreateSession(sessionName, snapshot.ProjectID, snapshot.WorkingDir)
	if err != nil {
		return createErrorResult(fmt.Sprintf("Failed to create session: %v", err)), RestoreSnapshotResult{}, nil
	}

	// Restore environment variables
	for key, value := range snapshot.Environment {
		session.Environment[key] = value
	}

	// Change to the saved current directory
	if snapshot.CurrentDir != "" && snapshot.CurrentDir != snapshot.WorkingDir {
		_, _ = t.manager.ExecuteCommandWithTimeout(session.ID, fmt.Sprintf("cd %s", shellEscape(snapshot.CurrentDir)), 5*time.Second)
	}

	result := RestoreSnapshotResult{
		NewSessionID: session.ID,
		SnapshotID:   snapshot.ID,
		RestoredName: sessionName,
		WorkingDir:   snapshot.CurrentDir,
		Message:      fmt.Sprintf("Session restored from snapshot '%s'", snapshot.Name),
	}

	t.logger.Info("Session restored from snapshot", map[string]interface{}{
		"snapshot_id":    snapshot.ID,
		"new_session_id": session.ID,
		"name":           sessionName,
	})

	return createJSONResult(result), result, nil
}

// shellEscape escapes a string for safe use in shell (duplicated for package scope)
func shellEscape(s string) string {
	if s == "" {
		return "''"
	}
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
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
