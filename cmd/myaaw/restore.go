package main

import (
	"encoding/json"
	"fmt"
	"os"

	"myaaw/internal/agentdir"
	"myaaw/internal/backup"
	"myaaw/internal/daemon"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	restoreForce    bool
	restorePackages bool
	restoreNoStart  bool
	restoreJSON     bool
	restoreDir      string
)

var restoreCmd = &cobra.Command{
	Use:   "restore [archive]",
	Short: "Restore an agent from a backup archive",
	Long: `Restore an agent from an archive created by "myaaw backup".

The archive is verified against manifest.json (SHA-256) before anything is
written. If the agent already exists it is refused unless --force is given; in
that case a pre-restore backup is written first and the running daemon is
stopped and restarted around the restore. Use the global --agent flag to restore
under a different agent name.

Without an archive path, restore searches the backup directory (--dir,
MYAAW_BACKUP_DIR, or ~/.myaaw-backups) and shows an interactive picker.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		archive := ""
		if len(args) == 1 {
			archive = args[0]
		} else {
			dir, err := backup.ResolveOutputDir(restoreDir)
			if err != nil {
				cmd.SilenceUsage = true
				return err
			}
			archives, err := backup.ListArchives(dir)
			if err != nil {
				cmd.SilenceUsage = true
				if os.IsNotExist(err) {
					return fmt.Errorf("no backups directory at %s; pass an archive path", dir)
				}
				return err
			}
			if len(archives) == 0 {
				cmd.SilenceUsage = true
				return fmt.Errorf("no backups found in %s; pass an archive path", dir)
			}
			if !stdinIsTerminal() {
				cmd.SilenceUsage = true
				return fmt.Errorf("no archive given; pass a path or run interactively to choose from %d backup(s) in %s", len(archives), dir)
			}
			selected, err := selectArchive(archives)
			if err != nil {
				cmd.SilenceUsage = true
				return fmt.Errorf("cancelled")
			}
			archive = selected
		}

		result, err := backup.Restore(backup.RestoreOptions{
			Archive:         archive,
			Agent:           agentFlag,
			Force:           restoreForce,
			RestorePackages: restorePackages,
			Start:           !restoreNoStart,
			Version:         Version,
			StopAgent:       stopAgentDaemon,
			StartAgent:      startAgentDaemon,
		})
		if err != nil {
			cmd.SilenceUsage = true
			return err
		}

		if restoreJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}

		fmt.Printf("Restored %s from %s: %d files\n", result.Agent, archive, result.Files)
		if result.PreRestoreBackup != "" {
			fmt.Printf("Pre-restore backup: %s\n", result.PreRestoreBackup)
		}
		if result.PortWarning != "" {
			fmt.Printf("Warning: %s\n", result.PortWarning)
		}
		if result.Note != "" {
			fmt.Printf("Note: %s\n", result.Note)
		}
		return nil
	},
}

func init() {
	restoreCmd.Flags().BoolVar(&restoreForce, "force", false, "replace the agent if it already exists")
	restoreCmd.Flags().BoolVar(&restorePackages, "restore-packages", false, "rebuild home/.venv from the archived pip freeze")
	restoreCmd.Flags().BoolVar(&restoreNoStart, "no-start", false, "do not start the agent daemon after restoring")
	restoreCmd.Flags().BoolVar(&restoreJSON, "json", false, "print a JSON summary")
	restoreCmd.Flags().StringVar(&restoreDir, "dir", "", "directory to search for backups when no archive is given")
	rootCmd.AddCommand(restoreCmd)
}

func selectArchive(archives []backup.ArchiveInfo) (string, error) {
	options := make([]huh.Option[string], 0, len(archives))
	for _, archive := range archives {
		label := fmt.Sprintf("%s  (%s, %s)", archive.Name, formatBytes(archive.Size), archive.CreatedAt.Format("2006-01-02 15:04"))
		options = append(options, huh.NewOption(label, archive.Path))
	}

	selected := archives[0].Path
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select backup to restore (↑/↓, enter)").
				Options(options...).
				Value(&selected),
		),
	).Run()
	return selected, err
}

func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func stopAgentDaemon(name string) (bool, error) {
	if !agentdir.AgentInitialized(name) {
		return false, nil
	}
	manager, err := daemon.NewAgentManager(name)
	if err != nil {
		return false, err
	}
	_, running, err := manager.Status()
	if err != nil {
		return false, err
	}
	if !running {
		return false, nil
	}
	if err := manager.Stop(); err != nil {
		return false, err
	}
	return true, nil
}

func startAgentDaemon(name string) error {
	manager, err := daemon.NewAgentManager(name)
	if err != nil {
		return err
	}
	return manager.Start([]string{"--agent", name, "server", "run"})
}
