# Database Backup Utility - Makefile

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
BINARY_NAME=db-backup
BINARY_UNIX=$(BINARY_NAME)_unix

# Build info
VERSION?=0.1.0
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)"

# Directories
BIN_DIR=bin
CMD_CLI_DIR=cmd/cli
CMD_SERVER_DIR=cmd/server
CMD_WORKER_DIR=cmd/worker

.PHONY: all build clean test coverage deps help

# Default target
all: test build

## help: Display this help message
help:
	@echo "Database Backup Utility - Available targets:"
	@echo ""
	@grep -E '^## .*:' $(MAKEFILE_LIST) | sed 's/## /  /'
	@echo ""

## deps: Download Go module dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

## build: Build all binaries
build: build-cli build-server build-worker

## build-cli: Build CLI binary
build-cli:
	@echo "Building CLI binary..."
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_CLI_DIR)/main.go

## build-server: Build server binary
build-server:
	@echo "Building server binary..."
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-server $(CMD_SERVER_DIR)/main.go

## build-worker: Build worker binary
build-worker:
	@echo "Building worker binary..."
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-worker $(CMD_WORKER_DIR)/main.go

## build-linux: Cross compile for Linux
build-linux:
	@echo "Building for Linux..."
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_UNIX) $(CMD_CLI_DIR)/main.go

## test: Run unit tests
test:
	$(GOTEST) -v -race -timeout 30s ./...

## test-coverage: Run tests with coverage
test-coverage:
	$(GOTEST) -v -race -coverprofile=coverage.out -covermode=atomic ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## test-integration: Run integration tests
test-integration:
	$(GOTEST) -v -race -tags=integration -timeout 5m ./tests/integration/...

## clean: Clean build artifacts
clean:
	$(GOCLEAN)
	rm -rf $(BIN_DIR)
	rm -f coverage.out coverage.html

## fmt: Format Go code
fmt:
	$(GOCMD) fmt ./...

## vet: Run go vet
vet:
	$(GOCMD) vet ./...

## lint: Run golangci-lint
lint:
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed. Install: https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run ./...

## run-cli: Run CLI application
run-cli:
	$(GOCMD) run $(CMD_CLI_DIR)/main.go

## run-server: Run server application
run-server:
	$(GOCMD) run $(CMD_SERVER_DIR)/main.go

## docker-build: Build Docker image
docker-build:
	docker build -f deployments/docker/Dockerfile.cli -t $(BINARY_NAME):$(VERSION) .

## docker-build-server: Build Docker server image
docker-build-server:
	docker build -f deployments/docker/Dockerfile.server -t $(BINARY_NAME)-server:$(VERSION) .

## docker-compose-up: Start development environment
docker-compose-up:
	docker-compose -f deployments/docker/docker-compose.yml up -d

## docker-compose-down: Stop development environment
docker-compose-down:
	docker-compose -f deployments/docker/docker-compose.yml down

## install: Install binaries to GOPATH/bin
install:
	$(GOCMD) install $(LDFLAGS) $(CMD_CLI_DIR)/main.go

## mod-tidy: Tidy go.mod
mod-tidy:
	$(GOMOD) tidy

## mod-vendor: Vendor dependencies
mod-vendor:
	$(GOMOD) vendor

## check: Run all checks (fmt, vet, lint, test)
check: fmt vet lint test

.DEFAULT_GOAL := help
