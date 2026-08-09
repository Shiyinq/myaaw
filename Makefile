.PHONY: all build build-all install uninstall clean check run help swagger

# Build variables
BINARY_NAME=myaaw
BUILD_DIR=bin
CMD_DIR=cmd/myaaw
MAIN_GO=$(CMD_DIR)/main.go

# Version
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "v0.0.1")
GIT_COMMIT=$(shell git rev-parse --short=8 HEAD 2>/dev/null || echo "none")
BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Linker flags
LDFLAGS=-ldflags "-s -w -X main.Version=$(VERSION) -X main.Commit=$(GIT_COMMIT) -X main.Date=$(BUILD_DATE)"

# Go variables
GO?=go
GOFLAGS?=-v

# Installation
PREFIX?=$(HOME)/.local
ifeq ($(OS),Windows_NT)
	INSTALL_DIR=$(HOME)/AppData/Local/bin
	BINARY_EXT=.exe
else
	INSTALL_DIR=$(PREFIX)/bin
	BINARY_EXT=
endif

# OS detection for local build
UNAME_S:=$(shell uname -s)
UNAME_M:=$(shell uname -m)

ifeq ($(UNAME_S),Linux)
	PLATFORM=linux
	ARCH=$(shell dpkg --print-architecture 2>/dev/null || echo "amd64")
else ifeq ($(UNAME_S),Darwin)
	PLATFORM=darwin
	ifeq ($(UNAME_M),x86_64)
		ARCH=amd64
	else
		ARCH=arm64
	endif
else
	PLATFORM=windows
	ARCH=amd64
endif

BINARY_PATH=$(BUILD_DIR)/$(BINARY_NAME)$(BINARY_EXT)

# Default target
all: help

## build: Build the binary for current platform
build: swagger
	@echo "🚧 Building $(BINARY_NAME) for $(PLATFORM)/$(ARCH)..."
	@echo "   Version: $(VERSION)"
	@mkdir -p $(BUILD_DIR)
	@$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINARY_PATH) ./$(CMD_DIR)
	@echo "✅ Build complete: $(BINARY_PATH)"

## swagger: Generate Swagger documentation
swagger:
	@echo "📝 Generating Swagger documentation..."
	@mkdir -p docs/swagger
	@swag init -g $(MAIN_GO) --parseDependency --parseInternal --output docs/swagger --exclude .myaaw --parseGoList=false
	@echo "✅ Swagger generated"

## build-all: Cross-compile for Linux, Darwin, and Windows
build-all:
	@echo "📦 Building releases..."
	@mkdir -p $(BUILD_DIR)
	
	@echo "   - Darwin (amd64)"
	@GOOS=darwin GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/darwin-amd64/$(BINARY_NAME) ./$(CMD_DIR)
	
	@echo "   - Darwin (arm64)"
	@GOOS=darwin GOARCH=arm64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/darwin-arm64/$(BINARY_NAME) ./$(CMD_DIR)
	
	@echo "   - Linux (amd64)"
	@GOOS=linux GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/linux-amd64/$(BINARY_NAME) ./$(CMD_DIR)
	
	@echo "   - Windows (amd64)"
	@GOOS=windows GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/windows-amd64/$(BINARY_NAME).exe ./$(CMD_DIR)
	
	@echo "✅ All builds complete in $(BUILD_DIR)/"

## install: Install binary to system path
install: build
	@echo "🚀 Installing $(BINARY_NAME) to $(INSTALL_DIR)..."
	@mkdir -p $(INSTALL_DIR)
	@rm -f $(INSTALL_DIR)/$(BINARY_NAME)$(BINARY_EXT)
	@cp $(BINARY_PATH) $(INSTALL_DIR)/$(BINARY_NAME)$(BINARY_EXT)
	@chmod +x $(INSTALL_DIR)/$(BINARY_NAME)$(BINARY_EXT)
	@echo "✅ Installed!"

## uninstall: Remove binary from system path
uninstall:
	@echo "🗑️  Uninstalling $(BINARY_NAME)..."
	@rm -f $(INSTALL_DIR)/$(BINARY_NAME)$(BINARY_EXT)
	@echo "✅ Uninstalled!"

## clean: Remove build artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@echo "✅ Clean complete"

## check: Run format, vet, and tests
check:
	@echo "🔍 Running checks..."
	@$(GO) fmt ./...
	@$(GO) vet ./...
	@$(GO) test ./...
	@echo "✅ Checks passed"

## run: Build and run
run: build
	@echo "▶️  Running $(BINARY_NAME)..."
	@$(BINARY_PATH) $(ARGS)

## help: Show help
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'

