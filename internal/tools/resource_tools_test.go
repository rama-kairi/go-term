package tools

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rama-kairi/go-term/internal/config"
	"github.com/rama-kairi/go-term/internal/logger"
	"github.com/rama-kairi/go-term/internal/terminal"
)

func setupTestTerminalToolsWithResourceMonitoring(t *testing.T) *TerminalTools {
	// Create test configuration
	cfg := &config.Config{
		Session: config.SessionConfig{
			MaxSessions:             10,
			MaxCommandLength:        1000,
			DefaultTimeout:          time.Minute * 30,
			CleanupInterval:         time.Minute,
			ResourceCleanupInterval: time.Minute,
		},
		Security: config.SecurityConfig{
			EnableSandbox:      false,
			BlockedCommands:    []string{},
			AllowNetworkAccess: true,
		},
		Logging: config.LoggingConfig{
			Level:  "info",
			Format: "json",
		},
		Database: config.DatabaseConfig{
			Enable: false,
		},
	}

	// Create test logger
	testLogger, err := logger.NewLogger(&cfg.Logging, "test")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create terminal manager
	manager := terminal.NewManager(cfg, testLogger, nil)

	// Create terminal tools
	tools := NewTerminalTools(manager, cfg, testLogger, nil)

	return tools
}

func TestGetResourceStatus(t *testing.T) {
	tools := setupTestTerminalToolsWithResourceMonitoring(t)

	// Test without force GC
	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	args := GetResourceStatusArgs{ForceGC: false}

	result, response, err := tools.GetResourceStatus(ctx, req, args)
	if err != nil {
		t.Fatalf("GetResourceStatus failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}

	if response.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", response.Status)
	}

	if !response.MonitorActive {
		t.Error("Expected monitor to be active")
	}

	if response.ResourceData == nil {
		t.Error("Expected resource data to be present")
	}

	// Test with force GC
	args.ForceGC = true
	result, response, err = tools.GetResourceStatus(ctx, req, args)
	if err != nil {
		t.Fatalf("GetResourceStatus with ForceGC failed: %v", err)
	}

	if response.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", response.Status)
	}

	t.Log("✅ GetResourceStatus test completed successfully")
}

func TestCheckResourceLeaks(t *testing.T) {
	tools := setupTestTerminalToolsWithResourceMonitoring(t)

	ctx := context.Background()
	req := &mcp.CallToolRequest{}
	args := CheckResourceLeaksArgs{Threshold: 50}

	result, response, err := tools.CheckResourceLeaks(ctx, req, args)
	if err != nil {
		t.Fatalf("CheckResourceLeaks failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}

	if response.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", response.Status)
	}

	if response.ResourceMetrics == nil {
		t.Error("Expected resource metrics to be present")
	}

	if response.Recommendations == nil {
		t.Error("Expected recommendations to be present")
	}

	if response.LeakAnalysis == nil {
		t.Error("Expected leak analysis to be present")
	}

	// For a fresh server, we shouldn't detect leaks
	if response.PotentialLeaks {
		t.Log("Warning: Potential leaks detected in fresh server - this may be expected during testing")
	}

	t.Log("✅ CheckResourceLeaks test completed successfully")
}

func TestForceCleanup(t *testing.T) {
	tools := setupTestTerminalToolsWithResourceMonitoring(t)

	ctx := context.Background()
	req := &mcp.CallToolRequest{}

	// Test without confirmation (should fail)
	args := ForceCleanupArgs{Confirm: false}
	result, _, err := tools.ForceCleanup(ctx, req, args)
	if err != nil {
		t.Fatalf("ForceCleanup failed: %v", err)
	}

	// Should return error result
	if result.Content == nil || len(result.Content) == 0 {
		t.Error("Expected error content for unconfirmed cleanup")
	}

	// Test with confirmation
	args.Confirm = true
	args.CleanupType = "gc"

	result, response, err := tools.ForceCleanup(ctx, req, args)
	if err != nil {
		t.Fatalf("ForceCleanup with confirmation failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}

	if response.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", response.Status)
	}

	if response.CleanupActions == nil || len(response.CleanupActions) == 0 {
		t.Error("Expected cleanup actions to be present")
	}

	if response.BeforeMetrics == nil {
		t.Error("Expected before metrics to be present")
	}

	if response.AfterMetrics == nil {
		t.Error("Expected after metrics to be present")
	}

	t.Log("✅ ForceCleanup test completed successfully")
}
