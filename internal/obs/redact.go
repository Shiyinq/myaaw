package obs

import (
	"log"
	"regexp"
)

// defaultRedactPatterns covers the secrets most likely to appear in a prompt or
// a tool argument. They are conservative on purpose: a false positive costs a
// few masked characters in a trace, a false negative leaks a credential into
// the database and the OTLP collector.
var defaultRedactPatterns = []string{
	`(?i)bearer\s+[a-z0-9._~+/=-]{16,}`,
	`\bsk-[A-Za-z0-9_-]{16,}`,
	`\bgh[pousr]_[A-Za-z0-9]{20,}`,
	`\bxox[abprs]-[A-Za-z0-9-]{10,}`,
}

// DefaultRedactPatterns returns a copy of the built-in patterns, for callers
// that need to show the effective configuration.
func DefaultRedactPatterns() []string {
	return append([]string(nil), defaultRedactPatterns...)
}

// compileRedactPatterns builds the matchers once per recorder. An invalid
// pattern is skipped with a warning: losing redaction for one pattern must not
// disable recording.
func compileRedactPatterns(patterns []string) []*regexp.Regexp {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		expression, err := regexp.Compile(pattern)
		if err != nil {
			log.Printf("[obs] ignoring invalid redact pattern %q: %v", pattern, err)
			continue
		}
		compiled = append(compiled, expression)
	}
	return compiled
}
