package monitoring

import (
	"context"
	"testing"
	"time"

	"github.com/rama-kairi/go-term/internal/config"
	"github.com/rama-kairi/go-term/internal/logger"
)

func TestResourceMonitor(t *testing.T) {
	// Create a test logger
	cfg := &config.LoggingConfig{
		Level:  "info",
		Format: "json",
	}
	testLogger, err := logger.NewLogger(cfg, "test")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create resource monitor
	monitor := NewResourceMonitor(testLogger, 100*time.Millisecond)
	
	// Test basic initialization
	if monitor == nil {
		t.Fatal("Expected monitor to be created")
	}

	// Test setting counters
	sessionCount := 5
	processCount := 3
	monitor.SetCounters(
		func() int { return sessionCount },
		func() int { return processCount },
	)

	// Start monitoring
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	
	monitor.Start(ctx)
	
	// Wait for at least one measurement
	time.Sleep(200 * time.Millisecond)
	
	// Get current metrics
	metrics := monitor.GetCurrentMetrics()
	
	// Verify metrics are populated
	if metrics.Timestamp.IsZero() {
		t.Error("Expected timestamp to be set")
	}
	
	if metrics.Goroutines <= 0 {
		t.Error("Expected goroutines count to be positive")
	}
	
	if metrics.ActiveSessions != sessionCount {
		t.Errorf("Expected %d active sessions, got %d", sessionCount, metrics.ActiveSessions)
	}
	
	if metrics.BgProcesses != processCount {
		t.Errorf("Expected %d background processes, got %d", processCount, metrics.BgProcesses)
	}
	
	// Test resource summary
	summary := monitor.GetResourceSummary()
	if summary == nil {
		t.Error("Expected resource summary to be available")
	}
	
	// Check required fields
	requiredFields := []string{
		"timestamp", "goroutines", "memory_alloc_mb", "active_sessions", "background_processes",
	}
	
	for _, field := range requiredFields {
		if _, exists := summary[field]; !exists {
			t.Errorf("Expected field '%s' to exist in resource summary", field)
		}
	}
	
	// Stop monitoring
	monitor.Stop()
	
	t.Log("✅ Resource monitor test completed successfully")
}

func TestResourceMonitorForceGC(t *testing.T) {
	// Create a test logger
	cfg := &config.LoggingConfig{
		Level:  "info",
		Format: "json",
	}
	testLogger, err := logger.NewLogger(cfg, "test")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create resource monitor
	monitor := NewResourceMonitor(testLogger, time.Second)
	
	// Get initial metrics
	initialMetrics := monitor.GetCurrentMetrics()
	
	// Force garbage collection
	monitor.ForceGC()
	
	// Get metrics after GC
	afterGCMetrics := monitor.GetCurrentMetrics()
	
	// Verify metrics were updated
	if !afterGCMetrics.Timestamp.After(initialMetrics.Timestamp) {
		t.Error("Expected timestamp to be updated after ForceGC")
	}
	
	t.Log("✅ Resource monitor ForceGC test completed successfully")
}
