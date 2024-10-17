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

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	Delta        Message `json:"delta,omitempty"`
	Logprobs     *string `json:"logprobs,omitempty"`
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

type Models struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
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
	client := resty.New()
	client.SetTimeout(120 * time.Second)

	request := OpenAIRequest{
		Model:    modelName,
		Stream:   true,
		Messages: messages,
	}

	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", o.apiKey)).
		SetBody(request).
		SetDoNotParseResponse(true).
		Post(o.baseURL + "/v1/chat/completions")

	if err != nil {
		return err
	}
	defer resp.RawBody().Close()

	reader := bufio.NewReader(resp.RawBody())
	var response ChatCompletion
	for {
		line, err := reader.ReadString('\n')
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
		err = callback(partialMessage)
		if err != nil {
			return fmt.Errorf("error in callback: %w", err)
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

func (o *OpenAIProvider) openAIModels() (*Models, error) {
	client := resty.New()

	var response Models
	_, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", o.apiKey)).
		SetResult(&response).
		Get(o.baseURL + "/v1/models")

	if err != nil {
		return nil, err
	}

	return &response, nil
}
