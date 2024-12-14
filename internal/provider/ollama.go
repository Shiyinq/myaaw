package provider

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"teo/internal/tools"
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

func (o *OllamaProvider) Chat(modelName string, messages []Message) (Message, error) {
	client := resty.New()
	client.SetTimeout(120 * time.Second)
	_ = o.apiKey // unused for ollama

	request := OllamaRequest{
		Model:    o.DefaultModel(modelName),
		Stream:   false,
		Messages: messages,
		Tools:    tools.GetTools(),
	}

	var response OllamaResponse
	res, _ := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(request).
		SetResult(&response).
		Post(o.baseURL + "/api/chat")

	if res.StatusCode() != 200 {
		return Message{}, fmt.Errorf("error fetching response: %v", res.String())
	}

	if response.Message.ToolCalls != nil {
		resp_tool := toolCalls(messages, response.Message)
		return o.Chat(modelName, resp_tool)
	}

	return response.Message, nil
}

func (o *OllamaProvider) ChatStream(modelName string, messages []Message, callback func(Message) error) error {
	client := resty.New()
	client.SetTimeout(120 * time.Second)
	_ = o.apiKey // unused for ollama

	request := OllamaRequest{
		Model:    o.DefaultModel(modelName),
		Stream:   true,
		Messages: messages,
	}

	res, _ := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(request).
		SetDoNotParseResponse(true).
		Post(o.baseURL + "/api/chat")

	defer res.RawBody().Close()

	reader := bufio.NewReader(res.RawBody())
	var response OllamaResponse

	for {
		line, err := reader.ReadBytes('\n')

		if res.StatusCode() != 200 {
			return fmt.Errorf("error fetching stream response: %v", string(line))
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error reading stream: %w", err)
		}

		err = json.Unmarshal(line, &response)
		if err != nil {
			return fmt.Errorf("error unmarshalling stream data: %w", err)
		}

		partialMessage := response.Message
		err = callback(partialMessage)
		if err != nil {
			return fmt.Errorf("error in callback: %w", err)
		}

		if response.Done {
			break
		}
	}

	return nil
}

func (o *OllamaProvider) Models() ([]string, error) {
	tags, err := o.ollamaTags()
	if err != nil {
		return nil, err
	}

	var models []string
	for _, model := range tags.Models {
		models = append(models, model.Name)
	}

	return models, nil
}

func (o *OllamaProvider) ollamaTags() (*OllamaTagsResponse, error) {
	client := resty.New()

	var response OllamaTagsResponse
	res, _ := client.R().
		SetHeader("Content-Type", "application/json").
		SetResult(&response).
		Get(o.baseURL + "/api/tags")

	if res.StatusCode() != 200 {
		return nil, fmt.Errorf("error fetching ollama models: %s", res.String())
	}

	return &response, nil
}
