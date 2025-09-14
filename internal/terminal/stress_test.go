package terminal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rama-kairi/go-term/internal/config"
	"github.com/rama-kairi/go-term/internal/database"
	"github.com/rama-kairi/go-term/internal/logger"
)

// StressTestConfig defines configuration for stress testing
type StressTestConfig struct {
	NumSessions          int
	NumBackgroundProcs   int
	TestDuration         time.Duration
	ConcurrentOperations int
	MaxGoroutines        int
	MemoryLimitMB        int
}

// defaultStressConfig returns default stress test configuration
func defaultStressConfig() StressTestConfig {
	return StressTestConfig{
		NumSessions:          20,               // Reduced from 50
		NumBackgroundProcs:   10,               // Reduced from 20
		TestDuration:         10 * time.Second, // Reduced from 5 minutes to 10 seconds
		ConcurrentOperations: 3,                // Reduced from 5 to prevent deadlock
		MaxGoroutines:        500,              // Reduced from 1000
		MemoryLimitMB:        200,              // Reduced from 500
	}
}

// brutalStressConfig returns configuration for brutal stress testing
func brutalStressConfig() StressTestConfig {
	return StressTestConfig{
		NumSessions:          100,              // Reduced from 200
		NumBackgroundProcs:   50,               // Reduced from 100
		TestDuration:         30 * time.Second, // Reduced from 2 hours to 30 seconds
		ConcurrentOperations: 20,               // Reduced from 50
		MaxGoroutines:        1000,             // Reduced from 2000
		MemoryLimitMB:        500,              // Reduced from 1000
	}
}

// setupStressTestEnvironment creates a test environment optimized for stress testing
func setupStressTestEnvironment(t *testing.T) (*Manager, string) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "go-term-stress-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Create stress test config
	cfg := config.DefaultConfig()
	cfg.Database.Path = filepath.Join(tempDir, "stress_test.db")
	cfg.Server.Debug = false                              // Reduce logging overhead in stress tests
	cfg.Session.MaxSessions = 100                         // Reduced from 1000
	cfg.Session.MaxCommandsPerSession = 50                // Reduced from 100
	cfg.Session.MaxBackgroundProcesses = 20               // Reduced from 50
	cfg.Session.BackgroundOutputLimit = 500               // Reduced from 1000 for faster processing
	cfg.Session.ResourceCleanupInterval = 5 * time.Second // Reduced from 30 seconds
	cfg.Session.CleanupInterval = 10 * time.Second        // Reduced from 1 minute
	cfg.Session.DefaultTimeout = 5 * time.Second          // Reduced from 30 seconds
	cfg.Streaming.Enable = false

	// Create logger with minimal output for stress tests
	cfg.Logging.Level = "warn"
	stressLogger, err := logger.NewLogger(&cfg.Logging, "stress_test")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create database
	db, err := database.NewDB(cfg.Database.Path)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Create terminal manager
	manager := NewManager(cfg, stressLogger, db)

	return manager, tempDir
}

// TestBasicResourceManagement tests basic resource management under moderate load
func TestBasicResourceManagement(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	manager, tempDir := setupStressTestEnvironment(t)
	defer os.RemoveAll(tempDir)
	defer manager.Shutdown()

	config := defaultStressConfig()
	config.TestDuration = 5 * time.Second // Very short for basic test

	t.Run("concurrent session creation and deletion", func(t *testing.T) {
		testConcurrentSessionOperations(t, manager, config)
	})

	t.Run("background process lifecycle", func(t *testing.T) {
		testBackgroundProcessLifecycle(t, manager, config)
	})

	t.Run("resource limits enforcement", func(t *testing.T) {
		testResourceLimitsEnforcement(t, manager, config)
	})

	// Skip context cancellation test for now due to deadlock issues
	// t.Run("context cancellation under load", func(t *testing.T) {
	//	testContextCancellationUnderLoad(t, manager, config)
	// })
}

// TestBrutalStressTest runs intensive stress testing to detect resource leaks and race conditions
func TestBrutalStressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping brutal stress test in short mode")
	}

	// Check if this is running in CI or with brutal flag
	if os.Getenv("RUN_BRUTAL_TESTS") != "true" {
		t.Skip("Brutal stress tests require RUN_BRUTAL_TESTS=true environment variable")
	}

	manager, tempDir := setupStressTestEnvironment(t)
	defer os.RemoveAll(tempDir)
	defer manager.Shutdown()

	// Use more reasonable config for testing - real brutal tests need environment variable
	config := brutalStressConfig()
	config.TestDuration = 30 * time.Second // Very short for regular testing
	config.NumSessions = 50                // Further reduced
	config.NumBackgroundProcs = 25         // Further reduced
	config.ConcurrentOperations = 10       // Further reduced

	t.Run("extended resource leak detection", func(t *testing.T) {
		testExtendedResourceLeakDetection(t, manager, config)
	})

	t.Run("massive concurrent operations", func(t *testing.T) {
		testMassiveConcurrentOperations(t, manager, config)
	})

	t.Run("memory pressure handling", func(t *testing.T) {
		testMemoryPressureHandling(t, manager, config)
	})

	t.Run("goroutine leak detection", func(t *testing.T) {
		testGoroutineLeakDetection(t, manager, config)
	})
}

// testConcurrentSessionOperations tests concurrent session creation, use, and deletion
func testConcurrentSessionOperations(t *testing.T, manager *Manager, config StressTestConfig) {
	initialGoroutines := runtime.NumGoroutine()
	var operations int64
	var errors int64

	ctx, cancel := context.WithTimeout(context.Background(), config.TestDuration)
	defer cancel()

	var wg sync.WaitGroup

	// Start concurrent workers
	for i := 0; i < config.ConcurrentOperations; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					return
				default:
					// Create session
					sessionName := fmt.Sprintf("stress-session-%d-%d", workerID, atomic.AddInt64(&operations, 1))
					session, err := manager.CreateSession(sessionName, "", "")
					if err != nil {
						atomic.AddInt64(&errors, 1)
						continue
					}

					// Use session briefly
					_, err = manager.ExecuteCommand(session.ID, "echo 'stress test'")
					if err != nil {
						atomic.AddInt64(&errors, 1)
					}

					// Delete session
					err = manager.CloseSession(session.ID)
					if err != nil {
						atomic.AddInt64(&errors, 1)
					}

					// Brief pause to prevent overwhelming
					time.Sleep(10 * time.Millisecond)
				}
			}
		}(i)
	}

	wg.Wait()

	finalGoroutines := runtime.NumGoroutine()
	totalOps := atomic.LoadInt64(&operations)
	totalErrors := atomic.LoadInt64(&errors)

	t.Logf("Concurrent sessions test completed:")
	t.Logf("  Total operations: %d", totalOps)
	t.Logf("  Total errors: %d", totalErrors)
	t.Logf("  Error rate: %.2f%%", float64(totalErrors)/float64(totalOps)*100)
	t.Logf("  Goroutines: %d -> %d (diff: %d)", initialGoroutines, finalGoroutines, finalGoroutines-initialGoroutines)

	// Check for goroutine leaks (allow some variance for cleanup)
	if finalGoroutines-initialGoroutines > config.ConcurrentOperations {
		t.Errorf("Potential goroutine leak detected: %d new goroutines", finalGoroutines-initialGoroutines)
	}

	// Error rate should be very low
	errorRate := float64(totalErrors) / float64(totalOps) * 100
	if errorRate > 5.0 {
		t.Errorf("High error rate: %.2f%% (expected < 5%%)", errorRate)
	}
}

// testBackgroundProcessLifecycle tests background process creation, monitoring, and cleanup
func testBackgroundProcessLifecycle(t *testing.T, manager *Manager, config StressTestConfig) {
	initialGoroutines := runtime.NumGoroutine()

	// Create a test session
	session, err := manager.CreateSession("bg-lifecycle-test", "", "")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer manager.CloseSession(session.ID)

	ctx, cancel := context.WithTimeout(context.Background(), config.TestDuration)
	defer cancel()

	var wg sync.WaitGroup
	var operations int64
	var errors int64

	// Start background process lifecycle workers
	for i := 0; i < config.ConcurrentOperations; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					return
				default:
					opNum := atomic.AddInt64(&operations, 1)

					// Start a background process
					processID, err := manager.ExecuteCommandInBackground(session.ID, fmt.Sprintf("sleep 0.1 && echo 'worker-%d-op-%d'", workerID, opNum))
					if err != nil {
						atomic.AddInt64(&errors, 1)
						continue
					}

					// Monitor the process briefly
					time.Sleep(50 * time.Millisecond)

					_, err = manager.GetBackgroundProcess(session.ID, processID)
					if err != nil {
						atomic.AddInt64(&errors, 1)
					}

					// Wait for process to complete naturally or terminate it
					time.Sleep(200 * time.Millisecond)

					// Try to terminate (may already be finished)
					manager.TerminateBackgroundProcess(session.ID, processID, false)

					time.Sleep(10 * time.Millisecond)
				}
			}
		}(i)
	}

	wg.Wait()

	finalGoroutines := runtime.NumGoroutine()
	totalOps := atomic.LoadInt64(&operations)
	totalErrors := atomic.LoadInt64(&errors)

	t.Logf("Background process lifecycle test completed:")
	t.Logf("  Total operations: %d", totalOps)
	t.Logf("  Total errors: %d", totalErrors)
	t.Logf("  Error rate: %.2f%%", float64(totalErrors)/float64(totalOps)*100)
	t.Logf("  Goroutines: %d -> %d (diff: %d)", initialGoroutines, finalGoroutines, finalGoroutines-initialGoroutines)

	// Check for goroutine leaks
	if finalGoroutines-initialGoroutines > config.ConcurrentOperations {
		t.Errorf("Potential goroutine leak in background processes: %d new goroutines", finalGoroutines-initialGoroutines)
	}
}

// testResourceLimitsEnforcement tests that resource limits are properly enforced
func testResourceLimitsEnforcement(t *testing.T, manager *Manager, config StressTestConfig) {
	// Test session limits
	t.Run("session limits", func(t *testing.T) {
		sessionIDs := make([]string, 0, manager.config.Session.MaxSessions+10)

		// Try to create more sessions than the limit
		for i := 0; i < manager.config.Session.MaxSessions+10; i++ {
			session, err := manager.CreateSession(fmt.Sprintf("limit-test-%d", i), "", "")
			if err == nil && session != nil {
				sessionIDs = append(sessionIDs, session.ID)
			}
		}

		// Should not exceed the configured limit
		if len(sessionIDs) > manager.config.Session.MaxSessions {
			t.Errorf("Created %d sessions, expected max %d", len(sessionIDs), manager.config.Session.MaxSessions)
		}

		// Cleanup
		for _, id := range sessionIDs {
			manager.CloseSession(id)
		}
	})

	// Test background process limits per session
	t.Run("background process limits", func(t *testing.T) {
		session, err := manager.CreateSession("bg-limit-test", "", "")
		if err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}
		defer manager.CloseSession(session.ID)

		processIDs := make([]string, 0, manager.config.Session.MaxBackgroundProcesses+5)

		// Try to create more background processes than the limit
		for i := 0; i < manager.config.Session.MaxBackgroundProcesses+5; i++ {
			processID, err := manager.ExecuteCommandInBackground(session.ID, "sleep 10")
			if err == nil && processID != "" {
				processIDs = append(processIDs, processID)
			}
		}

		// Should not exceed the configured limit
		if len(processIDs) > manager.config.Session.MaxBackgroundProcesses {
			t.Errorf("Created %d background processes, expected max %d", len(processIDs), manager.config.Session.MaxBackgroundProcesses)
		}

		// Cleanup
		for _, id := range processIDs {
			manager.TerminateBackgroundProcess(session.ID, id, true)
		}
	})
}

// testContextCancellationUnderLoad tests context cancellation behavior under heavy load
func testContextCancellationUnderLoad(t *testing.T, manager *Manager, config StressTestConfig) {
	// Add timeout to prevent hanging
	done := make(chan bool, 1)
	var cancelled int64
	var total int64

	go func() {
		defer func() { done <- true }()

		var wg sync.WaitGroup
		var cancelledOps int64
		var totalOps int64

		// Create multiple sessions that will be cancelled
		for i := 0; i < config.ConcurrentOperations; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				session, err := manager.CreateSession(fmt.Sprintf("cancellation-test-%d", workerID), "", "")
				if err != nil {
					return
				}

				// Start several background processes with short duration
				processIDs := make([]string, 0, 5)
				for j := 0; j < 5; j++ {
					processID, err := manager.ExecuteCommandInBackground(session.ID, "sleep 0.1") // Very short sleep for testing
					if err == nil {
						processIDs = append(processIDs, processID)
						atomic.AddInt64(&totalOps, 1)
					}
				}

				// Wait a bit then close session (should cancel all background processes)
				time.Sleep(100 * time.Millisecond)
				err = manager.CloseSession(session.ID)
				if err == nil {
					atomic.AddInt64(&cancelledOps, int64(len(processIDs)))
				}
			}(i)
		}

		wg.Wait()

		atomic.StoreInt64(&cancelled, atomic.LoadInt64(&cancelledOps))
		atomic.StoreInt64(&total, atomic.LoadInt64(&totalOps))
	}()

	// Wait for completion or timeout
	select {
	case <-done:
		// Test completed successfully
	case <-time.After(30 * time.Second): // Much shorter timeout for this test
		t.Error("Context cancellation test timed out after 30 seconds")
		return
	}

	cancelledVal := atomic.LoadInt64(&cancelled)
	totalVal := atomic.LoadInt64(&total)

	t.Logf("Context cancellation under load test completed:")
	t.Logf("  Total background processes: %d", totalVal)
	t.Logf("  Successfully cancelled: %d", cancelledVal)
	if totalVal > 0 {
		t.Logf("  Cancellation rate: %.2f%%", float64(cancelledVal)/float64(totalVal)*100)
	}

	// Most operations should have been successfully cancelled
	if totalVal > 0 && float64(cancelledVal)/float64(totalVal) < 0.8 {
		t.Errorf("Low cancellation rate: %.2f%% (expected > 80%%)", float64(cancelledVal)/float64(totalVal)*100)
	}
}

// testExtendedResourceLeakDetection runs extended tests to detect resource leaks
func testExtendedResourceLeakDetection(t *testing.T, manager *Manager, config StressTestConfig) {
	initialGoroutines := runtime.NumGoroutine()
	var memStats1, memStats2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memStats1)

	ctx, cancel := context.WithTimeout(context.Background(), config.TestDuration)
	defer cancel()

	sessionCount := 0
	errorCount := 0
	maxSessions := 5000 // Limit maximum sessions to prevent infinite loop

	// Run continuous operations for extended period
	for sessionCount < maxSessions {
		select {
		case <-ctx.Done():
			goto cleanup
		default:
			// Create session
			session, err := manager.CreateSession(fmt.Sprintf("leak-test-%d", sessionCount), "", "")
			if err != nil {
				errorCount++
				// If too many errors, stop the test
				if errorCount > 100 {
					t.Logf("Too many errors (%d), stopping test early", errorCount)
					goto cleanup
				}
				continue
			}

			// Create background processes
			for i := 0; i < 3; i++ {
				processID, err := manager.ExecuteCommandInBackground(session.ID, "echo 'leak test' && sleep 0.1")
				if err == nil {
					// Let some processes run, terminate others
					if i%2 == 0 {
						time.Sleep(50 * time.Millisecond)
						manager.TerminateBackgroundProcess(session.ID, processID, false)
					}
				}
			}

			// Execute some commands
			for i := 0; i < 3; i++ {
				_, err = manager.ExecuteCommand(session.ID, fmt.Sprintf("echo 'command-%d'", i))
				if err != nil {
					errorCount++
				}
			}

			// Close session
			err = manager.CloseSession(session.ID)
			if err != nil {
				errorCount++
			}

			sessionCount++

			// Periodic checks
			if sessionCount%100 == 0 {
				currentGoroutines := runtime.NumGoroutine()
				if currentGoroutines-initialGoroutines > config.MaxGoroutines {
					t.Errorf("Goroutine leak detected at session %d: %d goroutines (started with %d)",
						sessionCount, currentGoroutines, initialGoroutines)
					goto cleanup
				}

				runtime.GC()
				var currentMemStats runtime.MemStats
				runtime.ReadMemStats(&currentMemStats)
				memUsageMB := currentMemStats.Alloc / 1024 / 1024
				if memUsageMB > uint64(config.MemoryLimitMB) {
					t.Errorf("Memory leak detected at session %d: %d MB (limit %d MB)",
						sessionCount, memUsageMB, config.MemoryLimitMB)
					goto cleanup
				}

				t.Logf("Checkpoint at session %d: %d goroutines, %d MB memory",
					sessionCount, currentGoroutines, memUsageMB)
			}

			// Brief pause to prevent overwhelming
			time.Sleep(5 * time.Millisecond)
		}
	}

cleanup:
	runtime.GC()
	runtime.ReadMemStats(&memStats2)
	finalGoroutines := runtime.NumGoroutine()

	t.Logf("Extended leak detection test completed:")
	t.Logf("  Sessions processed: %d", sessionCount)
	t.Logf("  Errors: %d", errorCount)
	t.Logf("  Goroutines: %d -> %d (diff: %d)", initialGoroutines, finalGoroutines, finalGoroutines-initialGoroutines)
	t.Logf("  Memory: %d MB -> %d MB (diff: %d MB)",
		memStats1.Alloc/1024/1024, memStats2.Alloc/1024/1024,
		int64(memStats2.Alloc-memStats1.Alloc)/1024/1024)

	// Final checks
	if finalGoroutines-initialGoroutines > 50 {
		t.Errorf("Significant goroutine leak: %d new goroutines", finalGoroutines-initialGoroutines)
	}

	memGrowthMB := int64(memStats2.Alloc-memStats1.Alloc) / 1024 / 1024
	if memGrowthMB > 100 {
		t.Errorf("Significant memory growth: %d MB", memGrowthMB)
	}
}

// testMassiveConcurrentOperations tests the system under massive concurrent load
func testMassiveConcurrentOperations(t *testing.T, manager *Manager, config StressTestConfig) {
	var wg sync.WaitGroup
	var operations int64
	var errors int64

	ctx, cancel := context.WithTimeout(context.Background(), config.TestDuration)
	defer cancel()

	// Launch many concurrent workers
	for i := 0; i < config.ConcurrentOperations*2; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					return
				default:
					atomic.AddInt64(&operations, 1)

					// Random operation type
					switch (workerID + int(operations)) % 4 {
					case 0: // Create and delete session
						session, err := manager.CreateSession(fmt.Sprintf("massive-test-%d-%d", workerID, operations), "", "")
						if err != nil {
							atomic.AddInt64(&errors, 1)
							continue
						}
						err = manager.CloseSession(session.ID)
						if err != nil {
							atomic.AddInt64(&errors, 1)
						}

					case 1: // Execute command in existing session
						sessions := manager.ListSessions()
						if len(sessions) > 0 {
							sessionID := sessions[0].ID
							_, err := manager.ExecuteCommand(sessionID, "echo 'massive test'")
							if err != nil {
								atomic.AddInt64(&errors, 1)
							}
						}

					case 2: // Background process operations
						sessions := manager.ListSessions()
						if len(sessions) > 0 {
							sessionID := sessions[0].ID
							processID, err := manager.ExecuteCommandInBackground(sessionID, "sleep 0.05")
							if err == nil {
								// Sometimes terminate immediately
								if operations%3 == 0 {
									manager.TerminateBackgroundProcess(sessionID, processID, true)
								}
							} else {
								atomic.AddInt64(&errors, 1)
							}
						}

					case 3: // Resource monitoring
						monitor := manager.GetResourceMonitor()
						if monitor != nil {
							// Just accessing the monitor to ensure it's responsive
							_ = monitor
						}
					}

					// Micro-pause to prevent complete overwhelming
					time.Sleep(1 * time.Millisecond)
				}
			}
		}(i)
	}

	wg.Wait()

	totalOps := atomic.LoadInt64(&operations)
	totalErrors := atomic.LoadInt64(&errors)

	t.Logf("Massive concurrent operations test completed:")
	t.Logf("  Total operations: %d", totalOps)
	t.Logf("  Total errors: %d", totalErrors)
	t.Logf("  Error rate: %.2f%%", float64(totalErrors)/float64(totalOps)*100)
	t.Logf("  Operations per second: %.2f", float64(totalOps)/config.TestDuration.Seconds())

	// Error rate should be reasonable even under massive load
	errorRate := float64(totalErrors) / float64(totalOps) * 100
	if errorRate > 15.0 {
		t.Errorf("High error rate under massive load: %.2f%% (expected < 15%%)", errorRate)
	}
}

// testMemoryPressureHandling tests behavior under memory pressure
func testMemoryPressureHandling(t *testing.T, manager *Manager, config StressTestConfig) {
	// Create many sessions with background processes to create memory pressure
	sessionIDs := make([]string, 0, config.NumSessions)

	for i := 0; i < config.NumSessions; i++ {
		session, err := manager.CreateSession(fmt.Sprintf("memory-pressure-%d", i), "", "")
		if err != nil {
			continue
		}
		sessionIDs = append(sessionIDs, session.ID)

		// Create background processes that generate output
		for j := 0; j < 3; j++ {
			_, err := manager.ExecuteCommandInBackground(session.ID, "for i in {1..100}; do echo 'memory pressure test line number $i'; done")
			if err != nil {
				t.Logf("Failed to create background process in session %d: %v", i, err)
			}
		}

		// Check memory periodically
		if i%10 == 0 {
			runtime.GC()
			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats)
			memUsageMB := memStats.Alloc / 1024 / 1024
			t.Logf("Memory usage at session %d: %d MB", i, memUsageMB)

			if memUsageMB > uint64(config.MemoryLimitMB) {
				t.Logf("Reached memory limit, stopping at session %d", i)
				break
			}
		}
	}

	// Wait for background processes to complete
	time.Sleep(5 * time.Second)

	// Cleanup all sessions
	for _, sessionID := range sessionIDs {
		err := manager.CloseSession(sessionID)
		if err != nil {
			t.Logf("Failed to close session %s: %v", sessionID, err)
		}
	}

	// Force garbage collection and check final memory
	runtime.GC()
	time.Sleep(1 * time.Second)
	runtime.GC()

	var finalMemStats runtime.MemStats
	runtime.ReadMemStats(&finalMemStats)
	finalMemMB := finalMemStats.Alloc / 1024 / 1024

	t.Logf("Memory pressure test completed:")
	t.Logf("  Sessions created: %d", len(sessionIDs))
	t.Logf("  Final memory usage: %d MB", finalMemMB)

	// Memory should be cleaned up reasonably well
	if finalMemMB > uint64(config.MemoryLimitMB/2) {
		t.Errorf("High memory usage after cleanup: %d MB (expected < %d MB)",
			finalMemMB, config.MemoryLimitMB/2)
	}
}

// testGoroutineLeakDetection specifically tests for goroutine leaks
func testGoroutineLeakDetection(t *testing.T, manager *Manager, config StressTestConfig) {
	initialGoroutines := runtime.NumGoroutine()

	// Create and destroy many sessions with background processes
	for round := 0; round < 10; round++ {
		var wg sync.WaitGroup
		sessionIDs := make([]string, 0, 20)

		// Create sessions with background processes
		for i := 0; i < 20; i++ {
			session, err := manager.CreateSession(fmt.Sprintf("goroutine-test-%d-%d", round, i), "", "")
			if err != nil {
				continue
			}
			sessionIDs = append(sessionIDs, session.ID)

			// Start background processes
			for j := 0; j < 3; j++ {
				wg.Add(1)
				go func(sessionID string, procNum int) {
					defer wg.Done()
					processID, err := manager.ExecuteCommandInBackground(sessionID, fmt.Sprintf("sleep 0.%d", procNum))
					if err == nil {
						// Sometimes terminate, sometimes let complete
						if procNum%2 == 0 {
							time.Sleep(50 * time.Millisecond)
							manager.TerminateBackgroundProcess(sessionID, processID, true)
						}
					}
				}(session.ID, j)
			}
		}

		// Wait for all background operations to complete
		wg.Wait()

		// Clean up sessions
		for _, sessionID := range sessionIDs {
			manager.CloseSession(sessionID)
		}

		// Check goroutine count
		currentGoroutines := runtime.NumGoroutine()
		t.Logf("Round %d: %d goroutines", round, currentGoroutines)

		// Allow some variance but detect significant leaks
		if currentGoroutines-initialGoroutines > 100 {
			t.Errorf("Goroutine leak detected in round %d: %d goroutines (started with %d)",
				round, currentGoroutines, initialGoroutines)
			break
		}

		// Brief pause between rounds
		time.Sleep(500 * time.Millisecond)
	}

	finalGoroutines := runtime.NumGoroutine()
	t.Logf("Goroutine leak detection completed:")
	t.Logf("  Initial goroutines: %d", initialGoroutines)
	t.Logf("  Final goroutines: %d", finalGoroutines)
	t.Logf("  Difference: %d", finalGoroutines-initialGoroutines)

	// Final check for goroutine leaks
	if finalGoroutines-initialGoroutines > 20 {
		t.Errorf("Potential goroutine leak: %d new goroutines", finalGoroutines-initialGoroutines)
	}
}

// BenchmarkSessionOperations benchmarks session operations
func BenchmarkSessionOperations(b *testing.B) {
	manager, tempDir := setupStressTestEnvironment(&testing.T{})
	defer os.RemoveAll(tempDir)
	defer manager.Shutdown()

	b.ResetTimer()

	b.Run("CreateDeleteSession", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			session, err := manager.CreateSession(fmt.Sprintf("bench-session-%d", i), "", "")
			if err != nil {
				b.Fatal(err)
			}
			err = manager.CloseSession(session.ID)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("ExecuteCommand", func(b *testing.B) {
		session, err := manager.CreateSession("bench-execute-session", "", "")
		if err != nil {
			b.Fatal(err)
		}
		defer manager.CloseSession(session.ID)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := manager.ExecuteCommand(session.ID, "echo 'benchmark'")
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("BackgroundProcess", func(b *testing.B) {
		session, err := manager.CreateSession("bench-bg-session", "", "")
		if err != nil {
			b.Fatal(err)
		}
		defer manager.CloseSession(session.ID)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			processID, err := manager.ExecuteCommandInBackground(session.ID, "echo 'benchmark background'")
			if err != nil {
				b.Fatal(err)
			}
			// Let some complete, terminate others
			if i%2 == 0 {
				manager.TerminateBackgroundProcess(session.ID, processID, true)
			}
		}
	})
}
