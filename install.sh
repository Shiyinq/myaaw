#!/usr/bin/env bash

set -e

REPO="Shiyinq/myaaw"
BIN_NAME="myaaw"
INSTALL_DIR="$HOME/.local/bin"

WEBUI=0
WEBUI_ONLY=0
NO_ONBOARD=0
WEBUI_ARGS=()

while [ "$#" -gt 0 ]; do
    case "$1" in
        --webui) WEBUI=1; shift ;;
        --webui-only) WEBUI_ONLY=1; shift ;;
        --no-onboard) NO_ONBOARD=1; shift ;;
        --public) WEBUI_ARGS+=("--public"); shift ;;
        --connect|--webui-port|--webui-version)
            if [ "$#" -lt 2 ]; then
                echo "❌ $1 requires a value"
                exit 1
            fi
            case "$1" in
                --connect) WEBUI_ARGS+=("--connect" "$2") ;;
                --webui-port) WEBUI_ARGS+=("--port" "$2") ;;
                --webui-version) WEBUI_ARGS+=("--version" "$2") ;;
            esac
            shift 2
            ;;
        -h|--help)
            echo "Usage: install.sh [--webui] [--webui-only] [--no-onboard] [--public]"
            echo "                   [--connect URL] [--webui-port PORT] [--webui-version TAG]"
            echo ""
            echo "  (no flags)       Install the binary and run onboarding"
            echo "  --webui          Install the binary, then the dashboard"
            echo "  --webui-only     Install only the dashboard (no binary)"
            echo "  --no-onboard     Skip the onboarding wizard"
            echo "  --connect URL    Dashboard: use a remote control plane URL"
            echo "  --webui-port N   Dashboard port (default 5050)"
            echo "  --webui-version  Dashboard release tag (default: latest)"
            echo "  --public         Dashboard: disable /setup and host hints (public hosting)"
            exit 0
            ;;
        *)
            echo "❌ Unknown option: $1"
            exit 1
            ;;
    esac
done

install_webui() {
    curl -fsSL https://raw.githubusercontent.com/Shiyinq/myaaw/main/webui-install.sh \
        | bash -s -- "${WEBUI_ARGS[@]}"
}

if [ "$WEBUI_ONLY" -eq 1 ]; then
    install_webui
    exit 0
fi

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

# macOS: re-sign with the local stable identity so Screen Recording /
# Accessibility / Automation grants survive updates.
if [ "$OS_NAME" = "darwin" ]; then
    if security find-identity -v -p codesigning 2>/dev/null | grep -qF "Myaaw Agent"; then
        if codesign --force --identifier com.myaaw.cli --sign "Myaaw Agent" "${INSTALL_DIR}/${BIN_NAME}${EXT}"; then
            echo "🔏 Signed with 'Myaaw Agent' (stable macOS identity)."
        else
            echo "⚠️  codesign failed; macOS permissions may reset after updates."
        fi
    else
        echo "ℹ️  No 'Myaaw Agent' signing identity found."
        echo "    macOS permissions may reset after updates. Create it once with:"
        echo "      ${INSTALL_DIR}/${BIN_NAME} codesign setup"
        echo "    (or 'make codesign-setup' from a source checkout), then re-run this"
        echo "    installer or 'myaaw update' so the binary gets signed."
    fi
fi

echo "✅ Installation successful! MyAAW is now installed at ${INSTALL_DIR}/${BIN_NAME}${EXT}."

if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]] && [ "$OS_NAME" != "windows" ]; then
    echo ""
    echo "⚠️  WARNING: ${INSTALL_DIR} is not in your PATH."
    echo "You may need to add the following line to your ~/.zshrc or ~/.bashrc:"
    echo "export PATH=\"\$HOME/.local/bin:\$PATH\""
fi

if [ "$NO_ONBOARD" -eq 0 ]; then
    echo ""
    echo "🚀 Launching onboarding process..."
    echo ""

    "${INSTALL_DIR}/${BIN_NAME}${EXT}" onboard
fi

if [ "$WEBUI" -eq 1 ]; then
    echo ""
    echo "🖥  Installing the optional dashboard..."
    echo ""
    install_webui
fi
