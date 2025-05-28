package provider

import (
	"encoding/json"
	"errors"
	"fmt"
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
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string      `json:"name"`
	Arguments interface{} `json:"arguments"`
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

type TTSProvider interface {
	SpeechToText(audioFile []byte) (string, error)
}

type factoryLLM func(baseURL string, apiKey string, defaultModel string) LLMProvider
type factoryTTS func(apiKey string, defaultModel string) TTSProvider

var LLMproviderFactories = map[string]factoryLLM{
	"ollama":  NewOllamaProvider,
	"openai":  NewOpenAIProvider,
	"gemini":  NewGeminiProvider,
	"groq":    NewGroqProvider,
	"mistral": NewMistralProvider,
}

var defaultLLMModels = map[string]string{
	"ollama":  "qwen2.5:1.5b-instruct",
	"openai":  "gpt-4o",
	"gemini":  "models/gemini-1.5-flash",
	"groq":    "llama-3.2-1b-preview",
	"mistral": "ministral-3b-latest",
}

var TTSproviderFactories = map[string]factoryTTS{
	"groq": NewGroqTTSProvider,
}

var defaultTTSMModels = map[string]string{
	"groq": "whisper-large-v3-turbo",
}

func CreateLLMProvider(providerName string, apiKey string) (LLMProvider, error) {
	factory, exists := LLMproviderFactories[providerName]
	if !exists {
		return nil, errors.New("unknown llm provider")
	}
	defaultModel := defaultLLMModels[providerName]

	return factory(config.LLMProviderBaseURL, apiKey, defaultModel), nil
}

func CreateTTSProvider(providerName string, apiKey string) (TTSProvider, error) {
	factory, exists := TTSproviderFactories[providerName]
	if !exists {
		return nil, errors.New("unknown tts provider")
	}
	defaultModel := defaultTTSMModels[providerName]

	return factory(apiKey, defaultModel), nil
}

func argsToString(i interface{}) string {
	if str, ok := i.(string); ok {
		return str
	}

	jsonData, err := json.Marshal(i)
	if err != nil {
		return fmt.Sprintf("%v", i)
	}

	return string(jsonData)
}

func toolCalls(messages []Message, response Message) []Message {
	messages = append(messages, response)
	for _, toolCall := range response.ToolCalls {
		toolId := toolCall.ID
		toolName := toolCall.Function.Name
		toolArgs := toolCall.Function.Arguments

		tool := tools.NewTools(toolName, argsToString(toolArgs))
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
