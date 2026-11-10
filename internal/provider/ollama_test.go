package provider

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOllamaMessagesDecodeStringArguments(t *testing.T) {
	messages := []Message{{
		Role: "assistant",
		ToolCalls: []ToolCall{{
			ID:       "call_0",
			Type:     "function",
			Function: FunctionCall{Name: "bash", Arguments: `{"command":"ls"}`},
		}},
	}}

	encoded, err := json.Marshal(ollamaMessages(messages))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"arguments":{"command":"ls"}`) {
		t.Fatalf("ollama arguments are not an object: %s", encoded)
	}

	// The caller's slice must not be mutated.
	if _, ok := messages[0].ToolCalls[0].Function.Arguments.(string); !ok {
		t.Fatalf("source message was modified: %+v", messages[0].ToolCalls[0].Function)
	}
}

func TestOllamaArgumentsKeepObjectForm(t *testing.T) {
	decoded := ollamaArguments(map[string]interface{}{"action": "list"})
	args, ok := decoded.(map[string]interface{})
	if !ok || args["action"] != "list" {
		t.Fatalf("args = %+v", decoded)
	}

	empty := ollamaArguments("")
	if got, ok := empty.(map[string]interface{}); !ok || len(got) != 0 {
		t.Fatalf("empty args = %+v", empty)
	}
}
