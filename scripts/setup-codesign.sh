#!/usr/bin/env bash
# Create a stable self-signed code-signing certificate for local macOS builds.
#
# macOS ties Screen Recording / Accessibility / Automation grants to the
# binary's code identity. An unsigned (ad-hoc signed) binary gets a new
# identity on every rebuild, which silently invalidates those grants. Signing
# with a stable certificate keeps the TCC grants valid across rebuilds.
#
# Run once:   ./scripts/setup-codesign.sh   (or: make codesign-setup)
# Then:       make install                 (signing happens automatically)
#
# Installed binaries expose the same flow as `myaaw codesign setup` (implemented
# in internal/codesign), so users without this checkout can create the identity
# too. The two paths are independent but produce the same "Myaaw Agent" cert.
#
# The script may prompt for your login keychain password and for permission to
# sign with the new key; choose "Always Allow" so future builds do not prompt.

set -euo pipefail

IDENTITY="Myaaw Agent"

if [ "$(uname -s)" != "Darwin" ]; then
  echo "This script is macOS-only."
  exit 0
fi

if security find-identity -v -p codesigning 2>/dev/null | grep -qF "$IDENTITY"; then
  echo "✅ Signing identity '$IDENTITY' already exists."
  exit 0
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

KEY="$TMP_DIR/key.pem"
CERT="$TMP_DIR/cert.pem"
P12="$TMP_DIR/cert.p12"
CONF="$TMP_DIR/openssl.cnf"
PASS="myaaw-local"
KEYCHAIN="$HOME/Library/Keychains/login.keychain-db"

cat > "$CONF" <<EOF
[req]
distinguished_name = dn
x509_extensions = v3
prompt = no

[dn]
CN = $IDENTITY

[v3]
keyUsage = digitalSignature
extendedKeyUsage = codeSigning
basicConstraints = critical,CA:FALSE
EOF

echo "🔐 Generating self-signed code-signing certificate '$IDENTITY'..."
openssl req -x509 -newkey rsa:2048 -nodes -sha256 -days 3650 \
  -keyout "$KEY" -out "$CERT" -config "$CONF"

openssl pkcs12 -export -inkey "$KEY" -in "$CERT" -out "$P12" \
  -name "$IDENTITY" -passout "pass:$PASS"

echo "📥 Importing into the login keychain..."
security import "$P12" -k "$KEYCHAIN" -P "$PASS" \
  -T /usr/bin/codesign -T /usr/bin/security

cat <<'NOTE'
ℹ️  The FIRST signing run may show a dialog:
      "codesign wants to access key 'cert' in your keychain."
    Enter your macOS login password and click "Always Allow" so later
    builds sign without prompting.
NOTE

echo "🛡️  Trusting the certificate for code signing (user domain)..."
security add-trusted-cert -r trustRoot -p codeSign -k "$KEYCHAIN" "$CERT" || {
  echo "⚠️  Could not add trust automatically."
  echo "    Open Keychain Access → login → Certificates → '$IDENTITY' →"
  echo "    Get Info → Trust → Code Signing: Always Trust."
}

echo ""
if security find-identity -v -p codesigning 2>/dev/null | grep -qF "$IDENTITY"; then
  echo "✅ Identity '$IDENTITY' is ready."
  echo "   Rebuild and reinstall: make install"
  echo "   Sign an existing build:  make sign"
  echo ""
  echo "   Note: the identity changes from ad-hoc to this certificate once, so"
  echo "   re-grant Screen Recording, Accessibility, and Automation after the"
  echo "   first signed install. After that the grants survive rebuilds."
else
  echo "❌ Identity not found. Open Keychain Access → login → Certificates →"
  echo "   '$IDENTITY' → Get Info → Trust → Code Signing: Always Trust, then"
  echo "   re-run this script."
  exit 1
fi
