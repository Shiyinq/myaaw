package service

import (
	"testing"

	"myaaw/internal/config"
)

func TestNewBotServiceWithoutProviderDoesNotExit(t *testing.T) {
	oldName := config.LLMProviderName
	config.LLMProviderName = "not-a-real-provider"
	t.Cleanup(func() { config.LLMProviderName = oldName })

	svc, ok := NewBotService(nil, nil).(*BotServiceImpl)
	if !ok {
		t.Fatalf("unexpected service type %T", svc)
	}
	if err := svc.requireProvider(); err == nil {
		t.Fatal("expected requireProvider to refuse a missing provider")
	}
}
