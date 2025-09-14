package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestProjectIDGenerator tests project ID generation functionality
func TestProjectIDGenerator(t *testing.T) {
	gen := NewProjectIDGenerator()

	t.Run("GenerateProjectIDFromPath", func(t *testing.T) {
		// Test basic generation from path
		projectID := gen.GenerateProjectIDFromPath("/tmp/my-test-project")

		if projectID == "" {
			t.Errorf("Expected non-empty project ID")
		}

		// Should contain underscores instead of hyphens
		if !strings.Contains(projectID, "my_test_project") {
			t.Errorf("Expected project ID to contain normalized folder name, got: %s", projectID)
		}

		// Should have random suffix
		parts := strings.Split(projectID, "_")
		if len(parts) < 2 {
			t.Errorf("Expected project ID to have random suffix, got: %s", projectID)
		}
	})

	t.Run("GenerateProjectID", func(t *testing.T) {
		projectID, err := gen.GenerateProjectID()
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		if projectID == "" {
			t.Errorf("Expected non-empty project ID")
		}
	})

	t.Run("ValidateProjectID", func(t *testing.T) {
		// Valid project ID
		err := gen.ValidateProjectID("my_project_abc123")
		if err != nil {
			t.Errorf("Expected valid project ID to pass validation, got: %v", err)
		}

		// Invalid project IDs
		invalidIDs := []string{
			"",                 // empty
			"ab",               // too short
			"invalid@char_123", // invalid character
		}

		for _, invalidID := range invalidIDs {
			err := gen.ValidateProjectID(invalidID)
			if err == nil {
				t.Errorf("Expected invalid project ID '%s' to fail validation", invalidID)
			}
		}

		// Test the specific case that was failing
		err = gen.ValidateProjectID("no_underscore_suffix")
		if err != nil {
			t.Errorf("Expected 'no_underscore_suffix' to be valid since it has underscores, got: %v", err)
		}
	})

	t.Run("ParseProjectID", func(t *testing.T) {
		testID := "my_awesome_project_abc123"
		info := gen.ParseProjectID(testID)

		if info.FullID != testID {
			t.Errorf("Expected FullID to be %s, got %s", testID, info.FullID)
		}

		if info.RandomSuffix != "abc123" {
			t.Errorf("Expected RandomSuffix to be 'abc123', got %s", info.RandomSuffix)
		}

		if info.ProjectName != "my_awesome_project" {
			t.Errorf("Expected ProjectName to be 'my_awesome_project', got %s", info.ProjectName)
		}
	})

	t.Run("SuggestProjectID", func(t *testing.T) {
		suggested := gen.SuggestProjectID("My Test Project")
		if suggested == "" {
			t.Errorf("Expected non-empty suggested project ID")
		}

		// Should contain normalized name
		if !strings.Contains(suggested, "_") {
			t.Errorf("Expected suggested ID to contain underscores, got: %s", suggested)
		}
	})
}

// TestPackageManagerDetector tests package manager detection
func TestPackageManagerDetector(t *testing.T) {
	detector := NewPackageManagerDetector()

	t.Run("DetectNodejsProject", func(t *testing.T) {
		// Create a temporary directory with package.json
		tempDir, err := os.MkdirTemp("", "package-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Create package.json file
		packageJSON := filepath.Join(tempDir, "package.json")
		err = os.WriteFile(packageJSON, []byte(`{"name": "test"}`), 0o644)
		if err != nil {
			t.Fatalf("Failed to create package.json: %v", err)
		}

		// Test detection
		projectType := detector.DetectProjectType(tempDir)
		if projectType != "nodejs" {
			t.Errorf("Expected nodejs project type, got %s", projectType)
		}
	})

	t.Run("DetectPythonProject", func(t *testing.T) {
		// Create a temporary directory with pyproject.toml
		tempDir, err := os.MkdirTemp("", "python-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Create pyproject.toml file
		pyprojectToml := filepath.Join(tempDir, "pyproject.toml")
		err = os.WriteFile(pyprojectToml, []byte(`[project]
name = "test"`), 0o644)
		if err != nil {
			t.Fatalf("Failed to create pyproject.toml: %v", err)
		}

		// Test detection
		projectType := detector.DetectProjectType(tempDir)
		if projectType != "python" {
			t.Errorf("Expected python project type, got %s", projectType)
		}
	})

	t.Run("DetectPackageManager", func(t *testing.T) {
		// Create a temporary directory with npm lock file
		tempDir, err := os.MkdirTemp("", "npm-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Create package.json and package-lock.json
		packageJSON := filepath.Join(tempDir, "package.json")
		err = os.WriteFile(packageJSON, []byte(`{"name": "test"}`), 0o644)
		if err != nil {
			t.Fatalf("Failed to create package.json: %v", err)
		}

		lockFile := filepath.Join(tempDir, "package-lock.json")
		err = os.WriteFile(lockFile, []byte(`{}`), 0o644)
		if err != nil {
			t.Fatalf("Failed to create package-lock.json: %v", err)
		}

		// Test package manager detection
		pm, err := detector.DetectPackageManager(tempDir)
		if err != nil {
			t.Errorf("Expected to detect a package manager: %v", err)
		}

		if pm == nil {
			t.Errorf("Expected package manager but got nil")
		}

		if pm != nil && (pm.Name == "npm" || pm.Name == "bun" || pm.Name == "yarn" || pm.Name == "pnpm") {
			// Any Node.js package manager is acceptable
		} else {
			t.Errorf("Expected Node.js package manager, got %v", pm)
		}
	})

	t.Run("GetPreferredCommand", func(t *testing.T) {
		// Create a temporary directory with npm setup
		tempDir, err := os.MkdirTemp("", "command-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Create package.json
		packageJSON := filepath.Join(tempDir, "package.json")
		err = os.WriteFile(packageJSON, []byte(`{"name": "test"}`), 0o644)
		if err != nil {
			t.Fatalf("Failed to create package.json: %v", err)
		}

		// Test command preferences
		installCmd := detector.GetPreferredCommand(tempDir, "install")
		if installCmd == "" {
			t.Errorf("Expected non-empty install command")
		}

		runCmd := detector.GetPreferredCommand(tempDir, "run")
		if runCmd == "" {
			t.Errorf("Expected non-empty run command")
		}
	})

	t.Run("UnknownProject", func(t *testing.T) {
		// Create a temporary directory with no recognizable files
		tempDir, err := os.MkdirTemp("", "unknown-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Test detection on empty directory
		projectType := detector.DetectProjectType(tempDir)
		if projectType != "unknown" {
			t.Errorf("Expected unknown project type, got %s", projectType)
		}

		// Package manager detection should return nil (not an error)
		pm, err := detector.DetectPackageManager(tempDir)
		if err != nil {
			t.Errorf("Unexpected error when detecting package manager in empty directory: %v", err)
		}

		if pm != nil {
			t.Errorf("Expected nil package manager for unknown project type, got: %v", pm)
		}
	})
}
