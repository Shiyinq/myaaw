package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"myaaw/internal/config"
)

// StopReason describes why an assistant turn ended.
type StopReason string

const (
	StopNormal        StopReason = "normal"
	StopMaxIterations StopReason = "max_iterations"
	StopLength        StopReason = "length"
	StopError         StopReason = "error"
	StopAborted       StopReason = "aborted"
)

type Message struct {
	Role       string      `json:"role" bson:"role"`
	Name       string      `json:"name,omitempty" bson:"name,omitempty"`
	Content    interface{} `json:"content,omitempty" bson:"content,omitempty"`
	Images     []string    `json:"images,omitempty" bson:"images,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty" bson:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty" bson:"tool_call_id,omitempty"`
	// ReAct Trace Fields (UI metadata; not sent to providers)
	Thought     string `json:"thought,omitempty" bson:"thought,omitempty"`
	Action      string `json:"action,omitempty" bson:"action,omitempty"`
	ActionInput string `json:"action_input,omitempty" bson:"action_input,omitempty"`
	Observation string `json:"observation,omitempty" bson:"observation,omitempty"`
	// Trace stores all ReAct steps as JSON array for final response
	Trace []ReactStep `json:"trace,omitempty" bson:"trace,omitempty"`
	// Token Usage
	Usage Usage `json:"usage,omitempty" bson:"usage,omitempty"`
	// Transcript metadata (agent loop v2)
	IsError      bool       `json:"is_error,omitempty" bson:"is_error,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty" bson:"error_message,omitempty"`
	DurationMs   int64      `json:"duration_ms,omitempty" bson:"duration_ms,omitempty"`
	StopReason   StopReason `json:"stop_reason,omitempty" bson:"stop_reason,omitempty"`
	Version      int        `json:"v,omitempty" bson:"v,omitempty"`
}

type UsageDetail struct {
	Modality   string `json:"modality,omitempty" bson:"modality,omitempty"`
	TokenCount int    `json:"token_count,omitempty" bson:"token_count,omitempty"`
}

type Usage struct {
	PromptTokens     int           `json:"prompt_tokens,omitempty" bson:"prompt_tokens,omitempty"`
	CompletionTokens int           `json:"completion_tokens,omitempty" bson:"completion_tokens,omitempty"`
	TotalTokens      int           `json:"total_tokens,omitempty" bson:"total_tokens,omitempty"`
	ThoughtsTokens   int           `json:"thoughts_tokens,omitempty" bson:"thoughts_tokens,omitempty"`
	Details          []UsageDetail `json:"details,omitempty" bson:"details,omitempty"`
}

// ReactStep represents a single step in the ReAct loop
type ReactStep struct {
	Thought          string `json:"thought,omitempty" bson:"thought,omitempty"`
	Action           string `json:"action,omitempty" bson:"action,omitempty"`
	ActionInput      string `json:"action_input,omitempty" bson:"action_input,omitempty"`
	Observation      string `json:"observation,omitempty" bson:"observation,omitempty"`
	ThoughtSignature string `json:"thought_signature,omitempty" bson:"thought_signature,omitempty"`
	StepGroup        int    `json:"step_group,omitempty" bson:"step_group,omitempty"`
}

type ToolCall struct {
	Index    *int         `json:"index,omitempty"`
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name             string      `json:"name"`
	Arguments        interface{} `json:"arguments"`
	ThoughtSignature string      `json:"thought_signature,omitempty"`
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
	Chat(modelName string, messages []Message, tools []map[string]interface{}) (Message, error)
	// ChatStream is the legacy streaming API. New code should prefer
	// ChatStreamEvents, which exposes deltas and an explicit stop reason.
	ChatStream(modelName string, messages []Message, callback func(Message) error, tools []map[string]interface{}) error
	ChatStreamEvents(ctx context.Context, modelName string, messages []Message, tools []map[string]interface{}, onEvent func(StreamEvent) error) error
	Models() ([]string, error)
	DefaultModel(modelName string) string
}

type Transcriber interface {
	Transcribe(audioFile []byte) (string, error)
}

type Synthesizer interface {
	Synthesize(ctx context.Context, text string) ([]byte, string, error)
}

type factoryLLM func(baseURL string, apiKey string, defaultModel string) LLMProvider
type factoryTranscriber func(apiKey string, defaultModel string) Transcriber
type factorySynthesizer func(apiKey string, defaultModel string) (Synthesizer, error)

var LLMproviderFactories = map[string]factoryLLM{
	"ollama": NewOllamaProvider,
	"openai": NewOpenAIProvider,
	"gemini": NewGeminiProvider,
	"groq":   NewGroqProvider,
}

var defaultLLMModels = map[string]string{
	"ollama": "qwen2.5:1.5b-instruct",
	"openai": "gpt-4o",
	"gemini": "models/gemini-3-flash-preview",
	"groq":   "llama-3.2-1b-preview",
}

// DefaultBaseURL returns the public endpoint used when a provider config omits
// base_url. Providers build request URLs by appending their own path, so the
// value must not include a trailing slash.
func DefaultBaseURL(providerName string) string {
	switch providerName {
	case "openai":
		return "https://api.openai.com"
	case "gemini":
		return "https://generativelanguage.googleapis.com"
	case "groq":
		return "https://api.groq.com/openai"
	case "ollama":
		return "http://localhost:11434"
	default:
		return ""
	}
}

var TranscriberFactories = map[string]factoryTranscriber{
	"groq":   NewGroqTranscriber,
	"gemini": NewGeminiTranscriber,
}

var defaultTranscriberModels = map[string]string{
	"groq":   "whisper-large-v3-turbo",
	"gemini": "models/gemini-2.5-flash",
}

var SynthesizerFactories = map[string]factorySynthesizer{
	"gemini": NewGeminiSynthesizer,
}

var defaultSynthesizerModels = map[string]string{
	"gemini": "gemini-2.5-flash-preview-tts",
}

func CreateLLMProvider(providerName string, apiKey string) (LLMProvider, error) {
	factory, exists := LLMproviderFactories[providerName]
	if !exists {
		return nil, errors.New("unknown llm provider")
	}
	defaultModel := config.LLMDefaultModel
	if defaultModel == "" {
		defaultModel = defaultLLMModels[providerName]
	}

	return factory(config.LLMProviderBaseURL, apiKey, defaultModel), nil
}

func CreateTranscriber(providerName string, apiKey string) (Transcriber, error) {
	factory, exists := TranscriberFactories[providerName]
	if !exists {
		return nil, errors.New("unknown transcriber provider")
	}
	defaultModel := defaultTranscriberModels[providerName]

	return factory(apiKey, defaultModel), nil
}

func CreateSynthesizer(providerName string, apiKey string) (Synthesizer, error) {
	factory, exists := SynthesizerFactories[providerName]
	if !exists {
		return nil, errors.New("unknown synthesizer provider")
	}
	defaultModel := defaultSynthesizerModels[providerName]
	return factory(apiKey, defaultModel)
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
