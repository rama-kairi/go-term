package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"
)

// M8: HealthEndpoint provides HTTP health check endpoints
type HealthEndpoint struct {
	server       *http.Server
	resourceMon  *ResourceMonitor
	healthChecks map[string]HealthChecker
	mu           sync.RWMutex
	startTime    time.Time
}

// HealthChecker is an interface for components that can report health
type HealthChecker interface {
	HealthCheck() error
}

// HealthStatus represents the overall health status
type HealthStatus struct {
	Status     string                     `json:"status"` // "healthy", "degraded", "unhealthy"
	Timestamp  time.Time                  `json:"timestamp"`
	Uptime     string                     `json:"uptime"`
	Components map[string]ComponentHealth `json:"components"`
	Metrics    HealthMetrics              `json:"metrics"`
}

// ComponentHealth represents health of a single component
type ComponentHealth struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// HealthMetrics contains resource metrics
type HealthMetrics struct {
	MemoryUsedMB  uint64  `json:"memory_used_mb"`
	MemoryTotalMB uint64  `json:"memory_total_mb"`
	Goroutines    int     `json:"goroutines"`
	CPUs          int     `json:"cpus"`
	GCPauseMs     float64 `json:"gc_pause_ms"`
}

// NewHealthEndpoint creates a new health endpoint
func NewHealthEndpoint(port int, resourceMon *ResourceMonitor) *HealthEndpoint {
	he := &HealthEndpoint{
		resourceMon:  resourceMon,
		healthChecks: make(map[string]HealthChecker),
		startTime:    time.Now(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", he.handleHealth)
	mux.HandleFunc("/health/live", he.handleLiveness)
	mux.HandleFunc("/health/ready", he.handleReadiness)
	mux.HandleFunc("/metrics", he.handleMetrics)

	he.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return he
}

// RegisterHealthCheck registers a component for health checking
func (he *HealthEndpoint) RegisterHealthCheck(name string, checker HealthChecker) {
	he.mu.Lock()
	defer he.mu.Unlock()
	he.healthChecks[name] = checker
}

// Start starts the health endpoint server
func (he *HealthEndpoint) Start() error {
	go func() {
		if err := he.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Log error but don't panic - health endpoint is optional
			fmt.Printf("Health endpoint error: %v\n", err)
		}
	}()
	return nil
}

// Stop gracefully stops the health endpoint server
func (he *HealthEndpoint) Stop(ctx context.Context) error {
	return he.server.Shutdown(ctx)
}

// handleHealth returns comprehensive health status
func (he *HealthEndpoint) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := he.getHealthStatus()

	w.Header().Set("Content-Type", "application/json")

	switch status.Status {
	case "healthy":
		w.WriteHeader(http.StatusOK)
	case "degraded":
		w.WriteHeader(http.StatusOK) // Still OK but with warnings
	default:
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(status)
}

// handleLiveness returns simple liveness probe (is the process running?)
func (he *HealthEndpoint) handleLiveness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "alive",
		"uptime": time.Since(he.startTime).String(),
	})
}

// handleReadiness returns readiness probe (is the service ready to accept requests?)
func (he *HealthEndpoint) handleReadiness(w http.ResponseWriter, r *http.Request) {
	he.mu.RLock()
	defer he.mu.RUnlock()

	ready := true
	components := make(map[string]string)

	for name, checker := range he.healthChecks {
		if err := checker.HealthCheck(); err != nil {
			ready = false
			components[name] = err.Error()
		} else {
			components[name] = "ready"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if ready {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"ready":      ready,
		"components": components,
	})
}

// handleMetrics returns Prometheus-compatible metrics
func (he *HealthEndpoint) handleMetrics(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	// Basic metrics in Prometheus format
	fmt.Fprintf(w, "# HELP goterm_memory_alloc_bytes Current memory allocation in bytes\n")
	fmt.Fprintf(w, "# TYPE goterm_memory_alloc_bytes gauge\n")
	fmt.Fprintf(w, "goterm_memory_alloc_bytes %d\n", m.Alloc)

	fmt.Fprintf(w, "# HELP goterm_memory_sys_bytes Total memory obtained from system\n")
	fmt.Fprintf(w, "# TYPE goterm_memory_sys_bytes gauge\n")
	fmt.Fprintf(w, "goterm_memory_sys_bytes %d\n", m.Sys)

	fmt.Fprintf(w, "# HELP goterm_goroutines Current number of goroutines\n")
	fmt.Fprintf(w, "# TYPE goterm_goroutines gauge\n")
	fmt.Fprintf(w, "goterm_goroutines %d\n", runtime.NumGoroutine())

	fmt.Fprintf(w, "# HELP goterm_gc_total_count Total number of GC cycles\n")
	fmt.Fprintf(w, "# TYPE goterm_gc_total_count counter\n")
	fmt.Fprintf(w, "goterm_gc_total_count %d\n", m.NumGC)

	fmt.Fprintf(w, "# HELP goterm_uptime_seconds Server uptime in seconds\n")
	fmt.Fprintf(w, "# TYPE goterm_uptime_seconds counter\n")
	fmt.Fprintf(w, "goterm_uptime_seconds %.0f\n", time.Since(he.startTime).Seconds())

	// Add resource monitor metrics if available
	if he.resourceMon != nil {
		metrics := he.resourceMon.GetCurrentMetrics()
		fmt.Fprintf(w, "# HELP goterm_heap_alloc_mb Heap allocation in megabytes\n")
		fmt.Fprintf(w, "# TYPE goterm_heap_alloc_mb gauge\n")
		fmt.Fprintf(w, "goterm_heap_alloc_mb %d\n", metrics.MemoryAlloc)
	}
}

// getHealthStatus computes the current health status
func (he *HealthEndpoint) getHealthStatus() HealthStatus {
	he.mu.RLock()
	defer he.mu.RUnlock()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	components := make(map[string]ComponentHealth)
	overallHealthy := true
	hasDegraded := false

	for name, checker := range he.healthChecks {
		if err := checker.HealthCheck(); err != nil {
			components[name] = ComponentHealth{
				Status:  "unhealthy",
				Message: err.Error(),
			}
			overallHealthy = false
		} else {
			components[name] = ComponentHealth{
				Status: "healthy",
			}
		}
	}

	// Check resource health
	if runtime.NumGoroutine() > 1000 {
		hasDegraded = true
		components["goroutines"] = ComponentHealth{
			Status:  "degraded",
			Message: "High goroutine count",
		}
	}

	if m.Alloc > 500*1024*1024 { // >500MB
		hasDegraded = true
		components["memory"] = ComponentHealth{
			Status:  "degraded",
			Message: "High memory usage",
		}
	}

	status := "healthy"
	if !overallHealthy {
		status = "unhealthy"
	} else if hasDegraded {
		status = "degraded"
	}

	return HealthStatus{
		Status:     status,
		Timestamp:  time.Now(),
		Uptime:     time.Since(he.startTime).String(),
		Components: components,
		Metrics: HealthMetrics{
			MemoryUsedMB:  m.Alloc / (1024 * 1024),
			MemoryTotalMB: m.Sys / (1024 * 1024),
			Goroutines:    runtime.NumGoroutine(),
			CPUs:          runtime.NumCPU(),
			GCPauseMs:     float64(m.PauseNs[(m.NumGC+255)%256]) / 1e6,
		},
	}
}
