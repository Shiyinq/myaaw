package main

import (
	"fmt"
	"log"
	"myaaw/internal/cli/theme"
	"myaaw/internal/daemon"
	"time"

	"github.com/spf13/cobra"
)

var gatewayCmd = &cobra.Command{
	Use:   "gateway",
	Short: "Manage both server and consumer (Gateway)",
	Long:  "Manage the entire system (Server + Consumer) as a gateway service.",
}

var gatewayStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start both server and consumer in background",
	Run: func(cmd *cobra.Command, args []string) {
		startService("myaaw-server", []string{"server", "run"})
		startService("myaaw-consumer", []string{"consumer", "run"})
	},
}

var gatewayStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop both server and consumer",
	Run: func(cmd *cobra.Command, args []string) {
		stopService("myaaw-server")
		stopService("myaaw-consumer")
	},
}

var gatewayRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart both server and consumer",
	Run: func(cmd *cobra.Command, args []string) {
		stopService("myaaw-server")
		stopService("myaaw-consumer")

		fmt.Println(theme.RenderSecondary("⏳ Waiting for services to stop..."))
		time.Sleep(2 * time.Second)

		startService("myaaw-server", []string{"server", "run"})
		startService("myaaw-consumer", []string{"consumer", "run"})
	},
}

var gatewayStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check gateway status",
	Run: func(cmd *cobra.Command, args []string) {
		srv, srvRunning, _ := checkService("myaaw-server")
		cons, consRunning, _ := checkService("myaaw-consumer")

		fmt.Println(theme.RenderPrimary("🖥️  Gateway Process Status"))
		if srvRunning {
			fmt.Printf("  %-12s %s (PID: %d)\n", "Server", theme.RenderSuccess("✅ Running"), srv)
		} else {
			fmt.Printf("  %-12s %s\n", "Server", theme.RenderError("❌ Stopped"))
		}

		if consRunning {
			fmt.Printf("  %-12s %s (PID: %d)\n", "Consumer", theme.RenderSuccess("✅ Running"), cons)
		} else {
			fmt.Printf("  %-12s %s\n", "Consumer", theme.RenderError("❌ Stopped"))
		}

		fmt.Println(theme.RenderSecondary("  ---------------------------"))
		if srvRunning && consRunning {
			fmt.Printf("  %-12s %s\n", "Gateway", theme.RenderSuccess("✅ OPERATIONAL"))
		} else if !srvRunning && !consRunning {
			fmt.Printf("  %-12s %s\n", "Gateway", theme.RenderError("❌ OFFLINE"))
		} else {
			if srvRunning {
				fmt.Printf("  %-12s %s\n", "Gateway", theme.RenderError("⚠️  PARTIAL (Server Only)"))
			} else {
				fmt.Printf("  %-12s %s\n", "Gateway", theme.RenderError("⚠️  PARTIAL (Consumer Only)"))
			}
		}
	},
}

func init() {
	gatewayCmd.AddCommand(gatewayStartCmd)
	gatewayCmd.AddCommand(gatewayStopCmd)
	gatewayCmd.AddCommand(gatewayRestartCmd)
	gatewayCmd.AddCommand(gatewayStatusCmd)
	rootCmd.AddCommand(gatewayCmd)
}

func startService(name string, args []string) {
	dm, err := daemon.NewManager(name)
	if err != nil {
		log.Printf("❌ Failed to init manager for %s: %v", name, err)
		return
	}
	if err := dm.Start(args); err != nil {
		log.Printf("❌ Failed to start %s: %v", name, err)
	}
}

func stopService(name string) {
	dm, err := daemon.NewManager(name)
	if err != nil {
		log.Printf("❌ Failed to init manager for %s: %v", name, err)
		return
	}
	if err := dm.Stop(); err != nil {
		log.Printf("❌ Failed to stop %s: %v", name, err)
	}
}

func checkService(name string) (int, bool, error) {
	dm, err := daemon.NewManager(name)
	if err != nil {
		return 0, false, err
	}
	return dm.Status()
}
