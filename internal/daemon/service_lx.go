package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

const systemdTemplate = `[Unit]
Description={{.Name}}
After=network.target

[Service]
ExecStart={{.Exe}} {{.ArgsJoined}}
WorkingDirectory={{.HomeDir}}
StandardOutput=append:{{.LogFile}}
StandardError=append:{{.LogFile}}
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
`

type systemdData struct {
	Name       string
	Exe        string
	ArgsJoined string
	LogFile    string
	HomeDir    string
}

func startLinux(m *Manager, args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	serviceDir := filepath.Join(home, ".config", "systemd", "user")
	servicePath := filepath.Join(serviceDir, m.Name+".service")

	// Ensure systemd user directory exists
	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		return err
	}

	// Create service file
	f, err := os.Create(servicePath)
	if err != nil {
		return err
	}
	defer f.Close()

	data := systemdData{
		Name:       m.Name,
		Exe:        exe,
		ArgsJoined: strings.Join(args, " "),
		LogFile:    m.LogFile,
		HomeDir:    home,
	}

	tmpl, err := template.New("systemd").Parse(systemdTemplate)
	if err != nil {
		return err
	}

	if err := tmpl.Execute(f, data); err != nil {
		return err
	}

	// Reload systemd, enable, and start
	exec.Command("systemctl", "--user", "daemon-reload").Run()

	cmdEnable := exec.Command("systemctl", "--user", "enable", m.Name)
	if err := cmdEnable.Run(); err != nil {
		return fmt.Errorf("failed to enable systemd service: %w", err)
	}

	cmdStart := exec.Command("systemctl", "--user", "start", m.Name)
	if err := cmdStart.Run(); err != nil {
		return fmt.Errorf("failed to start systemd service: %w", err)
	}

	fmt.Printf("✅ %s registered and started via Systemd\n", m.Name)
	fmt.Printf("📄 Logs: %s\n", m.LogFile)

	return nil
}

func stopLinux(m *Manager) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	servicePath := filepath.Join(home, ".config", "systemd", "user", m.Name+".service")

	// Stop and disable the service
	exec.Command("systemctl", "--user", "stop", m.Name).Run()
	exec.Command("systemctl", "--user", "disable", m.Name).Run()

	// Remove service file
	if err := os.Remove(servicePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove service file: %w", err)
	}

	exec.Command("systemctl", "--user", "daemon-reload").Run()

	fmt.Printf("🛑 %s stopped and unregistered from Systemd\n", m.Name)
	return nil
}

func statusLinux(m *Manager) (int, bool, error) {
	cmd := exec.Command("systemctl", "--user", "show", "-p", "MainPID", "-p", "SubState", m.Name)
	out, err := cmd.Output()
	if err != nil {
		return 0, false, nil // Not found or error
	}

	outStr := string(out)

	isRunning := strings.Contains(outStr, "SubState=running")
	if !isRunning {
		return 0, false, nil
	}

	lines := strings.Split(outStr, "\n")
	var pid int
	for _, line := range lines {
		if strings.HasPrefix(line, "MainPID=") {
			fmt.Sscanf(line, "MainPID=%d", &pid)
			break
		}
	}

	if pid > 0 {
		return pid, true, nil
	}

	return 0, false, nil
}
