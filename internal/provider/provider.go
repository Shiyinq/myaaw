package provider

import (
	"errors"
	"teo/internal/config"
	"teo/internal/tools"
)

type Message struct {
	Role       string      `json:"role" bson:"role"`
	Name       string      `json:"name,omitempty" bson:"name,omitempty"`
	Content    interface{} `json:"content,omitempty" bson:"content,omitempty"`
	Images     []string    `json:"images,omitempty" bson:"images,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty" bson:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty" bson:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type,omitempty"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ContentItem struct {
	Type     string     `json:"type"`
	Text     string     `json:"text,omitempty"`
	ImageURL *ImageInfo `json:"image_url,omitempty"`
}

type ImageInfo struct {
	URL string `json:"url,omitempty"`
}

type LLMProvider interface {
	ProviderName() string
	Chat(modelName string, messages []Message) (Message, error)
	ChatStream(modelName string, messages []Message, callback func(Message) error) error
	Models() ([]string, error)
	DefaultModel(modelName string) string
}

type Factory func(baseURL string, apiKey string, defaultModel string) LLMProvider

var ProviderFactories = map[string]Factory{
	"ollama":  NewOllamaProvider,
	"openai":  NewOpenAIProvider,
	"gemini":  NewGeminiProvider,
	"groq":    NewGroqProvider,
	"mistral": NewMistralProvider,
}

var defaultModels = map[string]string{
	"ollama":  "qwen2.5:1.5b-instruct",
	"openai":  "gpt-4o",
	"gemini":  "models/gemini-1.5-flash",
	"groq":    "llama-3.2-1b-preview",
	"mistral": "ministral-3b-latest",
}

func CreateProvider(providerName string, apiKey string) (LLMProvider, error) {
	factory, exists := ProviderFactories[providerName]
	if !exists {
		return nil, errors.New("unknown llm provider")
	}
	defaultModel := defaultModels[providerName]

	return factory(config.LLMProviderBaseURL, apiKey, defaultModel), nil
}

func toolCalls(messages []Message, response Message) []Message {
	messages = append(messages, response)
	for _, toolCall := range response.ToolCalls {
		toolId := toolCall.ID
		toolName := toolCall.Function.Name
		toolArgs := toolCall.Function.Arguments

		tool := tools.NewTools(toolName, toolArgs)
		responseTool := []Message{
			{
				Role:       "tool",
				Name:       toolName,
				Content:    tool,
				ToolCallID: toolId,
			},
		}
		messages = append(messages, responseTool...)
	}

	return messages
}
