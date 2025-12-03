package utils

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// PackageManager represents different package managers and their characteristics
type PackageManager struct {
	Name           string
	ExecutableName string
	LockFile       string
	ConfigFile     string
	InstallCommand string
	RunCommand     string
	DevCommand     string
	BuildCommand   string
	TestCommand    string
}

// M5: cachedResult stores a cached package manager detection result
type cachedResult struct {
	manager    *PackageManager
	detectedAt time.Time
}

// PackageManagerDetector handles detection of package managers in projects
type PackageManagerDetector struct {
	managers []PackageManager
	cache    map[string]*cachedResult // M5: Cache by directory path
	cacheTTL time.Duration            // M5: Cache time-to-live
	mu       sync.RWMutex             // M5: Mutex for cache access
}

// NewPackageManagerDetector creates a new package manager detector with caching (M5)
func NewPackageManagerDetector() *PackageManagerDetector {
	return &PackageManagerDetector{
		cache:    make(map[string]*cachedResult),
		cacheTTL: 5 * time.Minute, // Cache results for 5 minutes
		managers: []PackageManager{
			// Node.js package managers (in order of preference)
			{
				Name:           "bun",
				ExecutableName: "bun",
				LockFile:       "bun.lockb",
				ConfigFile:     "package.json",
				InstallCommand: "bun install",
				RunCommand:     "bun run",
				DevCommand:     "bun run dev",
				BuildCommand:   "bun run build",
				TestCommand:    "bun test",
			},
			{
				Name:           "pnpm",
				ExecutableName: "pnpm",
				LockFile:       "pnpm-lock.yaml",
				ConfigFile:     "package.json",
				InstallCommand: "pnpm install",
				RunCommand:     "pnpm run",
				DevCommand:     "pnpm run dev",
				BuildCommand:   "pnpm run build",
				TestCommand:    "pnpm test",
			},
			{
				Name:           "yarn",
				ExecutableName: "yarn",
				LockFile:       "yarn.lock",
				ConfigFile:     "package.json",
				InstallCommand: "yarn install",
				RunCommand:     "yarn run",
				DevCommand:     "yarn dev",
				BuildCommand:   "yarn build",
				TestCommand:    "yarn test",
			},
			{
				Name:           "npm",
				ExecutableName: "npm",
				LockFile:       "package-lock.json",
				ConfigFile:     "package.json",
				InstallCommand: "npm install",
				RunCommand:     "npm run",
				DevCommand:     "npm run dev",
				BuildCommand:   "npm run build",
				TestCommand:    "npm test",
			},
			// Python package managers
			{
				Name:           "uv",
				ExecutableName: "uv",
				LockFile:       "uv.lock",
				ConfigFile:     "pyproject.toml",
				InstallCommand: "uv sync",
				RunCommand:     "uv run",
				DevCommand:     "uv run python",
				BuildCommand:   "uv build",
				TestCommand:    "uv run pytest",
			},
			{
				Name:           "poetry",
				ExecutableName: "poetry",
				LockFile:       "poetry.lock",
				ConfigFile:     "pyproject.toml",
				InstallCommand: "poetry install",
				RunCommand:     "poetry run",
				DevCommand:     "poetry run python",
				BuildCommand:   "poetry build",
				TestCommand:    "poetry run pytest",
			},
			{
				Name:           "pipenv",
				ExecutableName: "pipenv",
				LockFile:       "Pipfile.lock",
				ConfigFile:     "Pipfile",
				InstallCommand: "pipenv install",
				RunCommand:     "pipenv run",
				DevCommand:     "pipenv run python",
				BuildCommand:   "pipenv run build",
				TestCommand:    "pipenv run pytest",
			},
			// Go
			{
				Name:           "go",
				ExecutableName: "go",
				LockFile:       "go.sum",
				ConfigFile:     "go.mod",
				InstallCommand: "go mod download",
				RunCommand:     "go run",
				DevCommand:     "go run .",
				BuildCommand:   "go build",
				TestCommand:    "go test ./...",
			},
			// Rust
			{
				Name:           "cargo",
				ExecutableName: "cargo",
				LockFile:       "Cargo.lock",
				ConfigFile:     "Cargo.toml",
				InstallCommand: "cargo fetch",
				RunCommand:     "cargo run",
				DevCommand:     "cargo run",
				BuildCommand:   "cargo build",
				TestCommand:    "cargo test",
			},
		},
	}
}

// DetectPackageManager detects the appropriate package manager for a directory with caching (M5)
func (d *PackageManagerDetector) DetectPackageManager(workingDir string) (*PackageManager, error) {
	// M5: Check cache first
	d.mu.RLock()
	if cached, ok := d.cache[workingDir]; ok {
		if time.Since(cached.detectedAt) < d.cacheTTL {
			d.mu.RUnlock()
			return cached.manager, nil
		}
	}
	d.mu.RUnlock()

	// First, check for lock files and config files to determine the project type
	var result *PackageManager
	for _, manager := range d.managers {
		// Check if lock file exists
		if manager.LockFile != "" {
			lockPath := filepath.Join(workingDir, manager.LockFile)
			if _, err := os.Stat(lockPath); err == nil {
				// Verify the executable is available
				if d.isExecutableAvailable(manager.ExecutableName) {
					result = &manager
					break
				}
			}
		}

		// Check if config file exists and executable is available
		if manager.ConfigFile != "" {
			configPath := filepath.Join(workingDir, manager.ConfigFile)
			if _, err := os.Stat(configPath); err == nil {
				if d.isExecutableAvailable(manager.ExecutableName) {
					result = &manager
					break
				}
			}
		}
	}

	// M5: Cache the result (including nil)
	d.mu.Lock()
	d.cache[workingDir] = &cachedResult{
		manager:    result,
		detectedAt: time.Now(),
	}
	d.mu.Unlock()

	return result, nil
}

// InvalidateCache clears the cache for a specific directory or all directories (M5)
func (d *PackageManagerDetector) InvalidateCache(workingDir string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if workingDir == "" {
		// Clear entire cache
		d.cache = make(map[string]*cachedResult)
	} else {
		delete(d.cache, workingDir)
	}
}

// DetectProjectType determines the project type based on files in the directory
func (d *PackageManagerDetector) DetectProjectType(workingDir string) string {
	// Check for various project indicators
	indicators := map[string]string{
		"package.json":     "nodejs",
		"pyproject.toml":   "python",
		"requirements.txt": "python",
		"Pipfile":          "python",
		"go.mod":           "go",
		"Cargo.toml":       "rust",
		"pom.xml":          "java",
		"build.gradle":     "java",
		"Gemfile":          "ruby",
		"composer.json":    "php",
	}

	for file, projectType := range indicators {
		if _, err := os.Stat(filepath.Join(workingDir, file)); err == nil {
			return projectType
		}
	}

	return "unknown"
}

// GetPreferredCommand returns the preferred command for a given operation
func (d *PackageManagerDetector) GetPreferredCommand(workingDir, operation string) string {
	manager, err := d.DetectPackageManager(workingDir)
	if err != nil || manager == nil {
		return d.getFallbackCommand(workingDir, operation)
	}

	switch operation {
	case "install":
		return manager.InstallCommand
	case "run":
		return manager.RunCommand
	case "dev":
		return manager.DevCommand
	case "build":
		return manager.BuildCommand
	case "test":
		return manager.TestCommand
	default:
		return manager.RunCommand
	}
}

// GetRunCommand returns the appropriate run command for a script
func (d *PackageManagerDetector) GetRunCommand(workingDir, script string) string {
	manager, err := d.DetectPackageManager(workingDir)
	if err != nil || manager == nil {
		return d.getFallbackRunCommand(workingDir, script)
	}

	// For Node.js projects, check if it's a script in package.json
	if manager.ConfigFile == "package.json" && !strings.HasSuffix(script, ".js") && !strings.HasSuffix(script, ".ts") {
		return manager.RunCommand + " " + script
	}

	// For Python projects with uv
	if manager.Name == "uv" && (strings.HasSuffix(script, ".py") || script == "python") {
		return "uv run " + script
	}

	// For other Python managers
	if manager.ConfigFile == "pyproject.toml" || manager.ConfigFile == "Pipfile" {
		return manager.RunCommand + " " + script
	}

	// Default run command
	return manager.RunCommand + " " + script
}

// isExecutableAvailable checks if an executable is available in PATH
func (d *PackageManagerDetector) isExecutableAvailable(name string) bool {
	// Simple check - in a real implementation, you'd use exec.LookPath
	// For now, we'll assume if the config/lock file exists, the tool is available
	_ = name // Suppress unused parameter warning
	return true
}

// getFallbackCommand returns fallback commands when no package manager is detected
func (d *PackageManagerDetector) getFallbackCommand(workingDir, operation string) string {
	projectType := d.DetectProjectType(workingDir)

	switch projectType {
	case "nodejs":
		switch operation {
		case "install":
			return "npm install"
		case "dev":
			return "npm run dev"
		case "build":
			return "npm run build"
		case "test":
			return "npm test"
		default:
			return "npm run"
		}
	case "python":
		switch operation {
		case "install":
			return "pip install -r requirements.txt"
		case "dev", "run":
			return "python"
		case "test":
			return "python -m pytest"
		default:
			return "python"
		}
	case "go":
		switch operation {
		case "install":
			return "go mod download"
		case "dev", "run":
			return "go run ."
		case "build":
			return "go build"
		case "test":
			return "go test ./..."
		default:
			return "go run"
		}
	default:
		return ""
	}
}

// getFallbackRunCommand returns fallback run commands
func (d *PackageManagerDetector) getFallbackRunCommand(workingDir, script string) string {
	projectType := d.DetectProjectType(workingDir)

	switch projectType {
	case "nodejs":
		if strings.HasSuffix(script, ".js") || strings.HasSuffix(script, ".ts") {
			return "node " + script
		}
		return "npm run " + script
	case "python":
		if strings.HasSuffix(script, ".py") {
			return "python " + script
		}
		return "python " + script
	case "go":
		if strings.HasSuffix(script, ".go") {
			return "go run " + script
		}
		return "go run ."
	default:
		return script
	}
}

// IsLongRunningCommand determines if a command is likely to be long-running
func (d *PackageManagerDetector) IsLongRunningCommand(command string) bool {
	commandLower := strings.ToLower(command)

	// First, exclude commands that should never be background processes
	excludePatterns := []string{
		"sqlite3", "mysql", "psql", "redis-cli", // Database CLIs
		"grep", "find", "ls", "cat", "echo", "sleep", // Basic utilities
		"curl", "wget", "git", // Network/VCS utilities
		"npm install", "yarn install", "pip install", // Package installations
		"SELECT", "INSERT", "UPDATE", "DELETE", // SQL queries
	}

	for _, exclude := range excludePatterns {
		if strings.Contains(commandLower, strings.ToLower(exclude)) {
			return false
		}
	}

	// Then check for long-running patterns with more specific matching
	longRunningPatterns := []string{
		// Development servers
		"npm run dev", "npm start", "npm run serve",
		"yarn dev", "yarn start", "yarn serve",
		"pnpm dev", "pnpm start", "pnpm serve",
		"bun dev", "bun start", "bun serve",
		"python -m http.server", "python -m SimpleHTTPServer",
		"flask run", "django runserver", "uvicorn",
		"nodemon", "webpack-dev-server", "vite",
		"next dev", "nuxt dev", "gatsby develop",

		// Network monitoring and testing
		"ping", "traceroute", "tracert", "netstat -l", "netstat --listen",
		"nmap", "tcpdump", "wireshark",

		// File monitoring
		"tail -f", "tail --follow", "watch", "inotifywait",

		// System monitoring
		"top", "htop", "iostat", "vmstat", "sar",
		"monitor", "stress", "stress-ng",

		// Loop constructs and infinite operations
		"while true", "while (true)", "while(true)", "for(;;)", "for (;;)",
		"infinite", "loop", "continuous", "endless",
	}

	// Check exact patterns first (more specific)
	for _, pattern := range longRunningPatterns {
		if strings.Contains(commandLower, pattern) {
			return true
		}
	}

	// Check for commands with background/continuous flags
	backgroundFlags := []string{
		" -f ", " --follow ", " --watch ", " --continuous ",
		" --daemon ", " --background ", " --persist ", " --loop ",
		" --monitor ", " --tail ", " --stream ", " --live ",
	}

	for _, flag := range backgroundFlags {
		if strings.Contains(commandLower, flag) {
			return true
		}
	}

	// Check for Python scripts that are likely servers or long-running processes
	if strings.Contains(commandLower, "python") {
		serverIndicators := []string{
			"server.py", "http_server.py", "dev_server.py",
			"app.py", "main.py", "run.py", "wsgi.py",
		}

		for _, indicator := range serverIndicators {
			if strings.Contains(commandLower, indicator) {
				return true
			}
		}

		// Check for server-related imports/modules
		serverModules := []string{
			"http.server", "socketserver", "flask", "django", "fastapi", "tornado",
		}

		for _, module := range serverModules {
			if strings.Contains(commandLower, module) {
				return true
			}
		}

		// Check for common long-running script patterns
		longRunningPatterns := []string{
			"hang.py", "loop.py", "daemon.py", "worker.py",
			"monitor.py", "watch.py", "listen.py", "endless.py",
			"infinite.py", "continuous.py", "service.py",
			"background.py", "test.py", "background_test.py",
		}

		for _, pattern := range longRunningPatterns {
			if strings.Contains(commandLower, pattern) {
				return true
			}
		}
	}

	// Check for Node.js scripts that are likely servers
	if strings.Contains(commandLower, "node") {
		serverIndicators := []string{
			"server.js", "app.js", "index.js", "main.js",
		}

		for _, indicator := range serverIndicators {
			if strings.Contains(commandLower, indicator) {
				return true
			}
		}

		// Check for server frameworks
		serverFrameworks := []string{
			"express", "fastify", "koa", "hapi",
		}

		for _, framework := range serverFrameworks {
			if strings.Contains(commandLower, framework) {
				return true
			}
		}

		// Check for long-running JavaScript patterns in inline code
		longRunningJSPatterns := []string{
			"setinterval", "settimeout", "while(true)", "while (true)",
			"for(;;)", "for (;;)", "http.createserver", "http.listen",
			"server.listen", ".listen(", "process.on(",
		}

		for _, pattern := range longRunningJSPatterns {
			if strings.Contains(commandLower, pattern) {
				return true
			}
		}
	}

	// Generic patterns (least specific, checked last)
	// Only match if these are actual command parts, not just words in strings
	if strings.Contains(commandLower, " dev ") || strings.Contains(commandLower, " serve ") ||
		strings.Contains(commandLower, " start ") || strings.Contains(commandLower, " watch ") ||
		strings.HasSuffix(commandLower, " dev") || strings.HasSuffix(commandLower, " serve") ||
		strings.HasSuffix(commandLower, " start") || strings.HasSuffix(commandLower, " watch") {
		// Exclude simple echo commands or basic utilities
		if strings.HasPrefix(commandLower, "echo ") || strings.HasPrefix(commandLower, "printf ") {
			return false
		}
		return true
	}

	return false
}

// IsDevServerCommand determines if a command starts a development server
func (d *PackageManagerDetector) IsDevServerCommand(command string) bool {
	commandLower := strings.ToLower(command)

	// First, exclude commands that should never be background processes
	excludePatterns := []string{
		"sqlite3", "mysql", "psql", "redis-cli", // Database CLIs
		"grep", "find", "ls", "cat", "echo", "sleep", // Basic utilities
		"curl", "wget", "git", // Network/VCS utilities
		"npm install", "yarn install", "pip install", // Package installations
		"SELECT", "INSERT", "UPDATE", "DELETE", // SQL queries
	}

	for _, exclude := range excludePatterns {
		if strings.Contains(commandLower, strings.ToLower(exclude)) {
			return false
		}
	}

	devServerPatterns := []string{
		// Development servers
		"npm run dev", "npm start", "npm run serve",
		"yarn dev", "yarn start", "yarn serve",
		"pnpm dev", "pnpm start", "pnpm serve",
		"bun dev", "bun start", "bun serve",
		"flask run", "django runserver", "uvicorn", "gunicorn",
		"nodemon", "webpack-dev-server", "vite", "parcel",
		"next dev", "nuxt dev", "gatsby develop", "svelte-kit dev",
		"rails server", "php artisan serve",

		// Network testing and monitoring (commonly used by agents)
		"ping", "traceroute", "netstat -l", "netstat --listen",

		// File monitoring and watching
		"tail -f", "tail --follow", "watch",

		// System monitoring
		"top", "htop", "monitor",
	}

	// Check exact patterns first
	for _, pattern := range devServerPatterns {
		if strings.Contains(commandLower, pattern) {
			return true
		}
	}

	// Check for Python scripts that are likely dev servers
	if strings.Contains(commandLower, "python") {
		devServerIndicators := []string{
			"server.py", "http_server.py", "dev_server.py",
			"app.py", "main.py", "run.py",
			"flask", "django", "fastapi", "tornado",
			"http.server", "SimpleHTTPServer",
		}

		for _, indicator := range devServerIndicators {
			if strings.Contains(commandLower, indicator) {
				return true
			}
		}
	}

	// Check for Node.js scripts that are likely dev servers
	if strings.Contains(commandLower, "node") {
		devServerIndicators := []string{
			"server.js", "dev_server.js", "app.js",
			"express", "fastify", "koa", "hapi",
		}

		for _, indicator := range devServerIndicators {
			if strings.Contains(commandLower, indicator) {
				return true
			}
		}
	}

	return false
}
