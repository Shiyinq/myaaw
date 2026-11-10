package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"myaaw/internal/agentdir"
	"myaaw/internal/cli/theme"
	"myaaw/internal/config"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check service status",
	Long: "Check the API, the dashboard, and the agents.\n\n" +
		"Without flags it reports the default agent; --agent <name> reports one\n" +
		"agent instead; --all-agent reports every registered agent.",
	Run: func(cmd *cobra.Command, args []string) {
		runStatus()
	},
}

func init() {
	statusCmd.Flags().BoolVar(&serviceAllAgents, "all-agent", false, "report every registered agent")
}

// agentStatusView is everything `myaaw status` shows for one agent. It is built
// from that agent's files, so several agents can be inspected in one process
// without touching the global configuration.
type agentStatusView struct {
	Name             string
	Port             string
	PID              int
	Running          bool
	DBPath           string
	DBErr            error
	Telegram         bool
	TelegramMode     string
	Discord          bool
	HeartbeatActive  bool
	HeartbeatEvery   string
	HeartbeatTo      string
	HeartbeatChannel string
	Provider         string
	Streaming        bool
}

func runStatus() {
	fmt.Println(theme.RenderPrimary("Myaaw Service Status"))
	fmt.Println(theme.RenderSecondary("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))
	printServicesSection()

	names := statusAgentNames()
	for _, name := range names {
		fmt.Println(theme.RenderSecondary("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))
		printAgentStatus(collectAgentStatus(name))
	}

	if len(names) == 0 {
		fmt.Println(theme.RenderSecondary("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))
		fmt.Println(theme.RenderPrimary("Agents"))
		fmt.Println(theme.RenderMuted("  No agents registered. Run 'myaaw onboard' first."))
	}

	fmt.Println(theme.RenderSecondary("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"))
}

func statusAgentNames() []string {
	if serviceAllAgents {
		names, err := agentdir.ListAgents()
		if err != nil {
			return nil
		}
		return names
	}
	return []string{agentdir.Active()}
}

func printServicesSection() {
	fmt.Println(theme.RenderPrimary("Services"))

	if pid, running, _ := apiDaemonStatus(); running {
		fmt.Printf("  API      %s (PID: %d)  http://localhost:%s/docs/\n", theme.RenderSuccess("Running"), pid, apiPort)
	} else {
		fmt.Printf("  API      %s\n", theme.RenderError("Stopped"))
	}

	if !isWebUIInstalled() {
		fmt.Printf("  WebUI    %s\n", theme.RenderMuted("Not installed"))
		return
	}
	cfg, _ := loadWebUIConfig()
	if pid, running, _ := webUIDaemonStatus(); running {
		fmt.Printf("  WebUI    %s (PID: %d)  http://127.0.0.1:%s\n", theme.RenderSuccess("Running"), pid, cfg.Port)
	} else {
		fmt.Printf("  WebUI    %s\n", theme.RenderError("Stopped"))
	}
}

func printAgentStatus(status agentStatusView) {
	title := fmt.Sprintf("Agent: %s", status.Name)
	if isRegistryDefaultAgent(status.Name) {
		title += " (default)"
	}
	fmt.Println(theme.RenderPrimary(title))

	if status.Running {
		fmt.Printf("  %-12s %s\n", "Gateway", theme.RenderSuccess("OPERATIONAL"))
		fmt.Printf("  %-12s %s (PID: %d, Port: %s)\n", "Server", theme.RenderSuccess("Running"), status.PID, status.Port)
	} else {
		fmt.Printf("  %-12s %s\n", "Gateway", theme.RenderError("OFFLINE"))
		fmt.Printf("  %-12s %s\n", "Server", theme.RenderError("Stopped"))
	}

	if status.DBPath == "" {
		fmt.Printf("  %-12s %s\n", "SQLite DB", theme.RenderError("No database path"))
	} else if status.DBErr != nil {
		fmt.Printf("  %-12s %s (%s)\n", "SQLite DB", theme.RenderError("[ERR] Offline"), status.DBErr)
	} else {
		fmt.Printf("  %-12s %s (Path: %s)\n", "SQLite DB", theme.RenderSuccess("Online"), status.DBPath)
	}

	if status.Telegram {
		fmt.Printf("  %-12s %s (%s)\n", "Telegram", theme.RenderSuccess("Configured"), status.TelegramMode)
	} else {
		fmt.Printf("  %-12s %s\n", "Telegram", theme.RenderError("Not configured"))
	}

	if status.Discord {
		fmt.Printf("  %-12s %s\n", "Discord", theme.RenderSuccess("Configured"))
	} else {
		fmt.Printf("  %-12s %s\n", "Discord", theme.RenderError("Not configured"))
	}

	if status.HeartbeatActive {
		fmt.Printf("  %-12s %s (Every %s)\n", "Heartbeat", theme.RenderSuccess("Active"), status.HeartbeatEvery)
		if status.HeartbeatTo != "" {
			fmt.Printf("  %-12s %s: %s (%s)\n", "Channel", theme.RenderSecondary("To"), status.HeartbeatTo, status.HeartbeatChannel)
		}
	} else {
		fmt.Printf("  %-12s %s\n", "Heartbeat", theme.RenderError("Disabled"))
	}

	if status.Provider != "" {
		fmt.Printf("  %-12s %s\n", "Provider", theme.RenderPrimary(status.Provider))
	} else {
		fmt.Printf("  %-12s %s\n", "Provider", theme.RenderError("Not configured"))
	}

	if status.Streaming {
		fmt.Printf("  %-12s %s\n", "Streaming", theme.RenderSuccess("Enabled"))
	} else {
		fmt.Printf("  %-12s %s\n", "Streaming", theme.RenderError("Disabled"))
	}
}

// isRegistryDefaultAgent marks the registry default, which is not necessarily
// the agent --agent selected for this run.
func isRegistryDefaultAgent(name string) bool {
	registry, err := agentdir.LoadRegistry()
	if err != nil || registry.Default == "" {
		return name == agentdir.DefaultAgentName
	}
	return name == registry.Default
}

func collectAgentStatus(name string) agentStatusView {
	status := agentStatusView{Name: name, Streaming: true}

	env := readAgentEnv(name)
	status.Port = agentPort(name)

	status.PID, status.Running, _ = agentDaemonStatus(name)

	status.DBPath = strings.TrimSpace(env["DB_PATH"])
	if status.DBPath == "" {
		if path, err := agentdir.DatabasePath(name); err == nil {
			status.DBPath = path
		}
	}
	if status.DBPath != "" {
		status.DBErr = config.PingSQLiteAt(status.DBPath)
	}

	if raw, ok := env["STREAM_RESPONSE"]; ok {
		value := strings.ToLower(strings.TrimSpace(raw))
		status.Streaming = !(value == "false" || value == "0")
	}

	cfg := readAgentConfig(name)
	if cfg == nil {
		status.Provider = strings.TrimSpace(env["LLM_PROVIDER_NAME"])
		return status
	}

	status.HeartbeatActive = cfg.Heartbeat.Active
	status.HeartbeatEvery = cfg.Heartbeat.Every
	status.HeartbeatTo = cfg.Heartbeat.To
	status.HeartbeatChannel = cfg.Heartbeat.Channel

	if telegram := cfg.Channels.Telegram; telegram != nil && telegram.Active && telegram.Token != "" {
		status.Telegram = true
		status.TelegramMode = strings.ToLower(telegram.Mode)
		if status.TelegramMode == "" {
			status.TelegramMode = "polling"
		}
	}
	if discord := cfg.Channels.Discord; discord != nil && discord.Active && discord.Token != "" {
		status.Discord = true
	}

	if cfg.DefaultProvider != "" {
		if provider, ok := cfg.Providers[cfg.DefaultProvider]; ok {
			status.Provider = provider.Type
		}
	}
	if cfg.Bot.Stream != nil {
		status.Streaming = *cfg.Bot.Stream
	}
	if status.Provider == "" {
		status.Provider = strings.TrimSpace(env["LLM_PROVIDER_NAME"])
	}

	return status
}

// agentPort resolves one agent's HTTP port from its .env, falling back to the
// port the layout would assign, without loading the full configuration.
func agentPort(name string) string {
	if port := strings.TrimSpace(readAgentEnv(name)["PORT"]); port != "" {
		return port
	}
	return strconv.Itoa(agentdir.SuggestedPort(name))
}

// readAgentEnv reads one agent's .env without touching the process environment,
// so the same process can report every agent.
func readAgentEnv(name string) map[string]string {
	path, err := agentdir.EnvPath(name)
	if err != nil {
		return map[string]string{}
	}
	env, err := godotenv.Read(path)
	if err != nil {
		return map[string]string{}
	}
	return env
}

func readAgentConfig(name string) *config.Config {
	path, err := agentdir.ConfigPath(name)
	if err != nil {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	cfg, err := config.LoadConfigFile(path)
	if err != nil {
		return nil
	}
	return cfg
}
