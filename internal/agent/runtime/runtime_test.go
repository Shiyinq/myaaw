package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"myaaw/internal/agent"
	"myaaw/internal/agent/approvals"
	"myaaw/internal/config"
	"myaaw/internal/provider"
)

// TestMockCLIHelper acts as a deterministic fake CLI binary when invoked with
// MYAAW_TEST_MOCK_MODE=(acp|claude|pirpc|codex). This lets `go test` verify
// every stdio protocol driver end-to-end with zero external binaries or accounts.
func TestMockCLIHelper(t *testing.T) {
	mode := os.Getenv("MYAAW_TEST_MOCK_MODE")
	if mode == "" {
		return
	}
	switch mode {
	case "acp":
		runMockACP()
	case "claude":
		runMockClaude()
	case "pirpc":
		runMockPiRPC()
	case "codex":
		runMockCodex()
	}
	os.Exit(0)
}

func runMockACP() {
	sc := bufio.NewScanner(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	sessionID := "acp-sess-1"
	for sc.Scan() {
		var req jsonRPCMessage
		if json.Unmarshal(sc.Bytes(), &req) != nil {
			continue
		}
		switch req.Method {
		case "initialize":
			_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"protocolVersion": 1}})
		case "session/new":
			_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"sessionId": sessionID}})
		case "session/load":
			_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{}})
		case "session/prompt":
			// Emit thought, pre-tool text, permission request, tool call, tool result, then final text.
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"method":  "session/update",
				"params": map[string]any{
					"update": map[string]any{
						"sessionUpdate": "agent_thought_chunk",
						"content":       map[string]any{"type": "text", "text": "Checking files..."},
					},
				},
			})
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      99,
				"method":  "session/request_permission",
				"params": map[string]any{
					"toolCall": map[string]any{
						"toolCallId": "tc_1",
						"title":      "run_command",
						"kind":       "execute",
						"rawInput":   map[string]any{"command": "echo hi"},
					},
					"options": []map[string]any{
						{"optionId": "allow_1", "name": "Allow", "kind": "allow_once"},
						{"optionId": "deny_1", "name": "Deny", "kind": "reject_once"},
					},
				},
			})
			// Wait for permission response.
			permDecision := "unknown"
			if sc.Scan() {
				var resp struct {
					Result struct {
						Outcome struct {
							OptionID string `json:"optionId"`
						} `json:"outcome"`
					} `json:"result"`
				}
				if json.Unmarshal(sc.Bytes(), &resp) == nil {
					permDecision = resp.Result.Outcome.OptionID
				}
			}
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"method":  "session/update",
				"params": map[string]any{
					"update": map[string]any{
						"sessionUpdate": "tool_call",
						"toolCallId":    "tc_1",
						"title":         "run_command",
						"rawInput":      map[string]any{"command": "echo hi"},
					},
				},
			})
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"method":  "session/update",
				"params": map[string]any{
					"update": map[string]any{
						"sessionUpdate": "tool_call_update",
						"toolCallId":    "tc_1",
						"title":         "run_command",
						"status":        "completed",
						"rawOutput":     "perm=" + permDecision,
					},
				},
			})
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"method":  "session/update",
				"params": map[string]any{
					"update": map[string]any{
						"sessionUpdate": "agent_message_chunk",
						"content":       map[string]any{"type": "text", "text": "ACP final (" + permDecision + ")"},
					},
				},
			})
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"stopReason": "end_turn",
					"usage":      map[string]any{"inputTokens": 120, "outputTokens": 30},
				},
			})
		}
	}
}

func runMockClaude() {
	sc := bufio.NewScanner(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	if !sc.Scan() {
		return
	}
	_ = enc.Encode(map[string]any{"type": "system", "subtype": "init", "session_id": "claude-sess-9"})
	_ = enc.Encode(map[string]any{
		"type":       "control_request",
		"request_id": "perm_1",
		"subtype":    "can_use_tool",
		"request":    map[string]any{"tool_name": "Bash", "input": map[string]any{"command": "ls"}},
	})
	decision := "none"
	if sc.Scan() {
		var resp struct {
			Decision string `json:"decision"`
		}
		if json.Unmarshal(sc.Bytes(), &resp) == nil {
			decision = resp.Decision
		}
	}
	_ = enc.Encode(map[string]any{
		"type":  "stream_event",
		"event": map[string]any{"type": "content_block_delta", "delta": map[string]any{"type": "text_delta", "text": "Claude answer: " + decision}},
	})
	_ = enc.Encode(map[string]any{
		"type":       "result",
		"session_id": "claude-sess-9",
		"usage":      map[string]any{"input_tokens": 50, "output_tokens": 10},
	})
}

func runMockPiRPC() {
	sc := bufio.NewScanner(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	for sc.Scan() {
		var cmd struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		}
		if json.Unmarshal(sc.Bytes(), &cmd) != nil {
			continue
		}
		if cmd.Type == "prompt" {
			_ = enc.Encode(map[string]any{
				"type":                  "message_update",
				"assistantMessageEvent": map[string]any{"type": "text_delta", "delta": "Pi reply: " + cmd.Message},
			})
			_ = enc.Encode(map[string]any{"type": "agent_end"})
		}
	}
}

func runMockCodex() {
	sc := bufio.NewScanner(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	for sc.Scan() {
		var req jsonRPCMessage
		if json.Unmarshal(sc.Bytes(), &req) != nil {
			continue
		}
		switch req.Method {
		case "initialize":
			_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{}})
		case "thread/start":
			_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"thread": map[string]any{"id": "codex-thr-1"}}})
		case "turn/start":
			_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"turn": map[string]any{"id": "turn-1"}}})
			_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "item/agentMessage/delta", "params": map[string]any{"delta": "Codex OK"}})
			_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "turn/completed", "params": map[string]any{"turn": map[string]any{"status": "completed"}}})
		}
	}
}

func mockBinaryArgs(t *testing.T, mode string) (string, []string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// Wrap the test binary in a tiny POSIX script so extra CLI flags appended by
	// drivers (like claude's -p / --input-format or pirpc's --session) are
	// cleanly accepted without colliding with Go's `testing` flag parser.
	wrapper := t.TempDir() + "/mock-cli.sh"
	script := fmt.Sprintf("#!/bin/sh\nexec %q -test.run=^TestMockCLIHelper$\n", exe)
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MYAAW_TEST_MOCK_MODE", mode)
	return wrapper, nil
}

func TestACPRuntimeStreamingApprovalsAndSessionID(t *testing.T) {
	exe, args := mockBinaryArgs(t, "acp")
	mcpFalse := false
	rt, err := Create(config.RuntimeConfig{
		Preset:         "antigravity",
		Command:        exe,
		Args:           args,
		WorkingDir:     t.TempDir(),
		PermissionMode: config.PermissionModeBridge,
		MCPBridge:      &mcpFalse,
	}, nil)
	if err != nil {
		t.Fatalf("Create ACP runtime: %v", err)
	}
	defer rt.Close()

	var savedRuntime, savedSession string
	var sawToolEnd bool
	gate := &approvals.Gate{
		Policy: approvals.Policy{Mode: "manual"},
		Auto:   approvals.AllowOnce,
	}
	res, err := rt.RunEvents(context.Background(), agent.RunConfig{
		ConversationID: "conv-acp-1",
		ApprovalGate:   gate,
		OnExternalSession: func(rtName, sid string) {
			savedRuntime = rtName
			savedSession = sid
		},
	}, []provider.Message{
		{Role: "system", Content: "Be helpful"},
		{Role: "user", Content: "hello acp"},
	}, func(ev agent.AgentEvent) error {
		if ev.Type == agent.EventToolExecutionEnd {
			sawToolEnd = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ACP RunEvents failed: %v", err)
	}
	if got := provider.MessageText(res.Final); got != "ACP final (allow_1)" {
		t.Fatalf("unexpected final text: %q", got)
	}
	if !sawToolEnd || len(res.Final.Trace) != 1 || res.Final.Usage.TotalTokens != 150 {
		t.Fatalf("expected tool trace + usage 150, got trace=%d usage=%+v", len(res.Final.Trace), res.Final.Usage)
	}
	if savedRuntime != "acp:antigravity" || savedSession != "acp-sess-1" {
		t.Fatalf("unexpected saved session: %q / %q", savedRuntime, savedSession)
	}
}

func TestClaudeCodeRuntimePermissionBridge(t *testing.T) {
	exe, args := mockBinaryArgs(t, "claude")
	mcpFalse := false
	rt, err := Create(config.RuntimeConfig{
		Preset:         "claude_code",
		Command:        exe,
		Args:           args,
		WorkingDir:     t.TempDir(),
		PermissionMode: config.PermissionModeBridge,
		MCPBridge:      &mcpFalse,
	}, nil)
	if err != nil {
		t.Fatalf("Create Claude runtime: %v", err)
	}
	defer rt.Close()

	var savedSession string
	gate := &approvals.Gate{
		Policy: approvals.Policy{Mode: "manual"},
		Auto:   approvals.Deny,
	}
	res, err := rt.RunEvents(context.Background(), agent.RunConfig{
		ConversationID: "conv-claude-1",
		ApprovalGate:   gate,
		OnExternalSession: func(_, sid string) {
			savedSession = sid
		},
	}, []provider.Message{
		{Role: "user", Content: "run ls"},
	}, nil)
	if err != nil {
		t.Fatalf("Claude RunEvents failed: %v", err)
	}
	if got := provider.MessageText(res.Final); got != "Claude answer: deny" {
		t.Fatalf("expected bridged deny response, got %q", got)
	}
	if savedSession != "claude-sess-9" || res.Final.Usage.TotalTokens != 60 {
		t.Fatalf("unexpected session/usage: %q / %+v", savedSession, res.Final.Usage)
	}
}

func TestPiRPCRuntimeAndCodexRuntime(t *testing.T) {
	t.Run("omp_rpc", func(t *testing.T) {
		exe, args := mockBinaryArgs(t, "pirpc")
		mcpFalse := false
		rt, err := Create(config.RuntimeConfig{
			Preset:     "omp",
			Command:    exe,
			Args:       args,
			WorkingDir: t.TempDir(),
			MCPBridge:  &mcpFalse,
		}, nil)
		if err != nil {
			t.Fatalf("Create OMP runtime: %v", err)
		}
		defer rt.Close()

		res, err := rt.RunEvents(context.Background(), agent.RunConfig{
			ConversationID: "conv-omp-1",
		}, []provider.Message{{Role: "user", Content: "ping"}}, nil)
		if err != nil {
			t.Fatalf("OMP RunEvents failed: %v", err)
		}
		if got := provider.MessageText(res.Final); !strings.Contains(got, "Pi reply: ping") {
			t.Fatalf("unexpected OMP reply: %q", got)
		}
	})

	t.Run("codex_app_server", func(t *testing.T) {
		exe, args := mockBinaryArgs(t, "codex")
		mcpFalse := false
		rt, err := Create(config.RuntimeConfig{
			Preset:     "codex",
			Command:    exe,
			Args:       args,
			WorkingDir: t.TempDir(),
			MCPBridge:  &mcpFalse,
		}, nil)
		if err != nil {
			t.Fatalf("Create Codex runtime: %v", err)
		}
		defer rt.Close()

		var savedThread string
		res, err := rt.RunEvents(context.Background(), agent.RunConfig{
			ConversationID: "conv-codex-1",
			OnExternalSession: func(_, sid string) {
				savedThread = sid
			},
		}, []provider.Message{{Role: "user", Content: "status"}}, nil)
		if err != nil {
			t.Fatalf("Codex RunEvents failed: %v", err)
		}
		if got := provider.MessageText(res.Final); got != "Codex OK" || savedThread != "codex-thr-1" {
			t.Fatalf("unexpected Codex result: text=%q thread=%q", got, savedThread)
		}
	})
}

func TestBuildTurnInputDeltaAndRewindSeeding(t *testing.T) {
	history := []provider.Message{
		{Role: "system", Content: "System persona"},
		{Role: "user", Content: "First question"},
		{Role: "assistant", Content: "First answer"},
		{Role: "user", Content: "Follow-up"},
	}

	sys, delta, _ := BuildTurnInput(history, true, false)
	if sys != "System persona" || delta != "Follow-up" {
		t.Fatalf("expected delta-only prompt on resume, got sys=%q prompt=%q", sys, delta)
	}

	_, seeded, _ := BuildTurnInput(history, false, true)
	if !strings.Contains(seeded, "<previous_conversation>") || !strings.Contains(seeded, "First question") || !strings.Contains(seeded, "Follow-up") {
		t.Fatalf("expected seeded history on rewind, got: %q", seeded)
	}
	_ = fmt.Sprint(seeded)
}

func TestResolveBinaryFindsLocalBinUnderMinimalPATH(t *testing.T) {
	fakeHome := t.TempDir()
	localBin := fakeHome + "/.local/bin"
	if err := os.MkdirAll(localBin, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeAgy := localBin + "/agy-test-bin"
	if err := os.WriteFile(fakeAgy, []byte("#!/bin/sh\necho 1.0.0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", fakeHome)
	t.Setenv("PATH", "/usr/bin:/bin")

	resolved, err := ResolveBinary("agy-test-bin")
	if err != nil {
		t.Fatalf("expected ResolveBinary to find binary in ~/.local/bin under minimal PATH: %v", err)
	}
	if resolved != fakeAgy {
		t.Fatalf("expected %q, got %q", fakeAgy, resolved)
	}
}
