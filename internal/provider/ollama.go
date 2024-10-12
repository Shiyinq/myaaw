package provider

import (
	"teo/internal/config"
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

type Message struct {
	Role    string `json:"role" bson:"role"`
	Content string `json:"content" bson:"content"`
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

func NewOllamaProvider(apiKey string) LLMProvider {
	return &OllamaProvider{
		baseURL: config.OllamaBaseUrl,
		apiKey:  apiKey,
	}
}

func (o *OllamaProvider) Chat(modelName string, messages []Message) (Message, error) {
	client := resty.New()
	client.SetTimeout(90 * time.Second)
	_ = o.apiKey // unused for ollama

	request := OllamaRequest{
		Model:    modelName,
		Stream:   false,
		Messages: messages,
	}

	var response OllamaResponse
	_, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(request).
		SetResult(&response).
		Post(o.baseURL + "/api/chat")

	if err != nil {
		return Message{}, err
	}

	return response.Message, nil
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
	_, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetResult(&response).
		Get(o.baseURL + "/api/tags")

	if err != nil {
		return nil, err
	}

	return &response, nil
}
