package main

import (
	"fmt"
	"myaaw"
	"myaaw/internal/agentdir"
	"myaaw/internal/cli/theme"
	"os"
	"path/filepath"
	"strconv"

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
	Short: "Create a new isolated agent",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		if err := agentdir.ValidateName(name); err != nil {
			fmt.Println(theme.RenderError(err.Error()))
			return
		}
		if err := agentdir.EnsureLayout(); err != nil {
			fmt.Println(theme.RenderError(fmt.Sprintf("Failed to prepare layout: %v", err)))
			return
		}
		if agentdir.AgentInitialized(name) {
			fmt.Println(theme.RenderError(fmt.Sprintf("Agent %q already exists.", name)))
			return
		}

		root, err := agentdir.AgentRoot(name)
		if err != nil {
			fmt.Println(theme.RenderError(err.Error()))
			return
		}
		if err := extractEmbedDir(myaaw.DefaultMyaawDir, ".myaaw", root); err != nil {
			fmt.Println(theme.RenderError(fmt.Sprintf("Failed to create agent: %v", err)))
			return
		}
		for _, dir := range []string{
			filepath.Join(root, "home", "workspace", "projects"),
			filepath.Join(root, "home", "workspace", "downloads"),
		} {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				fmt.Println(theme.RenderError(fmt.Sprintf("Failed to create workspace: %v", err)))
				return
			}
		}

		envPath, err := agentdir.EnvPath(name)
		if err != nil {
			fmt.Println(theme.RenderError(err.Error()))
			return
		}
		ensureEnvFile(envPath)
		port := agentdir.SuggestedPort(name)
		if err := agentdir.UpsertEnvValue(envPath, "PORT", strconv.Itoa(port)); err != nil {
			fmt.Println(theme.RenderError(fmt.Sprintf("Failed to set port: %v", err)))
		}
		if err := agentdir.UpsertEnvValue(envPath, "MYAAW_BASE_URL", fmt.Sprintf("http://localhost:%d", port)); err != nil {
			fmt.Println(theme.RenderError(fmt.Sprintf("Failed to set base URL: %v", err)))
		}

		if err := agentdir.RewriteLegacyPaths(root, name); err != nil {
			fmt.Printf("⚠️  Could not rewrite legacy paths: %v\n", err)
		}
		if err := agentdir.RegisterAgent(name); err != nil {
			fmt.Println(theme.RenderError(fmt.Sprintf("Failed to register agent: %v", err)))
			return
		}
		if err := agentdir.SeedA2AContacts(name); err != nil {
			fmt.Printf("⚠️  Could not seed A2A contacts: %v\n", err)
		} else {
			fmt.Println(theme.RenderMuted("🔗 A2A contacts seeded across agents"))
		}

		fmt.Println(theme.RenderSuccess(fmt.Sprintf("✅ Agent %q created at %s", name, root)))
		fmt.Println(theme.RenderMuted(fmt.Sprintf("Configure it with: myaaw --agent %s onboard", name)))
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

		if err := os.RemoveAll(root); err != nil {
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
	agentCmd.AddCommand(agentListCmd)
	agentCmd.AddCommand(agentCreateCmd)
	agentCmd.AddCommand(agentUseCmd)
	agentCmd.AddCommand(agentDeleteCmd)
	rootCmd.AddCommand(agentCmd)
}
