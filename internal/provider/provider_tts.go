package provider

import (
	"errors"
	"teo/internal/config"
)

// TTSProvider defines the interface for text-to-speech providers.
type TTSProvider interface {
	SpeechToText(audioFile []byte, modelName string) (string, error)
}

// ttsProviderFactory is a function type that creates a TTSProvider.
type ttsProviderFactory func(apiKey string, defaultModel string) (TTSProvider, error)

// ttsProviders is a map of provider names to their factory functions.
var ttsProviders = make(map[string]ttsProviderFactory)

// RegisterTTSProvider is called by provider implementations to register themselves.
func RegisterTTSProvider(name string, factory ttsProviderFactory) {
	ttsProviders[name] = factory
}

// CreateTTSProvider creates a new TTS provider based on the provider name.
func CreateTTSProvider(providerName string, apiKey string, defaultModel string) (TTSProvider, error) {
	factory, ok := ttsProviders[providerName]
	if !ok {
		return nil, errors.New("unknown TTS provider: " + providerName)
	}
	return factory(apiKey, defaultModel)
}

// init initializes the known TTS providers.
func init() {
	// Currently, only "groq" is a known TTS provider.
	// We will add the actual GroqTTSProvider implementation in a later step.
	// For now, we can register a placeholder or leave it to be registered by the actual provider package.
	// Example of how Groq provider would register itself (actual implementation will be in its own file):
	// RegisterTTSProvider(config.ProviderGroq, NewGroqTTSProvider)
}
