package provider

import (
	"errors"
)

type LLMProvider interface {
	Chat(modelName string, messages []Message) (Message, error)
	Models() ([]string, error)
}

type Factory func(apiKey string) LLMProvider

var ProviderFactories = map[string]Factory{
	"ollama": NewOllamaProvider,
}

func CreateProvider(providerName string, apiKey string) (LLMProvider, error) {
	factory, exists := ProviderFactories[providerName]
	if !exists {
		return nil, errors.New("unknown provider")
	}
	return factory(apiKey), nil
}
