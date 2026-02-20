package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"myaaw/internal/cli/theme"
	"myaaw/internal/config"

	"github.com/spf13/cobra"
)

var envKeys = []string{
	"PORT", "HOST", "MYAAW_BASE_URL", "ALLOWED_ORIGINS",
	"DB_NAME", "MONGODB_URI",
	"REDIS_URL",
	"QUEUE_NAME", "RABBITMQ_URL",
	"LLM_PROVIDER_NAME", "LLM_PROVIDER_API_KEY", "LLM_PROVIDER_BASE_URL",
	"STREAM_RESPONSE",
	"TRANSCRIBER_PROVIDER_NAME", "TRANSCRIBER_API_KEY",
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

		fmt.Println("\n" + theme.RenderPrimary("Checking Environment Variables (.env / OS):"))
		missingCount := 0
		for _, key := range envKeys {
			val := os.Getenv(key)
			if val == "" {
				if key == "TRANSCRIBER_PROVIDER_NAME" && config.TranscriberProviderName != "" {
					fmt.Printf("%-25s : %s (Default: %s)\n", theme.RenderPrimary(key), theme.RenderSuccess("[OK]"), config.TranscriberProviderName)
					continue
				}
				fmt.Printf("%-25s : %s\n", theme.RenderPrimary(key), theme.RenderError("[MISSING]"))
				missingCount++
			} else {
				fmt.Printf("%-25s : %s\n", theme.RenderPrimary(key), theme.RenderSuccess("[OK]"))
			}
		}

		fmt.Println("\nChecking config.json:")
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Printf("❌ Failed to get Home Dir: %v\n", err)
			return
		}
		configPath := filepath.Join(homeDir, ".myaaw", "config.json")
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			fmt.Printf("%-25s : %s (Optional)\n", theme.RenderPrimary("~/.myaaw/config.json"), theme.RenderMuted("Not Found"))
		} else {
			fmt.Printf("%-25s : %s\n", theme.RenderPrimary("~/.myaaw/config.json"), theme.RenderSuccess("[FOUND]"))
			// We could validate JSON content here if desired
		}

		if missingCount > 0 {
			fmt.Println("\n⚠️  Some recommended environment variables are missing.")
		} else {
			fmt.Println("\n✨ System environment looks good!")
		}

		fmt.Println("\n" + theme.RenderPrimary("Telegram / Ngrok Status:"))
		fmt.Printf("🔹 Telegram Mode : %s\n", config.TelegramMode)
		if config.TelegramMode == "webhook" {
			if config.NgrokActive == "true" {
				fmt.Printf("%-16s : %s (Token: %s)\n", theme.RenderPrimary("Ngrok Active"), theme.RenderSuccess("[YES]"), config.NgrokAuthToken)
			} else {
				fmt.Printf("%-16s : %s (Webhook will fail if not publicly reachable)\n", theme.RenderPrimary("Ngrok Active"), theme.RenderError("[NO]"))
			}
		} else {
			fmt.Printf("%-16s : %s (No Ngrok required)\n", theme.RenderPrimary("Long Polling"), theme.RenderSuccess("[ACTIVE]"))
		}
	},
}

var dumpCmd = &cobra.Command{
	Use:   "dump",
	Short: "Print current configuration (Secrets Masked)",
	Run: func(cmd *cobra.Command, args []string) {
		config.LoadBaseConfig()

		fmt.Println("--- Current Configuration ---")
		for _, key := range envKeys {
			val := os.Getenv(key)

			isSensitive := strings.Contains(key, "KEY") ||
				strings.Contains(key, "TOKEN") ||
				strings.Contains(key, "PASSWORD") ||
				strings.Contains(key, "SECRET") ||
				strings.Contains(key, "URL") ||
				strings.Contains(key, "URI")

			if val == "" {
				fmt.Printf("%-25s: <EMPTY>\n", key)
			} else if isSensitive {
				fmt.Printf("%-25s: ******** (Masked)\n", key)
			} else {
				fmt.Printf("%-25s: %s\n", key, val)
			}
		}

		fmt.Println("\n--- Internals ---")
		fmt.Printf("BotType:       %s\n", config.BotType)
		fmt.Printf("OwnerIDs:      %v\n", config.OwnerIDs)
		fmt.Printf("TelegramMode:  %s\n", config.TelegramMode)
		fmt.Printf("NgrokActive:   %s\n", config.NgrokActive)
		if config.NgrokActive == "true" {
			fmt.Printf("NgrokToken:    ******** (Masked)\n")
		}
	},
}

func init() {
	configCmd.AddCommand(checkCmd)
	configCmd.AddCommand(dumpCmd)
}
