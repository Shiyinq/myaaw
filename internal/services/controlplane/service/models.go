package service

import (
	"fmt"
	"strings"

	"myaaw/internal/onboard"
	"myaaw/internal/provider"
)

// ModelListView is the response for the model discovery endpoints.
type ModelListView struct {
	Models  []string `json:"models"`
	Source  string   `json:"source" example:"live"` // "live" or "default"
	Warning string   `json:"warning,omitempty"`
}

// ModelLookupInput lets a client list models before the provider is saved, so
// the create-agent wizard can fill the model picker before calling onboard.
type ModelLookupInput struct {
	Type    string `json:"type" example:"openai"`
	BaseURL string `json:"base_url,omitempty" example:"https://api.deepseek.com"`
	APIKey  string `json:"api_key,omitempty"`
}

// ModelsForCredentials lists models for a provider that is not saved yet.
func (s *ConfigService) ModelsForCredentials(in ModelLookupInput) (ModelListView, error) {
	providerType := strings.ToLower(strings.TrimSpace(in.Type))
	if providerType == "" || !provider.IsKnownLLMProvider(providerType) {
		return ModelListView{}, fmt.Errorf("%w: unknown provider type %q", ErrInvalidConfig, in.Type)
	}
	return lookupModels(providerType, in.BaseURL, in.APIKey), nil
}

// ProviderModels lists models for an already saved provider, using its stored
// type, base URL, and API key.
func (s *ConfigService) ProviderModels(agentName, id string) (ModelListView, error) {
	cfg, err := loadAgentConfig(agentName)
	if err != nil {
		return ModelListView{}, err
	}
	saved, ok := cfg.Providers[id]
	if !ok {
		return ModelListView{}, ErrProviderNotFound
	}
	apiKey := saved.APIKey
	if onboard.IsTemplateValue(apiKey) {
		apiKey = ""
	}
	return lookupModels(strings.ToLower(saved.Type), saved.BaseURL, apiKey), nil
}

// lookupModels tries the live provider list and falls back to the built-in
// default model so the wizard can always continue.
func lookupModels(providerType, baseURL, apiKey string) ModelListView {
	models, err := provider.ModelsFor(providerType, baseURL, apiKey)
	if err == nil && len(models) > 0 {
		return ModelListView{Models: models, Source: "live"}
	}

	view := ModelListView{Source: "default"}
	if err != nil {
		view.Warning = err.Error()
	}
	if fallback := provider.DefaultModel(providerType); fallback != "" {
		view.Models = []string{fallback}
	} else {
		view.Models = []string{}
	}
	return view
}
