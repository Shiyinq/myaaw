package onboard

import (
	"fmt"
	"strings"

	"myaaw/internal/auth/chatgpt"
	"myaaw/internal/config"
)

// NormalizeProviderAuth keeps API-key providers independent of host OAuth
// credentials. A subscription reference must point to a verified local account,
// never to an unfinished dynamic-client registration.
func NormalizeProviderAuth(p *config.ProviderConfig) error {
	if !strings.EqualFold(strings.TrimSpace(p.Type), chatgpt.ProviderType) {
		p.AuthMode, p.CredentialID = "", ""
		return nil
	}
	p.Type = chatgpt.ProviderType
	baseURL := strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	if strings.TrimSpace(p.APIKey) != "" || (baseURL != "" && baseURL != "https://api.openai.com") {
		return fmt.Errorf("%s does not accept api_key or a custom base_url", chatgpt.Label)
	}
	if p.AuthMode != "" && p.AuthMode != "oauth" {
		return fmt.Errorf("%s requires auth_mode oauth", chatgpt.Label)
	}
	p.AuthMode = "oauth"
	p.CredentialID = strings.TrimSpace(p.CredentialID)
	if p.CredentialID == "" {
		return fmt.Errorf("%s requires a connected credential_id; connect using a browser on this machine", chatgpt.Label)
	}
	manager, err := chatgpt.DefaultManager()
	if err != nil {
		return fmt.Errorf("cannot open ChatGPT Subscription credentials")
	}
	if _, err := manager.Get(p.CredentialID); err != nil {
		return fmt.Errorf("ChatGPT Subscription credential is unavailable; reconnect on this machine")
	}
	p.APIKey, p.BaseURL = "", ""
	return nil
}
