package provider

import (
	"context"
	"fmt"

	"github.com/go-resty/resty/v2"
)

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

func (g *GroqProvider) config() openAICompatConfig {
	return openAICompatConfig{
		baseURL:      g.baseURL,
		apiKey:       g.apiKey,
		defaultModel: g.defaultModel,
		// Groq does not consistently accept stream_options; omit it.
		includeUsage: false,
	}
}

func (g *GroqProvider) Chat(modelName string, messages []Message, toolsList []map[string]interface{}) (Message, error) {
	return chatOpenAICompatible(context.Background(), g.config(), modelName, messages, toolsList)
}

func (g *GroqProvider) ChatStream(modelName string, messages []Message, callback func(Message) error, toolsList []map[string]interface{}) error {
	return chatStreamFromEvents(context.Background(), g, modelName, messages, toolsList, callback)
}

func (g *GroqProvider) ChatStreamEvents(
	ctx context.Context,
	modelName string,
	messages []Message,
	toolsList []map[string]interface{},
	onEvent func(StreamEvent) error,
) error {
	return streamOpenAICompatible(ctx, g.config(), modelName, messages, toolsList, onEvent)
}

func (g *GroqProvider) Models() ([]string, error) {
	tags, err := g.groqModels()
	if err != nil {
		return nil, err
	}

	models := make([]string, 0, len(tags.Data))
	for _, model := range tags.Data {
		models = append(models, model.ID)
	}

	return models, nil
}

func (g *GroqProvider) groqModels() (*GroqModels, error) {
	client := resty.New().SetTimeout(modelListTimeout)

	var response GroqModels
	res, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", g.apiKey)).
		SetResult(&response).
		Get(g.baseURL + "/v1/models")
	if err != nil {
		return nil, fmt.Errorf("error fetching groq models: %w", err)
	}
	if res == nil {
		return nil, fmt.Errorf("error fetching groq models: nil response")
	}
	if res.StatusCode() != 200 {
		return nil, fmt.Errorf("error fetching groq models: %s", res.String())
	}

	return &response, nil
}
