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
	"path/filepath"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "myaaw",
	Short: "Personal AI Assistant",
	Long:  "😼 A Cat who becomes your personal AI assistant.",
}

func init() {
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(chatCmd)
	rootCmd.AddCommand(voiceCmd)
	rootCmd.AddCommand(voiceClassicCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(webhookCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(logsCmd)

	rootCmd.PersistentFlags().BoolVarP(&config.Verbose, "verbose", "v", false, "enable verbose logging")

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// Add global spacing
		fmt.Println()

		// Commands that don't require .myaaw configuration
		metaCommands := map[string]bool{
			"onboard": true,
			"version": true,
			"help":    true,
			"update":  true,
		}

		if metaCommands[cmd.Name()] {
			return nil
		}

		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil // Fallback to system env if home unknown
		}

		myaawPath := filepath.Join(homeDir, ".myaaw")
		if _, err := os.Stat(myaawPath); os.IsNotExist(err) {
			fmt.Println("⚠️  Myaaw is not initialized!")
			fmt.Println("Please run 'myaaw onboard' to setup your configuration and features.")
			os.Exit(1)
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
					{name: "Automation", cmdNames: []string{"cron"}},
					{name: "Service Management", cmdNames: []string{"server", "webhook"}},
					{name: "System & Config", cmdNames: []string{"logs", "config", "completion", "update", "version"}},
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
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
