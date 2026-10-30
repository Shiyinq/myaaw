package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

// openAICompatConfig describes an OpenAI-compatible chat completions endpoint.
type openAICompatConfig struct {
	baseURL      string
	apiKey       string
	defaultModel string
	includeUsage bool
}

func (c openAICompatConfig) model(modelName string) string {
	if modelName == "" {
		return c.defaultModel
	}
	return modelName
}

func (c openAICompatConfig) endpoint() string {
	return c.baseURL + "/v1/chat/completions"
}

type openAIToolCallAccumulator struct {
	id   string
	typ  string
	name string
	args strings.Builder
}

// streamOpenAICompatible performs an SSE streaming request against an
// OpenAI-compatible endpoint and forwards provider events to onEvent.
func streamOpenAICompatible(
	ctx context.Context,
	cfg openAICompatConfig,
	modelName string,
	messages []Message,
	tools []map[string]interface{},
	onEvent func(StreamEvent) error,
) error {
	req := OpenAIRequest{
		Model:    cfg.model(modelName),
		Messages: messagesToOpenAIMessages(messages),
		Stream:   true,
	}
	if len(tools) > 0 {
		req.Tools = tools
		req.ToolChoice = "auto"
	}
	if cfg.includeUsage {
		req.StreamOptions = &OpenAIStreamOptions{IncludeUsage: true}
	}

	timeout := streamRequestTimeout()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := resty.New().SetTimeout(timeout)
	// Retry only while nothing reached the caller: a retry after partial output
	// would duplicate text in the transcript.
	return streamWithRetry(ctx,
		func(reqCtx context.Context) (*resty.Response, error) {
			return client.R().
				SetContext(reqCtx).
				SetHeader("Content-Type", "application/json").
				SetHeader("Authorization", "Bearer "+cfg.apiKey).
				SetBody(req).
				SetDoNotParseResponse(true).
				Post(cfg.endpoint())
		},
		consumeOpenAISSE,
		onEvent,
	)
}

// consumeOpenAISSE parses an OpenAI-compatible SSE stream.
//
// It accumulates text, thinking, tool calls, usage, and the finish reason so
// callers receive a single authoritative assistant message on StreamDone.
func consumeOpenAISSE(r io.Reader, onEvent func(StreamEvent) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var (
		text         strings.Builder
		thought      strings.Builder
		accumulators = map[int]*openAIToolCallAccumulator{}
		usage        Usage
		finishReason string
		sawData      bool
	)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}

		var chunk OpenAIChatCompletion
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			// Some proxies emit keep-alive or malformed frames; ignore those
			// rather than aborting an otherwise healthy stream.
			continue
		}
		sawData = true

		if chunk.Usage.TotalTokens > 0 {
			usage = Usage{
				PromptTokens:      chunk.Usage.PromptTokens,
				CompletionTokens:  chunk.Usage.CompletionTokens,
				TotalTokens:       chunk.Usage.TotalTokens,
				ThoughtsTokens:    chunk.Usage.CompletionTokensDetails.ReasoningTokens,
				CachedInputTokens: chunk.Usage.cachedTokens(),
			}
			if err := onEvent(StreamEvent{Type: StreamUsage, Usage: usage}); err != nil {
				return err
			}
		} else if chunk.XGroq != nil && chunk.XGroq.Usage != nil && chunk.XGroq.Usage.TotalTokens > 0 {
			usage = Usage{
				PromptTokens:     chunk.XGroq.Usage.PromptTokens,
				CompletionTokens: chunk.XGroq.Usage.CompletionTokens,
				TotalTokens:      chunk.XGroq.Usage.TotalTokens,
			}
			if err := onEvent(StreamEvent{Type: StreamUsage, Usage: usage}); err != nil {
				return err
			}
		}

		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		if choice.FinishReason != "" {
			finishReason = choice.FinishReason
		}

		delta := choice.Delta
		// A few compatible servers put the final payload in `message`
		// instead of `delta`. Only use it when nothing has been accumulated yet,
		// otherwise a final message would duplicate streamed deltas.
		if delta.Content == nil && delta.ReasoningContent == "" && len(delta.ToolCalls) == 0 &&
			text.Len() == 0 && thought.Len() == 0 && len(accumulators) == 0 &&
			(choice.Message.Content != nil || choice.Message.ReasoningContent != "" || len(choice.Message.ToolCalls) > 0) {
			delta = choice.Message
		}

		if content := deltaContentToString(delta.Content); content != "" {
			text.WriteString(content)
			if err := onEvent(StreamEvent{Type: StreamTextDelta, Text: content}); err != nil {
				return err
			}
		}
		if delta.ReasoningContent != "" {
			thought.WriteString(delta.ReasoningContent)
			if err := onEvent(StreamEvent{Type: StreamThinkingDelta, Thought: delta.ReasoningContent}); err != nil {
				return err
			}
		}

		for _, tc := range delta.ToolCalls {
			idx := 0
			if tc.Index != nil {
				idx = *tc.Index
			}
			acc := accumulators[idx]
			if acc == nil {
				acc = &openAIToolCallAccumulator{}
				accumulators[idx] = acc
			}
			if tc.ID != "" {
				acc.id = tc.ID
			}
			if tc.Type != "" {
				acc.typ = tc.Type
			}
			if tc.Function.Name != "" {
				acc.name = tc.Function.Name
			}
			acc.args.WriteString(toolArgumentsToString(tc.Function.Arguments))
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading stream: %w", err)
	}
	if !sawData {
		return errors.New("stream ended without any data")
	}

	final := Message{
		Role:    "assistant",
		Usage:   usage,
		Version: 2,
	}
	if text.Len() > 0 {
		final.Content = text.String()
	}
	if thought.Len() > 0 {
		final.Thought = thought.String()
	}
	if len(accumulators) > 0 {
		indexes := make([]int, 0, len(accumulators))
		for idx := range accumulators {
			indexes = append(indexes, idx)
		}
		sort.Ints(indexes)
		for _, idx := range indexes {
			acc := accumulators[idx]
			if acc.name == "" {
				continue
			}
			typ := acc.typ
			if typ == "" {
				typ = "function"
			}
			args := acc.args.String()
			if args == "" {
				args = "{}"
			}
			final.ToolCalls = append(final.ToolCalls, ToolCall{
				Index: intPtr(idx),
				ID:    acc.id,
				Type:  typ,
				Function: FunctionCall{
					Name:      acc.name,
					Arguments: args,
				},
			})
		}
	}

	stopReason := mapFinishReason(finishReason)
	var doneErr error
	if stopReason == StopError {
		doneErr = fmt.Errorf("provider stopped the response: %s", finishReason)
	}
	final.StopReason = stopReason
	if doneErr != nil {
		final.ErrorMessage = doneErr.Error()
	}

	return onEvent(StreamEvent{
		Type:       StreamDone,
		Message:    &final,
		Usage:      usage,
		StopReason: stopReason,
		Err:        doneErr,
	})
}

// chatOpenAICompatible performs a non-streaming chat completion.
func chatOpenAICompatible(
	ctx context.Context,
	cfg openAICompatConfig,
	modelName string,
	messages []Message,
	tools []map[string]interface{},
) (Message, error) {
	req := OpenAIRequest{
		Model:    cfg.model(modelName),
		Messages: messagesToOpenAIMessages(messages),
	}
	if len(tools) > 0 {
		req.Tools = tools
		req.ToolChoice = "auto"
	}

	client := resty.New().SetTimeout(120 * time.Second)
	var response OpenAIChatCompletion
	res, err := doWithRetry(func() (*resty.Response, error) {
		return client.R().
			SetContext(ctx).
			SetHeader("Content-Type", "application/json").
			SetHeader("Authorization", "Bearer "+cfg.apiKey).
			SetBody(req).
			SetResult(&response).
			Post(cfg.endpoint())
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
	if len(response.Choices) == 0 {
		return Message{}, errors.New("no choices returned from provider")
	}

	msg := openAIMessageToMessage(response.Choices[0].Message)
	msg.Version = 2
	msg.StopReason = mapFinishReason(response.Choices[0].FinishReason)
	if response.Usage.TotalTokens > 0 {
		msg.Usage = Usage{
			PromptTokens:      response.Usage.PromptTokens,
			CompletionTokens:  response.Usage.CompletionTokens,
			TotalTokens:       response.Usage.TotalTokens,
			ThoughtsTokens:    response.Usage.CompletionTokensDetails.ReasoningTokens,
			CachedInputTokens: response.Usage.cachedTokens(),
		}
	}
	if msg.StopReason == StopError {
		msg.IsError = true
		msg.ErrorMessage = fmt.Sprintf("provider stopped the response: %s", response.Choices[0].FinishReason)
	}
	return msg, nil
}

func mapFinishReason(reason string) StopReason {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "", "stop", "end_turn", "tool_calls", "function_call":
		return StopNormal
	case "length", "max_tokens":
		return StopLength
	case "content_filter", "safety", "recitation":
		return StopError
	case "error":
		return StopError
	default:
		return StopNormal
	}
}

func deltaContentToString(v interface{}) string {
	switch content := v.(type) {
	case nil:
		return ""
	case string:
		return content
	default:
		b, err := json.Marshal(content)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

func toolArgumentsToString(v interface{}) string {
	switch args := v.(type) {
	case nil:
		return ""
	case string:
		return args
	default:
		b, err := json.Marshal(args)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

func intPtr(v int) *int {
	return &v
}
