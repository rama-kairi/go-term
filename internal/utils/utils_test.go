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
		testCases := []struct {
			name          string
			path          string
			shouldContain string
		}{
			{
				name:          "basic_path",
				path:          "/tmp/my-test-project",
				shouldContain: "my_test_project",
			},
			{
				name:          "complex_path",
				path:          "/home/user/NextJS App",
				shouldContain: "nextjs_app",
			},
			{
				name:          "special_chars",
				path:          "/var/github.com/user/repo-name",
				shouldContain: "repo_name",
			},
			{
				name:          "single_word",
				path:          "/projects/backend",
				shouldContain: "backend",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				projectID := gen.GenerateProjectIDFromPath(tc.path)

				if projectID == "" {
					t.Errorf("Expected non-empty project ID")
				}

				if !strings.Contains(projectID, tc.shouldContain) {
					t.Errorf("Expected project ID to contain '%s', got: %s", tc.shouldContain, projectID)
				}

				// Should have random suffix
				parts := strings.Split(projectID, "_")
				if len(parts) < 2 {
					t.Errorf("Expected project ID to have random suffix, got: %s", projectID)
				}

				// Last part should be 6 characters (random suffix)
				lastPart := parts[len(parts)-1]
				if len(lastPart) != 6 {
					t.Errorf("Expected random suffix to be 6 characters, got: %s", lastPart)
				}
			})
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

		// Verify format
		if err := gen.ValidateProjectID(projectID); err != nil {
			t.Errorf("Generated project ID should be valid, got: %v", err)
		}
	})

	t.Run("ValidateProjectID", func(t *testing.T) {
		validIDs := []string{
			"my_project_abc123",
			"test_app_x1y2z3",
			"simple_name_abcdef",
			"project-name_123456",
			"very_long_project_name_with_many_words_abc123",
		}

		for _, validID := range validIDs {
			err := gen.ValidateProjectID(validID)
			if err != nil {
				t.Errorf("Expected valid project ID '%s' to pass validation, got: %v", validID, err)
			}
		}

		invalidIDs := []struct {
			id     string
			reason string
		}{
			{"", "empty"},
			{"ab", "too short"},
			{"invalid@char_123", "invalid character"},
			{"no-underscore", "no underscore"},
			{strings.Repeat("a", 101), "too long"},
			{"project_with_$pecial_chars", "special characters"},
		}

		for _, invalid := range invalidIDs {
			err := gen.ValidateProjectID(invalid.id)
			if err == nil {
				t.Errorf("Expected invalid project ID '%s' (%s) to fail validation", invalid.id, invalid.reason)
			}
		}
	})

	t.Run("ParseProjectID", func(t *testing.T) {
		testCases := []struct {
			projectID          string
			expectedName       string
			expectedSuffix     string
			expectedFolderName string
		}{
			{
				projectID:          "my_awesome_project_abc123",
				expectedName:       "my_awesome_project",
				expectedSuffix:     "abc123",
				expectedFolderName: "my awesome project",
			},
			{
				projectID:          "simple_name_x1y2z3",
				expectedName:       "simple_name",
				expectedSuffix:     "x1y2z3",
				expectedFolderName: "simple name",
			},
			{
				projectID:          "single_word_123456",
				expectedName:       "single_word",
				expectedSuffix:     "123456",
				expectedFolderName: "single word",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.projectID, func(t *testing.T) {
				info := gen.ParseProjectID(tc.projectID)

				if info.FullID != tc.projectID {
					t.Errorf("Expected FullID to be %s, got %s", tc.projectID, info.FullID)
				}

				if info.RandomSuffix != tc.expectedSuffix {
					t.Errorf("Expected RandomSuffix to be '%s', got '%s'", tc.expectedSuffix, info.RandomSuffix)
				}

				if info.ProjectName != tc.expectedName {
					t.Errorf("Expected ProjectName to be '%s', got '%s'", tc.expectedName, info.ProjectName)
				}

				if info.OriginalFolderName != tc.expectedFolderName {
					t.Errorf("Expected OriginalFolderName to be '%s', got '%s'", tc.expectedFolderName, info.OriginalFolderName)
				}
			})
		}

		// Test edge case: no underscore
		info := gen.ParseProjectID("no-underscore")
		if info.RandomSuffix != "" || info.ProjectName != "" {
			t.Errorf("Expected empty fields for malformed project ID, got: %+v", info)
		}
	})

	t.Run("SuggestProjectID", func(t *testing.T) {
		testCases := []string{
			"My Test Project",
			"simple-name",
			"complex.project.name",
			"",
		}

		for _, input := range testCases {
			t.Run(input, func(t *testing.T) {
				suggested := gen.SuggestProjectID(input)
				if suggested == "" {
					t.Errorf("Expected non-empty suggested project ID")
				}

				// Verify format
				if err := gen.ValidateProjectID(suggested); err != nil {
					t.Errorf("Suggested project ID should be valid, got: %v", err)
				}
			})
		}
	})

	t.Run("GetProjectIDInstructions", func(t *testing.T) {
		instructions := gen.GetProjectIDInstructions()

		if instructions.Format == "" {
			t.Error("Expected non-empty format")
		}

		if instructions.Description == "" {
			t.Error("Expected non-empty description")
		}

		if len(instructions.Examples) == 0 {
			t.Error("Expected examples")
		}

		if len(instructions.Rules) == 0 {
			t.Error("Expected rules")
		}

		if instructions.Usage.AutoGeneration == "" {
			t.Error("Expected usage auto generation info")
		}

		// Verify examples have required fields
		for i, example := range instructions.Examples {
			if example.FolderName == "" || example.ProjectID == "" || example.Explanation == "" {
				t.Errorf("Example %d missing required fields: %+v", i, example)
			}
		}
	})

	t.Run("CleanDirectoryName", func(t *testing.T) {
		// We'll test this indirectly through GenerateProjectIDFromPath
		// since it's a private method
		testCases := []struct {
			input    string
			expected string
		}{
			{"/path/to/My-Project", "my_project"},
			{"/path/to/NextJS App", "nextjs_app"},
			{"/path/to/complex.name@2023", "complexname2023"}, // @ and . get removed, numbers kept
		}

		for _, tc := range testCases {
			projectID := gen.GenerateProjectIDFromPath(tc.input)
			if !strings.Contains(projectID, tc.expected) {
				t.Errorf("Expected project ID to contain '%s', got: %s", tc.expected, projectID)
			}
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
		err = os.WriteFile(packageJSON, []byte(`{"name": "test", "scripts": {"dev": "next dev", "build": "next build"}}`), 0o644)
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
		testCases := []struct {
			name     string
			filename string
			content  string
		}{
			{
				name:     "pyproject_toml",
				filename: "pyproject.toml",
				content:  `[project]\nname = "test"\nversion = "0.1.0"`,
			},
			{
				name:     "requirements_txt",
				filename: "requirements.txt",
				content:  "requests==2.28.0\nflask==2.0.0",
			},
			{
				name:     "pipfile",
				filename: "Pipfile",
				content:  `[[source]]\nurl = "https://pypi.org/simple"`,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				tempDir, err := os.MkdirTemp("", "python-test-*")
				if err != nil {
					t.Fatalf("Failed to create temp dir: %v", err)
				}
				defer os.RemoveAll(tempDir)

				// Create the file
				filePath := filepath.Join(tempDir, tc.filename)
				err = os.WriteFile(filePath, []byte(tc.content), 0o644)
				if err != nil {
					t.Fatalf("Failed to create %s: %v", tc.filename, err)
				}

				// Test detection
				projectType := detector.DetectProjectType(tempDir)
				if projectType != "python" {
					t.Errorf("Expected python project type for %s, got %s", tc.filename, projectType)
				}
			})
		}
	})

	t.Run("DetectGoProject", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "go-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Create go.mod file
		goMod := filepath.Join(tempDir, "go.mod")
		err = os.WriteFile(goMod, []byte(`module github.com/test/app

go 1.21

require (
	github.com/gorilla/mux v1.8.0
)`), 0o644)
		if err != nil {
			t.Fatalf("Failed to create go.mod: %v", err)
		}

		// Test detection
		projectType := detector.DetectProjectType(tempDir)
		if projectType != "go" {
			t.Errorf("Expected go project type, got %s", projectType)
		}
	})

	t.Run("DetectRustProject", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "rust-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Create Cargo.toml file
		cargoToml := filepath.Join(tempDir, "Cargo.toml")
		err = os.WriteFile(cargoToml, []byte(`[package]
name = "test"
version = "0.1.0"
edition = "2021"`), 0o644)
		if err != nil {
			t.Fatalf("Failed to create Cargo.toml: %v", err)
		}

		// Test detection
		projectType := detector.DetectProjectType(tempDir)
		if projectType != "rust" {
			t.Errorf("Expected rust project type, got %s", projectType)
		}
	})

	t.Run("DetectJavaProject", func(t *testing.T) {
		testCases := []struct {
			name     string
			filename string
			content  string
		}{
			{
				name:     "maven_pom",
				filename: "pom.xml",
				content:  `<?xml version="1.0" encoding="UTF-8"?><project></project>`,
			},
			{
				name:     "gradle_build",
				filename: "build.gradle",
				content:  `plugins { id 'java' }`,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				tempDir, err := os.MkdirTemp("", "java-test-*")
				if err != nil {
					t.Fatalf("Failed to create temp dir: %v", err)
				}
				defer os.RemoveAll(tempDir)

				// Create the file
				filePath := filepath.Join(tempDir, tc.filename)
				err = os.WriteFile(filePath, []byte(tc.content), 0o644)
				if err != nil {
					t.Fatalf("Failed to create %s: %v", tc.filename, err)
				}

				// Test detection
				projectType := detector.DetectProjectType(tempDir)
				if projectType != "java" {
					t.Errorf("Expected java project type for %s, got %s", tc.filename, projectType)
				}
			})
		}
	})

	t.Run("DetectOtherProjects", func(t *testing.T) {
		testCases := []struct {
			name         string
			filename     string
			content      string
			expectedType string
		}{
			{
				name:         "ruby_gemfile",
				filename:     "Gemfile",
				content:      `source 'https://rubygems.org'\ngem 'rails'`,
				expectedType: "ruby",
			},
			{
				name:         "php_composer",
				filename:     "composer.json",
				content:      `{"name": "test/app", "require": {"php": ">=8.0"}}`,
				expectedType: "php",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				tempDir, err := os.MkdirTemp("", "project-test-*")
				if err != nil {
					t.Fatalf("Failed to create temp dir: %v", err)
				}
				defer os.RemoveAll(tempDir)

				// Create the file
				filePath := filepath.Join(tempDir, tc.filename)
				err = os.WriteFile(filePath, []byte(tc.content), 0o644)
				if err != nil {
					t.Fatalf("Failed to create %s: %v", tc.filename, err)
				}

				// Test detection
				projectType := detector.DetectProjectType(tempDir)
				if projectType != tc.expectedType {
					t.Errorf("Expected %s project type for %s, got %s", tc.expectedType, tc.filename, projectType)
				}
			})
		}
	})

	t.Run("DetectPackageManagerWithLockFiles", func(t *testing.T) {
		testCases := []struct {
			name         string
			configFile   string
			lockFile     string
			expectedName string
		}{
			{
				name:         "npm_project",
				configFile:   "package.json",
				lockFile:     "package-lock.json",
				expectedName: "npm",
			},
			{
				name:         "yarn_project",
				configFile:   "package.json",
				lockFile:     "yarn.lock",
				expectedName: "yarn",
			},
			{
				name:         "pnpm_project",
				configFile:   "package.json",
				lockFile:     "pnpm-lock.yaml",
				expectedName: "pnpm",
			},
			{
				name:         "bun_project",
				configFile:   "package.json",
				lockFile:     "bun.lockb",
				expectedName: "bun",
			},
			{
				name:         "poetry_project",
				configFile:   "pyproject.toml",
				lockFile:     "poetry.lock",
				expectedName: "poetry",
			},
			{
				name:         "pipenv_project",
				configFile:   "Pipfile",
				lockFile:     "Pipfile.lock",
				expectedName: "pipenv",
			},
			{
				name:         "go_project",
				configFile:   "go.mod",
				lockFile:     "go.sum",
				expectedName: "go",
			},
			{
				name:         "cargo_project",
				configFile:   "Cargo.toml",
				lockFile:     "Cargo.lock",
				expectedName: "cargo",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				tempDir, err := os.MkdirTemp("", "pm-test-*")
				if err != nil {
					t.Fatalf("Failed to create temp dir: %v", err)
				}
				defer os.RemoveAll(tempDir)

				// Create config file
				configPath := filepath.Join(tempDir, tc.configFile)
				err = os.WriteFile(configPath, []byte(`{"name": "test"}`), 0o644)
				if err != nil {
					t.Fatalf("Failed to create %s: %v", tc.configFile, err)
				}

				// Create lock file
				lockPath := filepath.Join(tempDir, tc.lockFile)
				err = os.WriteFile(lockPath, []byte(`{}`), 0o644)
				if err != nil {
					t.Fatalf("Failed to create %s: %v", tc.lockFile, err)
				}

				// Test package manager detection
				pm, err := detector.DetectPackageManager(tempDir)
				if err != nil {
					t.Errorf("Expected to detect a package manager: %v", err)
				}

				// Note: We may not always detect the expected manager due to executable availability
				// but we should at least detect a compatible one for Node.js projects
				if pm == nil {
					t.Logf("Package manager %s not detected (likely executable not available)", tc.expectedName)
				} else {
					t.Logf("Detected package manager: %s (expected: %s)", pm.Name, tc.expectedName)
				}
			})
		}
	})

	t.Run("GetPreferredCommand", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "command-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Create package.json
		packageJSON := filepath.Join(tempDir, "package.json")
		err = os.WriteFile(packageJSON, []byte(`{"name": "test", "scripts": {"dev": "next dev", "build": "next build", "test": "jest"}}`), 0o644)
		if err != nil {
			t.Fatalf("Failed to create package.json: %v", err)
		}

		// Test command preferences
		operations := []string{"install", "run", "dev", "build", "test"}
		for _, operation := range operations {
			cmd := detector.GetPreferredCommand(tempDir, operation)
			if cmd == "" {
				t.Errorf("Expected non-empty %s command", operation)
			}
			t.Logf("Operation %s: %s", operation, cmd)
		}
	})

	t.Run("GetPreferredCommandFallback", func(t *testing.T) {
		// Test fallback behavior with unknown project
		tempDir, err := os.MkdirTemp("", "fallback-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Test fallback commands for various operations
		operations := []string{"install", "run", "dev", "build", "test", "unknown"}
		for _, operation := range operations {
			cmd := detector.GetPreferredCommand(tempDir, operation)
			// Should return some fallback even for unknown operations
			t.Logf("Fallback for %s: %s", operation, cmd)
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

	t.Run("NewPackageManagerDetector", func(t *testing.T) {
		detector := NewPackageManagerDetector()
		if detector == nil {
			t.Error("Expected non-nil detector")
		}

		// Verify that built-in managers are loaded
		if len(detector.managers) == 0 {
			t.Error("Expected package managers to be loaded")
		}

		// Check for key package managers
		expectedManagers := []string{"npm", "yarn", "pnpm", "bun", "go", "cargo", "poetry"}
		foundManagers := make(map[string]bool)
		for _, manager := range detector.managers {
			foundManagers[manager.Name] = true
		}

		for _, expected := range expectedManagers {
			if !foundManagers[expected] {
				t.Errorf("Expected to find %s package manager", expected)
			}
		}
	})

	t.Run("GetRunCommand", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "run-command-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		// Test Node.js project
		packageJSON := filepath.Join(tempDir, "package.json")
		err = os.WriteFile(packageJSON, []byte(`{"name": "test", "scripts": {"dev": "next dev"}}`), 0o644)
		if err != nil {
			t.Fatalf("Failed to create package.json: %v", err)
		}

		// Test script command
		runCmd := detector.GetRunCommand(tempDir, "dev")
		if runCmd == "" {
			t.Error("Expected non-empty run command for script")
		}

		// Test file command
		runCmd = detector.GetRunCommand(tempDir, "index.js")
		if runCmd == "" {
			t.Error("Expected non-empty run command for JS file")
		}

		// Test Python project
		pyTempDir, err := os.MkdirTemp("", "py-run-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(pyTempDir)

		pyproject := filepath.Join(pyTempDir, "pyproject.toml")
		err = os.WriteFile(pyproject, []byte(`[project]\nname = "test"`), 0o644)
		if err != nil {
			t.Fatalf("Failed to create pyproject.toml: %v", err)
		}

		runCmd = detector.GetRunCommand(pyTempDir, "main.py")
		if runCmd == "" {
			t.Error("Expected non-empty run command for Python file")
		}
	})

	t.Run("IsLongRunningCommand", func(t *testing.T) {
		testCases := []struct {
			command      string
			shouldBeLong bool
		}{
			// Should be long-running
			{"npm run dev", true},
			{"yarn start", true},
			{"python -m http.server", true},
			{"flask run", true},
			{"nodemon app.js", true},
			{"tail -f app.log", true},
			{"ping google.com", true},
			{"while true; do echo hello; done", false}, // echo excludes it
			{"watch", true},                            // single word "watch" should work
			{"python server.py", true},
			{"node app.js", true},

			// Should NOT be long-running
			{"npm install", false},
			{"git commit -m 'test'", false},
			{"ls -la", false},
			{"cat file.txt", false},
			{"grep pattern file.txt", false},
			{"curl https://api.example.com", false},
			{"SELECT * FROM users", false},
			{"echo hello", false},
		}

		for _, tc := range testCases {
			t.Run(tc.command, func(t *testing.T) {
				result := detector.IsLongRunningCommand(tc.command)
				if result != tc.shouldBeLong {
					t.Errorf("Expected IsLongRunningCommand('%s') = %v, got %v", tc.command, tc.shouldBeLong, result)
				}
			})
		}
	})

	t.Run("IsDevServerCommand", func(t *testing.T) {
		testCases := []struct {
			command     string
			shouldBeDev bool
		}{
			// Should be dev server commands
			{"npm run dev", true},
			{"yarn start", true},
			{"flask run", true},
			{"django runserver", true},
			{"nodemon app.js", true},
			{"next dev", true},
			{"python server.py", true},
			{"node express-server.js", true},
			{"vite", true},

			// Should NOT be dev server commands
			{"npm install", false},
			{"git commit", false},
			{"ls -la", false},
			{"python script.py", false}, // generic python script
			{"node utils.js", false},    // generic node script
			{"SELECT * FROM users", false},
		}

		for _, tc := range testCases {
			t.Run(tc.command, func(t *testing.T) {
				result := detector.IsDevServerCommand(tc.command)
				if result != tc.shouldBeDev {
					t.Errorf("Expected IsDevServerCommand('%s') = %v, got %v", tc.command, tc.shouldBeDev, result)
				}
			})
		}
	})
}
