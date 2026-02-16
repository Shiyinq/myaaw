package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"myaaw/internal/config"

	"github.com/spf13/cobra"
)

var envKeys = []string{
	"PORT", "HOST", "MYAAW_BASE_URL", "ALLOWED_ORIGINS",
	"NGROK_ACTIVE", "NGROK_AUTHTOKEN",
	"DB_NAME", "MONGODB_URI",
	"REDIS_URL",
	"QUEUE_NAME", "RABBITMQ_URL",
	"LLM_PROVIDER_NAME", "LLM_PROVIDER_API_KEY", "LLM_PROVIDER_BASE_URL",
	"STREAM_RESPONSE",
	"TTS_PROVIDER_NAME", "TTS_PROVIDER_API_KEY",
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
		fmt.Println("🔍 Loading configuration...")
		config.LoadBaseConfig()

		fmt.Println("\nChecking Environment Variables (.env / OS):")
		missingCount := 0
		for _, key := range envKeys {
			val := os.Getenv(key)
			if val == "" {
				if key == "TTS_PROVIDER_NAME" && config.TTSProviderName != "" {
					fmt.Printf("✅ %-25s : [OK] (Default: %s)\n", key, config.TTSProviderName)
					continue
				}
				fmt.Printf("❌ %-25s : [MISSING]\n", key)
				missingCount++
			} else {
				fmt.Printf("✅ %-25s : [OK]\n", key)
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
			fmt.Printf("ℹ️  %-25s : Not Found (Optional)\n", "~/.myaaw/config.json")
		} else {
			fmt.Printf("✅ %-25s : [FOUND]\n", "~/.myaaw/config.json")
			// We could validate JSON content here if desired
		}

		if missingCount > 0 {
			fmt.Println("\n⚠️  Some recommended environment variables are missing.")
		} else {
			fmt.Println("\n✨ Configuration looks good!")
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
		fmt.Printf("BotType:  %s\n", config.BotType)
		fmt.Printf("OwnerIDs: %v\n", config.OwnerIDs)
	},
}

func init() {
	configCmd.AddCommand(checkCmd)
	configCmd.AddCommand(dumpCmd)
	rootCmd.AddCommand(configCmd)
}
