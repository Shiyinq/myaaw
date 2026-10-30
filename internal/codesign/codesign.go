// Package codesign keeps a stable macOS code identity for locally installed
// binaries. macOS binds Screen Recording / Accessibility / Automation grants
// to the binary's code identity, so re-signing every update with the same
// certificate keeps those permissions valid.
package codesign

// DefaultIdentity is the certificate name created by
// scripts/setup-codesign.sh (make codesign-setup).
const DefaultIdentity = "Myaaw Agent"

// DefaultIdentifier is the code identity reported to macOS. It must stay
// stable across builds and releases.
const DefaultIdentifier = "com.myaaw.cli"

// Result describes the outcome of Ensure.
type Result struct {
	Signed   bool
	Removed  bool
	Identity string
	Reason   string // why the action was skipped when !Signed/!Removed; empty when not applicable
}
