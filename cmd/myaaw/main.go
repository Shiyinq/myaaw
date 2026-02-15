package main

import (
	"fmt"
	"os"

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
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
