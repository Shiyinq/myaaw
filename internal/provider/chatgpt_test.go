package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"myaaw/internal/auth/chatgpt"
	"myaaw/internal/config"

	"go.mongodb.org/mongo-driver/bson"
)

func chatGPTTestProvider(t *testing.T, handler http.HandlerFunc) *ChatGPTProvider {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	p, err := NewChatGPTProvider(config.ProviderConfig{Type: chatgpt.ProviderType, AuthMode: "oauth", CredentialID: "account-1", DefaultModel: "account-model"})
	if err != nil {
		t.Fatal(err)
	}
	p.baseURL, p.client = server.URL, server.Client()
	p.tokenSource = func(ctx context.Context, id string) (string, error) {
		if id != "account-1" {
			t.Errorf("credential ID = %q", id)
		}
		return "oauth-test-token", nil
	}
	return p
}

func chatGPTTestSSE(w http.ResponseWriter, event interface{}) {
	data, _ := json.Marshal(event)
	fmt.Fprintf(w, "data: %s\n\n", data)
}

func chatGPTTestCompleted(w http.ResponseWriter, output interface{}) {
	chatGPTTestSSE(w, map[string]interface{}{"type": "response.completed", "response": map[string]interface{}{
		"status": "completed", "output": output, "usage": map[string]interface{}{
			"input_tokens": 9, "output_tokens": 6, "total_tokens": 15,
			"input_tokens_details":  map[string]interface{}{"cached_tokens": 3},
			"output_tokens_details": map[string]interface{}{"reasoning_tokens": 2},
		},
	}})
}

func chatGPTTestOutput(text string) []interface{} {
	return []interface{}{map[string]interface{}{"type": "message", "id": "msg-1", "role": "assistant", "status": "completed",
		"content": []interface{}{map[string]interface{}{"type": "output_text", "text": text, "annotations": []interface{}{}}}}}
}

func TestChatGPTConstructionAndConfig(t *testing.T) {
	if !IsKnownLLMProvider(chatgpt.ProviderType) {
		t.Fatal("OAuth type is unknown")
	}
	if _, ok := LLMproviderFactories[chatgpt.ProviderType]; ok {
		t.Fatal("OAuth must not use an API-key factory")
	}
	if DefaultModel(chatgpt.ProviderType) != "" {
		t.Fatal("must not guess an accessible model")
	}
	if DefaultBaseURL(chatgpt.ProviderType) != chatGPTBaseURL {
		t.Fatal("wrong public endpoint")
	}
	for _, base := range []string{"http://api.openai.com", "https://attacker.invalid", "https://api.openai.com/v1", "https://api.openai.com@attacker.invalid", "https://api.openai.com?x=1"} {
		_, err := CreateLLMProviderFromConfig(config.ProviderConfig{Type: chatgpt.ProviderType, CredentialID: "account-1", BaseURL: base})
		if err == nil || !strings.Contains(err.Error(), "custom base URLs") {
			t.Errorf("base URL %q: %v", base, err)
		}
	}
	for _, cfg := range []config.ProviderConfig{
		{Type: chatgpt.ProviderType},
		{Type: chatgpt.ProviderType, CredentialID: "account-1", APIKey: "sk-key"},
		{Type: chatgpt.ProviderType, CredentialID: "account-1", AuthMode: "api_key"},
		{Type: "openai", AuthMode: "oauth", CredentialID: "account-1"},
	} {
		if _, err := CreateLLMProviderFromConfig(cfg); err == nil {
			t.Errorf("accepted invalid config %+v", cfg)
		}
	}
	if _, err := ModelsFor(chatgpt.ProviderType, "", ""); err == nil || !strings.Contains(err.Error(), "credential_id") {
		t.Fatalf("legacy discovery should explain credentials: %v", err)
	}
	if _, err := ModelsForConfig(config.ProviderConfig{Type: chatgpt.ProviderType}); err == nil {
		t.Fatal("missing credential accepted")
	}
	p, err := NewChatGPTProvider(config.ProviderConfig{CredentialID: "account-1"})
	if err != nil {
		t.Fatal(err)
	}
	if p.ProviderName() != chatgpt.ProviderType || p.DisplayName() != chatgpt.Label || p.DefaultModel("") != "" {
		t.Fatal("provider identity/default mismatch")
	}
	if p.DefaultModel("selected") != "selected" {
		t.Fatal("explicit model lost")
	}
}

func TestChatGPTLegacyFactoryUsesOAuthGlobals(t *testing.T) {
	base, model, mode, id := config.LLMProviderBaseURL, config.LLMDefaultModel, config.LLMProviderAuthMode, config.LLMProviderCredentialID
	t.Cleanup(func() {
		config.LLMProviderBaseURL, config.LLMDefaultModel, config.LLMProviderAuthMode, config.LLMProviderCredentialID = base, model, mode, id
	})
	config.LLMProviderBaseURL, config.LLMDefaultModel = chatGPTBaseURL, "selected-model"
	config.LLMProviderAuthMode, config.LLMProviderCredentialID = "oauth", "selected-account"
	provider, err := CreateLLMProvider(chatgpt.ProviderType, "")
	if err != nil {
		t.Fatal(err)
	}
	p := provider.(*ChatGPTProvider)
	if p.credentialID != "selected-account" || p.DefaultModel("") != "selected-model" {
		t.Fatalf("wrong config: %+v", p)
	}
	// The old API-key branch must not start interpreting the new globals.
	legacy, err := CreateLLMProvider("openai", "legacy-key")
	if err != nil {
		t.Fatal(err)
	}
	if legacy.(*OpenAIProvider).apiKey != "legacy-key" {
		t.Fatal("old API key was changed")
	}
}

func TestChatGPTModelCatalog(t *testing.T) {
	requests := 0
	p := chatGPTTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/v1/models" || r.Method != "GET" || r.Header.Get("Authorization") != "Bearer oauth-test-token" {
			t.Errorf("wrong model request: %s %s %v", r.Method, r.URL, r.Header)
		}
		fmt.Fprint(w, `{"models":[{"slug":"second","display_name":"Second display","visibility":"list"},{"slug":"hidden","visibility":"hide"},{"slug":"first","display_name":"First display","visibility":"list"},{"slug":"","visibility":"list"}],"data":[{"id":"not-the-api-key-catalog"}]}`)
	})
	catalog, err := p.ModelCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) != 2 || catalog[0].Slug != "second" || catalog[0].DisplayName != "Second display" {
		t.Fatalf("bad catalog: %+v", catalog)
	}
	models, err := p.Models()
	if err != nil || !reflect.DeepEqual(models, []string{"second", "first"}) {
		t.Fatalf("models = %v, %v", models, err)
	}
	if requests != 2 {
		t.Fatal("model catalog unexpectedly cached")
	}
}

func TestChatGPTOAuthToolLoop(t *testing.T) {
	var requests []map[string]interface{}
	p := chatGPTTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" || r.Method != "POST" {
			t.Errorf("wrong response endpoint: %s %s", r.Method, r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer oauth-test-token" || r.Header.Get("Accept") != "text/event-stream" {
			t.Error("wrong auth/accept")
		}
		var request map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		requests = append(requests, request)
		w.Header().Set("Content-Type", "text/event-stream")
		if len(requests) == 1 {
			chatGPTTestSSE(w, map[string]interface{}{"type": "response.output_item.added", "output_index": 0, "item": map[string]interface{}{"type": "message", "id": "msg-1", "role": "assistant", "content": []interface{}{}}})
			chatGPTTestSSE(w, map[string]interface{}{"type": "response.output_text.delta", "output_index": 0, "item_id": "msg-1", "delta": "Checking."})
			chatGPTTestSSE(w, map[string]interface{}{"type": "response.output_item.added", "output_index": 2, "item": map[string]interface{}{"type": "function_call", "id": "fc-1", "call_id": "call-1", "namespace": "myaaw", "name": "lookup", "arguments": ""}})
			chatGPTTestSSE(w, map[string]interface{}{"type": "response.function_call_arguments.delta", "output_index": 2, "item_id": "fc-1", "delta": `{"city":`})
			chatGPTTestSSE(w, map[string]interface{}{"type": "response.function_call_arguments.delta", "output_index": 2, "item_id": "fc-1", "delta": `"Paris"}`})
			chatGPTTestSSE(w, map[string]interface{}{"type": "response.function_call_arguments.done", "output_index": 2, "item_id": "fc-1", "arguments": `{"city":"Paris"}`})
			output := chatGPTTestOutput("Checking.")
			output = append(output, map[string]interface{}{"type": "reasoning", "id": "rs-1", "encrypted_content": "opaque-ciphertext", "summary": []interface{}{}},
				map[string]interface{}{"type": "function_call", "id": "fc-1", "call_id": "call-1", "namespace": "myaaw", "name": "lookup", "arguments": `{"city":"Paris"}`, "status": "completed"})
			chatGPTTestCompleted(w, output)
		} else {
			chatGPTTestCompleted(w, chatGPTTestOutput("It is sunny."))
		}
	})
	tools := []map[string]interface{}{{"type": "function", "function": map[string]interface{}{"name": "lookup", "description": "Local weather", "parameters": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"city": map[string]interface{}{"type": "string"}}}}}}
	history := []Message{{Role: "system", Content: "Be helpful."}, {Role: "user", Content: "Weather?"}}
	first, err := p.Chat("", history, tools)
	if err != nil {
		t.Fatal(err)
	}
	if MessageText(first) != "Checking." || len(first.ToolCalls) != 1 || first.ToolCalls[0].ID != "call-1" || first.ToolCalls[0].Function.Name != "lookup" || ToolCallArgumentsString(first.ToolCalls[0].Function.Arguments) != `{"city":"Paris"}` {
		t.Fatalf("wrong first response: %+v", first)
	}
	if first.Usage.PromptTokens != 9 || first.Usage.CompletionTokens != 6 || first.Usage.CachedInputTokens != 3 || first.Usage.ThoughtsTokens != 2 {
		t.Fatalf("wrong usage: %+v", first.Usage)
	}
	// Exercise both transcript encodings before continuing the tool turn.
	data, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	var persisted Message
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	data, err = bson.Marshal(persisted)
	if err != nil {
		t.Fatal(err)
	}
	if err := bson.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	history = append(history, persisted, Message{Role: "tool", Name: "lookup", ToolCallID: "call-1", Content: `{"weather":"sunny"}`})
	second, err := p.Chat("", history, tools)
	if err != nil || MessageText(second) != "It is sunny." {
		t.Fatalf("continuation: %+v, %v", second, err)
	}
	if len(requests) != 2 {
		t.Fatalf("request count %d", len(requests))
	}
	for _, request := range requests {
		if request["store"] != false || request["stream"] != true || request["model"] != "account-model" {
			t.Errorf("wrong flags: %v", request)
		}
		for _, key := range []string{"previous_response_id", "temperature", "top_p", "max_output_tokens", "messages", "metadata", "conversation"} {
			if _, ok := request[key]; ok {
				t.Errorf("unsupported request field %s", key)
			}
		}
		wireTools := request["tools"].([]interface{})
		group := wireTools[0].(map[string]interface{})
		if group["type"] != "namespace" || group["name"] != "myaaw" {
			t.Errorf("bare or invalid tools: %v", group)
		}
		function := group["tools"].([]interface{})[0].(map[string]interface{})
		if function["type"] != "function" || function["name"] != "lookup" || function["strict"] != false {
			t.Errorf("wrong tool conversion: %v", function)
		}
	}
	input := requests[1]["input"].([]interface{})
	if len(input) != 6 || input[0].(map[string]interface{})["role"] != "developer" {
		t.Fatalf("not full local history: %v", input)
	}
	reasoning := input[3].(map[string]interface{})
	if reasoning["encrypted_content"] != "opaque-ciphertext" || reasoning["summary"] == nil {
		t.Fatalf("lost reasoning: %v", reasoning)
	}
	call, result := input[4].(map[string]interface{}), input[5].(map[string]interface{})
	if call["namespace"] != "myaaw" || call["call_id"] != result["call_id"] || result["type"] != "function_call_output" {
		t.Fatalf("bad tool pairing: %v %v", call, result)
	}
}

func TestChatGPTRedirectDoesNotForwardToken(t *testing.T) {
	forwarded := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded = true }))
	defer destination.Close()
	p := chatGPTTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	})
	_, err := p.Models()
	var apiError *ChatGPTAPIError
	if !errors.As(err, &apiError) || apiError.Status != 307 || forwarded {
		t.Fatalf("redirect policy: forwarded=%v, error=%v", forwarded, err)
	}
}

func TestChatGPTHTTPQuotaAndAdmissionErrors(t *testing.T) {
	for _, tc := range []struct {
		status         int
		body, contains string
	}{
		{429, `{"error":{"code":"subscription_sharing_usage_limit_exceeded","param":"model","message":"quota"}}`, "https://chatgpt.com/settings/usage"},
		{503, `{"error":{"code":"subscription_sharing_usage_unavailable","message":"unknown"}}`, "Keep your credential"},
		{403, `{"error":{"code":"subscription_sharing_user_not_eligible","message":"no"}}`, "eligible account"},
		{401, `{"detail":"identity permission not accepted"}`, "identity permission not accepted"},
		{403, `{"detail":"regional policy"}`, "regional policy"},
	} {
		t.Run(fmt.Sprint(tc.status, tc.contains), func(t *testing.T) {
			requests := 0
			p := chatGPTTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Header().Set("x-request-id", "req-test")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			})
			_, err := p.Chat("", []Message{{Role: "user", Content: "Hi"}}, nil)
			var apiError *ChatGPTAPIError
			if !errors.As(err, &apiError) || apiError.Status != tc.status || apiError.RequestID != "req-test" || apiError.Body != tc.body || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("lost error details: %+v, %v", apiError, err)
			}
			if requests != 1 {
				t.Fatalf("unexpected retry %d", requests)
			}
		})
	}
}

func TestChatGPTInputValidationAndImages(t *testing.T) {
	call := ToolCall{ID: "call-1", Type: "function", Function: FunctionCall{Name: "lookup", Arguments: `{}`}}
	for _, history := range [][]Message{
		{{Role: "tool", ToolCallID: "unknown", Content: "orphan"}},
		{{Role: "assistant", ToolCalls: []ToolCall{call}}},
		{{Role: "assistant", ToolCalls: []ToolCall{call}}, {Role: "user", Content: "intervening"}},
		{{Role: "assistant", ToolCalls: []ToolCall{call}}, {Role: "tool", ToolCallID: "call-1"}, {Role: "tool", ToolCallID: "call-1"}},
		{{Role: "assistant", ToolCalls: []ToolCall{call, call}}},
		{{Role: "user", Content: []ContentItem{{Type: "input_audio"}}}},
		{{Role: "user", Content: []interface{}{42}}},
		{{Role: "user", Content: map[string]interface{}{"text": "silently lost"}}},
		{{Role: "assistant", Images: []string{"iVBORw0KGgo="}}},
	} {
		if _, err := chatGPTInput(history); err == nil {
			t.Errorf("accepted invalid history: %+v", history)
		}
	}
	input, err := chatGPTInput([]Message{{Role: "user", Content: []ContentItem{{Type: "text", Text: "Look"}, {Type: "image_url", ImageURL: &ImageInfo{URL: "https://example.com/a.png"}}}, Images: []string{"iVBORw0KGgo="}}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(input)
	if !strings.Contains(string(encoded), `"type":"input_image"`) || !strings.Contains(string(encoded), "data:image/png;base64,iVBORw0KGgo=") {
		t.Fatalf("images missing: %s", encoded)
	}
	input, err = chatGPTInput([]Message{{Role: "assistant", ToolCalls: []ToolCall{call}}, {Role: "tool", ToolCallID: "call-1", Content: []ContentItem{{Type: "image_url", ImageURL: &ImageInfo{URL: "https://example.com/screenshot.png"}}}}})
	if err != nil || len(input) != 2 {
		t.Fatalf("tool image rejected: %v, %v", input, err)
	}
	if _, _, err := chatGPTTools([]map[string]interface{}{{"type": "tool_search"}}); err == nil {
		t.Fatal("accepted unsupported hosted tool")
	}
	for _, namespace := range []string{"", "myaaw"} {
		name, err := chatGPTLocalName(namespace, "myaaw.lookup", map[string]string{"lookup": "lookup"})
		if err != nil || name != "lookup" {
			t.Fatalf("name mapping %q %v", name, err)
		}
	}
	if _, err := chatGPTLocalName("foreign", "lookup", map[string]string{"lookup": "lookup"}); err == nil {
		t.Fatal("foreign namespace accepted")
	}
}

func TestOpenAIAPIKeyWireUnchanged(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer sk-legacy-key" {
			t.Errorf("API-key auth changed: %v", r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/models" {
			fmt.Fprint(w, `{"object":"list","data":[{"id":"legacy-model"}],"models":[{"slug":"wrong-oauth-catalog","visibility":"list"}]}`)
			return
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("API-key endpoint changed: %s", r.URL.Path)
		}
		var request map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		if request["model"] != "legacy-model" || request["messages"] == nil {
			t.Errorf("chat-completion wire changed: %v", request)
		}
		for _, key := range []string{"input", "store", "include", "previous_response_id"} {
			if _, ok := request[key]; ok {
				t.Errorf("OAuth field leaked: %s", key)
			}
		}
		tools := request["tools"].([]interface{})
		if tools[0].(map[string]interface{})["type"] != "function" {
			t.Error("legacy tools were namespaced")
		}
		if request["stream"] == true {
			if request["stream_options"].(map[string]interface{})["include_usage"] != true {
				t.Error("legacy include_usage changed")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"legacy stream\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		} else {
			fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"legacy reply"},"finish_reason":"stop"}]}`)
		}
	}))
	defer server.Close()
	p, err := CreateLLMProviderFromConfig(config.ProviderConfig{Type: "openai", BaseURL: server.URL, APIKey: "sk-legacy-key", DefaultModel: "legacy-model"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p.(*OpenAIProvider); !ok {
		t.Fatalf("wrong provider %T", p)
	}
	tools := []map[string]interface{}{{"type": "function", "function": map[string]interface{}{"name": "lookup", "parameters": map[string]interface{}{"type": "object"}}}}
	message, err := p.Chat("", []Message{{Role: "system", Content: "system stays system"}, {Role: "user", Content: "Hi"}}, tools)
	if err != nil || MessageText(message) != "legacy reply" {
		t.Fatalf("legacy Chat: %+v %v", message, err)
	}
	err = p.ChatStreamEvents(context.Background(), "", []Message{{Role: "user", Content: "Hi"}}, tools, func(ev StreamEvent) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	models, err := ModelsForConfig(config.ProviderConfig{Type: "openai", BaseURL: server.URL, APIKey: "sk-legacy-key"})
	if err != nil || !reflect.DeepEqual(models, []string{"legacy-model"}) {
		t.Fatalf("legacy model response changed: %v %v", models, err)
	}
	if !reflect.DeepEqual(paths, []string{"/v1/chat/completions", "/v1/chat/completions", "/v1/models"}) {
		t.Fatalf("wrong paths %v", paths)
	}
}
