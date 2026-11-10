// Package thinking turns the composer's thinking-level choice into a small,
// provider-agnostic spec. It is the single decision point for reasoning
// parameters; provider adapters only serialize the result.
package thinking

import (
	"context"
	"log"
	"strings"

	"myaaw/internal/capabilities"
)

// Spec describes what a provider adapter should send.
type Spec struct {
	// Enabled is true when reasoning should be on, false when the caller asked
	// to disable it explicitly.
	Enabled bool
	// Level is a documented effort token (for example "xhigh"), or empty for the
	// model's own default.
	Level string
}

// Resolve decides whether reasoning parameters should be sent for a model.
//
// The capability catalog is the source of truth. legacyCapable is the adapter's
// pre-catalog predicate, used only for model IDs the catalog does not describe
// yet (aliases and retired snapshots); a model present in the catalog never
// consults it. New models belong in the catalog, not in that predicate.
//
// ok=false means "omit every reasoning parameter". Resolve never reports an
// error: an unknown level or a model that cannot disable reasoning degrades to
// the model default.
func Resolve(model, requested string, legacyCapable bool) (Spec, bool) {
	requested = strings.ToLower(strings.TrimSpace(requested))

	entry, found := capabilities.Lookup(model)
	if !found {
		if !legacyCapable {
			return Spec{}, false
		}
		// No catalog entry means no documented effort list. Keep the legacy
		// behavior: include reasoning with the model's default effort.
		return Spec{Enabled: true}, true
	}
	if !capabilities.Supports(entry, "thinking") {
		return Spec{}, false
	}

	switch requested {
	case "":
		return Spec{Enabled: true}, true
	case "off", "none":
		if hasEffort(entry, "none") || canDisable(entry) {
			return Spec{Enabled: false}, true
		}
		log.Printf("thinking: %s cannot disable reasoning; using the model default", model)
		return Spec{Enabled: true}, true
	}

	if entry.Thinking != nil {
		for _, value := range entry.Thinking.EffortValues {
			if strings.EqualFold(value, requested) {
				return Spec{Enabled: true, Level: value}, true
			}
		}
	}
	log.Printf("thinking: %q is not a documented effort for %s; using the model default", requested, model)
	return Spec{Enabled: true}, true
}

func canDisable(entry capabilities.Entry) bool {
	return entry.Thinking != nil && entry.Thinking.CanDisable != nil && *entry.Thinking.CanDisable
}

func hasEffort(entry capabilities.Entry, want string) bool {
	if entry.Thinking == nil {
		return false
	}
	for _, value := range entry.Thinking.EffortValues {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}

type levelKey struct{}

// WithLevel carries the requested level for the duration of one provider call.
func WithLevel(ctx context.Context, level string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, levelKey{}, strings.TrimSpace(level))
}

// Level returns the requested level, or "" for the model default.
func Level(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	level, _ := ctx.Value(levelKey{}).(string)
	return level
}
