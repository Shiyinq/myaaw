package main

import (
	"context"
	"os"

	"myaaw/internal/agent/mcpbridge"
	"myaaw/internal/agent/tools"
	"myaaw/internal/config"

	"github.com/spf13/cobra"
)

var mcpBridgeCmd = &cobra.Command{
	Use:    "mcp-bridge",
	Short:  "Expose Myaaw ecosystem tools over MCP stdio for external runtimes",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		config.LoadBaseConfig()
		tools.LoadExternalTools()
		return mcpbridge.Serve(context.Background(), os.Stdin, os.Stdout)
	},
}
