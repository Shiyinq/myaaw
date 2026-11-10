package service

import "myaaw/internal/onboard"

// CapabilityView is one built-in skill or tool the onboarding wizard can
// install or enable.
type CapabilityView struct {
	Name        string `json:"name" example:"weather"`
	Description string `json:"description,omitempty"`
}

// ProviderOptionView is one provider preset the wizard offers, with the endpoint
// and model it prefills. Presets sharing a wire type (DeepSeek and OpenAI) only
// differ by id, endpoint, and model.
type ProviderOptionView struct {
	ID             string `json:"id" example:"deepseek"`
	Type           string `json:"type" example:"openai"`
	Label          string `json:"label" example:"OpenAI (or compatible)"`
	DefaultBaseURL string `json:"default_base_url,omitempty" example:"https://api.openai.com"`
	DefaultModel   string `json:"default_model,omitempty" example:"gpt-4o"`
}

// CapabilitiesView is everything a client needs to build an onboarding form
// before the agent exists. Agents that do not exist yet cannot be queried for
// their skills or tools, so this catalog is global.
type CapabilitiesView struct {
	Providers []ProviderOptionView `json:"providers"`
	Skills    []CapabilityView     `json:"skills"`
	Tools     []CapabilityView     `json:"tools"`
}

// BuiltinCapabilities lists the provider types plus the embedded skills and
// built-in tools an agent can be created with. It mirrors
// `myaaw onboard --list-capabilities --json`.
func BuiltinCapabilities() CapabilitiesView {
	caps := onboard.BuiltinCapabilities()

	view := CapabilitiesView{
		Providers: make([]ProviderOptionView, 0, len(caps.Providers)),
		Skills:    make([]CapabilityView, 0, len(caps.Skills)),
		Tools:     make([]CapabilityView, 0, len(caps.Tools)),
	}
	for _, option := range caps.Providers {
		view.Providers = append(view.Providers, ProviderOptionView{
			ID:             option.ID,
			Type:           option.Type,
			Label:          option.Label,
			DefaultBaseURL: option.DefaultBaseURL,
			DefaultModel:   option.DefaultModel,
		})
	}
	for _, entry := range caps.Skills {
		view.Skills = append(view.Skills, CapabilityView{Name: entry.Name, Description: entry.Description})
	}
	for _, entry := range caps.Tools {
		view.Tools = append(view.Tools, CapabilityView{Name: entry.Name, Description: entry.Description})
	}
	return view
}
