package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"myaaw/internal/auth/chatgpt"
	"myaaw/internal/config"
	"myaaw/internal/thinking"
)

const chatGPTBaseURL = "https://api.openai.com"
const chatGPTNamespace = "myaaw"

// ChatGPTProvider is the public Responses API's ChatGPT plan billing path.
// It is intentionally not an OpenAI-compatible API-key provider.
type ChatGPTProvider struct {
	credentialID string
	defaultModel string
	tokenSource  func(context.Context, string) (string, error)
	// Endpoint overrides are package-private and used only by local tests.
	baseURL string
	client  *http.Client
}

func NewChatGPTProvider(cfg config.ProviderConfig) (*ChatGPTProvider, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL != "" && baseURL != chatGPTBaseURL {
		return nil, errors.New("ChatGPT Subscription only supports https://api.openai.com; custom base URLs cannot receive OAuth tokens")
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.AuthMode))
	if mode != "" && mode != "oauth" {
		return nil, errors.New("ChatGPT Subscription requires auth_mode oauth, not an API key")
	}
	if strings.TrimSpace(cfg.APIKey) != "" {
		return nil, errors.New("ChatGPT Subscription uses a signed-in credential, not api_key; remove api_key and select credential_id")
	}
	if strings.TrimSpace(cfg.CredentialID) == "" {
		return nil, errors.New("sign in to ChatGPT Subscription and select a credential_id before using this provider")
	}
	// Initialize once, lazily: constructing a provider does not touch credential
	// storage, and every request still reloads/refreshes its selected credential.
	var once sync.Once
	var manager *chatgpt.Manager
	var managerErr error
	return &ChatGPTProvider{
		credentialID: strings.TrimSpace(cfg.CredentialID),
		defaultModel: strings.TrimSpace(cfg.DefaultModel),
		baseURL:      chatGPTBaseURL,
		client:       &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		tokenSource: func(ctx context.Context, id string) (string, error) {
			once.Do(func() { manager, managerErr = chatgpt.DefaultManager() })
			if managerErr != nil {
				return "", managerErr
			}
			return manager.AccessToken(ctx, id)
		},
	}, nil
}

func (p *ChatGPTProvider) ProviderName() string { return chatgpt.ProviderType }
func (p *ChatGPTProvider) DisplayName() string  { return chatgpt.Label }
func (p *ChatGPTProvider) DefaultModel(model string) string {
	if model == "" {
		return p.defaultModel
	}
	return model
}

func (p *ChatGPTProvider) Chat(model string, messages []Message, tools []map[string]interface{}) (Message, error) {
	var final Message
	err := p.ChatStreamEvents(context.Background(), model, messages, tools, func(ev StreamEvent) error {
		if ev.Type == StreamDone && ev.Message != nil {
			final = *ev.Message
		}
		return nil
	})
	return final, err
}

func (p *ChatGPTProvider) ChatStream(model string, messages []Message, callback func(Message) error, tools []map[string]interface{}) error {
	return chatStreamFromEvents(context.Background(), p, model, messages, tools, callback)
}

// ChatGPTModel preserves account-specific picker labels and server ordering.
type ChatGPTModel struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
	Visibility  string `json:"visibility"`
}

func (p *ChatGPTProvider) ModelCatalog(ctx context.Context) ([]ChatGPTModel, error) {
	ctx, cancel := context.WithTimeout(ctx, modelListTimeout)
	defer cancel()
	res, err := p.request(ctx, http.MethodGet, "/v1/models", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, chatGPTHTTPError(res)
	}
	var body struct {
		Models []ChatGPTModel `json:"models"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("invalid ChatGPT model catalog: %w", err)
	}
	models := make([]ChatGPTModel, 0, len(body.Models))
	for _, m := range body.Models {
		if m.Visibility == "list" && strings.TrimSpace(m.Slug) != "" {
			models = append(models, m)
		}
	}
	return models, nil
}

func (p *ChatGPTProvider) Models() ([]string, error) {
	catalog, err := p.ModelCatalog(context.Background())
	if err != nil {
		return nil, err
	}
	models := make([]string, 0, len(catalog))
	for _, m := range catalog {
		models = append(models, m.Slug)
	}
	return models, nil
}

func (p *ChatGPTProvider) request(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	token, err := p.tokenSource(ctx, p.credentialID)
	if err != nil {
		return nil, fmt.Errorf("ChatGPT Subscription credential unavailable; check the selected account and sign in again if needed: %w", err)
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("ChatGPT Subscription credential returned an empty access token; sign in again")
	}
	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("cannot construct ChatGPT Subscription request")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	if method == http.MethodPost {
		req.Header.Set("Accept", "text/event-stream")
	}
	// Also enforce the redirect policy on injected clients.
	client := *p.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("ChatGPT Subscription network request failed")
	}
	return res, nil
}

// ChatGPTAPIError retains structured diagnostics without switching billing paths.
type ChatGPTAPIError struct {
	Status    int
	Code      string
	Param     string
	RequestID string
	Body      string
	Message   string
}

func (e *ChatGPTAPIError) Error() string {
	message := e.Message
	switch e.Code {
	case "subscription_sharing_usage_limit_exceeded":
		message = "ChatGPT plan usage limit reached for this app or account. Pause requests and check https://chatgpt.com/settings/usage; no API-key billing fallback was used."
	case "subscription_sharing_usage_unavailable":
		message = "ChatGPT plan usage availability could not be checked. Keep your credential and try again later."
	case "subscription_sharing_user_not_eligible":
		message = "ChatGPT plan usage is unavailable for this account, workspace, or policy. Select an eligible account or another provider."
	case "subscription_sharing_unsupported_capability":
		message = "ChatGPT plan usage does not support the requested capability; check the model, input, and tools."
	}
	if message == "" {
		message = "request rejected"
	}
	return fmt.Sprintf("ChatGPT Subscription: %s (HTTP %d, code %q, param %q, request ID %q)", message, e.Status, e.Code, e.Param, e.RequestID)
}

func chatGPTHTTPError(res *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	var parsed struct {
		Error struct {
			Code    string `json:"code"`
			Param   string `json:"param"`
			Message string `json:"message"`
		} `json:"error"`
		Detail string `json:"detail"`
	}
	_ = json.Unmarshal(body, &parsed)
	message := parsed.Error.Message
	if message == "" {
		message = parsed.Detail
	}
	return &ChatGPTAPIError{Status: res.StatusCode, Code: parsed.Error.Code, Param: parsed.Error.Param,
		RequestID: res.Header.Get("x-request-id"), Body: string(body), Message: message}
}

type chatGPTRequest struct {
	Model     string                   `json:"model"`
	Input     []interface{}            `json:"input"`
	Store     bool                     `json:"store"`
	Stream    bool                     `json:"stream"`
	Include   []string                 `json:"include"`
	Tools     []map[string]interface{} `json:"tools,omitempty"`
	Reasoning *chatGPTReasoning        `json:"reasoning,omitempty"`
}

type chatGPTReasoning struct {
	Summary string `json:"summary,omitempty"`
	Effort  string `json:"effort,omitempty"`
}

var chatGPTReasoningFamily = regexp.MustCompile(`^gpt-[56]([.-]|$)`)

// legacyChatGPTThinking is a compatibility predicate for model IDs the
// capability catalog does not describe yet. Models present in the catalog never
// consult it; add new models there instead.
func legacyChatGPTThinking(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	// Older/non-reasoning models reject this parameter.
	if strings.Contains(model, "-chat") {
		return false
	}
	return chatGPTReasoningFamily.MatchString(model) ||
		model == "o3" || (strings.HasPrefix(model, "o3-") && !strings.HasPrefix(model, "o3-mini")) ||
		model == "o4-mini" || strings.HasPrefix(model, "o4-mini-")
}

// chatGPTReasoningFor resolves the reasoning object for one request.
// It returns nil when the model should receive no reasoning parameters at all.
func chatGPTReasoningFor(ctx context.Context, model string) *chatGPTReasoning {
	spec, ok := thinking.Resolve(model, thinking.Level(ctx), legacyChatGPTThinking(model))
	if !ok {
		return nil
	}
	reasoning := &chatGPTReasoning{}
	if spec.Enabled {
		reasoning.Summary = "auto"
		reasoning.Effort = spec.Level
	} else {
		reasoning.Effort = "none"
	}
	return reasoning
}

var chatGPTToolName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func chatGPTTools(tools []map[string]interface{}) ([]map[string]interface{}, map[string]string, error) {
	functions := make([]map[string]interface{}, 0, len(tools))
	names := make(map[string]string, len(tools))
	for _, tool := range tools {
		if tool["type"] != "function" {
			return nil, nil, errors.New("ChatGPT Subscription supports only local function tools in the myaaw namespace")
		}
		fn, ok := tool["function"].(map[string]interface{})
		if !ok {
			fn = tool
		}
		name, _ := fn["name"].(string)
		if !chatGPTToolName.MatchString(name) {
			return nil, nil, fmt.Errorf("unsupported ChatGPT function name %q", name)
		}
		if _, exists := names[name]; exists {
			return nil, nil, fmt.Errorf("duplicate ChatGPT function name %q", name)
		}
		names[name] = name
		function := map[string]interface{}{"type": "function", "name": name, "strict": false}
		for _, key := range []string{"description", "parameters", "strict"} {
			if value, exists := fn[key]; exists {
				function[key] = value
			}
		}
		if function["parameters"] == nil {
			function["parameters"] = map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
		}
		functions = append(functions, function)
	}
	if len(functions) == 0 {
		return nil, names, nil
	}
	return []map[string]interface{}{{"type": "namespace", "name": chatGPTNamespace,
		"description": "Myaaw local tools", "tools": functions}}, names, nil
}

func chatGPTLocalName(namespace, name string, names map[string]string) (string, error) {
	if namespace != "" && namespace != chatGPTNamespace {
		return "", fmt.Errorf("unknown ChatGPT tool namespace %q", namespace)
	}
	// Some clients serialize a qualified name rather than the namespace field.
	name = strings.TrimPrefix(name, chatGPTNamespace+".")
	if local, ok := names[name]; ok {
		return local, nil
	}
	return "", fmt.Errorf("ChatGPT returned an unknown local tool %q", name)
}

func chatGPTInput(messages []Message) ([]interface{}, error) {
	input := make([]interface{}, 0, len(messages))
	pending := map[string]bool{}
	seen := map[string]bool{}
	for _, m := range messages {
		if m.Role == "tool" {
			if m.ToolCallID == "" || !pending[m.ToolCallID] {
				return nil, fmt.Errorf("ChatGPT tool result %q has no pending call (or is duplicated)", m.ToolCallID)
			}
			content, err := chatGPTContent(m, "input_text")
			if err != nil {
				return nil, err
			}
			var output interface{} = content
			if len(content) == 0 {
				output = ""
			}
			if len(content) == 1 && content[0]["type"] == "input_text" {
				output = content[0]["text"]
			}
			input = append(input, map[string]interface{}{"type": "function_call_output", "call_id": m.ToolCallID, "output": output})
			delete(pending, m.ToolCallID)
			continue
		}
		if len(pending) > 0 {
			return nil, errors.New("ChatGPT history contains a tool call without its result before the next message")
		}
		role := m.Role
		if role == "system" {
			role = "developer"
		}
		if role != "developer" && role != "user" && role != "assistant" {
			return nil, fmt.Errorf("unsupported ChatGPT message role %q", m.Role)
		}
		if role != "assistant" && len(m.ToolCalls) != 0 {
			return nil, errors.New("ChatGPT tool calls must belong to assistant messages")
		}
		for _, call := range m.ToolCalls {
			if call.ID == "" || seen[call.ID] {
				return nil, errors.New("ChatGPT history requires unique, nonempty tool call IDs")
			}
			if !chatGPTToolName.MatchString(call.Function.Name) {
				return nil, fmt.Errorf("invalid ChatGPT history tool name %q", call.Function.Name)
			}
			if !json.Valid([]byte(ToolCallArgumentsString(call.Function.Arguments))) {
				return nil, errors.New("ChatGPT history contains invalid tool argument JSON")
			}
			pending[call.ID], seen[call.ID] = true, true
		}
		textType := "input_text"
		if role == "assistant" {
			textType = "output_text"
		}
		content, err := chatGPTContent(m, textType)
		if err != nil {
			return nil, err
		}
		if raw := m.ProviderMetadata[chatgpt.ProviderType]; len(raw) > 0 && role == "assistant" {
			var saved chatGPTContinuation
			if err := json.Unmarshal(raw, &saved); err != nil {
				return nil, errors.New("invalid ChatGPT continuation metadata")
			}
			projected, err := chatGPTMessage(saved.Output, nil, false)
			if err != nil {
				return nil, err
			}
			if MessageText(projected) != MessageText(m) || !chatGPTSameCalls(projected.ToolCalls, m.ToolCalls) {
				return nil, errors.New("ChatGPT continuation metadata no longer matches the assistant turn; preserve the original message and provider_metadata together")
			}
			for _, item := range saved.Output {
				input = append(input, item)
			}
			continue
		}
		if len(content) > 0 {
			input = append(input, map[string]interface{}{"role": role, "content": content})
		}
		for _, call := range m.ToolCalls {
			input = append(input, chatGPTOutputItem{Type: "function_call", CallID: call.ID, Name: call.Function.Name,
				Namespace: chatGPTNamespace, Arguments: ToolCallArgumentsString(call.Function.Arguments)})
		}
	}
	if len(pending) > 0 {
		return nil, errors.New("ChatGPT history ends with tool calls missing their results")
	}
	return input, nil
}

func chatGPTSameCalls(a, b []ToolCall) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Function.Name != b[i].Function.Name ||
			ToolCallArgumentsString(a[i].Function.Arguments) != ToolCallArgumentsString(b[i].Function.Arguments) {
			return false
		}
	}
	return true
}

func chatGPTContent(m Message, textType string) ([]map[string]interface{}, error) {
	switch m.Content.(type) {
	case nil, string, []ContentItem, []interface{}:
	default:
		return nil, fmt.Errorf("unsupported ChatGPT content shape %T", m.Content)
	}
	// Validate decoded blocks before the common normalizer can skip malformed ones.
	if raw, ok := m.Content.([]interface{}); ok {
		for _, block := range raw {
			if _, ok := block.(map[string]interface{}); !ok {
				return nil, errors.New("unsupported ChatGPT content block")
			}
		}
	}
	content := make([]map[string]interface{}, 0)
	for _, item := range MessageContentItems(m) {
		switch item.Type {
		case "text":
			content = append(content, map[string]interface{}{"type": textType, "text": item.Text})
		case "image_url":
			if item.ImageURL == nil {
				return nil, errors.New("ChatGPT image is missing its URL")
			}
			image, err := chatGPTImage(item.ImageURL.URL, false)
			if err != nil {
				return nil, err
			}
			content = append(content, image)
		default:
			return nil, fmt.Errorf("ChatGPT Subscription does not support content type %q (audio/video inputs are unsupported)", item.Type)
		}
	}
	for _, image := range m.Images {
		block, err := chatGPTImage(image, true)
		if err != nil {
			return nil, err
		}
		content = append(content, block)
	}
	if len(content) > 0 && m.Role != "user" && m.Role != "tool" {
		for _, block := range content {
			if block["type"] == "input_image" {
				return nil, errors.New("ChatGPT image inputs must belong to a user message or tool result")
			}
		}
	}
	return content, nil
}

func chatGPTImage(image string, allowBase64 bool) (map[string]interface{}, error) {
	if allowBase64 && !strings.HasPrefix(image, "data:") && !strings.HasPrefix(image, "https://") && !strings.HasPrefix(image, "http://") {
		mime := ""
		switch {
		case strings.HasPrefix(image, "iVBOR"):
			mime = "image/png"
		case strings.HasPrefix(image, "/9j/"):
			mime = "image/jpeg"
		case strings.HasPrefix(image, "UklGR"):
			mime = "image/webp"
		case strings.HasPrefix(image, "R0lGOD"):
			mime = "image/gif"
		}
		if mime == "" {
			return nil, errors.New("unsupported ChatGPT image encoding; use PNG, JPEG, WebP, or GIF")
		}
		image = "data:" + mime + ";base64," + image
	}
	if strings.HasPrefix(image, "data:") {
		header, data, ok := strings.Cut(image, ",")
		valid := header == "data:image/png;base64" || header == "data:image/jpeg;base64" || header == "data:image/webp;base64" || header == "data:image/gif;base64"
		if !ok || !valid || data == "" {
			return nil, errors.New("unsupported ChatGPT image data URL")
		}
		if _, err := base64.StdEncoding.DecodeString(data); err != nil {
			return nil, errors.New("invalid ChatGPT image base64")
		}
	} else {
		u, err := url.Parse(image)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
			return nil, errors.New("ChatGPT image requires an HTTP(S) URL or supported base64 data URL")
		}
	}
	return map[string]interface{}{"type": "input_image", "image_url": image}, nil
}
