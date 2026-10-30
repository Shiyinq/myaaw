// Package backup creates portable archives of an agent's data directory.
//
// An archive mirrors the on-disk layout (<agent>/...) so it can be restored by
// extracting it under ~/.myaaw. Metadata lives in manifest.json and a reserved
// .myaaw-backup/ directory.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"myaaw/internal/agentdir"

	_ "github.com/mattn/go-sqlite3"
)

const (
	// FormatVersion is bumped when the archive layout changes.
	FormatVersion = 1

	metaDir = ".myaaw-backup"
	// DefaultDirName is created next to ~/.myaaw, never inside it: folders
	// under ~/.myaaw are reserved for agents.
	DefaultDirName = ".myaaw-backups"
)

// snapshotRelPaths are the SQLite databases copied with VACUUM INTO instead of
// being read file-by-file. A WAL database cannot be copied consistently by
// reading its files, so every database the agent keeps lives here.
var snapshotRelPaths = []string{
	"database/myaaw.db",
	"database/observability.db",
}

// Options controls what Create includes in the archives.
type Options struct {
	// Agents is the list to back up. Empty means every initialized agent.
	Agents []string

	// OutputDir receives the archives. Empty uses MYAAW_BACKUP_DIR or
	// ~/.myaaw-backups. "-" streams a single agent archive to Stdout.
	OutputDir string

	IncludeWorkspace bool
	IncludeLogs      bool
	IncludeVenv      bool
	IncludeCaches    bool
	NoSecrets        bool

	// Version is recorded in the manifest (normally the CLI build version).
	Version string

	// Now is overridable for tests.
	Now time.Time

	// Stdout receives the archive when OutputDir is "-".
	Stdout io.Writer
}

// Result describes one written archive.
type Result struct {
	Agent string `json:"agent"`
	Path  string `json:"path"`
	Files int    `json:"files"`
	Bytes int64  `json:"bytes"`
}

// FileEntry is one manifest entry.
type FileEntry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Mode   string `json:"mode"`
	SHA256 string `json:"sha256"`
}

// Manifest is stored as manifest.json in every archive.
type Manifest struct {
	Format           int         `json:"format"`
	MyaawVersion     string      `json:"myaaw_version,omitempty"`
	Agent            string      `json:"agent"`
	CreatedAt        string      `json:"created_at"`
	Hostname         string      `json:"hostname,omitempty"`
	Includes         []string    `json:"includes"`
	Excludes         []string    `json:"excludes"`
	DatabaseSnapshot string      `json:"database_snapshot,omitempty"`
	Skipped          []string    `json:"skipped,omitempty"`
	Files            []FileEntry `json:"files"`
}

type generatedFile struct {
	name string // full path inside the archive
	data []byte
	mode int64
}

type builder func(*tar.Writer) (Manifest, error)

// Create writes one archive per agent and, when all agents are selected, an
// extra archive with the global registry files.
func Create(opts Options) ([]Result, error) {
	root, err := agentdir.MyaawRoot()
	if err != nil {
		return nil, err
	}

	all := len(opts.Agents) == 0
	agents := append([]string(nil), opts.Agents...)
	if all {
		agents, err = agentdir.ListAgents()
		if err != nil {
			return nil, err
		}
	}
	if len(agents) == 0 {
		return nil, errors.New("no initialized agents to back up")
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	toStdout := strings.TrimSpace(opts.OutputDir) == "-"
	if toStdout {
		if all || len(agents) != 1 {
			return nil, errors.New("streaming to stdout requires exactly one --agent")
		}
		if opts.Stdout == nil {
			opts.Stdout = os.Stdout
		}
	}

	outDir := ""
	if !toStdout {
		outDir, err = ResolveOutputDir(opts.OutputDir)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(outDir, 0o700); err != nil {
			return nil, fmt.Errorf("create output dir: %w", err)
		}
	}

	results := make([]Result, 0, len(agents)+1)
	for _, name := range agents {
		result, err := createAgent(root, outDir, toStdout, name, opts, now)
		if err != nil {
			return results, fmt.Errorf("backup %s: %w", name, err)
		}
		results = append(results, result)
	}
	if all {
		result, err := createGlobal(root, outDir, opts, now)
		if err != nil {
			return results, fmt.Errorf("backup global files: %w", err)
		}
		results = append(results, result)
	}
	return results, nil
}

// ResolveOutputDir applies flag > env > default precedence.
func ResolveOutputDir(flag string) (string, error) {
	dir := strings.TrimSpace(flag)
	if dir == "" {
		dir = strings.TrimSpace(os.Getenv("MYAAW_BACKUP_DIR"))
	}
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get user home directory: %w", err)
		}
		return filepath.Join(home, DefaultDirName), nil
	}
	return expandHome(dir)
}

func expandHome(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, path[2:]), nil
	}
	return filepath.Abs(path)
}

func createAgent(root, outDir string, toStdout bool, name string, opts Options, now time.Time) (Result, error) {
	agentRoot := filepath.Join(root, name)
	info, err := os.Stat(agentRoot)
	if err != nil {
		return Result{}, errors.New("agent directory not found")
	}
	if !info.IsDir() {
		return Result{}, fmt.Errorf("%s is not a directory", agentRoot)
	}

	outAbs := ""
	if !toStdout {
		outAbs, err = filepath.Abs(outDir)
		if err != nil {
			return Result{}, err
		}
		agentAbs, err := filepath.Abs(agentRoot)
		if err != nil {
			return Result{}, err
		}
		if outAbs == agentAbs {
			return Result{}, errors.New("output directory cannot be the agent directory itself")
		}
	}

	tempDir, err := os.MkdirTemp("", "myaaw-backup-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(tempDir)

	snapshots, err := snapshotDatabases(agentRoot, tempDir)
	if err != nil {
		return Result{}, err
	}
	generated := venvRecipe(agentRoot, opts.IncludeVenv)
	generated = append(generated, registryEntry(agentRoot, name)...)

	manifest := Manifest{
		Format:           FormatVersion,
		MyaawVersion:     opts.Version,
		Agent:            name,
		CreatedAt:        now.UTC().Format(time.RFC3339),
		Hostname:         hostname(),
		Includes:         includes(opts),
		Excludes:         excludes(opts),
		DatabaseSnapshot: "VACUUM INTO",
	}
	if len(snapshots) == 0 {
		manifest.DatabaseSnapshot = ""
	}

	build := func(tw *tar.Writer) (Manifest, error) {
		walk := func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, err := filepath.Rel(agentRoot, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if rel == "." {
				return nil
			}

			if d.IsDir() {
				if outAbs != "" && samePath(path, outAbs) {
					manifest.Skipped = append(manifest.Skipped, rel+"/")
					return filepath.SkipDir
				}
				if skipDir(rel, d.Name(), opts) {
					manifest.Skipped = append(manifest.Skipped, rel+"/")
					return filepath.SkipDir
				}
				return addDir(tw, filepath.ToSlash(filepath.Join(name, rel)), infoFor(path))
			}

			if d.Type()&fs.ModeSymlink != 0 || !d.Type().IsRegular() {
				manifest.Skipped = append(manifest.Skipped, rel)
				return nil
			}
			if skipFile(rel, d.Name(), opts) {
				manifest.Skipped = append(manifest.Skipped, rel)
				return nil
			}
			if isSnapshotPath(rel) {
				return nil // replaced by the snapshot below
			}

			archivePath := filepath.ToSlash(filepath.Join(name, rel))
			if opts.NoSecrets {
				switch rel {
				case ".env":
					manifest.Skipped = append(manifest.Skipped, rel)
					return nil
				case "config/config.json", "contacts.json":
					data, err := sanitizeSecrets(path)
					if err != nil {
						manifest.Skipped = append(manifest.Skipped, rel)
						return nil
					}
					entry, err := addBytes(tw, archivePath, data, 0o600)
					if err != nil {
						return err
					}
					manifest.Files = append(manifest.Files, entry)
					return nil
				}
			}
			entry, err := addFile(tw, archivePath, path)
			if err != nil {
				return err
			}
			manifest.Files = append(manifest.Files, entry)
			return nil
		}
		if err := filepath.WalkDir(agentRoot, walk); err != nil {
			return Manifest{}, err
		}

		for _, snapshot := range snapshots {
			entry, err := addFile(tw, filepath.ToSlash(filepath.Join(name, snapshot.rel)), snapshot.path)
			if err != nil {
				return Manifest{}, err
			}
			manifest.Files = append(manifest.Files, entry)
		}
		for _, file := range generated {
			entry, err := addBytes(tw, file.name, file.data, file.mode)
			if err != nil {
				return Manifest{}, err
			}
			manifest.Files = append(manifest.Files, entry)
		}
		return manifest, nil
	}

	path, bytes, err := writeArchive(outDir, toStdout, opts.Stdout, fmt.Sprintf("myaaw-backup-%s-%s.tar.gz", name, now.Format("20060102-150405")), build)
	if err != nil {
		return Result{}, err
	}
	return Result{Agent: name, Path: path, Files: len(manifest.Files) + 1, Bytes: bytes}, nil
}

func createGlobal(root, outDir string, opts Options, now time.Time) (Result, error) {
	manifest := Manifest{
		Format:       FormatVersion,
		MyaawVersion: opts.Version,
		Agent:        "global",
		CreatedAt:    now.UTC().Format(time.RFC3339),
		Hostname:     hostname(),
		Includes:     []string{"agents.json"},
	}
	if !opts.NoSecrets {
		manifest.Includes = append(manifest.Includes, "controlplane.json")
	}

	build := func(tw *tar.Writer) (Manifest, error) {
		for _, name := range []string{"agents.json", "controlplane.json"} {
			if opts.NoSecrets && name == "controlplane.json" {
				continue
			}
			path := filepath.Join(root, name)
			if _, err := os.Stat(path); err != nil {
				continue
			}
			entry, err := addFile(tw, name, path)
			if err != nil {
				return Manifest{}, err
			}
			manifest.Files = append(manifest.Files, entry)
		}
		return manifest, nil
	}

	path, bytes, err := writeArchive(outDir, false, nil, fmt.Sprintf("myaaw-backup-global-%s.tar.gz", now.Format("20060102-150405")), build)
	if err != nil {
		return Result{}, err
	}
	return Result{Agent: "global", Path: path, Files: len(manifest.Files) + 1, Bytes: bytes}, nil
}

// writeArchive writes to a temp file and renames it into place, or streams to
// stdout. The manifest is appended last so file hashes are known.
func writeArchive(outDir string, toStdout bool, stdout io.Writer, name string, build builder) (string, int64, error) {
	if toStdout {
		if stdout == nil {
			return "", 0, errors.New("stdout writer is required")
		}
		counter := &countWriter{writer: stdout}
		if err := writeStream(counter, build); err != nil {
			return "", 0, err
		}
		return "-", counter.count, nil
	}

	path := uniquePath(filepath.Join(outDir, name))
	tmp, err := os.CreateTemp(outDir, ".myaaw-backup-*.tmp")
	if err != nil {
		return "", 0, err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}

	if err := writeStream(tmp, build); err != nil {
		cleanup()
		return "", 0, err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return "", 0, err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return "", 0, err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		_ = os.Remove(tmpName)
		return "", 0, err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return "", 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", 0, err
	}
	return path, info.Size(), nil
}

func writeStream(w io.Writer, build builder) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)

	manifest, err := build(tw)
	if err != nil {
		_ = tw.Close()
		_ = gz.Close()
		return err
	}
	sort.Slice(manifest.Files, func(i, j int) bool { return manifest.Files[i].Path < manifest.Files[j].Path })
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		_ = tw.Close()
		_ = gz.Close()
		return err
	}
	if _, err := addBytes(tw, "manifest.json", append(data, '\n'), 0o600); err != nil {
		_ = tw.Close()
		_ = gz.Close()
		return err
	}
	if err := tw.Close(); err != nil {
		_ = gz.Close()
		return err
	}
	return gz.Close()
}

type countWriter struct {
	writer io.Writer
	count  int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.writer.Write(p)
	c.count += int64(n)
	return n, err
}

// databaseSnapshot is one database copied into the archive from a temporary
// consistent copy.
type databaseSnapshot struct {
	rel  string
	path string
}

func snapshotDatabases(agentRoot, tempDir string) ([]databaseSnapshot, error) {
	snapshots := make([]databaseSnapshot, 0, len(snapshotRelPaths))
	for _, rel := range snapshotRelPaths {
		path, err := snapshotDatabase(agentRoot, tempDir, rel)
		if err != nil {
			return nil, err
		}
		if path != "" {
			snapshots = append(snapshots, databaseSnapshot{rel: rel, path: path})
		}
	}
	return snapshots, nil
}

func snapshotDatabase(agentRoot, tempDir, rel string) (string, error) {
	src := filepath.Join(agentRoot, filepath.FromSlash(rel))
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	dst := filepath.Join(tempDir, snapshotBase(rel))
	db, err := sql.Open("sqlite3", src)
	if err != nil {
		return "", fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	_, _ = db.Exec(`PRAGMA busy_timeout = 5000`)

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		_ = os.Remove(dst)
		if _, err := db.Exec("VACUUM INTO " + quoteSQLite(dst)); err == nil {
			return dst, nil
		} else {
			lastErr = err
		}
		time.Sleep(300 * time.Millisecond)
	}
	return "", fmt.Errorf("snapshot database: %w (stop the agent daemon and retry)", lastErr)
}

func quoteSQLite(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func registryEntry(agentRoot, name string) []generatedFile {
	registry, err := agentdir.LoadRegistry()
	if err != nil {
		return nil
	}
	entry := struct {
		Name      string `json:"name"`
		Port      string `json:"port,omitempty"`
		CreatedAt string `json:"created_at,omitempty"`
	}{Name: name, Port: envPort(filepath.Join(agentRoot, ".env"))}
	if registry != nil {
		if info, ok := registry.Agents[name]; ok {
			entry.CreatedAt = info.CreatedAt
		}
	}
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return nil
	}
	return []generatedFile{{
		name: metaDir + "/registry-entry.json",
		data: append(data, '\n'),
		mode: 0o600,
	}}
}

func envPort(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "PORT=") {
			return strings.TrimSpace(strings.TrimPrefix(line, "PORT="))
		}
	}
	return ""
}

func hostname() string {
	name, err := os.Hostname()
	if err != nil {
		return ""
	}
	return name
}

func venvRecipe(agentRoot string, includeVenv bool) []generatedFile {
	if includeVenv {
		return nil
	}
	venv := filepath.Join(agentRoot, "home", ".venv")
	pip := filepath.Join(venv, "bin", "pip")
	if _, err := os.Stat(pip); err != nil {
		return nil
	}
	files := make([]generatedFile, 0, 2)
	if freeze, err := runQuietly(pip, "freeze"); err == nil && strings.TrimSpace(freeze) != "" {
		files = append(files, generatedFile{
			name: metaDir + "/venv-packages.txt",
			data: []byte(freeze),
			mode: 0o600,
		})
	}
	python := filepath.Join(venv, "bin", "python")
	if version, err := runQuietly(python, "--version"); err == nil && strings.TrimSpace(version) != "" {
		files = append(files, generatedFile{
			name: metaDir + "/python-version.txt",
			data: []byte(strings.TrimSpace(version) + "\n"),
			mode: 0o600,
		})
	}
	return files
}

func runQuietly(bin string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, args...).Output()
	return string(out), err
}

func includes(opts Options) []string {
	list := []string{"core"}
	if opts.IncludeWorkspace {
		list = append(list, "workspace")
	}
	if opts.IncludeLogs {
		list = append(list, "logs")
	}
	if opts.IncludeVenv {
		list = append(list, "venv")
	}
	if opts.IncludeCaches {
		list = append(list, "caches")
	}
	return list
}

func excludes(opts Options) []string {
	list := []string{"workspace", "logs", "cron/runs", "jobs/*.log", ".venv", ".logs", "tmp", ".playwright-mcp"}
	if opts.NoSecrets {
		list = append(list, ".env", "api_key")
	}
	return list
}

func skipDir(rel, base string, opts Options) bool {
	switch base {
	case ".venv":
		return !opts.IncludeVenv
	case "logs", ".logs":
		return !opts.IncludeLogs
	case "workspace":
		return !opts.IncludeWorkspace
	case "tmp", ".playwright-mcp":
		return !opts.IncludeCaches
	}
	if filepath.ToSlash(rel) == "cron/runs" {
		return !opts.IncludeLogs
	}
	return false
}

// snapshotBase returns the file name of a snapshot path expressed with forward
// slashes, independent of the host separator.
func snapshotBase(rel string) string {
	if idx := strings.LastIndex(rel, "/"); idx >= 0 {
		return rel[idx+1:]
	}
	return rel
}

func isSnapshotPath(rel string) bool {
	for _, snapshot := range snapshotRelPaths {
		if rel == snapshot {
			return true
		}
	}
	return false
}

func skipFile(rel, base string, opts Options) bool {
	if base == ".DS_Store" {
		return true
	}
	switch {
	case strings.HasSuffix(base, ".tmp"),
		strings.HasSuffix(base, ".part"):
		return true
	}
	// A snapshotted database is replaced by a consistent copy, so its side files
	// must not be archived. Other SQLite files keep their -wal/-shm so no
	// committed data is lost.
	for _, snapshot := range snapshotRelPaths {
		name := snapshotBase(snapshot)
		if base == name+"-wal" || base == name+"-shm" {
			return true
		}
	}
	if !opts.IncludeLogs && strings.HasSuffix(base, ".log") && filepath.ToSlash(filepath.Dir(rel)) == "jobs" {
		return true
	}
	return false
}

func sanitizeSecrets(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	switch filepath.Base(path) {
	case "config.json":
		var doc map[string]any
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, err
		}
		if providers, ok := doc["providers"].(map[string]any); ok {
			for _, value := range providers {
				if provider, ok := value.(map[string]any); ok {
					if _, exists := provider["api_key"]; exists {
						provider["api_key"] = ""
					}
				}
			}
		}
		return json.MarshalIndent(doc, "", "  ")
	case "contacts.json":
		var entries []map[string]any
		if err := json.Unmarshal(data, &entries); err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if _, exists := entry["token"]; exists {
				entry["token"] = ""
			}
		}
		return json.MarshalIndent(entries, "", "  ")
	}
	return data, nil
}

func addDir(tw *tar.Writer, name string, info os.FileInfo) error {
	mode := int64(0o755)
	if info != nil {
		mode = int64(info.Mode().Perm())
	}
	header := &tar.Header{
		Name:     strings.TrimSuffix(name, "/") + "/",
		Mode:     mode,
		Typeflag: tar.TypeDir,
		ModTime:  time.Now().UTC(),
	}
	return tw.WriteHeader(header)
}

func addFile(tw *tar.Writer, name, path string) (FileEntry, error) {
	info, err := os.Stat(path)
	if err != nil {
		return FileEntry{}, err
	}
	if !info.Mode().IsRegular() {
		return FileEntry{}, fmt.Errorf("%s is not a regular file", path)
	}
	header := &tar.Header{
		Name:     name,
		Mode:     int64(info.Mode().Perm()),
		Size:     info.Size(),
		ModTime:  info.ModTime().UTC(),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(header); err != nil {
		return FileEntry{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return FileEntry{}, err
	}
	defer file.Close()
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(tw, hash), file)
	if err != nil {
		return FileEntry{}, err
	}
	return FileEntry{
		Path:   name,
		Size:   written,
		Mode:   fmt.Sprintf("%04o", info.Mode().Perm()),
		SHA256: hex.EncodeToString(hash.Sum(nil)),
	}, nil
}

func addBytes(tw *tar.Writer, name string, data []byte, mode int64) (FileEntry, error) {
	header := &tar.Header{
		Name:     name,
		Mode:     mode,
		Size:     int64(len(data)),
		ModTime:  time.Now().UTC(),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(header); err != nil {
		return FileEntry{}, err
	}
	if _, err := tw.Write(data); err != nil {
		return FileEntry{}, err
	}
	sum := sha256.Sum256(data)
	return FileEntry{
		Path:   name,
		Size:   int64(len(data)),
		Mode:   fmt.Sprintf("%04o", mode),
		SHA256: hex.EncodeToString(sum[:]),
	}, nil
}

func infoFor(path string) os.FileInfo {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	return info
}

func samePath(a, b string) bool {
	aa, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	bb, err := filepath.Abs(b)
	if err != nil {
		return false
	}
	return filepath.Clean(aa) == filepath.Clean(bb)
}

func uniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	base := strings.TrimSuffix(path, ".tar.gz")
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d.tar.gz", base, i)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}
