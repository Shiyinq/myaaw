package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"myaaw/internal/backup"

	"github.com/spf13/cobra"
)

var (
	backupOutput           string
	backupIncludeWorkspace bool
	backupIncludeLogs      bool
	backupIncludeVenv      bool
	backupIncludeCaches    bool
	backupNoSecrets        bool
	backupJSON             bool
)

var backupCmd = &cobra.Command{
	Use:   "backup [agent ...]",
	Short: "Back up agent data to a portable tar.gz archive",
	Long: `Back up one or more agents into tar.gz archives.

By default every initialized agent is backed up, and the archive contains the
whole agent directory except workspace/ and logs/. The SQLite database is
snapshotted with VACUUM INTO so a running daemon does not produce an
inconsistent copy. Archives are written to ~/.myaaw-backups.

Use the global --agent flag (comma-separated) or positional arguments to select
specific agents.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(backupOutput) == "-" && backupJSON {
			cmd.SilenceUsage = true
			return fmt.Errorf("--json cannot be combined with -o -")
		}
		agents := splitCSV(agentFlag)
		if len(agents) == 0 && len(args) > 0 {
			agents = args
		}

		results, err := backup.Create(backup.Options{
			Agents:           agents,
			OutputDir:        backupOutput,
			IncludeWorkspace: backupIncludeWorkspace,
			IncludeLogs:      backupIncludeLogs,
			IncludeVenv:      backupIncludeVenv,
			IncludeCaches:    backupIncludeCaches,
			NoSecrets:        backupNoSecrets,
			Version:          Version,
		})
		if err != nil {
			cmd.SilenceUsage = true
			return err
		}

		if backupJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(results)
		}

		// Keep stdout clean when the archive itself is streamed there.
		out := os.Stdout
		if strings.TrimSpace(backupOutput) == "-" {
			out = os.Stderr
		}
		for _, result := range results {
			fmt.Fprintf(out, "%s: %d files, %s -> %s\n", result.Agent, result.Files, formatBytes(result.Bytes), result.Path)
		}
		return nil
	},
}

func init() {
	backupCmd.Flags().StringVarP(&backupOutput, "output", "o", "", "output directory, or - for stdout (default: ~/.myaaw-backups)")
	backupCmd.Flags().BoolVar(&backupIncludeWorkspace, "include-workspace", false, "include home/workspace")
	backupCmd.Flags().BoolVar(&backupIncludeLogs, "include-logs", false, "include logs, cron/runs, and jobs/*.log")
	backupCmd.Flags().BoolVar(&backupIncludeVenv, "include-venv", false, "include home/.venv (same platform only)")
	backupCmd.Flags().BoolVar(&backupIncludeCaches, "include-caches", false, "include .playwright-mcp and tmp")
	backupCmd.Flags().BoolVar(&backupNoSecrets, "no-secrets", false, "exclude .env and blank API keys and A2A tokens")
	backupCmd.Flags().BoolVar(&backupJSON, "json", false, "print a JSON summary")
	rootCmd.AddCommand(backupCmd)
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
