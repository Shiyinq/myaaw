package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"myaaw/internal/cli/theme"
	"myaaw/internal/daemon"
	"myaaw/internal/services/controlplane"
	"myaaw/internal/utils"

	"github.com/spf13/cobra"
)

var (
	apiPort           string
	apiHost           string
	apiTerminal       bool
	apiAllowedOrigins string
)

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

// applyAPIFlags turns flags into the environment variables the control plane
// reads at startup. The flags exist because daemon units carry no environment
// of their own.
func applyAPIFlags() {
	if apiTerminal {
		_ = os.Setenv("MYAAW_TERMINAL", "1")
	}
	if origins := effectiveAllowedOrigins(); origins != "" {
		_ = os.Setenv("ALLOWED_ORIGINS", origins)
	}
}

func runAPI() {
	applyAPIFlags()
	if err := controlplane.Run(net.JoinHostPort(apiHost, apiPort)); err != nil {
		log.Fatalf("API server error: %v", err)
	}
}

func apiDaemonName() string {
	return "myaaw-api"
}

// effectiveAllowedOrigins prefers the flag, then the environment, so both
// `myaaw api start --allowed-origins ...` and an exported ALLOWED_ORIGINS reach
// the daemon.
func effectiveAllowedOrigins() string {
	if origins := strings.TrimSpace(apiAllowedOrigins); origins != "" {
		return origins
	}
	return strings.TrimSpace(os.Getenv("ALLOWED_ORIGINS"))
}

func apiRunArgs() []string {
	args := []string{"api", "run", "--port", apiPort, "--host", apiHost}
	if apiTerminal {
		// Daemon units carry no environment, so the flag is how `api start` and
		// `api restart` remember that the terminal is enabled.
		args = append(args, "--terminal")
	}
	if origins := effectiveAllowedOrigins(); origins != "" {
		// The systemd unit joins args with spaces, so keep the list tight.
		args = append(args, "--allowed-origins", strings.Join(utils.ParseAllowedOrigins(origins), ","))
	}
	return args
}

func init() {
	apiCmd.PersistentFlags().StringVar(&apiPort, "port", "9000", "port to listen on")
	apiCmd.PersistentFlags().StringVar(&apiHost, "host", "127.0.0.1", "host/interface to bind (use 0.0.0.0 to expose)")
	apiCmd.PersistentFlags().BoolVar(&apiTerminal, "terminal", false, "allow the dashboard to open a shell on this host (enables MYAAW_TERMINAL)")
	apiCmd.PersistentFlags().StringVar(&apiAllowedOrigins, "allowed-origins", "", "comma-separated browser origins allowed to call the API (sets ALLOWED_ORIGINS)")
	apiCmd.AddCommand(apiRunCmd)
	apiCmd.AddCommand(apiStartCmd)
	apiCmd.AddCommand(apiStopCmd)
	apiCmd.AddCommand(apiRestartCmd)
	apiCmd.AddCommand(apiStatusCmd)
	rootCmd.AddCommand(apiCmd)
}
