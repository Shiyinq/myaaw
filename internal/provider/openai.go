package provider

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

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

type OpenAIUsage struct {
	PromptTokens            int                           `json:"prompt_tokens"`
	CompletionTokens        int                           `json:"completion_tokens"`
	TotalTokens             int                           `json:"total_tokens"`
	CompletionTokensDetails OpenAICompletionTokensDetails `json:"completion_tokens_details"`
}

type OpenAIChatCompletion struct {
	ID                string         `json:"id"`
	Object            string         `json:"object"`
	Created           int64          `json:"created"`
	Model             string         `json:"model"`
	SystemFingerprint string         `json:"system_fingerprint"`
	Choices           []OpenAIChoice `json:"choices"`
	Usage             OpenAIUsage    `json:"usage"`
}

type OpenAIRequest struct {
	Model      string                   `json:"model"`
	Messages   []OpenAIMessage          `json:"messages"`
	Stream     bool                     `json:"stream"`
	Tools      []map[string]interface{} `json:"tools,omitempty"`
	ToolChoice string                   `json:"tool_choice,omitempty"`
}

func messagesToOpenAIMessages(messages []Message) []OpenAIMessage {
	var openAIMessages []OpenAIMessage
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

func (o *OpenAIProvider) buildRequest(modelName string, messages []Message, stream bool, toolsList []map[string]interface{}) OpenAIRequest {
	req := OpenAIRequest{
		Model:      o.DefaultModel(modelName),
		Stream:     stream,
		Messages:   messagesToOpenAIMessages(messages),
	}
	
	if len(toolsList) > 0 {
		req.Tools = toolsList
		req.ToolChoice = "auto"
	}
	
	return req
}

func (o *OpenAIProvider) Chat(modelName string, messages []Message, toolsList []map[string]interface{}) (Message, error) {
	client := resty.New()
	client.SetTimeout(120 * time.Second)

	request := o.buildRequest(modelName, messages, false, toolsList)

	var response OpenAIChatCompletion
	res, _ := client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", o.apiKey)).
		SetBody(request).
		SetResult(&response).
		Post(o.baseURL + "/v1/chat/completions")

	if res.StatusCode() != 200 {
		return Message{}, fmt.Errorf("error fetching response: %v", res.String())
	}

	if response.Choices[0].FinishReason == "tool_calls" {
		return openAIMessageToMessage(response.Choices[0].Message), nil
	}

	return openAIMessageToMessage(response.Choices[0].Message), nil
}
func (o *OpenAIProvider) ChatStream(modelName string, messages []Message, callback func(Message) error, toolsList []map[string]interface{}) error {
	if modelName == "" {
		modelName = o.DefaultModel(modelName)
	}

	request := o.buildRequest(modelName, messages, true, toolsList)

	client := resty.New()
	client.SetTimeout(120 * time.Second)

	res, _ := client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", o.apiKey)).
		SetBody(request).
		SetDoNotParseResponse(true).
		Post(o.baseURL + "/v1/chat/completions")

	defer res.RawBody().Close()

	reader := bufio.NewReader(res.RawBody())
	var response OpenAIChatCompletion
	var accumulatedMessage OpenAIMessage
	accumulatedMessage.Role = "assistant"

	for {
		line, err := reader.ReadString('\n')

		if res.StatusCode() != 200 {
			return fmt.Errorf("error fetching stream response: %v", string(line))
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error reading stream: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "[DONE]" {
			break
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		jsonData := strings.TrimPrefix(line, "data: ")

		response = OpenAIChatCompletion{}
		err = json.Unmarshal([]byte(jsonData), &response)
		if err != nil {
			return fmt.Errorf("error unmarshalling stream data: %w", err)
		}

		partialMessage := response.Choices[0].Delta

		if len(partialMessage.ToolCalls) > 0 {
			for _, tcDelta := range partialMessage.ToolCalls {
				idx := 0
				if tcDelta.Index != nil {
					idx = *tcDelta.Index
				}
				for len(accumulatedMessage.ToolCalls) <= idx {
					accumulatedMessage.ToolCalls = append(accumulatedMessage.ToolCalls, ToolCall{})
				}
				if tcDelta.ID != "" {
					accumulatedMessage.ToolCalls[idx].ID = tcDelta.ID
				}
				if tcDelta.Type != "" {
					accumulatedMessage.ToolCalls[idx].Type = tcDelta.Type
				}
				if tcDelta.Function.Name != "" {
					accumulatedMessage.ToolCalls[idx].Function.Name = tcDelta.Function.Name
				}
				if tcDelta.Function.Arguments != nil {
					if argStr, ok := tcDelta.Function.Arguments.(string); ok {
						currArg, _ := accumulatedMessage.ToolCalls[idx].Function.Arguments.(string)
						accumulatedMessage.ToolCalls[idx].Function.Arguments = currArg + argStr
					}
				}
			}
		}

		hasContent := partialMessage.Content != nil && partialMessage.Content != ""
		hasReasoning := partialMessage.ReasoningContent != ""

		if hasContent || hasReasoning {
			err = callback(openAIMessageToMessage(partialMessage))
			if err != nil {
				return fmt.Errorf("error in callback: %w", err)
			}
		}

		if response.Choices[0].FinishReason == "tool_calls" {
			err = callback(openAIMessageToMessage(accumulatedMessage))
			if err != nil {
				return fmt.Errorf("error in callback for tool calls: %w", err)
			}
			break
		}

		if response.Choices[0].FinishReason == "stop" {
			break
		}
	}

	return nil
}

func (o *OpenAIProvider) Models() ([]string, error) {
	response, err := o.openAIModels()
	if err != nil {
		return nil, err
	}

	var models []string
	for _, model := range response.Data {
		models = append(models, model.ID)
	}

	return models, nil
}

func (o *OpenAIProvider) openAIModels() (*OpenAIModels, error) {
	client := resty.New()

	var response OpenAIModels
	res, _ := client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", o.apiKey)).
		SetResult(&response).
		Get(o.baseURL + "/v1/models")

	if res.StatusCode() != 200 {
		return nil, fmt.Errorf("error fetching openai models: %s", res.String())
	}

	return &response, nil
}
