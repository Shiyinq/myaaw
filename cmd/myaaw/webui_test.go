package main

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func writeTestTarGz(t *testing.T, path string, files map[string]string) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(body)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header %q: %v", name, err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatalf("write body %q: %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
}

func TestExtractTarGz(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "bundle.tar.gz")
	writeTestTarGz(t, archive, map[string]string{
		"build/index.js": "console.log('hi')",
		"package.json":   "{}",
	})

	dest := filepath.Join(dir, "out")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}
	if err := extractTarGz(archive, dest); err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dest, "build", "index.js"))
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if string(got) != "console.log('hi')" {
		t.Fatalf("extracted content = %q", got)
	}
}

func TestExtractTarGzRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "bad.tar.gz")
	writeTestTarGz(t, archive, map[string]string{
		"../evil.txt": "pwned",
	})

	dest := filepath.Join(dir, "out")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}
	if err := extractTarGz(archive, dest); err == nil {
		t.Fatal("expected traversal entry to be rejected")
	}
	if _, err := os.Stat(filepath.Join(dir, "evil.txt")); err == nil {
		t.Fatal("traversal wrote a file outside the destination")
	}
}

func TestExtractTarGzRejectsAbsolute(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "abs.tar.gz")
	writeTestTarGz(t, archive, map[string]string{
		"/etc/evil.txt": "pwned",
	})

	dest := filepath.Join(dir, "out")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}
	if err := extractTarGz(archive, dest); err == nil {
		t.Fatal("expected absolute entry to be rejected")
	}
}

func TestWebUIConfigPersistsPublicMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg := webUIConfig{Port: "5050", APIURL: "http://localhost:9000", Version: "v1", NodePath: "/opt/node/bin/node", Public: true}
	if err := saveWebUIConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	got, err := loadWebUIConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !got.Public {
		t.Fatal("public mode was not persisted")
	}
	if got.NodePath != "/opt/node/bin/node" {
		t.Fatalf("node path was not persisted: %q", got.NodePath)
	}

	// Turning it off must also stick.
	cfg.Public = false
	if err := saveWebUIConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	got, err = loadWebUIConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got.Public {
		t.Fatal("public mode should be off")
	}
}

func TestParseNodeMajor(t *testing.T) {
	tests := []struct {
		in    string
		want  int
		valid bool
	}{
		{"v22.14.0\n", 22, true},
		{"v24.14.1\n", 24, true},
		{"v20.20.2\n", 20, true},
		{"22.0.0", 22, true},
		{"v22", 0, false},
		{"", 0, false},
		{"not-a-version", 0, false},
	}
	for _, tt := range tests {
		got, ok := parseNodeMajor(tt.in)
		if ok != tt.valid || got != tt.want {
			t.Errorf("parseNodeMajor(%q) = (%d, %v), want (%d, %v)", tt.in, got, ok, tt.want, tt.valid)
		}
	}
}

func TestDownloadFileWritesBody(t *testing.T) {
	payload := "hello world"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = io.WriteString(w, payload)
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "bundle.tar.gz")
	if err := downloadFile(server.URL, dest); err != nil {
		t.Fatalf("downloadFile: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(got) != payload {
		t.Fatalf("downloaded body = %q, want %q", got, payload)
	}
}

func TestDownloadFileRejectsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "bundle.tar.gz")
	if err := downloadFile(server.URL, dest); err == nil {
		t.Fatal("downloadFile accepted a 404 response")
	}
}

func writeFakeNode(t *testing.T, dir, version string) string {
	t.Helper()

	path := filepath.Join(dir, "node")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho "+version+"\n"), 0o755); err != nil {
		t.Fatalf("write fake node: %v", err)
	}
	return path
}

func TestCheckNodeRejectsOldVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake node scripts are POSIX-only")
	}

	node := writeFakeNode(t, t.TempDir(), "v20.20.2")
	if _, err := checkNode(node); err == nil {
		t.Fatal("checkNode accepted Node 20")
	}
}

func TestNodeForConfigUsesPinnedPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake node scripts are POSIX-only")
	}

	node := writeFakeNode(t, t.TempDir(), "v24.1.0")
	// An empty PATH proves the pinned path is what resolved the binary.
	t.Setenv("PATH", t.TempDir())

	got, err := nodeForConfig(webUIConfig{NodePath: node})
	if err != nil {
		t.Fatalf("nodeForConfig: %v", err)
	}
	if got != node {
		t.Fatalf("nodeForConfig = %q, want %q", got, node)
	}
}

func TestNodeForConfigFallsBackWhenPinIsMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake node scripts are POSIX-only")
	}

	bin := t.TempDir()
	node := writeFakeNode(t, bin, "v24.1.0")
	t.Setenv("PATH", bin)

	got, err := nodeForConfig(webUIConfig{NodePath: filepath.Join(bin, "removed-by-upgrade")})
	if err != nil {
		t.Fatalf("nodeForConfig: %v", err)
	}
	if got != node {
		t.Fatalf("nodeForConfig = %q, want %q", got, node)
	}
}

func TestFindNodeReportsMissingPATH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake node scripts are POSIX-only")
	}

	empty := t.TempDir()
	t.Setenv("PATH", empty)

	_, err := findNode()
	if err == nil {
		t.Fatal("findNode found a node in an empty PATH")
	}
	if !strings.Contains(err.Error(), "was not found in PATH") || !strings.Contains(err.Error(), empty) {
		t.Fatalf("error does not report the searched PATH: %v", err)
	}
}

func TestStartWebUIConfigKeepsStoredVersion(t *testing.T) {
	stored := webUIConfig{Port: "5050", APIURL: "http://localhost:9000", Version: "v2"}
	desired := webUIConfig{Port: "6000", APIURL: "http://api.internal", Version: "v1", Public: true}

	got := startWebUIConfig(stored, desired, "/opt/node/bin/node")

	if got.Version != "v2" {
		t.Fatalf("version = %q, want the stored v2", got.Version)
	}
	if got.Port != "6000" || got.APIURL != "http://api.internal" || !got.Public {
		t.Fatalf("start values not applied: %+v", got)
	}
	if got.NodePath != "/opt/node/bin/node" {
		t.Fatalf("node path = %q", got.NodePath)
	}
}
