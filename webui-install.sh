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
#   --public          Public hosting: disable /setup and hide host hints
#   -h, --help        Show this help

set -e

REPO="Shiyinq/myaaw"
WEBUI_DIR="$HOME/.myaaw/webui"
PORT="5050"
API_URL="http://localhost:9000"
VERSION=""
PUBLIC=0

while [ "$#" -gt 0 ]; do
    case "$1" in
        --version) VERSION="$2"; shift 2 ;;
        --port) PORT="$2"; shift 2 ;;
        --api-url) API_URL="$2"; shift 2 ;;
        --connect) API_URL="$2"; shift 2 ;;
        --public) PUBLIC=1; shift ;;
        		-h|--help)
            			echo "Usage: webui-install.sh [--version TAG] [--port PORT] [--api-url URL] [--connect URL] [--public]"
            			exit 0
            			;;
        *)
            echo "Error: Unknown option: $1"
            exit 1
            ;;
    esac
done

echo "================================================="
echo "Welcome to Myaaw WebUI Setup"
echo "================================================="

if ! command -v node >/dev/null 2>&1; then
    echo "Error: Node.js 22+ is required but was not found in PATH."
    echo "   Install it from https://nodejs.org and re-run this script."
    exit 1
fi

NODE_MAJOR=$(node -p 'process.versions.node.split(".")[0]' 2>/dev/null)
if [ -z "$NODE_MAJOR" ] || [ "$NODE_MAJOR" -lt 22 ]; then
    echo "Error: Node.js 22+ is required, found $(node --version 2>/dev/null)."
    echo "   Install it from https://nodejs.org and re-run this script."
    exit 1
fi

if [ -n "$VERSION" ]; then
    RELEASE_URL="https://api.github.com/repos/${REPO}/releases/tags/${VERSION}"
else
    RELEASE_URL="https://api.github.com/repos/${REPO}/releases/latest"
fi

echo "Fetching the WebUI bundle..."
RELEASE_METADATA=$(curl -fsSL "$RELEASE_URL" | node -e '
    const release = JSON.parse(require("node:fs").readFileSync(0, "utf8"));
    const tag = release.tag_name;
    const bundle = release.assets?.find((asset) => /^myaaw-webui-.*\.tar\.gz$/.test(asset.name));
    const url = bundle?.browser_download_url;
    const checksums = release.assets?.find((asset) => asset.name === "SHA256SUMS");
    if (typeof tag === "string" && tag && typeof url === "string" && url) {
        console.log(tag);
        console.log(url);
        console.log(bundle.name);
        console.log(checksums?.browser_download_url ?? "");
    }
')
RELEASE_TAG=$(printf '%s\n' "$RELEASE_METADATA" | sed -n '1p')
DOWNLOAD_URL=$(printf '%s\n' "$RELEASE_METADATA" | sed -n '2p')
ASSET_NAME=$(printf '%s\n' "$RELEASE_METADATA" | sed -n '3p')
CHECKSUM_URL=$(printf '%s\n' "$RELEASE_METADATA" | sed -n '4p')

if [ -z "$RELEASE_TAG" ] || [ -z "$DOWNLOAD_URL" ]; then
    echo "Error: Could not find a WebUI bundle in the release."
    echo "   Check https://github.com/${REPO}/releases"
    exit 1
fi

echo "Downloading ${ASSET_NAME}..."
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT
curl -fsSL -o "$TMP_DIR/webui.tar.gz" "$DOWNLOAD_URL"

if [ -n "$CHECKSUM_URL" ]; then
    echo "Verifying WebUI bundle checksum..."
    curl -fsSL -o "$TMP_DIR/SHA256SUMS" "$CHECKSUM_URL"
    node - "$TMP_DIR/webui.tar.gz" "$TMP_DIR/SHA256SUMS" "$ASSET_NAME" <<'NODE'
const { createReadStream, readFileSync } = require('node:fs');
const { createHash } = require('node:crypto');
const [bundle, checksums, asset] = process.argv.slice(2);

(async () => {
    const entry = readFileSync(checksums, 'utf8').split(/\r?\n/)
        .map((line) => line.trim().split(/\s+/))
        .find(([, name]) => name?.replace(/^\*/, '') === asset);
    if (!entry) throw new Error(`SHA256SUMS does not list ${asset}`);

    const hash = createHash('sha256');
    for await (const chunk of createReadStream(bundle)) hash.update(chunk);
    if (hash.digest('hex') !== entry[0].toLowerCase()) {
        throw new Error(`Checksum mismatch for ${asset}`);
    }
})().catch((error) => {
    console.error(error.message);
    process.exitCode = 1;
});
NODE
fi

echo "Installing to ${WEBUI_DIR}/current..."
mkdir -p "$WEBUI_DIR"
rm -rf "$WEBUI_DIR/current"
mkdir -p "$WEBUI_DIR/current"
tar -xzf "$TMP_DIR/webui.tar.gz" -C "$WEBUI_DIR/current"

PUBLIC_JSON="false"
if [ "$PUBLIC" -eq 1 ]; then
    PUBLIC_JSON="true"
fi

node -e '
    const [port, api_url, version, publicMode] = process.argv.slice(1);
    console.log(JSON.stringify({ port, api_url, version, public: publicMode === "true" }, null, 2));
' "$PORT" "$API_URL" "$RELEASE_TAG" "$PUBLIC_JSON" > "$WEBUI_DIR/config.json"

echo "WebUI installed at ${WEBUI_DIR}/current"
echo "Starting on http://127.0.0.1:${PORT} (control plane: ${API_URL})"

cd "$WEBUI_DIR/current"
ENV_ARGS=(PORT="$PORT" HOST=127.0.0.1 MYAAW_API_BASE_URL="$API_URL")
if [ "$PUBLIC" -eq 1 ]; then
    ENV_ARGS+=(MYAAW_WEBUI_PUBLIC=1)
fi
nohup env "${ENV_ARGS[@]}" node build/index.js > "$WEBUI_DIR/webui.log" 2>&1 &

echo "WebUI started (PID: $!)"
echo "Logs: ${WEBUI_DIR}/webui.log"
echo ""
echo "Open http://127.0.0.1:${PORT} in your browser."

if [ "$PUBLIC" -eq 1 ]; then
    echo "Public hosting: /setup and the login host hints are disabled."
    echo "   Visitors only need the tunnel URL, username, and password."
elif [ "$API_URL" = "http://localhost:9000" ] &&
    ! command -v myaaw >/dev/null 2>&1 &&
    [ ! -x "$HOME/.local/bin/myaaw" ]; then
    echo ""
    echo "Myaaw binary not found in PATH or ~/.local/bin. Open /setup to install it"
    echo "or connect to a remote control plane."
fi
