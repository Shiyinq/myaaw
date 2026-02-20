package main

import (
	"fmt"
	"myaaw/internal/cli/theme"
	"myaaw/internal/config"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check service status",
	Long:  "Check the connection status of all services (MongoDB, Redis, RabbitMQ) and channel configurations.",
	Run: func(cmd *cobra.Command, args []string) {
		config.LoadBaseConfig()

		fmt.Println(theme.RenderPrimary("🔍 Myaaw Service Status"))
		fmt.Println(theme.RenderSecondary("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))

		srv, srvRunning, _ := checkService("myaaw-server")
		cons, consRunning, _ := checkService("myaaw-consumer")

		if srvRunning && consRunning {
			fmt.Printf("%-14s %s\n", "Gateway", theme.RenderSuccess("✅ OPERATIONAL"))
		} else if !srvRunning && !consRunning {
			fmt.Printf("%-14s %s\n", "Gateway", theme.RenderError("❌ OFFLINE"))
		} else {
			if srvRunning {
				fmt.Printf("%-14s %s\n", "Gateway", theme.RenderError("⚠️  PARTIAL (Server Only)"))
			} else {
				fmt.Printf("%-14s %s\n", "Gateway", theme.RenderError("⚠️  PARTIAL (Consumer Only)"))
			}
		}

		if srvRunning {
			fmt.Printf("  %-12s %s (PID: %d, Port: %s)\n", "Server", theme.RenderSuccess("✅ Running"), srv, config.PORT)
		} else {
			fmt.Printf("  %-12s %s\n", "Server", theme.RenderError("❌ Stopped"))
		}

		if consRunning {
			fmt.Printf("  %-12s %s (PID: %d)\n", "Consumer", theme.RenderSuccess("✅ Running"), cons)
		} else {
			fmt.Printf("  %-12s %s\n", "Consumer", theme.RenderError("❌ Stopped"))
		}

		fmt.Println(theme.RenderSecondary("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))

		mongoPort := getPortFromURL(os.Getenv("MONGODB_URI"), "27017")
		if err := config.PingMongoDB(); err != nil {
			fmt.Printf("  %-12s %s (%s)\n", "MongoDB", theme.RenderError("[ERR] Offline"), err)
		} else {
			fmt.Printf("  %-12s %s (Port: %s)\n", "MongoDB", theme.RenderSuccess("✅ Online"), mongoPort)
		}

		redisPort := getPortFromURL(os.Getenv("REDIS_URL"), "6379")
		if err := config.PingRedis(); err != nil {
			fmt.Printf("  %-12s %s (%s)\n", "Redis", theme.RenderError("❌ Offline"), err)
		} else {
			fmt.Printf("  %-12s %s (Port: %s)\n", "Redis", theme.RenderSuccess("✅ Online"), redisPort)
		}

		rabbitPort := getPortFromURL(os.Getenv("RABBITMQ_URL"), "5672")
		if err := config.PingRabbitMQ(); err != nil {
			fmt.Printf("  %-12s %s (%s)\n", "RabbitMQ", theme.RenderError("❌ Offline"), err)
		} else {
			fmt.Printf("  %-12s %s (Port: %s)\n", "RabbitMQ", theme.RenderSuccess("✅ Online"), rabbitPort)
		}

		fmt.Println(theme.RenderSecondary("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))

		fmt.Println(theme.RenderPrimary("Channels"))
		if config.TelegramBotToken != "" {
			fmt.Printf("  %-12s %s (%s)\n", "Telegram", theme.RenderSuccess("✅ Configured"), config.TelegramMode)
		} else {
			fmt.Printf("  %-12s %s\n", "Telegram", theme.RenderError("❌ Not configured"))
		}

		if config.DiscordBotToken != "" {
			fmt.Printf("  %-12s %s\n", "Discord", theme.RenderSuccess("✅ Configured"))
		} else {
			fmt.Printf("  %-12s %s\n", "Discord", theme.RenderError("❌ Not configured"))
		}

		fmt.Println(theme.RenderSecondary("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))

		fmt.Println(theme.RenderPrimary("Heartbeat"))
		if config.Heartbeat.Active {
			fmt.Printf("  %-12s %s (Every %s)\n", "Status", theme.RenderSuccess("✅ Active"), config.Heartbeat.Every)
			fmt.Printf("  %-12s %s: %s (%s)\n", "Channel", theme.RenderSecondary("To"), config.Heartbeat.To, config.Heartbeat.Channel)
		} else {
			fmt.Printf("  %-12s %s\n", "Status", theme.RenderError("❌ Disabled"))
		}

		fmt.Println(theme.RenderSecondary("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))

		fmt.Println(theme.RenderPrimary("LLM Provider"))
		if config.LLMProviderName != "" {
			fmt.Printf("  %-12s %s\n", "Provider", theme.RenderPrimary(config.LLMProviderName))
		} else {
			fmt.Printf("  %-12s %s\n", "Provider", theme.RenderError("❌ Not configured"))
		}

		if config.StreamResponse {
			fmt.Printf("  %-12s %s\n", "Streaming", theme.RenderSuccess("✅ Enabled"))
		} else {
			fmt.Printf("  %-12s %s\n", "Streaming", theme.RenderError("❌ Disabled"))
		}

		fmt.Println(theme.RenderSecondary("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))
	},
}

func getPortFromURL(uri string, defaultPort string) string {
	if uri == "" {
		return defaultPort
	}
	if !strings.Contains(uri, "://") {
	}

	u, err := url.Parse(uri)
	if err != nil {
		return defaultPort
	}
	port := u.Port()
	if port == "" {
		return defaultPort
	}
	return port
}
