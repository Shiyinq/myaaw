package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListArchives(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "myaaw-backup-agent-b-20261027-004339.tar.gz"), "old", 0o600)
	writeFile(t, filepath.Join(dir, "myaaw-backup-agent-c-20261027-010000.tar.gz"), "newer", 0o600)
	writeFile(t, filepath.Join(dir, "myaaw-backup-global-20261027-005000.tar.gz"), "global", 0o600)
	writeFile(t, filepath.Join(dir, "myaaw-backup-agent-b-copy-20261027-003000-2.tar.gz"), "copy", 0o600)
	writeFile(t, filepath.Join(dir, "myaaw-backup-agent_working-20261027-002000.tar.gz"), "underscore", 0o600)
	writeFile(t, filepath.Join(dir, "myaaw-backup-agent-personal-working-20261027-001500.tar.gz"), "multi-hyphen", 0o600)
	writeFile(t, filepath.Join(dir, "myaaw-backup-agent-personal-working-prod-01-20261027-001000.tar.gz"), "five-words", 0o600)
	writeFile(t, filepath.Join(dir, "notes.txt"), "ignore me", 0o600)
	if err := os.Mkdir(filepath.Join(dir, "myaaw-backup-dir-20260101-000000.tar.gz"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	archives, err := ListArchives(dir)
	if err != nil {
		t.Fatalf("ListArchives: %v", err)
	}
	if len(archives) != 7 {
		t.Fatalf("archives = %d, want 7", len(archives))
	}

	wantAgents := []string{"agent-c", "global", "agent-b", "agent-b-copy", "agent_working", "agent-personal-working", "agent-personal-working-prod-01"}
	for i, want := range wantAgents {
		if archives[i].Agent != want {
			t.Fatalf("archives[%d].Agent = %q, want %q", i, archives[i].Agent, want)
		}
	}
	if archives[0].Path != filepath.Join(dir, "myaaw-backup-agent-c-20261027-010000.tar.gz") {
		t.Fatalf("newest archive = %q", archives[0].Path)
	}
}

func TestListArchivesMissingDirectory(t *testing.T) {
	_, err := ListArchives(filepath.Join(t.TempDir(), "missing"))
	if err == nil || !os.IsNotExist(err) {
		t.Fatalf("err = %v, want not-exist error", err)
	}
}
