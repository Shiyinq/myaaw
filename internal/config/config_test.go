package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// LoadBaseConfig must apply an explicit 0 from config.json instead of falling
// back to the default retention.
func TestParseToolPrices(t *testing.T) {
	prices := parseToolPrices("tavily_search=0.008; image_gen=0.04; bad; negative=-1")
	if len(prices) != 2 {
		t.Fatalf("prices = %+v, want only the valid positive entries", prices)
	}
	if prices["tavily_search"].PerCall != 0.008 || prices["image_gen"].PerCall != 0.04 {
		t.Fatalf("prices = %+v", prices)
	}
	if got := parseToolPrices(""); got != nil {
		t.Fatalf("empty = %+v, want nil", got)
	}
}

func TestLoadBaseConfigRedactNoneDisables(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("OBS_REDACT_PATTERNS", "none")

	dir := filepath.Join(home, ".myaaw", "default", "config")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	original := ObsRedactPatterns
	t.Cleanup(func() { ObsRedactPatterns = original })
	ObsRedactPatterns = nil

	LoadBaseConfig()
	if ObsRedactPatterns == nil {
		t.Fatal("OBS_REDACT_PATTERNS=none must produce an explicit empty list")
	}
	if len(ObsRedactPatterns) != 0 {
		t.Fatalf("redact patterns = %+v, want none", ObsRedactPatterns)
	}
}

func TestLoadBaseConfigHonorsZeroRetention(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// godotenv does not override the process environment, so this pins the
	// starting value even when the repository has a .env file.
	t.Setenv("OBS_RETENTION_DAYS", "30")

	dir := filepath.Join(home, ".myaaw", "default", "config")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"observability":{"retention_days":0}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	original := ObsRetentionDays
	t.Cleanup(func() { ObsRetentionDays = original })
	ObsRetentionDays = 30

	LoadBaseConfig()
	if ObsRetentionDays != 0 {
		t.Fatalf("retention days = %d, want an explicit 0 from config.json", ObsRetentionDays)
	}
}

func TestObservabilityConfigKeepsAnExplicitZeroRetention(t *testing.T) {
	var withZero Config
	if err := json.Unmarshal([]byte(`{"observability":{"retention_days":0}}`), &withZero); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if withZero.Observability == nil || withZero.Observability.RetentionDays == nil {
		t.Fatalf("observability = %+v", withZero.Observability)
	}
	if *withZero.Observability.RetentionDays != 0 {
		t.Fatalf("retention days = %d, want an explicit 0", *withZero.Observability.RetentionDays)
	}

	var omitted Config
	if err := json.Unmarshal([]byte(`{"observability":{}}`), &omitted); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if omitted.Observability == nil || omitted.Observability.RetentionDays != nil {
		t.Fatalf("an omitted retention must stay nil, got %+v", omitted.Observability)
	}
}

func TestValidateAgentConfigClampsObservability(t *testing.T) {
	originalRetention, originalChars := ObsRetentionDays, ObsMaxContentChars
	t.Cleanup(func() {
		ObsRetentionDays = originalRetention
		ObsMaxContentChars = originalChars
	})

	ObsRetentionDays = -5
	ObsMaxContentChars = 0
	validateAgentConfig()
	if ObsRetentionDays != 0 {
		t.Fatalf("retention days = %d, want 0", ObsRetentionDays)
	}
	if ObsMaxContentChars != 16384 {
		t.Fatalf("max content chars = %d, want the default 16384", ObsMaxContentChars)
	}

	ObsRetentionDays = 7
	ObsMaxContentChars = 2048
	validateAgentConfig()
	if ObsRetentionDays != 7 || ObsMaxContentChars != 2048 {
		t.Fatalf("observability = %d/%d, want 7/2048 left untouched", ObsRetentionDays, ObsMaxContentChars)
	}
}

func TestEffectiveObservabilityDefaults(t *testing.T) {
	originalEnabled, originalStore := ObsEnabled, ObsStoreContent
	originalRetention, originalChars := ObsRetentionDays, ObsMaxContentChars
	t.Cleanup(func() {
		ObsEnabled, ObsStoreContent = originalEnabled, originalStore
		ObsRetentionDays, ObsMaxContentChars = originalRetention, originalChars
	})

	ObsEnabled = true
	ObsStoreContent = false
	ObsRetentionDays = 30
	ObsMaxContentChars = 16384

	settings := EffectiveObservability()
	if settings.Enabled == nil || !*settings.Enabled {
		t.Fatal("observability should be enabled")
	}
	if settings.StoreContent == nil || *settings.StoreContent {
		t.Fatal("store_content should be false")
	}
	if settings.RetentionDays == nil || *settings.RetentionDays != 30 || settings.MaxContentChars != 16384 {
		t.Fatalf("settings = %+v", settings)
	}
}

func TestValidateAgentConfigClampsQueueWorkers(t *testing.T) {
	original := QueueWorkers
	t.Cleanup(func() { QueueWorkers = original })

	QueueWorkers = 0
	validateAgentConfig()
	if QueueWorkers != 1 {
		t.Fatalf("queue workers = %d, want the minimum of 1", QueueWorkers)
	}

	QueueWorkers = MaxQueueWorkers + 5
	validateAgentConfig()
	if QueueWorkers != MaxQueueWorkers {
		t.Fatalf("queue workers = %d, want the cap of %d", QueueWorkers, MaxQueueWorkers)
	}

	QueueWorkers = 3
	validateAgentConfig()
	if QueueWorkers != 3 {
		t.Fatalf("queue workers = %d, want 3 left untouched", QueueWorkers)
	}
}

func approvalsIntPtr(v int) *int { return &v }

func TestValidateApprovals(t *testing.T) {
	valid := ApprovalsConfig{
		Mode:           "manual",
		TimeoutSeconds: approvalsIntPtr(60),
		Allow:          []string{"filesystem:read_file", "*"},
		Deny:           []string{"python", "bash"},
		Unattended:     "allow",
		Subagents:      "inherit",
	}
	if err := ValidateApprovals(&valid); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	cases := []ApprovalsConfig{
		{Mode: "maybe"},
		{TimeoutSeconds: approvalsIntPtr(0)},
		{TimeoutSeconds: approvalsIntPtr(3601)},
		{Unattended: "maybe"},
		{Subagents: "maybe"},
		{Allow: []string{"has space"}},
		{Allow: []string{"bash"}, Deny: []string{"bash"}},
	}
	for i, cfg := range cases {
		if err := ValidateApprovals(&cfg); err == nil {
			t.Errorf("case %d should be rejected: %+v", i, cfg)
		}
	}
}

func TestMergeAndNormalizeApprovals(t *testing.T) {
	merged := MergeApprovals(nil)
	if merged.Mode != "manual" || merged.Unattended != "allow" || merged.Subagents != "inherit" {
		t.Fatalf("defaults = %+v", merged)
	}
	if len(merged.Allow) == 0 {
		t.Fatal("default allow list should not be empty")
	}

	raw := &ApprovalsConfig{Mode: " OFF ", Allow: []string{" bash ", "", "bash", "filesystem:read_file"}}
	merged = MergeApprovals(raw)
	if merged.Mode != " OFF " {
		t.Fatalf("merge should not normalize: %q", merged.Mode)
	}
	norm := NormalizeApprovals(merged)
	if norm.Mode != "off" {
		t.Fatalf("mode = %q", norm.Mode)
	}
	if len(norm.Allow) != 2 || norm.Allow[0] != "bash" || norm.Allow[1] != "filesystem:read_file" {
		t.Fatalf("allow = %v", norm.Allow)
	}
}
