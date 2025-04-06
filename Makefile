# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=trusted-sequencer
BINARY_UNIX=$(BINARY_NAME)

# Build parameters
BUILD_DIR=build
VERSION?=0.1.0
BUILD_TIME=$(shell date +%FT%T%z)
GIT_COMMIT=$(shell git rev-parse HEAD)
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)"

# Colors for output
RED=\033[0;31m
GREEN=\033[0;32m
BLUE=\033[0;34m
NC=\033[0m

.PHONY: all build clean test lint run help release docs coverage benchmark proto generate

all: clean build

build:
	@echo "$(BLUE)Building $(BINARY_NAME)...$(NC)"
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/sequencer
	@echo "$(GREEN)Build completed!$(NC)"

clean:
	@echo "$(BLUE)Cleaning...$(NC)"
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -rf coverage
	@echo "$(GREEN)Clean completed!$(NC)"

test:
	@echo "$(BLUE)Running tests...$(NC)"
	$(GOTEST) -v ./...
	@echo "$(GREEN)Tests completed!$(NC)"

lint:
	@echo "$(BLUE)Running linter...$(NC)"
	golangci-lint run
	@echo "$(GREEN)Lint completed!$(NC)"

run: build
	@echo "$(BLUE)Running $(BINARY_NAME)...$(NC)"
	./$(BUILD_DIR)/$(BINARY_NAME)

# Code coverage
coverage:
	@echo "$(BLUE)Generating test coverage...$(NC)"
	@mkdir -p coverage
	$(GOTEST) -coverprofile=coverage/coverage.out ./...
	$(GOCMD) tool cover -html=coverage/coverage.out -o coverage/coverage.html
	@echo "$(GREEN)Coverage report generated in coverage/ directory$(NC)"

# Benchmarking
benchmark:
	@echo "$(BLUE)Running benchmarks...$(NC)"
	$(GOTEST) -bench=. -benchmem ./...
	@echo "$(GREEN)Benchmarks completed!$(NC)"

# Protocol buffer generation
proto:
	@echo "$(BLUE)Generating protocol buffer code...$(NC)"
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/*.proto
	@echo "$(GREEN)Protocol buffer code generated!$(NC)"

# Code generation
generate:
	@echo "$(BLUE)Generating code...$(NC)"
	$(GOCMD) generate ./...
	@echo "$(GREEN)Code generation completed!$(NC)"

# Documentation
docs:
	@echo "$(BLUE)Generating documentation...$(NC)"
	@mkdir -p docs
	swag init -g cmd/sequencer/main.go -o docs/swagger
	@echo "$(GREEN)Documentation generated!$(NC)"

# Release preparation
release: clean test lint coverage benchmark
	@echo "$(BLUE)Preparing release $(VERSION)...$(NC)"
	@echo "$(GREEN)Release preparation completed!$(NC)"

help:
	@echo "$(BLUE)Available commands:$(NC)"
	@echo "  make build         - Build the application"
	@echo "  make clean         - Remove build files"
	@echo "  make test          - Run tests"
	@echo "  make lint          - Run linter"
	@echo "  make run           - Build and run the application"
	@echo "  make coverage      - Generate test coverage report"
	@echo "  make benchmark     - Run benchmarks"
	@echo "  make proto         - Generate protocol buffer code"
	@echo "  make generate      - Generate code"
	@echo "  make docs          - Generate documentation"
	@echo "  make release       - Prepare release (test, lint, coverage, benchmark)"
	@echo "  make help          - Show this help message" 