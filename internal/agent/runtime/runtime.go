package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"
	"time"

	"myaaw/internal/agent"
	"myaaw/internal/agentdir"
	"myaaw/internal/config"
	"myaaw/internal/provider"
)

// Runtime extends agent.AgentProvider with lifecycle and display metadata so
// BotServiceImpl can run both native Myaaw and external CLI agents uniformly.
type Runtime interface {
	agent.AgentProvider
	// Type returns the protocol driver name (myaaw, acp, claude_code, omp, pi, codex).
	Type() string
	// Preset returns the catalog preset id (e.g. "myaaw", "antigravity", "claude_code").
	Preset() string
	// DisplayProvider returns a short identifier stored in user metadata and traces.
	DisplayProvider() string
	// DisplayModel returns the configured model label (or "default" when CLI-managed).
	DisplayModel() string
	// WorkingDir returns the resolved workspace directory for this runtime.
	WorkingDir() string
	// Capabilities reports which features this runtime handles natively vs delegates.
	Capabilities() config.RuntimeCapabilities
	// IsExternal reports whether turns are executed by an external CLI subprocess.
	IsExternal() bool
	// Close drains and terminates any pooled subprocesses or IPC listeners.
	Close() error
}

// Create builds the configured Runtime. Native "myaaw" requires a non-nil
// LLMProvider; external drivers do not require an LLMProvider.
func Create(raw config.RuntimeConfig, llm provider.LLMProvider) (Runtime, error) {
	cfg := config.MergeRuntime(&raw)
	if !cfg.IsExternal() {
		if llm == nil {
			return nil, errors.New("native myaaw runtime requires a configured LLM provider")
		}
		return newNativeRuntime(llm), nil
	}

	if err := config.ValidateRuntime(&cfg, nil); err != nil {
		return nil, err
	}

	switch cfg.Type {
	case config.RuntimeTypeACP:
		return newACPRuntime(cfg), nil
	case config.RuntimeTypeClaudeCode:
		return newClaudeRuntime(cfg), nil
	case config.RuntimeTypeOMP, config.RuntimeTypePi:
		return newPiRPCRuntime(cfg), nil
	case config.RuntimeTypeCodex:
		return newCodexRuntime(cfg), nil
	default:
		return nil, fmt.Errorf("unsupported runtime type %q", cfg.Type)
	}
}

// DetectedRuntime combines a catalog preset with host binary discovery status.
type DetectedRuntime struct {
	config.RuntimePreset
	Installed  bool   `json:"installed"`
	BinaryPath string `json:"binary_path,omitempty"`
	Version    string `json:"version,omitempty"`
}

// Discover probes the host PATH for every built-in runtime preset.
func Discover(ctx context.Context) []DetectedRuntime {
	presets := config.RuntimePresets()
	out := make([]DetectedRuntime, 0, len(presets))
	for _, preset := range presets {
		entry := DetectedRuntime{RuntimePreset: preset}
		if preset.Type == config.RuntimeTypeMyaaw {
			entry.Installed = true
			entry.Version = "built-in"
			out = append(out, entry)
			continue
		}
		if preset.DefaultCommand == "" {
			out = append(out, entry)
			continue
		}
		if path, err := ResolveBinary(preset.DefaultCommand); err == nil {
			entry.Installed = true
			entry.BinaryPath = path
			entry.Version = probeBinaryVersion(ctx, path)
		}
		out = append(out, entry)
	}
	return out
}

// ResolveBinary locates a CLI binary across the current PATH plus common user
// and package-manager bin directories (~/.local/bin, Homebrew, nvm, bun, etc.)
// so background daemons launched by macOS launchd or Linux systemd still find
// installed CLIs like agy, pi, omp, claude, codex, and opencode.
func ResolveBinary(command string) (string, error) {
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return "", errors.New("empty runtime command")
	}
	if strings.ContainsRune(cmd, filepath.Separator) || strings.HasPrefix(cmd, "~") {
		expanded := expandHome(cmd)
		if isExecutableFile(expanded) {
			return expanded, nil
		}
		return exec.LookPath(expanded)
	}
	if found, err := exec.LookPath(cmd); err == nil {
		return found, nil
	}
	for _, dir := range extraSearchDirs() {
		candidate := filepath.Join(dir, cmd)
		if isExecutableFile(candidate) {
			return candidate, nil
		}
		if goruntime.GOOS == "windows" {
			for _, ext := range []string{".exe", ".cmd", ".bat"} {
				if isExecutableFile(candidate + ext) {
					return candidate + ext, nil
				}
			}
		}
	}
	return "", fmt.Errorf("%q not found in PATH or common user bin directories", cmd)
}

// EnrichedPATH merges the current process PATH with common user bin directories
// so #!/usr/bin/env node/python scripts launched from launchd/systemd succeed.
func EnrichedPATH() string {
	existing := filepath.SplitList(os.Getenv("PATH"))
	seen := make(map[string]struct{}, len(existing)+16)
	out := make([]string, 0, len(existing)+16)
	add := func(dir string) {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			return
		}
		if _, ok := seen[dir]; ok {
			return
		}
		seen[dir] = struct{}{}
		out = append(out, dir)
	}
	for _, d := range existing {
		add(d)
	}
	for _, d := range extraSearchDirs() {
		add(d)
	}
	return strings.Join(out, string(os.PathListSeparator))
}

func extraSearchDirs() []string {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		staticUserDirs := []string{
			filepath.Join(home, ".local", "bin"),
			filepath.Join(home, ".gemini", "antigravity", "bin"),
			filepath.Join(home, ".antigravity", "antigravity", "bin"),
			filepath.Join(home, ".antigravity-ide", "antigravity-ide", "bin"),
			filepath.Join(home, ".opencode", "bin"),
			filepath.Join(home, ".bun", "bin"),
			filepath.Join(home, ".volta", "bin"),
			filepath.Join(home, ".npm-global", "bin"),
			filepath.Join(home, ".yarn", "bin"),
			filepath.Join(home, ".pnpm"),
			filepath.Join(home, "Library", "pnpm"),
			filepath.Join(home, ".asdf", "shims"),
			filepath.Join(home, ".mise", "shims"),
			filepath.Join(home, ".local", "share", "mise", "shims"),
			filepath.Join(home, ".cargo", "bin"),
			filepath.Join(home, "go", "bin"),
		}
		for _, d := range staticUserDirs {
			if info, err := os.Stat(d); err == nil && info.IsDir() {
				dirs = append(dirs, d)
			}
		}
		// Include version-managed Node bins (nvm, fnm), newest version first.
		for _, pattern := range []string{
			filepath.Join(home, ".nvm", "versions", "node", "*", "bin"),
			filepath.Join(home, ".fnm", "node-versions", "*", "installation", "bin"),
		} {
			matches, _ := filepath.Glob(pattern)
			sort.Sort(sort.Reverse(sort.StringSlice(matches)))
			for _, m := range matches {
				if info, err := os.Stat(m); err == nil && info.IsDir() {
					dirs = append(dirs, m)
				}
			}
		}
	}
	if goruntime.GOOS != "windows" {
		for _, d := range []string{"/opt/homebrew/bin", "/opt/homebrew/sbin", "/usr/local/bin"} {
			if info, err := os.Stat(d); err == nil && info.IsDir() {
				dirs = append(dirs, d)
			}
		}
	}
	return dirs
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if goruntime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}

func probeBinaryVersion(parent context.Context, binary string) string {
	ctx, cancel := context.WithTimeout(parent, 1500*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--version")
	cmd.Env = append(os.Environ(), "PATH="+EnrichedPATH())
	raw, err := cmd.CombinedOutput()
	if err != nil && len(raw) == 0 {
		return ""
	}
	line := strings.TrimSpace(string(raw))
	if idx := strings.IndexByte(line, '\n'); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	if len(line) > 80 {
		line = line[:80]
	}
	return line
}

// ResolveWorkingDir returns cfg.WorkingDir when set, otherwise the active
// agent's isolated ~/.myaaw/agents/<name>/home/workspace directory.
func ResolveWorkingDir(cfg config.RuntimeConfig) string {
	if wd := strings.TrimSpace(cfg.WorkingDir); wd != "" {
		return expandHome(wd)
	}
	if ws, err := agent.EnsureWorkspace(); err == nil && ws != "" {
		return ws
	}
	if home, err := agentdir.ActiveHome(); err == nil && home != "" {
		ws := filepath.Join(home, "workspace")
		_ = os.MkdirAll(ws, 0o755)
		return ws
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return "."
}

// AllowedDirs returns the deduplicated list of host directories an external
// runtime should be permitted to access (working dir + agent home/skills/user +
// any user-configured additional_dirs).
func AllowedDirs(cfg config.RuntimeConfig) []string {
	candidates := []string{ResolveWorkingDir(cfg)}
	if home, err := agentdir.ActiveHome(); err == nil && home != "" {
		candidates = append(candidates, home)
	}
	if skillsDir, err := agentdir.ActiveSkillsDir(); err == nil && skillsDir != "" {
		candidates = append(candidates, skillsDir)
	}
	if userDir, err := agentdir.ActiveUserDir(); err == nil && userDir != "" {
		candidates = append(candidates, userDir)
	}
	for _, extra := range cfg.AdditionalDirs {
		if trimmed := strings.TrimSpace(extra); trimmed != "" {
			candidates = append(candidates, expandHome(trimmed))
		}
	}

	seen := make(map[string]struct{}, len(candidates))
	out := make([]string, 0, len(candidates))
	for _, dir := range candidates {
		abs := dir
		if resolved, err := filepath.Abs(dir); err == nil {
			abs = resolved
		}
		if _, exists := seen[abs]; exists {
			continue
		}
		seen[abs] = struct{}{}
		out = append(out, abs)
	}
	return out
}

// BuildSubprocessEnv constructs the environment for an external CLI subprocess.
// It never reads or injects third-party OAuth credential files; it only sets
// Myaaw agent identity, agent venv PATH, optional explicit API key, and local
// MCP bridge connection metadata.
func BuildSubprocessEnv(cfg config.RuntimeConfig, bridgeURL, bridgeToken string) []string {
	env := os.Environ()
	set := func(k, v string) {
		prefix := k + "="
		for i, entry := range env {
			if strings.HasPrefix(entry, prefix) {
				env[i] = prefix + v
				return
			}
		}
		env = append(env, prefix+v)
	}

	if active := agentdir.Active(); active != "" {
		set("MYAAW_AGENT", active)
	}
	if root, err := agentdir.ActiveRoot(); err == nil && root != "" {
		set("MYAAW_AGENT_ROOT", root)
	}
	// Ensure user CLI and package-manager bin dirs are visible even when running
	// inside a launchd/systemd daemon with minimal PATH, and prepend the agent's
	// local Python venv (if present) so SKILL.md scripts resolve cleanly.
	basePath := EnrichedPATH()
	if home, err := agentdir.ActiveHome(); err == nil && home != "" {
		venvDir := filepath.Join(home, ".venv")
		venvBin := filepath.Join(venvDir, "bin")
		if info, err := os.Stat(venvBin); err == nil && info.IsDir() {
			set("VIRTUAL_ENV", venvDir)
			if basePath != "" {
				basePath = venvBin + string(os.PathListSeparator) + basePath
			} else {
				basePath = venvBin
			}
		}
	}
	if basePath != "" {
		set("PATH", basePath)
	}
	if cfg.AuthSource == config.AuthSourceAPIKey && cfg.APIKey != "" && cfg.APIKeyEnv != "" {
		set(cfg.APIKeyEnv, cfg.APIKey)
	}
	if bridgeURL != "" {
		set("MYAAW_BRIDGE_URL", bridgeURL)
	}
	if bridgeToken != "" {
		set("MYAAW_BRIDGE_TOKEN", bridgeToken)
	}
	return env
}

// BuildTurnInput splits a Myaaw transcript into system prompt, user turn text
// (delta-only when resuming an existing external session, or seeded with a
// compact prior conversation block on first turn / rewind / runtime switch),
// and inline base64 images from the latest user turn.
func BuildTurnInput(history []provider.Message, resuming bool, rewind bool) (systemPrompt string, promptText string, images []string) {
	var systemParts []string
	var conv []provider.Message
	for _, msg := range history {
		if msg.Role == "system" {
			if text := strings.TrimSpace(provider.MessageText(msg)); text != "" {
				systemParts = append(systemParts, text)
			}
			continue
		}
		conv = append(conv, msg)
	}
	systemPrompt = strings.Join(systemParts, "\n\n")

	// Find the trailing contiguous user message(s) that belong to this turn.
	turnStart := len(conv)
	for i := len(conv) - 1; i >= 0; i-- {
		if conv[i].Role == "user" {
			turnStart = i
			continue
		}
		break
	}

	var latestParts []string
	for i := turnStart; i < len(conv); i++ {
		if text := strings.TrimSpace(provider.MessageText(conv[i])); text != "" {
			latestParts = append(latestParts, text)
		}
		images = append(images, extractMessageImages(conv[i])...)
	}
	latestText := strings.Join(latestParts, "\n\n")

	if resuming && !rewind {
		return systemPrompt, latestText, images
	}

	// Seed prior conversation history when starting a fresh external session on
	// an existing Myaaw conversation (e.g. after rewind, runtime switch, or
	// expired external session).
	prior := conv[:turnStart]
	if len(prior) == 0 {
		return systemPrompt, latestText, images
	}

	var sb strings.Builder
	sb.WriteString("<previous_conversation>\n")
	for _, msg := range prior {
		switch msg.Role {
		case "user":
			text := strings.TrimSpace(provider.StripContextBlocks(provider.MessageText(msg)))
			if text != "" {
				sb.WriteString("User: ")
				sb.WriteString(truncateSeed(text, 2000))
				sb.WriteString("\n")
			}
		case "assistant":
			text := strings.TrimSpace(provider.MessageText(msg))
			if text != "" {
				sb.WriteString("Assistant: ")
				sb.WriteString(truncateSeed(text, 2000))
				sb.WriteString("\n")
			}
		}
	}
	sb.WriteString("</previous_conversation>\n\n")
	sb.WriteString(latestText)
	return systemPrompt, strings.TrimSpace(sb.String()), images
}

func extractMessageImages(msg provider.Message) []string {
	var out []string
	if len(msg.Images) > 0 {
		out = append(out, msg.Images...)
	}
	if items, ok := msg.Content.([]provider.ContentItem); ok {
		for _, item := range items {
			if item.Type == "image_url" && item.ImageURL != nil && strings.HasPrefix(item.ImageURL.URL, "data:") {
				if _, b64, found := strings.Cut(item.ImageURL.URL, ","); found && b64 != "" {
					out = append(out, b64)
				}
			}
		}
	}
	return out
}

func truncateSeed(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			if p == "~" {
				return home
			}
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

func runtimeDisplayProvider(cfg config.RuntimeConfig) string {
	if cfg.Preset != "" && cfg.Preset != cfg.Type {
		return cfg.Type + ":" + cfg.Preset
	}
	return cfg.Type
}

func runtimeDisplayModel(cfg config.RuntimeConfig) string {
	if m := strings.TrimSpace(cfg.Model); m != "" {
		return m
	}
	return "default"
}

func marshalArgsJSON(v any) string {
	if v == nil {
		return "{}"
	}
	if s, ok := v.(string); ok {
		if strings.TrimSpace(s) == "" {
			return "{}"
		}
		return s
	}
	raw, err := json.Marshal(v)
	if err != nil || len(raw) == 0 {
		return "{}"
	}
	return string(raw)
}
