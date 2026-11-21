package backup

import (
	"archive/tar"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myaaw/internal/agentdir"
	"myaaw/internal/obs"
)

func setupHome(t *testing.T, agents ...string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	root := filepath.Join(home, ".myaaw")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	registry := &agentdir.Registry{Default: agents[0], Agents: map[string]agentdir.AgentInfo{}}
	for _, name := range agents {
		registry.Agents[name] = agentdir.AgentInfo{CreatedAt: "2026-01-01"}
		if err := os.MkdirAll(filepath.Join(root, "agents", name), 0o755); err != nil {
			t.Fatalf("mkdir agent: %v", err)
		}
	}
	if err := agentdir.SaveRegistry(registry); err != nil {
		t.Fatalf("save registry: %v", err)
	}
	return root
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// createAgentFixture builds a representative agent tree.
func createAgentFixture(t *testing.T, root, name string) {
	t.Helper()
	base := filepath.Join(root, "agents", name)
	writeFile(t, filepath.Join(base, "config", "config.json"), `{
  "default_provider": "deepseek",
  "providers": {"deepseek": {"type": "openai", "api_key": "SECRET-KEY", "base_url": "https://x"}}
}`, 0o600)
	writeFile(t, filepath.Join(base, ".env"), "PORT=8080\nTAVILY_API_KEY=SECRET-ENV\n", 0o600)
	writeFile(t, filepath.Join(base, "contacts.json"), `[{"name":"agent-b","url":"http://localhost:8082/a2a","token":"SECRET-TOKEN"}]`, 0o600)
	writeFile(t, filepath.Join(base, "home", "AGENTS.md"), "# agents\n", 0o644)
	writeFile(t, filepath.Join(base, "home", "memory", "RULES.md"), "# rules\n", 0o644)
	writeFile(t, filepath.Join(base, "home", "workspace", "project.txt"), "workspace data\n", 0o644)
	writeFile(t, filepath.Join(base, "home", ".logs", "run.log"), "log\n", 0o644)
	writeFile(t, filepath.Join(base, "home", ".venv", "bin", "pip"), "#!/bin/sh\necho 'requests==2.0'\n", 0o755)
	writeFile(t, filepath.Join(base, "home", ".venv", "bin", "python"), "#!/bin/sh\necho 'Python 3.12.0'\n", 0o755)
	writeFile(t, filepath.Join(base, "home", ".venv", "lib", "big.bin"), strings.Repeat("x", 1024), 0o644)
	writeFile(t, filepath.Join(base, "skills", "foo", "SKILL.md"), "# skill\n", 0o644)
	writeFile(t, filepath.Join(base, "tools", "tools.json"), "{}\n", 0o644)
	writeFile(t, filepath.Join(base, "cron", "jobs.json"), "{}\n", 0o644)
	writeFile(t, filepath.Join(base, "cron", "runs", "run.jsonl"), "{}\n", 0o644)
	writeFile(t, filepath.Join(base, "jobs", "job_1.log"), "job log\n", 0o644)
	writeFile(t, filepath.Join(base, "jobs", "a2a", "t1", "output", "result.txt"), "artifact\n", 0o644)
	writeFile(t, filepath.Join(base, "logs", "server.log"), "server log\n", 0o644)
	writeFile(t, filepath.Join(base, "database", "notes", "n.json"), `{"note":"x"}`, 0o644)
	writeFile(t, filepath.Join(base, ".DS_Store"), "junk", 0o644)
}

func createTestDatabase(t *testing.T, path string) *sql.DB {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir db: %v", err)
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		t.Fatalf("wal: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE memories (id INTEGER PRIMARY KEY, value TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO memories (value) VALUES ('kept')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	return db
}

func readArchive(t *testing.T, path string) (map[string][]byte, map[string]*tar.Header) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	tr := tar.NewReader(gz)
	files := map[string][]byte{}
	headers := map[string]*tar.Header{}
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		copyHeader := *header
		headers[header.Name] = &copyHeader
		if header.Typeflag != tar.TypeReg {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read %s: %v", header.Name, err)
		}
		files[header.Name] = data
	}
	return files, headers
}

func TestCreateSnapshotsObservabilityDatabase(t *testing.T) {
	root := setupHome(t, "default")
	createAgentFixture(t, root, "default")

	dbPath := filepath.Join(root, "agents", "default", "database", "observability.db")
	store, err := obs.Open(dbPath, obs.Options{})
	if err != nil {
		t.Fatalf("open observability db: %v", err)
	}
	if err := store.UpsertRun(obs.Run{
		ID: "run_traced", StartedAt: 1_700_000_000_000, Status: obs.StatusCompleted,
		Model: "deepseek-flash", ConversationID: "conv-1", InputTokens: 10,
	}); err != nil {
		t.Fatalf("upsert run: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	// Side files of a live WAL database: they must never be archived raw.
	writeFile(t, dbPath+"-wal", "stale wal", 0o600)
	writeFile(t, dbPath+"-shm", "stale shm", 0o600)

	results, err := Create(Options{Agents: []string{"default"}, OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	files, _ := readArchive(t, results[0].Path)

	if _, ok := files["default/database/observability.db"]; !ok {
		t.Fatal("archive is missing the observability snapshot")
	}
	for _, unwanted := range []string{
		"default/database/observability.db-wal",
		"default/database/observability.db-shm",
	} {
		if _, ok := files[unwanted]; ok {
			t.Errorf("archive should not contain %s", unwanted)
		}
	}

	snapshotPath := filepath.Join(t.TempDir(), "observability.db")
	if err := os.WriteFile(snapshotPath, files["default/database/observability.db"], 0o600); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	snapshot, err := sql.Open("sqlite3", snapshotPath)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer snapshot.Close()
	var count int
	if err := snapshot.QueryRow(`SELECT COUNT(*) FROM runs`).Scan(&count); err != nil {
		t.Fatalf("query snapshot: %v", err)
	}
	if count != 1 {
		t.Fatalf("snapshot rows = %d, want 1", count)
	}
}

func TestCreateAgentCoreAndExcludes(t *testing.T) {
	root := setupHome(t, "default")
	createAgentFixture(t, root, "default")
	db := createTestDatabase(t, filepath.Join(root, "agents", "default", "database", "myaaw.db"))
	defer db.Close()

	outDir := t.TempDir()
	results, err := Create(Options{Agents: []string{"default"}, OutputDir: outDir, Version: "test"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}

	files, headers := readArchive(t, results[0].Path)

	for _, want := range []string{
		"manifest.json",
		".myaaw-backup/registry-entry.json",
		".myaaw-backup/venv-packages.txt",
		".myaaw-backup/python-version.txt",
		"default/.env",
		"default/config/config.json",
		"default/contacts.json",
		"default/home/AGENTS.md",
		"default/home/memory/RULES.md",
		"default/skills/foo/SKILL.md",
		"default/tools/tools.json",
		"default/cron/jobs.json",
		"default/jobs/a2a/t1/output/result.txt",
		"default/database/myaaw.db",
		"default/database/notes/n.json",
	} {
		if _, ok := files[want]; !ok {
			t.Errorf("archive is missing %s", want)
		}
	}

	for _, unwanted := range []string{
		"default/home/workspace/project.txt",
		"default/home/.logs/run.log",
		"default/home/.venv/lib/big.bin",
		"default/logs/server.log",
		"default/cron/runs/run.jsonl",
		"default/jobs/job_1.log",
		"default/.DS_Store",
	} {
		if _, ok := files[unwanted]; ok {
			t.Errorf("archive should not contain %s", unwanted)
		}
	}

	if headers["default/.env"].Mode != 0o600 {
		t.Errorf(".env mode = %o, want 600", headers["default/.env"].Mode)
	}
	if headers["default/home/workspace/"] != nil {
		// workspace is skipped entirely.
	}
	if headers["default/cron/"] != nil && headers["default/cron/"].Typeflag != tar.TypeDir {
		t.Errorf("cron should be a directory entry")
	}

	var manifest Manifest
	if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if manifest.Agent != "default" || manifest.Format != FormatVersion {
		t.Fatalf("manifest = %+v", manifest)
	}
	if len(manifest.Files) == 0 {
		t.Fatal("manifest has no files")
	}
	for _, entry := range manifest.Files {
		if entry.SHA256 == "" {
			t.Fatalf("entry %s has no sha256", entry.Path)
		}
	}

	// The database entry must be a readable SQLite snapshot with the row.
	dbPath := filepath.Join(t.TempDir(), "snapshot.db")
	if err := os.WriteFile(dbPath, files["default/database/myaaw.db"], 0o600); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	snapshot, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer snapshot.Close()
	var value string
	if err := snapshot.QueryRow(`SELECT value FROM memories`).Scan(&value); err != nil {
		t.Fatalf("query snapshot: %v", err)
	}
	if value != "kept" {
		t.Fatalf("snapshot value = %q", value)
	}
}

func TestCreateIncludesWorkspaceAndLogs(t *testing.T) {
	root := setupHome(t, "default")
	createAgentFixture(t, root, "default")

	outDir := t.TempDir()
	results, err := Create(Options{
		Agents:           []string{"default"},
		OutputDir:        outDir,
		IncludeWorkspace: true,
		IncludeLogs:      true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	files, _ := readArchive(t, results[0].Path)
	for _, want := range []string{
		"default/home/workspace/project.txt",
		"default/home/.logs/run.log",
		"default/logs/server.log",
		"default/cron/runs/run.jsonl",
		"default/jobs/job_1.log",
	} {
		if _, ok := files[want]; !ok {
			t.Errorf("archive is missing %s", want)
		}
	}
}

func TestCreateNoSecrets(t *testing.T) {
	root := setupHome(t, "default")
	createAgentFixture(t, root, "default")

	results, err := Create(Options{Agents: []string{"default"}, OutputDir: t.TempDir(), NoSecrets: true})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	files, _ := readArchive(t, results[0].Path)
	if _, ok := files["default/.env"]; ok {
		t.Fatal(".env should be excluded with --no-secrets")
	}
	if strings.Contains(string(files["default/config/config.json"]), "SECRET-KEY") {
		t.Fatal("api_key was not blanked")
	}
	if strings.Contains(string(files["default/contacts.json"]), "SECRET-TOKEN") {
		t.Fatal("contacts token was not blanked")
	}
}

func createControlPlaneDBFixture(t *testing.T, root string) string {
	t.Helper()
	dir := filepath.Join(root, "controlplane")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir controlplane: %v", err)
	}
	path := filepath.Join(dir, "controlplane.db")
	db, err := sql.Open("sqlite3", "file:"+path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		t.Fatalf("open controlplane.db: %v", err)
	}
	defer db.Close()
	for _, stmt := range []string{
		`CREATE TABLE auth_account (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			username TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'admin',
			user_id INTEGER NOT NULL DEFAULT 0,
			password_changed INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			data_json TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE groups (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			discussion_mode TEXT NOT NULL DEFAULT 'autonomous',
			max_turns INTEGER NOT NULL DEFAULT 8,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE group_members (
			group_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			position INTEGER NOT NULL,
			last_seen_message_id TEXT NOT NULL DEFAULT '',
			conversation_id TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (group_id, name)
		)`,
		`CREATE TABLE group_messages (
			seq INTEGER PRIMARY KEY AUTOINCREMENT,
			id TEXT NOT NULL UNIQUE,
			group_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
			sender_type TEXT NOT NULL,
			sender_name TEXT NOT NULL,
			agent_color TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL DEFAULT '',
			timeline_json TEXT NOT NULL DEFAULT '',
			reply_to_id TEXT NOT NULL DEFAULT '',
			reply_preview_json TEXT NOT NULL DEFAULT '',
			quotes_json TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`INSERT INTO auth_account (id, username, password_hash, role, user_id, password_changed, created_at, updated_at)
		 VALUES (1, 'admin', '$2a$10$SECRET_HASH_VALUE', 'admin', 0, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO settings (id, data_json, updated_at)
		 VALUES (1, '{"theme_mode":"dark","shell_mode":"bots"}', '2026-01-01T00:00:00Z')`,
		`INSERT INTO groups (id, name, created_at, updated_at)
		 VALUES ('grp-1', 'Council', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO group_members (group_id, name, position) VALUES ('grp-1', 'default', 0)`,
		`INSERT INTO group_messages (id, group_id, sender_type, sender_name, content, created_at)
		 VALUES ('msg-1', 'grp-1', 'user', 'User', 'hello council', '2026-01-01T00:00:00Z')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	return path
}

func TestCreateAllIncludesGlobalArchive(t *testing.T) {
	root := setupHome(t, "default", "agent-b")
	createAgentFixture(t, root, "default")
	createAgentFixture(t, root, "agent-b")
	createControlPlaneDBFixture(t, root)

	results, err := Create(Options{OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3 (two agents + global)", len(results))
	}

	var globalPath string
	for _, result := range results {
		if result.Agent == "global" {
			globalPath = result.Path
		}
	}
	if globalPath == "" {
		t.Fatal("global archive was not created")
	}
	files, _ := readArchive(t, globalPath)
	if _, ok := files["agents.json"]; !ok {
		t.Fatal("global archive is missing agents.json")
	}
	cpBytes, ok := files["controlplane/controlplane.db"]
	if !ok {
		t.Fatal("global archive is missing controlplane/controlplane.db")
	}
	var manifest Manifest
	if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if manifest.DatabaseSnapshot != "VACUUM INTO" {
		t.Fatalf("database_snapshot = %q, want VACUUM INTO", manifest.DatabaseSnapshot)
	}

	snapPath := filepath.Join(t.TempDir(), "cp.db")
	if err := os.WriteFile(snapPath, cpBytes, 0o600); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	db, err := sql.Open("sqlite3", snapPath)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer db.Close()

	var authCount, groupCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM auth_account`).Scan(&authCount); err != nil || authCount != 1 {
		t.Fatalf("auth_account count = %d, err = %v, want 1", authCount, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM groups`).Scan(&groupCount); err != nil || groupCount != 1 {
		t.Fatalf("groups count = %d, err = %v, want 1", groupCount, err)
	}
}

func TestCreateAllNoSecretsSanitizesControlPlaneDB(t *testing.T) {
	root := setupHome(t, "default")
	createAgentFixture(t, root, "default")
	createControlPlaneDBFixture(t, root)

	results, err := Create(Options{OutputDir: t.TempDir(), NoSecrets: true})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, result := range results {
		if result.Agent != "global" {
			continue
		}
		files, _ := readArchive(t, result.Path)
		cpBytes, ok := files["controlplane/controlplane.db"]
		if !ok {
			t.Fatal("controlplane/controlplane.db should be included in sanitized form with --no-secrets")
		}
		if strings.Contains(string(cpBytes), "SECRET_HASH_VALUE") {
			t.Fatal("sanitized controlplane.db still contains password_hash bytes")
		}

		snapPath := filepath.Join(t.TempDir(), "cp-sanitized.db")
		if err := os.WriteFile(snapPath, cpBytes, 0o600); err != nil {
			t.Fatalf("write snapshot: %v", err)
		}
		db, err := sql.Open("sqlite3", snapPath)
		if err != nil {
			t.Fatalf("open snapshot: %v", err)
		}
		defer db.Close()

		var authCount, settingsCount, msgCount int
		if err := db.QueryRow(`SELECT COUNT(*) FROM auth_account`).Scan(&authCount); err != nil || authCount != 0 {
			t.Fatalf("auth_account count = %d, err = %v, want 0", authCount, err)
		}
		if err := db.QueryRow(`SELECT COUNT(*) FROM settings`).Scan(&settingsCount); err != nil || settingsCount != 1 {
			t.Fatalf("settings count = %d, err = %v, want 1", settingsCount, err)
		}
		if err := db.QueryRow(`SELECT COUNT(*) FROM group_messages`).Scan(&msgCount); err != nil || msgCount != 1 {
			t.Fatalf("group_messages count = %d, err = %v, want 1", msgCount, err)
		}
		return
	}
	t.Fatal("global archive was not created")
}

func TestCreateSkipsOutputDirInsideAgent(t *testing.T) {
	root := setupHome(t, "default")
	createAgentFixture(t, root, "default")
	outDir := filepath.Join(root, "agents", "default", "backups")

	results, err := Create(Options{Agents: []string{"default"}, OutputDir: outDir})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	files, _ := readArchive(t, results[0].Path)
	for name := range files {
		if strings.HasPrefix(name, "default/backups/") {
			t.Fatalf("archive included its own output directory: %s", name)
		}
	}
}
