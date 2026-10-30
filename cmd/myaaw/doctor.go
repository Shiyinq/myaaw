package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"myaaw/internal/agent/tools/computer"
	"myaaw/internal/cli/theme"
	"myaaw/internal/config"

	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check environment readiness for Myaaw features",
	Long: "Probe the environment for what Myaaw needs: desktop automation " +
		"permissions, dependencies, and feature configuration.",
	Run: func(cmd *cobra.Command, args []string) {
		config.LoadBaseConfig()

		fmt.Println(theme.RenderPrimary("🩺 Myaaw Doctor"))
		fmt.Println(theme.RenderSecondary("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))

		fmt.Println(theme.RenderMuted("Computer use (desktop automation)"))
		fmt.Printf("  %-30s %s\n", "Launched from", theme.RenderMuted(launchContext()))
		if config.ComputerUseEnabled {
			fmt.Printf("  %-30s %s\n", "Feature enabled", theme.RenderSuccess("✅ ON"))
		} else {
			fmt.Printf("  %-30s %s\n", "Feature enabled", theme.RenderWarning("⚠️  OFF"))
			fmt.Printf("    %s\n", theme.RenderMuted("Enable with MYAAW_COMPUTER_USE=true or bot.computer_use.enabled in config.json."))
		}

		ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
		defer cancel()
		for _, result := range computer.Probe(ctx) {
			status := theme.RenderSuccess("✅ OK")
			if !result.OK {
				status = theme.RenderError("❌ FAIL")
			}
			fmt.Printf("  %-30s %s\n", result.Name, status)
			if result.Detail != "" {
				fmt.Printf("    %s\n", theme.RenderMuted(result.Detail))
			}
			if !result.OK && result.Fix != "" {
				fmt.Printf("    %s\n", theme.RenderMuted("Fix: "+result.Fix))
			}
		}

		fmt.Println(theme.RenderSecondary("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))
		if !config.ComputerUseEnabled {
			fmt.Println(theme.RenderMuted("Tip: enable computer use first, then re-run doctor to verify permissions."))
		} else {
			fmt.Println(theme.RenderMuted("Tip: grant permissions to the app shown in \"Launched from\" — the terminal for `myaaw chat`, or the myaaw binary for the daemon. Each needs its own grant."))
		}
	},
}

// launchContext names the app macOS will attribute the permission request to.
func launchContext() string {
	if term := os.Getenv("TERM_PROGRAM"); term != "" {
		return term + " (terminal) — grant permissions to this app, then restart it"
	}
	exe, err := os.Executable()
	if err != nil {
		exe = "myaaw"
	}
	return exe + " (no terminal detected: daemon/background) — grant permissions to this binary"
}
