package utils

import (
	"net/url"
	"strings"
)

// ParseAllowedOrigins splits a comma-separated ALLOWED_ORIGINS value and drops
// empty entries, so "https://a, https://b," yields two origins.
func ParseAllowedOrigins(raw string) []string {
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		if origin := strings.TrimSpace(part); origin != "" {
			origins = append(origins, origin)
		}
	}
	return origins
}

// IsLoopbackOrigin reports whether an Origin header points at this machine.
// Browsers on another origin have to opt in through ALLOWED_ORIGINS.
func IsLoopbackOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}
