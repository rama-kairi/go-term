package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DB represents the SQLite database connection and operations
type DB struct {
	conn *sql.DB
	path string
}

// SessionRecord represents a session stored in the database
type SessionRecord struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	ProjectID    string    `json:"project_id"`
	WorkingDir   string    `json:"working_dir"`
	Environment  string    `json:"environment"` // JSON-encoded map[string]string
	CreatedAt    time.Time `json:"created_at"`
	LastUsedAt   time.Time `json:"last_used_at"`
	IsActive     bool      `json:"is_active"`
	CommandCount int       `json:"command_count"`
}

// CommandRecord represents a command execution record
type CommandRecord struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"session_id"`
	ProjectID   string    `json:"project_id"`
	Command     string    `json:"command"`
	Output      string    `json:"output"`
	ErrorOutput string    `json:"error_output"`
	Success     bool      `json:"success"`
	ExitCode    int       `json:"exit_code"`
	Duration    int64     `json:"duration_ms"` // Duration in milliseconds
	WorkingDir  string    `json:"working_dir"`
	Timestamp   time.Time `json:"timestamp"`
	Tags        string    `json:"tags"` // JSON-encoded []string
}

// StreamChunk represents a real-time output chunk
type StreamChunk struct {
	SessionID   string    `json:"session_id"`
	CommandID   string    `json:"command_id"`
	ChunkType   string    `json:"chunk_type"` // "stdout", "stderr", "status"
	Content     string    `json:"content"`
	Timestamp   time.Time `json:"timestamp"`
	SequenceNum int       `json:"sequence_num"`
}

// NewDB creates a new database connection
func NewDB(dataDir string) (*DB, error) {
	// Ensure the data directory exists
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	dbPath := filepath.Join(dataDir, "go-term.db")

	conn, err := sql.Open("sqlite3", dbPath+"?_journal=WAL&_timeout=5000&_fk=1")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	conn.SetMaxOpenConns(10)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxLifetime(time.Hour)

	db := &DB{
		conn: conn,
		path: dbPath,
	}

	if err := db.initialize(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	return db, nil
}

// initialize creates the database schema
func (db *DB) initialize() error {
	schema := `
	-- Sessions table
	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		project_id TEXT NOT NULL,
		working_dir TEXT NOT NULL,
		environment TEXT DEFAULT '{}',
		created_at DATETIME NOT NULL,
		last_used_at DATETIME NOT NULL,
		is_active BOOLEAN DEFAULT 1,
		command_count INTEGER DEFAULT 0
	);

	-- Commands table
	CREATE TABLE IF NOT EXISTS commands (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		project_id TEXT NOT NULL,
		command TEXT NOT NULL,
		output TEXT DEFAULT '',
		error_output TEXT DEFAULT '',
		success BOOLEAN NOT NULL,
		exit_code INTEGER NOT NULL,
		duration_ms INTEGER NOT NULL,
		working_dir TEXT NOT NULL,
		timestamp DATETIME NOT NULL,
		tags TEXT DEFAULT '[]',
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	);

	-- Stream chunks table (for real-time streaming)
	CREATE TABLE IF NOT EXISTS stream_chunks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		command_id TEXT NOT NULL,
		chunk_type TEXT NOT NULL,
		content TEXT NOT NULL,
		timestamp DATETIME NOT NULL,
		sequence_num INTEGER NOT NULL,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
		FOREIGN KEY (command_id) REFERENCES commands(id) ON DELETE CASCADE
	);

	-- Indexes for better performance
	CREATE INDEX IF NOT EXISTS idx_sessions_project_id ON sessions(project_id);
	CREATE INDEX IF NOT EXISTS idx_sessions_last_used ON sessions(last_used_at);
	CREATE INDEX IF NOT EXISTS idx_commands_session_id ON commands(session_id);
	CREATE INDEX IF NOT EXISTS idx_commands_project_id ON commands(project_id);
	CREATE INDEX IF NOT EXISTS idx_commands_timestamp ON commands(timestamp);
	CREATE INDEX IF NOT EXISTS idx_stream_chunks_command_id ON stream_chunks(command_id);
	CREATE INDEX IF NOT EXISTS idx_stream_chunks_session_id ON stream_chunks(session_id);
	`

	_, err := db.conn.Exec(schema)
	return err
}

// Close closes the database connection
func (db *DB) Close() error {
	if db.conn != nil {
		return db.conn.Close()
	}
	return nil
}

// Session operations

// CreateSession creates a new session record
func (db *DB) CreateSession(session *SessionRecord) error {
	envJSON, err := json.Marshal(map[string]string{})
	if err != nil {
		return fmt.Errorf("failed to marshal environment: %w", err)
	}

	query := `
	INSERT INTO sessions (id, name, project_id, working_dir, environment, created_at, last_used_at, is_active, command_count)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = db.conn.Exec(query, session.ID, session.Name, session.ProjectID, session.WorkingDir,
		string(envJSON), session.CreatedAt, session.LastUsedAt, session.IsActive, session.CommandCount)

	return err
}

// GetSession retrieves a session by ID
func (db *DB) GetSession(sessionID string) (*SessionRecord, error) {
	query := `
	SELECT id, name, project_id, working_dir, environment, created_at, last_used_at, is_active, command_count
	FROM sessions WHERE id = ?
	`

	row := db.conn.QueryRow(query, sessionID)

	var session SessionRecord
	var envJSON string

	err := row.Scan(&session.ID, &session.Name, &session.ProjectID, &session.WorkingDir,
		&envJSON, &session.CreatedAt, &session.LastUsedAt, &session.IsActive, &session.CommandCount)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("session not found: %s", sessionID)
		}
		return nil, err
	}

	session.Environment = envJSON
	return &session, nil
}

// ListSessions retrieves all sessions, optionally filtered by project
func (db *DB) ListSessions(projectID string) ([]*SessionRecord, error) {
	var query string
	var args []interface{}

	if projectID != "" {
		query = `
		SELECT id, name, project_id, working_dir, environment, created_at, last_used_at, is_active, command_count
		FROM sessions WHERE project_id = ? ORDER BY last_used_at DESC
		`
		args = []interface{}{projectID}
	} else {
		query = `
		SELECT id, name, project_id, working_dir, environment, created_at, last_used_at, is_active, command_count
		FROM sessions ORDER BY last_used_at DESC
		`
	}

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*SessionRecord

	for rows.Next() {
		var session SessionRecord
		var envJSON string

		err := rows.Scan(&session.ID, &session.Name, &session.ProjectID, &session.WorkingDir,
			&envJSON, &session.CreatedAt, &session.LastUsedAt, &session.IsActive, &session.CommandCount)
		if err != nil {
			return nil, err
		}

		session.Environment = envJSON
		sessions = append(sessions, &session)
	}

	return sessions, rows.Err()
}

// UpdateSession updates session information
func (db *DB) UpdateSession(session *SessionRecord) error {
	query := `
	UPDATE sessions
	SET name = ?, working_dir = ?, environment = ?, last_used_at = ?, is_active = ?, command_count = ?
	WHERE id = ?
	`

	_, err := db.conn.Exec(query, session.Name, session.WorkingDir, session.Environment,
		session.LastUsedAt, session.IsActive, session.CommandCount, session.ID)

	return err
}

// DeleteSession deletes a session and all related data
func (db *DB) DeleteSession(sessionID string) error {
	// SQLite with foreign keys will cascade delete commands and stream_chunks
	query := `DELETE FROM sessions WHERE id = ?`
	result, err := db.conn.Exec(query, sessionID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	return nil
}

// DeleteProjectSessions deletes all sessions for a project
func (db *DB) DeleteProjectSessions(projectID string) (int64, error) {
	query := `DELETE FROM sessions WHERE project_id = ?`
	result, err := db.conn.Exec(query, projectID)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

// Command operations

// CreateCommand creates a new command record
func (db *DB) CreateCommand(cmd *CommandRecord) error {
	tagsJSON, err := json.Marshal([]string{})
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}

	query := `
	INSERT INTO commands (id, session_id, project_id, command, output, error_output, success, exit_code, duration_ms, working_dir, timestamp, tags)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = db.conn.Exec(query, cmd.ID, cmd.SessionID, cmd.ProjectID, cmd.Command, cmd.Output,
		cmd.ErrorOutput, cmd.Success, cmd.ExitCode, cmd.Duration, cmd.WorkingDir, cmd.Timestamp, string(tagsJSON))

	return err
}

// StoreCommand stores a command execution record
func (db *DB) StoreCommand(sessionID, projectID, command, output string, exitCode int, success bool, startTime, endTime time.Time, duration time.Duration, workingDir string) error {
	cmd := &CommandRecord{
		ID:         fmt.Sprintf("%s_%d", sessionID, time.Now().UnixNano()),
		SessionID:  sessionID,
		ProjectID:  projectID,
		Command:    command,
		Output:     output,
		Success:    success,
		ExitCode:   exitCode,
		Duration:   duration.Milliseconds(),
		WorkingDir: workingDir,
		Timestamp:  startTime,
	}

	return db.CreateCommand(cmd)
}

// SearchCommands searches command history with various filters
func (db *DB) SearchCommands(sessionID, projectID, command, output string, success *bool, startTime, endTime time.Time, limit int) ([]*CommandRecord, error) {
	query := `
	SELECT id, session_id, project_id, command, output, error_output, success, exit_code, duration_ms, working_dir, timestamp, tags
	FROM commands WHERE 1=1
	`

	var args []interface{}

	if sessionID != "" {
		query += " AND session_id = ?"
		args = append(args, sessionID)
	}

	if projectID != "" {
		query += " AND project_id = ?"
		args = append(args, projectID)
	}

	if command != "" {
		query += " AND command LIKE ?"
		args = append(args, "%"+command+"%")
	}

	if output != "" {
		query += " AND (output LIKE ? OR error_output LIKE ?)"
		args = append(args, "%"+output+"%", "%"+output+"%")
	}

	if success != nil {
		query += " AND success = ?"
		args = append(args, *success)
	}

	if !startTime.IsZero() {
		query += " AND timestamp >= ?"
		args = append(args, startTime)
	}

	if !endTime.IsZero() {
		query += " AND timestamp <= ?"
		args = append(args, endTime)
	}

	query += " ORDER BY timestamp DESC"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var commands []*CommandRecord

	for rows.Next() {
		var cmd CommandRecord
		var tagsJSON string

		err := rows.Scan(&cmd.ID, &cmd.SessionID, &cmd.ProjectID, &cmd.Command, &cmd.Output,
			&cmd.ErrorOutput, &cmd.Success, &cmd.ExitCode, &cmd.Duration, &cmd.WorkingDir, &cmd.Timestamp, &tagsJSON)
		if err != nil {
			return nil, err
		}

		cmd.Tags = tagsJSON
		commands = append(commands, &cmd)
	}

	return commands, rows.Err()
}

// Stream operations

// CreateStreamChunk stores a real-time stream chunk
func (db *DB) CreateStreamChunk(chunk *StreamChunk) error {
	query := `
	INSERT INTO stream_chunks (session_id, command_id, chunk_type, content, timestamp, sequence_num)
	VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := db.conn.Exec(query, chunk.SessionID, chunk.CommandID, chunk.ChunkType,
		chunk.Content, chunk.Timestamp, chunk.SequenceNum)

	return err
}

// GetStreamChunks retrieves stream chunks for a command
func (db *DB) GetStreamChunks(commandID string) ([]*StreamChunk, error) {
	query := `
	SELECT session_id, command_id, chunk_type, content, timestamp, sequence_num
	FROM stream_chunks WHERE command_id = ? ORDER BY sequence_num
	`

	rows, err := db.conn.Query(query, commandID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks []*StreamChunk

	for rows.Next() {
		var chunk StreamChunk

		err := rows.Scan(&chunk.SessionID, &chunk.CommandID, &chunk.ChunkType,
			&chunk.Content, &chunk.Timestamp, &chunk.SequenceNum)
		if err != nil {
			return nil, err
		}

		chunks = append(chunks, &chunk)
	}

	return chunks, rows.Err()
}

// Utility methods

// GetSessionStats returns statistics for a session
func (db *DB) GetSessionStats(sessionID string) (map[string]interface{}, error) {
	query := `
	SELECT
		COUNT(*) as total_commands,
		SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END) as successful_commands,
		AVG(duration_ms) as avg_duration_ms,
		MAX(timestamp) as last_command_time
	FROM commands WHERE session_id = ?
	`

	row := db.conn.QueryRow(query, sessionID)

	var totalCommands, successfulCommands int
	var avgDuration float64
	var lastCommandTime time.Time

	err := row.Scan(&totalCommands, &successfulCommands, &avgDuration, &lastCommandTime)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"total_commands":      totalCommands,
		"successful_commands": successfulCommands,
		"failed_commands":     totalCommands - successfulCommands,
		"avg_duration_ms":     avgDuration,
		"last_command_time":   lastCommandTime,
	}, nil
}

// GetProjectStats returns statistics for a project
func (db *DB) GetProjectStats(projectID string) (map[string]interface{}, error) {
	query := `
	SELECT
		COUNT(DISTINCT s.id) as total_sessions,
		COUNT(c.id) as total_commands,
		SUM(CASE WHEN c.success = 1 THEN 1 ELSE 0 END) as successful_commands,
		AVG(c.duration_ms) as avg_duration_ms
	FROM sessions s
	LEFT JOIN commands c ON s.id = c.session_id
	WHERE s.project_id = ?
	`

	row := db.conn.QueryRow(query, projectID)

	var totalSessions, totalCommands, successfulCommands int
	var avgDuration float64

	err := row.Scan(&totalSessions, &totalCommands, &successfulCommands, &avgDuration)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"total_sessions":      totalSessions,
		"total_commands":      totalCommands,
		"successful_commands": successfulCommands,
		"failed_commands":     totalCommands - successfulCommands,
		"avg_duration_ms":     avgDuration,
	}, nil
}
