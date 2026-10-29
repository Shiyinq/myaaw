package onboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myaaw/internal/config"
)

func setupExistingAgentDir(t *testing.T, configJSON string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "home"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "home", "AGENTS.md"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "config.json"), []byte(configJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestSetupHomeMigratesInlinePricesToFiles(t *testing.T) {
	dir := setupExistingAgentDir(t, `{
        "observability": {
            "model_prices": {"custom-model": {"input": 1, "output": 2}},
            "tool_prices": {"custom_tool": {"per_call": 0.5}}
        }
    }`)
	if err := setupHome(dir); err != nil {
		t.Fatalf("setupHome: %v", err)
	}

	var models map[string]config.ModelPriceConfig
	data, err := os.ReadFile(filepath.Join(dir, "config", "model_prices.json"))
	if err != nil {
		t.Fatalf("model prices file missing: %v", err)
	}
	if err := json.Unmarshal(data, &models); err != nil {
		t.Fatal(err)
	}
	if models["custom-model"].Output != 2 {
		t.Fatalf("inline model prices were not migrated: %+v", models)
	}

	var tools map[string]config.ToolPriceConfig
	data, err = os.ReadFile(filepath.Join(dir, "config", "tool_prices.json"))
	if err != nil {
		t.Fatalf("tool prices file missing: %v", err)
	}
	if err := json.Unmarshal(data, &tools); err != nil {
		t.Fatal(err)
	}
	if tools["custom_tool"].PerCall != 0.5 {
		t.Fatalf("inline tool prices were not migrated: %+v", tools)
	}
}

func TestSetupHomeSeedsTemplateWhenNoInlinePrices(t *testing.T) {
	dir := setupExistingAgentDir(t, `{}`)
	if err := setupHome(dir); err != nil {
		t.Fatalf("setupHome: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "config", "model_prices.json"))
	if err != nil {
		t.Fatalf("model prices file missing: %v", err)
	}
	if !strings.Contains(string(data), "deepseek-chat") {
		t.Fatalf("template model prices not copied: %s", data)
	}
	if _, err := os.Stat(filepath.Join(dir, "config", "tool_prices.json")); err != nil {
		t.Fatalf("tool prices file missing: %v", err)
	}
}

func TestSetupHomeKeepsExistingPriceFiles(t *testing.T) {
	dir := setupExistingAgentDir(t, `{}`)
	custom := []byte(`{"custom": {"input": 1, "output": 2}}`)
	path := filepath.Join(dir, "config", "model_prices.json")
	if err := os.WriteFile(path, custom, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := setupHome(dir); err != nil {
		t.Fatalf("setupHome: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(custom) {
		t.Fatalf("existing model prices were overwritten: %s", got)
	}
}
