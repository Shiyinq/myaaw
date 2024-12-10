package provider

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"teo/internal/tools"
	"time"

	"github.com/go-resty/resty/v2"
)

type GroqChoice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	Delta        Message `json:"delta,omitempty"`
	Logprobs     *string `json:"logprobs,omitempty"`
	FinishReason string  `json:"finish_reason"`
}

type GroqUsage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	QueueTime        float64 `json:"queue_time"`
	PromptTime       float64 `json:"prompt_time"`
	CompletionTime   float64 `json:"completion_time"`
	TotalTime        float64 `json:"total_time"`
}

type XGroq struct {
	ID string `json:"id"`
}

type GroqChatCompletion struct {
	ID                string       `json:"id"`
	Object            string       `json:"object"`
	Created           int64        `json:"created"`
	Model             string       `json:"model"`
	SystemFingerprint string       `json:"system_fingerprint"`
	Choices           []GroqChoice `json:"choices"`
	Usage             GroqUsage    `json:"usage"`
	XGroq             XGroq        `json:"x_groq"`
}

type GroqRequest struct {
	Model      string                   `json:"model"`
	Messages   []Message                `json:"messages"`
	Stream     bool                     `json:"stream"`
	Tools      []map[string]interface{} `json:"tools,omitempty"`
	ToolChoice string                   `json:"tool_choice,omitempty"`
}

type GroqModels struct {
	Object string      `json:"object"`
	Data   []GroqModel `json:"data"`
}

type GroqModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type GroqProvider struct {
	baseURL      string
	apiKey       string
	defaultModel string
}

func NewGroqProvider(baseURL string, apiKey string, defaultModel string) LLMProvider {
	return &GroqProvider{
		baseURL:      baseURL,
		apiKey:       apiKey,
		defaultModel: defaultModel,
	}
}

func (g *GroqProvider) ProviderName() string {
	return "groq"
}

func (g *GroqProvider) DefaultModel(modelName string) string {
	if modelName == "" {
		return g.defaultModel
	}
	return modelName
}

func (g *GroqProvider) Chat(modelName string, messages []Message) (Message, error) {
	client := resty.New()
	client.SetTimeout(120 * time.Second)

	request := GroqRequest{
		Model:      g.DefaultModel(modelName),
		Stream:     false,
		Messages:   messages,
		Tools:      tools.GetTools(),
		ToolChoice: "auto",
	}

	var response GroqChatCompletion
	res, _ := client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", g.apiKey)).
		SetBody(request).
		SetResult(&response).
		Post(g.baseURL + "/v1/chat/completions")

	if res.StatusCode() != 200 {
		return Message{}, fmt.Errorf("error fetching response: %s", res.String())
	}

	if response.Choices[0].FinishReason == "tool_calls" {
		resp_tool := toolCalls(messages, response.Choices[0].Message)
		return g.Chat(modelName, resp_tool)
	}

	return response.Choices[0].Message, nil
}

func (g *GroqProvider) ChatStream(modelName string, messages []Message, callback func(Message) error) error {
	client := resty.New()
	client.SetTimeout(120 * time.Second)

	request := GroqRequest{
		Model:      g.DefaultModel(modelName),
		Stream:     true,
		Messages:   messages,
		Tools:      tools.GetTools(),
		ToolChoice: "auto",
	}

	res, _ := client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", g.apiKey)).
		SetBody(request).
		SetDoNotParseResponse(true).
		Post(g.baseURL + "/v1/chat/completions")

	defer res.RawBody().Close()

	reader := bufio.NewReader(res.RawBody())
	var response GroqChatCompletion
	for {
		line, err := reader.ReadString('\n')

		if res.StatusCode() != 200 {
			return fmt.Errorf("error fetching stream response: %v", line)
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

		err = json.Unmarshal([]byte(jsonData), &response)
		if err != nil {
			return fmt.Errorf("error unmarshalling stream data: %w", err)
		}

		partialMessage := response.Choices[0].Delta
		if partialMessage.Content == nil {
			partialMessage.Content = ""
		}
		err = callback(partialMessage)
		if err != nil {
			return fmt.Errorf("error in callback: %w", err)
		}

		if response.Choices[0].Delta.ToolCalls != nil {
			partialMessage.Role = "assistant"
			resp_tool := toolCalls(messages, partialMessage)
			return g.ChatStream(modelName, resp_tool, callback)
		}

		if response.Choices[0].FinishReason == "stop" {
			break
		}
	}

	return nil
}

func (g *GroqProvider) Models() ([]string, error) {
	response, err := g.groqModels()
	if err != nil {
		return nil, err
	}

	var models []string
	for _, model := range response.Data {
		models = append(models, model.ID)
	}

	return models, nil
}

func (g *GroqProvider) groqModels() (*GroqModels, error) {
	client := resty.New()

	var response GroqModels
	res, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", g.apiKey)).
		SetResult(&response).
		Get(g.baseURL + "/v1/models")

	if err != nil || res.StatusCode() != 200 {
		msg := fmt.Sprintf("error fetching openai models: %v", err)
		if err == nil {
			msg = fmt.Sprintf("error fetching openai models: %s", res.String())
		}
		return nil, fmt.Errorf(msg)
	}

	return &response, nil
}
