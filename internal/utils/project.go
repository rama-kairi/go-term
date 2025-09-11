package utils

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ProjectIDGenerator provides functionality to generate consistent project IDs
type ProjectIDGenerator struct{}

// NewProjectIDGenerator creates a new project ID generator
func NewProjectIDGenerator() *ProjectIDGenerator {
	return &ProjectIDGenerator{}
}

// GenerateProjectID generates a project ID based on the current working directory
// Format: folder_name_with_underscores_RANDOM
// Example: my_awesome_project_a7b3c9
func (p *ProjectIDGenerator) GenerateProjectID() (string, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}

	return p.GenerateProjectIDFromPath(currentDir), nil
}

// GenerateProjectIDFromPath generates a project ID from a given path
func (p *ProjectIDGenerator) GenerateProjectIDFromPath(path string) string {
	// Get the directory name
	dirName := filepath.Base(path)

	// Clean and normalize the directory name
	cleanName := p.cleanDirectoryName(dirName)

	// Generate random suffix
	randomSuffix := p.generateRandomSuffix(6)

	return fmt.Sprintf("%s_%s", cleanName, randomSuffix)
}

// ParseProjectID extracts information from a project ID
func (p *ProjectIDGenerator) ParseProjectID(projectID string) ProjectIDInfo {
	parts := strings.Split(projectID, "_")

	info := ProjectIDInfo{
		FullID: projectID,
	}

	if len(parts) >= 2 {
		// Last part is the random suffix
		info.RandomSuffix = parts[len(parts)-1]

		// Everything before the last part is the project name
		info.ProjectName = strings.Join(parts[:len(parts)-1], "_")

		// Try to extract the original folder name
		info.OriginalFolderName = p.reconstructFolderName(info.ProjectName)
	}

	return info
}

// ValidateProjectID validates if a project ID follows the expected format
func (p *ProjectIDGenerator) ValidateProjectID(projectID string) error {
	if projectID == "" {
		return fmt.Errorf("project ID cannot be empty")
	}

	// Check overall format: alphanumeric, underscores, and hyphens only
	validFormat := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validFormat.MatchString(projectID) {
		return fmt.Errorf("project ID contains invalid characters (only letters, numbers, underscores, and hyphens allowed)")
	}

	// Must contain at least one underscore (to separate name from random suffix)
	if !strings.Contains(projectID, "_") {
		return fmt.Errorf("project ID must contain at least one underscore")
	}

	// Check length (should be reasonable)
	if len(projectID) < 3 || len(projectID) > 100 {
		return fmt.Errorf("project ID length must be between 3 and 100 characters")
	}

	return nil
}

// SuggestProjectID suggests a project ID for a given directory or project name
func (p *ProjectIDGenerator) SuggestProjectID(input string) string {
	if input == "" {
		// Use current directory if no input provided
		if projectID, err := p.GenerateProjectID(); err == nil {
			return projectID
		}
		return "default_project_" + p.generateRandomSuffix(6)
	}

	// Clean the input and generate project ID
	cleanName := p.cleanDirectoryName(input)
	randomSuffix := p.generateRandomSuffix(6)

	return fmt.Sprintf("%s_%s", cleanName, randomSuffix)
}

// GetProjectIDInstructions returns detailed instructions on how to use project IDs
func (p *ProjectIDGenerator) GetProjectIDInstructions() ProjectIDInstructions {
	return ProjectIDInstructions{
		Format:      "folder_name_with_underscores_RANDOM",
		Description: "Project IDs are automatically generated based on your current working directory with a random suffix for uniqueness",
		Examples: []ProjectIDExample{
			{
				FolderName:  "my-awesome-project",
				ProjectID:   "my_awesome_project_a7b3c9",
				Explanation: "Hyphens converted to underscores, random 6-character suffix added",
			},
			{
				FolderName:  "NextJS App",
				ProjectID:   "nextjs_app_x4m8n2",
				Explanation: "Spaces converted to underscores, case normalized, random suffix added",
			},
			{
				FolderName:  "github.com/rama-kairi/go-term",
				ProjectID:   "terminal_mcp_k9p5q1",
				Explanation: "Hyphens converted to underscores, case normalized, random suffix added",
			},
		},
		Rules: []string{
			"Project IDs are automatically generated when creating sessions",
			"Based on the current working directory name",
			"Special characters are converted to underscores",
			"Random 6-character suffix ensures uniqueness",
			"Case is normalized to lowercase",
			"Only letters, numbers, underscores, and hyphens are allowed",
			"Length must be between 3 and 100 characters",
		},
		Usage: ProjectIDUsage{
			AutoGeneration:      "Project IDs are automatically generated when you create a new terminal session. The system uses your current working directory to create a meaningful project ID.",
			ManualSpecification: "You can specify a custom project ID when creating a session, but it must follow the format rules.",
			Consistency:         "Use the same project ID across related sessions to group them together. The search tool can filter by project ID to find related commands.",
			BestPractices: []string{
				"Let the system auto-generate project IDs for consistency",
				"Use descriptive folder names for better project IDs",
				"Group related work in the same directory/project",
				"Use the search tool to find commands across project sessions",
			},
		},
	}
}

// cleanDirectoryName cleans and normalizes a directory name for use in project IDs
func (p *ProjectIDGenerator) cleanDirectoryName(dirName string) string {
	// Convert to lowercase
	clean := strings.ToLower(dirName)

	// Replace spaces and hyphens with underscores
	clean = strings.ReplaceAll(clean, " ", "_")
	clean = strings.ReplaceAll(clean, "-", "_")

	// Remove special characters except underscores and alphanumeric
	reg := regexp.MustCompile(`[^a-z0-9_]`)
	clean = reg.ReplaceAllString(clean, "")

	// Remove multiple consecutive underscores
	reg = regexp.MustCompile(`_+`)
	clean = reg.ReplaceAllString(clean, "_")

	// Trim underscores from start and end
	clean = strings.Trim(clean, "_")

	// Ensure it's not empty
	if clean == "" {
		clean = "project"
	}

	// Limit length
	if len(clean) > 50 {
		clean = clean[:50]
	}

	return clean
}

// reconstructFolderName attempts to reconstruct the original folder name from the cleaned version
func (p *ProjectIDGenerator) reconstructFolderName(cleanName string) string {
	// This is a best-effort reconstruction
	// Replace underscores with spaces for readability
	return strings.ReplaceAll(cleanName, "_", " ")
}

// generateRandomSuffix generates a random alphanumeric suffix
func (p *ProjectIDGenerator) generateRandomSuffix(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)

	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based generation if crypto/rand fails
		return fmt.Sprintf("%d", os.Getpid()%1000000)[:length]
	}

	for i := range b {
		b[i] = charset[b[i]%byte(len(charset))]
	}

	return string(b)
}

// ProjectIDInfo contains parsed information about a project ID
type ProjectIDInfo struct {
	FullID             string `json:"full_id"`
	ProjectName        string `json:"project_name"`
	RandomSuffix       string `json:"random_suffix"`
	OriginalFolderName string `json:"original_folder_name"`
}

// ProjectIDInstructions provides comprehensive instructions for project ID usage
type ProjectIDInstructions struct {
	Format      string             `json:"format"`
	Description string             `json:"description"`
	Examples    []ProjectIDExample `json:"examples"`
	Rules       []string           `json:"rules"`
	Usage       ProjectIDUsage     `json:"usage"`
}

// ProjectIDExample shows an example of project ID generation
type ProjectIDExample struct {
	FolderName  string `json:"folder_name"`
	ProjectID   string `json:"project_id"`
	Explanation string `json:"explanation"`
}

// ProjectIDUsage explains how to use project IDs effectively
type ProjectIDUsage struct {
	AutoGeneration      string   `json:"auto_generation"`
	ManualSpecification string   `json:"manual_specification"`
	Consistency         string   `json:"consistency"`
	BestPractices       []string `json:"best_practices"`
}
