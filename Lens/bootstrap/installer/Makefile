# Primus Lens Installer Makefile

# Variables
BINARY_NAME := primus-lens-installer
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

# Docker parameters
DOCKER_REGISTRY ?= primussafe
DOCKER_IMAGE := $(DOCKER_REGISTRY)/primus-lens-installer
DOCKER_TAG ?= $(VERSION)

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod
GOFMT := gofmt
GOLINT := golangci-lint

# Directories
CMD_DIR := ./cmd/installer
BUILD_DIR := ./build
COVERAGE_DIR := ./coverage
CHARTS_DIR := ../../charts

.PHONY: all build clean test test-coverage lint fmt deps help install run docker-build docker-push

# Default target
all: deps fmt lint test build

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)
	@echo "Built: $(BUILD_DIR)/$(BINARY_NAME)"

# Build for multiple platforms
build-all: build-linux build-darwin build-windows

build-linux:
	@echo "Building for Linux..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_DIR)

build-darwin:
	@echo "Building for macOS..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(CMD_DIR)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(CMD_DIR)

build-windows:
	@echo "Building for Windows..."
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(CMD_DIR)

# Install the binary
install: build
	@echo "Installing $(BINARY_NAME)..."
	cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)
	rm -rf $(COVERAGE_DIR)
	$(GOCMD) clean

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@mkdir -p $(COVERAGE_DIR)
	$(GOTEST) -v -coverprofile=$(COVERAGE_DIR)/coverage.out ./...
	$(GOCMD) tool cover -html=$(COVERAGE_DIR)/coverage.out -o $(COVERAGE_DIR)/coverage.html
	@echo "Coverage report: $(COVERAGE_DIR)/coverage.html"

# Run tests with race detection
test-race:
	@echo "Running tests with race detection..."
	$(GOTEST) -v -race ./...

# Format code
fmt:
	@echo "Formatting code..."
	$(GOFMT) -s -w .

# Lint code
lint:
	@echo "Linting code..."
	@if command -v $(GOLINT) > /dev/null; then \
		$(GOLINT) run ./...; \
	else \
		echo "golangci-lint not installed, skipping lint"; \
	fi

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Verify dependencies
verify:
	@echo "Verifying dependencies..."
	$(GOMOD) verify

# Run the installer (for development)
run: build
	$(BUILD_DIR)/$(BINARY_NAME) $(ARGS)

# Run with dataplane install
run-dataplane: build
	$(BUILD_DIR)/$(BINARY_NAME) install dataplane --verbose

# Run status check
run-status: build
	$(BUILD_DIR)/$(BINARY_NAME) status dataplane

# Generate mocks (if needed)
mocks:
	@echo "Generating mocks..."
	@if command -v mockgen > /dev/null; then \
		go generate ./...; \
	else \
		echo "mockgen not installed, skipping mock generation"; \
	fi

# Docker build
docker-build:
	@echo "Building Docker image $(DOCKER_IMAGE):$(DOCKER_TAG)..."
	@# Create a temporary build context with charts
	@rm -rf $(BUILD_DIR)/docker-context
	@mkdir -p $(BUILD_DIR)/docker-context
	@cp -r . $(BUILD_DIR)/docker-context/
	@cp -r $(CHARTS_DIR) $(BUILD_DIR)/docker-context/charts
	@cd $(BUILD_DIR)/docker-context && docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) -f Dockerfile .
	@rm -rf $(BUILD_DIR)/docker-context
	@echo "Built: $(DOCKER_IMAGE):$(DOCKER_TAG)"

# Docker push
docker-push: docker-build
	@echo "Pushing Docker image $(DOCKER_IMAGE):$(DOCKER_TAG)..."
	docker push $(DOCKER_IMAGE):$(DOCKER_TAG)
	@if [ "$(DOCKER_TAG)" != "latest" ]; then \
		docker tag $(DOCKER_IMAGE):$(DOCKER_TAG) $(DOCKER_IMAGE):latest; \
		docker push $(DOCKER_IMAGE):latest; \
	fi

# Show help
help:
	@echo "Primus Lens Installer - Makefile Commands"
	@echo ""
	@echo "Usage:"
	@echo "  make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all           - Run deps, fmt, lint, test, and build"
	@echo "  build         - Build the binary"
	@echo "  build-all     - Build for all platforms (linux, darwin, windows)"
	@echo "  clean         - Remove build artifacts"
	@echo "  test          - Run tests"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  test-race     - Run tests with race detection"
	@echo "  fmt           - Format code"
	@echo "  lint          - Lint code"
	@echo "  deps          - Download dependencies"
	@echo "  install       - Install binary to /usr/local/bin"
	@echo "  run           - Build and run (use ARGS='...' for arguments)"
	@echo "  docker-build  - Build Docker image"
	@echo "  docker-push   - Build and push Docker image"
	@echo "  help          - Show this help message"
	@echo ""
	@echo "Docker Examples:"
	@echo "  make docker-build"
	@echo "  make docker-build DOCKER_TAG=v1.0.0"
	@echo "  make docker-push DOCKER_REGISTRY=myregistry"
	@echo ""
	@echo "Examples:"
	@echo "  make build"
	@echo "  make test"
	@echo "  make run ARGS='install dataplane --dry-run'"

