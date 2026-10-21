package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

type OllamaProvider struct {
	baseURL      string
	apiKey       string
	defaultModel string
}

type OllamaRequest struct {
	Model    string                   `json:"model"`
	Messages []Message                `json:"messages"`
	Stream   bool                     `json:"stream"`
	Tools    []map[string]interface{} `json:"tools,omitempty"`
}

type OllamaResponse struct {
	Model              string    `json:"model"`
	CreatedAt          time.Time `json:"created_at"`
	Message            Message   `json:"message"`
	DoneReason         string    `json:"done_reason"`
	Done               bool      `json:"done"`
	TotalDuration      int64     `json:"total_duration"`
	LoadDuration       int64     `json:"load_duration"`
	PromptEvalCount    int       `json:"prompt_eval_count"`
	PromptEvalDuration int64     `json:"prompt_eval_duration"`
	EvalCount          int       `json:"eval_count"`
	EvalDuration       int64     `json:"eval_duration"`
}

type OllamaModels struct {
	Name  string `json:"name"`
	Model string `json:"model"`
}

type OllamaTagsResponse struct {
	Models []OllamaModels `json:"models"`
}

func NewOllamaProvider(baseURL string, apiKey string, defaultModel string) LLMProvider {
	return &OllamaProvider{
		baseURL:      baseURL,
		apiKey:       apiKey,
		defaultModel: defaultModel,
	}
}

func (o *OllamaProvider) ProviderName() string {
	return "ollama"
}

func (o *OllamaProvider) DefaultModel(modelName string) string {
	if modelName == "" {
		return o.defaultModel
	}
	return modelName
}

func (o *OllamaProvider) buildRequest(modelName string, messages []Message, stream bool, toolsList []map[string]interface{}) OllamaRequest {
	req := OllamaRequest{
		Model:    o.DefaultModel(modelName),
		Stream:   stream,
		Messages: messages,
	}
	if len(toolsList) > 0 {
		req.Tools = toolsList
	}
	return req
}

func (o *OllamaProvider) Chat(modelName string, messages []Message, toolsList []map[string]interface{}) (Message, error) {
	return o.chat(context.Background(), modelName, messages, toolsList)
}

func (o *OllamaProvider) chat(ctx context.Context, modelName string, messages []Message, toolsList []map[string]interface{}) (Message, error) {
	client := resty.New().SetTimeout(120 * time.Second)
	request := o.buildRequest(modelName, messages, false, toolsList)

	var response OllamaResponse
	res, err := doWithRetry(func() (*resty.Response, error) {
		return client.R().
			SetContext(ctx).
			SetHeader("Content-Type", "application/json").
			SetBody(request).
			SetResult(&response).
			Post(o.baseURL + "/api/chat")
	})
	if err != nil {
		return Message{}, fmt.Errorf("chat request failed: %w", err)
	}
	if res == nil {
		return Message{}, errors.New("chat request returned a nil response")
	}
	if res.StatusCode() != 200 {
		return Message{}, fmt.Errorf("chat request failed (%d): %s", res.StatusCode(), strings.TrimSpace(res.String()))
	}

	msg := response.Message
	msg.Version = 2
	msg.StopReason = mapOllamaDoneReason(response.DoneReason)
	msg.Usage = Usage{
		PromptTokens:     response.PromptEvalCount,
		CompletionTokens: response.EvalCount,
		TotalTokens:      response.PromptEvalCount + response.EvalCount,
	}
	if msg.StopReason == StopError {
		msg.IsError = true
		msg.ErrorMessage = fmt.Sprintf("ollama stopped the response: %s", response.DoneReason)
	}
	return msg, nil
}

func (o *OllamaProvider) ChatStream(modelName string, messages []Message, callback func(Message) error, toolsList []map[string]interface{}) error {
	return chatStreamFromEvents(context.Background(), o, modelName, messages, toolsList, callback)
}

func (o *OllamaProvider) ChatStreamEvents(
	ctx context.Context,
	modelName string,
	messages []Message,
	toolsList []map[string]interface{},
	onEvent func(StreamEvent) error,
) error {
	timeout := streamRequestTimeout()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := resty.New().SetTimeout(timeout)
	request := o.buildRequest(modelName, messages, true, toolsList)

	res, err := doWithRetry(func() (*resty.Response, error) {
		return client.R().
			SetContext(ctx).
			SetHeader("Content-Type", "application/json").
			SetBody(request).
			SetDoNotParseResponse(true).
			Post(o.baseURL + "/api/chat")
	})
	if err != nil {
		return fmt.Errorf("stream request failed: %w", err)
	}
	if res == nil {
		return errors.New("stream request returned a nil response")
	}
	defer res.RawBody().Close()

	if res.StatusCode() != 200 {
		body, _ := io.ReadAll(io.LimitReader(res.RawBody(), 64*1024))
		return fmt.Errorf("stream request failed (%d): %s", res.StatusCode(), strings.TrimSpace(string(body)))
	}

	return o.consumeOllamaStream(res.RawBody(), onEvent)
}

// consumeOllamaStream parses Ollama's NDJSON stream.
func (o *OllamaProvider) consumeOllamaStream(r io.Reader, onEvent func(StreamEvent) error) error {
	decoder := json.NewDecoder(r)

	var (
		text       strings.Builder
		thought    strings.Builder
		toolCalls  []ToolCall
		usage      Usage
		done       bool
		doneReason string
		sawChunk   bool
	)

	for {
		var chunk OllamaResponse
		if err := decoder.Decode(&chunk); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error decoding stream chunk: %w", err)
		}
		sawChunk = true

		if content, ok := chunk.Message.Content.(string); ok && content != "" {
			text.WriteString(content)
			if err := onEvent(StreamEvent{Type: StreamTextDelta, Text: content}); err != nil {
				return err
			}
		}
		if chunk.Message.Thought != "" {
			thought.WriteString(chunk.Message.Thought)
			if err := onEvent(StreamEvent{Type: StreamThinkingDelta, Thought: chunk.Message.Thought}); err != nil {
				return err
			}
		}
		if len(chunk.Message.ToolCalls) > 0 {
			toolCalls = chunk.Message.ToolCalls
		}

		if chunk.PromptEvalCount > 0 || chunk.EvalCount > 0 {
			usage = Usage{
				PromptTokens:     chunk.PromptEvalCount,
				CompletionTokens: chunk.EvalCount,
				TotalTokens:      chunk.PromptEvalCount + chunk.EvalCount,
			}
			if err := onEvent(StreamEvent{Type: StreamUsage, Usage: usage}); err != nil {
				return err
			}
		}

		if chunk.Done {
			done = true
			if chunk.DoneReason != "" {
				doneReason = chunk.DoneReason
			}
			break
		}
	}

	if !sawChunk {
		return errors.New("stream ended without any data")
	}

	final := Message{
		Role:       "assistant",
		Usage:      usage,
		ToolCalls:  toolCalls,
		StopReason: mapOllamaDoneReason(doneReason),
		Version:    2,
	}
	if text.Len() > 0 {
		final.Content = text.String()
	}
	if thought.Len() > 0 {
		final.Thought = thought.String()
	}
	if !done {
		final.StopReason = StopNormal
	}

	var doneErr error
	if final.StopReason == StopError {
		doneErr = fmt.Errorf("ollama stopped the response: %s", doneReason)
		final.ErrorMessage = doneErr.Error()
	}

	return onEvent(StreamEvent{
		Type:       StreamDone,
		Message:    &final,
		Usage:      usage,
		StopReason: final.StopReason,
		Err:        doneErr,
	})
}

func mapOllamaDoneReason(reason string) StopReason {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "", "stop":
		return StopNormal
	case "length":
		return StopLength
	case "error":
		return StopError
	default:
		return StopNormal
	}
}

func (o *OllamaProvider) Models() ([]string, error) {
	tags, err := o.ollamaTags()
	if err != nil {
		return nil, err
	}

	models := make([]string, 0, len(tags.Models))
	for _, model := range tags.Models {
		models = append(models, model.Name)
	}

	return models, nil
}

func (o *OllamaProvider) ollamaTags() (*OllamaTagsResponse, error) {
	client := resty.New().SetTimeout(30 * time.Second)

	var response OllamaTagsResponse
	res, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetResult(&response).
		Get(o.baseURL + "/api/tags")
	if err != nil {
		return nil, fmt.Errorf("error fetching ollama models: %w", err)
	}
	if res == nil {
		return nil, fmt.Errorf("error fetching ollama models: nil response")
	}
	if res.StatusCode() != 200 {
		return nil, fmt.Errorf("error fetching ollama models: %s", res.String())
	}

	return &response, nil
}
