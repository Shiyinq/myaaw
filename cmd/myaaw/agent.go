package main

import (
	"fmt"
	"myaaw/internal/agentdir"
	"myaaw/internal/cli/theme"
	"myaaw/internal/daemon"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

var agentDeleteYes bool

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage isolated Myaaw agents",
	Long:  "Create, list, select, and delete isolated agents stored under ~/.myaaw/<name>.",
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered agents",
	Run: func(cmd *cobra.Command, args []string) {
		if err := agentdir.EnsureLayout(); err != nil {
			fmt.Println(theme.RenderError(fmt.Sprintf("Failed to prepare layout: %v", err)))
			return
		}
		registry, err := agentdir.LoadRegistry()
		if err != nil {
			fmt.Println(theme.RenderError(fmt.Sprintf("Failed to load registry: %v", err)))
			return
		}
		names, err := agentdir.ListAgents()
		if err != nil {
			fmt.Println(theme.RenderError(fmt.Sprintf("Failed to list agents: %v", err)))
			return
		}
		if len(names) == 0 {
			fmt.Println(theme.RenderMuted("No agents registered. Run 'myaaw onboard' to create one."))
			return
		}
		for _, name := range names {
			marker := ""
			if name == registry.Default {
				marker = " (default)"
			}
			fmt.Printf("  %s%s\n", name, theme.RenderMuted(marker))
		}
	},
}

var agentCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create an agent with the onboarding wizard",
	Long:  "Same as `myaaw onboard` for a new agent: choose the provider, skills, tools, channels, and heartbeat, then save.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := strings.TrimSpace(args[0])
		if agentFlag != "" && agentFlag != name {
			fmt.Println(theme.RenderError("--agent and `agent create <name>` point at different agents"))
			return
		}
		if err := agentdir.ValidateName(name); err != nil {
			fmt.Println(theme.RenderError(err.Error()))
			return
		}
		taken, err := agentdir.AgentNameTaken(name)
		if err != nil {
			fmt.Println(theme.RenderError(err.Error()))
			return
		}
		if taken {
			fmt.Println(theme.RenderError(fmt.Sprintf("Agent %q already exists. Reconfigure it with: myaaw --agent %s onboard", name, name)))
			return
		}

		onboardOpts.disableToolsSet = cmd.Flags().Changed("disable-tools")
		runOnboarding(name, true)
	},
}

var agentUseCmd = &cobra.Command{
	Use:   "use [name]",
	Short: "Set the default agent",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var name string
		if len(args) == 1 {
			name = args[0]
		} else {
			names, err := agentdir.ListAgents()
			if err != nil || len(names) == 0 {
				fmt.Println(theme.RenderError("No initialized agents found."))
				return
			}
			registry, _ := agentdir.LoadRegistry()
			current := ""
			if registry != nil {
				current = registry.Default
			}
			selected, err := selectAgent("Select default agent", names, current)
			if err != nil {
				fmt.Println(theme.RenderError("Cancelled."))
				return
			}
			name = selected
		}

		if err := agentdir.ValidateName(name); err != nil {
			fmt.Println(theme.RenderError(err.Error()))
			return
		}
		if !agentdir.AgentInitialized(name) {
			fmt.Println(theme.RenderError(fmt.Sprintf("Agent %q does not exist.", name)))
			return
		}
		if err := agentdir.SetDefault(name); err != nil {
			fmt.Println(theme.RenderError(err.Error()))
			return
		}
		fmt.Println(theme.RenderSuccess(fmt.Sprintf("✅ Default agent set to %q", name)))
	},
}

var agentDeleteCmd = &cobra.Command{
	Use:   "delete [name]",
	Short: "Delete an agent and its data",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var name string
		if len(args) == 1 {
			name = args[0]
		} else {
			names, err := agentdir.ListAgents()
			if err != nil {
				fmt.Println(theme.RenderError(err.Error()))
				return
			}
			registry, _ := agentdir.LoadRegistry()
			deletable := make([]string, 0, len(names))
			for _, candidate := range names {
				if candidate == agentdir.DefaultAgentName {
					continue
				}
				if registry != nil && candidate == registry.Default {
					continue
				}
				deletable = append(deletable, candidate)
			}
			if len(deletable) == 0 {
				fmt.Println(theme.RenderError("No deletable agents found."))
				return
			}
			selected, err := selectAgent("Select agent to delete", deletable, "")
			if err != nil {
				fmt.Println(theme.RenderError("Cancelled."))
				return
			}
			name = selected
		}

		if err := agentdir.ValidateName(name); err != nil {
			fmt.Println(theme.RenderError(err.Error()))
			return
		}
		if name == agentdir.DefaultAgentName {
			fmt.Println(theme.RenderError("The default agent cannot be deleted."))
			return
		}
		registry, err := agentdir.LoadRegistry()
		if err == nil && registry.Default == name {
			fmt.Println(theme.RenderError("The current default agent cannot be deleted. Switch default first with 'myaaw agent use <name>'."))
			return
		}
		root, err := agentdir.AgentRoot(name)
		if err != nil {
			fmt.Println(theme.RenderError(err.Error()))
			return
		}
		if _, err := os.Stat(root); os.IsNotExist(err) {
			fmt.Println(theme.RenderError(fmt.Sprintf("Agent %q does not exist.", name)))
			return
		}

		confirmed := agentDeleteYes
		if !confirmed {
			err = huh.NewForm(
				huh.NewGroup(
					huh.NewConfirm().
						Title(fmt.Sprintf("Delete agent %q and all its data?", name)).
						Value(&confirmed),
				),
			).Run()
			if err != nil || !confirmed {
				fmt.Println(theme.RenderError("Deletion cancelled."))
				return
			}
		}

		// Stop the daemon before deleting files; otherwise it keeps running and
		// recreates directories such as tools/ and logs/ after the delete.
		if manager, err := daemon.NewAgentManager(name); err == nil {
			if pid, running, _ := manager.Status(); running {
				if err := manager.Stop(); err != nil {
					fmt.Println(theme.RenderError(fmt.Sprintf("Failed to stop agent server (PID %d): %v", pid, err)))
					return
				}
			}
		}

		// Drop the deleted agent from every remaining agent's contacts.
		if err := agentdir.RemoveA2AContacts(name); err != nil {
			fmt.Println(theme.RenderWarning(fmt.Sprintf("Agent deleted, but contacts were not fully cleaned up: %v", err)))
		}

		if err := removeAgentDir(root); err != nil {
			fmt.Println(theme.RenderError(fmt.Sprintf("Failed to delete agent: %v", err)))
			return
		}
		if err := agentdir.UnregisterAgent(name); err != nil {
			fmt.Println(theme.RenderError(fmt.Sprintf("Failed to update registry: %v", err)))
			return
		}
		fmt.Println(theme.RenderSuccess(fmt.Sprintf("✅ Agent %q deleted.", name)))
	},
}

// removeAgentDir deletes an agent directory and retries briefly in case a
// just-stopped daemon still recreates a runtime subdirectory.
func removeAgentDir(root string) error {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if err := os.RemoveAll(root); err != nil {
			lastErr = err
		} else if _, err := os.Stat(root); os.IsNotExist(err) {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("agent directory still exists after deletion; is the daemon still running?")
}

func selectAgent(title string, names []string, current string) (string, error) {
	options := make([]huh.Option[string], 0, len(names))
	for _, name := range names {
		label := name
		if name == current {
			label += " (default)"
		}
		options = append(options, huh.NewOption(label, name))
	}

	selected := names[0]
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Options(options...).
				Value(&selected),
		),
	).Run()
	return selected, err
}

func init() {
	agentDeleteCmd.Flags().BoolVar(&agentDeleteYes, "yes", false, "confirm agent deletion")
	registerOnboardFlags(agentCreateCmd)
	agentCmd.AddCommand(agentListCmd)
	agentCmd.AddCommand(agentCreateCmd)
	agentCmd.AddCommand(agentUseCmd)
	agentCmd.AddCommand(agentDeleteCmd)
	rootCmd.AddCommand(agentCmd)
}
