.PHONY: all build install uninstall clean help test docker-up docker-down docker-rebuild docker-logs docker-build docker-build-full docker-test docker-run docker-run-full docker-run-agent docker-run-agent-full docker-clean new-agent

# Build variables
BINARY_NAME=picoclaw
BUILD_DIR=build
CMD_DIR=cmd/$(BINARY_NAME)
MAIN_GO=$(CMD_DIR)/main.go

# Version
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT=$(shell git rev-parse --short=8 HEAD 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date +%FT%T%z)
GO_VERSION=$(shell $(GO) version | awk '{print $$3}')
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.gitCommit=$(GIT_COMMIT) -X main.buildTime=$(BUILD_TIME) -X main.goVersion=$(GO_VERSION) -s -w"

# Go variables
GO?=go
GOFLAGS?=-v -tags stdjson

# Golangci-lint
GOLANGCI_LINT?=golangci-lint

# Installation
INSTALL_PREFIX?=$(HOME)/.local
INSTALL_BIN_DIR=$(INSTALL_PREFIX)/bin
INSTALL_MAN_DIR=$(INSTALL_PREFIX)/share/man/man1

# Workspace and Skills
PICOCLAW_HOME?=$(HOME)/.picoclaw
WORKSPACE_DIR?=$(PICOCLAW_HOME)/workspace
WORKSPACE_SKILLS_DIR=$(WORKSPACE_DIR)/skills
BUILTIN_SKILLS_DIR=$(CURDIR)/skills

# OS detection
UNAME_S:=$(shell uname -s)
UNAME_M:=$(shell uname -m)

# Platform-specific settings
ifeq ($(UNAME_S),Linux)
	PLATFORM=linux
	ifeq ($(UNAME_M),x86_64)
		ARCH=amd64
	else ifeq ($(UNAME_M),aarch64)
		ARCH=arm64
	else ifeq ($(UNAME_M),loongarch64)
		ARCH=loong64
	else ifeq ($(UNAME_M),riscv64)
		ARCH=riscv64
	else
		ARCH=$(UNAME_M)
	endif
else ifeq ($(UNAME_S),Darwin)
	PLATFORM=darwin
	ifeq ($(UNAME_M),x86_64)
		ARCH=amd64
	else ifeq ($(UNAME_M),arm64)
		ARCH=arm64
	else
		ARCH=$(UNAME_M)
	endif
else
	PLATFORM=$(UNAME_S)
	ARCH=$(UNAME_M)
endif

BINARY_PATH=$(BUILD_DIR)/$(BINARY_NAME)-$(PLATFORM)-$(ARCH)

# Default target
all: build

## generate: Run generate
generate:
	@echo "Run generate..."
	@rm -r ./$(CMD_DIR)/workspace 2>/dev/null || true
	@$(GO) generate ./...
	@echo "Run generate complete"

## build: Build the picoclaw binary for current platform
build: generate
	@echo "Building $(BINARY_NAME) for $(PLATFORM)/$(ARCH)..."
	@mkdir -p $(BUILD_DIR)
	@$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINARY_PATH) ./$(CMD_DIR)
	@echo "Build complete: $(BINARY_PATH)"
	@ln -sf $(BINARY_NAME)-$(PLATFORM)-$(ARCH) $(BUILD_DIR)/$(BINARY_NAME)

## build-all: Build picoclaw for all platforms
build-all: generate
	@echo "Building for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./$(CMD_DIR)
	GOOS=linux GOARCH=arm64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./$(CMD_DIR)
	GOOS=linux GOARCH=loong64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-loong64 ./$(CMD_DIR)
	GOOS=linux GOARCH=riscv64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-riscv64 ./$(CMD_DIR)
	GOOS=darwin GOARCH=arm64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./$(CMD_DIR)
	GOOS=windows GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./$(CMD_DIR)
	@echo "All builds complete"

## install: Install picoclaw to system and copy builtin skills
install: build
	@echo "Installing $(BINARY_NAME)..."
	@mkdir -p $(INSTALL_BIN_DIR)
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_BIN_DIR)/$(BINARY_NAME)
	@chmod +x $(INSTALL_BIN_DIR)/$(BINARY_NAME)
	@echo "Installed binary to $(INSTALL_BIN_DIR)/$(BINARY_NAME)"
	@echo "Installation complete!"

## uninstall: Remove picoclaw from system
uninstall:
	@echo "Uninstalling $(BINARY_NAME)..."
	@rm -f $(INSTALL_BIN_DIR)/$(BINARY_NAME)
	@echo "Removed binary from $(INSTALL_BIN_DIR)/$(BINARY_NAME)"
	@echo "Note: Only the executable file has been deleted."
	@echo "If you need to delete all configurations (config.json, workspace, etc.), run 'make uninstall-all'"

## uninstall-all: Remove picoclaw and all data
uninstall-all:
	@echo "Removing workspace and skills..."
	@rm -rf $(PICOCLAW_HOME)
	@echo "Removed workspace: $(PICOCLAW_HOME)"
	@echo "Complete uninstallation done!"

## clean: Remove build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@echo "Clean complete"

## vet: Run go vet for static analysis
vet:
	@$(GO) vet ./...

## test: Test Go code
test:
	@$(GO) test ./...

## fmt: Format Go code
fmt:
	@$(GOLANGCI_LINT) fmt

## lint: Run linters
lint:
	@$(GOLANGCI_LINT) run

## deps: Download dependencies
deps:
	@$(GO) mod download
	@$(GO) mod verify

## update-deps: Update dependencies
update-deps:
	@$(GO) get -u ./...
	@$(GO) mod tidy

## check: Run vet, fmt, and verify dependencies
check: deps fmt vet test

## run: Build and run picoclaw
run: build
	@$(BUILD_DIR)/$(BINARY_NAME) $(ARGS)

# Docker Compose (multi-agent)
# Discovers all agents from config/agents/*/compose.yml automatically
COMPOSE_FILE=docker-compose.multi.yml
AGENT_COMPOSE_FILES:=$(wildcard config/agents/*/compose.yml)
COMPOSE_FLAGS=-f $(COMPOSE_FILE) $(foreach f,$(AGENT_COMPOSE_FILES),-f $(f)) --project-directory .

## docker-up: Build and start all gateway agents
docker-up:
	@docker compose $(COMPOSE_FLAGS) --profile gateway up --build -d

## docker-up-%: Build and start a single agent (e.g. make docker-up-senior-developer)
docker-up-%:
	@docker compose $(COMPOSE_FLAGS) up --build -d picoclaw-$*

## docker-rebuild: Rebuild and restart containers without downtime
docker-rebuild:
	@docker compose $(COMPOSE_FLAGS) --profile gateway up --build -d --force-recreate

## docker-rebuild-%: Rebuild and restart a single agent (e.g. make docker-rebuild-senior-developer)
docker-rebuild-%:
	@docker compose $(COMPOSE_FLAGS) up --build -d --force-recreate picoclaw-$*

## docker-down: Stop all containers
docker-down:
	@docker compose $(COMPOSE_FLAGS) --profile gateway down

## docker-logs: Follow logs from all gateway agents
docker-logs:
	@docker compose $(COMPOSE_FLAGS) --profile gateway logs -f

## new-agent: Scaffold a new agent (usage: make new-agent NAME=my-agent)
new-agent:
	@if [ -z "$(NAME)" ]; then echo "Usage: make new-agent NAME=my-agent"; exit 1; fi
	@if [ -d "config/agents/$(NAME)" ]; then echo "Error: agent '$(NAME)' already exists"; exit 1; fi
	@mkdir -p config/agents/$(NAME)
	@sed 's/example/$(NAME)/g' config/agents/example/config.example.json > config/agents/$(NAME)/config.json
	@sed 's/example/$(NAME)/g' config/agents/example/compose.example.yml > config/agents/$(NAME)/compose.yml
	@mkdir -p workspace/$(NAME)
	@ln -sfn ../skills workspace/$(NAME)/skills
	@echo "Agent '$(NAME)' created:"
	@echo "  config/agents/$(NAME)/config.json   <- edit API keys and Discord token"
	@echo "  config/agents/$(NAME)/compose.yml    <- edit port if needed"
	@echo ""
	@echo "Start with: make docker-up-$(NAME)"

## docker-build: Build Docker image (minimal Alpine-based)
docker-build:
	@echo "Building minimal Docker image (Alpine-based)..."
	docker compose build picoclaw-agent picoclaw-gateway

## docker-build-full: Build Docker image with full MCP support (Node.js 24)
docker-build-full:
	@echo "Building full-featured Docker image (Node.js 24)..."
	docker compose -f docker-compose.full.yml build picoclaw-agent picoclaw-gateway

## docker-test: Test MCP tools in Docker container
docker-test:
	@echo "Testing MCP tools in Docker..."
	@chmod +x scripts/test-docker-mcp.sh
	@./scripts/test-docker-mcp.sh

## docker-run: Run picoclaw gateway in Docker (Alpine-based)
docker-run:
	docker compose --profile gateway up

## docker-run-full: Run picoclaw gateway in Docker (full-featured)
docker-run-full:
	docker compose -f docker-compose.full.yml --profile gateway up

## docker-run-agent: Run picoclaw agent in Docker (interactive, Alpine-based)
docker-run-agent:
	docker compose run --rm picoclaw-agent

## docker-run-agent-full: Run picoclaw agent in Docker (interactive, full-featured)
docker-run-agent-full:
	docker compose -f docker-compose.full.yml run --rm picoclaw-agent

## docker-clean: Clean Docker images and volumes
docker-clean:
	docker compose down -v
	docker compose -f docker-compose.full.yml down -v
	docker rmi picoclaw:latest picoclaw:full 2>/dev/null || true

## help: Show this help message
help:
	@echo "picoclaw Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
	@echo ""
	@echo "Examples:"
	@echo "  make build              # Build for current platform"
	@echo "  make install            # Install to ~/.local/bin"
	@echo "  make uninstall          # Remove from /usr/local/bin"
	@echo "  make install-skills     # Install skills to workspace"
	@echo "  make docker-build       # Build minimal Docker image"
	@echo "  make docker-test        # Test MCP tools in Docker"
	@echo ""
	@echo "Environment Variables:"
	@echo "  INSTALL_PREFIX          # Installation prefix (default: ~/.local)"
	@echo "  WORKSPACE_DIR           # Workspace directory (default: ~/.picoclaw/workspace)"
	@echo "  VERSION                 # Version string (default: git describe)"
	@echo ""
	@echo "Current Configuration:"
	@echo "  Platform: $(PLATFORM)/$(ARCH)"
	@echo "  Binary: $(BINARY_PATH)"
	@echo "  Install Prefix: $(INSTALL_PREFIX)"
	@echo "  Workspace: $(WORKSPACE_DIR)"
