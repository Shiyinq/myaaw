#!/usr/bin/env bash
# Install the Myaaw WebUI (dashboard) without the myaaw binary.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/Shiyinq/myaaw/main/webui-install.sh | bash
#   curl -fsSL ... | bash -s -- --connect https://abc.ngrok-free.app
#
# Options:
#   --version <tag>   Install a specific release tag (default: latest)
#   --port <port>     WebUI port (default: 5050)
#   --api-url <url>   Control-plane URL (default: http://localhost:9000)
#   --connect <url>   Alias for --api-url (remote control plane)
#   -h, --help        Show this help

set -e

REPO="Shiyinq/myaaw"
WEBUI_DIR="$HOME/.myaaw/webui"
PORT="5050"
API_URL="http://localhost:9000"
VERSION=""

while [ "$#" -gt 0 ]; do
    case "$1" in
        --version) VERSION="$2"; shift 2 ;;
        --port) PORT="$2"; shift 2 ;;
        --api-url) API_URL="$2"; shift 2 ;;
        --connect) API_URL="$2"; shift 2 ;;
        		-h|--help)
            			echo "Usage: webui-install.sh [--version TAG] [--port PORT] [--api-url URL] [--connect URL]"
            			exit 0
            			;;
        *)
            echo "❌ Unknown option: $1"
            exit 1
            ;;
    esac
done

echo "================================================="
echo "🐈 Welcome to Myaaw WebUI Setup"
echo "================================================="

if ! command -v node >/dev/null 2>&1; then
    echo "❌ Node.js 20+ is required but was not found in PATH."
    echo "   Install it from https://nodejs.org and re-run this script."
    exit 1
fi

if [ -n "$VERSION" ]; then
    RELEASE_URL="https://api.github.com/repos/${REPO}/releases/tags/${VERSION}"
else
    RELEASE_URL="https://api.github.com/repos/${REPO}/releases/latest"
fi

echo "Fetching the latest WebUI bundle..."
DOWNLOAD_URL=$(curl -s "$RELEASE_URL" | grep "browser_download_url.*myaaw-webui-.*\.tar\.gz\"" | cut -d '"' -f 4)

if [ -z "$DOWNLOAD_URL" ]; then
    echo "❌ Could not find a WebUI bundle in the release."
    echo "   Check https://github.com/${REPO}/releases"
    exit 1
fi

echo "⬇️  Downloading $(basename "$DOWNLOAD_URL")..."
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT
curl -fsSL -o "$TMP_DIR/webui.tar.gz" "$DOWNLOAD_URL"

echo "📦 Installing to ${WEBUI_DIR}/current..."
mkdir -p "$WEBUI_DIR"
rm -rf "$WEBUI_DIR/current"
mkdir -p "$WEBUI_DIR/current"
tar -xzf "$TMP_DIR/webui.tar.gz" -C "$WEBUI_DIR/current"

cat > "$WEBUI_DIR/config.json" <<EOF
{
  "port": "${PORT}",
  "api_url": "${API_URL}",
  "version": "${VERSION:-latest}"
}
EOF

echo "✅ WebUI installed at ${WEBUI_DIR}/current"
echo "🚀 Starting on http://127.0.0.1:${PORT} (control plane: ${API_URL})"

cd "$WEBUI_DIR/current"
nohup env PORT="$PORT" HOST=127.0.0.1 MYAAW_API_BASE_URL="$API_URL" \
    node build/index.js > "$WEBUI_DIR/webui.log" 2>&1 &

echo "✅ WebUI started (PID: $!)"
echo "📄 Logs: ${WEBUI_DIR}/webui.log"
echo ""
echo "Open http://127.0.0.1:${PORT} in your browser."

if [ "$API_URL" = "http://localhost:9000" ]; then
    echo ""
    echo "ℹ️  No myaaw binary detected in this flow? The dashboard's /setup page"
    echo "   can download and install it, or connect to a remote control plane."
fi
