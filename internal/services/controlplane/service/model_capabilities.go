package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"myaaw/internal/agentdir"
)

// ModelCapabilitiesView is the trimmed capability catalog the dashboard uses.
// Metadata keys in the source file are dropped; every model entry keeps the
// fields the picker renders.
type ModelCapabilitiesView struct {
	Version int                             `json:"version"`
	Models  map[string]ModelCapabilityEntry `json:"models"`
	Warning string                          `json:"warning,omitempty"`
}

// ModelCapabilityEntry mirrors one model in model_capabilities.json.
type ModelCapabilityEntry struct {
	Provider         string           `json:"provider,omitempty"`
	Status           string           `json:"status,omitempty"`
	Capabilities     map[string]*bool `json:"capabilities,omitempty"`
	Thinking         *ThinkingEntry   `json:"thinking,omitempty"`
	InputModalities  []string         `json:"input_modalities,omitempty"`
	OutputModalities []string         `json:"output_modalities,omitempty"`
	ContextWindow    *int             `json:"context_window_tokens,omitempty"`
	BuiltinTools     map[string]*bool `json:"builtin_tools,omitempty"`
}

// ThinkingEntry is the thinking subset the picker needs.
type ThinkingEntry struct {
	CanDisable   *bool    `json:"can_disable,omitempty"`
	EffortValues []string `json:"effort_values,omitempty"`
}

type rawCapabilityEntry struct {
	Provider         string              `json:"provider"`
	Status           string              `json:"status"`
	Capabilities     map[string]*bool    `json:"capabilities"`
	Thinking         *ThinkingEntry      `json:"thinking"`
	InputModalities  []string            `json:"input_modalities"`
	OutputModalities []string            `json:"output_modalities"`
	BuiltinTools     map[string]*bool    `json:"builtin_tools"`
	Limits           *rawCapabilityLimit `json:"limits"`
}

type rawCapabilityLimit struct {
	ContextWindowTokens *int `json:"context_window_tokens"`
}

// ModelCapabilities reads the global capability catalog. The file under
// ~/.myaaw/config is the only runtime source; the embedded template is copied
// once when the file is missing, then read back from disk so the dashboard
// always reflects the editable file.
func (s *ConfigService) ModelCapabilities() (ModelCapabilitiesView, error) {
	dir, err := agentdir.GlobalConfigDir()
	if err != nil {
		return ModelCapabilitiesView{}, err
	}
	path := filepath.Join(dir, "model_capabilities.json")

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if seedErr := agentdir.SeedGlobalDataFiles(); seedErr != nil {
			return ModelCapabilitiesView{
				Version: 1,
				Models:  map[string]ModelCapabilityEntry{},
				Warning: seedErr.Error(),
			}, nil
		}
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return ModelCapabilitiesView{
			Version: 1,
			Models:  map[string]ModelCapabilityEntry{},
			Warning: fmt.Sprintf("read %s: %v", path, err),
		}, nil
	}

	view, parseErr := parseModelCapabilities(data)
	if parseErr != nil {
		return ModelCapabilitiesView{
			Version: 1,
			Models:  map[string]ModelCapabilityEntry{},
			Warning: parseErr.Error(),
		}, nil
	}
	return view, nil
}

func parseModelCapabilities(data []byte) (ModelCapabilitiesView, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return ModelCapabilitiesView{}, fmt.Errorf("model capabilities: invalid JSON: %w", err)
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

	models := make(map[string]ModelCapabilityEntry)
	for name, value := range raw {
		key := strings.TrimSpace(name)
		if key == "" || strings.HasPrefix(key, "_") {
			continue
		}
		var entry rawCapabilityEntry
		if err := json.Unmarshal(value, &entry); err != nil {
			continue
		}
		view := ModelCapabilityEntry{
			Provider:         entry.Provider,
			Status:           entry.Status,
			Capabilities:     entry.Capabilities,
			Thinking:         entry.Thinking,
			InputModalities:  entry.InputModalities,
			OutputModalities: entry.OutputModalities,
			BuiltinTools:     entry.BuiltinTools,
		}
		if entry.Limits != nil {
			view.ContextWindow = entry.Limits.ContextWindowTokens
		}
		models[key] = view
	}
	return ModelCapabilitiesView{Version: version, Models: models}, nil
}
