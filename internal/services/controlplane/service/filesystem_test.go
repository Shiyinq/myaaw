package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func setupFileAgent(t *testing.T) (string, string) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	agentRoot := filepath.Join(home, ".myaaw", "myfriend")
	for _, dir := range []string{
		filepath.Join(agentRoot, "home", "notes"),
		filepath.Join(agentRoot, "skills"),
		filepath.Join(agentRoot, "jobs"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(agentRoot, "home", "AGENTS.md"), []byte("# Agents"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentRoot, "home", "notes", "todo.md"), []byte("buy milk\nwalk cat"), 0o644); err != nil {
		t.Fatalf("write todo.md: %v", err)
	}

	return home, agentRoot
}

func TestFileServiceListsVirtualRoots(t *testing.T) {
	setupFileAgent(t)
	files := NewFileService()

	entries, err := files.List("myfriend", "")
	if err != nil {
		t.Fatalf("List roots: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("roots = %+v", entries)
	}
	if entries[0].Path != "/home" || entries[1].Path != "/skills" || entries[2].Path != "/jobs" {
		t.Fatalf("roots = %+v", entries)
	}
}

func TestFileServiceReadsAndWrites(t *testing.T) {
	setupFileAgent(t)
	files := NewFileService()

	written, err := files.Write("myfriend", "/home/AGENTS.md", "# Updated")
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if written.Path != "/home/AGENTS.md" {
		t.Fatalf("written = %+v", written)
	}

	read, err := files.Read("myfriend", "/home/AGENTS.md")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if read.Content != "# Updated" {
		t.Fatalf("content = %q", read.Content)
	}
}

func TestFileServiceListsDirectory(t *testing.T) {
	setupFileAgent(t)
	files := NewFileService()

	entries, err := files.List("myfriend", "/home")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) < 2 {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].Name != "notes" || !entries[0].IsDir {
		t.Fatalf("first entry = %+v", entries[0])
	}
}

func TestFileServiceRejectsTraversalAndForeignPaths(t *testing.T) {
	setupFileAgent(t)
	files := NewFileService()

	for _, virtualPath := range []string{
		"/home/../../etc/passwd",
		"/etc/passwd",
		"/home/../config",
		"../../etc/passwd",
	} {
		if _, err := files.Read("myfriend", virtualPath); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("Read(%q) err = %v, want ErrInvalidPath", virtualPath, err)
		}
	}
}

func TestFileServiceRejectsSymlinkEscape(t *testing.T) {
	home, agentRoot := setupFileAgent(t)
	outside := filepath.Join(home, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(agentRoot, "home", "link")); err != nil {
		t.Skipf("symlinks not available: %v", err)
	}

	files := NewFileService()
	if _, err := files.Read("myfriend", "/home/link/secret.txt"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("err = %v, want ErrInvalidPath", err)
	}
}

func TestFileServiceProtectsRoots(t *testing.T) {
	setupFileAgent(t)
	files := NewFileService()

	if err := files.Delete("myfriend", "/home", true); !errors.Is(err, ErrRootProtected) {
		t.Fatalf("delete root err = %v, want ErrRootProtected", err)
	}
	if err := files.Rename("myfriend", "/home", "/home2"); !errors.Is(err, ErrRootProtected) {
		t.Fatalf("rename root err = %v, want ErrRootProtected", err)
	}
}

func TestFileServiceRenameAndDelete(t *testing.T) {
	setupFileAgent(t)
	files := NewFileService()

	if err := files.Rename("myfriend", "/home/notes/todo.md", "/home/notes/done.md"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if _, err := files.Read("myfriend", "/home/notes/todo.md"); !errors.Is(err, ErrPathNotFound) {
		t.Fatalf("old path err = %v, want ErrPathNotFound", err)
	}
	if _, err := files.Read("myfriend", "/home/notes/done.md"); err != nil {
		t.Fatalf("new path: %v", err)
	}
	if err := files.Delete("myfriend", "/home/notes/done.md", false); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestFileServiceSearch(t *testing.T) {
	setupFileAgent(t)
	files := NewFileService()

	matches, err := files.Search("myfriend", "/home", "milk")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches = %+v", matches)
	}
	if matches[0].Path != "/home/notes/todo.md" || matches[0].Line != 1 {
		t.Fatalf("match = %+v", matches[0])
	}
}

func TestFileServiceUnknownAgent(t *testing.T) {
	setupFileAgent(t)
	files := NewFileService()

	if _, err := files.List("missing", "/home"); !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("err = %v, want ErrAgentNotFound", err)
	}
}
