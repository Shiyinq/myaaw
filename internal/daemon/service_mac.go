package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>{{.Name}}</string>
	<key>ProgramArguments</key>
	<array>
{{range .Args}}		<string>{{.}}</string>
{{end}}	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>{{.LogFile}}</string>
	<key>StandardErrorPath</key>
	<string>{{.LogFile}}</string>
	<key>WorkingDirectory</key>
	<string>{{.HomeDir}}</string>
</dict>
</plist>
`

type plistData struct {
	Name    string
	Args    []string
	LogFile string
	HomeDir string
}

func startDarwin(m *Manager, args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	plistPath := filepath.Join(home, "Library", "LaunchAgents", m.Name+".plist")

	// Ensure LaunchAgents directory exists
	if err := os.MkdirAll(filepath.Dir(plistPath), 0755); err != nil {
		return err
	}

	// Create plist file
	f, err := os.Create(plistPath)
	if err != nil {
		return err
	}
	defer f.Close()

	fullArgs := append([]string{exe}, args...)

	data := plistData{
		Name:    m.Name,
		Args:    fullArgs,
		LogFile: m.LogFile,
		HomeDir: home,
	}

	tmpl, err := template.New("plist").Parse(plistTemplate)
	if err != nil {
		return err
	}

	if err := tmpl.Execute(f, data); err != nil {
		return err
	}

	// Unload if previously exists, ignore error
	exec.Command("launchctl", "unload", plistPath).Run()

	// Load and start
	cmd := exec.Command("launchctl", "load", "-w", plistPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to load launchd service: %w", err)
	}

	fmt.Printf("✅ %s registered and started via Launchd\n", m.Name)
	fmt.Printf("📄 Logs: %s\n", m.LogFile)

	return nil
}

func stopDarwin(m *Manager) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	plistPath := filepath.Join(home, "Library", "LaunchAgents", m.Name+".plist")

	// Unload the service
	cmd := exec.Command("launchctl", "unload", "-w", plistPath)
	if err := cmd.Run(); err != nil {
		// Ignore error if it's already unloaded or doesn't exist
	}

	// Remove plist file
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove plist file: %w", err)
	}

	fmt.Printf("🛑 %s stopped and unregistered from Launchd\n", m.Name)
	return nil
}

func statusDarwin(m *Manager) (int, bool, error) {
	// Status can be checked using launchctl list
	cmd := exec.Command("launchctl", "list")
	out, err := cmd.Output()
	if err != nil {
		return 0, false, err
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.Contains(line, m.Name) {
			parts := strings.Fields(line)
			if len(parts) >= 3 && parts[2] == m.Name {
				pidStr := parts[0]
				if pidStr != "-" && pidStr != "" {
					var pid int
					fmt.Sscanf(pidStr, "%d", &pid)
					return pid, true, nil
				}
				// Service is registered but not running
				return 0, false, nil
			}
		}
	}
	return 0, false, nil
}
