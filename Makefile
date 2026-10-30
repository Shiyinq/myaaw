.PHONY: all build build-all install uninstall clean check run help swagger swagger-controlplane sign codesign-setup

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

# macOS code signing: a stable identity keeps TCC grants (Screen Recording,
# Accessibility, Automation) valid across rebuilds. Set up the certificate
# once with `make codesign-setup`, then signing happens automatically.
CODESIGN_IDENTITY?=Myaaw Agent
CODESIGN_ID=com.myaaw.cli
CODESIGN_CHECK=security find-identity -v -p codesigning 2>/dev/null | grep -qF "$(CODESIGN_IDENTITY)"

# Default target
all: help

## build: Build the binary for current platform
build: swagger swagger-controlplane
	@echo "🚧 Building $(BINARY_NAME) for $(PLATFORM)/$(ARCH)..."
	@echo "   Version: $(VERSION)"
	@mkdir -p $(BUILD_DIR)
	@$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BINARY_PATH) ./$(CMD_DIR)
	@if [ "$(UNAME_S)" = "Darwin" ] && $(CODESIGN_CHECK); then \
		codesign --force --identifier $(CODESIGN_ID) --sign "$(CODESIGN_IDENTITY)" $(BINARY_PATH); \
		echo "🔏 Signed with '$(CODESIGN_IDENTITY)'"; \
	fi
	@echo "✅ Build complete: $(BINARY_PATH)"

## sign: Codesign the built binary with the stable identity (macOS only)
sign:
	@if [ "$(UNAME_S)" != "Darwin" ]; then \
		echo "ℹ️  codesign skipped (not macOS)"; \
	elif $(CODESIGN_CHECK); then \
		codesign --force --identifier $(CODESIGN_ID) --sign "$(CODESIGN_IDENTITY)" $(BINARY_PATH); \
		echo "🔏 Signed $(BINARY_PATH) with '$(CODESIGN_IDENTITY)'"; \
	else \
		echo "⚠️  Signing identity '$(CODESIGN_IDENTITY)' not found."; \
		echo "    Run: myaaw codesign setup (no checkout needed), or ./scripts/setup-codesign.sh"; \
	fi

## codesign-setup: Create the self-signed 'Myaaw Agent' signing certificate (macOS)
codesign-setup:
	@./scripts/setup-codesign.sh

## swagger: Generate Swagger documentation
swagger:
	@echo "📝 Generating Swagger documentation..."
	@mkdir -p docs/swagger
	# NB: do NOT add --parseGoList=false here — it makes swag recursively parse
	# the Go stdlib + all dependency sources and can take over an hour.
	@swag init -g $(MAIN_GO) --parseDependency --parseInternal --output docs/swagger --exclude .myaaw,internal/services/controlplane
	@echo "✅ Swagger generated"

## swagger-controlplane: Generate Swagger documentation for `myaaw api`
swagger-controlplane:
	@echo "📝 Generating control plane Swagger documentation..."
	@mkdir -p docs/controlplane
	@swag init -g internal/services/controlplane/docs.go --parseDependency --parseInternal --output docs/controlplane --exclude .myaaw,internal/services/bot/handler,internal/services/queue/handler --instanceName controlplane
	@echo "✅ Control plane Swagger generated"

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
	@if [ "$(UNAME_S)" = "Darwin" ] && $(CODESIGN_CHECK); then \
		codesign --force --identifier $(CODESIGN_ID) --sign "$(CODESIGN_IDENTITY)" $(INSTALL_DIR)/$(BINARY_NAME)$(BINARY_EXT); \
	fi
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

