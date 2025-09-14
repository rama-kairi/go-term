package streaming

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestNewCommandStreamer tests creation of command streamer
func TestNewCommandStreamer(t *testing.T) {
	sessionID := "test-session"
	commandID := "test-command"
	workingDir := "/tmp"

	streamer := NewCommandStreamer(sessionID, commandID, workingDir)

	if streamer == nil {
		t.Fatalf("Expected non-nil command streamer")
	}

	// Test getting chunks channel
	chunks := streamer.GetChunks()
	if chunks == nil {
		t.Fatalf("Expected non-nil chunks channel")
	}

	// Test getting result channel
	result := streamer.GetResult()
	if result == nil {
		t.Fatalf("Expected non-nil result channel")
	}
}

// TestStreamChunks tests streaming functionality
func TestStreamChunks(t *testing.T) {
	streamer := NewCommandStreamer("test-session", "test-command", "/tmp")

	// Test getting chunks channel
	chunks := streamer.GetChunks()
	if chunks == nil {
		t.Fatalf("Expected non-nil chunks channel")
	}

	// Test getting result channel
	result := streamer.GetResult()
	if result == nil {
		t.Fatalf("Expected non-nil result channel")
	}

	// Send a test chunk
	go func() {
		streamer.sendChunk(StreamTypeStdout, "test output")
		close(streamer.chunks)
	}()

	// Receive the chunk
	select {
	case chunk := <-chunks:
		if chunk.Type != StreamTypeStdout {
			t.Errorf("Expected StreamTypeStdout, got %v", chunk.Type)
		}
		if chunk.Content != "test output" {
			t.Errorf("Expected \"test output\", got %s", chunk.Content)
		}
		if chunk.SequenceNum != 1 {
			t.Errorf("Expected sequence number 1, got %d", chunk.SequenceNum)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("Timeout waiting for chunk")
	}
}

// TestStreamCollector tests the stream collector functionality
func TestStreamCollector(t *testing.T) {
	collector := NewStreamCollector()

	if collector == nil {
		t.Fatalf("Expected non-nil stream collector")
	}

	// Add some test chunks
	chunks := []StreamChunk{
		{Type: StreamTypeStdout, Content: "stdout line 1\n", SequenceNum: 1},
		{Type: StreamTypeStderr, Content: "stderr line 1\n", SequenceNum: 2},
		{Type: StreamTypeStatus, Content: "status update\n", SequenceNum: 3},
		{Type: StreamTypeStdout, Content: "stdout line 2\n", SequenceNum: 4},
	}

	for _, chunk := range chunks {
		collector.AddChunk(chunk)
	}

	// Test output retrieval
	stdout, stderr, status := collector.GetOutput()

	expectedStdout := "stdout line 1\nstdout line 2\n"
	if stdout != expectedStdout {
		t.Errorf("Expected stdout: %q, got: %q", expectedStdout, stdout)
	}

	expectedStderr := "stderr line 1\n"
	if stderr != expectedStderr {
		t.Errorf("Expected stderr: %q, got: %q", expectedStderr, stderr)
	}

	expectedStatus := "status update\n"
	if status != expectedStatus {
		t.Errorf("Expected status: %q, got: %q", expectedStatus, status)
	}

	// Test chunk retrieval
	retrievedChunks := collector.GetChunks()
	if len(retrievedChunks) != len(chunks) {
		t.Errorf("Expected %d chunks, got %d", len(chunks), len(retrievedChunks))
	}
}

// TestStreamingExecutor tests the high-level streaming executor
func TestStreamingExecutor(t *testing.T) {
	executor := NewStreamingExecutor(5 * time.Second)

	if executor == nil {
		t.Fatalf("Expected non-nil streaming executor")
	}

	// Test with a simple echo command
	sessionID := "test-session"
	commandID := "test-command"
	workingDir := "/tmp"
	command := "echo hello world"

	collector, result, err := executor.ExecuteWithStreaming(sessionID, commandID, workingDir, command)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if collector == nil {
		t.Fatalf("Expected non-nil collector")
	}

	if result == nil {
		t.Fatalf("Expected non-nil result")
	}

	if !result.Success {
		t.Errorf("Expected successful command execution")
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}

	// Check output
	stdout, _, _ := collector.GetOutput()
	if !strings.Contains(stdout, "hello world") {
		t.Errorf("Expected stdout to contain 'hello world', got: %s", stdout)
	}
}

// TestExecuteCommand tests basic command execution with streaming
func TestExecuteCommand(t *testing.T) {
	streamer := NewCommandStreamer("test-session", "test-command", "/tmp")
	ctx := context.Background()

	// Test with a simple echo command
	go func() {
		err := streamer.ExecuteCommand(ctx, "echo hello world")
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	}()

	// Collect chunks
	chunks := streamer.GetChunks()
	var foundOutput bool

	// Collect chunks with timeout
	timeout := time.After(5 * time.Second)
	done := false

	for !done {
		select {
		case chunk, ok := <-chunks:
			if !ok {
				done = true
				break
			}
			if chunk.Type == StreamTypeStdout && strings.Contains(chunk.Content, "hello world") {
				foundOutput = true
			}
		case <-timeout:
			t.Fatalf("Timeout waiting for command execution")
		}
	}

	if !foundOutput {
		t.Errorf("Expected to find \"hello world\" in output chunks")
	}

	// Check for result
	select {
	case result := <-streamer.GetResult():
		if !result.Success {
			t.Errorf("Expected successful command execution")
		}
		if result.ExitCode != 0 {
			t.Errorf("Expected exit code 0, got %d", result.ExitCode)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("Timeout waiting for result")
	}
}
