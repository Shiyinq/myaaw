#!/usr/bin/env bash

set -e

REPO="Shiyinq/myaaw"
BIN_NAME="myaaw"
INSTALL_DIR="$HOME/.local/bin"

echo "================================================="
echo "🐈 Welcome to MyAAW (Personal AI Assistant) Setup"
echo "================================================="
echo "Detecting operating system and architecture..."

# Detect OS
OS="$(uname -s)"
case "${OS}" in
    Linux*)     OS_NAME="linux";;
    Darwin*)    OS_NAME="darwin";;
    CYGWIN*|MINGW*|MSYS*) OS_NAME="windows";;
    *)          echo "❌ Unsupported OS: ${OS}"; exit 1;;
esac

# Detect Architecture
ARCH="$(uname -m)"
case "${ARCH}" in
    x86_64)  ARCH_NAME="amd64";;
    arm64|aarch64) ARCH_NAME="arm64";;
    *)       echo "❌ Unsupported Architecture: ${ARCH}"; exit 1;;
esac

if [ "$OS_NAME" = "windows" ]; then
    EXT=".exe"
    # Windows typically doesn't use /usr/local/bin in standard setups without MSYS,
    # but git bash does. We'll try to put it somewhere accessible or current dir.
    INSTALL_DIR="$HOME/AppData/Local/Microsoft/WindowsApps"
else
    EXT=""
fi

ASSET_NAME="${BIN_NAME}-${OS_NAME}-${ARCH_NAME}${EXT}"

echo "Fetching the latest release info for ${OS_NAME}/${ARCH_NAME}..."
LATEST_RELEASE_URL="https://api.github.com/repos/${REPO}/releases/latest"

# Get download URL from the latest release API
DOWNLOAD_URL=$(curl -s "$LATEST_RELEASE_URL" | grep "browser_download_url.*${ASSET_NAME}\"" | cut -d '"' -f 4)

if [ -z "$DOWNLOAD_URL" ]; then
    echo "❌ Error: Could not find binary '${ASSET_NAME}' in the latest release."
    echo "Please check https://github.com/${REPO}/releases"
    exit 1
fi

echo "⬇️  Downloading ${ASSET_NAME}..."
TMP_FILE="/tmp/${BIN_NAME}${EXT}"
curl -fsSL -o "$TMP_FILE" "$DOWNLOAD_URL"
chmod +x "$TMP_FILE"

echo "📦 Installing to ${INSTALL_DIR}..."
mkdir -p "$INSTALL_DIR" || true

mv "$TMP_FILE" "${INSTALL_DIR}/${BIN_NAME}${EXT}"

echo "✅ Installation successful! MyAAW is now installed at ${INSTALL_DIR}/${BIN_NAME}${EXT}."

if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]] && [ "$OS_NAME" != "windows" ]; then
    echo ""
    echo "⚠️  WARNING: ${INSTALL_DIR} is not in your PATH."
    echo "You may need to add the following line to your ~/.zshrc or ~/.bashrc:"
    echo "export PATH=\"\$HOME/.local/bin:\$PATH\""
fi

echo ""
echo "🚀 Launching onboarding process..."
echo ""

"${INSTALL_DIR}/${BIN_NAME}${EXT}" onboard
