package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	logsFollow bool
	logsTail   int
)

var logsCmd = &cobra.Command{
	Use:   "logs [filter]",
	Short: "View or stream application logs",
	Long: `Stream logs from ~/.myaaw/logs/.

Modes:
  1. Interactive: Run without arguments to select files from a menu.
  2. Filter: Provide a keyword (e.g. "chat") to stream all matching log files.`,
	Example: `  myaaw logs             # Interactive menu
  myaaw logs chat        # Stream all files with "chat" in name
  myaaw logs -n 100      # Tail last 100 lines
  myaaw logs error -f=false # Search "error" logs without following`,
	Run: runLogs,
}

func init() {
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", true, "Stream logs (tail -f)")
	logsCmd.Flags().IntVarP(&logsTail, "tail", "n", 10, "Number of lines to show from the end")
}

func runLogs(cmd *cobra.Command, args []string) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("Error getting home directory:", err)
	}

	logDir := filepath.Join(homeDir, ".myaaw", "logs")
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		fmt.Printf("No logs found. Directory %s does not exist.\n", logDir)
		return
	}

	files, err := os.ReadDir(logDir)
	if err != nil {
		log.Fatal("Error reading log directory:", err)
	}

	var targetFiles []string

	if len(args) > 0 {
		filter := strings.ToLower(args[0])

		if filter == "help" {
			cmd.Help()
			return
		}
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(file.Name(), ".log") {
				if filter == "" || strings.Contains(strings.ToLower(file.Name()), filter) {
					targetFiles = append(targetFiles, filepath.Join(logDir, file.Name()))
				}
			}
		}

		if len(targetFiles) == 0 {
			fmt.Printf("No log files found matching '%s' in %s\n", filter, logDir)
			return
		}
	} else {
		selected := selectLogFilesInteractive(files, logDir)
		if len(selected) == 0 {
			return // User quit or selected nothing
		}
		targetFiles = selected
	}

	fmt.Printf("Tail-ing %d files:\n", len(targetFiles))
	for _, f := range targetFiles {
		fmt.Printf(" - %s\n", filepath.Base(f))
	}
	fmt.Println("---")

	tailArgs := []string{}
	if logsFollow {
		tailArgs = append(tailArgs, "-f")
	}
	tailArgs = append(tailArgs, "-n", fmt.Sprintf("%d", logsTail))
	tailArgs = append(tailArgs, targetFiles...)

	tailCmd := exec.Command("tail", tailArgs...)
	tailCmd.Stdout = os.Stdout
	tailCmd.Stderr = os.Stderr

	if err := tailCmd.Run(); err != nil {
	}
}

type logItem struct {
	title, desc string
	path        string // absolute path or empty for "ALL"
}

func (i logItem) Title() string       { return i.title }
func (i logItem) Description() string { return i.desc }
func (i logItem) FilterValue() string { return i.title }

type logsModel struct {
	list     list.Model
	selected []string
	quitting bool
}

func (m logsModel) Init() tea.Cmd {
	return nil
}

func (m logsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
		if msg.String() == "enter" {
			i, ok := m.list.SelectedItem().(logItem)
			if ok {
				if i.path == "" {
					// ALL selected, return nil to indicate all files in dir (handled by caller?)
					// Actually, caller expects a list of paths.
					// Let's convention: empty string in selection means "User chose ALL"
					// But better: return all paths here if "ALL" is selected.
					// Since model doesn't know all paths easily without recalculating,
					// stick to returning special marker or handle in SelectLogFilesInteractive.
					// Let's treat empty path as marker for ALL.
					m.selected = []string{""}
				} else {
					m.selected = []string{i.path}
				}
			}
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		h, v := logsDocStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m logsModel) View() string {
	if m.quitting {
		return ""
	}
	return logsDocStyle.Render(m.list.View())
}

var logsDocStyle = lipgloss.NewStyle().Margin(1, 2)

func selectLogFilesInteractive(files []os.DirEntry, logDir string) []string {
	items := []list.Item{
		logItem{title: "All Logs", desc: "Stream all log files simultaneously", path: ""},
	}

	allPaths := []string{}

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".log") {
			fullPath := filepath.Join(logDir, file.Name())
			items = append(items, logItem{
				title: file.Name(),
				desc:  fmt.Sprintf("Stream %s", file.Name()),
				path:  fullPath,
			})
			allPaths = append(allPaths, fullPath)
		}
	}

	const defaultWidth = 20
	const listHeight = 14

	l := list.New(items, list.NewDefaultDelegate(), defaultWidth, listHeight)
	l.Title = "Select Log File to Stream"
	l.SetShowHelp(false)

	m := logsModel{list: l}

	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		fmt.Println("Error running TUI:", err)
		return nil
	}

	finalState := finalModel.(logsModel)
	if finalState.quitting || len(finalState.selected) == 0 {
		return nil
	}

	if finalState.selected[0] == "" {
		return allPaths
	}

	return finalState.selected
}
