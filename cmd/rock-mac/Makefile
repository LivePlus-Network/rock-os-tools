# rock-mac - ROCK OS MAC Address Dispenser
# Makefile for building and installing the tool

BINARY_NAME=rock-mac
VERSION=1.0.0
BUILD_DIR=build
INSTALL_DIR=/usr/local/bin
GO_CMD=go
GO_BUILD=$(GO_CMD) build
GO_CLEAN=$(GO_CMD) clean
GO_TEST=$(GO_CMD) test
GO_GET=$(GO_CMD) get
GO_MOD=$(GO_CMD) mod

# Build flags
LDFLAGS=-ldflags "-X main.version=$(VERSION)"

# Colors for output
GREEN=\033[0;32m
YELLOW=\033[1;33m
NC=\033[0m

.PHONY: all build clean test install uninstall deps init help

all: deps build

## help: Display this help message
help:
	@echo "rock-mac Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^##' Makefile | sed 's/## /  /'

## deps: Download and install dependencies
deps:
	@echo "$(GREEN)Downloading dependencies...$(NC)"
	$(GO_MOD) download
	$(GO_MOD) tidy

## build: Build the binary
build:
	@echo "$(GREEN)Building $(BINARY_NAME) v$(VERSION)...$(NC)"
	@mkdir -p $(BUILD_DIR)
	$(GO_BUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "$(GREEN)Build complete: $(BUILD_DIR)/$(BINARY_NAME)$(NC)"

## install: Build and install to system
install: build
	@echo "$(GREEN)Installing $(BINARY_NAME) to $(INSTALL_DIR)...$(NC)"
	@sudo cp $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_DIR)/
	@sudo chmod 755 $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "$(GREEN)Installation complete!$(NC)"
	@echo "Run 'rock-mac --help' to get started"

## uninstall: Remove from system
uninstall:
	@echo "$(YELLOW)Removing $(BINARY_NAME) from $(INSTALL_DIR)...$(NC)"
	@sudo rm -f $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "$(GREEN)Uninstall complete$(NC)"

## clean: Clean build files
clean:
	@echo "$(YELLOW)Cleaning...$(NC)"
	$(GO_CLEAN)
	@rm -rf $(BUILD_DIR)
	@echo "$(GREEN)Clean complete$(NC)"

## test: Run tests
test:
	@echo "$(GREEN)Running tests...$(NC)"
	$(GO_TEST) -v ./...

## init: Initialize the database (run once)
init:
	@echo "$(GREEN)Initializing MAC dispenser database...$(NC)"
	@bash ../scripts/init-mac-dispenser.sh
	@echo "$(GREEN)Database initialized!$(NC)"

## run: Build and run with arguments
run: build
	@./$(BUILD_DIR)/$(BINARY_NAME) $(filter-out $@,$(MAKECMDGOALS))

# Quick commands for common operations
## allocate: Quick allocate a MAC address
allocate: build
	@./$(BUILD_DIR)/$(BINARY_NAME) allocate

## list: Quick list all allocations
list: build
	@./$(BUILD_DIR)/$(BINARY_NAME) list

## stats: Quick show statistics
stats: build
	@./$(BUILD_DIR)/$(BINARY_NAME) stats

# Catch all for run arguments
%:
	@: