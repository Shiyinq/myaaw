package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"myaaw/internal/agentdir"
	"myaaw/internal/cli/theme"
	"myaaw/internal/daemon"

	"github.com/spf13/cobra"
)

const (
	targetAgent = "agent"
	targetAPI   = "api"
	targetWebUI = "webui"
)

// daemonStartTimeout bounds how long a start waits for the supervisor to report
// a live PID before the target is reported as failed.
const daemonStartTimeout = 5 * time.Second

// serviceAllAgents backs --all-agent on start, stop, restart, and status.
var serviceAllAgents bool

// serviceTarget is one daemon the service commands operate on.
type serviceTarget struct {
	kind string
	name string
}

var serviceStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the agent servers, the API, and the WebUI",
	Long: "Start the agent servers, the control plane API, and the dashboard.\n\n" +
		"Without flags only the default agent is started. --agent <name> picks one\n" +
		"agent instead, and --all-agent starts every registered agent. The API and\n" +
		"the WebUI (when installed) are always included.",
	Run: func(cmd *cobra.Command, args []string) {
		startServices()
	},
}

var serviceStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the agent servers, the API, and the WebUI",
	Long:  "Stop the same targets that `myaaw start` would start.",
	Run: func(cmd *cobra.Command, args []string) {
		stopServices()
	},
}

var serviceRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the agent servers, the API, and the WebUI",
	Long:  "Stop the same targets that `myaaw start` would start, then start them again.",
	Run: func(cmd *cobra.Command, args []string) {
		restartServices()
	},
}

func init() {
	for _, cmd := range []*cobra.Command{serviceStartCmd, serviceStopCmd, serviceRestartCmd} {
		cmd.Flags().BoolVar(&serviceAllAgents, "all-agent", false, "operate on every registered agent")
		rootCmd.AddCommand(cmd)
	}
}

// allAgentsRequested reports whether the running command was invoked with
// --all-agent, which takes precedence over --agent.
func allAgentsRequested(cmd *cobra.Command) bool {
	if cmd.Flags().Lookup("all-agent") == nil {
		return false
	}
	value, err := cmd.Flags().GetBool("all-agent")
	return err == nil && value
}

// buildServiceTargets is the pure target selector: agents first, then the API and
// the WebUI. `agents` is only read in all-agent mode.
func buildServiceTargets(agentFlag string, allAgents bool, agents []string, active string) []serviceTarget {
	targets := make([]serviceTarget, 0, len(agents)+2)
	if allAgents {
		for _, name := range agents {
			targets = append(targets, serviceTarget{kind: targetAgent, name: name})
		}
	} else {
		name := strings.TrimSpace(agentFlag)
		if name == "" {
			name = active
		}
		targets = append(targets, serviceTarget{kind: targetAgent, name: name})
	}
	targets = append(targets,
		serviceTarget{kind: targetAPI},
		serviceTarget{kind: targetWebUI},
	)
	return targets
}

func currentServiceTargets() ([]serviceTarget, error) {
	if serviceAllAgents {
		names, err := agentdir.ListAgents()
		if err != nil {
			return nil, err
		}
		return buildServiceTargets("", true, names, agentdir.Active()), nil
	}
	return buildServiceTargets(agentFlag, false, nil, agentdir.Active()), nil
}

func startServices() {
	targets, err := currentServiceTargets()
	if err != nil {
		failService(err)
	}

	fmt.Println(theme.RenderPrimary("Starting services"))
	failed := 0
	for _, target := range targets {
		if err := startServiceTarget(target); err != nil {
			failed++
			printTargetError(target, "start", err)
		}
	}
	finishService(failed, "targets", "started")
}

func stopServices() {
	targets, err := currentServiceTargets()
	if err != nil {
		failService(err)
	}

	fmt.Println(theme.RenderSecondary("Stopping services"))
	failed := 0
	for _, target := range reverseTargets(targets) {
		if err := stopServiceTarget(target); err != nil {
			failed++
			printTargetError(target, "stop", err)
		}
	}
	finishService(failed, "targets", "stopped")
}

func restartServices() {
	targets, err := currentServiceTargets()
	if err != nil {
		failService(err)
	}

	fmt.Println(theme.RenderPrimary("Restarting services"))
	failed := 0
	for _, target := range reverseTargets(targets) {
		if err := stopServiceTarget(target); err != nil {
			failed++
			printTargetError(target, "stop", err)
		}
	}
	time.Sleep(1 * time.Second)
	for _, target := range targets {
		if err := startServiceTarget(target); err != nil {
			failed++
			printTargetError(target, "start", err)
		}
	}
	finishService(failed, "targets", "restarted")
}

func startServiceTarget(target serviceTarget) error {
	switch target.kind {
	case targetAgent:
		pid, err := startAgentAndWait(target.name)
		if err != nil {
			return err
		}
		fmt.Printf("  %s\n", theme.RenderSuccess(fmt.Sprintf("agent %q started (PID %d)", target.name, pid)))
	case targetAPI:
		pid, err := startAPIAndWait()
		if err != nil {
			return err
		}
		fmt.Printf("  %s\n", theme.RenderSuccess(fmt.Sprintf("API started (PID %d)", pid)))
	case targetWebUI:
		if !isWebUIInstalled() {
			fmt.Printf("  %s\n", theme.RenderMuted("WebUI skipped (not installed)"))
			return nil
		}
		pid, err := startWebUIAndWait()
		if err != nil {
			return err
		}
		fmt.Printf("  %s\n", theme.RenderSuccess(fmt.Sprintf("WebUI started (PID %d)", pid)))
	}
	return nil
}

func stopServiceTarget(target serviceTarget) error {
	switch target.kind {
	case targetAgent:
		if err := stopAgent(target.name); err != nil {
			return err
		}
		fmt.Printf("  %s\n", theme.RenderSuccess(fmt.Sprintf("agent %q stopped", target.name)))
	case targetAPI:
		if err := stopAPIDaemon(); err != nil {
			return err
		}
		fmt.Printf("  %s\n", theme.RenderSuccess("API stopped"))
	case targetWebUI:
		if !isWebUIInstalled() {
			return nil
		}
		if err := stopWebUIDaemon(); err != nil {
			return err
		}
		fmt.Printf("  %s\n", theme.RenderSuccess("WebUI stopped"))
	}
	return nil
}

func startAgentAndWait(agentName string) (int, error) {
	dm, err := daemon.NewAgentManager(agentName)
	if err != nil {
		return 0, err
	}
	if pid, running, _ := dm.Status(); running {
		return pid, nil
	}
	if err := dm.Start(agentRunArgs(agentName)); err != nil {
		return 0, err
	}
	pid, running := waitDaemonStatus(dm.Status, daemonStartTimeout)
	if !running {
		return 0, fmt.Errorf("daemon exited right after starting; check %s", dm.LogFile)
	}
	return pid, nil
}

func startAPIAndWait() (int, error) {
	dm, err := daemon.NewGlobalManager(apiDaemonName())
	if err != nil {
		return 0, err
	}
	if pid, running, _ := dm.Status(); running {
		return pid, nil
	}
	if err := dm.Start(apiRunArgs()); err != nil {
		return 0, err
	}
	pid, running := waitDaemonStatus(dm.Status, daemonStartTimeout)
	if !running {
		return 0, fmt.Errorf("daemon exited right after starting; check %s", dm.LogFile)
	}
	return pid, nil
}

func startWebUIAndWait() (int, error) {
	if pid, running, _ := webUIDaemonStatus(); running {
		return pid, nil
	}
	// startWebUI already waits for a live process and pins the Node.js binary.
	if err := startWebUI(effectiveWebUIConfig()); err != nil {
		return 0, err
	}
	pid, _, err := webUIDaemonStatus()
	if err != nil {
		return 0, err
	}
	return pid, nil
}

// waitDaemonStatus polls a daemon until the supervisor reports a live PID.
func waitDaemonStatus(status func() (int, bool, error), timeout time.Duration) (int, bool) {
	deadline := time.Now().Add(timeout)
	for {
		if pid, running, err := status(); err == nil && running {
			return pid, true
		}
		if time.Now().After(deadline) {
			return 0, false
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func reverseTargets(targets []serviceTarget) []serviceTarget {
	reversed := make([]serviceTarget, len(targets))
	for i, target := range targets {
		reversed[len(targets)-1-i] = target
	}
	return reversed
}

func describeTarget(target serviceTarget) string {
	switch target.kind {
	case targetAgent:
		return fmt.Sprintf("agent %q", target.name)
	case targetAPI:
		return "API"
	default:
		return "WebUI"
	}
}

func printTargetError(target serviceTarget, action string, err error) {
	fmt.Printf("  %s\n", theme.RenderError(fmt.Sprintf("%s failed to %s: %v", describeTarget(target), action, err)))
}

func finishService(failed int, subject, verb string) {
	if failed > 0 {
		fmt.Println(theme.RenderError(fmt.Sprintf("%d %s failed", failed, subject)))
		os.Exit(1)
	}
	fmt.Println(theme.RenderSuccess(fmt.Sprintf("All %s %s", subject, verb)))
}

func failService(err error) {
	fmt.Println(theme.RenderError("" + err.Error()))
	os.Exit(1)
}
