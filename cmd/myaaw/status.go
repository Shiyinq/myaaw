package main

import (
	"fmt"
	"myaaw/internal/cli/theme"
	"myaaw/internal/config"
	"myaaw/internal/daemon"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check service status",
	Long:  "Check the connection status of the application components and channel configurations.",
	Run: func(cmd *cobra.Command, args []string) {
		config.LoadBaseConfig()

		fmt.Println(theme.RenderPrimary("🔍 Myaaw Service Status"))
		fmt.Println(theme.RenderSecondary("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))

		srv, srvRunning, _ := checkService("myaaw-server")

		if srvRunning {
			fmt.Printf("%-14s %s\n", "Gateway", theme.RenderSuccess("✅ OPERATIONAL"))
		} else {
			fmt.Printf("%-14s %s\n", "Gateway", theme.RenderError("❌ OFFLINE"))
		}

		if srvRunning {
			fmt.Printf("  %-12s %s (PID: %d, Port: %s)\n", "Server", theme.RenderSuccess("✅ Running"), srv, config.PORT)
		} else {
			fmt.Printf("  %-12s %s\n", "Server", theme.RenderError("❌ Stopped"))
		}

		fmt.Println(theme.RenderSecondary("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))

		dbPath := os.Getenv("DB_PATH")
		if dbPath == "" {
			homeDir, _ := os.UserHomeDir()
			dbPath = filepath.Join(homeDir, ".myaaw", "myaaw.db")
		}

		if err := config.PingSQLite(); err != nil {
			fmt.Printf("  %-12s %s (%s)\n", "SQLite DB", theme.RenderError("[ERR] Offline"), err)
		} else {
			fmt.Printf("  %-12s %s (Path: %s)\n", "SQLite DB", theme.RenderSuccess("✅ Online"), dbPath)
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

func checkService(serviceName string) (int, bool, error) {
	dm, err := daemon.NewManager(serviceName)
	if err != nil {
		return 0, false, err
	}
	return dm.Status()
}
