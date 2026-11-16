package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeProjectFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestProjectStatusStatic(t *testing.T) {
	_, agentRoot := setupFileAgent(t)
	site := filepath.Join(agentRoot, "home", "workspace", "site")
	writeProjectFile(t, site, "index.html", "<h1>hi</h1>")
	writeProjectFile(t, site, "assets/app.js", "console.log('hi')")

	projects := NewProjectService(NewFileService())
	status, err := projects.Status("myfriend", "/home/workspace/site")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Type != "static" || status.State != "ready" {
		t.Fatalf("status = %+v", status)
	}
	if status.Entry != "index.html" {
		t.Errorf("entry = %q", status.Entry)
	}
	if status.FileCount != 2 || status.Size == 0 {
		t.Errorf("count/size = %d/%d", status.FileCount, status.Size)
	}
}

func TestProjectStatusRejectsFile(t *testing.T) {
	setupFileAgent(t)
	projects := NewProjectService(NewFileService())

	_, err := projects.Status("myfriend", "/home/AGENTS.md")
	if !errors.Is(err, ErrProjectNotDirectory) {
		t.Fatalf("err = %v, want ErrProjectNotDirectory", err)
	}
}

func TestProjectPreviewFileRejectsTraversal(t *testing.T) {
	_, agentRoot := setupFileAgent(t)
	site := filepath.Join(agentRoot, "home", "workspace", "site")
	writeProjectFile(t, site, "index.html", "<h1>hi</h1>")
	writeProjectFile(t, site, "assets/app.js", "console.log('hi')")

	projects := NewProjectService(NewFileService())
	abs, err := projects.PreviewFile("myfriend", "/home/workspace/site", "assets/app.js")
	if err != nil {
		t.Fatalf("PreviewFile: %v", err)
	}
	if filepath.Base(abs) != "app.js" {
		t.Errorf("abs = %q", abs)
	}

	for _, rel := range []string{"../AGENTS.md", ".env", "node_modules/pkg/index.js", "/etc/passwd"} {
		if _, err := projects.PreviewFile("myfriend", "/home/workspace/site", rel); err == nil {
			t.Errorf("PreviewFile(%q) should fail", rel)
		}
	}
}

func TestProjectStatusLargeIsDownloadOnly(t *testing.T) {
	_, agentRoot := setupFileAgent(t)
	site := filepath.Join(agentRoot, "home", "workspace", "big")
	writeProjectFile(t, site, "index.html", "hi")
	for i := 0; i < ProjectMaxFiles; i++ {
		writeProjectFile(t, site, fmt.Sprintf("files/f%03d.txt", i), "x")
	}

	projects := NewProjectService(NewFileService())
	status, err := projects.Status("myfriend", "/home/workspace/big")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !status.DownloadOnly || status.State != "failed" {
		t.Fatalf("status = %+v", status)
	}
}

func TestProjectTokenRoundTrip(t *testing.T) {
	signer := &MediaSigner{key: []byte("01234567890123456789012345678901"), ttl: ProjectPreviewTTL, now: time.Now}
	token, exp := signer.SignProjectToken("myfriend", "/home/workspace/site")
	if exp == 0 || token == "" {
		t.Fatalf("token = %q exp = %d", token, exp)
	}
	path, ok := signer.VerifyProjectToken("myfriend", token)
	if !ok || path != "/home/workspace/site" {
		t.Fatalf("verify = %q, %v", path, ok)
	}
	if _, ok := signer.VerifyProjectToken("other", token); ok {
		t.Fatal("token must not verify for another agent")
	}
	if _, ok := signer.VerifyProjectToken("myfriend", token+"x"); ok {
		t.Fatal("tampered token must not verify")
	}
}

func TestNPMExecutableOverride(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "npm")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MYAAW_NPM", bin)

	got, err := npmExecutable()
	if err != nil {
		t.Fatalf("npmExecutable: %v", err)
	}
	if got != bin {
		t.Fatalf("npm = %q, want %q", got, bin)
	}
}

func TestNPMExecutableRejectsBadOverride(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "npm")
	t.Setenv("MYAAW_NPM", missing)

	if _, err := npmExecutable(); err == nil || !strings.Contains(err.Error(), "MYAAW_NPM") {
		t.Fatalf("err = %v, want an MYAAW_NPM hint", err)
	}
}

func TestNPMExecutablePrefersNewestVersionManager(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PATH", "")

	writeBin := func(version string) string {
		t.Helper()
		dir := filepath.Join(home, ".nvm", "versions", "node", version, "bin")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"npm", "node"} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		return filepath.Join(dir, "npm")
	}
	writeBin("v18.20.0")
	newest := writeBin("v22.1.0")

	got, err := npmExecutable()
	if err != nil {
		t.Fatalf("npmExecutable: %v", err)
	}
	if got != newest {
		t.Fatalf("npm = %q, want %q", got, newest)
	}
}

func TestEnvWithPathPrependsExactlyOnce(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/bin")

	env := envWithPath("/opt/homebrew/bin")
	count := 0
	for _, entry := range env {
		if !strings.HasPrefix(entry, "PATH=") {
			continue
		}
		count++
		if entry != "PATH=/opt/homebrew/bin:/usr/bin:/bin" {
			t.Fatalf("PATH = %q", entry)
		}
	}
	if count != 1 {
		t.Fatalf("PATH entries = %d, want 1", count)
	}
}

func TestRewritePreviewPaths(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "index.html", `<link href="/assets/app.css"><script src='/assets/app.js'></script><a href="/">home</a><a href="https://example.com/x">x</a>`)
	writeProjectFile(t, root, "about/index.html", `<script src="/assets/app.js"></script>`)
	writeProjectFile(t, root, "assets/app.css", `body{background:url(/fonts/a.woff2)} @import "/assets/more.css"; .x{background:url(//cdn.example/x.png)}`)

	if err := rewritePreviewPaths(root); err != nil {
		t.Fatalf("rewritePreviewPaths: %v", err)
	}

	index, _ := os.ReadFile(filepath.Join(root, "index.html"))
	wantIndex := `<link href="./assets/app.css"><script src='./assets/app.js'></script><a href="./">home</a><a href="https://example.com/x">x</a>`
	if string(index) != wantIndex {
		t.Fatalf("index = %s", index)
	}

	nested, _ := os.ReadFile(filepath.Join(root, "about", "index.html"))
	if !strings.Contains(string(nested), `src="../assets/app.js"`) {
		t.Fatalf("nested = %s", nested)
	}

	css, _ := os.ReadFile(filepath.Join(root, "assets", "app.css"))
	if !strings.Contains(string(css), `url(../fonts/a.woff2)`) {
		t.Fatalf("css = %s", css)
	}
	if !strings.Contains(string(css), `@import "../assets/more.css"`) {
		t.Fatalf("css import = %s", css)
	}
	if !strings.Contains(string(css), `url(//cdn.example/x.png)`) {
		t.Fatalf("protocol-relative URL changed: %s", css)
	}
}

func TestUsesViteRelativeBase(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "vite.config.ts", "export default {}")
	if !usesViteRelativeBase(dir) {
		t.Fatal("plain Vite project should use the relative base")
	}

	writeProjectFile(t, dir, "svelte.config.js", "export default {}")
	if usesViteRelativeBase(dir) {
		t.Fatal("SvelteKit owns its base; the flag must be skipped")
	}
}

func TestNodeVersionRank(t *testing.T) {
	cases := map[string]int{
		"/home/u/.nvm/versions/node/v22.1.0/bin/npm": 22_001_000,
		"/home/u/.nvm/versions/node/v9.11.2/bin/npm": 9_011_002,
		"/usr/local/bin/npm":                         0,
	}
	for path, want := range cases {
		if got := nodeVersionRank(path); got != want {
			t.Errorf("nodeVersionRank(%q) = %d, want %d", path, got, want)
		}
	}
}
