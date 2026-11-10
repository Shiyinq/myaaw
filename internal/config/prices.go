package config

import (
	"encoding/json"
	"fmt"
	"myaaw"
	"os"
	"path"
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

// LoadPriceFiles reads one price directory. Missing files are not an error;
// invalid files produce warnings and are ignored so the agent keeps running.
func LoadPriceFiles(dir string) (models map[string]ModelPriceConfig, tools map[string]ToolPriceConfig, warnings []string) {
	models, warnings = loadModelPricesFile(filepath.Join(dir, ModelPricesFile))
	var toolWarnings []string
	tools, toolWarnings = loadToolPricesFile(filepath.Join(dir, ToolPricesFile))
	return models, tools, append(warnings, toolWarnings...)
}

// LoadGlobalPriceFiles reads the shared tables under ~/.myaaw/config.
func LoadGlobalPriceFiles() (models map[string]ModelPriceConfig, tools map[string]ToolPriceConfig, warnings []string) {
	dir, err := agentdir.GlobalConfigDir()
	if err != nil {
		return nil, nil, []string{fmt.Sprintf("global config: %v", err)}
	}
	return LoadPriceFiles(dir)
}

// LoadEffectivePriceFiles applies the runtime precedence: global table first,
// then an optional per-agent override. When neither file exists it falls back
// to the embedded JSON template, never to the retired Go built-in table.
func LoadEffectivePriceFiles(overrideDir string) (models map[string]ModelPriceConfig, tools map[string]ToolPriceConfig, warnings []string) {
	models, tools, warnings = LoadGlobalPriceFiles()

	if strings.TrimSpace(overrideDir) != "" {
		overrideModels, overrideTools, overrideWarnings := LoadPriceFiles(overrideDir)
		warnings = append(warnings, overrideWarnings...)
		models = mergeModelPrices(models, overrideModels)
		tools = mergeToolPrices(tools, overrideTools)
	}

	if len(models) == 0 || len(tools) == 0 {
		embeddedModels, embeddedTools, embeddedWarnings := loadEmbeddedPriceFiles()
		warnings = append(warnings, embeddedWarnings...)
		if len(models) == 0 {
			models = embeddedModels
		}
		if len(tools) == 0 {
			tools = embeddedTools
		}
	}
	return models, tools, warnings
}

// WriteModelPricesFile writes the model price table atomically.
func WriteModelPricesFile(dir string, models map[string]ModelPriceConfig) error {
	return writeJSONAtomic(filepath.Join(dir, ModelPricesFile), models)
}

// WriteToolPricesFile writes the tool price table atomically.
func WriteToolPricesFile(dir string, tools map[string]ToolPriceConfig) error {
	return writeJSONAtomic(filepath.Join(dir, ToolPricesFile), tools)
}

// ApplyPriceFiles overlays the effective price tables over the inline values
// and returns any warnings.
func ApplyPriceFiles() []string {
	overrideDir, _ := configFileDir()
	return applyEffectivePriceFiles(overrideDir)
}

func applyEffectivePriceFiles(overrideDir string) []string {
	models, tools, warnings := LoadEffectivePriceFiles(overrideDir)
	if len(models) > 0 {
		ObsModelPrices = mergeModelPrices(ObsModelPrices, models)
	}
	if len(tools) > 0 {
		ObsToolPrices = mergeToolPrices(ObsToolPrices, tools)
	}
	return warnings
}

// applyPriceFilesAt overlays one directory only; kept for callers and tests
// that already resolved the directory.
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
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []string{fmt.Sprintf("model prices: cannot read %s: %v", path, err)}
	}
	return parseModelPricesData(data, path)
}

func parseModelPricesData(data []byte, source string) (map[string]ModelPriceConfig, []string) {
	raw, warnings := parsePriceEntries(data, source, "model prices")
	if raw == nil {
		return nil, warnings
	}
	out := make(map[string]ModelPriceConfig, len(raw))
	for name, value := range raw {
		var price ModelPriceConfig
		if err := json.Unmarshal(value, &price); err != nil {
			warnings = append(warnings, fmt.Sprintf("model prices: %s: invalid entry %q: %v", source, name, err))
			continue
		}
		if price.Input < 0 || price.CacheHit < 0 || price.Output < 0 ||
			(price.OffPeak != nil && (price.OffPeak.Input < 0 || price.OffPeak.CacheHit < 0 || price.OffPeak.Output < 0)) {
			warnings = append(warnings, fmt.Sprintf("model prices: %s: negative price for %q", source, name))
			continue
		}
		out[name] = price
	}
	return out, warnings
}

func loadToolPricesFile(path string) (map[string]ToolPriceConfig, []string) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []string{fmt.Sprintf("tool prices: cannot read %s: %v", path, err)}
	}
	return parseToolPricesData(data, path)
}

func parseToolPricesData(data []byte, source string) (map[string]ToolPriceConfig, []string) {
	raw, warnings := parsePriceEntries(data, source, "tool prices")
	if raw == nil {
		return nil, warnings
	}
	out := make(map[string]ToolPriceConfig, len(raw))
	for name, value := range raw {
		var price ToolPriceConfig
		if err := json.Unmarshal(value, &price); err != nil {
			warnings = append(warnings, fmt.Sprintf("tool prices: %s: invalid entry %q: %v", source, name, err))
			continue
		}
		if price.PerCall < 0 {
			warnings = append(warnings, fmt.Sprintf("tool prices: %s: negative price for %q", source, name))
			continue
		}
		out[name] = price
	}
	return out, warnings
}

// loadEmbeddedPriceFiles is the bootstrap fallback when no editable file exists
// yet. It parses the JSON template shipped with the binary.
func loadEmbeddedPriceFiles() (map[string]ModelPriceConfig, map[string]ToolPriceConfig, []string) {
	var warnings []string

	var models map[string]ModelPriceConfig
	if data, err := myaaw.DefaultMyaawDir.ReadFile(path.Join(".myaaw", "config", ModelPricesFile)); err == nil {
		var parseWarnings []string
		models, parseWarnings = parseModelPricesData(data, "embedded "+ModelPricesFile)
		warnings = append(warnings, parseWarnings...)
	}

	var tools map[string]ToolPriceConfig
	if data, err := myaaw.DefaultMyaawDir.ReadFile(path.Join(".myaaw", "config", ToolPricesFile)); err == nil {
		var parseWarnings []string
		tools, parseWarnings = parseToolPricesData(data, "embedded "+ToolPricesFile)
		warnings = append(warnings, parseWarnings...)
	}

	return models, tools, warnings
}

// parsePriceEntries parses a price file into raw entries, dropping empty and
// underscore-prefixed keys so files can carry "_comment" metadata.
func parsePriceEntries(data []byte, source, kind string) (map[string]json.RawMessage, []string) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, []string{fmt.Sprintf("%s: invalid JSON in %s: %v", kind, source, err)}
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
