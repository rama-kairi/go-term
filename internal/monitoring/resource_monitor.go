package monitoring

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/rama-kairi/go-term/internal/logger"
)

// ResourceMetrics holds resource usage metrics
type ResourceMetrics struct {
	Timestamp       time.Time `json:"timestamp"`
	Goroutines      int       `json:"goroutines"`
	MemoryAlloc     uint64    `json:"memory_alloc_mb"`
	MemoryHeapInuse uint64    `json:"memory_heap_inuse_mb"`
	MemoryHeapObjs  uint64    `json:"memory_heap_objects"`
	GCCount         uint32    `json:"gc_count"`
	ActiveSessions  int       `json:"active_sessions"`
	BgProcesses     int       `json:"background_processes"`
}

// ResourceMonitor monitors system resources and detects potential leaks
type ResourceMonitor struct {
	logger   *logger.Logger
	metrics  []ResourceMetrics
	mutex    sync.RWMutex
	ticker   *time.Ticker
	stopCh   chan struct{}
	interval time.Duration

	// Baseline metrics for leak detection
	baselineGoroutines int
	baselineMemory     uint64

	// Leak detection thresholds
	maxGoroutineIncrease int
	maxMemoryIncreaseMB  int

	// Callbacks for resource monitoring
	sessionCounter func() int
	processCounter func() int
}

// NewResourceMonitor creates a new resource monitor
func NewResourceMonitor(logger *logger.Logger, interval time.Duration) *ResourceMonitor {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return &ResourceMonitor{
		logger:               logger,
		metrics:              make([]ResourceMetrics, 0, 1000), // Keep last 1000 metrics
		interval:             interval,
		stopCh:               make(chan struct{}),
		baselineGoroutines:   runtime.NumGoroutine(),
		baselineMemory:       m.Alloc,
		maxGoroutineIncrease: 100, // Alert if more than 100 goroutines increase
		maxMemoryIncreaseMB:  200, // Alert if more than 200MB memory increase
	}
}

// SetCounters sets the callback functions for counting sessions and processes
func (rm *ResourceMonitor) SetCounters(sessionCounter, processCounter func() int) {
	rm.sessionCounter = sessionCounter
	rm.processCounter = processCounter
}

// Start begins resource monitoring
func (rm *ResourceMonitor) Start(ctx context.Context) {
	rm.ticker = time.NewTicker(rm.interval)

	go func() {
		defer rm.ticker.Stop()

		// Take initial measurement
		rm.recordMetrics()

		for {
			select {
			case <-rm.ticker.C:
				rm.recordMetrics()
				rm.checkForLeaks()
			case <-rm.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	rm.logger.Info("Resource monitor started", map[string]interface{}{
		"interval":            rm.interval.String(),
		"baseline_goroutines": rm.baselineGoroutines,
		"baseline_memory_mb":  rm.baselineMemory / 1024 / 1024,
		"goroutine_threshold": rm.maxGoroutineIncrease,
		"memory_threshold_mb": rm.maxMemoryIncreaseMB,
	})
}

// Stop stops resource monitoring
func (rm *ResourceMonitor) Stop() {
	close(rm.stopCh)
	rm.logger.Info("Resource monitor stopped")
}

// recordMetrics captures current resource usage
func (rm *ResourceMonitor) recordMetrics() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	activeSessions := 0
	bgProcesses := 0

	if rm.sessionCounter != nil {
		activeSessions = rm.sessionCounter()
	}

	if rm.processCounter != nil {
		bgProcesses = rm.processCounter()
	}

	metric := ResourceMetrics{
		Timestamp:       time.Now(),
		Goroutines:      runtime.NumGoroutine(),
		MemoryAlloc:     m.Alloc / 1024 / 1024, // Convert to MB
		MemoryHeapInuse: m.HeapInuse / 1024 / 1024,
		MemoryHeapObjs:  m.HeapObjects,
		GCCount:         m.NumGC,
		ActiveSessions:  activeSessions,
		BgProcesses:     bgProcesses,
	}

	rm.mutex.Lock()
	rm.metrics = append(rm.metrics, metric)

	// Keep only last 1000 metrics to prevent memory leak
	if len(rm.metrics) > 1000 {
		rm.metrics = rm.metrics[1:]
	}
	rm.mutex.Unlock()
}

// checkForLeaks analyzes current metrics for potential resource leaks
func (rm *ResourceMonitor) checkForLeaks() {
	rm.mutex.RLock()
	if len(rm.metrics) == 0 {
		rm.mutex.RUnlock()
		return
	}

	current := rm.metrics[len(rm.metrics)-1]
	rm.mutex.RUnlock()

	goroutineIncrease := current.Goroutines - rm.baselineGoroutines
	memoryIncreaseMB := int(current.MemoryAlloc) - int(rm.baselineMemory/1024/1024)

	// Check for goroutine leaks
	if goroutineIncrease > rm.maxGoroutineIncrease {
		rm.logger.Warn("potential_goroutine_leak", map[string]interface{}{
			"current_goroutines":   current.Goroutines,
			"baseline_goroutines":  rm.baselineGoroutines,
			"increase":             goroutineIncrease,
			"threshold":            rm.maxGoroutineIncrease,
			"active_sessions":      current.ActiveSessions,
			"background_processes": current.BgProcesses,
		})
	}

	// Check for memory leaks
	if memoryIncreaseMB > rm.maxMemoryIncreaseMB {
		rm.logger.Warn("potential_memory_leak", map[string]interface{}{
			"current_memory_mb":    current.MemoryAlloc,
			"baseline_memory_mb":   rm.baselineMemory / 1024 / 1024,
			"increase_mb":          memoryIncreaseMB,
			"threshold_mb":         rm.maxMemoryIncreaseMB,
			"heap_objects":         current.MemoryHeapObjs,
			"active_sessions":      current.ActiveSessions,
			"background_processes": current.BgProcesses,
		})
	}

	// Check for excessive heap objects
	if current.MemoryHeapObjs > 1000000 {
		rm.logger.Warn("excessive_heap_objects", map[string]interface{}{
			"heap_objects":         current.MemoryHeapObjs,
			"current_memory_mb":    current.MemoryAlloc,
			"active_sessions":      current.ActiveSessions,
			"background_processes": current.BgProcesses,
		})
	}
}

// GetCurrentMetrics returns the current resource metrics
func (rm *ResourceMonitor) GetCurrentMetrics() ResourceMetrics {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	if len(rm.metrics) == 0 {
		return ResourceMetrics{}
	}

	return rm.metrics[len(rm.metrics)-1]
}

// ForceGC triggers garbage collection and logs metrics
func (rm *ResourceMonitor) ForceGC() {
	runtime.GC()
	runtime.GC() // Run twice for more thorough cleanup

	rm.recordMetrics()
	current := rm.GetCurrentMetrics()

	rm.logger.Info("forced_garbage_collection", map[string]interface{}{
		"goroutines":           current.Goroutines,
		"memory_alloc_mb":      current.MemoryAlloc,
		"memory_heap_inuse_mb": current.MemoryHeapInuse,
		"heap_objects":         current.MemoryHeapObjs,
		"gc_count":             current.GCCount,
		"active_sessions":      current.ActiveSessions,
		"background_processes": current.BgProcesses,
	})
}

// GetResourceSummary returns a summary of resource usage
func (rm *ResourceMonitor) GetResourceSummary() map[string]interface{} {
	current := rm.GetCurrentMetrics()

	goroutineIncrease := current.Goroutines - rm.baselineGoroutines
	memoryIncreaseMB := int(current.MemoryAlloc) - int(rm.baselineMemory/1024/1024)

	return map[string]interface{}{
		"timestamp":                current.Timestamp.Format(time.RFC3339),
		"goroutines":               current.Goroutines,
		"goroutines_increase":      goroutineIncrease,
		"memory_alloc_mb":          current.MemoryAlloc,
		"memory_increase_mb":       memoryIncreaseMB,
		"memory_heap_inuse_mb":     current.MemoryHeapInuse,
		"heap_objects":             current.MemoryHeapObjs,
		"gc_count":                 current.GCCount,
		"active_sessions":          current.ActiveSessions,
		"background_processes":     current.BgProcesses,
		"potential_goroutine_leak": goroutineIncrease > rm.maxGoroutineIncrease,
		"potential_memory_leak":    memoryIncreaseMB > rm.maxMemoryIncreaseMB,
		"baseline_goroutines":      rm.baselineGoroutines,
		"baseline_memory_mb":       rm.baselineMemory / 1024 / 1024,
	}
}
