//go:build darwin

package computer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProbeAllHealthy(t *testing.T) {
	origRun, origLook := runCommand, lookPath
	defer func() { runCommand, lookPath = origRun, origLook }()

	lookPath = func(name string) (string, error) {
		if name == "cliclick" || name == "screencapture" {
			return "/usr/local/bin/" + name, nil
		}
		return "", os.ErrNotExist
	}
	runCommand = func(_ context.Context, name string, args ...string) (string, error) {
		switch name {
		case "screencapture":
			if err := os.WriteFile(args[len(args)-1], []byte("PNG"), 0o644); err != nil {
				return "", err
			}
			return "", nil
		case "osascript":
			if strings.Contains(args[len(args)-1], "System Events") {
				return "Finder", nil
			}
			return "0, 0, 1440, 900", nil
		default:
			return "", nil
		}
	}

	results := Probe(context.Background())
	byName := map[string]ProbeResult{}
	for _, result := range results {
		byName[result.Name] = result
	}
	for _, name := range []string{"Operating system", "cliclick", "Screen Recording permission", "System Events access", "Desktop size"} {
		result, ok := byName[name]
		if !ok {
			t.Fatalf("missing probe %q: %+v", name, results)
		}
		if !result.OK {
			t.Fatalf("probe %q = %+v, want OK", name, result)
		}
	}
	if detail := byName["Desktop size"].Detail; detail != "1440x900 points" {
		t.Fatalf("desktop size detail = %q", detail)
	}
}

func TestProbeReportsFailures(t *testing.T) {
	origRun, origLook := runCommand, lookPath
	origCandidates := cliclickPaths
	defer func() { runCommand, lookPath, cliclickPaths = origRun, origLook, origCandidates }()
	cliclickPaths = nil

	lookPath = func(name string) (string, error) {
		if name == "screencapture" {
			return "/usr/sbin/screencapture", nil
		}
		return "", os.ErrNotExist
	}
	runCommand = func(_ context.Context, name string, args ...string) (string, error) {
		return "", fmt.Errorf("%s denied", name)
	}

	results := Probe(context.Background())
	failures := map[string]ProbeResult{}
	for _, result := range results {
		if !result.OK {
			failures[result.Name] = result
		}
	}
	for _, name := range []string{"cliclick", "Screen Recording permission", "System Events access"} {
		result, ok := failures[name]
		if !ok {
			t.Fatalf("expected %q to fail: %+v", name, results)
		}
		if result.Fix == "" {
			t.Fatalf("probe %q has no fix hint: %+v", name, result)
		}
	}
}

func TestResolveCliclickFallback(t *testing.T) {
	origLook := lookPath
	origCandidates := cliclickPaths
	defer func() { lookPath, cliclickPaths = origLook, origCandidates }()

	// PATH lookup fails (launchd-style minimal PATH), Homebrew path succeeds.
	lookPath = func(string) (string, error) { return "", os.ErrNotExist }
	bin := filepath.Join(t.TempDir(), "cliclick")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cliclickPaths = []string{"/does/not/exist/cliclick", bin}
	path, err := resolveCliclick()
	if err != nil || path != bin {
		t.Fatalf("resolveCliclick() = %q, %v; want %q", path, err, bin)
	}

	// Nothing available -> ErrUnsupportedAction.
	cliclickPaths = []string{"/does/not/exist/cliclick"}
	if _, err := resolveCliclick(); !errors.Is(err, ErrUnsupportedAction) {
		t.Fatalf("resolveCliclick() error = %v, want ErrUnsupportedAction", err)
	}
}
