package service

import (
	"errors"
	"testing"

	"myaaw/internal/agent/approvals"
	"myaaw/internal/agentdir"
)

func TestConfigPatchApprovals(t *testing.T) {
	setupConfigAgent(t, testConfig())
	svc := NewConfigService()

	mode := "off"
	timeout := 120
	allow := []string{" bash ", "filesystem:read_file", "bash"}
	view, err := svc.Patch("myfriend", ConfigPatch{Approvals: &ApprovalsPatch{
		Mode:           &mode,
		TimeoutSeconds: &timeout,
		Allow:          &allow,
	}})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if view.Approvals == nil || view.Approvals.Mode != "off" {
		t.Fatalf("view approvals = %+v", view.Approvals)
	}

	cfg := loadTestConfig(t)
	if cfg.Approvals == nil {
		t.Fatal("approvals block was not written")
	}
	if cfg.Approvals.Mode != "off" || cfg.Approvals.TimeoutSeconds == nil || *cfg.Approvals.TimeoutSeconds != 120 {
		t.Fatalf("config approvals = %+v", cfg.Approvals)
	}
	if len(cfg.Approvals.Allow) != 2 || cfg.Approvals.Allow[0] != "bash" || cfg.Approvals.Allow[1] != "filesystem:read_file" {
		t.Fatalf("normalized allow = %v", cfg.Approvals.Allow)
	}

	bad := "maybe"
	if _, err := svc.Patch("myfriend", ConfigPatch{Approvals: &ApprovalsPatch{Mode: &bad}}); err == nil {
		t.Fatal("expected an invalid mode to be rejected")
	}

	conflictAllow := []string{"bash"}
	conflictDeny := []string{"bash"}
	if _, err := svc.Patch("myfriend", ConfigPatch{Approvals: &ApprovalsPatch{
		Allow: &conflictAllow,
		Deny:  &conflictDeny,
	}}); err == nil {
		t.Fatal("expected allow/deny overlap to be rejected")
	}
}

func TestConfigResetApprovals(t *testing.T) {
	setupConfigAgent(t, testConfig())
	svc := NewConfigService()

	mode := "off"
	empty := []string{}
	if _, err := svc.Patch("myfriend", ConfigPatch{Approvals: &ApprovalsPatch{
		Mode:  &mode,
		Allow: &empty,
	}}); err != nil {
		t.Fatalf("Patch: %v", err)
	}

	view, err := svc.ResetFields("myfriend", ConfigResetRequest{Approvals: wholeSection()})
	if err != nil {
		t.Fatalf("ResetFields: %v", err)
	}
	if view.Approvals == nil || view.Approvals.Mode != "manual" {
		t.Fatalf("reset approvals = %+v", view.Approvals)
	}
	if len(view.Approvals.Allow) == 0 {
		t.Fatal("reset should restore the template allow list")
	}
}

func TestApprovalsServiceViewAndRevoke(t *testing.T) {
	setupConfigAgent(t, testConfig())

	path, err := agentdir.ApprovalsPath("myfriend")
	if err != nil {
		t.Fatal(err)
	}
	store := approvals.NewStore(path)
	if err := store.GrantAlways("bash", approvals.Scope{Channel: "telegram", UserID: "111"}); err != nil {
		t.Fatal(err)
	}
	scope := approvals.Scope{Channel: "telegram", UserID: "111"}
	if err := store.GrantSession("filesystem:write_file", approvals.SessionScope{Scope: scope, ConversationID: "conv-1"}); err != nil {
		t.Fatal(err)
	}
	if err := store.GrantSession("filesystem:write_file", approvals.SessionScope{Scope: scope, ConversationID: "conv-2"}); err != nil {
		t.Fatal(err)
	}

	svc := NewApprovalsService()
	view, err := svc.View("myfriend")
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	if view.Config == nil || view.Config.Mode != "manual" {
		t.Fatalf("config = %+v", view.Config)
	}
	if len(view.Grants) != 2 {
		t.Fatalf("grants = %+v", view.Grants)
	}
	if view.Grants[0].Key != "bash" || len(view.Grants[0].Always) != 1 || view.Grants[0].Always[0].UserID != "111" {
		t.Fatalf("grants[0] = %+v", view.Grants[0])
	}
	if view.Grants[1].Key != "filesystem:write_file" || len(view.Grants[1].Sessions) != 2 {
		t.Fatalf("grants[1] = %+v", view.Grants[1])
	}

	if err := svc.RevokeKey("myfriend", "bash"); err != nil {
		t.Fatalf("RevokeKey: %v", err)
	}
	if err := svc.RevokeSession("myfriend", "filesystem:write_file", "conv-1", "telegram", "111"); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}
	if err := svc.RevokeKey("myfriend", "missing"); !errors.Is(err, ErrGrantNotFound) {
		t.Fatalf("missing key error = %v", err)
	}
	if err := svc.RevokeSession("myfriend", "filesystem:write_file", "nope", "telegram", "111"); !errors.Is(err, ErrGrantNotFound) {
		t.Fatalf("missing session error = %v", err)
	}
	if err := svc.Clear("myfriend"); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if grants := approvals.NewStore(path).Snapshot(); len(grants) != 0 {
		t.Fatalf("grants after clear = %v", grants)
	}
}
