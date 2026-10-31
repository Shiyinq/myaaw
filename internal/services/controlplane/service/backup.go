package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"myaaw/internal/agentdir"
	"myaaw/internal/backup"
)

var (
	// ErrBackupNotFound covers both a missing archive and a name that is not an
	// archive in the backup directory.
	ErrBackupNotFound = errors.New("backup archive not found")
	// ErrBackupInvalid is returned for an unusable request, such as an unknown
	// agent name or an archive that cannot be restored.
	ErrBackupInvalid = errors.New("invalid backup request")
)

// archiveNamePattern is the only shape accepted from the API. Names come from
// backup.Create, so anything else would be a path traversal attempt.
var archiveNamePattern = regexp.MustCompile(`^myaaw-backup-[A-Za-z0-9._-]+\.tar\.gz$`)

// BackupArchive is one archive on disk, as the API reports it.
type BackupArchive struct {
	Name      string `json:"name"`
	Agent     string `json:"agent"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"created_at"`
}

// BackupList is the backup directory plus its archives, newest first.
type BackupList struct {
	Dir      string          `json:"dir"`
	Archives []BackupArchive `json:"archives"`
}

// BackupRequest controls a new set of archives.
type BackupRequest struct {
	// Agents is empty to back up every initialized agent.
	Agents           []string `json:"agents,omitempty"`
	IncludeWorkspace bool     `json:"include_workspace"`
	IncludeLogs      bool     `json:"include_logs"`
	IncludeVenv      bool     `json:"include_venv"`
	IncludeCaches    bool     `json:"include_caches"`
	NoSecrets        bool     `json:"no_secrets"`
}

// BackupCreated is the outcome of a create request.
type BackupCreated struct {
	Dir      string          `json:"dir"`
	Archives []backup.Result `json:"archives"`
}

// RestoreResult mirrors backup.RestoreResult for the API schema.
type RestoreResult = backup.RestoreResult

// RestoreRequest controls how an archive is applied.
type RestoreRequest struct {
	// Agent renames the agent on restore. Empty keeps the archived name.
	Agent           string `json:"agent,omitempty"`
	Force           bool   `json:"force"`
	Start           bool   `json:"start"`
	RestorePackages bool   `json:"restore_packages"`
}

// BackupService exposes the archive and restore machinery to the dashboard.
type BackupService struct {
	servers *ServerService
}

func NewBackupService(servers *ServerService) *BackupService {
	return &BackupService{servers: servers}
}

// Dir is where archives are written and read from.
func (s *BackupService) Dir() (string, error) {
	return backup.ResolveOutputDir("")
}

// List returns every archive in the backup directory, newest first. A missing
// directory is not an error: it only means nothing has been backed up yet.
func (s *BackupService) List() (BackupList, error) {
	dir, err := s.Dir()
	if err != nil {
		return BackupList{}, err
	}

	archives, err := backup.ListArchives(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return BackupList{}, fmt.Errorf("list backups: %w", err)
	}

	list := BackupList{Dir: dir, Archives: make([]BackupArchive, 0, len(archives))}
	for _, archive := range archives {
		list.Archives = append(list.Archives, BackupArchive{
			Name:      archive.Name,
			Agent:     archive.Agent,
			Size:      archive.Size,
			CreatedAt: archive.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	return list, nil
}

// Create writes one archive per agent, plus the global files when every agent
// is selected.
func (s *BackupService) Create(req BackupRequest) (BackupCreated, error) {
	for _, name := range req.Agents {
		if err := agentdir.ValidateName(name); err != nil {
			return BackupCreated{}, fmt.Errorf("%w: %v", ErrBackupInvalid, err)
		}
	}

	dir, err := s.Dir()
	if err != nil {
		return BackupCreated{}, err
	}

	results, err := backup.Create(backup.Options{
		Agents:           req.Agents,
		OutputDir:        dir,
		IncludeWorkspace: req.IncludeWorkspace,
		IncludeLogs:      req.IncludeLogs,
		IncludeVenv:      req.IncludeVenv,
		IncludeCaches:    req.IncludeCaches,
		NoSecrets:        req.NoSecrets,
	})
	if err != nil {
		return BackupCreated{}, err
	}
	return BackupCreated{Dir: dir, Archives: results}, nil
}

// Archive resolves an archive name inside the backup directory.
func (s *BackupService) Archive(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !archiveNamePattern.MatchString(name) || name != filepath.Base(name) {
		return "", ErrBackupNotFound
	}

	dir, err := s.Dir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", ErrBackupNotFound
	}
	return path, nil
}

// Delete removes an archive. A missing file is not an error, so the dashboard
// can retry a delete safely.
func (s *BackupService) Delete(name string) error {
	path, err := s.Archive(name)
	if errors.Is(err, ErrBackupNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete backup: %w", err)
	}
	return nil
}

// Restore applies an archive. A running agent is stopped first, because the
// restore rewrites its directory.
func (s *BackupService) Restore(name string, req RestoreRequest) (backup.RestoreResult, error) {
	path, err := s.Archive(name)
	if err != nil {
		return backup.RestoreResult{}, err
	}
	if req.Agent != "" {
		if err := agentdir.ValidateName(req.Agent); err != nil {
			return backup.RestoreResult{}, fmt.Errorf("%w: %v", ErrBackupInvalid, err)
		}
	}

	return backup.Restore(backup.RestoreOptions{
		Archive:         path,
		Agent:           req.Agent,
		Force:           req.Force,
		RestorePackages: req.RestorePackages,
		Start:           req.Start,
		StopAgent:       s.stopAgent,
		StartAgent:      s.startAgent,
	})
}

func (s *BackupService) stopAgent(name string) (bool, error) {
	if s.servers == nil {
		return false, nil
	}
	status, err := s.servers.Status(name)
	if err != nil || !status.Running {
		return false, nil
	}
	if err := s.servers.Stop(name); err != nil {
		return false, err
	}
	return true, nil
}

func (s *BackupService) startAgent(name string) error {
	if s.servers == nil {
		return nil
	}
	return s.servers.Start(name)
}
