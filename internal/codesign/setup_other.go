//go:build !darwin

package codesign

// IdentityExists reports whether the local signing identity exists. Always
// false outside macOS.
func IdentityExists(identity string) bool {
	return false
}

// Setup is a no-op outside macOS.
func Setup(identity string) (Result, error) {
	if identity == "" {
		identity = DefaultIdentity
	}
	return Result{Identity: identity, Reason: "not macOS"}, nil
}

// Remove is a no-op outside macOS.
func Remove(identity string) (Result, error) {
	if identity == "" {
		identity = DefaultIdentity
	}
	return Result{Identity: identity, Reason: "not macOS"}, nil
}

// SignatureInfo returns empty values outside macOS.
func SignatureInfo(path string) (authority, identifier string, adhoc bool) {
	_ = path
	return "", "", true
}
