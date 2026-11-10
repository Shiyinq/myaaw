// Package capabilities owns the model capability catalog that both the control
// plane (dashboard) and the agent runtime read. It intentionally depends only on
// agentdir and the embedded assets so every other package can use it without an
// import cycle.
package capabilities

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"myaaw"
	"myaaw/internal/agentdir"
)

// FileName is the catalog file name inside the shared config folder.
const FileName = "model_capabilities.json"

// embeddedPath mirrors the //go:embed layout in assets.go.
var embeddedPath = path.Join(".myaaw", "config", FileName)

// View is the trimmed catalog the dashboard serves and the runtime reads.
// Metadata keys in the source file are dropped; every model entry keeps the
// fields the picker and the thinking resolver use.
type View struct {
	Version int              `json:"version"`
	Models  map[string]Entry `json:"models"`
	Warning string           `json:"warning,omitempty"`
}

// Entry mirrors one model in model_capabilities.json.
type Entry struct {
	Provider         string           `json:"provider,omitempty"`
	Status           string           `json:"status,omitempty"`
	Capabilities     map[string]*bool `json:"capabilities,omitempty"`
	Thinking         *Thinking        `json:"thinking,omitempty"`
	InputModalities  []string         `json:"input_modalities,omitempty"`
	OutputModalities []string         `json:"output_modalities,omitempty"`
	ContextWindow    *int             `json:"context_window_tokens,omitempty"`
	BuiltinTools     map[string]*bool `json:"builtin_tools,omitempty"`
}

// Thinking is the reasoning subset: whether it can be turned off and which
// effort tokens the provider documents.
type Thinking struct {
	CanDisable   *bool    `json:"can_disable,omitempty"`
	EffortValues []string `json:"effort_values,omitempty"`
}

type rawEntry struct {
	Provider         string           `json:"provider"`
	Status           string           `json:"status"`
	Capabilities     map[string]*bool `json:"capabilities"`
	Thinking         *Thinking        `json:"thinking"`
	InputModalities  []string         `json:"input_modalities"`
	OutputModalities []string         `json:"output_modalities"`
	BuiltinTools     map[string]*bool `json:"builtin_tools"`
	Limits           *rawLimit        `json:"limits"`
}

type rawLimit struct {
	ContextWindowTokens *int `json:"context_window_tokens"`
}

// Parse turns the catalog bytes into a View. Unknown metadata keys are skipped,
// and an entry that cannot be decoded is ignored instead of failing the file.
func Parse(data []byte) (View, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return View{}, fmt.Errorf("model capabilities: invalid JSON: %w", err)
	}

	version := 1
	if schemaData, ok := raw["_schema"]; ok {
		var schema struct {
			Version int `json:"version"`
		}
		if json.Unmarshal(schemaData, &schema) == nil && schema.Version > 0 {
			version = schema.Version
		}
	}

	models := make(map[string]Entry)
	for name, value := range raw {
		key := strings.TrimSpace(name)
		if key == "" || strings.HasPrefix(key, "_") {
			continue
		}
		var entry rawEntry
		if err := json.Unmarshal(value, &entry); err != nil {
			continue
		}
		out := Entry{
			Provider:         entry.Provider,
			Status:           entry.Status,
			Capabilities:     entry.Capabilities,
			Thinking:         entry.Thinking,
			InputModalities:  entry.InputModalities,
			OutputModalities: entry.OutputModalities,
			BuiltinTools:     entry.BuiltinTools,
		}
		if entry.Limits != nil {
			out.ContextWindow = entry.Limits.ContextWindowTokens
		}
		models[key] = out
	}
	return View{Version: version, Models: models}, nil
}

// Supports reports whether one capability flag is explicitly true. A nil map or
// nil flag means "unverified", which never counts as support.
func Supports(entry Entry, capability string) bool {
	if entry.Capabilities == nil {
		return false
	}
	value := entry.Capabilities[capability]
	return value != nil && *value
}

// Lookup finds a model entry, tolerating Gemini's "models/" namespace.
func Lookup(model string) (Entry, bool) {
	view := Load()
	model = strings.TrimSpace(model)
	if entry, ok := view.Models[model]; ok {
		return entry, true
	}
	if trimmed := strings.TrimPrefix(model, "models/"); trimmed != model {
		entry, ok := view.Models[trimmed]
		return entry, ok
	}
	return Entry{}, false
}

type snapshot struct {
	view    View
	path    string
	modTime time.Time
	size    int64
	pinned  bool
}

var (
	mu      sync.Mutex
	current *snapshot
)

// Load returns the catalog, re-reading the global file when it changed on disk.
// A missing or unreadable file falls back to the embedded template, which is how
// a fresh install behaves before EnsureLayout seeds the folder.
func Load() View {
	mu.Lock()
	defer mu.Unlock()

	if current != nil && current.pinned {
		return current.view
	}

	if file, err := agentdir.GlobalConfigPath(FileName); err == nil {
		info, statErr := os.Stat(file)
		if statErr == nil {
			if current != nil && current.path == file &&
				current.modTime.Equal(info.ModTime()) && current.size == info.Size() {
				return current.view
			}
			if data, readErr := os.ReadFile(file); readErr == nil {
				if view, parseErr := Parse(data); parseErr == nil {
					current = &snapshot{view: view, path: file, modTime: info.ModTime(), size: info.Size()}
					return view
				}
			}
		}
	}

	if current != nil && current.path == "" {
		return current.view
	}
	view := embeddedView()
	current = &snapshot{view: view}
	return view
}

func embeddedView() View {
	data, err := myaaw.DefaultMyaawDir.ReadFile(embeddedPath)
	if err != nil {
		return View{Version: 1, Models: map[string]Entry{}}
	}
	view, err := Parse(data)
	if err != nil {
		return View{Version: 1, Models: map[string]Entry{}}
	}
	return view
}

// SetForTest pins a catalog so provider tests do not depend on the developer's
// ~/.myaaw. Reset clears both the pin and the disk cache.
func SetForTest(view View) {
	mu.Lock()
	defer mu.Unlock()
	current = &snapshot{view: view, pinned: true}
}

// Reset clears the cached catalog. Tests use it after changing HOME.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	current = nil
}
