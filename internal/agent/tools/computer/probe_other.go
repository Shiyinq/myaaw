//go:build !darwin

package computer

import "context"

// Probe checks whether desktop automation can run on this machine. Outside
// macOS the darwin backend is unavailable, so every check collapses into a
// single unsupported-platform result.
func Probe(ctx context.Context) []ProbeResult {
	return []ProbeResult{{
		Name:   "Operating system",
		OK:     false,
		Detail: "desktop automation is macOS-only",
		Fix:    "use the cua-driver MCP route for desktop automation on this platform",
	}}
}
