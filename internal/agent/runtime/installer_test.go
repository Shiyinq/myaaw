package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
	"time"

	"myaaw/internal/config"
)

func TestInstallMethodsAllowlistAndAuthArgs(t *testing.T) {
	for _, preset := range []string{"antigravity", "claude_code", "codex", "opencode", "omp", "pi"} {
		methods := InstallMethodsForPreset(preset)
		if len(methods) == 0 {
			t.Fatalf("expected at least 1 official install method for %s on %s", preset, goruntime.GOOS)
		}
		for _, m := range methods {
			if m.ID == "" || m.CommandPreview == "" || m.SourceURL == "" {
				t.Fatalf("incomplete install method for %s: %+v", preset, m)
			}
			if !strings.HasPrefix(m.SourceURL, "https://") {
				t.Fatalf("non-https source URL for %s: %s", preset, m.SourceURL)
			}
		}
		if OfficialDocsURL(preset) == "" {
			t.Fatalf("missing OfficialDocsURL for %s", preset)
		}
	}

	if got := InstallMethodsForPreset("myaaw"); len(got) != 0 {
		t.Fatalf("myaaw should have no external install methods, got %+v", got)
	}
	if got := InstallMethodsForPreset("custom_acp"); len(got) != 0 {
		t.Fatalf("custom_acp should have no vendor install methods, got %+v", got)
	}

	err := RunOfficialInstall(context.Background(), "unknown_cli", "official_script", nil)
	if !errors.Is(err, ErrUnsupportedRuntimePreset) {
		t.Fatalf("expected ErrUnsupportedRuntimePreset, got %v", err)
	}
	err = RunOfficialInstall(context.Background(), "claude_code", "arbitrary_shell_injection", nil)
	if !errors.Is(err, ErrUnsupportedInstallMethod) {
		t.Fatalf("expected ErrUnsupportedInstallMethod, got %v", err)
	}

	bin, args, preview, err := AuthCommandArgs("codex", "login", true)
	if err != nil || bin != "codex" || preview != "codex login --device-auth" || len(args) != 2 {
		t.Fatalf("unexpected codex headless auth args: bin=%q args=%v preview=%q err=%v", bin, args, preview, err)
	}
	if _, _, _, err := AuthCommandArgs("custom_acp", "login", false); !errors.Is(err, ErrUnsupportedAuthAction) {
		t.Fatalf("expected ErrUnsupportedAuthAction for custom_acp, got %v", err)
	}
}

func TestProbeAuthStatusAndTimeoutSafety(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("shell script fake binaries use POSIX sh")
	}

	dir := t.TempDir()
	writeScript := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatalf("write fake script %s: %v", name, err)
		}
		return p
	}

	t.Setenv("ANTHROPIC_API_KEY", "")
	claudeOK := writeScript("claude-ok", `echo '{"loggedIn":true,"authMethod":"claude.ai"}'; exit 0`)
	claudePreset, _ := config.LookupRuntimePreset("claude_code")
	st := ProbeAuthStatus(context.Background(), claudePreset, claudeOK, "", false)
	if st.Status != AuthStatusAuthenticated || st.Detail != "claude.ai" {
		out, code, probeErr := runNonInteractiveProbe(context.Background(), claudeOK, "auth", "status")
		t.Fatalf("claudeOK auth status = %+v (out=%q code=%d err=%v), want authenticated (claude.ai)", st, out, code, probeErr)
	}

	claudeUnauth := writeScript("claude-unauth", `echo '{"loggedIn":false}'; exit 1`)
	st = ProbeAuthStatus(context.Background(), claudePreset, claudeUnauth, "ANTHROPIC_API_KEY", false)
	if st.Status != AuthStatusUnauthenticated {
		t.Fatalf("claudeUnauth status = %+v, want unauthenticated", st)
	}

	stWithKey := ProbeAuthStatus(context.Background(), claudePreset, claudeUnauth, "ANTHROPIC_API_KEY", true)
	if stWithKey.Status != AuthStatusAPIKeyConfigured {
		t.Fatalf("claudeUnauth with saved global key = %+v, want api_key_configured", stWithKey)
	}

	hungScript := writeScript("agy-hung", `sleep 30`)
	agyPreset, _ := config.LookupRuntimePreset("antigravity")
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	_ = ProbeAuthStatus(ctx, agyPreset, hungScript, "", false)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("hung probe took too long (%v); process group was not terminated", elapsed)
	}

	t.Setenv("OPENAI_API_KEY", "")
	codexOK := writeScript("codex-ok", `
if [ "$1" = "login" ] && [ "$2" = "status" ]; then
  echo "Logged in using ChatGPT"
  exit 0
fi
if [ "$1" = "app-server" ]; then
  while IFS= read -r line; do
    case "$line" in
      *'"initialize"'*)
        echo '{"jsonrpc":"2.0","id":1,"result":{}}'
        ;;
      *'"model/list"'*)
        echo '{"jsonrpc":"2.0","id":2,"result":{"data":[{"id":"gpt-5.4","model":"gpt-5.4","displayName":"GPT-5.4","isDefault":true},{"id":"gpt-5.3-codex","model":"gpt-5.3-codex","displayName":"GPT-5.3 Codex"}]}}'
        exit 0
        ;;
    esac
  done
fi
exit 1
`)
	codexPreset, _ := config.LookupRuntimePreset("codex")
	codexStatus := ProbeAuthStatus(context.Background(), codexPreset, codexOK, "", false)
	if codexStatus.Status != AuthStatusAuthenticated || codexStatus.Detail != "ChatGPT · 2 models" {
		t.Fatalf("codexOK auth status = %+v, want authenticated (ChatGPT · 2 models)", codexStatus)
	}
}

func TestWritableNPMPrefixFallbackAndLocalBinLink(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("shell script fake binaries use POSIX sh")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	// Fake nvm-style directory where npm relies on finding sibling `fake-node` via PATH
	// (simulating launchd where system PATH lacks nvm's bin directory).
	nvmPrefix := filepath.Join(home, ".nvm", "versions", "node", "v24.0.0")
	nvmBin := filepath.Join(nvmPrefix, "bin")
	if err := os.MkdirAll(filepath.Join(nvmPrefix, "lib", "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nvmBin, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeNode := filepath.Join(nvmBin, "fake-node-sibling")
	if err := os.WriteFile(fakeNode, []byte("#!/bin/sh\necho \""+nvmPrefix+"\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	fakeNPM := filepath.Join(nvmBin, "npm")
	if err := os.WriteFile(fakeNPM, []byte("#!/bin/sh\nexec fake-node-sibling\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Minimal launchd-like PATH without nvmBin.
	t.Setenv("PATH", "/usr/bin:/bin")
	if fallback := writableNPMPrefixFallback(context.Background(), fakeNPM); fallback != "" {
		t.Fatalf("expected writable nvm prefix to skip fallback, got %q", fallback)
	}

	// When fallback IS used, exposeFallbackBinaryToLocalBin links ~/.npm-global/bin/codex -> ~/.local/bin/codex.
	fallbackPrefix := filepath.Join(home, ".npm-global")
	if err := os.MkdirAll(filepath.Join(fallbackPrefix, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	codexSrc := filepath.Join(fallbackPrefix, "bin", "codex")
	if err := os.WriteFile(codexSrc, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var linkedMsg string
	exposeFallbackBinaryToLocalBin("codex", fallbackPrefix, func(line string) { linkedMsg = line })
	dst := filepath.Join(home, ".local", "bin", "codex")
	target, err := os.Readlink(dst)
	if err != nil {
		t.Fatalf("expected symlink at %s, got err: %v", dst, err)
	}
	if target != codexSrc || !strings.Contains(linkedMsg, dst) {
		t.Fatalf("unexpected symlink target=%q msg=%q", target, linkedMsg)
	}
}
