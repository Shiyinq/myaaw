package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"myaaw/internal/agentdir"
	"myaaw/internal/cli/theme"

	"github.com/spf13/cobra"
)

var (
	upgradeCheckOnly bool
	upgradeForce     bool
	upgradeNoRestart bool
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Update the CLI and the WebUI to the latest release, then restart running services",
	Long: "Update the Myaaw CLI, update the WebUI bundle when it is installed, and\n" +
		"restart the services that are already running. Stopped services stay stopped.",
	Run: func(cmd *cobra.Command, args []string) {
		runUpgrade()
	},
}

func init() {
	upgradeCmd.Flags().BoolVar(&upgradeCheckOnly, "check", false, "report available updates without changing anything")
	upgradeCmd.Flags().BoolVar(&upgradeForce, "force", false, "reinstall the WebUI bundle even when its tag matches")
	upgradeCmd.Flags().BoolVar(&upgradeNoRestart, "no-restart", false, "do not restart running services")
	rootCmd.AddCommand(upgradeCmd)
}

func runUpgrade() {
	fmt.Printf("%s %s...\n", theme.RenderSecondary("Checking for updates at"), githubRepo)

	latest, downloadURL, err := getLatestReleaseInfo()
	if err != nil {
		log.Fatalf("%s checking for updates: %v", theme.RenderError("Error"), err)
	}

	cliOutdated := latest != Version
	webuiInstalled := isWebUIInstalled()
	webuiCurrent, _ := installedWebUIVersion()
	webuiOutdated := webuiInstalled && !strings.EqualFold(webuiCurrent, latest)

	if upgradeCheckOnly {
		printUpgradeStatus(latest, cliOutdated, webuiInstalled, webuiCurrent, webuiOutdated)
		return
	}

	cliChanged := false
	if cliOutdated {
		fmt.Printf("%s %s (current: %s)\n", theme.RenderPrimary("Updating the CLI to"), latest, Version)
		if err := performUpdate(downloadURL); err != nil {
			log.Fatalf("%s: %v", theme.RenderError("CLI update failed"), err)
		}
		cliChanged = true
	} else {
		fmt.Printf("%s (Version: %s)\n", theme.RenderSuccess("CLI is already up to date"), Version)
	}

	webuiChanged := false
	if !webuiInstalled {
		fmt.Println(theme.RenderMuted("WebUI is not installed; skipped."))
	} else {
		changed, err := installWebUI("", upgradeForce)
		if err != nil {
			log.Fatalf("%s: %v", theme.RenderError("WebUI update failed"), err)
		}
		webuiChanged = changed
		if changed && isWebUIRunning() {
			if err := restartWebUI(effectiveWebUIConfig()); err != nil {
				fmt.Printf("%s: %v\n", theme.RenderError("WebUI restart warning"), err)
			}
		}
	}

	switch {
	case upgradeNoRestart:
		fmt.Println(theme.RenderMuted("Skipping service restarts (--no-restart)."))
	case cliChanged:
		// The agent servers and the control plane run the CLI binary, so they
		// only pick up a new build after a restart.
		restartRunningServices()
	case webuiChanged:
		fmt.Println(theme.RenderMuted("No service restart needed: the WebUI runs Node, not the CLI binary."))
	}

	fmt.Println(theme.RenderSuccess("Upgrade finished."))
}

func printUpgradeStatus(latest string, cliOutdated, webuiInstalled bool, webuiCurrent string, webuiOutdated bool) {
	cliState := "up to date"
	if cliOutdated {
		cliState = latest + " available"
	}
	fmt.Printf("%s %s (%s)\n", theme.RenderSecondary("CLI:"), Version, cliState)

	switch {
	case !webuiInstalled:
		fmt.Printf("%s %s\n", theme.RenderSecondary("WebUI:"), theme.RenderMuted("not installed"))
	case webuiOutdated:
		fmt.Printf("%s %s (%s available)\n", theme.RenderSecondary("WebUI:"), webuiVersionLabel(webuiCurrent), latest)
	default:
		fmt.Printf("%s %s (up to date)\n", theme.RenderSecondary("WebUI:"), webuiVersionLabel(webuiCurrent))
	}
}

func webuiVersionLabel(version string) string {
	if strings.TrimSpace(version) == "" {
		return "unknown"
	}
	return version
}

// restartRunningServices restarts the agent daemons and the control plane that
// are already running. Services that are stopped are left alone.
func restartRunningServices() {
	fmt.Println(theme.RenderPrimary("Restarting running services"))

	names, err := agentdir.ListAgents()
	if err != nil {
		fmt.Printf("  %s\n", theme.RenderError("could not list agents: "+err.Error()))
	}

	restarted, failed := 0, 0
	for _, name := range runningAgents(names, func(agentName string) bool {
		_, running, _ := agentDaemonStatus(agentName)
		return running
	}) {
		if err := restartAgent(name); err != nil {
			failed++
			fmt.Printf("  %s\n", theme.RenderError(fmt.Sprintf("agent %q: %v", name, err)))
			continue
		}
		restarted++
		fmt.Printf("  %s\n", theme.RenderSuccess(fmt.Sprintf("agent %q restarted", name)))
	}

	if _, running, _ := apiDaemonStatus(); running {
		if err := restartAPI(); err != nil {
			failed++
			fmt.Printf("  %s\n", theme.RenderError("API: "+err.Error()))
		} else {
			restarted++
			fmt.Printf("  %s\n", theme.RenderSuccess("API restarted"))
		}
	}

	if restarted == 0 && failed == 0 {
		fmt.Println(theme.RenderMuted("  No running services to restart."))
	}
	if failed > 0 {
		fmt.Println(theme.RenderError(fmt.Sprintf("%d service(s) failed to restart", failed)))
	}
}

// runningAgents filters names with isRunning, preserving the input order.
func runningAgents(names []string, isRunning func(string) bool) []string {
	running := make([]string, 0, len(names))
	for _, name := range names {
		if isRunning(name) {
			running = append(running, name)
		}
	}
	return running
}

func restartAgent(name string) error {
	if err := stopAgent(name); err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)
	_, err := startAgentAndWait(name)
	return err
}

func restartAPI() error {
	if err := stopAPIDaemon(); err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)
	_, err := startAPIAndWait()
	return err
}
