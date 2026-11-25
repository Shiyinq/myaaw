package mcpbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	agentruntime "myaaw/internal/agent/runtime"
	"myaaw/internal/agent/tools"
	"myaaw/internal/provider"
)

type testEchoTool struct{}

func (t *testEchoTool) ToolDefinition() []byte {
	return []byte(`{"type":"function","function":{"name":"test_bridge_echo","description":"Echo with user_id","parameters":{"type":"object","properties":{"msg":{"type":"string"}}}}}`)
}

func (t *testEchoTool) CallTool(arguments string, ctx *tools.ToolsContext) string {
	return arguments
}

func (t *testEchoTool) CallToolStructured(_ context.Context, arguments string, ctx *tools.ToolsContext) tools.StructuredResult {
	uid := ""
	if ctx != nil {
		uid = ctx.UserID
	}
	return tools.StructuredResult{
		Text: "echo:" + uid + ":" + arguments,
		ExtraMessages: []provider.ExtraMessage{
			{ID: "blk_1", Blocks: []provider.ExtraBlock{{ID: "b1", Type: "card"}}},
		},
	}
}

func TestServeMCPBridgeExcludesCoreToolsAndPropagatesSideEffects(t *testing.T) {
	tools.RegisterBuiltin("test_bridge_echo", &testEchoTool{})

	bridge, err := agentruntime.NewTurnBridgeServer()
	if err != nil {
		t.Fatalf("NewTurnBridgeServer: %v", err)
	}
	defer bridge.Close()

	var gotExtra []provider.ExtraMessage
	release := bridge.SetActive(&agentruntime.TurnSideEffects{
		ToolsContext: &tools.ToolsContext{UserID: "42", Channel: "api"},
		OnExtraMessages: func(items []provider.ExtraMessage) {
			gotExtra = append(gotExtra, items...)
		},
	})
	defer release()

	t.Setenv("MYAAW_BRIDGE_URL", bridge.URL())
	t.Setenv("MYAAW_BRIDGE_TOKEN", bridge.Token())

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"test_bridge_echo","arguments":{"msg":"hi"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"filesystem","arguments":{}}}`,
	}, "\n") + "\n"

	var out bytes.Buffer
	if err := Serve(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 JSON-RPC responses, got %d:\n%s", len(lines), out.String())
	}

	// Verify tools/list excludes core coding tools.
	var listResp struct {
		Result struct {
			Tools []mcpToolDesc `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &listResp); err != nil {
		t.Fatalf("parse tools/list: %v", err)
	}
	foundEcho := false
	for _, tool := range listResp.Result.Tools {
		for _, core := range CoreCodingTools {
			if tool.Name == core {
				t.Fatalf("core tool %q must not be exposed via mcp-bridge", core)
			}
		}
		if tool.Name == "test_bridge_echo" {
			foundEcho = true
		}
	}
	if !foundEcho {
		t.Fatalf("expected test_bridge_echo in tools/list")
	}

	// Verify tools/call propagated ToolsContext and ExtraMessages side effect.
	var callResp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[2]), &callResp); err != nil || len(callResp.Result.Content) == 0 || callResp.Result.Content[0].Text != `echo:42:{"msg":"hi"}` {
		t.Fatalf("unexpected tools/call response: %s", lines[2])
	}
	if len(gotExtra) != 1 || gotExtra[0].ID != "blk_1" {
		t.Fatalf("expected side-effect ExtraMessages to be streamed, got: %+v", gotExtra)
	}

	// Verify calling blocked core tool returns isError=true.
	if !strings.Contains(lines[3], `"isError":true`) {
		t.Fatalf("expected filesystem tool call to return isError, got: %s", lines[3])
	}
}
