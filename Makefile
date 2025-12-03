# Go-Term Makefile
# MCP Server for Terminal Session Management

BINARY_NAME := go-term
GOFLAGS := -v
COVERAGE_FILE := coverage.out

.PHONY: all build run test test-cover lint fmt vet clean install help

## Build Commands
all: clean fmt vet lint test build

build: ## Build the binary
	go build $(GOFLAGS) -o $(BINARY_NAME) .

run: ## Run the server in debug mode
	go run . --debug

install: ## Install the binary to GOPATH/bin
	go install .

## Testing
test: ## Run all tests
	go test $(GOFLAGS) ./...

test-cover: ## Run tests with coverage report
	go test $(GOFLAGS) -cover -coverprofile=$(COVERAGE_FILE) ./...
	go tool cover -func=$(COVERAGE_FILE)

test-html: test-cover ## Generate HTML coverage report
	go tool cover -html=$(COVERAGE_FILE) -o coverage.html
	@echo "Coverage report: coverage.html"

## Code Quality
fmt: ## Format code with gofumpt
	@which gofumpt > /dev/null || (echo "Installing gofumpt..." && go install mvdan.cc/gofumpt@latest)
	gofumpt -w .

lint: ## Run staticcheck linter
	@which staticcheck > /dev/null || (echo "Installing staticcheck..." && go install honnef.co/go/tools/cmd/staticcheck@latest)
	staticcheck ./...

vet: ## Run go vet
	go vet ./...

## Dependencies
deps: ## Download and tidy dependencies
	go mod download
	go mod tidy

deps-update: ## Update all dependencies
	go get -u ./...
	go mod tidy

## Cleanup
clean: ## Remove build artifacts and generated files
	rm -f $(BINARY_NAME)
	rm -f *.out
	rm -f coverage.html
	rm -f *.log
	rm -f *.db *.db-shm *.db-wal
	rm -rf dist/ build/ bin/

clean-all: clean ## Deep clean including caches
	go clean -cache -testcache

## Development
dev: fmt vet ## Quick development cycle (format + vet)
	go build $(GOFLAGS) -o $(BINARY_NAME) .

watch: ## Run with file watcher (requires watchexec)
	@which watchexec > /dev/null || echo "Install watchexec: brew install watchexec"
	watchexec -r -e go -- go run . --debug

## Help
help: ## Show this help message
	@echo "Go-Term Makefile Commands:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "Examples:"
	@echo "  make build      # Build the binary"
	@echo "  make test-cover # Run tests with coverage"
	@echo "  make clean      # Remove generated files"
