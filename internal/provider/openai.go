package provider

import (
	"context"
	"fmt"

	"github.com/go-resty/resty/v2"
)

type OpenAIMessage struct {
	Role             string      `json:"role"`
	Name             string      `json:"name,omitempty"`
	Content          interface{} `json:"content,omitempty"`
	ReasoningContent string      `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID       string      `json:"tool_call_id,omitempty"`
}

type OpenAIChoice struct {
	Index        int           `json:"index"`
	Message      OpenAIMessage `json:"message"`
	Delta        OpenAIMessage `json:"delta,omitempty"`
	Logprobs     *string       `json:"logprobs,omitempty"`
	FinishReason string        `json:"finish_reason"`
}

type OpenAICompletionTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

type OpenAIPromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

type OpenAIUsage struct {
	PromptTokens            int                           `json:"prompt_tokens"`
	CompletionTokens        int                           `json:"completion_tokens"`
	TotalTokens             int                           `json:"total_tokens"`
	CompletionTokensDetails OpenAICompletionTokensDetails `json:"completion_tokens_details"`
	PromptTokensDetails     *OpenAIPromptTokensDetails    `json:"prompt_tokens_details,omitempty"`
	// PromptCacheHitTokens is DeepSeek's spelling of the cache-hit count.
	PromptCacheHitTokens int `json:"prompt_cache_hit_tokens,omitempty"`
}

// cachedTokens returns the reported prompt-cache hit count, if any.
func (u OpenAIUsage) cachedTokens() int {
	if u.PromptTokensDetails != nil && u.PromptTokensDetails.CachedTokens > 0 {
		return u.PromptTokensDetails.CachedTokens
	}
	return u.PromptCacheHitTokens
}

type OpenAIGroqUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type OpenAIGroqMeta struct {
	Usage *OpenAIGroqUsage `json:"usage"`
}

type OpenAIChatCompletion struct {
	ID                string          `json:"id"`
	Object            string          `json:"object"`
	Created           int64           `json:"created"`
	Model             string          `json:"model"`
	SystemFingerprint string          `json:"system_fingerprint"`
	Choices           []OpenAIChoice  `json:"choices"`
	Usage             OpenAIUsage     `json:"usage"`
	XGroq             *OpenAIGroqMeta `json:"x_groq,omitempty"`
}

type OpenAIRequest struct {
	Model         string                   `json:"model"`
	Messages      []OpenAIMessage          `json:"messages"`
	Stream        bool                     `json:"stream"`
	Tools         []map[string]interface{} `json:"tools,omitempty"`
	ToolChoice    string                   `json:"tool_choice,omitempty"`
	StreamOptions *OpenAIStreamOptions     `json:"stream_options,omitempty"`
}

type OpenAIStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

func messagesToOpenAIMessages(messages []Message) []OpenAIMessage {
	openAIMessages := make([]OpenAIMessage, 0, len(messages))
	for _, m := range messages {
		openAIMessages = append(openAIMessages, OpenAIMessage{
			Role:             m.Role,
			Name:             m.Name,
			Content:          m.Content,
			ReasoningContent: m.Thought,
			ToolCalls:        m.ToolCalls,
			ToolCallID:       m.ToolCallID,
		})
	}
	return openAIMessages
}

func openAIMessageToMessage(o OpenAIMessage) Message {
	return Message{
		Role:       o.Role,
		Name:       o.Name,
		Content:    o.Content,
		Thought:    o.ReasoningContent,
		ToolCalls:  o.ToolCalls,
		ToolCallID: o.ToolCallID,
	}
}

type OpenAIModels struct {
	Object string        `json:"object"`
	Data   []OpenAIModel `json:"data"`
}

type OpenAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type OpenAIProvider struct {
	baseURL      string
	apiKey       string
	defaultModel string
}

func NewOpenAIProvider(baseURL string, apiKey string, defaultModel string) LLMProvider {
	return &OpenAIProvider{
		baseURL:      baseURL,
		apiKey:       apiKey,
		defaultModel: defaultModel,
	}
}

func (o *OpenAIProvider) ProviderName() string {
	return "openai"
}

func (o *OpenAIProvider) DefaultModel(modelName string) string {
	if modelName == "" {
		return o.defaultModel
	}
	return modelName
}

func (o *OpenAIProvider) config() openAICompatConfig {
	return openAICompatConfig{
		baseURL:      o.baseURL,
		apiKey:       o.apiKey,
		defaultModel: o.defaultModel,
		includeUsage: true,
	}
}

func (o *OpenAIProvider) Chat(modelName string, messages []Message, toolsList []map[string]interface{}) (Message, error) {
	return chatOpenAICompatible(context.Background(), o.config(), modelName, messages, toolsList)
}

func (o *OpenAIProvider) ChatStream(modelName string, messages []Message, callback func(Message) error, toolsList []map[string]interface{}) error {
	return chatStreamFromEvents(context.Background(), o, modelName, messages, toolsList, callback)
}

func (o *OpenAIProvider) ChatStreamEvents(
	ctx context.Context,
	modelName string,
	messages []Message,
	toolsList []map[string]interface{},
	onEvent func(StreamEvent) error,
) error {
	return streamOpenAICompatible(ctx, o.config(), modelName, messages, toolsList, onEvent)
}

func (o *OpenAIProvider) Models() ([]string, error) {
	response, err := o.openAIModels()
	if err != nil {
		return nil, err
	}

	models := make([]string, 0, len(response.Data))
	for _, model := range response.Data {
		models = append(models, model.ID)
	}

	return models, nil
}

func (o *OpenAIProvider) openAIModels() (*OpenAIModels, error) {
	client := resty.New().SetTimeout(modelListTimeout)

	var response OpenAIModels
	res, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", o.apiKey)).
		SetResult(&response).
		Get(o.baseURL + "/v1/models")
	if err != nil {
		return nil, fmt.Errorf("error fetching openai models: %w", err)
	}
	if res == nil {
		return nil, fmt.Errorf("error fetching openai models: nil response")
	}

	if res.StatusCode() != 200 {
		return nil, fmt.Errorf("error fetching openai models: %s", res.String())
	}

	return &response, nil
}
