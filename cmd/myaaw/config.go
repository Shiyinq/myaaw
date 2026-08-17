package main

import (
	"fmt"
	"os"
	"strings"

	"myaaw/internal/cli/theme"
	"myaaw/internal/config"

	"github.com/spf13/cobra"
)

var envKeys = []string{
	"PORT", "HOST", "MYAAW_BASE_URL", "ALLOWED_ORIGINS",
	"DB_PATH",
	"TAVILY_API_KEY",
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage and inspect Myaaw configuration",
}

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Validate environment variables and config files",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(theme.RenderSecondary("🔍 Loading configuration..."))
		config.LoadBaseConfig()
		
		cfg, err := config.LoadJSONConfigOnly()
		if err != nil {
			fmt.Println(theme.RenderError(fmt.Sprintf("Failed to load config.json: %v", err)))
			fmt.Println(theme.RenderMuted("You might want to run 'myaaw onboard' to generate one."))
			cfg = &config.Config{}
		}

		fmt.Println("\n" + theme.RenderPrimary("Checking Environment Variables (.env / OS):"))
		for _, key := range envKeys {
			val := os.Getenv(key)
			if val == "" {
				if key == "DB_PATH" {
					fmt.Printf("%-25s : %s (Default: ~/.myaaw/database/myaaw.db)\n", theme.RenderPrimary(key), theme.RenderMuted("[EMPTY]"))
				} else {
					fmt.Printf("%-25s : %s\n", theme.RenderPrimary(key), theme.RenderMuted("[EMPTY]"))
				}
			} else {
				fmt.Printf("%-25s : %s\n", theme.RenderPrimary(key), theme.RenderSuccess("[OK]"))
			}
		}

		fmt.Println("\n" + theme.RenderPrimary("Checking config.json Health:"))
		
		// 1. LLM
		if cfg.DefaultProvider == "" {
			fmt.Printf("%-25s : %s\n", theme.RenderPrimary("Default LLM Provider"), theme.RenderError("[MISSING]"))
		} else {
			if _, exists := cfg.Providers[cfg.DefaultProvider]; exists {
				fmt.Printf("%-25s : %s (Active: %s)\n", theme.RenderPrimary("Default LLM Provider"), theme.RenderSuccess("[OK]"), cfg.DefaultProvider)
			} else {
				fmt.Printf("%-25s : %s (Provider '%s' not found in providers block)\n", theme.RenderPrimary("Default LLM Provider"), theme.RenderError("[BROKEN REF]"), cfg.DefaultProvider)
			}
		}

		// 2. Transcriber
		if cfg.Transcriber.Provider == "" {
			fmt.Printf("%-25s : %s\n", theme.RenderPrimary("Transcriber"), theme.RenderMuted("[NOT SET] (Will fallback to default/OS)"))
		} else {
			if cfg.Transcriber.APIKey != "" {
				fmt.Printf("%-25s : %s (Active: %s)\n", theme.RenderPrimary("Transcriber"), theme.RenderSuccess("[OK]"), cfg.Transcriber.Provider)
			} else {
				fmt.Printf("%-25s : %s (Missing API Key for '%s')\n", theme.RenderPrimary("Transcriber"), theme.RenderError("[WARNING]"), cfg.Transcriber.Provider)
			}
		}

		// 3. Channels
		if cfg.Channels.Telegram != nil && cfg.Channels.Telegram.Active {
			if cfg.Channels.Telegram.Token != "" {
				fmt.Printf("%-25s : %s (Mode: %s)\n", theme.RenderPrimary("Channel: Telegram"), theme.RenderSuccess("[OK]"), cfg.Channels.Telegram.Mode)
			} else {
				fmt.Printf("%-25s : %s\n", theme.RenderPrimary("Channel: Telegram"), theme.RenderError("[MISSING TOKEN]"))
			}
		} else {
			fmt.Printf("%-25s : %s\n", theme.RenderPrimary("Channel: Telegram"), theme.RenderMuted("[INACTIVE]"))
		}

		if cfg.Channels.Discord != nil && cfg.Channels.Discord.Active {
			if cfg.Channels.Discord.Token != "" {
				fmt.Printf("%-25s : %s\n", theme.RenderPrimary("Channel: Discord"), theme.RenderSuccess("[OK]"))
			} else {
				fmt.Printf("%-25s : %s\n", theme.RenderPrimary("Channel: Discord"), theme.RenderError("[MISSING TOKEN]"))
			}
		} else {
			fmt.Printf("%-25s : %s\n", theme.RenderPrimary("Channel: Discord"), theme.RenderMuted("[INACTIVE]"))
		}

		fmt.Println("\n✨ System health check completed!")
	},
}

var dumpCmd = &cobra.Command{
	Use:   "dump",
	Short: "Print current configuration (Secrets Masked)",
	Run: func(cmd *cobra.Command, args []string) {
		config.LoadBaseConfig()
		
		fmt.Println(theme.RenderPrimary("--- OS / Environment Variables ---"))
		for _, key := range envKeys {
			val := os.Getenv(key)

			isSensitive := strings.Contains(key, "KEY") ||
				strings.Contains(key, "TOKEN") ||
				strings.Contains(key, "PASSWORD") ||
				strings.Contains(key, "SECRET")

			if val == "" {
				if key == "DB_PATH" {
					fmt.Printf("%-25s: <EMPTY> (Default: ~/.myaaw/database/myaaw.db)\n", key)
				} else {
					fmt.Printf("%-25s: <EMPTY>\n", key)
				}
			} else if isSensitive {
				fmt.Printf("%-25s: ******** (Masked)\n", key)
			} else {
				fmt.Printf("%-25s: %s\n", key, val)
			}
		}

		fmt.Println("\n" + theme.RenderPrimary("--- Config.json Values ---"))
		cfg, err := config.LoadJSONConfigOnly()
		if err != nil || cfg == nil {
			fmt.Println(theme.RenderError("No config.json found."))
			return
		}

		fmt.Printf("DefaultProvider: %s\n", cfg.DefaultProvider)
		for name, p := range cfg.Providers {
			fmt.Printf("\n[Provider: %s]\n", name)
			fmt.Printf("  Type:         %s\n", p.Type)
			fmt.Printf("  BaseURL:      %s\n", p.BaseURL)
			fmt.Printf("  DefaultModel: %s\n", p.DefaultModel)
			if p.APIKey != "" {
				fmt.Printf("  APIKey:       ******** (Masked)\n")
			}
		}

		fmt.Println("\n[Bot]")
		fmt.Printf("  Type:              %s\n", cfg.Bot.Type)
		if cfg.Bot.MaxIterations != nil {
			fmt.Printf("  MaxIterations:     %d\n", *cfg.Bot.MaxIterations)
		}
		if cfg.Bot.WarningIterations != nil {
			fmt.Printf("  WarningIterations: %d\n", *cfg.Bot.WarningIterations)
		}

		fmt.Println("\n[Transcriber]")
		fmt.Printf("  Provider:     %s\n", cfg.Transcriber.Provider)
		if cfg.Transcriber.APIKey != "" {
			fmt.Printf("  APIKey:       ******** (Masked)\n")
		}

		fmt.Println("\n[Channels]")
		if cfg.Channels.Telegram != nil {
			fmt.Printf("  Telegram.Active: %v\n", cfg.Channels.Telegram.Active)
			fmt.Printf("  Telegram.Mode:   %s\n", cfg.Channels.Telegram.Mode)
			if cfg.Channels.Telegram.Token != "" {
				fmt.Printf("  Telegram.Token:  ******** (Masked)\n")
			}
		}
		if cfg.Channels.Discord != nil {
			fmt.Printf("  Discord.Active:  %v\n", cfg.Channels.Discord.Active)
			if cfg.Channels.Discord.Token != "" {
				fmt.Printf("  Discord.Token:   ******** (Masked)\n")
			}
		}
	},
}

func init() {
	configCmd.AddCommand(checkCmd)
	configCmd.AddCommand(dumpCmd)
}
