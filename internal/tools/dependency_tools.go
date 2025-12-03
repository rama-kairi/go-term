package tools

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// F7: ProcessDependency represents a dependency between background processes
type ProcessDependency struct {
	ID           string    `json:"id"`
	ProcessID    string    `json:"process_id"`
	DependsOnID  string    `json:"depends_on_id"`
	SessionID    string    `json:"session_id"`
	WaitForReady bool      `json:"wait_for_ready"`  // Wait for process to be ready
	ReadyPattern string    `json:"ready_pattern"`   // Pattern in output indicating ready
	Timeout      int       `json:"timeout_seconds"` // Timeout for waiting
	CreatedAt    time.Time `json:"created_at"`
}

// F7: ProcessChain represents a chain of processes with dependencies
type ProcessChain struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	SessionID   string           `json:"session_id"`
	Processes   []ChainedProcess `json:"processes"`
	Status      string           `json:"status"` // pending, running, completed, failed
	StartedAt   time.Time        `json:"started_at,omitempty"`
	CompletedAt time.Time        `json:"completed_at,omitempty"`
	Error       string           `json:"error,omitempty"`
}

// ChainedProcess represents a process in a chain
type ChainedProcess struct {
	Name         string `json:"name"`
	Command      string `json:"command"`
	ReadyPattern string `json:"ready_pattern,omitempty"` // Pattern indicating process is ready
	WaitSeconds  int    `json:"wait_seconds,omitempty"`  // Wait this many seconds before next
	ProcessID    string `json:"process_id,omitempty"`    // Set after starting
	Status       string `json:"status"`                  // pending, starting, running, ready, failed
}

// F7: DependencyManager manages process dependencies
type DependencyManager struct {
	dependencies map[string]*ProcessDependency
	chains       map[string]*ProcessChain
	mu           sync.RWMutex
}

// NewDependencyManager creates a new dependency manager
func NewDependencyManager() *DependencyManager {
	return &DependencyManager{
		dependencies: make(map[string]*ProcessDependency),
		chains:       make(map[string]*ProcessChain),
	}
}

// AddDependency adds a process dependency
func (dm *DependencyManager) AddDependency(dep *ProcessDependency) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dep.ProcessID == dep.DependsOnID {
		return fmt.Errorf("process cannot depend on itself")
	}

	dep.ID = fmt.Sprintf("dep-%s-%s", dep.ProcessID[:8], dep.DependsOnID[:8])
	dep.CreatedAt = time.Now()
	dm.dependencies[dep.ID] = dep
	return nil
}

// GetDependencies returns dependencies for a process
func (dm *DependencyManager) GetDependencies(processID string) []*ProcessDependency {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	var result []*ProcessDependency
	for _, dep := range dm.dependencies {
		if dep.ProcessID == processID {
			result = append(result, dep)
		}
	}
	return result
}

// CreateChain creates a new process chain
func (dm *DependencyManager) CreateChain(chain *ProcessChain) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if chain.Name == "" {
		return fmt.Errorf("chain name cannot be empty")
	}
	if len(chain.Processes) == 0 {
		return fmt.Errorf("chain must have at least one process")
	}

	chain.ID = fmt.Sprintf("chain-%s-%d", chain.Name, time.Now().Unix())
	chain.Status = "pending"
	for i := range chain.Processes {
		chain.Processes[i].Status = "pending"
	}

	dm.chains[chain.ID] = chain
	return nil
}

// GetChain retrieves a chain by ID
func (dm *DependencyManager) GetChain(chainID string) (*ProcessChain, bool) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	chain, exists := dm.chains[chainID]
	return chain, exists
}

// ListChains returns all chains
func (dm *DependencyManager) ListChains() []*ProcessChain {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	result := make([]*ProcessChain, 0, len(dm.chains))
	for _, c := range dm.chains {
		result = append(result, c)
	}
	return result
}

// UpdateChainStatus updates the status of a chain
func (dm *DependencyManager) UpdateChainStatus(chainID, status, errorMsg string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if chain, exists := dm.chains[chainID]; exists {
		chain.Status = status
		if errorMsg != "" {
			chain.Error = errorMsg
		}
		if status == "completed" || status == "failed" {
			chain.CompletedAt = time.Now()
		}
	}
}

// UpdateProcessStatus updates the status of a process in a chain
func (dm *DependencyManager) UpdateProcessStatus(chainID string, processIndex int, status, processID string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if chain, exists := dm.chains[chainID]; exists {
		if processIndex >= 0 && processIndex < len(chain.Processes) {
			chain.Processes[processIndex].Status = status
			if processID != "" {
				chain.Processes[processIndex].ProcessID = processID
			}
		}
	}
}

// =============================================================================
// F7: Dependency Tool Handlers
// =============================================================================

// CreateProcessChainArgs represents arguments for creating a process chain
type CreateProcessChainArgs struct {
	SessionID   string           `json:"session_id" jsonschema:"required,description=Session ID to run processes in"`
	Name        string           `json:"name" jsonschema:"required,description=Name for the process chain"`
	Description string           `json:"description,omitempty" jsonschema:"description=Description of the chain"`
	Processes   []ChainedProcess `json:"processes" jsonschema:"required,description=List of processes to run in order"`
}

// CreateProcessChainResult represents the result of creating a chain
type CreateProcessChainResult struct {
	ChainID   string `json:"chain_id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Processes int    `json:"process_count"`
	Message   string `json:"message"`
}

// StartProcessChainArgs represents arguments for starting a chain
type StartProcessChainArgs struct {
	ChainID string `json:"chain_id" jsonschema:"required,description=Chain ID to start"`
}

// StartProcessChainResult represents the result of starting a chain
type StartProcessChainResult struct {
	ChainID    string   `json:"chain_id"`
	Status     string   `json:"status"`
	ProcessIDs []string `json:"process_ids"`
	Message    string   `json:"message"`
}

// GetProcessChainStatusArgs represents arguments for getting chain status
type GetProcessChainStatusArgs struct {
	ChainID string `json:"chain_id" jsonschema:"required,description=Chain ID to check"`
}

// CreateProcessChain creates a new process chain with dependencies
func (t *TerminalTools) CreateProcessChain(ctx context.Context, req *mcp.CallToolRequest, args CreateProcessChainArgs) (*mcp.CallToolResult, CreateProcessChainResult, error) {
	// Validate session exists
	if _, err := t.manager.GetSession(args.SessionID); err != nil {
		return createErrorResult(fmt.Sprintf("Session not found: %v", err)), CreateProcessChainResult{}, nil
	}

	chain := &ProcessChain{
		Name:        args.Name,
		Description: args.Description,
		SessionID:   args.SessionID,
		Processes:   args.Processes,
	}

	if err := t.dependencyManager.CreateChain(chain); err != nil {
		return createErrorResult(fmt.Sprintf("Failed to create chain: %v", err)), CreateProcessChainResult{}, nil
	}

	result := CreateProcessChainResult{
		ChainID:   chain.ID,
		Name:      chain.Name,
		Status:    chain.Status,
		Processes: len(chain.Processes),
		Message:   fmt.Sprintf("Process chain '%s' created with %d processes", chain.Name, len(chain.Processes)),
	}

	t.logger.Info("Process chain created", map[string]interface{}{
		"chain_id":      chain.ID,
		"name":          chain.Name,
		"process_count": len(chain.Processes),
	})

	return createJSONResult(result), result, nil
}

// StartProcessChain starts executing a process chain
func (t *TerminalTools) StartProcessChain(ctx context.Context, req *mcp.CallToolRequest, args StartProcessChainArgs) (*mcp.CallToolResult, StartProcessChainResult, error) {
	chain, exists := t.dependencyManager.GetChain(args.ChainID)
	if !exists {
		return createErrorResult(fmt.Sprintf("Chain not found: %s", args.ChainID)), StartProcessChainResult{}, nil
	}

	if chain.Status != "pending" {
		return createErrorResult(fmt.Sprintf("Chain is already %s", chain.Status)), StartProcessChainResult{}, nil
	}

	// Update chain status
	t.dependencyManager.UpdateChainStatus(args.ChainID, "running", "")
	chain.StartedAt = time.Now()

	var processIDs []string

	// Start processes in order with dependency handling
	go func() {
		for i, proc := range chain.Processes {
			t.dependencyManager.UpdateProcessStatus(args.ChainID, i, "starting", "")

			// Start the background process
			processID, err := t.manager.ExecuteCommandInBackground(chain.SessionID, proc.Command)
			if err != nil {
				t.dependencyManager.UpdateProcessStatus(args.ChainID, i, "failed", "")
				t.dependencyManager.UpdateChainStatus(args.ChainID, "failed", fmt.Sprintf("Process %d failed: %v", i, err))
				return
			}

			processIDs = append(processIDs, processID)
			t.dependencyManager.UpdateProcessStatus(args.ChainID, i, "running", processID)

			// Wait for ready pattern or fixed delay
			if proc.WaitSeconds > 0 {
				time.Sleep(time.Duration(proc.WaitSeconds) * time.Second)
			}

			// Check if process is still running
			bgProc, err := t.manager.GetBackgroundProcess(chain.SessionID, processID)
			if err != nil || !bgProc.IsRunning {
				t.dependencyManager.UpdateProcessStatus(args.ChainID, i, "failed", processID)
				t.dependencyManager.UpdateChainStatus(args.ChainID, "failed", fmt.Sprintf("Process %d exited unexpectedly", i))
				return
			}

			t.dependencyManager.UpdateProcessStatus(args.ChainID, i, "ready", processID)
		}

		t.dependencyManager.UpdateChainStatus(args.ChainID, "completed", "")
	}()

	result := StartProcessChainResult{
		ChainID:    chain.ID,
		Status:     "running",
		ProcessIDs: processIDs,
		Message:    fmt.Sprintf("Started process chain '%s'", chain.Name),
	}

	return createJSONResult(result), result, nil
}

// GetProcessChainStatus gets the current status of a process chain
func (t *TerminalTools) GetProcessChainStatus(ctx context.Context, req *mcp.CallToolRequest, args GetProcessChainStatusArgs) (*mcp.CallToolResult, *ProcessChain, error) {
	chain, exists := t.dependencyManager.GetChain(args.ChainID)
	if !exists {
		return createErrorResult(fmt.Sprintf("Chain not found: %s", args.ChainID)), nil, nil
	}

	return createJSONResult(chain), chain, nil
}
