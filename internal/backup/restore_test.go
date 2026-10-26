package backup

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myaaw/internal/agentdir"
)

type tarEntry struct {
	name string
	data []byte
}

func writeTarGz(t *testing.T, path string, entries []tarEntry) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		header := &tar.Header{
			Name:     entry.name,
			Mode:     0o600,
			Size:     int64(len(entry.data)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if _, err := tw.Write(entry.data); err != nil {
			t.Fatalf("write entry: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}
}

func TestRestoreAgent(t *testing.T) {
	root := setupHome(t, "default")
	createAgentFixture(t, root, "default")
	db := createTestDatabase(t, filepath.Join(root, "default", "database", "myaaw.db"))
	db.Close()

	archives, err := Create(Options{Agents: []string{"default"}, OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	archive := archives[0].Path

	if err := os.RemoveAll(filepath.Join(root, "default")); err != nil {
		t.Fatalf("remove agent: %v", err)
	}

	result, err := Restore(RestoreOptions{Archive: archive})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if result.Agent != "default" || result.Files == 0 {
		t.Fatalf("result = %+v", result)
	}

	for _, rel := range []string{".env", "home/AGENTS.md", "config/config.json", "database/myaaw.db"} {
		if _, err := os.Stat(filepath.Join(root, "default", filepath.FromSlash(rel))); err != nil {
			t.Errorf("restored file %s: %v", rel, err)
		}
	}
	info, err := os.Stat(filepath.Join(root, "default", ".env"))
	if err != nil {
		t.Fatalf(".env: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf(".env mode = %o, want 600", info.Mode().Perm())
	}

	restored, err := sql.Open("sqlite3", filepath.Join(root, "default", "database", "myaaw.db"))
	if err != nil {
		t.Fatalf("open restored db: %v", err)
	}
	defer restored.Close()
	var value string
	if err := restored.QueryRow(`SELECT value FROM memories`).Scan(&value); err != nil {
		t.Fatalf("query restored db: %v", err)
	}
	if value != "kept" {
		t.Fatalf("restored value = %q", value)
	}

	registry, err := agentdir.LoadRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	if _, ok := registry.Agents["default"]; !ok {
		t.Fatal("restored agent is not registered")
	}
}

func TestRestoreRefusesExistingWithoutForce(t *testing.T) {
	root := setupHome(t, "default")
	createAgentFixture(t, root, "default")

	archives, err := Create(Options{Agents: []string{"default"}, OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	writeFile(t, filepath.Join(root, "default", ".env"), "CHANGED=1\n", 0o600)

	if _, err := Restore(RestoreOptions{Archive: archives[0].Path}); err == nil {
		t.Fatal("Restore should refuse to overwrite an existing agent")
	}
	data, err := os.ReadFile(filepath.Join(root, "default", ".env"))
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	if string(data) != "CHANGED=1\n" {
		t.Fatalf(".env was modified: %q", data)
	}
}

func TestRestoreForceCreatesPreRestoreBackup(t *testing.T) {
	root := setupHome(t, "default")
	createAgentFixture(t, root, "default")

	archives, err := Create(Options{Agents: []string{"default"}, OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	writeFile(t, filepath.Join(root, "default", ".env"), "CHANGED=1\n", 0o600)

	result, err := Restore(RestoreOptions{Archive: archives[0].Path, Force: true})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if result.PreRestoreBackup == "" {
		t.Fatal("pre-restore backup was not created")
	}
	if _, err := os.Stat(result.PreRestoreBackup); err != nil {
		t.Fatalf("pre-restore backup: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "default", ".env"))
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	if strings.Contains(string(data), "CHANGED") {
		t.Fatalf(".env was not restored: %q", data)
	}
}

func TestRestoreRejectsPathTraversal(t *testing.T) {
	root := setupHome(t, "default")
	writeFile(t, filepath.Join(root, "default", "config", "config.json"), "{}", 0o600)

	archive := filepath.Join(t.TempDir(), "evil.tar.gz")
	writeTarGz(t, archive, []tarEntry{{name: "../evil.txt", data: []byte("x")}})

	if _, err := Restore(RestoreOptions{Archive: archive, Force: true}); err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("err = %v, want unsafe path error", err)
	}
	if _, err := os.Stat(filepath.Join(root, "..", "evil.txt")); err == nil {
		t.Fatal("path traversal wrote outside the staging directory")
	}
}

func TestRestoreRejectsChecksumMismatch(t *testing.T) {
	root := setupHome(t, "default")
	writeFile(t, filepath.Join(root, "default", "config", "config.json"), "{}", 0o600)

	manifest := Manifest{
		Format: FormatVersion,
		Agent:  "default",
		Files: []FileEntry{{
			Path:   "default/.env",
			Size:   3,
			Mode:   "0600",
			SHA256: strings.Repeat("0", 64),
		}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	archive := filepath.Join(t.TempDir(), "bad.tar.gz")
	writeTarGz(t, archive, []tarEntry{
		{name: "manifest.json", data: data},
		{name: "default/.env", data: []byte("abc")},
	})

	if _, err := Restore(RestoreOptions{Archive: archive, Force: true}); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("err = %v, want checksum error", err)
	}
}

func TestRestoreRejectsUnlistedFile(t *testing.T) {
	root := setupHome(t, "default")
	writeFile(t, filepath.Join(root, "default", "config", "config.json"), "{}", 0o600)

	content := []byte("abc")
	sum := sha256.Sum256(content)
	manifest := Manifest{
		Format: FormatVersion,
		Agent:  "default",
		Files: []FileEntry{{
			Path:   "default/.env",
			Size:   int64(len(content)),
			Mode:   "0600",
			SHA256: hex.EncodeToString(sum[:]),
		}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	archive := filepath.Join(t.TempDir(), "extra.tar.gz")
	writeTarGz(t, archive, []tarEntry{
		{name: "manifest.json", data: data},
		{name: "default/.env", data: content},
		{name: "default/extra.txt", data: []byte("unlisted")},
	})

	if _, err := Restore(RestoreOptions{Archive: archive, Force: true}); err == nil || !strings.Contains(err.Error(), "unlisted") {
		t.Fatalf("err = %v, want unlisted file error", err)
	}
}

func TestRestoreGlobalArchive(t *testing.T) {
	root := setupHome(t, "default")
	createAgentFixture(t, root, "default")
	writeFile(t, filepath.Join(root, "controlplane.json"), `{"username":"admin"}`, 0o600)

	archives, err := Create(Options{OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	var globalArchive string
	for _, archive := range archives {
		if archive.Agent == "global" {
			globalArchive = archive.Path
		}
	}
	if globalArchive == "" {
		t.Fatal("global archive was not created")
	}

	writeFile(t, filepath.Join(root, "agents.json"), `{"changed":true}`, 0o600)
	writeFile(t, filepath.Join(root, "controlplane.json"), `{"changed":true}`, 0o600)

	result, err := Restore(RestoreOptions{Archive: globalArchive, Force: true})
	if err != nil {
		t.Fatalf("Restore global: %v", err)
	}
	if result.Agent != "global" {
		t.Fatalf("agent = %q", result.Agent)
	}

	data, err := os.ReadFile(filepath.Join(root, "agents.json"))
	if err != nil {
		t.Fatalf("read agents.json: %v", err)
	}
	if strings.Contains(string(data), "changed") {
		t.Fatalf("agents.json was not restored: %s", data)
	}
	previous, err := filepath.Glob(filepath.Join(root, "agents.json.pre-restore-*"))
	if err != nil || len(previous) == 0 {
		t.Fatalf("pre-restore copy of agents.json was not created: %v", err)
	}
}

func TestInertAgentDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "tools"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, filepath.Join(dir, "logs", "daemon.log"), "log\n", 0o644)
	if !inertAgentDir(dir) {
		t.Fatal("empty runtime dirs and logs should be inert")
	}

	writeFile(t, filepath.Join(dir, "tools", "tools.json"), "{}\n", 0o644)
	if inertAgentDir(dir) {
		t.Fatal("a directory with real files is not inert")
	}
}

func TestRestoreReplacesInertLogsDirWithoutForce(t *testing.T) {
	root := setupHome(t, "default")
	createAgentFixture(t, root, "default")

	archives, err := Create(Options{Agents: []string{"default"}, OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(root, "default")); err != nil {
		t.Fatalf("remove agent: %v", err)
	}
	writeFile(t, filepath.Join(root, "default", "logs", "tools.log"), "old log\n", 0o644)
	if err := os.MkdirAll(filepath.Join(root, "default", "tools"), 0o755); err != nil {
		t.Fatalf("mkdir tools: %v", err)
	}

	if _, err := Restore(RestoreOptions{Archive: archives[0].Path}); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "default", "home", "AGENTS.md")); err != nil {
		t.Fatalf("restored agent: %v", err)
	}
}

func TestRestoreAgentOverride(t *testing.T) {
	root := setupHome(t, "default")
	createAgentFixture(t, root, "default")

	archives, err := Create(Options{Agents: []string{"default"}, OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(root, "default")); err != nil {
		t.Fatalf("remove agent: %v", err)
	}

	result, err := Restore(RestoreOptions{Archive: archives[0].Path, Agent: "renamed"})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if result.Agent != "renamed" {
		t.Fatalf("agent = %q", result.Agent)
	}
	if _, err := os.Stat(filepath.Join(root, "renamed", "home", "AGENTS.md")); err != nil {
		t.Fatalf("renamed agent: %v", err)
	}
	registry, err := agentdir.LoadRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	if _, ok := registry.Agents["renamed"]; !ok {
		t.Fatal("renamed agent is not registered")
	}
}
