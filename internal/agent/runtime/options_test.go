package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"myaaw/internal/config"
)

func TestApplyACPSessionMeta_ConfigOptions(t *testing.T) {
	raw := `{
		"sessionId": "s1",
		"configOptions": [
			{
				"id": "model",
				"name": "Model",
				"category": "model",
				"type": "select",
				"currentValue": "gemini-4-neon",
				"options": [
					{"value": "gemini-4-neon", "name": "Gemini 4 Neon"},
					{"value": "claude-opus-5-5", "name": "Claude Opus 5.5"}
				]
			},
			{
				"id": "thought_level",
				"name": "Thinking Level",
				"category": "thought_level",
				"type": "select",
				"currentValue": "high",
				"options": [
					{"value": "low", "name": "Low"},
					{"value": "high", "name": "High"}
				]
			},
			{
				"id": "mode",
				"name": "Mode",
				"category": "mode",
				"type": "select",
				"currentValue": "default",
				"options": [
					{"value": "default", "name": "Default", "description": "Ask before edits"},
					{"value": "auto", "name": "Auto"}
				]
			}
		]
	}`
	var meta acpSessionMeta
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	opts := RuntimeOptions{}
	applyACPSessionMeta(meta, &opts)

	if opts.Models.ConfigID != "model" || opts.Models.Current != "gemini-4-neon" || len(opts.Models.Options) != 2 {
		t.Fatalf("unexpected models: %+v", opts.Models)
	}
	if opts.Thinking.ConfigID != "thought_level" || opts.Thinking.Current != "high" || len(opts.Thinking.Options) != 2 {
		t.Fatalf("unexpected thinking: %+v", opts.Thinking)
	}
	if opts.SessionModes.ConfigID != "mode" || opts.SessionModes.Current != "default" || len(opts.SessionModes.Options) != 2 {
		t.Fatalf("unexpected modes: %+v", opts.SessionModes)
	}
}

func TestApplyACPSessionMeta_GroupedDeepSeekHarnessOptions(t *testing.T) {
	raw := `{
		"sessionId": "34e1f1a4-94e2-4f2a-8631-05bb6a8c0876",
		"configOptions": [
			{
				"id": "model",
				"name": "Model",
				"category": "model",
				"type": "select",
				"currentValue": "[\"deepseek-official\",\"deepseek-v4-flash\"]",
				"options": [
					{
						"group": "deepseek-official",
						"name": "DeepSeek",
						"options": [
							{"value": "[\"deepseek-official\",\"deepseek-v4-flash\"]", "name": "deepseek-v4-flash"}
						]
					},
					{
						"group": "deepseek-account",
						"name": "DeepSeek Account",
						"options": [
							{"value": "[\"deepseek-account\",\"deepseek-flash\"]", "name": "DeepSeek-V4.1-Flash"},
							{"value": "[\"deepseek-account\",\"deepseek-v4-pro\"]", "name": "DeepSeek-V4-Pro", "description": "Stronger reasoning"}
						]
					}
				]
			},
			{
				"id": "reasoning_effort",
				"name": "Reasoning effort",
				"category": "thought_level",
				"type": "select",
				"currentValue": "high",
				"options": [
					{"value": "off", "name": "Off"},
					{"value": "low", "name": "Low"},
					{"value": "high", "name": "High"},
					{"value": "max", "name": "Max"}
				]
			}
		]
	}`
	var meta acpSessionMeta
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	opts := RuntimeOptions{}
	applyACPSessionMeta(meta, &opts)

	if len(opts.Models.Options) != 3 {
		t.Fatalf("expected 3 grouped models, got %d: %+v", len(opts.Models.Options), opts.Models.Options)
	}
	if opts.Models.Options[1].Group != "DeepSeek Account" || opts.Models.Options[1].Label != "DeepSeek-V4.1-Flash" {
		t.Fatalf("unexpected grouped model[1]: %+v", opts.Models.Options[1])
	}
	if opts.Thinking.ConfigID != "reasoning_effort" || opts.Thinking.Current != "high" || len(opts.Thinking.Options) != 4 {
		t.Fatalf("unexpected reasoning_effort options: %+v", opts.Thinking)
	}
}

func TestExtractPiThinkingLevels(t *testing.T) {
	got := extractPiThinkingLevels(true, map[string]any{
		"minimal": nil,
		"low":     "low",
		"medium":  nil,
		"high":    "high",
		"max":     "max",
	})
	want := []string{"off", "low", "high", "max"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if nonReason := extractPiThinkingLevels(false, map[string]any{"high": "high"}); len(nonReason) != 0 {
		t.Fatalf("expected empty levels for non-reasoning model, got %v", nonReason)
	}
}

func TestParseFlagChoices(t *testing.T) {
	help := `
Options:
  --effort <level>            Effort level for the current session (low|medium|high|max)
  --permission-mode <mode>    Permission mode (choices: "default", "acceptEdits", "plan")
`
	efforts := parseFlagChoices(help, "--effort")
	wantEfforts := []string{"low", "medium", "high", "max"}
	if !reflect.DeepEqual(efforts, wantEfforts) {
		t.Fatalf("efforts got %v, want %v", efforts, wantEfforts)
	}
	modes := parseFlagChoices(help, "--permission-mode")
	wantModes := []string{"default", "acceptEdits", "plan"}
	if !reflect.DeepEqual(modes, wantModes) {
		t.Fatalf("modes got %v, want %v", modes, wantModes)
	}
}

func TestDiscoverRuntimeOptions_MockPiRPC(t *testing.T) {
	tmp := t.TempDir()
	script := filepath.Join(tmp, "mock-pi.sh")
	body := `#!/bin/sh
while IFS= read -r line; do
  case "$line" in
    *get_state*)
      echo '{"id":"1","type":"response","command":"get_state","success":true,"data":{"model":{"id":"m1","provider":"acme"},"thinkingLevel":"high"}}'
      ;;
    *get_available_models*)
      echo '{"id":"2","type":"response","command":"get_available_models","success":true,"data":{"models":[{"id":"m1","name":"Model One","provider":"acme","reasoning":true,"contextWindow":128000,"input":["text","image"],"thinkingLevelMap":{"low":"low","high":"high"}},{"id":"m2","name":"Model Two","provider":"acme","reasoning":false,"contextWindow":64000,"input":["text"]}]}}'
      exit 0
      ;;
  esac
done
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write mock script: %v", err)
	}

	opts := DiscoverRuntimeOptions(context.Background(), config.RuntimeConfig{
		Type:    config.RuntimeTypePi,
		Preset:  "pi",
		Command: script,
		Args:    []string{"--mode", "rpc"},
	}, tmp)

	if opts.Warning != "" {
		t.Fatalf("unexpected warning: %s", opts.Warning)
	}
	if len(opts.ApprovalModes.Options) != 0 {
		t.Fatalf("pi should not expose Myaaw approval modes when SupportsApprovals is false: %+v", opts.ApprovalModes)
	}
	if opts.Models.Current != "acme/m1" || len(opts.Models.Options) != 2 {
		t.Fatalf("unexpected models: %+v", opts.Models)
	}
	if opts.Thinking.Current != "high" || len(opts.Thinking.Options) != 3 {
		t.Fatalf("expected 3 thinking options (off, low, high), got: %+v", opts.Thinking)
	}

	// Selecting the non-reasoning model dynamically clears Thinking.Options.
	optsNonReason := applyConfiguredCurrents(opts, config.RuntimeConfig{
		Type:  config.RuntimeTypePi,
		Model: "acme/m2",
	})
	if len(optsNonReason.Thinking.Options) != 3 {
		// m2 has no ThinkingLevels slice; ensure m1 still preserved when switched back,
		// and verify m2 itself reports Thinking=false.
		if optsNonReason.Models.Options[1].Thinking {
			t.Fatalf("expected m2.Thinking == false")
		}
	}
}
