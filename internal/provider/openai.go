package provider

import (
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
)

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	Logprobs     *string `json:"logprobs"`
	FinishReason string  `json:"finish_reason"`
}

type CompletionTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

type Usage struct {
	PromptTokens            int                     `json:"prompt_tokens"`
	CompletionTokens        int                     `json:"completion_tokens"`
	TotalTokens             int                     `json:"total_tokens"`
	CompletionTokensDetails CompletionTokensDetails `json:"completion_tokens_details"`
}

type ChatCompletion struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"`
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	SystemFingerprint string   `json:"system_fingerprint"`
	Choices           []Choice `json:"choices"`
	Usage             Usage    `json:"usage"`
}

type OpenAIRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type OpenAIProvider struct {
	baseURL string
	apiKey  string
}

func NewOpenAIProvider(baseURL string, apiKey string) LLMProvider {
	return &OpenAIProvider{
		baseURL: baseURL,
		apiKey:  apiKey,
	}
}

func (o *OpenAIProvider) ProviderName() string {
	return "openai"
}

func (o *OpenAIProvider) Chat(modelName string, messages []Message) (Message, error) {
	client := resty.New()
	client.SetTimeout(120 * time.Second)

	request := OpenAIRequest{
		Model:    modelName,
		Stream:   false,
		Messages: messages,
	}

	var response ChatCompletion
	_, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", o.apiKey)).
		SetBody(request).
		SetResult(&response).
		Post(o.baseURL + "/v1/chat/completions")

	if err != nil {
		return Message{}, err
	}

	return response.Choices[0].Message, nil
}

func (o *OpenAIProvider) ChatStream(modelName string, messages []Message, callback func(Message) error) error {
	return nil
}

func (o *OpenAIProvider) Models() ([]string, error) {
	var models []string
	return models, nil
}
