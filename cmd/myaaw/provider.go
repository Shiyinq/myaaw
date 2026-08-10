package main

import (
	"fmt"
	"strings"

	"myaaw/internal/cli/theme"
	"myaaw/internal/config"
	"myaaw/internal/provider"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

var providerCmd = &cobra.Command{
	Use:   "provider",
	Short: "Manage LLM Provider integrations",
}

var providerCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Interactively create a new LLM provider integration",
	Run: func(cmd *cobra.Command, args []string) {
		var pType, pName, pBaseURL, pAPIKey string

		fmt.Println(theme.RenderPrimary("--- Create LLM Provider Integration ---"))
		
		err := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Provider Type").
					Options(
						huh.NewOption("OpenAI (or OpenAI Compatible)", "openai"),
						huh.NewOption("Gemini", "gemini"),
						huh.NewOption("Groq", "groq"),
						huh.NewOption("Ollama", "ollama"),
					).
					Value(&pType),

				huh.NewInput().
					Title("Custom Name (e.g., deepseek, local-ollama)").
					Value(&pName).
					Validate(func(v string) error {
						if strings.TrimSpace(v) == "" {
							return fmt.Errorf("name is required")
						}
						return nil
					}),

				huh.NewInput().
					Title("Base URL (Leave empty for default API)").
					Value(&pBaseURL),

				huh.NewInput().
					Title("API Key").
					EchoMode(huh.EchoModePassword).
					Value(&pAPIKey),
			),
		).Run()

		if err != nil {
			fmt.Println(theme.RenderError("Setup cancelled."))
			return
		}

		pName = strings.TrimSpace(pName)
		pBaseURL = strings.TrimSpace(pBaseURL)
		pAPIKey = strings.TrimSpace(pAPIKey)

		fmt.Println(theme.RenderSecondary("\nFetching available models..."))
		
		// Temporarily override config to initialize provider
		config.LLMProviderBaseURL = pBaseURL
		llm, err := provider.CreateLLMProvider(pType, pAPIKey)
		if err != nil {
			fmt.Println(theme.RenderError(fmt.Sprintf("Failed to create provider: %v", err)))
			return
		}

		models, err := llm.Models()
		if err != nil {
			fmt.Println(theme.RenderError(fmt.Sprintf("Failed to fetch models: %v", err)))
			fmt.Print("Enter default model manually: ")
		}

		var defaultModel string
		if len(models) > 0 {
			options := []huh.Option[string]{}
			for _, m := range models {
				options = append(options, huh.NewOption(m, m))
			}

			err = huh.NewSelect[string]().
				Title("Select default model").
				Options(options...).
				Value(&defaultModel).
				Run()

			if err != nil {
				fmt.Println(theme.RenderError("Setup cancelled."))
				return
			}
			fmt.Printf("Selected model: %s\n", defaultModel)
		} else {
			err = huh.NewInput().
				Title("Enter default model manually").
				Value(&defaultModel).
				Run()

			if err != nil {
				fmt.Println(theme.RenderError("Setup cancelled."))
				return
			}
			defaultModel = strings.TrimSpace(defaultModel)
		}

		// Load existing config
		cfg, err := config.LoadJSONConfigOnly()
		if cfg == nil {
			cfg = &config.Config{}
		}

		if cfg.Providers == nil {
			cfg.Providers = make(map[string]config.ProviderConfig)
		}

		cfg.Providers[pName] = config.ProviderConfig{
			Type:         pType,
			BaseURL:      pBaseURL,
			APIKey:       pAPIKey,
			DefaultModel: defaultModel,
		}

		// If this is the first provider, set as default
		if cfg.DefaultProvider == "" {
			cfg.DefaultProvider = pName
		}

		err = config.SaveConfig(cfg)
		if err != nil {
			fmt.Println(theme.RenderError(fmt.Sprintf("Failed to save config: %v", err)))
			return
		}

		fmt.Println(theme.RenderSuccess(fmt.Sprintf("✨ Provider '%s' created and saved successfully!", pName)))
	},
}

var providerListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured LLM providers",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, _ := config.LoadJSONConfigOnly()
		if cfg == nil || len(cfg.Providers) == 0 {
			fmt.Println("No providers configured.")
			return
		}

		fmt.Println(theme.RenderPrimary("Configured Providers:"))
		for name, p := range cfg.Providers {
			marker := " "
			if name == cfg.DefaultProvider {
				marker = "*"
			}
			fmt.Printf("%s %-15s [Type: %-10s | Model: %s]\n", marker, name, p.Type, p.DefaultModel)
		}
	},
}

var providerDefaultCmd = &cobra.Command{
	Use:   "default [name]",
	Short: "Set the default LLM provider",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, _ := config.LoadJSONConfigOnly()
		if cfg == nil || len(cfg.Providers) == 0 {
			fmt.Println("No providers configured.")
			return
		}

		var name string
		if len(args) == 0 {
			// Interactive mode using huh
			options := []huh.Option[string]{}
			for n := range cfg.Providers {
				label := n
				if n == cfg.DefaultProvider {
					label = n + " (Current Default)"
				}
				options = append(options, huh.NewOption(label, n))
			}

			err := huh.NewSelect[string]().
				Title("Select provider to set as default").
				Options(options...).
				Value(&name).
				Run()

			if err != nil {
				fmt.Println(theme.RenderError("Cancelled."))
				return
			}
		} else {
			name = args[0]
		}

		if _, exists := cfg.Providers[name]; !exists {
			fmt.Println(theme.RenderError(fmt.Sprintf("Provider '%s' not found.", name)))
			return
		}

		cfg.DefaultProvider = name
		config.SaveConfig(cfg)
		fmt.Println(theme.RenderSuccess(fmt.Sprintf("✨ Default provider set to '%s'", name)))
	},
}

var providerDeleteCmd = &cobra.Command{
	Use:   "delete [name]",
	Short: "Delete an LLM provider integration",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, _ := config.LoadJSONConfigOnly()
		if cfg == nil || len(cfg.Providers) == 0 {
			fmt.Println("No providers configured.")
			return
		}

		var name string
		if len(args) == 0 {
			// Interactive mode using huh
			options := []huh.Option[string]{}
			for n := range cfg.Providers {
				label := n
				if n == cfg.DefaultProvider {
					label = n + " (Current Default)"
				}
				options = append(options, huh.NewOption(label, n))
			}

			err := huh.NewSelect[string]().
				Title("Select provider to delete").
				Options(options...).
				Value(&name).
				Run()

			if err != nil {
				fmt.Println(theme.RenderError("Cancelled."))
				return
			}
		} else {
			name = args[0]
		}

		if _, exists := cfg.Providers[name]; !exists {
			fmt.Println(theme.RenderError(fmt.Sprintf("Provider '%s' not found.", name)))
			return
		}

		delete(cfg.Providers, name)
		
		if cfg.DefaultProvider == name {
			cfg.DefaultProvider = ""
			// Fallback to another one if available
			for k := range cfg.Providers {
				cfg.DefaultProvider = k
				break
			}
		}

		config.SaveConfig(cfg)
		fmt.Println(theme.RenderSuccess(fmt.Sprintf("🗑️  Provider '%s' deleted.", name)))
	},
}

var providerSetModelCmd = &cobra.Command{
	Use:   "set-model [name]",
	Short: "Change the default model for an LLM provider integration",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, _ := config.LoadJSONConfigOnly()
		if cfg == nil || len(cfg.Providers) == 0 {
			fmt.Println("No providers configured.")
			return
		}

		var name string
		if len(args) == 0 {
			options := []huh.Option[string]{}
			for n := range cfg.Providers {
				label := n
				if n == cfg.DefaultProvider {
					label = n + " (Active)"
				}
				options = append(options, huh.NewOption(label, n))
			}

			err := huh.NewSelect[string]().
				Title("Select provider to update model").
				Options(options...).
				Value(&name).
				Run()

			if err != nil {
				fmt.Println(theme.RenderError("Cancelled."))
				return
			}
		} else {
			name = args[0]
		}

		pConfig, exists := cfg.Providers[name]
		if !exists {
			fmt.Println(theme.RenderError(fmt.Sprintf("Provider '%s' not found.", name)))
			return
		}

		fmt.Println(theme.RenderSecondary(fmt.Sprintf("\nFetching available models for %s...", name)))
		
		// Temporarily override config to initialize provider
		config.LLMProviderBaseURL = pConfig.BaseURL
		llm, err := provider.CreateLLMProvider(pConfig.Type, pConfig.APIKey)
		if err != nil {
			fmt.Println(theme.RenderError(fmt.Sprintf("Failed to create provider: %v", err)))
			return
		}

		models, err := llm.Models()
		if err != nil {
			fmt.Println(theme.RenderError(fmt.Sprintf("Failed to fetch models: %v", err)))
			fmt.Print("Enter default model manually: ")
		}

		var defaultModel string
		if len(models) > 0 {
			options := []huh.Option[string]{}
			for _, m := range models {
				label := m
				if m == pConfig.DefaultModel {
					label = m + " (Current)"
				}
				options = append(options, huh.NewOption(label, m))
			}

			err = huh.NewSelect[string]().
				Title(fmt.Sprintf("Select default model for %s", name)).
				Options(options...).
				Value(&defaultModel).
				Run()

			if err != nil {
				fmt.Println(theme.RenderError("Setup cancelled."))
				return
			}
		} else {
			err = huh.NewInput().
				Title(fmt.Sprintf("Enter default model manually for %s", name)).
				Value(&defaultModel).
				Run()

			if err != nil {
				fmt.Println(theme.RenderError("Setup cancelled."))
				return
			}
			defaultModel = strings.TrimSpace(defaultModel)
		}

		pConfig.DefaultModel = defaultModel
		cfg.Providers[name] = pConfig

		err = config.SaveConfig(cfg)
		if err != nil {
			fmt.Println(theme.RenderError(fmt.Sprintf("Failed to save config: %v", err)))
			return
		}

		fmt.Println(theme.RenderSuccess(fmt.Sprintf("✨ Model for provider '%s' updated to '%s'", name, defaultModel)))
	},
}

func init() {
	providerCmd.AddCommand(providerCreateCmd)
	providerCmd.AddCommand(providerListCmd)
	providerCmd.AddCommand(providerDefaultCmd)
	providerCmd.AddCommand(providerSetModelCmd)
	providerCmd.AddCommand(providerDeleteCmd)
}
