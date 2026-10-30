//go:build darwin

package codesign

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const p12Password = "myaaw-local"

// IdentityExists reports whether a code-signing identity with the given name
// is available in the keychain (empty means DefaultIdentity).
func IdentityExists(identity string) bool {
	if identity == "" {
		identity = DefaultIdentity
	}
	return hasIdentity(identity)
}

// Setup creates the local self-signed "Myaaw Agent" certificate when it does
// not exist, so users who only download a release binary can still get a
// stable code identity. Result.Reason is "already exists" when nothing was done.
func Setup(identity string) (Result, error) {
	if identity == "" {
		identity = DefaultIdentity
	}
	if hasIdentity(identity) {
		return Result{Identity: identity, Reason: "already exists"}, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return Result{Identity: identity}, err
	}
	keychain := filepath.Join(home, "Library", "Keychains", "login.keychain-db")

	dir, err := os.MkdirTemp("", "myaaw-codesign-")
	if err != nil {
		return Result{Identity: identity}, err
	}
	defer os.RemoveAll(dir)

	conf := filepath.Join(dir, "openssl.cnf")
	confBody := "[req]\ndistinguished_name = dn\nx509_extensions = v3\nprompt = no\n\n" +
		"[dn]\nCN = " + identity + "\n\n" +
		"[v3]\nkeyUsage = digitalSignature\nextendedKeyUsage = codeSigning\nbasicConstraints = critical,CA:FALSE\n"
	if err := os.WriteFile(conf, []byte(confBody), 0o600); err != nil {
		return Result{Identity: identity}, err
	}

	key := filepath.Join(dir, "key.pem")
	cert := filepath.Join(dir, "cert.pem")
	p12 := filepath.Join(dir, "cert.p12")

	if out, err := run("openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-sha256",
		"-days", "3650", "-keyout", key, "-out", cert, "-config", conf); err != nil {
		return Result{Identity: identity}, fmt.Errorf("openssl req: %w: %s", err, out)
	}
	if out, err := run("openssl", "pkcs12", "-export", "-inkey", key, "-in", cert, "-out", p12,
		"-name", identity, "-passout", "pass:"+p12Password); err != nil {
		return Result{Identity: identity}, fmt.Errorf("openssl pkcs12: %w: %s", err, out)
	}
	if out, err := run("security", "import", p12, "-k", keychain, "-P", p12Password,
		"-T", "/usr/bin/codesign", "-T", "/usr/bin/security"); err != nil {
		return Result{Identity: identity}, fmt.Errorf("security import: %w: %s", err, out)
	}
	if out, err := run("security", "add-trusted-cert", "-r", "trustRoot", "-p", "codeSign",
		"-k", keychain, cert); err != nil {
		return Result{Identity: identity}, fmt.Errorf("security add-trusted-cert: %w: %s (open Keychain Access and trust %q for code signing manually)", err, out, identity)
	}
	if !hasIdentity(identity) {
		return Result{Identity: identity}, fmt.Errorf("identity %q was not found after setup; trust it manually in Keychain Access", identity)
	}
	return Result{Identity: identity, Signed: true}, nil
}

// Remove deletes the local signing identity and its user trust settings.
// Reason is "not found" when there was nothing to remove.
func Remove(identity string) (Result, error) {
	if identity == "" {
		identity = DefaultIdentity
	}
	if !hasIdentity(identity) {
		return Result{Identity: identity, Reason: "not found"}, nil
	}
	out, err := run("security", "delete-identity", "-c", identity, "-t")
	if err != nil {
		return Result{Identity: identity}, fmt.Errorf("security delete-identity: %w: %s", err, out)
	}
	if hasIdentity(identity) {
		return Result{Identity: identity}, fmt.Errorf("identity %q is still present after deletion", identity)
	}
	return Result{Identity: identity, Removed: true}, nil
}

// SignatureInfo reports the code signature of path. adhoc is true when the
// binary is unsigned or ad-hoc signed.
func SignatureInfo(path string) (authority, identifier string, adhoc bool) {
	out, err := exec.Command("codesign", "-dv", "--verbose=2", path).CombinedOutput()
	if err != nil {
		return "", "", true
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Authority="):
			authority = strings.TrimPrefix(line, "Authority=")
		case strings.HasPrefix(line, "Identifier="):
			identifier = strings.TrimPrefix(line, "Identifier=")
		case strings.HasPrefix(line, "Signature=adhoc"):
			adhoc = true
		}
	}
	return authority, identifier, adhoc
}

func run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
