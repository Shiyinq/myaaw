package provider

import (
	"testing"

	"myaaw/internal/config"
)

func TestModelsForUnknownProvider(t *testing.T) {
	if _, err := ModelsFor("does-not-exist", "", "x"); err == nil {
		t.Fatal("expected an error for an unknown provider")
	}
}

func TestModelsForDoesNotTouchGlobalBaseURL(t *testing.T) {
	before := config.LLMProviderBaseURL

	// Connection refused on port 1 fails fast; the point is that the global
	// base URL must stay untouched even when the request fails.
	if _, err := ModelsFor("openai", "http://127.0.0.1:1", "sk-x"); err == nil {
		t.Fatal("expected the request to fail")
	}
	if config.LLMProviderBaseURL != before {
		t.Fatalf("global base URL changed: %q -> %q", before, config.LLMProviderBaseURL)
	}
}
