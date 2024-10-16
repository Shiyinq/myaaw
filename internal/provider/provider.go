package provider

import (
	"errors"
	"teo/internal/config"
)

type LLMProvider interface {
	ProviderName() string
	Chat(modelName string, messages []Message) (Message, error)
	ChatStream(modelName string, messages []Message, callback func(Message) error) error
	Models() ([]string, error)
}

type Factory func(baseURL string, apiKey string) LLMProvider

var ProviderFactories = map[string]Factory{
	"ollama": NewOllamaProvider,
	"openai": NewOpenAIProvider,
}

func CreateProvider(providerName string, apiKey string) (LLMProvider, error) {
	factory, exists := ProviderFactories[providerName]
	if !exists {
		return nil, errors.New("unknown llm provider")
	}
	return factory(config.LLMProviderBaseURL, apiKey), nil
}
