package service

import (
	"fmt"
	"os"
	"path/filepath"

	"myaaw/internal/agentdir"
	"myaaw/internal/capabilities"
)

// ModelCapabilitiesView is the trimmed capability catalog the dashboard uses.
// Metadata keys in the source file are dropped; every model entry keeps the
// fields the picker renders.
type ModelCapabilitiesView = capabilities.View

// ModelCapabilityEntry mirrors one model in model_capabilities.json.
type ModelCapabilityEntry = capabilities.Entry

// ThinkingEntry is the thinking subset the picker needs.
type ThinkingEntry = capabilities.Thinking

// ModelCapabilities reads the global capability catalog. The file under
// ~/.myaaw/config is the only runtime source; the embedded template is copied
// once when the file is missing, then read back from disk so the dashboard
// always reflects the editable file.
func (s *ConfigService) ModelCapabilities() (ModelCapabilitiesView, error) {
	dir, err := agentdir.GlobalConfigDir()
	if err != nil {
		return ModelCapabilitiesView{}, err
	}
	path := filepath.Join(dir, capabilities.FileName)

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
	return capabilities.Parse(data)
}
