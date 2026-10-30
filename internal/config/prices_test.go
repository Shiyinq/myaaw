package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withPriceGlobals(t *testing.T, models map[string]ModelPriceConfig, tools map[string]ToolPriceConfig) {
	t.Helper()
	prevModels, prevTools := ObsModelPrices, ObsToolPrices
	ObsModelPrices, ObsToolPrices = models, tools
	t.Cleanup(func() {
		ObsModelPrices, ObsToolPrices = prevModels, prevTools
	})
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestApplyPriceFilesSkipsNegativePrices(t *testing.T) {
	dir := t.TempDir()
	withPriceGlobals(t, nil, nil)
	writeFile(t, dir, ModelPricesFile, `{"bad": {"input": -1, "output": 2}, "good": {"input": 1, "output": 2}}`)
	writeFile(t, dir, ToolPricesFile, `{"bad_tool": {"per_call": -1}, "good_tool": {"per_call": 0.2}}`)

	warnings := applyPriceFilesAt(dir)
	if len(warnings) < 2 {
		t.Fatalf("warnings = %v", warnings)
	}
	if _, ok := ObsModelPrices["bad"]; ok {
		t.Fatal("negative model entry should be skipped")
	}
	if ObsModelPrices["good"].Input != 1 {
		t.Fatalf("good model = %+v", ObsModelPrices)
	}
	if _, ok := ObsToolPrices["bad_tool"]; ok {
		t.Fatal("negative tool entry should be skipped")
	}
	if ObsToolPrices["good_tool"].PerCall != 0.2 {
		t.Fatalf("good tool = %+v", ObsToolPrices)
	}
}

func TestLoadBaseConfigReadsPriceFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	dir := filepath.Join(home, ".myaaw", "default", "config")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ModelPricesFile), []byte(`{"file-model": {"input": 1, "output": 2}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ToolPricesFile), []byte(`{"file_tool": {"per_call": 0.3}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	prevModels, prevTools := ObsModelPrices, ObsToolPrices
	t.Cleanup(func() { ObsModelPrices, ObsToolPrices = prevModels, prevTools })
	ObsModelPrices, ObsToolPrices = nil, nil

	LoadBaseConfig()

	if got := ObsModelPrices["file-model"].Output; got != 2 {
		t.Fatalf("model price from file = %v (all: %+v)", got, ObsModelPrices)
	}
	if got := ObsToolPrices["file_tool"].PerCall; got != 0.3 {
		t.Fatalf("tool price from file = %v (all: %+v)", got, ObsToolPrices)
	}
}

func TestApplyPriceFilesOverridesInlinePerKey(t *testing.T) {
	dir := t.TempDir()
	withPriceGlobals(t,
		map[string]ModelPriceConfig{
			"keep":     {Input: 1},
			"override": {Input: 2},
		},
		map[string]ToolPriceConfig{
			"keep_tool": {PerCall: 1},
		})

	writeFile(t, dir, ModelPricesFile, `{
        "_comment": "ignored",
        "override": {"input": 20, "output": 30},
        "added": {"input": 4}
    }`)
	writeFile(t, dir, ToolPricesFile, `{"added_tool": {"per_call": 2}}`)

	if warnings := applyPriceFilesAt(dir); len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	if got := ObsModelPrices["keep"].Input; got != 1 {
		t.Fatalf("keep = %v", got)
	}
	if got := ObsModelPrices["override"].Input; got != 20 {
		t.Fatalf("override = %v", got)
	}
	if got := ObsModelPrices["added"].Input; got != 4 {
		t.Fatalf("added = %v", got)
	}
	if _, ok := ObsModelPrices["_comment"]; ok {
		t.Fatal("_comment should be ignored")
	}
	if got := ObsToolPrices["keep_tool"].PerCall; got != 1 {
		t.Fatalf("keep_tool = %v", got)
	}
	if got := ObsToolPrices["added_tool"].PerCall; got != 2 {
		t.Fatalf("added_tool = %v", got)
	}
}

func TestApplyPriceFilesMissingFilesIsNoOp(t *testing.T) {
	withPriceGlobals(t, nil, nil)
	if warnings := applyPriceFilesAt(t.TempDir()); len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if ObsModelPrices != nil || ObsToolPrices != nil {
		t.Fatalf("globals changed: %v %v", ObsModelPrices, ObsToolPrices)
	}
}

func TestApplyPriceFilesInvalidJSONWarns(t *testing.T) {
	dir := t.TempDir()
	withPriceGlobals(t, map[string]ModelPriceConfig{"m": {Input: 1}}, nil)
	writeFile(t, dir, ModelPricesFile, `{not json`)

	warnings := applyPriceFilesAt(dir)
	if len(warnings) == 0 || !strings.Contains(warnings[0], "invalid JSON") {
		t.Fatalf("warnings = %v", warnings)
	}
	if got := ObsModelPrices["m"].Input; got != 1 {
		t.Fatalf("inline values should survive: %v", got)
	}
}

func TestWriteModelPricesFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := map[string]ModelPriceConfig{"m": {Input: 1, Output: 2}}
	if err := WriteModelPricesFile(dir, want); err != nil {
		t.Fatal(err)
	}
	got, warnings := loadModelPricesFile(filepath.Join(dir, ModelPricesFile))
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if got["m"].Output != 2 {
		t.Fatalf("round trip = %+v", got)
	}
}
