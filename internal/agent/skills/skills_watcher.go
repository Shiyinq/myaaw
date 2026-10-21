package skills

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"myaaw/internal/agent"
	"myaaw/internal/agentdir"

	"github.com/fsnotify/fsnotify"
)

var (
	// SkillsLogger writes to the active agent's logs/skills.log, separate from
	// the tools log, so skill enable/disable activity can be monitored on its own.
	SkillsLogger  *log.Logger
	skillsLogFile *os.File
	skillsWatcher *fsnotify.Watcher
)

func init() {
	SkillsLogger = log.New(io.Discard, "", 0)
	ConfigureLogging()
}

// ConfigureLogging reopens skills.log for the active agent.
func ConfigureLogging() {
	if skillsLogFile != nil {
		_ = skillsLogFile.Close()
		skillsLogFile = nil
	}

	logDir, err := agentdir.ActiveLogsDir()
	if err != nil {
		SkillsLogger = log.New(io.Discard, "", 0)
		return
	}
	_ = os.MkdirAll(logDir, 0755)
	f, err := os.OpenFile(filepath.Join(logDir, "skills.log"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		SkillsLogger = log.New(io.Discard, "", 0)
		return
	}
	skillsLogFile = f
	SkillsLogger = log.New(f, "", log.LstdFlags)
}

// getEnabledSkillNames returns the sorted directory names of all currently
// enabled skills (respecting skills.json and the SKILL.md frontmatter).
func getEnabledSkillNames() []string {
	skillsDir, err := agentdir.ActiveSkillsDir()
	if err != nil {
		return nil
	}
	skills, err := listEnabledSkills(skillsDir, loadSkillsFilter(skillsDir))
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(skills))
	for _, skill := range skills {
		names = append(names, skill.dirName)
	}
	sort.Strings(names)
	return names
}

// LogActiveSkills logs the count and names of all currently enabled skills.
func LogActiveSkills() {
	names := getEnabledSkillNames()
	SkillsLogger.Printf("Total active skills: %d | Skills: %v", len(names), names)
}

// WatchSkills watches the skills directory (skills.json included) for changes
// and logs the resulting enabled/disabled skill state, mirroring the behavior
// of the tools watcher. Logs go to ~/.myaaw/logs/skills.log.
func WatchSkills() {
	// Log the initial state at startup
	LogActiveSkills()

	skillsDir, err := agentdir.ActiveSkillsDir()
	if err != nil {
		SkillsLogger.Printf("Failed to resolve skills directory: %v", err)
		return
	}
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		return
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		SkillsLogger.Printf("Failed to create skills watcher: %v", err)
		return
	}
	skillsWatcher = watcher

	if err := watcher.Add(skillsDir); err != nil {
		SkillsLogger.Printf("Failed to watch skills directory: %v", err)
		watcher.Close()
		return
	}

	var debounceTimer *time.Timer
	debounceDuration := 500 * time.Millisecond

	go func() {
		defer watcher.Close()
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Chmod) || event.Has(fsnotify.Remove) {
					if debounceTimer != nil {
						debounceTimer.Stop()
					}
					debounceTimer = time.AfterFunc(debounceDuration, func() {
						SkillsLogger.Println("Skills configuration changed, reloading...")
						logSkillStates()
					})
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				SkillsLogger.Printf("Skills watcher error: %v", err)
			}
		}
	}()
}

// logSkillStates logs the skills explicitly disabled by config and the
// currently active skill set.
func logSkillStates() {
	myaawPath, err := agent.EnsureMyaawConfig()
	if err != nil {
		return
	}
	skillsDir := filepath.Join(myaawPath, "skills")
	filter := loadSkillsFilter(skillsDir)

	disabled := make([]string, 0)
	for name, enabled := range filter {
		if !enabled {
			disabled = append(disabled, name)
		}
	}
	sort.Strings(disabled)
	for _, name := range disabled {
		SkillsLogger.Printf("Skill '%s' disabled by config", name)
	}

	LogActiveSkills()
}
