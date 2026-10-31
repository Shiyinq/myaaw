package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"myaaw/internal/agentdir"
)

// RestoreOptions controls Restore.
type RestoreOptions struct {
	Archive string
	// Agent overrides the agent name stored in the manifest.
	Agent string
	// Force replaces an existing agent or global files.
	Force bool
	// RestorePackages rebuilds home/.venv from .myaaw-backup/venv-packages.txt.
	RestorePackages bool
	// Start starts the agent daemon after a successful restore.
	Start bool
	// Version is recorded when a pre-restore archive is created.
	Version string

	// StopAgent and StartAgent are supplied by the CLI. StopAgent returns true
	// when a running daemon was actually stopped.
	StopAgent  func(name string) (bool, error)
	StartAgent func(name string) error
}

// RestoreResult describes a completed restore.
type RestoreResult struct {
	Agent            string `json:"agent"`
	Files            int    `json:"files"`
	PreRestoreBackup string `json:"pre_restore_backup,omitempty"`
	PortWarning      string `json:"port_warning,omitempty"`
	Note             string `json:"note,omitempty"`
}

type registryEntryFile struct {
	Name      string `json:"name"`
	Port      string `json:"port,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// Restore extracts a backup archive created by Create.
func Restore(opts RestoreOptions) (result RestoreResult, err error) {
	if strings.TrimSpace(opts.Archive) == "" {
		return RestoreResult{}, errors.New("archive path is required")
	}
	root, err := agentdir.MyaawRoot()
	if err != nil {
		return RestoreResult{}, err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return RestoreResult{}, err
	}

	staging, err := os.MkdirTemp(root, ".restore-")
	if err != nil {
		return RestoreResult{}, fmt.Errorf("create staging directory: %w", err)
	}
	defer os.RemoveAll(staging)

	if err := extractArchive(opts.Archive, staging); err != nil {
		return RestoreResult{}, err
	}
	manifest, err := loadManifest(staging)
	if err != nil {
		return RestoreResult{}, err
	}
	if err := verifyManifest(staging, manifest); err != nil {
		return RestoreResult{}, err
	}

	if manifest.Agent == "global" {
		return restoreGlobal(root, staging, manifest, opts)
	}

	name := manifest.Agent
	if strings.TrimSpace(opts.Agent) != "" {
		name = strings.TrimSpace(opts.Agent)
	}
	if err := agentdir.ValidateName(name); err != nil {
		return RestoreResult{}, err
	}
	if name == "global" {
		return RestoreResult{}, errors.New(`"global" is reserved for global archives`)
	}

	sourceDir := filepath.Join(staging, manifest.Agent)
	if info, err := os.Stat(sourceDir); err != nil || !info.IsDir() {
		return RestoreResult{}, fmt.Errorf("archive has no %q directory", manifest.Agent)
	}
	targetDir, err := agentdir.AgentRoot(name)
	if err != nil {
		return RestoreResult{}, err
	}

	exists := false
	if info, err := os.Lstat(targetDir); err == nil {
		if !info.IsDir() {
			return RestoreResult{}, fmt.Errorf("%s exists and is not a directory", targetDir)
		}
		if !opts.Force {
			if agentdir.AgentInitialized(name) || !inertAgentDir(targetDir) {
				return RestoreResult{}, fmt.Errorf("agent %q already exists; use --force to replace it", name)
			}
		}
		exists = true
	} else if !os.IsNotExist(err) {
		return RestoreResult{}, err
	}

	stopped := false
	if exists && opts.StopAgent != nil {
		stopped, err = opts.StopAgent(name)
		if err != nil {
			return RestoreResult{}, fmt.Errorf("stop agent %q: %w", name, err)
		}
	}
	defer func() {
		if err != nil && stopped && opts.StartAgent != nil {
			_ = opts.StartAgent(name)
		}
	}()

	if exists {
		archives, backupErr := Create(Options{Agents: []string{name}, Version: opts.Version})
		if backupErr != nil {
			err = fmt.Errorf("pre-restore backup: %w", backupErr)
			return RestoreResult{}, err
		}
		if len(archives) > 0 {
			result.PreRestoreBackup = archives[0].Path
		}
		if removeErr := os.RemoveAll(targetDir); removeErr != nil {
			err = removeErr
			return RestoreResult{}, err
		}
	}

	if name != manifest.Agent {
		renamed := filepath.Join(staging, name)
		if renameErr := os.Rename(sourceDir, renamed); renameErr != nil {
			err = renameErr
			return RestoreResult{}, err
		}
		sourceDir = renamed
	}
	if renameErr := os.Rename(sourceDir, targetDir); renameErr != nil {
		err = renameErr
		return RestoreResult{}, err
	}

	secureRestoredFiles(targetDir)
	note := ""
	if _, statErr := os.Stat(filepath.Join(targetDir, ".env")); os.IsNotExist(statErr) {
		note = "archive has no .env (likely created with --no-secrets); configure credentials before starting the agent"
	}
	result.Agent = name
	result.Files = len(manifest.Files)
	result.Note = note
	result.PortWarning = portConflict(root, name, targetDir)

	entry := readRegistryEntry(staging)
	if registerErr := registerAgent(name, entry); registerErr != nil {
		err = fmt.Errorf("register agent: %w", registerErr)
		return result, err
	}

	if opts.RestorePackages {
		if pkgErr := installPackages(staging, targetDir); pkgErr != nil {
			err = fmt.Errorf("restore completed, but Python packages failed: %w", pkgErr)
			return result, err
		}
	}

	if opts.Start && opts.StartAgent != nil {
		if startErr := opts.StartAgent(name); startErr != nil {
			err = fmt.Errorf("restore completed, but starting the agent failed: %w", startErr)
			return result, err
		}
		stopped = false
	}
	return result, nil
}

func restoreGlobal(root, staging string, manifest Manifest, opts RestoreOptions) (RestoreResult, error) {
	if strings.TrimSpace(opts.Agent) != "" {
		return RestoreResult{}, errors.New("--agent cannot be used with a global archive")
	}
	allowed := map[string]bool{"agents.json": true, "controlplane.json": true}
	for _, file := range manifest.Files {
		if !allowed[file.Path] {
			return RestoreResult{}, fmt.Errorf("unexpected file %q in global archive", file.Path)
		}
	}
	if !opts.Force {
		for _, file := range manifest.Files {
			if _, err := os.Stat(filepath.Join(root, file.Path)); err == nil {
				return RestoreResult{}, fmt.Errorf("%s already exists; use --force to replace it", file.Path)
			}
		}
	}

	stamp := time.Now().Format("20060102-150405")
	for _, file := range manifest.Files {
		src := filepath.Join(staging, filepath.FromSlash(file.Path))
		dst := filepath.Join(root, file.Path)
		if _, err := os.Stat(dst); err == nil {
			if err := copyFile(dst, dst+".pre-restore-"+stamp); err != nil {
				return RestoreResult{}, fmt.Errorf("save previous %s: %w", file.Path, err)
			}
		}
		if err := moveFile(src, dst); err != nil {
			return RestoreResult{}, err
		}
	}
	return RestoreResult{
		Agent: "global",
		Files: len(manifest.Files),
		Note:  "restart the control plane API to load the restored configuration",
	}, nil
}

func loadManifest(staging string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(staging, "manifest.json"))
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest.json: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest.json: %w", err)
	}
	if manifest.Format != FormatVersion {
		return Manifest{}, fmt.Errorf("unsupported backup format %d (this build supports %d)", manifest.Format, FormatVersion)
	}
	if strings.TrimSpace(manifest.Agent) == "" {
		return Manifest{}, errors.New("manifest is missing the agent name")
	}
	return manifest, nil
}

func verifyManifest(staging string, manifest Manifest) error {
	allowed := map[string]bool{"manifest.json": true}
	for _, file := range manifest.Files {
		name, err := safeArchivePath(file.Path)
		if err != nil {
			return fmt.Errorf("manifest contains an unsafe path: %w", err)
		}
		allowed[name] = true
		info, err := os.Stat(filepath.Join(staging, filepath.FromSlash(name)))
		if err != nil {
			return fmt.Errorf("archive is missing %s", name)
		}
		if info.Size() != file.Size {
			return fmt.Errorf("%s has size %d, manifest says %d", name, info.Size(), file.Size)
		}
		sum, err := fileSHA256(filepath.Join(staging, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		if !strings.EqualFold(sum, file.SHA256) {
			return fmt.Errorf("checksum mismatch for %s", name)
		}
	}

	return filepath.WalkDir(staging, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(staging, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !allowed[rel] {
			return fmt.Errorf("archive contains unlisted file %q", rel)
		}
		return nil
	})
}

func extractArchive(archive, staging string) error {
	file, err := os.Open(archive)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("read archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read archive: %w", err)
		}
		name, err := safeArchivePath(header.Name)
		if err != nil {
			return err
		}
		target := filepath.Join(staging, filepath.FromSlash(name))
		mode := os.FileMode(header.Mode).Perm()

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, mode|0o700); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
			if err != nil {
				return fmt.Errorf("extract %s: %w", name, err)
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return fmt.Errorf("extract %s: %w", name, err)
			}
			if err := out.Close(); err != nil {
				return err
			}
			if err := os.Chmod(target, mode); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported archive entry %q (type %d)", name, header.Typeflag)
		}
	}
}

func safeArchivePath(name string) (string, error) {
	name = strings.TrimPrefix(name, "./")
	cleaned := path.Clean(name)
	if cleaned == "." || cleaned == "" {
		return "", errors.New("archive contains an empty path")
	}
	if path.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	if strings.Contains(cleaned, "\\") {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	return cleaned, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// inertAgentDir reports whether a directory contains only runtime leftovers
// (empty directories and log files), which a previous CLI invocation or a
// still-shutting-down daemon may have created. Such a directory is safe to
// replace without --force.
func inertAgentDir(root string) bool {
	inert := true
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			inert = false
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			inert = false
			return filepath.SkipAll
		}
		rel = filepath.ToSlash(rel)
		if rel == ".DS_Store" || strings.HasPrefix(rel, "logs/") || strings.HasPrefix(rel, ".logs/") {
			return nil
		}
		inert = false
		return filepath.SkipAll
	})
	return err == nil && inert
}

func secureRestoredFiles(agentRoot string) {
	for _, rel := range []string{".env", "contacts.json", "config/config.json"} {
		p := filepath.Join(agentRoot, filepath.FromSlash(rel))
		if _, err := os.Stat(p); err == nil {
			_ = os.Chmod(p, 0o600)
		}
	}
}

func readRegistryEntry(staging string) registryEntryFile {
	var entry registryEntryFile
	data, err := os.ReadFile(filepath.Join(staging, metaDir, "registry-entry.json"))
	if err != nil {
		return entry
	}
	_ = json.Unmarshal(data, &entry)
	return entry
}

func registerAgent(name string, entry registryEntryFile) error {
	registry, err := agentdir.LoadRegistry()
	if err != nil || registry == nil {
		registry = &agentdir.Registry{Agents: map[string]agentdir.AgentInfo{}}
	}
	if registry.Agents == nil {
		registry.Agents = map[string]agentdir.AgentInfo{}
	}
	if _, exists := registry.Agents[name]; !exists {
		created := strings.TrimSpace(entry.CreatedAt)
		if created == "" {
			created = agentdir.CreatedAtUTC()
		}
		registry.Agents[name] = agentdir.AgentInfo{CreatedAt: created}
	}
	if registry.Default == "" {
		registry.Default = name
	}
	return agentdir.SaveRegistry(registry)
}

func portConflict(root, name, agentRoot string) string {
	port := envPort(filepath.Join(agentRoot, ".env"))
	if port == "" {
		return ""
	}
	names, err := agentdir.ListAgents()
	if err != nil {
		return ""
	}
	for _, other := range names {
		if other == name {
			continue
		}
		otherPort := envPort(filepath.Join(root, other, ".env"))
		if otherPort != "" && otherPort == port {
			return fmt.Sprintf("PORT %s is also configured for agent %q", port, other)
		}
	}
	return ""
}

func installPackages(staging, agentRoot string) error {
	data, err := os.ReadFile(filepath.Join(staging, metaDir, "venv-packages.txt"))
	if err != nil || strings.TrimSpace(string(data)) == "" {
		return nil
	}
	packages := filepath.Join(staging, metaDir, "venv-packages.txt")
	venv := filepath.Join(agentRoot, "home", ".venv")
	pip := filepath.Join(venv, "bin", "pip")
	if _, err := os.Stat(pip); err != nil {
		python, lookErr := exec.LookPath("python3")
		if lookErr != nil {
			return fmt.Errorf("python3 not found: %w", lookErr)
		}
		if err := runCommand(python, "-m", "venv", venv); err != nil {
			return fmt.Errorf("create venv: %w", err)
		}
	}
	if err := runCommand(pip, "install", "-r", packages); err != nil {
		return fmt.Errorf("pip install: %w", err)
	}
	return nil
}

func runCommand(bin string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w (%s)", filepath.Base(bin), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, info.Mode().Perm())
}

func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// Cross-device fallback is not expected (staging lives under ~/.myaaw),
	// but keep the restore working if a caller stages elsewhere.
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}
