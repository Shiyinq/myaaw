package agent

import (
	"testing"

	"myaaw/internal/config"
)

func TestDefaultRunConfigCarriesThinkingLevel(t *testing.T) {
	previous := config.LLMProviderThinkingLevel
	t.Cleanup(func() { config.LLMProviderThinkingLevel = previous })

	config.LLMProviderThinkingLevel = "high"
	if got := DefaultRunConfig("model").ThinkingLevel; got != "high" {
		t.Fatalf("thinking level = %q, want high", got)
	}

	config.LLMProviderThinkingLevel = ""
	if got := DefaultRunConfig("model").ThinkingLevel; got != "" {
		t.Fatalf("thinking level = %q, want empty", got)
	}
}
