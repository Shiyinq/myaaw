package provider

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/go-resty/resty/v2"
)

type OllamaProvider struct {
	baseURL string
	apiKey  string
}

type OllamaRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
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

func NewOllamaProvider(baseURL string, apiKey string) LLMProvider {
	return &OllamaProvider{
		baseURL: baseURL,
		apiKey:  apiKey,
	}
}

func (o *OllamaProvider) ProviderName() string {
	return "ollama"
}

func (o *OllamaProvider) Chat(modelName string, messages []Message) (Message, error) {
	client := resty.New()
	client.SetTimeout(120 * time.Second)
	_ = o.apiKey // unused for ollama

	request := OllamaRequest{
		Model:    modelName,
		Stream:   false,
		Messages: messages,
	}

	var response OllamaResponse
	res, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(request).
		SetResult(&response).
		Post(o.baseURL + "/api/chat")

	if err != nil || res.StatusCode() != 200 {
		msg := fmt.Sprintf("error fetching response: %v", err)
		if err == nil {
			msg = fmt.Sprintf("error fetching response: %s", res.String())
		}
		return Message{}, fmt.Errorf(msg)
	}

	return response.Message, nil
}

func (o *OllamaProvider) ChatStream(modelName string, messages []Message, callback func(Message) error) error {
	client := resty.New()
	client.SetTimeout(120 * time.Second)
	_ = o.apiKey // unused for ollama

	request := OllamaRequest{
		Model:    modelName,
		Stream:   true,
		Messages: messages,
	}

	res, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(request).
		SetDoNotParseResponse(true).
		Post(o.baseURL + "/api/chat")

	if err != nil || res.StatusCode() != 200 {
		msg := fmt.Sprintf("error fetching stream response: %v", err)
		if err == nil {
			msg = fmt.Sprintf("error fetching stream response: %s", res.String())
		}
		return fmt.Errorf(msg)
	}

	defer res.RawBody().Close()

	reader := bufio.NewReader(res.RawBody())
	var response OllamaResponse

	for {
		line, err := reader.ReadBytes('\n')
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
	res, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetResult(&response).
		Get(o.baseURL + "/api/tags")

	if err != nil || res.StatusCode() != 200 {
		msg := fmt.Sprintf("error fetching ollama tags: %v", err)
		if err == nil {
			msg = fmt.Sprintf("error fetching ollama tags: %s", res.String())
		}
		return nil, fmt.Errorf(msg)
	}

	return &response, nil
}
