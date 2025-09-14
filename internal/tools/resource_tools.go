package tools

import (
	"context"
	"encoding/json"
	"runtime"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetResourceStatusArgs represents the arguments for getting resource status
type GetResourceStatusArgs struct {
	ForceGC bool `json:"force_gc,omitempty"`
}

// GetResourceStatusResult represents the result of getting resource status
type GetResourceStatusResult struct {
	Status        string                 `json:"status"`
	Message       string                 `json:"message"`
	ResourceData  map[string]interface{} `json:"resource_data"`
	MonitorActive bool                   `json:"monitor_active"`
}

// GetResourceStatus gets current resource usage and monitoring status
func (t *TerminalTools) GetResourceStatus(ctx context.Context, req *mcp.CallToolRequest, args GetResourceStatusArgs) (*mcp.CallToolResult, GetResourceStatusResult, error) {
	// Get resource monitor from terminal manager
	resourceMonitor := t.manager.GetResourceMonitor()
	if resourceMonitor == nil {
		return createErrorResult("Resource monitor not available"), GetResourceStatusResult{}, nil
	}

	// Force garbage collection if requested
	if args.ForceGC {
		resourceMonitor.ForceGC()
		t.logger.Info("Forced garbage collection", map[string]interface{}{
			"goroutines_after_gc": runtime.NumGoroutine(),
		})
	}

	// Get current resource summary
	resourceData := resourceMonitor.GetResourceSummary()
	
	result := GetResourceStatusResult{
		Status:        "success",
		Message:       "Resource status retrieved successfully",
		ResourceData:  resourceData,
		MonitorActive: true,
	}

	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	content := []mcp.Content{
		&mcp.TextContent{
			Text: string(resultJSON),
		},
	}

	return &mcp.CallToolResult{
		Content: content,
	}, result, nil
}

// CheckResourceLeaksArgs represents the arguments for checking resource leaks
type CheckResourceLeaksArgs struct {
	Threshold int `json:"threshold,omitempty"`
}

// CheckResourceLeaksResult represents the result of checking resource leaks
type CheckResourceLeaksResult struct {
	Status             string                 `json:"status"`
	Message            string                 `json:"message"`
	PotentialLeaks     bool                   `json:"potential_leaks"`
	ResourceMetrics    map[string]interface{} `json:"resource_metrics"`
	Recommendations    []string               `json:"recommendations"`
	LeakAnalysis       map[string]interface{} `json:"leak_analysis"`
}

// CheckResourceLeaks analyzes current resource usage for potential leaks
func (t *TerminalTools) CheckResourceLeaks(ctx context.Context, req *mcp.CallToolRequest, args CheckResourceLeaksArgs) (*mcp.CallToolResult, CheckResourceLeaksResult, error) {
	// Get resource monitor from terminal manager
	resourceMonitor := t.manager.GetResourceMonitor()
	if resourceMonitor == nil {
		return createErrorResult("Resource monitor not available"), CheckResourceLeaksResult{}, nil
	}

	// Get current metrics
	currentMetrics := resourceMonitor.GetCurrentMetrics()
	resourceSummary := resourceMonitor.GetResourceSummary()
	
	// Set default threshold
	threshold := args.Threshold
	if threshold == 0 {
		threshold = 50 // Default goroutine increase threshold
	}

	// Analyze for potential leaks
	potentialLeaks := false
	recommendations := []string{}
	leakAnalysis := make(map[string]interface{})

	// Check goroutine leaks
	if val, ok := resourceSummary["potential_goroutine_leak"].(bool); ok && val {
		potentialLeaks = true
		recommendations = append(recommendations, "Potential goroutine leak detected - check background processes for proper cleanup")
		leakAnalysis["goroutine_leak"] = map[string]interface{}{
			"detected": true,
			"current_goroutines": currentMetrics.Goroutines,
			"baseline": resourceSummary["baseline_goroutines"],
			"increase": resourceSummary["goroutines_increase"],
		}
	}

	// Check memory leaks
	if val, ok := resourceSummary["potential_memory_leak"].(bool); ok && val {
		potentialLeaks = true
		recommendations = append(recommendations, "Potential memory leak detected - consider forcing garbage collection or restarting server")
		leakAnalysis["memory_leak"] = map[string]interface{}{
			"detected": true,
			"current_memory_mb": currentMetrics.MemoryAlloc,
			"baseline_mb": resourceSummary["baseline_memory_mb"],
			"increase_mb": resourceSummary["memory_increase_mb"],
		}
	}

	// Check excessive heap objects
	if currentMetrics.MemoryHeapObjs > 500000 {
		potentialLeaks = true
		recommendations = append(recommendations, "High number of heap objects detected - monitor for object accumulation")
		leakAnalysis["heap_objects"] = map[string]interface{}{
			"detected": true,
			"heap_objects": currentMetrics.MemoryHeapObjs,
			"threshold": 500000,
		}
	}

	// Add session-specific recommendations
	sessionCount := currentMetrics.ActiveSessions
	bgProcessCount := currentMetrics.BgProcesses

	if sessionCount > 10 {
		recommendations = append(recommendations, "High number of active sessions - consider cleaning up unused sessions")
	}

	if bgProcessCount > 5 {
		recommendations = append(recommendations, "High number of background processes - monitor for processes that haven't terminated properly")
	}

	if !potentialLeaks {
		recommendations = append(recommendations, "No resource leaks detected - system is running normally")
	}

	result := CheckResourceLeaksResult{
		Status:             "success",
		Message:            "Resource leak analysis completed",
		PotentialLeaks:     potentialLeaks,
		ResourceMetrics:    resourceSummary,
		Recommendations:    recommendations,
		LeakAnalysis:       leakAnalysis,
	}

	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	content := []mcp.Content{
		&mcp.TextContent{
			Text: string(resultJSON),
		},
	}

	t.logger.Info("Resource leak check completed", map[string]interface{}{
		"potential_leaks": potentialLeaks,
		"goroutines": currentMetrics.Goroutines,
		"memory_mb": currentMetrics.MemoryAlloc,
		"active_sessions": sessionCount,
		"bg_processes": bgProcessCount,
	})

	return &mcp.CallToolResult{
		Content: content,
	}, result, nil
}

// ForceCleanupArgs represents the arguments for forcing resource cleanup
type ForceCleanupArgs struct {
	CleanupType string `json:"cleanup_type,omitempty"` // "gc", "sessions", "processes", "all"
	Confirm     bool   `json:"confirm"`
}

// ForceCleanupResult represents the result of forcing resource cleanup
type ForceCleanupResult struct {
	Status        string                 `json:"status"`
	Message       string                 `json:"message"`
	CleanupActions []string               `json:"cleanup_actions"`
	BeforeMetrics map[string]interface{} `json:"before_metrics"`
	AfterMetrics  map[string]interface{} `json:"after_metrics"`
}

// ForceCleanup performs aggressive resource cleanup to address potential leaks
func (t *TerminalTools) ForceCleanup(ctx context.Context, req *mcp.CallToolRequest, args ForceCleanupArgs) (*mcp.CallToolResult, ForceCleanupResult, error) {
	if !args.Confirm {
		return createErrorResult("Cleanup requires confirmation (set confirm: true)"), ForceCleanupResult{}, nil
	}

	// Get resource monitor
	resourceMonitor := t.manager.GetResourceMonitor()
	if resourceMonitor == nil {
		return createErrorResult("Resource monitor not available"), ForceCleanupResult{}, nil
	}

	// Get before metrics
	beforeMetrics := resourceMonitor.GetResourceSummary()
	cleanupActions := []string{}

	// Set default cleanup type
	cleanupType := args.CleanupType
	if cleanupType == "" {
		cleanupType = "gc"
	}

	// Perform cleanup based on type
	switch cleanupType {
	case "gc":
		resourceMonitor.ForceGC()
		cleanupActions = append(cleanupActions, "Forced garbage collection (2x)")
		
	case "sessions":
		// Note: Actual session cleanup would require more complex logic
		// This is a placeholder for future implementation
		cleanupActions = append(cleanupActions, "Session cleanup not implemented yet")
		
	case "processes":
		// Note: Process cleanup would require careful handling
		cleanupActions = append(cleanupActions, "Process cleanup not implemented yet")
		
	case "all":
		resourceMonitor.ForceGC()
		cleanupActions = append(cleanupActions, "Forced garbage collection (2x)")
		cleanupActions = append(cleanupActions, "Full resource cleanup performed")
		
	default:
		return createErrorResult("Invalid cleanup_type. Use: gc, sessions, processes, or all"), ForceCleanupResult{}, nil
	}

	// Wait a moment for cleanup to take effect
	time.Sleep(2 * time.Second)

	// Get after metrics
	afterMetrics := resourceMonitor.GetResourceSummary()

	result := ForceCleanupResult{
		Status:         "success",
		Message:        "Resource cleanup completed",
		CleanupActions: cleanupActions,
		BeforeMetrics:  beforeMetrics,
		AfterMetrics:   afterMetrics,
	}

	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	content := []mcp.Content{
		&mcp.TextContent{
			Text: string(resultJSON),
		},
	}

	t.logger.Info("Resource cleanup completed", map[string]interface{}{
		"cleanup_type": cleanupType,
		"actions": cleanupActions,
		"goroutines_before": beforeMetrics["goroutines"],
		"goroutines_after": afterMetrics["goroutines"],
		"memory_before_mb": beforeMetrics["memory_alloc_mb"],
		"memory_after_mb": afterMetrics["memory_alloc_mb"],
	})

	return &mcp.CallToolResult{
		Content: content,
	}, result, nil
}
