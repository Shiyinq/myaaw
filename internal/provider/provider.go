package provider

import (
	"errors"
)

type LLMProvider interface {
	ProviderName() string
	Chat(modelName string, messages []Message) (Message, error)
	ChatStream(modelName string, messages []Message, callback func(Message) error) error
	Models() ([]string, error)
}

type Factory func(apiKey string) LLMProvider

var ProviderFactories = map[string]Factory{
	"ollama": NewOllamaProvider,
}

func CreateProvider(providerName string, apiKey string) (LLMProvider, error) {
	factory, exists := ProviderFactories[providerName]
	if !exists {
		return nil, errors.New("unknown llm provider")
	}
	return factory(apiKey), nil
}
