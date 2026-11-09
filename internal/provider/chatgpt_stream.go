package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"myaaw/internal/auth/chatgpt"
)

type chatGPTContentPart struct {
	Type        string            `json:"type"`
	Text        string            `json:"text,omitempty"`
	Refusal     string            `json:"refusal,omitempty"`
	Annotations []json.RawMessage `json:"annotations,omitempty"`
}

type chatGPTOutputItem struct {
	Type             string               `json:"type"`
	ID               string               `json:"id,omitempty"`
	Role             string               `json:"role,omitempty"`
	Status           string               `json:"status,omitempty"`
	Content          []chatGPTContentPart `json:"content,omitempty"`
	CallID           string               `json:"call_id,omitempty"`
	Name             string               `json:"name,omitempty"`
	Namespace        string               `json:"namespace,omitempty"`
	Arguments        string               `json:"arguments,omitempty"`
	EncryptedContent *string              `json:"encrypted_content,omitempty"`
	Summary          *[]json.RawMessage   `json:"summary,omitempty"`
}

type chatGPTContinuation struct {
	Output []chatGPTOutputItem `json:"output"`
}

type chatGPTResponse struct {
	Status            string              `json:"status"`
	Output            []chatGPTOutputItem `json:"output"`
	Error             *chatGPTWireError   `json:"error"`
	IncompleteDetails struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Usage struct {
		InputTokens        int `json:"input_tokens"`
		OutputTokens       int `json:"output_tokens"`
		TotalTokens        int `json:"total_tokens"`
		InputTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"input_tokens_details"`
		OutputTokensDetails struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"output_tokens_details"`
	} `json:"usage"`
}

type chatGPTWireError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Param   string `json:"param"`
}

type chatGPTStreamEvent struct {
	Type         string             `json:"type"`
	OutputIndex  int                `json:"output_index"`
	ContentIndex int                `json:"content_index"`
	SummaryIndex int                `json:"summary_index"`
	Part         json.RawMessage    `json:"part"`
	ItemID       string             `json:"item_id"`
	Item         *chatGPTOutputItem `json:"item"`
	Delta        string             `json:"delta"`
	Text         string             `json:"text"`
	Arguments    string             `json:"arguments"`
	Response     *chatGPTResponse   `json:"response"`
	Error        *chatGPTWireError  `json:"error"`
	Code         string             `json:"code"`
	Message      string             `json:"message"`
	Param        string             `json:"param"`
}

func (p *ChatGPTProvider) ChatStreamEvents(ctx context.Context, model string, messages []Message, tools []map[string]interface{}, onEvent func(StreamEvent) error) (err error) {
	ctx, cancel := context.WithTimeout(ctx, streamRequestTimeout())
	defer cancel()
	final := Message{Role: "assistant", Version: 2}
	// One lifecycle owner covers validation, auth, HTTP, parsing, cancellation,
	// and callback failures. The parser never emits StreamDone itself.
	defer func() {
		stop := StopNormal
		if err != nil {
			stop = StopError
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				stop = StopAborted
			}
			final.IsError, final.ErrorMessage = true, err.Error()
		}
		final.StopReason = stop
		doneErr := onEvent(StreamEvent{Type: StreamDone, Message: &final, Usage: final.Usage, StopReason: stop, Err: err})
		if err == nil {
			err = doneErr
		}
	}()
	model = strings.TrimSpace(p.DefaultModel(model))
	if model == "" {
		return errors.New("select a model from this ChatGPT account's model catalog; no subscription model is assumed")
	}
	wireTools, names, err := chatGPTTools(tools)
	if err != nil {
		return err
	}
	input, err := chatGPTInput(messages)
	if err != nil {
		return err
	}
	body, err := json.Marshal(chatGPTRequest{Model: model, Input: input, Store: false, Stream: true,
		Include: []string{"reasoning.encrypted_content"}, Tools: wireTools, Reasoning: chatGPTReasoningForModel(model)})
	if err != nil {
		return fmt.Errorf("cannot encode ChatGPT request: %w", err)
	}
	res, err := p.request(ctx, http.MethodPost, "/v1/responses", body)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return chatGPTHTTPError(res)
	}
	// Deliberately no automatic retries. In particular, quota errors can arrive
	// after text has already been delivered and must not duplicate that text.
	final, err = consumeChatGPTSSE(res.Body, names, res.Header.Get("x-request-id"), onEvent)
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func chatGPTMessage(output []chatGPTOutputItem, names map[string]string, validateTools bool) (Message, error) {
	m := Message{Role: "assistant", Version: 2}
	var text strings.Builder
	var summaries []string
	seen := map[string]bool{}
	for i := range output {
		item := &output[i]
		if item.Status == "in_progress" || item.Status == "incomplete" {
			return m, errors.New("ChatGPT completed response contains an unfinished output item")
		}
		switch item.Type {
		case "message":
			if item.Role != "assistant" {
				return m, errors.New("ChatGPT returned a non-assistant output message")
			}
			for _, part := range item.Content {
				switch part.Type {
				case "output_text":
					text.WriteString(part.Text)
				case "refusal":
					text.WriteString(part.Refusal)
				default:
					return m, fmt.Errorf("unsupported ChatGPT output content %q", part.Type)
				}
			}
		case "function_call":
			if item.CallID == "" || seen[item.CallID] {
				return m, errors.New("ChatGPT returned duplicate or missing tool call IDs")
			}
			seen[item.CallID] = true
			name := strings.TrimPrefix(item.Name, chatGPTNamespace+".")
			if item.Namespace != "" && item.Namespace != chatGPTNamespace {
				return m, fmt.Errorf("unknown ChatGPT tool namespace %q", item.Namespace)
			}
			if !chatGPTToolName.MatchString(name) {
				return m, errors.New("ChatGPT returned an invalid tool name")
			}
			if validateTools {
				var err error
				name, err = chatGPTLocalName(item.Namespace, item.Name, names)
				if err != nil {
					return m, err
				}
			}
			if !json.Valid([]byte(item.Arguments)) {
				return m, errors.New("ChatGPT returned invalid or truncated tool argument JSON")
			}
			item.Name, item.Namespace = name, chatGPTNamespace
			m.ToolCalls = append(m.ToolCalls, ToolCall{ID: item.CallID, Type: "function", Function: FunctionCall{Name: name, Arguments: item.Arguments}})
		case "reasoning":
			if item.ID == "" {
				return m, errors.New("ChatGPT reasoning continuation is missing its item ID")
			}
			if item.Summary == nil {
				// An empty summary array is required on reasoning input items.
				empty := []json.RawMessage{}
				item.Summary = &empty
			}
			for _, raw := range *item.Summary {
				summary, err := chatGPTSummaryText(raw)
				if err != nil {
					return m, err
				}
				if summary != "" {
					summaries = append(summaries, summary)
				}
			}
			// Encrypted content is opaque; only explicit summaries are displayable.
		default:
			return m, fmt.Errorf("unsupported ChatGPT output item %q", item.Type)
		}
	}
	m.Content = text.String()
	m.Thought = strings.Join(summaries, "\n\n")
	if len(output) > 0 {
		metadata, err := json.Marshal(chatGPTContinuation{Output: output})
		if err != nil {
			return m, err
		}
		m.ProviderMetadata = map[string]json.RawMessage{chatgpt.ProviderType: metadata}
	}
	return m, nil
}

func chatGPTSummaryText(raw json.RawMessage) (string, error) {
	var part struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &part); err != nil {
		return "", errors.New("invalid ChatGPT reasoning summary part")
	}
	if part.Type != "summary_text" {
		return "", nil
	}
	return part.Text, nil
}

func chatGPTSetSummary(item *chatGPTOutputItem, index int, text string, raw json.RawMessage) error {
	if item.Summary == nil {
		parts := []json.RawMessage{}
		item.Summary = &parts
	}
	for len(*item.Summary) <= index {
		*item.Summary = append(*item.Summary, json.RawMessage(`{"type":"summary_text","text":""}`))
	}
	part := map[string]interface{}{}
	if len(raw) == 0 {
		raw = (*item.Summary)[index]
	}
	if err := json.Unmarshal(raw, &part); err != nil || part == nil {
		return errors.New("invalid ChatGPT reasoning summary part")
	}
	part["type"], part["text"] = "summary_text", text
	encoded, err := json.Marshal(part)
	if err != nil {
		return errors.New("cannot preserve ChatGPT reasoning summary part")
	}
	(*item.Summary)[index] = encoded
	return nil
}

func consumeChatGPTSSE(r io.Reader, names map[string]string, requestID string, onEvent func(StreamEvent) error) (final Message, err error) {
	final = Message{Role: "assistant", Version: 2}
	items := map[int]*chatGPTOutputItem{}
	completedItems := map[string]chatGPTOutputItem{}
	var streamedText, thought strings.Builder
	// Track identities, not text equality: two different parts may legitimately
	// say the same thing, while done/item/completed snapshots repeat one part.
	type summaryKey struct {
		output, part int
		raw          bool
	}
	summaries := map[summaryKey]string{}
	var lastSummary summaryKey
	updateSummary := func(key summaryKey, text string, snapshot bool) error {
		if key.output < 0 || key.output > 10000 || key.part < 0 || key.part > 10000 {
			return errors.New("invalid ChatGPT reasoning summary index")
		}
		if text == "" {
			return nil
		}
		previous := summaries[key]
		if snapshot {
			if strings.HasPrefix(previous, text) {
				return nil
			}
			if !strings.HasPrefix(text, previous) {
				return errors.New("ChatGPT reasoning summary changed after streaming")
			}
			summaries[key] = text
		} else {
			summaries[key] = previous + text
		}
		delta := summaries[key][len(previous):]
		// Raw-content and summary indices belong to different arrays. Keep both
		// identities and stream delivery order, even if their events interleave.
		if thought.Len() > 0 && lastSummary != key {
			delta = "\n\n" + delta
		}
		lastSummary = key
		thought.WriteString(delta)
		if delta != "" {
			return onEvent(StreamEvent{Type: StreamThinkingDelta, Thought: delta})
		}
		return nil
	}
	summarySnapshot := func(index int, item *chatGPTOutputItem) error {
		if item.Type != "reasoning" || item.Summary == nil {
			return nil
		}
		for part, raw := range *item.Summary {
			text, err := chatGPTSummaryText(raw)
			if err != nil {
				return err
			}
			if err := updateSummary(summaryKey{output: index, part: part}, text, true); err != nil {
				return err
			}
		}
		return nil
	}
	var usage Usage
	defer func() {
		if err != nil {
			// Keep partial text, but never advertise unfinished calls for execution.
			final = Message{Role: "assistant", Version: 2, Content: streamedText.String()}
		}
		final.Thought, final.Usage = thought.String(), usage
	}()
	ordered := func() []chatGPTOutputItem {
		indexes := make([]int, 0, len(items))
		for index := range items {
			indexes = append(indexes, index)
		}
		sort.Ints(indexes)
		out := make([]chatGPTOutputItem, 0, len(indexes))
		for _, index := range indexes {
			out = append(out, *items[index])
		}
		return out
	}
	getItem := func(ev chatGPTStreamEvent) (*chatGPTOutputItem, error) {
		item := items[ev.OutputIndex]
		if item == nil || (ev.ItemID != "" && item.ID != ev.ItemID) {
			return nil, errors.New("ChatGPT stream delta has no matching output item")
		}
		return item, nil
	}
	apiError := func(wire *chatGPTWireError, fallback string, raw string) error {
		e := &ChatGPTAPIError{Status: http.StatusOK, RequestID: requestID, Body: raw, Message: fallback}
		if wire != nil {
			e.Code, e.Param, e.Message = wire.Code, wire.Param, wire.Message
		}
		return e
	}
	completed := false
	process := func(eventType, data string) error {
		if data == "[DONE]" {
			return errors.New("ChatGPT stream ended without response.completed")
		}
		var ev chatGPTStreamEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return fmt.Errorf("invalid ChatGPT SSE event: %w", err)
		}
		if ev.Type == "" {
			ev.Type = eventType
		}
		switch ev.Type {
		case "response.output_item.added", "response.output_item.done":
			if ev.Item == nil || ev.OutputIndex < 0 {
				return errors.New("ChatGPT stream is missing its output item")
			}
			items[ev.OutputIndex] = ev.Item
			if ev.Type == "response.output_item.done" && ev.Item.ID != "" {
				completedItems[ev.Item.ID] = *ev.Item
			}
			if err := summarySnapshot(ev.OutputIndex, ev.Item); err != nil {
				return err
			}
		case "response.output_text.delta", "response.refusal.delta":
			item, err := getItem(ev)
			if err != nil {
				return err
			}
			if item.Type != "message" || ev.ContentIndex < 0 || ev.ContentIndex > 10000 {
				return errors.New("invalid ChatGPT text delta index or item")
			}
			for len(item.Content) <= ev.ContentIndex {
				item.Content = append(item.Content, chatGPTContentPart{})
			}
			part := &item.Content[ev.ContentIndex]
			if ev.Type == "response.refusal.delta" {
				part.Type = "refusal"
				part.Refusal += ev.Delta
			} else {
				part.Type = "output_text"
				part.Text += ev.Delta
			}
			streamedText.WriteString(ev.Delta)
			if ev.Delta != "" {
				return onEvent(StreamEvent{Type: StreamTextDelta, Text: ev.Delta})
			}
		case "response.output_text.done":
			item, err := getItem(ev)
			if err != nil {
				return err
			}
			if item.Type != "message" || ev.ContentIndex < 0 || ev.ContentIndex > 10000 {
				return errors.New("invalid ChatGPT text completion")
			}
			for len(item.Content) <= ev.ContentIndex {
				item.Content = append(item.Content, chatGPTContentPart{})
			}
			item.Content[ev.ContentIndex] = chatGPTContentPart{Type: "output_text", Text: ev.Text}
		case "response.function_call_arguments.delta", "response.function_call_arguments.done":
			item, err := getItem(ev)
			if err != nil {
				return err
			}
			if item.Type != "function_call" {
				return errors.New("ChatGPT tool arguments do not belong to a function call")
			}
			if ev.Type == "response.function_call_arguments.done" {
				item.Arguments = ev.Arguments
			} else {
				item.Arguments += ev.Delta
			}
		case "response.reasoning_summary_text.delta", "response.reasoning_summary_text.done",
			"response.reasoning_summary_part.added", "response.reasoning_summary_part.done",
			"response.reasoning_text.delta", "response.reasoning_text.done":
			part := ev.SummaryIndex
			rawReasoning := strings.HasPrefix(ev.Type, "response.reasoning_text.")
			if rawReasoning {
				part = ev.ContentIndex
			}
			key := summaryKey{output: ev.OutputIndex, part: part, raw: rawReasoning}
			if key.output < 0 || key.output > 10000 || key.part < 0 || key.part > 10000 {
				return errors.New("invalid ChatGPT reasoning summary index")
			}
			item := items[ev.OutputIndex]
			if ev.ItemID != "" && (item == nil || item.ID != ev.ItemID || item.Type != "reasoning") {
				return errors.New("ChatGPT reasoning summary has no matching output item")
			}
			text := ev.Text
			snapshot := !strings.HasSuffix(ev.Type, ".delta")
			if !snapshot {
				text = ev.Delta
			} else if strings.HasPrefix(ev.Type, "response.reasoning_summary_part.") {
				var summaryPart struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}
				if err := json.Unmarshal(ev.Part, &summaryPart); err != nil {
					return errors.New("invalid ChatGPT reasoning summary part")
				}
				if summaryPart.Type != "summary_text" {
					return nil
				}
				text = summaryPart.Text
			}
			if err := updateSummary(key, text, snapshot); err != nil {
				return err
			}
			if item != nil && item.Type == "reasoning" && !strings.HasPrefix(ev.Type, "response.reasoning_text.") {
				// Keep replayable summary parts when a terminal response omits output.
				if err := chatGPTSetSummary(item, part, summaries[key], ev.Part); err != nil {
					return err
				}
			}
		case "response.completed":
			if ev.Response == nil {
				return errors.New("ChatGPT completed event is missing its response")
			}
			if ev.Response.Status != "" && ev.Response.Status != "completed" {
				return errors.New("ChatGPT completed event has a non-completed response status")
			}
			if ev.Response.Error != nil {
				return apiError(ev.Response.Error, "response failed", data)
			}
			output := ev.Response.Output
			if output == nil {
				output = ordered()
			}
			for i := range output {
				item := &output[i]
				if item.Type == "reasoning" && item.EncryptedContent == nil {
					// Some terminal snapshots omit the ciphertext. Only a done
					// item, never an added item, contains safe continuation state.
					if done, ok := completedItems[item.ID]; ok {
						item.EncryptedContent = done.EncryptedContent
					}
				}
				if err := summarySnapshot(i, item); err != nil {
					return err
				}
			}
			message, err := chatGPTMessage(output, names, true)
			if err != nil {
				return err
			}
			u := ev.Response.Usage
			usage = Usage{PromptTokens: u.InputTokens, CompletionTokens: u.OutputTokens, TotalTokens: u.TotalTokens,
				CachedInputTokens: u.InputTokensDetails.CachedTokens, ThoughtsTokens: u.OutputTokensDetails.ReasoningTokens}
			final = message
			if err := onEvent(StreamEvent{Type: StreamUsage, Usage: usage}); err != nil {
				return err
			}
			completed = true
		case "response.failed":
			if ev.Response == nil {
				return apiError(nil, "response failed", data)
			}
			return apiError(ev.Response.Error, "response failed", data)
		case "response.incomplete":
			reason := "unknown reason"
			if ev.Response != nil && ev.Response.IncompleteDetails.Reason != "" {
				reason = ev.Response.IncompleteDetails.Reason
			}
			return fmt.Errorf("ChatGPT response incomplete: %s", reason)
		case "error", "response.error":
			wire := ev.Error
			if wire == nil {
				wire = &chatGPTWireError{Code: ev.Code, Param: ev.Param, Message: ev.Message}
			}
			return apiError(wire, "stream error", data)
		}
		return nil
	}

	// SSE data fields are joined with newlines, not parsed independently. A
	// terminal JSON frame without its blank-line delimiter is a truncated event.
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 8<<20)
	var data strings.Builder
	eventType := ""
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if data.Len() > 0 {
				if err := process(eventType, strings.TrimSuffix(data.String(), "\n")); err != nil {
					return final, err
				}
				if completed {
					return final, nil
				}
			}
			data.Reset()
			eventType = ""
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			eventType = value
		case "data":
			if data.Len()+len(value) > 16<<20 {
				return final, errors.New("ChatGPT SSE event exceeds size limit")
			}
			data.WriteString(value)
			data.WriteByte('\n')
		}
	}
	if err := scanner.Err(); err != nil {
		return final, fmt.Errorf("error reading ChatGPT stream: %w", err)
	}
	return final, errors.New("ChatGPT stream interrupted: EOF without response.completed")
}
