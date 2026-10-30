//go:build darwin

package codesign

import (
	"os"
	"os/exec"
	"strings"
)

// Ensure signs path with the local stable identity when it exists. It never
// fails the caller: a missing identity or codesign error is reported in
// Result.Reason. MYAAW_CODESIGN_IDENTITY overrides the certificate name.
func Ensure(path string) Result {
	identity := strings.TrimSpace(os.Getenv("MYAAW_CODESIGN_IDENTITY"))
	if identity == "" {
		identity = DefaultIdentity
	}
	if !hasIdentity(identity) {
		return Result{
			Identity: identity,
			Reason:   "signing identity \"" + identity + "\" not found (run make codesign-setup)",
		}
	}
	out, err := exec.Command("codesign", "--force", "--identifier", DefaultIdentifier, "--sign", identity, path).CombinedOutput()
	if err != nil {
		return Result{Identity: identity, Reason: strings.TrimSpace(string(out))}
	}
	return Result{Signed: true, Identity: identity}
}

func hasIdentity(identity string) bool {
	out, err := exec.Command("security", "find-identity", "-v", "-p", "codesigning").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), identity)
}
