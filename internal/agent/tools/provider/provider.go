package provider

import (
	"encoding/json"
	"fmt"
	"myaaw/internal/agent/tools"
	"myaaw/internal/config"
	baseProvider "myaaw/internal/provider"
	"strings"
)

func init() {
	tools.Register("provider", NewProviderTool())
}

type ProviderTool struct{}

func NewProviderTool() *ProviderTool {
	return &ProviderTool{}
}

func (t *ProviderTool) CallTool(arguments string, ctx *tools.ToolsContext) string {
	var args map[string]interface{}
	err := json.Unmarshal([]byte(arguments), &args)
	if err != nil {
		return fmt.Sprintf("Error parsing arguments: %v", err)
	}

	action, ok := args["action"].(string)
	if !ok {
		return "Error: action argument is required. Available actions: list, set_default, set_model, fetch_models"
	}

	cfg, err := config.LoadJSONConfigOnly()
	if err != nil || cfg == nil {
		return "Error: Failed to load configuration."
	}

	switch action {
	case "list":
		var sb strings.Builder
		sb.WriteString("Configured Providers:\n")
		for name, p := range cfg.Providers {
			marker := " "
			if name == cfg.DefaultProvider {
				marker = "*"
			}
			sb.WriteString(fmt.Sprintf("%s %-15s [Type: %-10s | Model: %s]\n", marker, name, p.Type, p.DefaultModel))
		}
		if len(cfg.Providers) == 0 {
			return "No providers configured."
		}
		return sb.String()

	case "set_default":
		name, _ := args["name"].(string)
		if name == "" {
			return "Error: name is required for set_default"
		}
		if _, exists := cfg.Providers[name]; !exists {
			return fmt.Sprintf("Error: Provider '%s' not found.", name)
		}
		cfg.DefaultProvider = name
		config.SaveConfig(cfg)
		return fmt.Sprintf("Success: Default provider set to '%s'", name)

	case "set_model":
		name, _ := args["name"].(string)
		model, _ := args["model"].(string)
		if name == "" || model == "" {
			return "Error: name and model are required for set_model"
		}
		pConfig, exists := cfg.Providers[name]
		if !exists {
			return fmt.Sprintf("Error: Provider '%s' not found.", name)
		}
		pConfig.DefaultModel = model
		cfg.Providers[name] = pConfig
		config.SaveConfig(cfg)
		return fmt.Sprintf("Success: Model for provider '%s' updated to '%s'", name, model)

	case "fetch_models":
		name, _ := args["name"].(string)
		if name == "" {
			return "Error: name is required for fetch_models"
		}
		pConfig, exists := cfg.Providers[name]
		if !exists {
			return fmt.Sprintf("Error: Provider '%s' not found.", name)
		}

		// Save global base url state
		oldBaseURL := config.LLMProviderBaseURL
		config.LLMProviderBaseURL = pConfig.BaseURL
		defer func() { config.LLMProviderBaseURL = oldBaseURL }()

		llm, err := baseProvider.CreateLLMProvider(pConfig.Type, pConfig.APIKey)
		if err != nil {
			return fmt.Sprintf("Error: Failed to create provider API client: %v", err)
		}
		models, err := llm.Models()
		if err != nil {
			return fmt.Sprintf("Error: Failed to fetch models: %v", err)
		}
		return fmt.Sprintf("Available models for %s:\n%s", name, strings.Join(models, "\n"))

	default:
		return fmt.Sprintf("Error: Unknown action '%s'", action)
	}
}
