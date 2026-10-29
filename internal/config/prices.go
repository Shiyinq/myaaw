package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"myaaw/internal/agentdir"
)

// Price file names live next to config.json. They exist so a long price table
// does not bloat the agent config.
const (
	ModelPricesFile = "model_prices.json"
	ToolPricesFile  = "tool_prices.json"
)

// LoadPriceFiles reads model_prices.json and tool_prices.json from dir. Missing
// files are not an error; invalid files produce warnings and are ignored so the
// agent keeps running with the inline or built-in prices.
func LoadPriceFiles(dir string) (models map[string]ModelPriceConfig, tools map[string]ToolPriceConfig, warnings []string) {
	models, warnings = loadModelPricesFile(filepath.Join(dir, ModelPricesFile))
	var toolWarnings []string
	tools, toolWarnings = loadToolPricesFile(filepath.Join(dir, ToolPricesFile))
	return models, tools, append(warnings, toolWarnings...)
}

// WriteModelPricesFile writes the model price table atomically.
func WriteModelPricesFile(dir string, models map[string]ModelPriceConfig) error {
	return writeJSONAtomic(filepath.Join(dir, ModelPricesFile), models)
}

// WriteToolPricesFile writes the tool price table atomically.
func WriteToolPricesFile(dir string, tools map[string]ToolPriceConfig) error {
	return writeJSONAtomic(filepath.Join(dir, ToolPricesFile), tools)
}

// ApplyPriceFiles overlays the price files of the active config directory on
// top of the inline values and returns any warnings.
func ApplyPriceFiles() []string {
	dir, ok := configFileDir()
	if !ok {
		return nil
	}
	return applyPriceFilesAt(dir)
}

func applyPriceFilesAt(dir string) []string {
	models, tools, warnings := LoadPriceFiles(dir)
	if len(models) > 0 {
		ObsModelPrices = mergeModelPrices(ObsModelPrices, models)
	}
	if len(tools) > 0 {
		ObsToolPrices = mergeToolPrices(ObsToolPrices, tools)
	}
	return warnings
}

// configFileDir mirrors loadJSONConfig's resolution: the active agent config
// directory, then the current directory.
func configFileDir() (string, bool) {
	if path, err := agentdir.ActiveConfigPath(); err == nil {
		if _, err := os.Stat(path); err == nil {
			return filepath.Dir(path), true
		}
	}
	if _, err := os.Stat("config.json"); err == nil {
		return ".", true
	}
	return "", false
}

func loadModelPricesFile(path string) (map[string]ModelPriceConfig, []string) {
	raw, warnings := readPriceEntries(path, "model prices")
	if raw == nil {
		return nil, warnings
	}
	out := make(map[string]ModelPriceConfig, len(raw))
	for name, value := range raw {
		var price ModelPriceConfig
		if err := json.Unmarshal(value, &price); err != nil {
			warnings = append(warnings, fmt.Sprintf("model prices: %s: invalid entry %q: %v", path, name, err))
			continue
		}
		if price.Input < 0 || price.CacheHit < 0 || price.Output < 0 ||
			(price.OffPeak != nil && (price.OffPeak.Input < 0 || price.OffPeak.CacheHit < 0 || price.OffPeak.Output < 0)) {
			warnings = append(warnings, fmt.Sprintf("model prices: %s: negative price for %q", path, name))
			continue
		}
		out[name] = price
	}
	return out, warnings
}

func loadToolPricesFile(path string) (map[string]ToolPriceConfig, []string) {
	raw, warnings := readPriceEntries(path, "tool prices")
	if raw == nil {
		return nil, warnings
	}
	out := make(map[string]ToolPriceConfig, len(raw))
	for name, value := range raw {
		var price ToolPriceConfig
		if err := json.Unmarshal(value, &price); err != nil {
			warnings = append(warnings, fmt.Sprintf("tool prices: %s: invalid entry %q: %v", path, name, err))
			continue
		}
		if price.PerCall < 0 {
			warnings = append(warnings, fmt.Sprintf("tool prices: %s: negative price for %q", path, name))
			continue
		}
		out[name] = price
	}
	return out, warnings
}

// readPriceEntries parses a price file into raw entries, dropping empty and
// underscore-prefixed keys so files can carry "_comment" metadata.
func readPriceEntries(path, kind string) (map[string]json.RawMessage, []string) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: cannot read %s: %v", kind, path, err)}
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, []string{fmt.Sprintf("%s: invalid JSON in %s: %v", kind, path, err)}
	}
	out := make(map[string]json.RawMessage, len(raw))
	for name, value := range raw {
		key := strings.TrimSpace(name)
		if key == "" || strings.HasPrefix(key, "_") {
			continue
		}
		out[key] = value
	}
	return out, nil
}

func mergeModelPrices(base, override map[string]ModelPriceConfig) map[string]ModelPriceConfig {
	if len(override) == 0 {
		return base
	}
	out := make(map[string]ModelPriceConfig, len(base)+len(override))
	for name, price := range base {
		out[name] = price
	}
	for name, price := range override {
		out[name] = price
	}
	return out
}

func mergeToolPrices(base, override map[string]ToolPriceConfig) map[string]ToolPriceConfig {
	if len(override) == 0 {
		return base
	}
	out := make(map[string]ToolPriceConfig, len(base)+len(override))
	for name, price := range base {
		out[name] = price
	}
	for name, price := range override {
		out[name] = price
	}
	return out
}

func writeJSONAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "    ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".price-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		return err
	}
	return os.Rename(name, path)
}
