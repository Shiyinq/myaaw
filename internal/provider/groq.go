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
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
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
	baseURL string
	apiKey  string
}

func NewGroqProvider(baseURL string, apiKey string) LLMProvider {
	return &GroqProvider{
		baseURL: baseURL,
		apiKey:  apiKey,
	}
}

func (o *GroqProvider) ProviderName() string {
	return "groq"
}

func (o *GroqProvider) Chat(modelName string, messages []Message) (Message, error) {
	client := resty.New()
	client.SetTimeout(120 * time.Second)

	request := GroqRequest{
		Model:    modelName,
		Stream:   false,
		Messages: messages,
	}

	var response GroqChatCompletion
	res, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", o.apiKey)).
		SetBody(request).
		SetResult(&response).
		Post(o.baseURL + "/v1/chat/completions")

	if err != nil || res.StatusCode() != 200 {
		msg := fmt.Sprintf("error fetching response: %v", err)
		if err == nil {
			msg = fmt.Sprintf("error fetching response: %s", res.String())
		}
		return Message{}, fmt.Errorf(msg)
	}

	return response.Choices[0].Message, nil
}

func (o *GroqProvider) ChatStream(modelName string, messages []Message, callback func(Message) error) error {
	client := resty.New()
	client.SetTimeout(120 * time.Second)

	request := GroqRequest{
		Model:    modelName,
		Stream:   true,
		Messages: messages,
	}

	res, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", o.apiKey)).
		SetBody(request).
		SetDoNotParseResponse(true).
		Post(o.baseURL + "/v1/chat/completions")

	if err != nil || res.StatusCode() != 200 {
		msg := fmt.Sprintf("error fetching stream response: %v", err)
		if err == nil {
			msg = fmt.Sprintf("error fetching stream response: %s", res.String())
		}
		return fmt.Errorf(msg)
	}

	defer res.RawBody().Close()

	reader := bufio.NewReader(res.RawBody())
	var response GroqChatCompletion
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

func (o *GroqProvider) Models() ([]string, error) {
	response, err := o.groqModels()
	if err != nil {
		return nil, err
	}

	var models []string
	for _, model := range response.Data {
		models = append(models, model.ID)
	}

	return models, nil
}

func (o *GroqProvider) groqModels() (*GroqModels, error) {
	client := resty.New()

	var response GroqModels
	res, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", o.apiKey)).
		SetResult(&response).
		Get(o.baseURL + "/v1/models")

	if err != nil || res.StatusCode() != 200 {
		msg := fmt.Sprintf("error fetching openai models: %v", err)
		if err == nil {
			msg = fmt.Sprintf("error fetching openai models: %s", res.String())
		}
		return nil, fmt.Errorf(msg)
	}

	return &response, nil
}
