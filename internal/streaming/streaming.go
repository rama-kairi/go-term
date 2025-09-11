package streaming

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// StreamType represents the type of stream output
type StreamType string

const (
	StreamTypeStdout StreamType = "stdout"
	StreamTypeStderr StreamType = "stderr"
	StreamTypeStatus StreamType = "status"
)

// StreamChunk represents a piece of streaming output
type StreamChunk struct {
	Type        StreamType `json:"type"`
	Content     string     `json:"content"`
	Timestamp   time.Time  `json:"timestamp"`
	SequenceNum int        `json:"sequence_num"`
}

// StreamResult represents the final result of a streamed command
type StreamResult struct {
	Success     bool          `json:"success"`
	ExitCode    int           `json:"exit_code"`
	Duration    time.Duration `json:"duration"`
	TotalChunks int           `json:"total_chunks"`
}

// CommandStreamer handles real-time streaming of command execution
type CommandStreamer struct {
	sessionID   string
	commandID   string
	workingDir  string
	chunks      chan StreamChunk
	result      chan StreamResult
	sequenceNum int
	mu          sync.Mutex
}

// NewCommandStreamer creates a new command streamer
func NewCommandStreamer(sessionID, commandID, workingDir string) *CommandStreamer {
	return &CommandStreamer{
		sessionID:  sessionID,
		commandID:  commandID,
		workingDir: workingDir,
		chunks:     make(chan StreamChunk, 100), // Buffered channel for chunks
		result:     make(chan StreamResult, 1),  // Single result
	}
}

// GetChunks returns the channel for receiving stream chunks
func (cs *CommandStreamer) GetChunks() <-chan StreamChunk {
	return cs.chunks
}

// GetResult returns the channel for receiving the final result
func (cs *CommandStreamer) GetResult() <-chan StreamResult {
	return cs.result
}

// nextSequenceNum returns the next sequence number in a thread-safe way
func (cs *CommandStreamer) nextSequenceNum() int {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.sequenceNum++
	return cs.sequenceNum
}

// sendChunk sends a stream chunk with automatic sequence numbering
func (cs *CommandStreamer) sendChunk(streamType StreamType, content string) {
	chunk := StreamChunk{
		Type:        streamType,
		Content:     content,
		Timestamp:   time.Now(),
		SequenceNum: cs.nextSequenceNum(),
	}

	select {
	case cs.chunks <- chunk:
		// Chunk sent successfully
	default:
		// Channel is full, skip this chunk to avoid blocking
		// In production, you might want to implement backpressure handling
	}
}

// ExecuteCommand executes a command with real-time streaming
func (cs *CommandStreamer) ExecuteCommand(ctx context.Context, command string) error {
	startTime := time.Now()

	// Send start status
	cs.sendChunk(StreamTypeStatus, fmt.Sprintf("Starting command: %s", command))

	// Create the command
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = cs.workingDir

	// Create pipes for stdout and stderr
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		cs.sendChunk(StreamTypeStatus, fmt.Sprintf("Error creating stdout pipe: %v", err))
		cs.sendResult(false, -1, time.Since(startTime))
		return err
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		cs.sendChunk(StreamTypeStatus, fmt.Sprintf("Error creating stderr pipe: %v", err))
		cs.sendResult(false, -1, time.Since(startTime))
		return err
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		cs.sendChunk(StreamTypeStatus, fmt.Sprintf("Error starting command: %v", err))
		cs.sendResult(false, -1, time.Since(startTime))
		return err
	}

	// Use WaitGroup to wait for both stdout and stderr goroutines
	var wg sync.WaitGroup
	wg.Add(2)

	// Stream stdout
	go func() {
		defer wg.Done()
		cs.streamOutput(stdoutPipe, StreamTypeStdout)
	}()

	// Stream stderr
	go func() {
		defer wg.Done()
		cs.streamOutput(stderrPipe, StreamTypeStderr)
	}()

	// Wait for the command to complete
	err = cmd.Wait()

	// Wait for all output to be streamed
	wg.Wait()

	// Get exit code
	exitCode := 0
	success := true

	if err != nil {
		success = false
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = -1
		}
		cs.sendChunk(StreamTypeStatus, fmt.Sprintf("Command failed: %v", err))
	} else {
		cs.sendChunk(StreamTypeStatus, "Command completed successfully")
	}

	duration := time.Since(startTime)
	cs.sendResult(success, exitCode, duration)

	// Close channels
	close(cs.chunks)
	close(cs.result)

	return nil
}

// streamOutput reads from a pipe and sends chunks
func (cs *CommandStreamer) streamOutput(pipe io.ReadCloser, streamType StreamType) {
	defer pipe.Close()

	scanner := bufio.NewScanner(pipe)

	// Increase buffer size for large output lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024) // 1MB max token size

	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			cs.sendChunk(streamType, line+"\n")
		}
	}

	if err := scanner.Err(); err != nil {
		cs.sendChunk(StreamTypeStatus, fmt.Sprintf("Error reading %s: %v", streamType, err))
	}
}

// sendResult sends the final result
func (cs *CommandStreamer) sendResult(success bool, exitCode int, duration time.Duration) {
	result := StreamResult{
		Success:     success,
		ExitCode:    exitCode,
		Duration:    duration,
		TotalChunks: cs.sequenceNum,
	}

	select {
	case cs.result <- result:
		// Result sent successfully
	default:
		// This shouldn't happen as result channel has buffer size 1
	}
}

// StreamCollector collects and aggregates stream chunks
type StreamCollector struct {
	chunks []StreamChunk
	stdout []string
	stderr []string
	status []string
	mu     sync.RWMutex
}

// NewStreamCollector creates a new stream collector
func NewStreamCollector() *StreamCollector {
	return &StreamCollector{
		chunks: make([]StreamChunk, 0),
		stdout: make([]string, 0),
		stderr: make([]string, 0),
		status: make([]string, 0),
	}
}

// AddChunk adds a chunk to the collector
func (sc *StreamCollector) AddChunk(chunk StreamChunk) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.chunks = append(sc.chunks, chunk)

	switch chunk.Type {
	case StreamTypeStdout:
		sc.stdout = append(sc.stdout, chunk.Content)
	case StreamTypeStderr:
		sc.stderr = append(sc.stderr, chunk.Content)
	case StreamTypeStatus:
		sc.status = append(sc.status, chunk.Content)
	}
}

// GetOutput returns the collected output
func (sc *StreamCollector) GetOutput() (stdout, stderr, status string) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	return sc.joinStrings(sc.stdout), sc.joinStrings(sc.stderr), sc.joinStrings(sc.status)
}

// GetChunks returns all collected chunks
func (sc *StreamCollector) GetChunks() []StreamChunk {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	// Return a copy to avoid race conditions
	chunks := make([]StreamChunk, len(sc.chunks))
	copy(chunks, sc.chunks)
	return chunks
}

// joinStrings joins string slices with no separator (strings already include newlines)
func (sc *StreamCollector) joinStrings(strings []string) string {
	if len(strings) == 0 {
		return ""
	}

	var result string
	for _, s := range strings {
		result += s
	}
	return result
}

// StreamingExecutor provides a high-level interface for streaming command execution
type StreamingExecutor struct {
	timeout time.Duration
}

// NewStreamingExecutor creates a new streaming executor
func NewStreamingExecutor(timeout time.Duration) *StreamingExecutor {
	return &StreamingExecutor{
		timeout: timeout,
	}
}

// ExecuteWithStreaming executes a command with real-time streaming and returns both chunks and final output
func (se *StreamingExecutor) ExecuteWithStreaming(sessionID, commandID, workingDir, command string) (*StreamCollector, *StreamResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), se.timeout)
	defer cancel()

	streamer := NewCommandStreamer(sessionID, commandID, workingDir)
	collector := NewStreamCollector()

	// Start collecting chunks in a goroutine
	go func() {
		for chunk := range streamer.GetChunks() {
			collector.AddChunk(chunk)
		}
	}()

	// Execute the command
	err := streamer.ExecuteCommand(ctx, command)

	// Get the final result
	var result *StreamResult
	select {
	case r := <-streamer.GetResult():
		result = &r
	case <-ctx.Done():
		result = &StreamResult{
			Success:  false,
			ExitCode: -1,
			Duration: se.timeout,
		}
		err = fmt.Errorf("command timed out after %v", se.timeout)
	}

	return collector, result, err
}
