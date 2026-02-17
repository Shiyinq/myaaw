package main

import (
	"fmt"
	"myaaw/internal/config"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "myaaw",
	Short: "Myaaw — Personal AI Assistant",
	Long:  "Myaaw — Integrate your favorite LLM with Telegram, Discord, and more.\nRun as a server, consumer, or chat directly from the terminal.",
}

func init() {
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(consumerCmd)
	rootCmd.AddCommand(chatCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(webhookCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(dockerCmd)
	rootCmd.AddCommand(versionCmd)

	rootCmd.PersistentFlags().BoolVarP(&config.Verbose, "verbose", "v", false, "enable verbose logging")

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
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
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
