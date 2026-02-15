package main

import (
	"fmt"
	"myaaw/internal/config"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check service status",
	Long:  "Check the connection status of all services (MongoDB, Redis, RabbitMQ) and channel configurations.",
	Run: func(cmd *cobra.Command, args []string) {
		config.LoadBaseConfig()

		fmt.Println("🔍 Myaaw Service Status")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// MongoDB
		fmt.Print("  MongoDB     ")
		if err := config.PingMongoDB(); err != nil {
			fmt.Printf("❌ Offline  (%s)\n", err)
		} else {
			fmt.Println("✅ Online")
		}

		// Redis
		fmt.Print("  Redis       ")
		if err := config.PingRedis(); err != nil {
			fmt.Printf("❌ Offline  (%s)\n", err)
		} else {
			fmt.Println("✅ Online")
		}

		// RabbitMQ
		fmt.Print("  RabbitMQ    ")
		if err := config.PingRabbitMQ(); err != nil {
			fmt.Printf("❌ Offline  (%s)\n", err)
		} else {
			fmt.Println("✅ Online")
		}

		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Channels
		fmt.Println("📡 Channels")
		if config.TelegramBotToken != "" {
			fmt.Println("  Telegram    ✅ Configured")
		} else {
			fmt.Println("  Telegram    ❌ Not configured")
		}

		if config.DiscordBotToken != "" {
			fmt.Println("  Discord     ✅ Configured")
		} else {
			fmt.Println("  Discord     ❌ Not configured")
		}

		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// LLM Provider
		fmt.Println("🤖 LLM Provider")
		if config.LLMProviderName != "" {
			fmt.Printf("  Provider    %s\n", config.LLMProviderName)
		} else {
			fmt.Println("  Provider    ❌ Not configured")
		}

		if config.StreamResponse {
			fmt.Println("  Streaming   ✅ Enabled")
		} else {
			fmt.Println("  Streaming   ❌ Disabled")
		}

		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	},
}
