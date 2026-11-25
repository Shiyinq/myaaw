package config

import (
	"strings"
	"testing"
)

func TestMergeRuntimeDefaultsAndPresets(t *testing.T) {
	def := MergeRuntime(nil)
	if def.Type != RuntimeTypeMyaaw || def.Preset != "myaaw" || def.IsExternal() {
		t.Fatalf("unexpected default runtime: %+v", def)
	}

	agy := MergeRuntime(&RuntimeConfig{Preset: "antigravity"})
	if agy.Type != RuntimeTypeACP || agy.Command != "agy" || len(agy.Args) != 1 || agy.Args[0] != "acp" {
		t.Fatalf("unexpected antigravity preset merge: %+v", agy)
	}
	if !agy.IsExternal() || !agy.MCPBridgeEnabled() {
		t.Fatalf("expected external runtime with MCP bridge enabled: %+v", agy)
	}

	omp := MergeRuntime(&RuntimeConfig{Preset: "omp"})
	if omp.Type != RuntimeTypeOMP || omp.Command != "omp" || omp.AuthSource != AuthSourceAPIKey {
		t.Fatalf("unexpected omp preset merge: %+v", omp)
	}
}

func TestValidateRuntimeToSGuardrails(t *testing.T) {
	agy := MergeRuntime(&RuntimeConfig{
		Preset:     "antigravity",
		AuthSource: AuthSourceCLILogin,
	})
	if err := ValidateRuntime(&agy, &GlobalBotConfig{Type: "public"}); err == nil {
		t.Fatalf("expected public bot with cli_login to be rejected")
	}
	if err := ValidateRuntime(&agy, &GlobalBotConfig{Type: "private"}); err != nil {
		t.Fatalf("expected private bot with cli_login to pass, got: %v", err)
	}

	agyAPI := MergeRuntime(&RuntimeConfig{
		Preset:     "antigravity",
		AuthSource: AuthSourceAPIKey,
		APIKey:     "test-key",
	})
	if err := ValidateRuntime(&agyAPI, &GlobalBotConfig{Type: "public"}); err != nil {
		t.Fatalf("expected public bot with api_key to pass, got: %v", err)
	}

	customMissingCmd := &RuntimeConfig{Preset: "custom_acp"}
	if err := ValidateRuntime(customMissingCmd, nil); err == nil {
		t.Fatalf("expected custom_acp without command to fail validation")
	}

	warnings := RuntimeWarnings(&agy, &HeartbeatConfig{Active: true, Every: "5m"})
	if len(warnings) == 0 || !strings.Contains(warnings[0], "high-frequency") {
		t.Fatalf("expected high-frequency heartbeat warning, got: %v", warnings)
	}
}

func TestRuntimePresetsOrder(t *testing.T) {
	want := []string{
		"myaaw",
		"claude_code",
		"codex",
		"antigravity",
		"opencode",
		"pi",
		"omp",
		"custom_acp",
	}
	presets := RuntimePresets()
	if len(presets) != len(want) {
		t.Fatalf("RuntimePresets() len = %d, want %d", len(presets), len(want))
	}
	for i, id := range want {
		if presets[i].ID != id {
			t.Fatalf("RuntimePresets()[%d].ID = %q, want %q", i, presets[i].ID, id)
		}
	}
}
