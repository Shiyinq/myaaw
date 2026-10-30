package onboard

import (
	"path"
	"strings"

	myaaw "myaaw"
	"myaaw/internal/agent/tools"
	"myaaw/internal/provider"

	"gopkg.in/yaml.v3"
)

// Capability describes one built-in skill or tool offered by onboarding.
type Capability struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// ProviderOption describes a provider preset the onboarding wizard offers, with
// the endpoint and model it prefills. Several presets can share a wire type:
// DeepSeek speaks the OpenAI protocol, so it is stored as `provider.type:
// openai` with its own endpoint.
type ProviderOption struct {
	ID             string `json:"id"`
	Type           string `json:"type"`
	Label          string `json:"label"`
	DefaultBaseURL string `json:"default_base_url,omitempty"`
	DefaultModel   string `json:"default_model,omitempty"`
}

// Capabilities is everything a wizard needs before an agent exists: the
// provider types it can offer plus the skills and tools it can install.
type Capabilities struct {
	Providers []ProviderOption
	Skills    []Capability
	Tools     []Capability
}

// providerOptions is the preset list the wizard shows, in order. Only
// `openai` and `gemini` are valid wire types; the rest are shortcuts that keep
// the stored type but point the provider at another endpoint.
var providerOptions = []ProviderOption{
	{ID: "openai", Type: "openai", Label: "OpenAI (or compatible)"},
	{
		ID:             "deepseek",
		Type:           "openai",
		Label:          "DeepSeek",
		DefaultBaseURL: "https://api.deepseek.com",
		DefaultModel:   "deepseek-chat",
	},
	{ID: "gemini", Type: "gemini", Label: "Gemini"},
}

// BuiltinCapabilities lists the provider types, the embedded skills, and the
// built-in tools an agent can be created with.
func BuiltinCapabilities() Capabilities {
	skills := BuiltinSkillNames()
	skillDescriptions := SkillDescriptions()
	tools := BuiltinToolNames()
	toolDescriptions := ToolDescriptions()

	caps := Capabilities{
		Providers: make([]ProviderOption, 0, len(providerOptions)),
		Skills:    make([]Capability, 0, len(skills)),
		Tools:     make([]Capability, 0, len(tools)),
	}
	for _, option := range providerOptions {
		if option.DefaultBaseURL == "" {
			option.DefaultBaseURL = provider.DefaultBaseURL(option.Type)
		}
		if option.DefaultModel == "" {
			option.DefaultModel = provider.DefaultModel(option.Type)
		}
		caps.Providers = append(caps.Providers, option)
	}
	for _, name := range skills {
		caps.Skills = append(caps.Skills, Capability{Name: name, Description: skillDescriptions[name]})
	}
	for _, name := range tools {
		caps.Tools = append(caps.Tools, Capability{Name: name, Description: toolDescriptions[name]})
	}
	return caps
}

// SkillDescriptions returns the one-line description of every embedded skill,
// read from its SKILL.md frontmatter.
func SkillDescriptions() map[string]string {
	descriptions := make(map[string]string)
	for _, name := range BuiltinSkillNames() {
		data, err := myaaw.DefaultMyaawDir.ReadFile(path.Join(".myaaw", "skills", name, "SKILL.md"))
		if err != nil {
			continue
		}
		if description := FrontmatterDescription(string(data)); description != "" {
			descriptions[name] = description
		}
	}
	return descriptions
}

// ToolDescriptions returns the description of every built-in tool.
func ToolDescriptions() map[string]string {
	descriptions := make(map[string]string)
	for _, info := range tools.GetBuiltinTools() {
		if info.Description != "" {
			descriptions[info.Name] = info.Description
		}
	}
	return descriptions
}

// FrontmatterDescription extracts a single-line description from a SKILL.md
// frontmatter block.
func FrontmatterDescription(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(content, "---") {
		return ""
	}
	rest := strings.TrimPrefix(content, "---")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return ""
	}
	var meta struct {
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(rest[:end]), &meta); err != nil {
		return ""
	}
	return strings.Join(strings.Fields(meta.Description), " ")
}
