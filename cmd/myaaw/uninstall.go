package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"myaaw/internal/agentdir"
	"myaaw/internal/backup"
	"myaaw/internal/cli/theme"

	"github.com/spf13/cobra"
)

var (
	uninstallYes         bool
	uninstallDryRun      bool
	uninstallKeepBinary  bool
	uninstallPurgeBackup bool
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Stop and remove every Myaaw service, file, and the CLI binary",
	Long: "Stop every Myaaw daemon, remove ~/.myaaw, the service definitions, and the\n" +
		"CLI binary. No backup is created here: use `myaaw backup` first if you need\n" +
		"one. Existing archives under ~/.myaaw-backup are offered for deletion at the\n" +
		"end of the run.",
	Run: func(cmd *cobra.Command, args []string) {
		runUninstall()
	},
}

func init() {
	uninstallCmd.Flags().BoolVarP(&uninstallYes, "yes", "y", false, "do not ask for confirmation")
	uninstallCmd.Flags().BoolVar(&uninstallDryRun, "dry-run", false, "show what would be removed without changing anything")
	uninstallCmd.Flags().BoolVar(&uninstallKeepBinary, "keep-binary", false, "do not remove the myaaw binary")
	uninstallCmd.Flags().BoolVar(&uninstallPurgeBackup, "purge-backup", false, "delete the archives in ~/.myaaw-backup without asking")
	rootCmd.AddCommand(uninstallCmd)
}

type uninstallAgent struct {
	Name    string
	PID     int
	Running bool
}

type uninstallTargets struct {
	Home           string
	MyaawRoot      string
	Agents         []uninstallAgent
	APIRunning     bool
	WebUIRunning   bool
	WebUIInstalled bool
	DaemonFiles    []string
	BinaryFiles    []string
	BackupDir      string
	BackupItems    []string
}

func collectUninstallTargets() uninstallTargets {
	targets := uninstallTargets{}

	if home, err := os.UserHomeDir(); err == nil {
		targets.Home = home
		targets.BackupDir = defaultBackupDir(home)
		targets.BackupItems = listBackupItems(targets.BackupDir)
		targets.DaemonFiles = daemonDefinitionFiles(home)
	}
	if root, err := agentdir.MyaawRoot(); err == nil {
		targets.MyaawRoot = root
	}
	if names, err := agentdir.ListAgents(); err == nil {
		for _, name := range names {
			pid, running, _ := agentDaemonStatus(name)
			targets.Agents = append(targets.Agents, uninstallAgent{Name: name, PID: pid, Running: running})
		}
	}
	if _, running, _ := apiDaemonStatus(); running {
		targets.APIRunning = true
	}
	targets.WebUIInstalled = isWebUIInstalled()
	if _, running, _ := webUIDaemonStatus(); running {
		targets.WebUIRunning = true
	}
	targets.BinaryFiles = binaryFiles()

	return targets
}

// daemonDefinitionFiles lists service definitions left by Myaaw, including ones
// whose agent was removed from the registry.
func daemonDefinitionFiles(home string) []string {
	patterns := []string{
		filepath.Join(home, "Library", "LaunchAgents", "myaaw-*.plist"),
		filepath.Join(home, ".config", "systemd", "user", "myaaw-*.service"),
	}

	seen := map[string]bool{}
	var files []string
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		for _, match := range matches {
			if !seen[match] {
				seen[match] = true
				files = append(files, match)
			}
		}
	}
	sort.Strings(files)
	return files
}

func binaryFiles() []string {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}

	files := []string{exe}
	for _, suffix := range []string{".old", ".tmp"} {
		candidate := exe + suffix
		if _, err := os.Stat(candidate); err == nil {
			files = append(files, candidate)
		}
	}
	return files
}

// defaultBackupDir mirrors `myaaw backup`: MYAAW_BACKUP_DIR wins, otherwise the
// folder next to ~/.myaaw.
func defaultBackupDir(home string) string {
	if dir := strings.TrimSpace(os.Getenv("MYAAW_BACKUP_DIR")); dir != "" {
		return dir
	}
	return filepath.Join(home, backup.DefaultDirName)
}

func listBackupItems(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	items := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Name() == ".DS_Store" {
			continue
		}
		items = append(items, entry.Name())
	}
	sort.Strings(items)
	return items
}

func runUninstall() {
	targets := collectUninstallTargets()
	printUninstallPlan(targets)

	if uninstallDryRun {
		return
	}
	if !uninstallYes && !askYesNo("Continue with the removal?", false) {
		fmt.Println(theme.RenderMuted("Cancelled."))
		return
	}

	fmt.Println(theme.RenderPrimary("Stopping services"))
	failed := stopUninstallTargets(targets, stopUninstallDaemon, os.Stdout)

	fmt.Println(theme.RenderPrimary("Removing files"))
	// Re-scan after stopping: service managers remove their definition on stop,
	// and this also catches definitions for agents that are no longer registered.
	if targets.Home != "" {
		targets.DaemonFiles = daemonDefinitionFiles(targets.Home)
	}
	failed += removeUninstallFiles(targets, uninstallKeepBinary, os.Stdout)

	handleUninstallBackups(targets)

	if failed > 0 {
		fmt.Println(theme.RenderError(fmt.Sprintf("%d item(s) could not be removed", failed)))
		os.Exit(1)
	}
	fmt.Println(theme.RenderSuccess("Myaaw removed."))
}

func printUninstallPlan(targets uninstallTargets) {
	fmt.Println(theme.RenderPrimary("This will:"))
	for _, agent := range targets.Agents {
		if agent.Running {
			fmt.Printf("  stop agent %q (PID %d)\n", agent.Name, agent.PID)
		} else {
			fmt.Printf("  stop agent %q (not running)\n", agent.Name)
		}
	}
	if targets.APIRunning {
		fmt.Println("  stop the control plane API")
	}
	if targets.WebUIInstalled {
		if targets.WebUIRunning {
			fmt.Println("  stop the WebUI")
		} else {
			fmt.Println("  unregister the WebUI (not running)")
		}
	}
	for _, file := range targets.DaemonFiles {
		fmt.Printf("  remove %s\n", file)
	}
	if targets.MyaawRoot != "" {
		fmt.Printf("  remove %s\n", targets.MyaawRoot)
	}
	if uninstallKeepBinary {
		fmt.Println(theme.RenderMuted("  keep the myaaw binary (--keep-binary)"))
	} else {
		for _, file := range targets.BinaryFiles {
			fmt.Printf("  remove %s\n", file)
		}
	}
	if len(targets.BackupItems) > 0 {
		fmt.Printf("  ask whether to delete the archives in %s\n", targets.BackupDir)
		for _, item := range targets.BackupItems {
			fmt.Printf("    %s\n", item)
		}
	}
}

// stopUninstallTargets stops every target and prints one line per agent. The
// stop function is injected so tests never reach the real service manager.
func stopUninstallTargets(targets uninstallTargets, stop func(kind, name string) error, out io.Writer) int {
	failed := 0

	if targets.WebUIInstalled {
		if err := stop("webui", ""); err != nil && targets.WebUIRunning {
			failed++
			fmt.Fprintf(out, "  webui: %v\n", err)
		} else {
			fmt.Fprintln(out, "  webui unregistered")
		}
	}

	if targets.APIRunning {
		fmt.Fprintln(out, "  stopping the control plane API")
		if err := stop("api", ""); err != nil {
			failed++
			fmt.Fprintf(out, "  API: %v\n", err)
		} else {
			fmt.Fprintln(out, "  API stopped")
		}
	}

	for _, agent := range targets.Agents {
		if agent.Running {
			fmt.Fprintf(out, "  stopping agent %q (PID %d)\n", agent.Name, agent.PID)
			if err := stop("agent", agent.Name); err != nil {
				failed++
				fmt.Fprintf(out, "  agent %q: %v\n", agent.Name, err)
			} else {
				fmt.Fprintf(out, "  agent %q stopped\n", agent.Name)
			}
			continue
		}
		// Not running, but a stale definition may remain.
		_ = stop("agent", agent.Name)
		fmt.Fprintf(out, "  agent %q was not running\n", agent.Name)
	}

	return failed
}

func stopUninstallDaemon(kind, name string) error {
	switch kind {
	case "webui":
		return stopWebUIDaemon()
	case "api":
		return stopAPIDaemon()
	case "agent":
		return stopAgent(name)
	}
	return nil
}

// removeUninstallFiles deletes the Myaaw directories, service definitions, and
// the binary. The binary is skipped with --keep-binary; on Windows a running
// executable cannot be deleted, so that failure is reported with a hint.
func removeUninstallFiles(targets uninstallTargets, keepBinary bool, out io.Writer) int {
	failed := 0

	if targets.MyaawRoot != "" {
		if err := os.RemoveAll(targets.MyaawRoot); err != nil {
			failed++
			fmt.Fprintf(out, "  %s: %v\n", targets.MyaawRoot, err)
		} else {
			fmt.Fprintf(out, "  removed %s\n", targets.MyaawRoot)
		}
	}

	for _, file := range targets.DaemonFiles {
		if err := os.Remove(file); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			failed++
			fmt.Fprintf(out, "  %s: %v\n", file, err)
			continue
		}
		fmt.Fprintf(out, "  removed %s\n", file)
	}

	if keepBinary {
		return failed
	}

	for _, file := range targets.BinaryFiles {
		if err := os.Remove(file); err != nil {
			failed++
			fmt.Fprintf(out, "  could not remove %s: %v\n", file, err)
			if runtime.GOOS == "windows" {
				fmt.Fprintf(out, "  close every Myaaw process, then delete %s manually\n", file)
			}
			continue
		}
		fmt.Fprintf(out, "  removed %s\n", file)
	}

	return failed
}

// handleUninstallBackups offers to delete the archives that live outside
// ~/.myaaw. Nothing is deleted without an explicit yes or --purge-backup.
func handleUninstallBackups(targets uninstallTargets) {
	if len(targets.BackupItems) == 0 {
		fmt.Println(theme.RenderMuted("No backup archives found in " + targets.BackupDir))
		return
	}

	fmt.Printf("%s %s\n", theme.RenderSecondary("Backup archives:"), targets.BackupDir)
	for _, item := range targets.BackupItems {
		fmt.Printf("  %s\n", item)
	}

	remove := uninstallPurgeBackup
	if uninstallDryRun {
		if remove {
			fmt.Println(theme.RenderMuted("Would delete " + targets.BackupDir + " (--dry-run)"))
		} else {
			fmt.Println(theme.RenderMuted("Would ask whether to delete the archives above (--dry-run)"))
		}
		return
	}
	if !remove && !uninstallYes {
		remove = askYesNo("Delete every backup in "+targets.BackupDir+"?", false)
	}

	if !remove {
		fmt.Println(theme.RenderMuted("Backups kept at " + targets.BackupDir))
		return
	}

	if err := os.RemoveAll(targets.BackupDir); err != nil {
		fmt.Printf("%s: %v\n", theme.RenderError("Could not delete the backups"), err)
		return
	}
	fmt.Println(theme.RenderSuccess("Deleted " + targets.BackupDir))
}
