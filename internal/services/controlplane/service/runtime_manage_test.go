package service

import (
	"context"
	"path/filepath"
	"testing"

	"myaaw/internal/agentdir"
	"myaaw/internal/config"
)

func TestRuntimeManageDefaultsMaskingAndOnboardInheritance(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}

	cpdb, err := OpenControlPlaneDBAt(filepath.Join(home, "controlplane", "controlplane.db"))
	if err != nil {
		t.Fatalf("OpenControlPlaneDBAt: %v", err)
	}
	defer cpdb.Close()

	agents := NewAgentService()
	mgr := NewRuntimeManageService(cpdb, agents)
	agents.SetRuntimeManager(mgr)

	mcpFalse := false
	saved, err := mgr.SaveDefaults(context.Background(), "opencode", RuntimeDefaultsInput{
		Command:        "opencode",
		Args:           []string{"acp"},
		Model:          "anthropic/claude-sonnet-4-5",
		ThinkingLevel:  "high",
		SessionMode:    "plan",
		AuthSource:     config.AuthSourceAPIKey,
		APIKeyEnv:      "ANTHROPIC_API_KEY",
		APIKey:         "sk-ant-secret-global-1234",
		PermissionMode: config.PermissionModeAuto,
		MCPBridge:      &mcpFalse,
	})
	if err != nil {
		t.Fatalf("SaveDefaults: %v", err)
	}
	if !saved.Defaults.APIKey.Configured || saved.Defaults.APIKey.Masked != "********" {
		t.Fatalf("expected masked api_key in view, got %+v", saved.Defaults.APIKey)
	}
	if saved.Defaults.Model != "anthropic/claude-sonnet-4-5" || saved.Defaults.ThinkingLevel != "high" || saved.Defaults.MCPBridge {
		t.Fatalf("unexpected saved defaults view: %+v", saved.Defaults)
	}

	// Empty APIKey on subsequent save keeps the existing secret.
	saved2, err := mgr.SaveDefaults(context.Background(), "opencode", RuntimeDefaultsInput{
		Command:        "opencode",
		Args:           []string{"acp"},
		Model:          "openai/gpt-5",
		ThinkingLevel:  "medium",
		AuthSource:     config.AuthSourceAPIKey,
		APIKeyEnv:      "OPENAI_API_KEY",
		PermissionMode: config.PermissionModeBridge,
	})
	if err != nil {
		t.Fatalf("SaveDefaults update: %v", err)
	}
	if !saved2.Defaults.APIKey.Configured {
		t.Fatalf("expected existing API key to be preserved when APIKey is omitted")
	}

	// Onboarding a new external agent with omitted fields inherits global defaults + global API key.
	res, err := agents.Onboard("coder-oc", OnboardRequest{
		Runtime: &OnboardRuntimeInput{
			Preset: "opencode",
		},
	})
	if err != nil {
		t.Fatalf("Onboard coder-oc: %v", err)
	}
	if !res.Created {
		t.Fatalf("expected Created = true")
	}
	if res.Config.Runtime.Model != "openai/gpt-5" || res.Config.Runtime.ThinkingLevel != "medium" {
		t.Fatalf("expected agent to inherit global model/thinking, got %+v", res.Config.Runtime)
	}
	if !res.Config.Runtime.APIKey.Configured {
		t.Fatalf("expected agent to inherit global API key, got %+v", res.Config.Runtime.APIKey)
	}

	// UsedByAgents lists coder-oc, and ApplyToAgents propagates updates to coder-oc.
	mcpTrue := true
	updated, err := mgr.SaveDefaults(context.Background(), "opencode", RuntimeDefaultsInput{
		Command:        "opencode",
		Args:           []string{"acp"},
		Model:          "openai/gpt-5-mini",
		ThinkingLevel:  "low",
		AuthSource:     config.AuthSourceAPIKey,
		APIKeyEnv:      "OPENAI_API_KEY",
		PermissionMode: config.PermissionModeReadOnly,
		MCPBridge:      &mcpTrue,
		ApplyToAgents:  []string{"coder-oc"},
	})
	if err != nil {
		t.Fatalf("SaveDefaults with ApplyToAgents: %v", err)
	}
	if len(updated.UsedByAgents) != 1 || updated.UsedByAgents[0] != "coder-oc" {
		t.Fatalf("expected UsedByAgents = [coder-oc], got %v", updated.UsedByAgents)
	}
	agentCfg, err := NewConfigService().Config("coder-oc")
	if err != nil {
		t.Fatalf("Config(coder-oc): %v", err)
	}
	if agentCfg.Runtime.Model != "openai/gpt-5-mini" || agentCfg.Runtime.PermissionMode != config.PermissionModeReadOnly || !agentCfg.Runtime.MCPBridge {
		t.Fatalf("expected ApplyToAgents to update coder-oc, got %+v", agentCfg.Runtime)
	}

	// ClearAPIKey removes the stored global secret.
	cleared, err := mgr.SaveDefaults(context.Background(), "opencode", RuntimeDefaultsInput{
		Command:        "opencode",
		Args:           []string{"acp"},
		AuthSource:     config.AuthSourceAPIKey,
		PermissionMode: config.PermissionModeBridge,
		ClearAPIKey:    true,
	})
	if err != nil {
		t.Fatalf("SaveDefaults clear key: %v", err)
	}
	if cleared.Defaults.APIKey.Configured {
		t.Fatalf("expected global API key to be cleared")
	}
}

func TestRuntimeManageDisabledEnvFlag(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("MYAAW_RUNTIME_MANAGEMENT", "0")

	cpdb, err := OpenControlPlaneDBAt(filepath.Join(home, "controlplane", "controlplane.db"))
	if err != nil {
		t.Fatalf("OpenControlPlaneDBAt: %v", err)
	}
	defer cpdb.Close()

	mgr := NewRuntimeManageService(cpdb, NewAgentService())
	if mgr.Enabled() {
		t.Fatalf("expected mgr.Enabled() = false when MYAAW_RUNTIME_MANAGEMENT=0")
	}
	if _, err := mgr.Install(context.Background(), "opencode", "official_script", nil); err != ErrRuntimeManagementDisabled {
		t.Fatalf("expected ErrRuntimeManagementDisabled, got %v", err)
	}
}
