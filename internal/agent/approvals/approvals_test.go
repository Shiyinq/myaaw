package approvals

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

type fakeRequester struct {
	decision Decision
	err      error
	delay    time.Duration
	requests []Request
}

func (f *fakeRequester) Request(ctx context.Context, req Request) (Decision, error) {
	f.requests = append(f.requests, req)
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return Deny, ctx.Err()
		}
	}
	return f.decision, f.err
}

func newGate(t *testing.T) (*Gate, *Store) {
	t.Helper()
	store := NewStore(filepath.Join(t.TempDir(), "approvals.json"))
	return &Gate{
		Policy: Policy{
			Mode:       "manual",
			Timeout:    time.Second,
			Unattended: AllowOnce,
		},
		Store:   store,
		Session: Session{RunID: "run-1", Channel: "telegram", ConversationID: "chat-1", UserID: "42"},
	}, store
}

func TestKeyUsesActionWhenPresent(t *testing.T) {
	key, action := Key("computer", json.RawMessage(`{"action":"click","x":1}`))
	if key != "computer:click" || action != "click" {
		t.Fatalf("key = %q action = %q", key, action)
	}

	key, action = Key("filesystem", json.RawMessage(`{"tool_name":"read_file","path":"/tmp"}`))
	if key != "filesystem:read_file" || action != "read_file" {
		t.Fatalf("key = %q action = %q", key, action)
	}

	key, action = Key("bash", json.RawMessage(`{"command":"ls"}`))
	if key != "bash" || action != "" {
		t.Fatalf("key = %q action = %q", key, action)
	}

	key, action = Key("bash", nil)
	if key != "bash" || action != "" {
		t.Fatalf("key = %q action = %q", key, action)
	}
}

func TestGateManualRequiresApprovalForEverything(t *testing.T) {
	gate, _ := newGate(t)
	requester := &fakeRequester{decision: AllowOnce}
	gate.Requester = requester

	for _, call := range []struct {
		tool string
		args string
	}{
		{"filesystem", `{"tool_name":"read_file","path":"/tmp"}`},
		{"bash", `{"command":"ls"}`},
		{"python", `{"code":"print(1)"}`},
	} {
		if out := gate.Check(context.Background(), call.tool, json.RawMessage(call.args)); !out.Allowed {
			t.Fatalf("%s should be allowed after approval, got %+v", call.tool, out)
		}
	}
	if len(requester.requests) != 3 {
		t.Fatalf("requests = %d, want 3", len(requester.requests))
	}
}

func TestGateAllowListSkipsPrompt(t *testing.T) {
	gate, _ := newGate(t)
	gate.Policy.Allow = []string{"filesystem:read_file", "computer:screenshot"}
	requester := &fakeRequester{decision: Deny}
	gate.Requester = requester

	if out := gate.Check(context.Background(), "filesystem", json.RawMessage(`{"tool_name":"read_file"}`)); !out.Allowed {
		t.Fatalf("read_file should be allowed by the whitelist, got %+v", out)
	}
	if out := gate.Check(context.Background(), "computer", json.RawMessage(`{"action":"screenshot"}`)); !out.Allowed {
		t.Fatalf("screenshot should be allowed by the whitelist, got %+v", out)
	}
	if len(requester.requests) != 0 {
		t.Fatalf("whitelisted calls must not ask, got %d requests", len(requester.requests))
	}

	// A bare tool rule allows every action of that tool.
	gate.Policy.Allow = []string{"filesystem"}
	if out := gate.Check(context.Background(), "filesystem", json.RawMessage(`{"tool_name":"delete_path"}`)); !out.Allowed {
		t.Fatalf("tool-level allow should cover every action, got %+v", out)
	}
}

func TestGateDenyWinsOverEverything(t *testing.T) {
	gate, _ := newGate(t)
	gate.Policy.Deny = []string{"bash"}
	gate.Policy.Allow = []string{"bash"}
	gate.Requester = &fakeRequester{decision: AllowAlways}

	out := gate.Check(context.Background(), "bash", json.RawMessage(`{"command":"ls"}`))
	if out.Allowed || out.Decision != Deny {
		t.Fatalf("expected deny, got %+v", out)
	}
	if len(gate.Requester.(*fakeRequester).requests) != 0 {
		t.Fatal("requester should not be asked for a denied tool")
	}
}

func TestGateModeOffAllowsEverything(t *testing.T) {
	gate, _ := newGate(t)
	gate.Policy.Mode = "off"
	requester := &fakeRequester{decision: Deny}
	gate.Requester = requester

	out := gate.Check(context.Background(), "bash", json.RawMessage(`{"command":"rm -rf /"}`))
	if !out.Allowed {
		t.Fatalf("expected allow with mode off, got %+v", out)
	}
	if len(requester.requests) != 0 {
		t.Fatal("mode off must not ask")
	}
}

func TestGateNoRequesterUsesUnattendedDecision(t *testing.T) {
	gate, _ := newGate(t)
	gate.Auto = AllowOnce
	out := gate.Check(context.Background(), "bash", json.RawMessage(`{"command":"ls"}`))
	if !out.Allowed {
		t.Fatalf("expected unattended allow, got %+v", out)
	}

	gate.Auto = Deny
	out = gate.Check(context.Background(), "bash", json.RawMessage(`{"command":"ls"}`))
	if out.Allowed {
		t.Fatalf("expected unattended deny, got %+v", out)
	}
}

func TestGateRequesterDecisions(t *testing.T) {
	gate, store := newGate(t)
	requester := &fakeRequester{decision: AllowSession}
	gate.Requester = requester

	out := gate.Check(context.Background(), "bash", json.RawMessage(`{"command":"ls"}`))
	if !out.Allowed || out.Decision != AllowSession {
		t.Fatalf("expected session allow, got %+v", out)
	}
	if len(requester.requests) != 1 {
		t.Fatalf("requests = %d", len(requester.requests))
	}
	req := requester.requests[0]
	if req.ToolName != "bash" || req.Channel != "telegram" || req.ConversationID != "chat-1" || req.ID == "" {
		t.Fatalf("request = %+v", req)
	}

	// The session grant covers the next call without asking again.
	out = gate.Check(context.Background(), "bash", json.RawMessage(`{"command":"pwd"}`))
	if !out.Allowed || out.Decision != AllowSession {
		t.Fatalf("expected stored session grant, got %+v", out)
	}
	if len(requester.requests) != 1 {
		t.Fatalf("requester should not be asked twice, got %d", len(requester.requests))
	}

	// A different conversation must ask again.
	gate.Session.ConversationID = "chat-2"
	_ = gate.Check(context.Background(), "bash", json.RawMessage(`{"command":"ls"}`))
	if len(requester.requests) != 2 {
		t.Fatalf("other conversation should ask again, got %d requests", len(requester.requests))
	}

	if d, ok := store.Grant("bash", "chat-1"); !ok || d != AllowSession {
		t.Fatalf("stored grant = %q ok=%v", d, ok)
	}
}

func TestGateAlwaysGrantPersists(t *testing.T) {
	gate, _ := newGate(t)
	gate.Requester = &fakeRequester{decision: AllowAlways}

	if out := gate.Check(context.Background(), "bash", nil); !out.Allowed {
		t.Fatalf("expected allow, got %+v", out)
	}

	again := NewStore(gate.Store.path)
	if d, ok := again.Grant("bash", "anything"); !ok || d != AllowAlways {
		t.Fatalf("always grant did not persist: %q ok=%v", d, ok)
	}
}

func TestGateRequesterDenyAndError(t *testing.T) {
	gate, _ := newGate(t)
	gate.Requester = &fakeRequester{decision: Deny}
	if out := gate.Check(context.Background(), "bash", nil); out.Allowed {
		t.Fatalf("expected deny, got %+v", out)
	}

	gate.Requester = &fakeRequester{err: errors.New("boom")}
	if out := gate.Check(context.Background(), "bash", nil); out.Allowed {
		t.Fatalf("expected deny on requester error, got %+v", out)
	}
}

func TestGateRequesterTimeout(t *testing.T) {
	gate, _ := newGate(t)
	gate.Policy.Timeout = 20 * time.Millisecond
	gate.Requester = &fakeRequester{decision: AllowOnce, delay: time.Second}

	start := time.Now()
	out := gate.Check(context.Background(), "bash", nil)
	if out.Allowed {
		t.Fatalf("expected timeout deny, got %+v", out)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatalf("timeout took too long: %s", time.Since(start))
	}
}

func TestDenyAll(t *testing.T) {
	gate := DenyAll()
	if out := gate.Check(context.Background(), "anything", nil); out.Allowed {
		t.Fatalf("DenyAll allowed a tool: %+v", out)
	}
}

func TestRegistryResolve(t *testing.T) {
	registry := NewRegistry()
	ch := registry.Register("tok")
	if !registry.Resolve("tok", AllowOnce) {
		t.Fatal("resolve returned false")
	}
	if got := <-ch; got != AllowOnce {
		t.Fatalf("decision = %q", got)
	}
	if registry.Resolve("tok", Deny) {
		t.Fatal("second resolve should return false")
	}
}

func TestContextHelpers(t *testing.T) {
	gate := &Gate{}
	ctx := WithGate(context.Background(), gate)
	if GateFromContext(ctx) != gate {
		t.Fatal("gate round-trip failed")
	}

	requester := &fakeRequester{}
	ctx = WithRequester(ctx, requester)
	if RequesterFromContext(ctx) != requester {
		t.Fatal("requester round-trip failed")
	}

	if IsSubagent(ctx) {
		t.Fatal("ctx should not be a subagent")
	}
	if !IsSubagent(WithSubagent(ctx)) {
		t.Fatal("subagent marker missing")
	}
	if got := SubagentName(WithSubagentNamed(ctx, "download-x-photo")); got != "download-x-photo" {
		t.Fatalf("subagent name = %q", got)
	}
	if got := SubagentName(WithSubagent(ctx)); got != "" {
		t.Fatalf("unnamed subagent name = %q", got)
	}
}

func TestGateRequestCarriesSubagentName(t *testing.T) {
	gate, _ := newGate(t)
	requester := &fakeRequester{decision: AllowOnce}
	gate.Requester = requester

	ctx := WithSubagentNamed(context.Background(), "download-x-photo")
	if out := gate.Check(ctx, "bash", nil); !out.Allowed {
		t.Fatalf("expected allow, got %+v", out)
	}
	if len(requester.requests) != 1 || requester.requests[0].SubAgent != "download-x-photo" {
		t.Fatalf("request did not carry the sub-agent name: %+v", requester.requests)
	}

	requester.requests = nil
	gate.Check(context.Background(), "bash", nil)
	if len(requester.requests) != 1 || requester.requests[0].SubAgent != "" {
		t.Fatalf("main-agent request should not name a sub-agent: %+v", requester.requests)
	}
}

func TestStoreMergesExternalWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "approvals.json")
	first := NewStore(path)
	second := NewStore(path) // constructed before first writes

	if err := first.GrantAlways("bash"); err != nil {
		t.Fatal(err)
	}
	if err := second.GrantSession("filesystem:read_file", "conv-1"); err != nil {
		t.Fatal(err)
	}

	reloaded := NewStore(path)
	if d, ok := reloaded.Grant("bash", ""); !ok || d != AllowAlways {
		t.Fatalf("bash grant lost: %q ok=%v", d, ok)
	}
	if d, ok := reloaded.Grant("filesystem:read_file", "conv-1"); !ok || d != AllowSession {
		t.Fatalf("session grant lost: %q ok=%v", d, ok)
	}
}

func TestStoreRevokeSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "approvals.json")
	store := NewStore(path)
	if err := store.GrantSession("bash", "conv-1"); err != nil {
		t.Fatal(err)
	}
	if err := store.GrantSession("bash", "conv-2"); err != nil {
		t.Fatal(err)
	}

	removed, err := store.RevokeSession("bash", "conv-1")
	if err != nil || !removed {
		t.Fatalf("revoke session: removed=%v err=%v", removed, err)
	}
	if _, ok := store.Grant("bash", "conv-1"); ok {
		t.Fatal("conv-1 grant should be gone")
	}
	if _, ok := store.Grant("bash", "conv-2"); !ok {
		t.Fatal("conv-2 grant should remain")
	}

	removed, _ = store.RevokeSession("bash", "missing")
	if removed {
		t.Fatal("revoking an unknown session should report false")
	}
}

func TestStoreClear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "approvals.json")
	store := NewStore(path)
	_ = store.GrantAlways("bash")
	_ = store.GrantSession("filesystem:read_file", "conv-1")

	if err := store.Clear(); err != nil {
		t.Fatal(err)
	}
	if grants := NewStore(path).Snapshot(); len(grants) != 0 {
		t.Fatalf("grants after clear = %v", grants)
	}
}
