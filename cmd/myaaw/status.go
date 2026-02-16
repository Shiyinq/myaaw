package main

import (
	"fmt"
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

		fmt.Println("🔍 Myaaw Service Status")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		srv, srvRunning, _ := checkService("myaaw-server")
		cons, consRunning, _ := checkService("myaaw-consumer")

		if srvRunning && consRunning {
			fmt.Printf("%-14s ✅ OPERATIONAL\n", "Gateway")
		} else if !srvRunning && !consRunning {
			fmt.Printf("%-14s ❌ OFFLINE\n", "Gateway")
		} else {
			if srvRunning {
				fmt.Printf("%-14s ⚠️  PARTIAL (Server Only)\n", "Gateway")
			} else {
				fmt.Printf("%-14s ⚠️  PARTIAL (Consumer Only)\n", "Gateway")
			}
		}

		if srvRunning {
			fmt.Printf("  %-12s ✅ Running (PID: %d, Port: %s)\n", "Server", srv, config.PORT)
		} else {
			fmt.Printf("  %-12s ❌ Stopped\n", "Server")
		}

		if consRunning {
			fmt.Printf("  %-12s ✅ Running (PID: %d)\n", "Consumer", cons)
		} else {
			fmt.Printf("  %-12s ❌ Stopped\n", "Consumer")
		}

		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		mongoPort := getPortFromURL(os.Getenv("MONGODB_URI"), "27017")
		if err := config.PingMongoDB(); err != nil {
			fmt.Printf("  %-12s [ERR] Offline (%s)\n", "MongoDB", err)
		} else {
			fmt.Printf("  %-12s ✅ Online (Port: %s)\n", "MongoDB", mongoPort)
		}

		redisPort := getPortFromURL(os.Getenv("REDIS_URL"), "6379")
		if err := config.PingRedis(); err != nil {
			fmt.Printf("  %-12s ❌ Offline (%s)\n", "Redis", err)
		} else {
			fmt.Printf("  %-12s ✅ Online (Port: %s)\n", "Redis", redisPort)
		}

		rabbitPort := getPortFromURL(os.Getenv("RABBITMQ_URL"), "5672")
		if err := config.PingRabbitMQ(); err != nil {
			fmt.Printf("  %-12s ❌ Offline (%s)\n", "RabbitMQ", err)
		} else {
			fmt.Printf("  %-12s ✅ Online (Port: %s)\n", "RabbitMQ", rabbitPort)
		}

		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		fmt.Println("Channels")
		if config.TelegramBotToken != "" {
			fmt.Printf("  %-12s ✅ Configured (%s)\n", "Telegram", config.TelegramMode)
		} else {
			fmt.Printf("  %-12s ❌ Not configured\n", "Telegram")
		}

		if config.DiscordBotToken != "" {
			fmt.Printf("  %-12s ✅ Configured\n", "Discord")
		} else {
			fmt.Printf("  %-12s ❌ Not configured\n", "Discord")
		}

		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		fmt.Println("Heartbeat")
		if config.Heartbeat.Active {
			fmt.Printf("  %-12s ✅ Active (Every %s)\n", "Status", config.Heartbeat.Every)
			fmt.Printf("  %-12s To: %s (%s)\n", "Channel", config.Heartbeat.To, config.Heartbeat.Channel)
		} else {
			fmt.Printf("  %-12s ❌ Disabled\n", "Status")
		}

		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		fmt.Println("LLM Provider")
		if config.LLMProviderName != "" {
			fmt.Printf("  %-12s %s\n", "Provider", config.LLMProviderName)
		} else {
			fmt.Printf("  %-12s ❌ Not configured\n", "Provider")
		}

		if config.StreamResponse {
			fmt.Printf("  %-12s ✅ Enabled\n", "Streaming")
		} else {
			fmt.Printf("  %-12s ❌ Disabled\n", "Streaming")
		}

		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
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
