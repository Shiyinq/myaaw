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
	"sync"
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

type cachedDiscover struct {
	fetchedAt time.Time
	items     []DetectedRuntime
}

var (
	discoverCacheMu  sync.Mutex
	discoverCache    cachedDiscover
	discoverTTL      = 5 * time.Minute
	discoverInflight chan struct{}

	extraDirsMu      sync.Mutex
	extraDirsKey     string
	extraDirsCached  []string
	extraDirsFetched time.Time
	extraDirsTTL     = 30 * time.Second
)

// InvalidateExtraSearchDirsCache clears the cached extra PATH bin directory
// list so newly created installer directories (e.g. ~/.opencode/bin) are found
// immediately without waiting for the 30s directory scan TTL.
func InvalidateExtraSearchDirsCache() {
	extraDirsMu.Lock()
	extraDirsCached = nil
	extraDirsFetched = time.Time{}
	extraDirsMu.Unlock()
}

// InvalidateDiscoverCache drops cached host CLI binary discovery so the next
// query re-probes binary paths and versions live.
func InvalidateDiscoverCache() {
	discoverCacheMu.Lock()
	discoverCache = cachedDiscover{}
	discoverCacheMu.Unlock()
	InvalidateExtraSearchDirsCache()
}

// DiscoverCached returns cached host binary discovery unless refresh is true
// or the cache TTL has expired. Concurrent callers coalesce onto one in-flight
// probe so parallel requests (e.g. /runtimes + /runtimes/manage) never spawn
// duplicate CLI version checks.
func DiscoverCached(ctx context.Context, refresh bool) []DetectedRuntime {
	if refresh {
		InvalidateExtraSearchDirsCache()
	}
	for {
		discoverCacheMu.Lock()
		if !refresh && len(discoverCache.items) > 0 && time.Since(discoverCache.fetchedAt) < discoverTTL {
			out := cloneDetectedRuntimes(discoverCache.items)
			discoverCacheMu.Unlock()
			return out
		}
		if waitCh := discoverInflight; waitCh != nil {
			discoverCacheMu.Unlock()
			select {
			case <-waitCh:
				refresh = false
				continue
			case <-ctx.Done():
				return Discover(ctx)
			}
		}
		ch := make(chan struct{})
		discoverInflight = ch
		discoverCacheMu.Unlock()

		items := Discover(ctx)
		discoverCacheMu.Lock()
		discoverCache = cachedDiscover{
			fetchedAt: time.Now(),
			items:     cloneDetectedRuntimes(items),
		}
		discoverInflight = nil
		close(ch)
		discoverCacheMu.Unlock()
		return items
	}
}

// DiscoverOne probes a single runtime preset live and updates the cached entry
// in place when a cache snapshot is present.
func DiscoverOne(ctx context.Context, preset config.RuntimePreset) DetectedRuntime {
	InvalidateExtraSearchDirsCache()
	entry := DetectedRuntime{RuntimePreset: preset}
	if preset.Type == config.RuntimeTypeMyaaw {
		entry.Installed = true
		entry.Version = "built-in"
		return entry
	}
	if preset.DefaultCommand != "" {
		if path, err := ResolveBinary(preset.DefaultCommand); err == nil {
			entry.Installed = true
			entry.BinaryPath = path
			entry.Version = ProbeBinaryVersion(ctx, path)
		}
	}
	discoverCacheMu.Lock()
	for i := range discoverCache.items {
		if discoverCache.items[i].ID == preset.ID {
			discoverCache.items[i] = entry
			break
		}
	}
	discoverCacheMu.Unlock()
	return entry
}

func cloneDetectedRuntimes(in []DetectedRuntime) []DetectedRuntime {
	out := make([]DetectedRuntime, len(in))
	for i, item := range in {
		if len(item.DefaultArgs) > 0 {
			item.DefaultArgs = append([]string(nil), item.DefaultArgs...)
		}
		out[i] = item
	}
	return out
}

// Discover probes the host PATH concurrently for every built-in runtime preset.
func Discover(ctx context.Context) []DetectedRuntime {
	presets := config.RuntimePresets()
	out := make([]DetectedRuntime, len(presets))
	var wg sync.WaitGroup
	for i, preset := range presets {
		idx := i
		p := preset
		if p.Type == config.RuntimeTypeMyaaw {
			out[idx] = DetectedRuntime{
				RuntimePreset: p,
				Installed:     true,
				Version:       "built-in",
			}
			continue
		}
		if p.DefaultCommand == "" {
			out[idx] = DetectedRuntime{RuntimePreset: p}
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			entry := DetectedRuntime{RuntimePreset: p}
			if path, err := ResolveBinary(p.DefaultCommand); err == nil {
				entry.Installed = true
				entry.BinaryPath = path
				entry.Version = ProbeBinaryVersion(ctx, path)
			}
			out[idx] = entry
		}()
	}
	wg.Wait()
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
	home, _ := os.UserHomeDir()
	localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	cacheKey := home + "\x00" + localAppData

	extraDirsMu.Lock()
	if extraDirsKey == cacheKey && !extraDirsFetched.IsZero() && time.Since(extraDirsFetched) < extraDirsTTL {
		cached := append([]string(nil), extraDirsCached...)
		extraDirsMu.Unlock()
		return cached
	}
	extraDirsMu.Unlock()

	var dirs []string
	if home != "" {
		staticUserDirs := []string{
			filepath.Join(home, ".local", "bin"),
			filepath.Join(home, ".gemini", "antigravity", "bin"),
			filepath.Join(home, ".antigravity", "antigravity", "bin"),
			filepath.Join(home, ".antigravity-ide", "antigravity-ide", "bin"),
			filepath.Join(home, ".claude", "bin"),
			filepath.Join(home, ".claude", "local"),
			filepath.Join(home, ".opencode", "bin"),
			filepath.Join(home, ".omp", "bin"),
			filepath.Join(home, ".pi", "bin"),
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
	// Bundled CLI inside DeepSeek Harness Desktop installations.
	var dshDesktopDirs []string
	switch goruntime.GOOS {
	case "darwin":
		dshDesktopDirs = append(dshDesktopDirs, "/Applications/DeepSeek Harness.app/Contents/Resources/runtime/cli/bin")
		if home != "" {
			dshDesktopDirs = append(dshDesktopDirs, filepath.Join(home, "Applications", "DeepSeek Harness.app", "Contents/Resources/runtime/cli/bin"))
		}
	case "windows":
		if localAppData != "" {
			dshDesktopDirs = append(dshDesktopDirs, filepath.Join(localAppData, "Programs", "DeepSeek Harness", "resources", "runtime", "cli", "bin"))
		}
	default:
		dshDesktopDirs = append(dshDesktopDirs, "/opt/DeepSeek Harness/resources/runtime/cli/bin")
	}
	for _, d := range dshDesktopDirs {
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			dirs = append(dirs, d)
		}
	}

	extraDirsMu.Lock()
	extraDirsKey = cacheKey
	extraDirsCached = append([]string(nil), dirs...)
	extraDirsFetched = time.Now()
	extraDirsMu.Unlock()
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

// ProbeBinaryVersion runs `<binary> --version` with a short bounded timeout
// and returns the first trimmed output line.
func ProbeBinaryVersion(parent context.Context, binary string) string {
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

// EffectiveWorkingDir returns the per-conversation working directory override
// when set and existing, falling back to the runtime's default working dir.
func EffectiveWorkingDir(runCfg agent.RunConfig, fallback string) string {
	if wd := strings.TrimSpace(runCfg.WorkingDir); wd != "" {
		expanded := expandHome(wd)
		if abs, err := filepath.Abs(expanded); err == nil {
			expanded = abs
		}
		if info, err := os.Stat(expanded); err == nil && info.IsDir() {
			return filepath.Clean(expanded)
		}
	}
	return fallback
}

// AllowedDirs returns the deduplicated list of host directories an external
// runtime should be permitted to access (working dir + agent home/skills/user +
// any user-configured additional_dirs).
func AllowedDirs(cfg config.RuntimeConfig) []string {
	return AllowedDirsWithExtra(cfg, "")
}

// AllowedDirsWithExtra is AllowedDirs plus an optional per-conversation working
// directory prepended to the allowlist.
func AllowedDirsWithExtra(cfg config.RuntimeConfig, extraDir string) []string {
	candidates := make([]string, 0, 8)
	if trimmed := strings.TrimSpace(extraDir); trimmed != "" {
		candidates = append(candidates, expandHome(trimmed))
	}
	candidates = append(candidates, ResolveWorkingDir(cfg))
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
	applyTelemetryOptOutEnv(set, cfg)
	return env
}

// applyTelemetryOptOutEnv injects standard and per-CLI telemetry opt-out
// environment variables so external agent subprocesses never ship usage
// analytics or OTel traces out of Myaaw.
func applyTelemetryOptOutEnv(set func(k, v string), cfg config.RuntimeConfig) {
	// Universal privacy / OpenTelemetry opt-out signals.
	set("DO_NOT_TRACK", "1")
	set("OTEL_SDK_DISABLED", "true")
	set("OTEL_TRACES_EXPORTER", "none")
	set("OTEL_METRICS_EXPORTER", "none")
	set("OTEL_LOGS_EXPORTER", "none")

	preset := strings.ToLower(strings.TrimSpace(cfg.Preset))
	rtType := strings.ToLower(strings.TrimSpace(cfg.Type))
	cmdBase := strings.ToLower(strings.TrimSuffix(filepath.Base(strings.TrimSpace(cfg.Command)), ".exe"))

	switch {
	case rtType == config.RuntimeTypeClaudeCode || preset == "claude_code" || cmdBase == "claude":
		set("DISABLE_TELEMETRY", "1")
		set("DISABLE_ERROR_REPORTING", "1")
		set("CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", "1")
	case preset == "deepseek_harness" || cmdBase == "dsh":
		set("DSH_TELEMETRY_DISABLED", "1")
		set("DSH_TELEMETRY_MODE", "DISABLED")
	case preset == "antigravity" || cmdBase == "agy" || cmdBase == "gemini":
		set("GEMINI_TELEMETRY_ENABLED", "false")
	case preset == "opencode" || cmdBase == "opencode":
		set("OPENCODE_DISABLE_AUTOUPDATE", "true")
	case rtType == config.RuntimeTypePi || rtType == config.RuntimeTypeOMP || preset == "pi" || preset == "omp" || cmdBase == "pi" || cmdBase == "omp":
		set("PI_TELEMETRY", "false")
	}
}

// acpSubprocessArgs builds the argv for an ACP CLI subprocess and ensures
// Antigravity (`agy acp`) always receives `--disable-telemetry`, including for
// existing saved agents whose config only stored `["acp"]`.
func acpSubprocessArgs(cfg config.RuntimeConfig) []string {
	args := append([]string(nil), cfg.Args...)
	preset := strings.ToLower(strings.TrimSpace(cfg.Preset))
	cmdBase := strings.ToLower(strings.TrimSuffix(filepath.Base(strings.TrimSpace(cfg.Command)), ".exe"))
	if preset == "antigravity" || cmdBase == "agy" {
		hasACP := false
		hasDisableTelemetry := false
		for _, a := range args {
			trimmed := strings.TrimSpace(a)
			if trimmed == "acp" {
				hasACP = true
			}
			if trimmed == "--disable-telemetry" || trimmed == "--disable_telemetry" ||
				strings.HasPrefix(trimmed, "--disable-telemetry=") || strings.HasPrefix(trimmed, "--disable_telemetry=") {
				hasDisableTelemetry = true
			}
		}
		if hasACP && !hasDisableTelemetry {
			args = append(args, "--disable-telemetry")
		}
	}
	return args
}

// codexSubprocessArgs builds the argv for `codex app-server`, disabling Codex
// analytics (`-c analytics.enabled=false`) and optionally injecting reasoning
// effort/summary overrides for turns.
func codexSubprocessArgs(cfg config.RuntimeConfig, includeReasoning bool) []string {
	args := append([]string(nil), cfg.Args...)
	if len(args) == 0 {
		args = []string{"app-server"}
	}
	hasAnalyticsOverride := false
	for _, a := range args {
		if strings.Contains(a, "analytics.enabled") {
			hasAnalyticsOverride = true
			break
		}
	}
	if !hasAnalyticsOverride {
		args = append(args, "-c", "analytics.enabled=false")
	}
	if includeReasoning {
		effort, summary := codexReasoningParams(cfg.ThinkingLevel)
		if effort != "" {
			args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%s", marshalTOMLString(effort)))
		}
		if summary != "" {
			args = append(args, "-c", fmt.Sprintf("model_reasoning_summary=%s", marshalTOMLString(summary)))
		}
	}
	return args
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
