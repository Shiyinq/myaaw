package main

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
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

	cfg := webUIConfig{Port: "5050", APIURL: "http://localhost:9000", Version: "v1", Public: true}
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
