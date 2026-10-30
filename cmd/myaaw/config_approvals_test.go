package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"myaaw/internal/agent/approvals"
	"myaaw/internal/agentdir"
	"myaaw/internal/config"
)

func setupApprovalsCLI(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := agentdir.SetActive("myfriend"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	dir := filepath.Join(home, ".myaaw", "myfriend", "config")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	defaults := config.DefaultApprovals()
	cfg.Approvals = &defaults
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestApprovalsCLIAllowAndDeny(t *testing.T) {
	setupApprovalsCLI(t)

	if err := mutateApprovalList("allow", "add", "bash"); err != nil {
		t.Fatalf("allow add: %v", err)
	}
	cfg, err := config.LoadJSONConfigOnly()
	if err != nil || cfg == nil {
		t.Fatalf("load config: %v", err)
	}
	if !containsKey(cfg.Approvals.Allow, "bash") {
		t.Fatalf("allow = %v", cfg.Approvals.Allow)
	}

	// Adding the same key again is idempotent after normalization.
	if err := mutateApprovalList("allow", "add", "bash"); err != nil {
		t.Fatalf("allow add twice: %v", err)
	}
	cfg, _ = config.LoadJSONConfigOnly()
	if countKey(cfg.Approvals.Allow, "bash") != 1 {
		t.Fatalf("duplicate key: %v", cfg.Approvals.Allow)
	}

	if err := mutateApprovalList("allow", "remove", "bash"); err != nil {
		t.Fatalf("allow remove: %v", err)
	}
	cfg, _ = config.LoadJSONConfigOnly()
	if containsKey(cfg.Approvals.Allow, "bash") {
		t.Fatalf("bash still allowed: %v", cfg.Approvals.Allow)
	}

	// A key cannot be in both lists.
	if err := mutateApprovalList("deny", "add", "filesystem:read_file"); err == nil {
		t.Fatal("expected deny add to reject a key present in allow")
	}
}

func TestApprovalsCLISet(t *testing.T) {
	setupApprovalsCLI(t)

	mode := "off"
	timeout := 120
	if err := setApprovalPolicy(&mode, &timeout, nil, nil); err != nil {
		t.Fatalf("set: %v", err)
	}

	cfg, _ := config.LoadJSONConfigOnly()
	if cfg.Approvals == nil || cfg.Approvals.Mode != "off" {
		t.Fatalf("approvals = %+v", cfg.Approvals)
	}
	if cfg.Approvals.TimeoutSeconds == nil || *cfg.Approvals.TimeoutSeconds != 120 {
		t.Fatalf("timeout = %v", cfg.Approvals.TimeoutSeconds)
	}
}

func TestApprovalsCLIGrantsJSON(t *testing.T) {
	setupApprovalsCLI(t)

	path, err := agentdir.ActiveApprovalsPath()
	if err != nil {
		t.Fatal(err)
	}
	store := approvals.NewStore(path)
	if err := store.GrantAlways("bash", approvals.Scope{Channel: "telegram", UserID: "111"}); err != nil {
		t.Fatal(err)
	}
	if err := store.GrantSession("filesystem:write_file", approvals.SessionScope{
		Scope:          approvals.Scope{Channel: "telegram", UserID: "111"},
		ConversationID: "conv-1",
	}); err != nil {
		t.Fatal(err)
	}

	grants, err := loadApprovalGrants()
	if err != nil {
		t.Fatal(err)
	}
	out := grantsJSON(grants)
	if len(out) != 2 {
		t.Fatalf("grants json = %+v", out)
	}
	if out[0].Key != "bash" || len(out[0].Always) != 1 || out[0].Always[0].Channel != "telegram" || out[0].Always[0].UserID != "111" {
		t.Fatalf("grants[0] = %+v", out[0])
	}
	if out[1].Key != "filesystem:write_file" || len(out[1].Sessions) != 1 || out[1].Sessions[0].ID != "conv-1" || out[1].Sessions[0].UserID != "111" {
		t.Fatalf("grants[1] = %+v", out[1])
	}
}

func TestSaveApprovalsRejectsOverlap(t *testing.T) {
	cfg := &config.Config{}
	if err := saveApprovalsConfig(cfg, config.ApprovalsConfig{
		Allow: []string{"bash"},
		Deny:  []string{"bash"},
	}); err == nil {
		t.Fatal("expected allow/deny overlap to be rejected")
	}
}

func containsKey(keys []string, want string) bool {
	return countKey(keys, want) > 0
}

func countKey(keys []string, want string) int {
	count := 0
	for _, key := range keys {
		if key == want {
			count++
		}
	}
	return count
}
