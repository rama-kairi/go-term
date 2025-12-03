package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// F1: CommandTemplate represents a pre-defined command template
type CommandTemplate struct {
	Name        string            `json:"name"`
	Command     string            `json:"command"`
	Description string            `json:"description"`
	Category    string            `json:"category"`
	Variables   map[string]string `json:"variables,omitempty"` // Variable placeholders and defaults
	Tags        []string          `json:"tags,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

// F1: TemplateManager manages command templates/aliases
type TemplateManager struct {
	templates map[string]*CommandTemplate
	mu        sync.RWMutex
}

// NewTemplateManager creates a new template manager with default templates
func NewTemplateManager() *TemplateManager {
	tm := &TemplateManager{
		templates: make(map[string]*CommandTemplate),
	}

	// Add default templates for common operations
	tm.addDefaultTemplates()

	return tm
}

// addDefaultTemplates adds commonly used command templates
func (tm *TemplateManager) addDefaultTemplates() {
	defaults := []*CommandTemplate{
		// Node.js templates
		{Name: "npm-install", Command: "npm install", Description: "Install npm dependencies", Category: "nodejs", Tags: []string{"npm", "install"}},
		{Name: "npm-dev", Command: "npm run dev", Description: "Start development server", Category: "nodejs", Tags: []string{"npm", "dev"}},
		{Name: "npm-build", Command: "npm run build", Description: "Build production bundle", Category: "nodejs", Tags: []string{"npm", "build"}},
		{Name: "npm-test", Command: "npm test", Description: "Run tests", Category: "nodejs", Tags: []string{"npm", "test"}},
		{Name: "npm-lint", Command: "npm run lint", Description: "Run linter", Category: "nodejs", Tags: []string{"npm", "lint"}},

		// Python templates
		{Name: "py-venv", Command: "python -m venv venv && source venv/bin/activate", Description: "Create and activate virtualenv", Category: "python", Tags: []string{"python", "venv"}},
		{Name: "pip-install", Command: "pip install -r requirements.txt", Description: "Install Python dependencies", Category: "python", Tags: []string{"pip", "install"}},
		{Name: "pytest", Command: "pytest -v", Description: "Run pytest with verbose output", Category: "python", Tags: []string{"python", "test"}},
		{Name: "uv-sync", Command: "uv sync", Description: "Sync dependencies with uv", Category: "python", Tags: []string{"uv", "sync"}},

		// Go templates
		{Name: "go-build", Command: "go build ./...", Description: "Build Go project", Category: "go", Tags: []string{"go", "build"}},
		{Name: "go-test", Command: "go test -v ./...", Description: "Run Go tests", Category: "go", Tags: []string{"go", "test"}},
		{Name: "go-mod-tidy", Command: "go mod tidy", Description: "Tidy Go modules", Category: "go", Tags: []string{"go", "mod"}},
		{Name: "go-vet", Command: "go vet ./...", Description: "Run Go vet", Category: "go", Tags: []string{"go", "vet"}},

		// Git templates
		{Name: "git-status", Command: "git status", Description: "Show git status", Category: "git", Tags: []string{"git", "status"}},
		{Name: "git-log", Command: "git log --oneline -10", Description: "Show last 10 commits", Category: "git", Tags: []string{"git", "log"}},
		{Name: "git-pull", Command: "git pull origin main", Description: "Pull from main branch", Category: "git", Tags: []string{"git", "pull"}},
		{Name: "git-push", Command: "git push origin HEAD", Description: "Push current branch", Category: "git", Tags: []string{"git", "push"}},

		// Docker templates
		{Name: "docker-ps", Command: "docker ps", Description: "List running containers", Category: "docker", Tags: []string{"docker", "ps"}},
		{Name: "docker-build", Command: "docker build -t {{name}} .", Description: "Build Docker image", Category: "docker", Tags: []string{"docker", "build"}, Variables: map[string]string{"name": "myapp"}},
		{Name: "docker-compose-up", Command: "docker-compose up -d", Description: "Start docker-compose services", Category: "docker", Tags: []string{"docker", "compose"}},

		// System templates
		{Name: "disk-usage", Command: "df -h", Description: "Show disk usage", Category: "system", Tags: []string{"system", "disk"}},
		{Name: "find-large", Command: "find . -type f -size +100M", Description: "Find files larger than 100MB", Category: "system", Tags: []string{"system", "find"}},
		{Name: "port-check", Command: "lsof -i :{{port}}", Description: "Check what's using a port", Category: "system", Tags: []string{"system", "port"}, Variables: map[string]string{"port": "3000"}},
	}

	for _, t := range defaults {
		t.CreatedAt = time.Now()
		tm.templates[t.Name] = t
	}
}

// AddTemplate adds a new command template
func (tm *TemplateManager) AddTemplate(template *CommandTemplate) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if template.Name == "" {
		return fmt.Errorf("template name cannot be empty")
	}
	if template.Command == "" {
		return fmt.Errorf("template command cannot be empty")
	}

	template.CreatedAt = time.Now()
	tm.templates[template.Name] = template
	return nil
}

// GetTemplate retrieves a template by name
func (tm *TemplateManager) GetTemplate(name string) (*CommandTemplate, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	t, exists := tm.templates[name]
	return t, exists
}

// ListTemplates returns all templates, optionally filtered by category
func (tm *TemplateManager) ListTemplates(category string) []*CommandTemplate {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	var result []*CommandTemplate
	for _, t := range tm.templates {
		if category == "" || t.Category == category {
			result = append(result, t)
		}
	}
	return result
}

// DeleteTemplate removes a template
func (tm *TemplateManager) DeleteTemplate(name string) bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if _, exists := tm.templates[name]; exists {
		delete(tm.templates, name)
		return true
	}
	return false
}

// ExpandTemplate expands a template with given variables
func (tm *TemplateManager) ExpandTemplate(name string, variables map[string]string) (string, error) {
	tm.mu.RLock()
	t, exists := tm.templates[name]
	tm.mu.RUnlock()

	if !exists {
		return "", fmt.Errorf("template '%s' not found", name)
	}

	// Start with the template command
	cmd := t.Command

	// Merge default variables with provided ones
	vars := make(map[string]string)
	for k, v := range t.Variables {
		vars[k] = v
	}
	for k, v := range variables {
		vars[k] = v
	}

	// Expand {{variable}} patterns
	for key, value := range vars {
		placeholder := "{{" + key + "}}"
		cmd = strings.ReplaceAll(cmd, placeholder, value)
	}

	return cmd, nil
}

// =============================================================================
// F1: Template Tool Handlers
// =============================================================================

// ListTemplatesArgs represents arguments for listing templates
type ListTemplatesArgs struct {
	Category string `json:"category,omitempty" jsonschema:"description=Filter templates by category (nodejs/python/go/git/docker/system)"`
}

// ListTemplatesResult represents the result of listing templates
type ListTemplatesResult struct {
	Templates  []*CommandTemplate `json:"templates"`
	Count      int                `json:"count"`
	Categories []string           `json:"categories"`
}

// AddTemplateArgs represents arguments for adding a template
type AddTemplateArgs struct {
	Name        string            `json:"name" jsonschema:"required,description=Unique name for the template"`
	Command     string            `json:"command" jsonschema:"required,description=The command to execute"`
	Description string            `json:"description,omitempty" jsonschema:"description=Description of what the template does"`
	Category    string            `json:"category,omitempty" jsonschema:"description=Category for the template"`
	Variables   map[string]string `json:"variables,omitempty" jsonschema:"description=Variable placeholders with default values"`
}

// ExecuteTemplateArgs represents arguments for executing a template
type ExecuteTemplateArgs struct {
	SessionID    string            `json:"session_id" jsonschema:"required,description=Session ID to run the template in"`
	TemplateName string            `json:"template_name" jsonschema:"required,description=Name of the template to execute"`
	Variables    map[string]string `json:"variables,omitempty" jsonschema:"description=Variable values to substitute in the template"`
	Timeout      int               `json:"timeout,omitempty" jsonschema:"description=Timeout in seconds"`
}

// ListCommandTemplates lists all available command templates
func (t *TerminalTools) ListCommandTemplates(ctx context.Context, req *mcp.CallToolRequest, args ListTemplatesArgs) (*mcp.CallToolResult, ListTemplatesResult, error) {
	templates := t.templateManager.ListTemplates(args.Category)

	// Collect unique categories
	categorySet := make(map[string]bool)
	for _, tmpl := range templates {
		categorySet[tmpl.Category] = true
	}
	var categories []string
	for cat := range categorySet {
		categories = append(categories, cat)
	}

	result := ListTemplatesResult{
		Templates:  templates,
		Count:      len(templates),
		Categories: categories,
	}

	return createJSONResult(result), result, nil
}

// AddCommandTemplate adds a new command template
func (t *TerminalTools) AddCommandTemplate(ctx context.Context, req *mcp.CallToolRequest, args AddTemplateArgs) (*mcp.CallToolResult, map[string]interface{}, error) {
	template := &CommandTemplate{
		Name:        args.Name,
		Command:     args.Command,
		Description: args.Description,
		Category:    args.Category,
		Variables:   args.Variables,
	}

	if err := t.templateManager.AddTemplate(template); err != nil {
		return createErrorResult(err.Error()), nil, nil
	}

	result := map[string]interface{}{
		"success":  true,
		"message":  fmt.Sprintf("Template '%s' added successfully", args.Name),
		"template": template,
	}

	return createJSONResult(result), result, nil
}

// ExecuteCommandTemplate executes a command from a template
func (t *TerminalTools) ExecuteCommandTemplate(ctx context.Context, req *mcp.CallToolRequest, args ExecuteTemplateArgs) (*mcp.CallToolResult, RunCommandResult, error) {
	// Expand the template
	command, err := t.templateManager.ExpandTemplate(args.TemplateName, args.Variables)
	if err != nil {
		return createErrorResult(err.Error()), RunCommandResult{}, nil
	}

	// Execute the expanded command using RunCommand
	return t.RunCommand(ctx, req, RunCommandArgs{
		SessionID: args.SessionID,
		Command:   command,
		Timeout:   args.Timeout,
	})
}
