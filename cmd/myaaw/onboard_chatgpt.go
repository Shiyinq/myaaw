package main

import (
	"context"

	"myaaw/internal/auth/chatgpt"
	"myaaw/internal/config"
	"myaaw/internal/provider"

	tea "github.com/charmbracelet/bubbletea"
)

func chatGPTAuthMode(providerType string) string {
	if providerType == chatgpt.ProviderType {
		return "oauth"
	}
	return ""
}

type chatGPTSignedInMsg struct {
	credentialID string
	err          error
}

func (m onboardModel) startChatGPTSignIn() (tea.Model, tea.Cmd) {
	m.step = stepProviderModel
	m.modelsLoading, m.modelManual = true, false
	m.models, m.choices, m.modelsErr = nil, nil, nil
	ctx, cancel := context.WithCancel(context.Background())
	m.chatGPTCancel = cancel
	id := m.credentialID
	return m, func() tea.Msg {
		defer cancel()
		credentialID, err := signInChatGPT(ctx, id)
		return chatGPTSignedInMsg{credentialID: credentialID, err: err}
	}
}

func fetchChatGPTModels(credentialID string) tea.Cmd {
	return func() tea.Msg {
		models, err := provider.ModelsForConfig(config.ProviderConfig{
			Type: chatgpt.ProviderType, AuthMode: "oauth", CredentialID: credentialID,
		})
		return modelsFetchedMsg{models: models, err: err}
	}
}
