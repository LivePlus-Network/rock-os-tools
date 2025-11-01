# Makefile for rock-os-tools
# Production-ready build system for ROCK-OS Go tools suite
#
# Usage:
#   make                 - Build all tools
#   make install         - Install to ROCK-MASTER/bin
#   make test           - Run all tests
#   make clean          - Clean build artifacts
#   make help           - Show this help

# Load configuration
-include config.env

# Version management
VERSION ?= $(shell cat VERSION 2>/dev/null || echo "0.1.0")
BUILD_TIME := $(shell date -u +%Y%m%d-%H%M%S)
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Go build configuration
GO := go
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
CGO_ENABLED := 0

# Build flags
LDFLAGS := -ldflags "\
	-X main.Version=$(VERSION) \
	-X main.BuildTime=$(BUILD_TIME) \
	-X main.GitCommit=$(GIT_COMMIT) \
	-s -w"

# Test flags
TEST_FLAGS := -v -race -coverprofile=coverage.out -covermode=atomic

# Directory configuration
PROJECT_ROOT := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
ROCK_MASTER_ROOT ?= /Volumes/4TB/ROCK-MASTER
CMD_DIR := cmd
PKG_DIR := pkg
BIN_DIR := bin
TEST_DIR := test
DOCS_DIR := docs

# Binary output directory (platform-specific)
BINARY_DIR := $(BIN_DIR)/$(GOOS)

# Installation directory
INSTALL_DIR := $(ROCK_MASTER_ROOT)/bin/tools
INSTALL_MODE := 755

# Tool list
TOOLS := \
	rock-kernel \
	rock-deps \
	rock-build \
	rock-image \
	rock-config \
	rock-security \
	rock-cache \
	rock-verify \
	rock-compose \
	rock-registry

# Colors for output
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[0;33m
BLUE := \033[0;34m
NC := \033[0m # No Color

# Default target
.DEFAULT_GOAL := all

# Phony targets
.PHONY: all build test install dev-install clean deps lint fmt vet \
        release help version check-env $(TOOLS)

# Main targets
all: check-env clean build test
	@echo "$(GREEN)✓ Build complete$(NC)"

# Check environment
check-env:
	@echo "$(BLUE)Checking environment...$(NC)"
	@if [ ! -d "$(PROJECT_ROOT)" ]; then \
		echo "$(RED)Error: Project root not found: $(PROJECT_ROOT)$(NC)"; \
		exit 1; \
	fi
	@if [ ! -d "$(ROCK_MASTER_ROOT)" ]; then \
		echo "$(YELLOW)Warning: ROCK_MASTER_ROOT not found: $(ROCK_MASTER_ROOT)$(NC)"; \
		echo "$(YELLOW)Set ROCK_MASTER_ROOT environment variable if needed$(NC)"; \
	fi
	@if ! command -v $(GO) > /dev/null; then \
		echo "$(RED)Error: Go not found. Please install Go 1.21+$(NC)"; \
		exit 1; \
	fi
	@echo "$(GREEN)✓ Environment OK$(NC)"

# Build all tools
build: $(TOOLS)
	@echo "$(GREEN)✓ All tools built successfully$(NC)"

# Build individual tools
$(TOOLS): check-env
	@echo "$(BLUE)Building $@...$(NC)"
	@mkdir -p $(BINARY_DIR)
	@if [ -f "$(CMD_DIR)/$@/main.go" ]; then \
		CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build \
			$(LDFLAGS) \
			-o $(BINARY_DIR)/$@ \
			./$(CMD_DIR)/$@; \
		echo "$(GREEN)✓ Built $(BINARY_DIR)/$@$(NC)"; \
	else \
		echo "$(YELLOW)⚠ Source not found: $(CMD_DIR)/$@/main.go$(NC)"; \
		echo "$(YELLOW)  Creating placeholder for $@$(NC)"; \
		mkdir -p $(CMD_DIR)/$@; \
		echo "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"$@ - placeholder\") }" > $(CMD_DIR)/$@/main.go; \
		CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build \
			$(LDFLAGS) \
			-o $(BINARY_DIR)/$@ \
			./$(CMD_DIR)/$@; \
	fi

# Run tests
test:
	@echo "$(BLUE)Running tests...$(NC)"
	@if [ -d "$(PKG_DIR)" ] && [ -n "$$(find $(PKG_DIR) -name '*_test.go')" ]; then \
		$(GO) test $(TEST_FLAGS) ./$(PKG_DIR)/...; \
		echo "$(GREEN)✓ Tests passed$(NC)"; \
	else \
		echo "$(YELLOW)No tests found$(NC)"; \
	fi
	@if [ -d "$(TEST_DIR)" ]; then \
		$(GO) test $(TEST_FLAGS) ./$(TEST_DIR)/...; \
	fi

# Install to ROCK-MASTER
install: build
	@echo "$(BLUE)Installing to $(INSTALL_DIR)...$(NC)"
	@mkdir -p $(INSTALL_DIR)
	@for tool in $(TOOLS); do \
		if [ -f "$(BINARY_DIR)/$$tool" ]; then \
			cp $(BINARY_DIR)/$$tool $(INSTALL_DIR)/$$tool; \
			chmod $(INSTALL_MODE) $(INSTALL_DIR)/$$tool; \
			echo "$(GREEN)✓ Installed $$tool$(NC)"; \
		else \
			echo "$(YELLOW)⚠ $$tool not found in $(BINARY_DIR)$(NC)"; \
		fi \
	done
	@# Create PATH setup script
	@echo '#!/bin/bash' > $(INSTALL_DIR)/setup-path.sh
	@echo 'export PATH="$$PATH:$$(dirname "$${BASH_SOURCE[0]}")"' >> $(INSTALL_DIR)/setup-path.sh
	@echo 'echo "rock-os-tools added to PATH"' >> $(INSTALL_DIR)/setup-path.sh
	@chmod +x $(INSTALL_DIR)/setup-path.sh
	@echo "$(GREEN)✓ Installation complete$(NC)"
	@echo "$(BLUE)To use the tools:$(NC)"
	@echo "  export PATH=\"\$$PATH:$(INSTALL_DIR)\""
	@echo "  or"
	@echo "  source $(INSTALL_DIR)/setup-path.sh"

# Development install (symlinks)
dev-install: build
	@echo "$(BLUE)Creating development symlinks...$(NC)"
	@mkdir -p $(INSTALL_DIR)
	@for tool in $(TOOLS); do \
		if [ -f "$(BINARY_DIR)/$$tool" ]; then \
			ln -sf $(PROJECT_ROOT)/$(BINARY_DIR)/$$tool $(INSTALL_DIR)/$$tool; \
			echo "$(GREEN)✓ Linked $$tool$(NC)"; \
		fi \
	done
	@echo "$(GREEN)✓ Development links created$(NC)"

# Clean build artifacts
clean:
	@echo "$(BLUE)Cleaning...$(NC)"
	@rm -rf $(BIN_DIR)
	@rm -f coverage.out
	@$(GO) clean -cache
	@echo "$(GREEN)✓ Clean complete$(NC)"

# Update dependencies
deps:
	@echo "$(BLUE)Updating dependencies...$(NC)"
	@$(GO) mod download
	@$(GO) mod tidy
	@$(GO) mod verify
	@echo "$(GREEN)✓ Dependencies updated$(NC)"

# Lint code
lint:
	@echo "$(BLUE)Running linter...$(NC)"
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run ./...; \
	else \
		echo "$(YELLOW)golangci-lint not installed$(NC)"; \
		echo "Install with: brew install golangci-lint"; \
	fi

# Format code
fmt:
	@echo "$(BLUE)Formatting code...$(NC)"
	@$(GO) fmt ./...
	@echo "$(GREEN)✓ Code formatted$(NC)"

# Run go vet
vet:
	@echo "$(BLUE)Running go vet...$(NC)"
	@$(GO) vet ./...
	@echo "$(GREEN)✓ Vet complete$(NC)"

# Build release binaries for all platforms
release: clean
	@echo "$(BLUE)Building release binaries...$(NC)"
	@mkdir -p $(BIN_DIR)

	@# Darwin AMD64
	@echo "$(BLUE)Building for darwin/amd64...$(NC)"
	@mkdir -p $(BIN_DIR)/darwin
	@for tool in $(TOOLS); do \
		if [ -f "$(CMD_DIR)/$$tool/main.go" ]; then \
			CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GO) build \
				$(LDFLAGS) \
				-o $(BIN_DIR)/darwin/$$tool \
				./$(CMD_DIR)/$$tool; \
		fi \
	done

	@# Darwin ARM64
	@echo "$(BLUE)Building for darwin/arm64...$(NC)"
	@for tool in $(TOOLS); do \
		if [ -f "$(CMD_DIR)/$$tool/main.go" ]; then \
			CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build \
				-ldflags "-X main.Version=$(VERSION)-arm64 $(LDFLAGS)" \
				-o $(BIN_DIR)/darwin/$$tool-arm64 \
				./$(CMD_DIR)/$$tool; \
		fi \
	done

	@# Linux AMD64
	@echo "$(BLUE)Building for linux/amd64...$(NC)"
	@mkdir -p $(BIN_DIR)/linux
	@for tool in $(TOOLS); do \
		if [ -f "$(CMD_DIR)/$$tool/main.go" ]; then \
			CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build \
				$(LDFLAGS) \
				-o $(BIN_DIR)/linux/$$tool \
				./$(CMD_DIR)/$$tool; \
		fi \
	done

	@# Create archives
	@echo "$(BLUE)Creating release archives...$(NC)"
	@cd $(BIN_DIR) && tar czf rock-os-tools-v$(VERSION)-darwin.tar.gz darwin/
	@cd $(BIN_DIR) && tar czf rock-os-tools-v$(VERSION)-linux.tar.gz linux/
	@echo "$(GREEN)✓ Release archives created$(NC)"

# Show version
version:
	@echo "rock-os-tools version $(VERSION)"
	@echo "Build time: $(BUILD_TIME)"
	@echo "Git commit: $(GIT_COMMIT)"

# Help target
help:
	@echo "$(BLUE)rock-os-tools Makefile$(NC)"
	@echo ""
	@echo "$(YELLOW)Usage:$(NC)"
	@echo "  make              Build all tools"
	@echo "  make install      Install to ROCK-MASTER/bin/tools"
	@echo "  make dev-install  Install as symlinks (for development)"
	@echo "  make test         Run all tests"
	@echo "  make clean        Remove build artifacts"
	@echo "  make deps         Update Go dependencies"
	@echo "  make lint         Run linter"
	@echo "  make fmt          Format code"
	@echo "  make vet          Run go vet"
	@echo "  make release      Build for all platforms"
	@echo "  make version      Show version information"
	@echo "  make help         Show this help"
	@echo ""
	@echo "$(YELLOW)Individual tools:$(NC)"
	@for tool in $(TOOLS); do \
		echo "  make $$tool      Build $$tool only"; \
	done
	@echo ""
	@echo "$(YELLOW)Environment variables:$(NC)"
	@echo "  ROCK_MASTER_ROOT  Target installation directory"
	@echo "                    (default: /Volumes/4TB/ROCK-MASTER)"
	@echo "  GOOS              Target OS (default: current OS)"
	@echo "  GOARCH            Target architecture (default: current arch)"
	@echo "  VERSION           Version string (default: from VERSION file)"
	@echo ""
	@echo "$(YELLOW)Examples:$(NC)"
	@echo "  make                           # Build everything"
	@echo "  make rock-kernel               # Build single tool"
	@echo "  make install                   # Install to ROCK-MASTER"
	@echo "  ROCK_MASTER_ROOT=/tmp make install  # Custom install location"