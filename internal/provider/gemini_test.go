package provider

import "testing"

func TestMessagesToContentsToolResultImages(t *testing.T) {
	contents := MessagesToContents([]Message{{
		Role: "tool",
		Name: "screenshot",
		Content: []ContentItem{
			{Type: "text", Text: "captured"},
			{Type: "image_url", ImageURL: &ImageInfo{URL: "data:image/png;base64,AAAA"}},
		},
	}})

	if len(contents) != 1 {
		t.Fatalf("contents = %d, want 1", len(contents))
	}
	var (
		sawText     bool
		sawImage    bool
		sawResponse bool
	)
	for _, part := range contents[0].Parts {
		if part.Text == "captured" {
			sawText = true
		}
		if part.InlineData != nil {
			if part.InlineData.MimeType != "image/png" || part.InlineData.Data != "AAAA" {
				t.Fatalf("inline data = %+v", part.InlineData)
			}
			sawImage = true
		}
		if part.FunctionResponse != nil {
			sawResponse = true
			if got := part.FunctionResponse.Response["result"]; got != "captured" {
				t.Fatalf("function response result = %v, want the text only", got)
			}
		}
	}
	if !sawText || !sawImage || !sawResponse {
		t.Fatalf("parts = %+v (text=%v image=%v response=%v)", contents[0].Parts, sawText, sawImage, sawResponse)
	}
}

func TestMessagesToContentsTextOnlyUnchanged(t *testing.T) {
	contents := MessagesToContents([]Message{
		{Role: "user", Content: "hello"},
		{Role: "tool", Name: "ls", Content: `{"ok":true}`},
	})
	// Both roles map to Gemini's "user" and are bundled into one content.
	if len(contents) != 1 {
		t.Fatalf("contents = %d", len(contents))
	}
	parts := contents[0].Parts
	if len(parts) != 3 || parts[0].Text != "hello" || parts[1].Text != `{"ok":true}` || parts[2].FunctionResponse == nil {
		t.Fatalf("parts = %+v", parts)
	}
}

func TestContentToMessageStoresToolArgumentsAsJSONString(t *testing.T) {
	msg := contentToMessage(GeminiContent{
		Role: "model",
		Parts: []GeminiPart{{
			FunctionCall:     &GeminiFunctionCall{Name: "provider", Args: map[string]interface{}{"action": "list"}},
			ThoughtSignature: "sig-1",
		}},
	})

	if msg.Role != "assistant" {
		t.Fatalf("role = %q", msg.Role)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("tool calls = %+v", msg.ToolCalls)
	}
	call := msg.ToolCalls[0]
	if call.Function.Name != "provider" || call.Function.Arguments != `{"action":"list"}` {
		t.Fatalf("call = %+v", call.Function)
	}
	if call.Function.ThoughtSignature != "sig-1" {
		t.Fatalf("thought signature = %q", call.Function.ThoughtSignature)
	}
}

func TestMessagesToContentsDecodesStringToolArguments(t *testing.T) {
	contents := MessagesToContents([]Message{{
		Role: "assistant",
		ToolCalls: []ToolCall{{
			ID:       "call_0",
			Type:     "function",
			Function: FunctionCall{Name: "provider", Arguments: `{"action":"list"}`},
		}},
	}})

	if len(contents) != 1 || len(contents[0].Parts) != 1 {
		t.Fatalf("contents = %+v", contents)
	}
	call := contents[0].Parts[0].FunctionCall
	if call == nil || call.Name != "provider" {
		t.Fatalf("function call = %+v", call)
	}
	if call.Args["action"] != "list" {
		t.Fatalf("args = %+v", call.Args)
	}
}

func TestParseImageDataURL(t *testing.T) {
	mimeType, data, ok := parseImageDataURL("data:image/jpeg;base64,/9j/AAAA")
	if !ok || mimeType != "image/jpeg" || data != "/9j/AAAA" {
		t.Fatalf("parsed = %q %q %v", mimeType, data, ok)
	}
	for _, raw := range []string{"", "http://example.com/a.png", "data:image/png,raw", "data:image/png;base64,"} {
		if _, _, ok := parseImageDataURL(raw); ok {
			t.Fatalf("%q should not parse", raw)
		}
	}
}
