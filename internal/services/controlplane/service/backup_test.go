package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestBackupService(t *testing.T) *BackupService {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("MYAAW_BACKUP_DIR", dir)
	return NewBackupService(NewServerService(NewAgentService()))
}

func TestBackupServiceCreateListDelete(t *testing.T) {
	setupConfigAgent(t, nil)
	backups := newTestBackupService(t)

	created, err := backups.Create(BackupRequest{Agents: []string{"myfriend"}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(created.Archives) != 1 {
		t.Fatalf("archives = %+v, want 1", created.Archives)
	}
	archive := created.Archives[0]
	if archive.Agent != "myfriend" {
		t.Fatalf("agent = %q", archive.Agent)
	}
	if !strings.HasPrefix(filepath.Base(archive.Path), "myaaw-backup-myfriend-") {
		t.Fatalf("path = %q", archive.Path)
	}
	if _, err := os.Stat(archive.Path); err != nil {
		t.Fatalf("archive missing: %v", err)
	}

	list, err := backups.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list.Archives) != 1 || list.Archives[0].Agent != "myfriend" {
		t.Fatalf("list = %+v", list)
	}
	if list.Archives[0].Size <= 0 {
		t.Fatalf("size = %d", list.Archives[0].Size)
	}

	name := filepath.Base(archive.Path)
	path, err := backups.Archive(name)
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if path != archive.Path {
		t.Fatalf("path = %q, want %q", path, archive.Path)
	}

	if err := backups.Delete(name); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := backups.Archive(name); !errors.Is(err, ErrBackupNotFound) {
		t.Fatalf("Archive after delete = %v", err)
	}
	// Deleting twice stays harmless.
	if err := backups.Delete(name); err != nil {
		t.Fatalf("Delete twice: %v", err)
	}
}

func TestBackupServiceEmptyDirectory(t *testing.T) {
	setupConfigAgent(t, nil)
	t.Setenv("MYAAW_BACKUP_DIR", filepath.Join(t.TempDir(), "not-created-yet"))
	backups := NewBackupService(nil)

	list, err := backups.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list.Archives) != 0 {
		t.Fatalf("archives = %+v, want none", list.Archives)
	}
}

func TestBackupServiceRejectsBadNames(t *testing.T) {
	setupConfigAgent(t, nil)
	backups := newTestBackupService(t)

	for _, name := range []string{
		"../../etc/passwd",
		"/etc/passwd",
		"myaaw-backup-x.tar.gz/../../x",
		"other-archive.tar.gz",
		"",
	} {
		if _, err := backups.Archive(name); !errors.Is(err, ErrBackupNotFound) {
			t.Fatalf("Archive(%q) = %v, want ErrBackupNotFound", name, err)
		}
	}
}

func TestBackupServiceRejectsUnknownAgent(t *testing.T) {
	setupConfigAgent(t, nil)
	backups := newTestBackupService(t)

	if _, err := backups.Create(BackupRequest{Agents: []string{"../escape"}}); !errors.Is(err, ErrBackupInvalid) {
		t.Fatalf("Create with a bad agent = %v", err)
	}
}

func TestBackupServiceRestore(t *testing.T) {
	agentRoot := setupConfigAgent(t, nil)
	backups := newTestBackupService(t)

	created, err := backups.Create(BackupRequest{Agents: []string{"myfriend"}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	name := filepath.Base(created.Archives[0].Path)

	marker := filepath.Join(agentRoot, "home", "AGENTS.md")
	if err := os.WriteFile(marker, []byte("changed after backup"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	result, err := backups.Restore(name, RestoreRequest{Force: true})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if result.Agent != "myfriend" {
		t.Fatalf("result = %+v", result)
	}

	restored, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if string(restored) != "test" {
		t.Fatalf("AGENTS.md = %q, want the archived copy", restored)
	}

	// A missing archive is reported as not found, not as an internal error.
	if _, err := backups.Restore("myaaw-backup-gone-20260101-000000.tar.gz", RestoreRequest{}); !errors.Is(err, ErrBackupNotFound) {
		t.Fatalf("Restore missing = %v", err)
	}
}
