package main

import (
	"fmt"
	"log"
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

		fmt.Println("⏳ Waiting for services to stop...")
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

		fmt.Println("🖥️  Gateway Process Status")
		if srvRunning {
			fmt.Printf("  Server      ✅ Running (PID: %d)\n", srv)
		} else {
			fmt.Println("  Server      ❌ Stopped")
		}

		if consRunning {
			fmt.Printf("  Consumer    ✅ Running (PID: %d)\n", cons)
		} else {
			fmt.Println("  Consumer    ❌ Stopped")
		}

		fmt.Println("  ---------------------------")
		if srvRunning && consRunning {
			fmt.Println("  Gateway     ✅ OPERATIONAL")
		} else if !srvRunning && !consRunning {
			fmt.Println("  Gateway     ❌ OFFLINE")
		} else {
			if srvRunning {
				fmt.Println("  Gateway     ⚠️ PARTIAL (Server Only)")
			} else {
				fmt.Println("  Gateway     ⚠️ PARTIAL (Consumer Only)")
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
