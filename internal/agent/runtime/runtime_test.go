package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
			// Trigger MCP tool approval via mcpServer/elicitation/request and wait for response.
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      77,
				"method":  "mcpServer/elicitation/request",
				"params": map[string]any{
					"threadId":   "codex-thr-1",
					"turnId":     "turn-1",
					"serverName": "myaaw",
					"mode":       "form",
					"message":    "Allow MCP tool a2a_contact?",
					"_meta":      map[string]any{"tool_name": "a2a_contact"},
				},
			})
			action := "missing"
			if sc.Scan() {
				var resp struct {
					Result struct {
						Action string `json:"action"`
					} `json:"result"`
				}
				if json.Unmarshal(sc.Bytes(), &resp) == nil && resp.Result.Action != "" {
					action = resp.Result.Action
				}
			}
			_ = enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "item/agentMessage/delta", "params": map[string]any{"delta": "Codex OK (" + action + ")"}})
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
		gate := &approvals.Gate{
			Policy: approvals.Policy{Mode: "manual"},
			Auto:   approvals.AllowOnce,
		}
		res, err := rt.RunEvents(context.Background(), agent.RunConfig{
			ConversationID: "conv-codex-1",
			ApprovalGate:   gate,
			OnExternalSession: func(_, sid string) {
				savedThread = sid
			},
		}, []provider.Message{{Role: "user", Content: "status"}}, nil)
		if err != nil {
			t.Fatalf("Codex RunEvents failed: %v", err)
		}
		if got := provider.MessageText(res.Final); got != "Codex OK (accept)" || savedThread != "codex-thr-1" {
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

func TestCodexAndPiMCPBridgeWiring(t *testing.T) {
	specs := []MCPServerSpec{
		{
			Name:    "myaaw",
			Command: "/usr/local/bin/myaaw",
			Args:    []string{"--agent", "coder", "mcp-bridge"},
			Env: map[string]string{
				"MYAAW_BRIDGE_URL":   "http://127.0.0.1:9999",
				"MYAAW_BRIDGE_TOKEN": "secret-tok",
			},
		},
		{
			Name:    "custom.tool",
			Command: "/opt/tools/srv",
		},
	}

	codexArgs := BuildCodexMCPArgs(specs)
	joined := strings.Join(codexArgs, " ")
	for _, want := range []string{
		`-c mcp_servers.myaaw.command="/usr/local/bin/myaaw"`,
		`-c mcp_servers.myaaw.args=["--agent","coder","mcp-bridge"]`,
		`-c mcp_servers.myaaw.env={MYAAW_BRIDGE_TOKEN="secret-tok",MYAAW_BRIDGE_URL="http://127.0.0.1:9999"}`,
		`-c mcp_servers."custom.tool".command="/opt/tools/srv"`,
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("BuildCodexMCPArgs missing %q in %q", want, joined)
		}
	}

	wd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wd, ".pi"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wd, ".pi", "mcp.json"), []byte(`{"mcpServers":{"user_srv":{"command":"echo"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfgPath, err := SyncPiProjectMCPConfig(wd, specs[:1])
	if err != nil {
		t.Fatalf("SyncPiProjectMCPConfig failed: %v", err)
	}
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read synced .pi/mcp.json: %v", err)
	}
	var parsed struct {
		MCPServers map[string]struct {
			Command  string            `json:"command"`
			Args     []string          `json:"args"`
			Env      map[string]string `json:"env"`
			Exposure string            `json:"exposure"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal .pi/mcp.json: %v", err)
	}
	if parsed.MCPServers["user_srv"].Command != "echo" {
		t.Fatalf("expected existing user_srv to be preserved, got %+v", parsed.MCPServers)
	}
	myaawSrv := parsed.MCPServers["myaaw"]
	if myaawSrv.Command != "/usr/local/bin/myaaw" || myaawSrv.Exposure != "direct" || myaawSrv.Env["MYAAW_BRIDGE_TOKEN"] != "secret-tok" {
		t.Fatalf("unexpected myaaw entry in .pi/mcp.json: %+v", myaawSrv)
	}

	if _, err := SyncPiProjectMCPConfig(wd, nil); err != nil {
		t.Fatalf("SyncPiProjectMCPConfig disable failed: %v", err)
	}
	raw2, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw2), `"myaaw"`) || !strings.Contains(string(raw2), `"user_srv"`) {
		t.Fatalf("expected myaaw removed and user_srv kept, got: %s", string(raw2))
	}
}

func TestTurnAccumulatorOrderingAndDuplicateToolUpdates(t *testing.T) {
	t.Run("acp tool_call with completed status stays before final answer", func(t *testing.T) {
		acc := newTurnAccumulator(context.Background(), agent.RunConfig{}, []provider.Message{
			{Role: "user", Content: "check pwd"},
		}, nil)
		handleACPUpdate(json.RawMessage(`{"update":{"sessionUpdate":"agent_thought_chunk","content":{"type":"text","text":"checking workspace"}}}`), acc)
		handleACPUpdate(json.RawMessage(`{"update":{"sessionUpdate":"tool_call","toolCallId":"tc-1","title":"pwd","status":"completed","rawInput":{},"rawOutput":"/tmp/ws"}}`), acc)
		handleACPUpdate(json.RawMessage(`{"update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"Workspace is /tmp/ws"}}}`), acc)
		// Duplicate trailing completed update after final text must be ignored.
		handleACPUpdate(json.RawMessage(`{"update":{"sessionUpdate":"tool_call_update","toolCallId":"tc-1","title":"pwd","status":"completed","rawOutput":"/tmp/ws"}}`), acc)

		res, err := acc.Finish(provider.StopNormal, nil)
		if err != nil {
			t.Fatalf("Finish failed: %v", err)
		}
		if len(res.NewMessages) != 3 {
			t.Fatalf("expected 3 messages [assistant(tool_call), tool, assistant(final)], got %d: %+v", len(res.NewMessages), res.NewMessages)
		}
		if res.NewMessages[0].Role != "assistant" || len(res.NewMessages[0].ToolCalls) != 1 || res.NewMessages[0].Thought != "checking workspace" {
			t.Fatalf("unexpected msg[0]: %+v", res.NewMessages[0])
		}
		if res.NewMessages[1].Role != "tool" || res.NewMessages[1].ToolCallID != "tc-1" {
			t.Fatalf("unexpected msg[1]: %+v", res.NewMessages[1])
		}
		if res.NewMessages[2].Role != "assistant" || len(res.NewMessages[2].ToolCalls) != 0 || provider.MessageText(res.NewMessages[2]) != "Workspace is /tmp/ws" {
			t.Fatalf("unexpected final msg[2]: %+v", res.NewMessages[2])
		}
	})

	t.Run("text streamed while tool is still open persists after tool result", func(t *testing.T) {
		acc := newTurnAccumulator(context.Background(), agent.RunConfig{}, []provider.Message{
			{Role: "user", Content: "check status"},
		}, nil)
		acc.AddThinkingDelta("thinking first")
		acc.StartTool("tc-open", "bash", `{"cmd":"pwd"}`)
		acc.AddTextDelta("Final answer text")
		// Simulate CLI finishing without sending tool_call_update(completed).
		res, err := acc.Finish(provider.StopNormal, nil)
		if err != nil {
			t.Fatalf("Finish failed: %v", err)
		}
		if len(res.NewMessages) != 3 {
			t.Fatalf("expected 3 messages [assistant(tool_call), tool, assistant(final)], got %d: %+v", len(res.NewMessages), res.NewMessages)
		}
		if len(res.NewMessages[0].ToolCalls) != 1 || provider.MessageText(res.NewMessages[0]) != "" {
			t.Fatalf("expected tool-call stub without final answer text, got %+v", res.NewMessages[0])
		}
		if res.NewMessages[1].Role != "tool" || res.NewMessages[1].ToolCallID != "tc-open" {
			t.Fatalf("expected synthetic tool close msg, got %+v", res.NewMessages[1])
		}
		if res.NewMessages[2].Role != "assistant" || len(res.NewMessages[2].ToolCalls) != 0 || provider.MessageText(res.NewMessages[2]) != "Final answer text" {
			t.Fatalf("expected clean final assistant msg, got %+v", res.NewMessages[2])
		}
	})

	t.Run("acp multi-step with array tool_call_update content preserves exact chronological order", func(t *testing.T) {
		acc := newTurnAccumulator(context.Background(), agent.RunConfig{}, []provider.Message{
			{Role: "user", Content: "multi-step test"},
		}, nil)
		// Step 1: thinking -> text -> Bash (pending {}) -> in_progress (real args) -> completed (array content)
		handleACPUpdate(json.RawMessage(`{"update":{"sessionUpdate":"agent_thought_chunk","content":{"type":"text","text":"Think 1"}}}`), acc)
		handleACPUpdate(json.RawMessage(`{"update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"Tahap 1"}}}`), acc)
		handleACPUpdate(json.RawMessage(`{"update":{"sessionUpdate":"tool_call","toolCallId":"call-bash","title":"Bash","status":"pending","rawInput":{}}}`), acc)
		handleACPUpdate(json.RawMessage(`{"update":{"sessionUpdate":"tool_call_update","toolCallId":"call-bash","title":"Bash","status":"in_progress","rawInput":{"command":"date -u"}}}`), acc)
		handleACPUpdate(json.RawMessage(`{"update":{"sessionUpdate":"tool_call_update","toolCallId":"call-bash","title":"Bash","status":"completed","content":[{"type":"content","content":{"type":"text","text":"20:54:56 UTC"}}]}}`), acc)

		// Step 2: thinking -> text -> Read (pending {}) -> in_progress (real path) -> completed (array content)
		handleACPUpdate(json.RawMessage(`{"update":{"sessionUpdate":"agent_thought_chunk","content":{"type":"text","text":"Think 2"}}}`), acc)
		handleACPUpdate(json.RawMessage(`{"update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"Tahap 2"}}}`), acc)
		handleACPUpdate(json.RawMessage(`{"update":{"sessionUpdate":"tool_call","toolCallId":"call-read","title":"Read","status":"pending","rawInput":{}}}`), acc)
		handleACPUpdate(json.RawMessage(`{"update":{"sessionUpdate":"tool_call_update","toolCallId":"call-read","title":"Read","status":"in_progress","rawInput":{"path":"."}}}`), acc)
		handleACPUpdate(json.RawMessage(`{"update":{"sessionUpdate":"tool_call_update","toolCallId":"call-read","title":"Read","status":"completed","content":[{"type":"text","text":"README.md"}]}}`), acc)

		// Step 3: final thinking -> final answer
		handleACPUpdate(json.RawMessage(`{"update":{"sessionUpdate":"agent_thought_chunk","content":{"type":"text","text":"Think 3"}}}`), acc)
		handleACPUpdate(json.RawMessage(`{"update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"Jawaban akhir"}}}`), acc)

		res, err := acc.Finish(provider.StopNormal, nil)
		if err != nil {
			t.Fatalf("Finish failed: %v", err)
		}
		if len(res.NewMessages) != 5 {
			t.Fatalf("expected 5 messages [asst1, tool1, asst2, tool2, asst3], got %d: %+v", len(res.NewMessages), res.NewMessages)
		}
		if res.NewMessages[0].Thought != "Think 1" || provider.MessageText(res.NewMessages[0]) != "Tahap 1" || len(res.NewMessages[0].ToolCalls) != 1 || res.NewMessages[0].ToolCalls[0].Function.Arguments != `{"command":"date -u"}` {
			t.Fatalf("unexpected step 1 assistant msg: %+v", res.NewMessages[0])
		}
		if res.NewMessages[1].Role != "tool" || res.NewMessages[1].ToolCallID != "call-bash" || provider.MessageText(res.NewMessages[1]) != "20:54:56 UTC" {
			t.Fatalf("unexpected step 1 tool msg: %+v", res.NewMessages[1])
		}
		if res.NewMessages[2].Thought != "Think 2" || provider.MessageText(res.NewMessages[2]) != "Tahap 2" || len(res.NewMessages[2].ToolCalls) != 1 || res.NewMessages[2].ToolCalls[0].Function.Arguments != `{"path":"."}` {
			t.Fatalf("unexpected step 2 assistant msg: %+v", res.NewMessages[2])
		}
		if res.NewMessages[3].Role != "tool" || res.NewMessages[3].ToolCallID != "call-read" || provider.MessageText(res.NewMessages[3]) != "README.md" {
			t.Fatalf("unexpected step 2 tool msg: %+v", res.NewMessages[3])
		}
		if res.NewMessages[4].Thought != "Think 3" || provider.MessageText(res.NewMessages[4]) != "Jawaban akhir" || len(res.NewMessages[4].ToolCalls) != 0 {
			t.Fatalf("unexpected final assistant msg: %+v", res.NewMessages[4])
		}
	})
}

func TestCodexReasoningAndTurnNotifications(t *testing.T) {
	t.Run("codexReasoningParams maps thinking level to effort and summary", func(t *testing.T) {
		cases := []struct {
			in          string
			wantEffort  string
			wantSummary string
		}{
			{"", "", "auto"},
			{"high", "high", "auto"},
			{"XHigh", "xhigh", "auto"},
			{"off", "none", "none"},
			{"none", "none", "none"},
		}
		for _, tc := range cases {
			effort, summary := codexReasoningParams(tc.in)
			if effort != tc.wantEffort || summary != tc.wantSummary {
				t.Fatalf("codexReasoningParams(%q) = (%q, %q), want (%q, %q)", tc.in, effort, summary, tc.wantEffort, tc.wantSummary)
			}
		}
	})

	t.Run("codex turn preserves reasoning summaries, commentary separation, and tool calls", func(t *testing.T) {
		acc := newTurnAccumulator(context.Background(), agent.RunConfig{}, []provider.Message{
			{Role: "user", Content: "multi-step codex test"},
		}, nil)
		state := newCodexTurnState(acc)
		var doneErr error
		doneCalled := false
		signalDone := func(err error) {
			doneCalled = true
			doneErr = err
		}

		// Step 1: streamed multi-part reasoning summary -> commentary agentMessage -> dynamicToolCall
		state.handleNotification("item/reasoning/summaryPartAdded", json.RawMessage(`{"itemId":"rs-1","summaryIndex":0}`), signalDone)
		state.handleNotification("item/reasoning/summaryTextDelta", json.RawMessage(`{"itemId":"rs-1","delta":"**Step 1**"}`), signalDone)
		state.handleNotification("item/reasoning/summaryPartAdded", json.RawMessage(`{"itemId":"rs-1","summaryIndex":1}`), signalDone)
		state.handleNotification("item/reasoning/summaryTextDelta", json.RawMessage(`{"itemId":"rs-1","delta":"Check current time."}`), signalDone)
		state.handleNotification("item/completed", json.RawMessage(`{"item":{"id":"rs-1","type":"reasoning","summary":["**Step 1**","Check current time."],"content":[]}}`), signalDone)

		state.handleNotification("item/agentMessage/delta", json.RawMessage(`{"itemId":"msg-1","delta":"Checking time now."}`), signalDone)
		state.handleNotification("item/completed", json.RawMessage(`{"item":{"id":"msg-1","type":"agentMessage","phase":"commentary","text":"Checking time now."}}`), signalDone)

		state.handleNotification("item/started", json.RawMessage(`{"item":{"id":"dyn-1","type":"dynamicToolCall","tool":"get_time","arguments":{"tz":"WIB"}}}`), signalDone)
		state.handleNotification("item/completed", json.RawMessage(`{"item":{"id":"dyn-1","type":"dynamicToolCall","tool":"get_time","status":"completed","success":true,"contentItems":[{"type":"inputText","text":"23:59 WIB"}]}}`), signalDone)

		// Step 2: non-streamed reasoning fallback on item/completed -> commentary -> sleep + commandExecution
		state.handleNotification("item/completed", json.RawMessage(`{"item":{"id":"rs-2","type":"reasoning","summary":["**Step 2**","Run pwd."],"content":[]}}`), signalDone)
		state.handleNotification("item/agentMessage/delta", json.RawMessage(`{"itemId":"msg-2","delta":"Now running pwd."}`), signalDone)
		state.handleNotification("item/started", json.RawMessage(`{"item":{"id":"sleep-1","type":"sleep","durationMs":1000}}`), signalDone)
		state.handleNotification("item/completed", json.RawMessage(`{"item":{"id":"sleep-1","type":"sleep","durationMs":1000}}`), signalDone)
		state.handleNotification("item/started", json.RawMessage(`{"item":{"id":"cmd-1","type":"commandExecution","command":"/bin/zsh -lc pwd"}}`), signalDone)
		state.handleNotification("item/completed", json.RawMessage(`{"item":{"id":"cmd-1","type":"commandExecution","status":"completed","aggregatedOutput":"/workspace\n"}}`), signalDone)

		// Step 3: back-to-back commentary + final_answer agentMessage items without an intervening tool
		state.handleNotification("item/agentMessage/delta", json.RawMessage(`{"itemId":"msg-3","delta":"Wrapping up."}`), signalDone)
		state.handleNotification("item/agentMessage/delta", json.RawMessage(`{"itemId":"msg-4","delta":"All done in /workspace."}`), signalDone)
		state.handleNotification("turn/completed", json.RawMessage(`{"turn":{"status":"completed"}}`), signalDone)

		if !doneCalled || doneErr != nil {
			t.Fatalf("expected clean turn completion, called=%v err=%v", doneCalled, doneErr)
		}

		res, err := acc.Finish(provider.StopNormal, nil)
		if err != nil {
			t.Fatalf("Finish failed: %v", err)
		}
		// Expected: [asst1(rs-1 + msg-1 + dyn-1), tool1, asst2(rs-2 + msg-2 + sleep-1), tool(sleep), asst3(cmd-1), tool(cmd), asst4(msg-3), asst5(msg-4)]
		if len(res.NewMessages) != 8 {
			t.Fatalf("expected 8 messages, got %d: %+v", len(res.NewMessages), res.NewMessages)
		}
		if res.NewMessages[0].Thought != "**Step 1**\n\nCheck current time." || provider.MessageText(res.NewMessages[0]) != "Checking time now." {
			t.Fatalf("unexpected step 1 msg: %+v", res.NewMessages[0])
		}
		if res.NewMessages[1].Role != "tool" || res.NewMessages[1].Name != "get_time" || provider.MessageText(res.NewMessages[1]) != "23:59 WIB" {
			t.Fatalf("unexpected step 1 tool msg: %+v", res.NewMessages[1])
		}
		if res.NewMessages[2].Thought != "**Step 2**\n\nRun pwd." || provider.MessageText(res.NewMessages[2]) != "Now running pwd." {
			t.Fatalf("unexpected step 2 msg: %+v", res.NewMessages[2])
		}
		if res.NewMessages[3].Role != "tool" || res.NewMessages[3].Name != "sleep" || provider.MessageText(res.NewMessages[3]) != "Slept 1000ms" {
			t.Fatalf("unexpected sleep tool msg: %+v", res.NewMessages[3])
		}
		if res.NewMessages[5].Role != "tool" || res.NewMessages[5].Name != "command" || provider.MessageText(res.NewMessages[5]) != "/workspace\n" {
			t.Fatalf("unexpected step 2 command tool msg: %+v", res.NewMessages[5])
		}
		if provider.MessageText(res.NewMessages[6]) != "Wrapping up." {
			t.Fatalf("expected separate commentary msg[6], got %+v", res.NewMessages[6])
		}
		if provider.MessageText(res.NewMessages[7]) != "All done in /workspace." {
			t.Fatalf("expected clean final answer msg[7], got %+v", res.NewMessages[7])
		}
	})
}

func TestTelemetryOptOutArgsAndEnv(t *testing.T) {
	envMap := func(env []string) map[string]string {
		out := make(map[string]string, len(env))
		for _, entry := range env {
			if k, v, ok := strings.Cut(entry, "="); ok {
				out[k] = v
			}
		}
		return out
	}

	// Antigravity ACP injects --disable-telemetry even for legacy saved args ["acp"]
	agyLegacy := config.RuntimeConfig{Type: config.RuntimeTypeACP, Preset: "antigravity", Command: "agy", Args: []string{"acp"}}
	agyArgs := acpSubprocessArgs(agyLegacy)
	if len(agyArgs) != 2 || agyArgs[0] != "acp" || agyArgs[1] != "--disable-telemetry" {
		t.Fatalf("expected --disable-telemetry appended for agy acp, got %v", agyArgs)
	}
	// Does not duplicate when already present
	agyPreset := config.MergeRuntime(&config.RuntimeConfig{Preset: "antigravity"})
	if got := acpSubprocessArgs(agyPreset); len(got) != 2 {
		t.Fatalf("expected no duplicate --disable-telemetry, got %v", got)
	}

	// Codex disables analytics in both probe and turn modes
	codexCfg := config.MergeRuntime(&config.RuntimeConfig{Preset: "codex", ThinkingLevel: "high"})
	probeArgs := strings.Join(codexSubprocessArgs(codexCfg, false), " ")
	if !strings.Contains(probeArgs, "-c analytics.enabled=false") {
		t.Fatalf("expected codex probe args to disable analytics, got %q", probeArgs)
	}
	turnArgs := strings.Join(codexSubprocessArgs(codexCfg, true), " ")
	for _, want := range []string{"-c analytics.enabled=false", `-c model_reasoning_effort="high"`, `-c model_reasoning_summary="auto"`} {
		if !strings.Contains(turnArgs, want) {
			t.Fatalf("expected codex turn args to contain %q, got %q", want, turnArgs)
		}
	}

	// Per-runtime telemetry opt-out env vars
	cases := []struct {
		name string
		cfg  config.RuntimeConfig
		want map[string]string
	}{
		{
			name: "antigravity",
			cfg:  agyPreset,
			want: map[string]string{"DO_NOT_TRACK": "1", "OTEL_SDK_DISABLED": "true", "GEMINI_TELEMETRY_ENABLED": "false"},
		},
		{
			name: "custom_gemini",
			cfg:  config.RuntimeConfig{Type: config.RuntimeTypeACP, Preset: "custom_acp", Command: "/usr/local/bin/gemini"},
			want: map[string]string{"DO_NOT_TRACK": "1", "OTEL_SDK_DISABLED": "true", "GEMINI_TELEMETRY_ENABLED": "false"},
		},
		{
			name: "claude_code",
			cfg:  config.MergeRuntime(&config.RuntimeConfig{Preset: "claude_code"}),
			want: map[string]string{"DO_NOT_TRACK": "1", "DISABLE_TELEMETRY": "1", "DISABLE_ERROR_REPORTING": "1", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"},
		},
		{
			name: "deepseek_harness",
			cfg:  config.MergeRuntime(&config.RuntimeConfig{Preset: "deepseek_harness"}),
			want: map[string]string{"DO_NOT_TRACK": "1", "DSH_TELEMETRY_DISABLED": "1", "DSH_TELEMETRY_MODE": "DISABLED"},
		},
		{
			name: "opencode",
			cfg:  config.MergeRuntime(&config.RuntimeConfig{Preset: "opencode"}),
			want: map[string]string{"DO_NOT_TRACK": "1", "OTEL_SDK_DISABLED": "true", "OPENCODE_DISABLE_AUTOUPDATE": "true"},
		},
		{
			name: "pi",
			cfg:  config.MergeRuntime(&config.RuntimeConfig{Preset: "pi"}),
			want: map[string]string{"DO_NOT_TRACK": "1", "PI_TELEMETRY": "false"},
		},
		{
			name: "omp",
			cfg:  config.MergeRuntime(&config.RuntimeConfig{Preset: "omp"}),
			want: map[string]string{"DO_NOT_TRACK": "1", "PI_TELEMETRY": "false"},
		},
		{
			name: "codex",
			cfg:  codexCfg,
			want: map[string]string{"DO_NOT_TRACK": "1", "OTEL_SDK_DISABLED": "true", "OTEL_TRACES_EXPORTER": "none", "OTEL_METRICS_EXPORTER": "none", "OTEL_LOGS_EXPORTER": "none"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := envMap(BuildSubprocessEnv(tc.cfg, "", ""))
			for k, wantVal := range tc.want {
				if got[k] != wantVal {
					t.Fatalf("%s: env[%q] = %q, want %q", tc.name, k, got[k], wantVal)
				}
			}
		})
	}
}
