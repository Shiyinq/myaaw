package service

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestOpenControlPlaneDBSchemaPermsCascadeAndConcurrency(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "controlplane")
	path := filepath.Join(dir, "controlplane.db")

	cpdb, err := OpenControlPlaneDBAt(path)
	if err != nil {
		t.Fatalf("OpenControlPlaneDBAt: %v", err)
	}
	defer cpdb.Close()

	if runtime.GOOS != "windows" {
		if info, err := os.Stat(dir); err != nil {
			t.Fatalf("stat dir: %v", err)
		} else if info.Mode().Perm() != 0o700 {
			t.Fatalf("dir mode = %o, want 700", info.Mode().Perm())
		}
		if info, err := os.Stat(path); err != nil {
			t.Fatalf("stat db: %v", err)
		} else if info.Mode().Perm() != 0o600 {
			t.Fatalf("db mode = %o, want 600", info.Mode().Perm())
		}
	}

	var version int
	if err := cpdb.DB().QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("user_version: %v", err)
	}
	if version != controlPlaneSchemaVersion {
		t.Fatalf("user_version = %d, want %d", version, controlPlaneSchemaVersion)
	}

	var fk int
	if err := cpdb.DB().QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("foreign_keys = %d, err = %v, want 1", fk, err)
	}

	// Verify ON DELETE CASCADE removes group_members and group_messages.
	if _, err := cpdb.DB().Exec(
		`INSERT INTO groups (id, name, created_at, updated_at) VALUES ('g1', 'Room', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
	); err != nil {
		t.Fatalf("insert group: %v", err)
	}
	if _, err := cpdb.DB().Exec(
		`INSERT INTO group_members (group_id, name, position) VALUES ('g1', 'alice', 0)`,
	); err != nil {
		t.Fatalf("insert member: %v", err)
	}
	if _, err := cpdb.DB().Exec(
		`INSERT INTO group_messages (id, group_id, sender_type, sender_name, content, created_at) VALUES ('m1', 'g1', 'user', 'User', 'hi', '2026-01-01T00:00:00Z')`,
	); err != nil {
		t.Fatalf("insert message: %v", err)
	}
	if _, err := cpdb.DB().Exec(`DELETE FROM groups WHERE id = 'g1'`); err != nil {
		t.Fatalf("delete group: %v", err)
	}

	var memberCount, msgCount int
	_ = cpdb.DB().QueryRow(`SELECT COUNT(*) FROM group_members`).Scan(&memberCount)
	_ = cpdb.DB().QueryRow(`SELECT COUNT(*) FROM group_messages`).Scan(&msgCount)
	if memberCount != 0 || msgCount != 0 {
		t.Fatalf("cascade failed: members=%d messages=%d", memberCount, msgCount)
	}

	// Verify concurrent AuthService + GroupStore writes on shared handle never hit SQLITE_BUSY.
	auth, err := NewAuthServiceWithDB(cpdb)
	if err != nil {
		t.Fatalf("NewAuthServiceWithDB: %v", err)
	}
	store := NewGroupStoreWithDB(cpdb)

	var wg sync.WaitGroup
	errCh := make(chan error, 20)
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := auth.UpdateSettings(SettingsPatch{ThemeMode: strPtr("dark")}); err != nil {
				errCh <- err
			}
		}()
		go func(idx int) {
			defer wg.Done()
			id := "grp-" + string(rune('a'+idx))
			if err := store.Create(&storedGroup{
				Group: Group{
					ID:        id,
					Name:      id,
					Members:   []GroupMember{{Name: "alice"}},
					CreatedAt: "2026-01-01T00:00:00Z",
					UpdatedAt: "2026-01-01T00:00:00Z",
				},
			}); err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent write error: %v", err)
	}
}
