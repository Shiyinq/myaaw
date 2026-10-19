package main

import (
	"fmt"
	"log"
	"time"

	"myaaw/internal/cli/theme"
	"myaaw/internal/daemon"
	"myaaw/internal/services/controlplane"

	"github.com/spf13/cobra"
)

var apiPort string

var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Run the unified control plane API",
	Long:  "Run the single API gateway used by web, desktop, and mobile clients. Chat is proxied to the selected agent server.",
	Run: func(cmd *cobra.Command, args []string) {
		runAPI()
	},
}

var apiRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the API in the foreground",
	Run: func(cmd *cobra.Command, args []string) {
		runAPI()
	},
}

var apiStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the API as a background daemon",
	Run: func(cmd *cobra.Command, args []string) {
		manager, err := daemon.NewGlobalManager(apiDaemonName())
		if err != nil {
			log.Fatalf("Error: %v", err)
		}
		if err := manager.Start(apiRunArgs()); err != nil {
			log.Fatalf("Failed to start API: %v", err)
		}
	},
}

var apiStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the API daemon",
	Run: func(cmd *cobra.Command, args []string) {
		manager, err := daemon.NewGlobalManager(apiDaemonName())
		if err != nil {
			log.Fatalf("Error: %v", err)
		}
		if err := manager.Stop(); err != nil {
			log.Fatalf("Failed to stop API: %v", err)
		}
	},
}

var apiRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the API daemon",
	Run: func(cmd *cobra.Command, args []string) {
		manager, err := daemon.NewGlobalManager(apiDaemonName())
		if err != nil {
			log.Fatalf("Error: %v", err)
		}

		fmt.Println(theme.RenderSecondary("🔄 Stopping API..."))
		if err := manager.Stop(); err != nil {
			fmt.Printf("%s: %v\n", theme.RenderError("⚠️  Stop warning"), err)
		}

		time.Sleep(1 * time.Second)

		fmt.Println(theme.RenderPrimary("🚀 Starting API..."))
		if err := manager.Start(apiRunArgs()); err != nil {
			log.Fatalf("Failed to start API: %v", err)
		}
	},
}

var apiStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check API daemon status",
	Run: func(cmd *cobra.Command, args []string) {
		manager, err := daemon.NewGlobalManager(apiDaemonName())
		if err != nil {
			log.Fatalf("Error: %v", err)
		}

		pid, running, err := manager.Status()
		if err != nil {
			log.Fatalf("Error checking status: %v", err)
		}

		if running {
			fmt.Printf("%s (PID: %d)\n", theme.RenderSuccess("✅ API is running"), pid)
			fmt.Printf("   http://localhost:%s/docs/\n", apiPort)
		} else {
			fmt.Println(theme.RenderError("❌ API is stopped"))
		}
	},
}

func runAPI() {
	if err := controlplane.Run(":" + apiPort); err != nil {
		log.Fatalf("API server error: %v", err)
	}
}

func apiDaemonName() string {
	return "myaaw-api"
}

func apiRunArgs() []string {
	return []string{"api", "run", "--port", apiPort}
}

func init() {
	apiCmd.PersistentFlags().StringVar(&apiPort, "port", "9000", "port to listen on")
	apiCmd.AddCommand(apiRunCmd)
	apiCmd.AddCommand(apiStartCmd)
	apiCmd.AddCommand(apiStopCmd)
	apiCmd.AddCommand(apiRestartCmd)
	apiCmd.AddCommand(apiStatusCmd)
	rootCmd.AddCommand(apiCmd)
}
