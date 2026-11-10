package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestDaemonDefinitionFiles(t *testing.T) {
	home := t.TempDir()

	plist := filepath.Join(home, "Library", "LaunchAgents", "myaaw-server-agent-a.plist")
	unit := filepath.Join(home, ".config", "systemd", "user", "myaaw-api.service")
	unrelated := filepath.Join(home, "Library", "LaunchAgents", "com.example.other.plist")
	for _, file := range []string{plist, unit, unrelated} {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatalf("create dir: %v", err)
		}
		if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", file, err)
		}
	}

	got := daemonDefinitionFiles(home)
	want := []string{plist, unit}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("daemonDefinitionFiles = %v, want %v", got, want)
	}
}

func TestListBackupItems(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"2026-01-02", "2026-01-01", ".DS_Store"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatalf("create backup dir: %v", err)
		}
	}

	got := listBackupItems(dir)
	want := []string{"2026-01-01", "2026-01-02"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("listBackupItems = %v, want %v", got, want)
	}

	if got := listBackupItems(filepath.Join(dir, "missing")); len(got) != 0 {
		t.Fatalf("missing directory should be empty, got %v", got)
	}
}

func TestStopUninstallTargetsPrintsEveryAgent(t *testing.T) {
	targets := uninstallTargets{
		Agents: []uninstallAgent{
			{Name: "agent-a", PID: 11, Running: true},
			{Name: "agent-b"},
			{Name: "agent-c", PID: 33, Running: true},
		},
		APIRunning:     true,
		WebUIInstalled: true,
		WebUIRunning:   true,
	}

	var calls []string
	var out bytes.Buffer
	failed := stopUninstallTargets(targets, func(kind, name string) error {
		calls = append(calls, kind+":"+name)
		return nil
	}, &out)

	want := []string{"webui:", "api:", "agent:agent-a", "agent:agent-b", "agent:agent-c"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("stop calls = %v, want %v", calls, want)
	}
	if failed != 0 {
		t.Fatalf("failed = %d, want 0", failed)
	}

	output := out.String()
	for _, fragment := range []string{
		`agent "agent-a" stopped`,
		`agent "agent-b" was not running`,
		`agent "agent-c" stopped`,
	} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("output missing %q:\n%s", fragment, output)
		}
	}
}

func TestStopUninstallTargetsCountsFailures(t *testing.T) {
	targets := uninstallTargets{
		Agents:     []uninstallAgent{{Name: "agent-a", PID: 11, Running: true}},
		APIRunning: true,
	}

	var out bytes.Buffer
	failed := stopUninstallTargets(targets, func(kind, name string) error {
		if kind == "agent" {
			return os.ErrPermission
		}
		return nil
	}, &out)

	if failed != 1 {
		t.Fatalf("failed = %d, want 1", failed)
	}
}

func TestRemoveUninstallFiles(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".myaaw")
	if err := os.MkdirAll(filepath.Join(root, "agents", "agent-a"), 0o755); err != nil {
		t.Fatalf("create root: %v", err)
	}

	plist := filepath.Join(home, "Library", "LaunchAgents", "myaaw-server-agent-a.plist")
	if err := os.MkdirAll(filepath.Dir(plist), 0o755); err != nil {
		t.Fatalf("create plist dir: %v", err)
	}
	if err := os.WriteFile(plist, []byte("x"), 0o644); err != nil {
		t.Fatalf("write plist: %v", err)
	}

	binaryDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binaryDir, 0o755); err != nil {
		t.Fatalf("create bin dir: %v", err)
	}
	binaryPath := filepath.Join(binaryDir, "myaaw")
	if err := os.WriteFile(binaryPath, []byte("x"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	targets := uninstallTargets{
		MyaawRoot:   root,
		DaemonFiles: []string{plist},
		BinaryFiles: []string{binaryPath},
	}

	var out bytes.Buffer
	if failed := removeUninstallFiles(targets, false, &out); failed != 0 {
		t.Fatalf("failed = %d:\n%s", failed, out.String())
	}
	for _, path := range []string{root, plist, binaryPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s still exists", path)
		}
	}
}

func TestRemoveUninstallFilesKeepsBinary(t *testing.T) {
	home := t.TempDir()
	binaryPath := filepath.Join(home, "myaaw")
	if err := os.WriteFile(binaryPath, []byte("x"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	targets := uninstallTargets{BinaryFiles: []string{binaryPath}}
	var out bytes.Buffer
	if failed := removeUninstallFiles(targets, true, &out); failed != 0 {
		t.Fatalf("failed = %d", failed)
	}
	if _, err := os.Stat(binaryPath); err != nil {
		t.Fatalf("binary should be kept: %v", err)
	}
}

func TestCollectUninstallTargets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	agentDir := filepath.Join(home, ".myaaw", "agents", "uninstall-test-a", "config")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("create agent dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "config.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write agent config: %v", err)
	}

	registryDir := filepath.Join(home, ".myaaw", "config")
	if err := os.MkdirAll(registryDir, 0o755); err != nil {
		t.Fatalf("create registry dir: %v", err)
	}
	registry := `{"default":"uninstall-test-a","agents":{"uninstall-test-a":{}}}`
	if err := os.WriteFile(filepath.Join(registryDir, "agents.json"), []byte(registry), 0o644); err != nil {
		t.Fatalf("write registry: %v", err)
	}

	plist := filepath.Join(home, "Library", "LaunchAgents", "myaaw-server-uninstall-test-a.plist")
	if err := os.MkdirAll(filepath.Dir(plist), 0o755); err != nil {
		t.Fatalf("create plist dir: %v", err)
	}
	if err := os.WriteFile(plist, []byte("x"), 0o644); err != nil {
		t.Fatalf("write plist: %v", err)
	}

	backupItem := filepath.Join(home, ".myaaw-backups", "2026-01-01")
	if err := os.MkdirAll(backupItem, 0o755); err != nil {
		t.Fatalf("create backup: %v", err)
	}

	targets := collectUninstallTargets()

	if targets.MyaawRoot != filepath.Join(home, ".myaaw") {
		t.Fatalf("root = %q", targets.MyaawRoot)
	}
	var names []string
	for _, agent := range targets.Agents {
		names = append(names, agent.Name)
	}
	if !reflect.DeepEqual(names, []string{"uninstall-test-a"}) {
		t.Fatalf("agents = %v", names)
	}
	if len(targets.DaemonFiles) != 1 || targets.DaemonFiles[0] != plist {
		t.Fatalf("daemon files = %v", targets.DaemonFiles)
	}
	if !reflect.DeepEqual(targets.BackupItems, []string{"2026-01-01"}) {
		t.Fatalf("backup items = %v", targets.BackupItems)
	}
	if want := filepath.Join(home, ".myaaw-backups"); targets.BackupDir != want {
		t.Fatalf("backup dir = %q, want %q", targets.BackupDir, want)
	}
}

func TestDefaultBackupDir(t *testing.T) {
	home := t.TempDir()

	t.Setenv("MYAAW_BACKUP_DIR", "")
	if got, want := defaultBackupDir(home), filepath.Join(home, ".myaaw-backups"); got != want {
		t.Fatalf("defaultBackupDir = %q, want %q", got, want)
	}

	custom := filepath.Join(home, "elsewhere")
	t.Setenv("MYAAW_BACKUP_DIR", custom)
	if got := defaultBackupDir(home); got != custom {
		t.Fatalf("defaultBackupDir with env = %q, want %q", got, custom)
	}
}
