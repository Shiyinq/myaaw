// @title           Myaaw API Gateway
// @version         0.0.1
// @description     Myaaw is an intelligent API Gateway with multi-channel support (Telegram, Discord, etc.).
// @host      localhost:8080
// @BasePath  /
package main

import (
	"fmt"
	"myaaw/internal/cli/theme"
	"myaaw/internal/config"
	"os"

	"myaaw/internal/agent"
	"myaaw/internal/agent/skills"
	"myaaw/internal/agent/tools"
	"myaaw/internal/agent/tools/filesystem"
	"myaaw/internal/agentdir"

	_ "myaaw/internal/agent/tools/a2aauth"
	_ "myaaw/internal/agent/tools/a2ainput"
	_ "myaaw/internal/agent/tools/bash"
	_ "myaaw/internal/agent/tools/computer"
	_ "myaaw/internal/agent/tools/contact"
	_ "myaaw/internal/agent/tools/cron"
	_ "myaaw/internal/agent/tools/provider"
	_ "myaaw/internal/agent/tools/python"
	_ "myaaw/internal/agent/tools/subagent"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "myaaw",
	Short: "Personal AI Assistant",
	Long:  "😼 A Cat who becomes your personal AI assistant.",
	// Errors are printed once by main; commands suppress usage for logical
	// failures so the output stays readable.
	SilenceErrors: true,
}

var agentFlag string

func isAgentCommand(cmd *cobra.Command) bool {
	for current := cmd; current != nil; current = current.Parent() {
		if current.Name() == "agent" {
			return true
		}
	}
	return false
}

func isAPISubcommand(cmd *cobra.Command) bool {
	for current := cmd; current != nil; current = current.Parent() {
		if current.Name() == "api" {
			return true
		}
	}
	return false
}

func startAgentBackgroundLoaders() {
	tools.ConfigureLogging()
	skills.ConfigureLogging()
	go func() {
		// Log skills state first — it's independent of tool loading and should
		// appear immediately, even if loading external tools is slow.
		skills.WatchSkills()
		tools.LoadExternalTools()
		tools.WatchExternalTools()
	}()
}

func init() {
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(chatCmd)
	rootCmd.AddCommand(voiceCmd)
	rootCmd.AddCommand(voiceClassicCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(webhookCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(providerCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(logsCmd)
	rootCmd.AddCommand(observabilityCmd)

	// Aliases for convenience so users can type `myaaw start` instead of `myaaw server start`
	rootCmd.AddCommand(serverStartCmd)
	rootCmd.AddCommand(serverStopCmd)
	rootCmd.AddCommand(serverRestartCmd)

	rootCmd.PersistentFlags().BoolVarP(&config.Verbose, "verbose", "v", false, "enable verbose logging")
	rootCmd.PersistentFlags().StringVar(&agentFlag, "agent", "", "agent name to use (defaults to the registry default)")

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// Add global spacing, except for commands that may stream data to
		// stdout (backup -o -).
		if cmd.Name() != "backup" {
			fmt.Println()
		}

		// Commands that don't require .myaaw configuration
		metaCommands := map[string]bool{
			"onboard": true,
			"version": true,
			"help":    true,
			"update":  true,
			"agent":   true,
			"api":     true,
			"backup":  true,
			"restore": true,
		}

		if metaCommands[cmd.Name()] || isAgentCommand(cmd) || isAPISubcommand(cmd) {
			return nil
		}

		if err := agentdir.EnsureLayout(); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Could not prepare agent layout: %v\n", err)
			os.Exit(1)
		}

		activeAgent, err := agentdir.ResolveActive(agentFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Invalid agent: %v\n", err)
			os.Exit(1)
		}
		if !agentdir.AgentInitialized(activeAgent) && agentFlag == "" {
			// The registry default may be missing while another agent exists.
			if names, listErr := agentdir.ListAgents(); listErr == nil && len(names) > 0 {
				activeAgent = names[0]
				_ = agentdir.SetActive(activeAgent)
			}
		}
		if !agentdir.AgentInitialized(activeAgent) {
			fmt.Printf("⚠️  Agent %q is not initialized!\n", activeAgent)
			fmt.Println("Please run 'myaaw onboard' or 'myaaw agent create <name>' first.")
			os.Exit(1)
		}

		cwd, _ := os.Getwd()
		if err := filesystem.ConfigureForActiveAgent(cwd); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Could not configure filesystem paths: %v\n", err)
		}

		// Prepare the workspace before any command runs so it exists even before
		// the first agent request.
		if _, err := agent.EnsureWorkspace(); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Could not prepare agent workspace: %v\n", err)
		}

		// Local agent sessions treat their launch directory as the active
		// project, even when it has no marker files.
		if cmd.Name() == "chat" || cmd.Name() == "voice-classic" {
			if cwd != "" {
				if err := filesystem.SetProjectDirectory(cwd); err != nil {
					fmt.Fprintf(os.Stderr, "⚠️  Could not register current directory as project: %v\n", err)
				}
			}
		}

		// Load and watch external tools asynchronously in the background so CLI commands open instantly.
		// Chat defers this until the agent picker has resolved the active agent.
		if cmd.Name() != "chat" {
			startAgentBackgroundLoaders()
		}

		return nil
	}

	// Apply Theme to Help
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, strings []string) {
		fmt.Println()
		// Capitalize app name for the main header
		name := cmd.Name()
		if name == "myaaw" {
			name = "Myaaw"
		}
		title := theme.RenderPrimary(fmt.Sprintf("%s — %s", name, cmd.Short))
		fmt.Println(title)
		fmt.Println(theme.RenderSecondary(cmd.Long))
		fmt.Println("")
		fmt.Println(theme.RenderPrimary("Usage:"))
		fmt.Printf("  %s\n\n", cmd.UseLine())

		if len(cmd.Commands()) > 0 {
			// Custom grouping ONLY for the root command
			if cmd.Name() == "myaaw" && cmd.HasParent() == false {
				type cmdGroup struct {
					name     string
					cmdNames []string
				}

				groups := []cmdGroup{
					{name: "Core Commands", cmdNames: []string{"chat", "voice", "voice-classic", "status"}},
					{name: "Agents & API", cmdNames: []string{"agent", "api"}},
					{name: "Automation", cmdNames: []string{"cron"}},
					{name: "Backup & Restore", cmdNames: []string{"backup", "restore"}},
					{name: "Service Management", cmdNames: []string{"server", "start", "stop", "restart", "webhook"}},
					{name: "Integrations", cmdNames: []string{"provider", "channel"}},
					{name: "System & Config", cmdNames: []string{"logs", "observability", "config", "completion", "update", "version"}},
				}

				findCmd := func(name string) *cobra.Command {
					for _, c := range cmd.Commands() {
						if c.Name() == name {
							return c
						}
					}
					return nil
				}

				// Render groups
				for _, g := range groups {
					fmt.Printf("  %s\n", theme.RenderMuted(g.name+":"))
					for _, name := range g.cmdNames {
						c := findCmd(name)
						if c != nil && c.IsAvailableCommand() {
							fmt.Printf("    %-15s %s\n", theme.RenderSecondary(c.Name()), c.Short)
						}
					}
					fmt.Println("")
				}

				// Render onboard separately
				if c := findCmd("onboard"); c != nil && c.IsAvailableCommand() {
					fmt.Printf("  %s\n", theme.RenderMuted("Getting Started:"))
					fmt.Printf("    %-15s %s\n\n", theme.RenderSecondary(c.Name()), c.Short)
				}
			} else {
				// Standard layout for subcommands
				fmt.Printf("  %s\n", theme.RenderMuted("Available Commands:"))
				for _, c := range cmd.Commands() {
					if c.IsAvailableCommand() {
						fmt.Printf("    %-15s %s\n", theme.RenderSecondary(c.Name()), c.Short)
					}
				}
				fmt.Println("")
			}
		}

		if len(cmd.LocalFlags().FlagUsages()) > 0 {
			fmt.Println(theme.RenderPrimary("Flags:"))
			fmt.Println(cmd.LocalFlags().FlagUsages())
		}
	})
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
