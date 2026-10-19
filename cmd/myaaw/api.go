package main

import (
	"log"

	"myaaw/internal/services/controlplane"

	"github.com/spf13/cobra"
)

var apiPort string

var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Run the unified control plane API",
	Long:  "Run the single API gateway used by web, desktop, and mobile clients. Chat is proxied to the selected agent server.",
	Run: func(cmd *cobra.Command, args []string) {
		if err := controlplane.Run(":" + apiPort); err != nil {
			log.Fatalf("API server error: %v", err)
		}
	},
}

func init() {
	apiCmd.Flags().StringVar(&apiPort, "port", "9000", "port to listen on")
	rootCmd.AddCommand(apiCmd)
}
