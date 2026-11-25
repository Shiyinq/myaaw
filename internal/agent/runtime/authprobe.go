package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"myaaw/internal/config"
)

// Auth readiness states reported for external CLI runtimes.
const (
	AuthStatusAuthenticated    = "authenticated"
	AuthStatusFreeReady        = "free_tier"
	AuthStatusUnauthenticated  = "unauthenticated"
	AuthStatusAPIKeyConfigured = "api_key_configured"
	AuthStatusUnknown          = "unknown"
	AuthStatusNotApplicable    = "not_applicable"
)

// ErrUnsupportedAuthAction is returned when an unknown runtime or auth action
// is requested for a scoped interactive PTY session.
var ErrUnsupportedAuthAction = errors.New("unsupported runtime authentication action")

// AuthStatusInfo summarizes non-intrusive authentication readiness and
// supported official CLI login/logout actions for one external runtime.
type AuthStatusInfo struct {
	Status           string `json:"status"`
	Detail           string `json:"detail,omitempty"`
	SupportsLogin    bool   `json:"supports_login"`
	SupportsLogout   bool   `json:"supports_logout"`
	SupportsHeadless bool   `json:"supports_headless"`
	LoginCommand     string `json:"login_command,omitempty"`
	LogoutCommand    string `json:"logout_command,omitempty"`
}

type cachedAuthStatus struct {
	key       string
	fetchedAt time.Time
	value     AuthStatusInfo
}

type authProbeInflight struct {
	done  chan struct{}
	value AuthStatusInfo
}

var (
	authStatusCacheMu  sync.Mutex
	authStatusCache    = make(map[string]cachedAuthStatus)
	authStatusInflight = make(map[string]*authProbeInflight)
	authStatusTTL      = 5 * time.Minute
	authUnauthTTL      = 15 * time.Second
)

func authCacheEntryFresh(entry cachedAuthStatus) bool {
	ttl := authStatusTTL
	if entry.value.Status == AuthStatusUnauthenticated {
		ttl = authUnauthTTL
	}
	return time.Since(entry.fetchedAt) < ttl
}

// InvalidateAuthStatusCache drops cached CLI authentication status and any
// paired runtime option discovery for presetID (or all presets when empty) so
// the next query re-probes live.
func InvalidateAuthStatusCache(presetID string) {
	id := strings.ToLower(strings.TrimSpace(presetID))
	authStatusCacheMu.Lock()
	if id == "" {
		authStatusCache = make(map[string]cachedAuthStatus)
	} else {
		delete(authStatusCache, id)
	}
	authStatusCacheMu.Unlock()
	InvalidateRuntimeOptionsCache(id)
}

// ProbeAuthStatusCached returns cached CLI auth readiness unless refresh is true
// or the TTL expired. Concurrent callers for the same cache key coalesce onto a
// single in-flight subprocess probe instead of spawning duplicate CLI processes.
func ProbeAuthStatusCached(
	parent context.Context,
	preset config.RuntimePreset,
	resolvedBin string,
	apiKeyEnv string,
	hasSavedGlobalKey bool,
	refresh bool,
) AuthStatusInfo {
	cacheKey := fmt.Sprintf("%s|%s|%s|%t", preset.ID, resolvedBin, apiKeyEnv, hasSavedGlobalKey)
	if refresh {
		InvalidateRuntimeOptionsCache(preset.ID)
	}

	authStatusCacheMu.Lock()
	if !refresh {
		if entry, ok := authStatusCache[preset.ID]; ok && entry.key == cacheKey && authCacheEntryFresh(entry) {
			val := entry.value
			authStatusCacheMu.Unlock()
			return val
		}
	}
	if inflight, ok := authStatusInflight[cacheKey]; ok {
		authStatusCacheMu.Unlock()
		select {
		case <-inflight.done:
			return inflight.value
		case <-parent.Done():
			return AuthActionMetadata(preset.ID)
		}
	}
	inflight := &authProbeInflight{done: make(chan struct{})}
	authStatusInflight[cacheKey] = inflight
	authStatusCacheMu.Unlock()

	res := ProbeAuthStatus(parent, preset, resolvedBin, apiKeyEnv, hasSavedGlobalKey)
	authStatusCacheMu.Lock()
	if res.Status != AuthStatusUnknown {
		authStatusCache[preset.ID] = cachedAuthStatus{
			key:       cacheKey,
			fetchedAt: time.Now(),
			value:     res,
		}
	}
	inflight.value = res
	delete(authStatusInflight, cacheKey)
	close(inflight.done)
	authStatusCacheMu.Unlock()
	return res
}

// AuthActionMetadata returns the static CLI auth capabilities and command
// previews for presetID without running any subprocess.
func AuthActionMetadata(presetID string) AuthStatusInfo {
	id := strings.ToLower(strings.TrimSpace(presetID))
	switch id {
	case "antigravity":
		return AuthStatusInfo{
			Status:         AuthStatusUnknown,
			SupportsLogin:  true,
			SupportsLogout: false,
			LoginCommand:   "agy",
		}
	case "claude_code":
		return AuthStatusInfo{
			Status:           AuthStatusUnknown,
			SupportsLogin:    true,
			SupportsLogout:   true,
			SupportsHeadless: true,
			LoginCommand:     "claude auth login",
			LogoutCommand:    "claude auth logout",
		}
	case "codex":
		return AuthStatusInfo{
			Status:           AuthStatusUnknown,
			SupportsLogin:    true,
			SupportsLogout:   true,
			SupportsHeadless: true,
			LoginCommand:     "codex login",
			LogoutCommand:    "codex logout",
		}
	case "opencode":
		return AuthStatusInfo{
			Status:         AuthStatusUnknown,
			SupportsLogin:  true,
			SupportsLogout: true,
			LoginCommand:   "opencode auth login",
			LogoutCommand:  "opencode auth logout",
		}
	case "deepseek_harness":
		return AuthStatusInfo{
			Status:         AuthStatusUnknown,
			SupportsLogin:  false,
			SupportsLogout: false,
		}
	case "omp":
		return AuthStatusInfo{
			Status:        AuthStatusUnknown,
			SupportsLogin: true,
			LoginCommand:  "omp login",
		}
	case "pi":
		return AuthStatusInfo{
			Status:        AuthStatusUnknown,
			SupportsLogin: true,
			LoginCommand:  "pi",
		}
	default:
		return AuthStatusInfo{
			Status: AuthStatusNotApplicable,
		}
	}
}

// AuthCommandArgs returns the compiled-in allowlisted argument vector and
// human-readable preview for launching an interactive official CLI login or
// logout session inside a scoped PTY. Never accepts user-supplied args.
func AuthCommandArgs(presetID, mode string, headless bool) (defaultBin string, args []string, preview string, err error) {
	id := strings.ToLower(strings.TrimSpace(presetID))
	action := strings.ToLower(strings.TrimSpace(mode))
	if action == "" {
		action = "login"
	}

	switch id {
	case "antigravity":
		if action == "login" {
			return "agy", nil, "agy", nil
		}
	case "claude_code":
		if action == "login" {
			if headless {
				return "claude", []string{"auth", "login", "--no-browser"}, "claude auth login --no-browser", nil
			}
			return "claude", []string{"auth", "login"}, "claude auth login", nil
		}
		if action == "logout" {
			return "claude", []string{"auth", "logout"}, "claude auth logout", nil
		}
	case "codex":
		if action == "login" {
			if headless {
				return "codex", []string{"login", "--device-auth"}, "codex login --device-auth", nil
			}
			return "codex", []string{"login"}, "codex login", nil
		}
		if action == "logout" {
			return "codex", []string{"logout"}, "codex logout", nil
		}
	case "opencode":
		if action == "login" {
			return "opencode", []string{"auth", "login"}, "opencode auth login", nil
		}
		if action == "logout" {
			return "opencode", []string{"auth", "logout"}, "opencode auth logout", nil
		}
	case "omp":
		if action == "login" {
			return "omp", []string{"login"}, "omp login", nil
		}
	case "pi":
		if action == "login" {
			return "pi", nil, "pi", nil
		}
	}
	return "", nil, "", fmt.Errorf("%w: %s (%s)", ErrUnsupportedAuthAction, presetID, mode)
}

// ProbeAuthStatus checks whether an installed external runtime CLI is signed in,
// ready via built-in free models, or has an API key configured, strictly using
// official non-interactive CLI subcommands with closed stdin and a bounded
// timeout. It never reads or touches private credential files or OS keychains.
func ProbeAuthStatus(
	parent context.Context,
	preset config.RuntimePreset,
	resolvedBin string,
	apiKeyEnv string,
	hasSavedGlobalKey bool,
) AuthStatusInfo {
	info := AuthActionMetadata(preset.ID)
	if preset.ID == "myaaw" || preset.ID == "custom_acp" {
		if hasSavedGlobalKey {
			info.Status = AuthStatusAPIKeyConfigured
			info.Detail = "Global API key saved"
		}
		return info
	}
	if strings.TrimSpace(resolvedBin) == "" {
		info.Status = AuthStatusUnauthenticated
		return info
	}

	envKeyName := strings.TrimSpace(apiKeyEnv)
	if envKeyName == "" {
		envKeyName = strings.TrimSpace(preset.DefaultAPIKeyEnv)
	}
	hostHasEnvKey := envKeyName != "" && strings.TrimSpace(os.Getenv(envKeyName)) != ""

	// Antigravity (`agy models`) cold-starts its local language server (~7-10s)
	// on the first invocation, so allow 15s unless the caller set a shorter deadline.
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()

	switch preset.ID {
	case "claude_code":
		out, exitCode, err := runNonInteractiveProbe(ctx, resolvedBin, "auth", "status")
		if err == nil && exitCode == 0 {
			info.Status = AuthStatusAuthenticated
			info.Detail = parseClaudeAuthSummary(out)
			return info
		}
		if hasSavedGlobalKey || hostHasEnvKey {
			info.Status = AuthStatusAPIKeyConfigured
			info.Detail = envKeyName
			return info
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			info.Status = AuthStatusUnknown
			return info
		}
		if exitCode == 1 || strings.Contains(strings.ToLower(out), "not logged in") || strings.Contains(strings.ToLower(out), "unauthenticated") {
			info.Status = AuthStatusUnauthenticated
			return info
		}

	case "codex":
		// Run `codex login status` before `codex app-server` so two codex processes
		// never race on ~/.codex PATH alias creation or state locks.
		out, exitCode, err := runNonInteractiveProbe(ctx, resolvedBin, "login", "status")
		lower := strings.ToLower(out)
		if err == nil && exitCode == 0 && !strings.Contains(lower, "not logged in") {
			info.Status = AuthStatusAuthenticated
			summary := parseCodexAuthSummary(out)
			probeOpts := DiscoverRuntimeOptionsCached(ctx, "authprobe:"+preset.ID, config.RuntimeConfig{
				Type:    preset.Type,
				Preset:  preset.ID,
				Command: resolvedBin,
				Args:    preset.DefaultArgs,
			}, os.TempDir(), false)
			if n := len(probeOpts.Models.Options); n > 0 {
				if summary != "" {
					info.Detail = fmt.Sprintf("%s · %d models", summary, n)
				} else {
					info.Detail = fmt.Sprintf("%d models", n)
				}
			} else {
				info.Detail = summary
			}
			return info
		}
		if hasSavedGlobalKey || hostHasEnvKey {
			info.Status = AuthStatusAPIKeyConfigured
			info.Detail = envKeyName
			return info
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			info.Status = AuthStatusUnknown
			return info
		}
		info.Status = AuthStatusUnauthenticated
		return info

	case "antigravity":
		out, exitCode, err := runNonInteractiveProbe(ctx, resolvedBin, "models")
		lower := strings.ToLower(out)
		modelLines := countNonSpinnerLines(out)
		if err == nil && exitCode == 0 && modelLines > 0 &&
			!strings.Contains(lower, "not logged in") &&
			!strings.Contains(lower, "sign in") &&
			!strings.Contains(lower, "authenticate") {
			info.Status = AuthStatusAuthenticated
			info.Detail = fmt.Sprintf("Google Account · %d models", modelLines)
			return info
		}
		if hasSavedGlobalKey || hostHasEnvKey {
			info.Status = AuthStatusAPIKeyConfigured
			info.Detail = envKeyName
			return info
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			info.Status = AuthStatusUnknown
			return info
		}
		info.Status = AuthStatusUnauthenticated
		return info

	case "opencode":
		// Run `opencode auth list` then `opencode models` sequentially so two
		// opencode processes never contend on OpenCode's local state/cache DB.
		out, exitCode, err := runNonInteractiveProbe(ctx, resolvedBin, "auth", "list")
		if err == nil && exitCode == 0 {
			count := countOpenCodeProviders(out)
			if count > 0 {
				info.Status = AuthStatusAuthenticated
				info.Detail = fmt.Sprintf("%d provider(s)", count)
				return info
			}
		}
		if hasSavedGlobalKey || hostHasEnvKey {
			info.Status = AuthStatusAPIKeyConfigured
			info.Detail = envKeyName
			return info
		}
		// OpenCode ships built-in OpenCode Zen free models (opencode/*-free) that
		// work immediately without logging into any paid provider.
		modelsOut, mCode, mErr := runNonInteractiveProbe(ctx, resolvedBin, "models")
		if mErr == nil && mCode == 0 {
			freeCount := countNonSpinnerLines(modelsOut)
			if freeCount > 0 {
				info.Status = AuthStatusFreeReady
				info.Detail = fmt.Sprintf("%d model", freeCount)
				return info
			}
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(mErr, context.DeadlineExceeded) || mErr != nil {
			info.Status = AuthStatusUnknown
			return info
		}
		info.Status = AuthStatusUnauthenticated
		return info

	case "deepseek_harness":
		probeOpts := DiscoverRuntimeOptionsCached(ctx, "authprobe:"+preset.ID, config.RuntimeConfig{
			Type:    preset.Type,
			Preset:  preset.ID,
			Command: resolvedBin,
			Args:    preset.DefaultArgs,
		}, os.TempDir(), false)
		hasAccountModel := false
		for _, m := range probeOpts.Models.Options {
			if strings.Contains(strings.ToLower(m.Value), "deepseek-account") || strings.EqualFold(m.Group, "DeepSeek Account") {
				hasAccountModel = true
				break
			}
		}
		if hasAccountModel {
			info.Status = AuthStatusAuthenticated
			info.Detail = fmt.Sprintf("DeepSeek Account · %d models", len(probeOpts.Models.Options))
			return info
		}
		if hasSavedGlobalKey || hostHasEnvKey {
			info.Status = AuthStatusAPIKeyConfigured
			if n := len(probeOpts.Models.Options); n > 0 {
				info.Detail = fmt.Sprintf("%s · %d models", envKeyName, n)
			} else {
				info.Detail = envKeyName
			}
			return info
		}
		info.Status = AuthStatusUnauthenticated
		return info

	case "omp", "pi":
		if hasSavedGlobalKey || hostHasEnvKey {
			info.Status = AuthStatusAPIKeyConfigured
			info.Detail = envKeyName
			return info
		}
		probeOpts := DiscoverRuntimeOptionsCached(ctx, "authprobe:"+preset.ID, config.RuntimeConfig{
			Type:    preset.Type,
			Preset:  preset.ID,
			Command: resolvedBin,
			Args:    preset.DefaultArgs,
		}, os.TempDir(), false)
		if len(probeOpts.Models.Options) > 0 {
			info.Status = AuthStatusAuthenticated
			info.Detail = fmt.Sprintf("%d models ready", len(probeOpts.Models.Options))
			return info
		}
		info.Status = AuthStatusUnauthenticated
		return info
	}

	if hasSavedGlobalKey || hostHasEnvKey {
		info.Status = AuthStatusAPIKeyConfigured
		info.Detail = envKeyName
	}
	return info
}

func runNonInteractiveProbe(ctx context.Context, bin string, args ...string) (string, int, error) {
	cmd := exec.Command(bin, args...)
	configureProcessGroup(cmd)
	cmd.Stdin = nil // Immediate EOF if the CLI attempts to read stdin.
	cmd.Dir = os.TempDir()
	cmd.Env = setEnvVar(append([]string(nil), os.Environ()...), "PATH", EnrichedPATH())

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Start(); err != nil {
		return "", -1, err
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			terminateProcessGroup(cmd)
		case <-done:
		}
	}()
	waitErr := cmd.Wait()
	close(done)

	out := stripANSI(buf.String())
	if ctx.Err() != nil {
		return out, -1, ctx.Err()
	}
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			return out, exitErr.ExitCode(), waitErr
		}
		return out, -1, waitErr
	}
	return out, 0, nil
}

func countNonSpinnerLines(raw string) int {
	// Normalize carriage-return spinner frames (e.g. "\r⠋ Fetching available models...")
	// so only real output lines are counted.
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	count := 0
	for _, line := range strings.Split(normalized, "\n") {
		if idx := strings.LastIndexByte(line, '\r'); idx >= 0 {
			line = line[idx+1:]
		}
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		lower := strings.ToLower(t)
		if strings.Contains(lower, "fetching available models") || strings.HasPrefix(lower, "loading") {
			continue
		}
		count++
	}
	return count
}

func parseCodexAuthSummary(raw string) string {
	line := firstCleanLine(raw)
	if line == "" {
		return ""
	}
	lower := strings.ToLower(line)
	for _, prefix := range []string{"logged in using ", "logged in with ", "logged in as "} {
		if strings.HasPrefix(lower, prefix) {
			trimmed := strings.TrimSpace(line[len(prefix):])
			if trimmed != "" {
				return trimmed
			}
		}
	}
	return line
}

func parseClaudeAuthSummary(raw string) string {
	var parsed struct {
		LoggedIn    *bool  `json:"loggedIn"`
		AuthMethod  string `json:"authMethod"`
		APIProvider string `json:"apiProvider"`
	}
	trimmed := strings.TrimSpace(raw)
	if json.Unmarshal([]byte(trimmed), &parsed) == nil {
		if parsed.AuthMethod != "" {
			return parsed.AuthMethod
		}
		if parsed.APIProvider != "" {
			return parsed.APIProvider
		}
	}
	return firstCleanLine(raw)
}

func countOpenCodeProviders(raw string) int {
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "0 credentials") || strings.Contains(lower, "no credentials") || strings.Contains(lower, "empty") {
		return 0
	}
	count := 0
	for _, line := range strings.Split(raw, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		lt := strings.ToLower(t)
		if strings.HasPrefix(lt, "┌") || strings.HasPrefix(lt, "└") || strings.HasPrefix(lt, "│") || strings.HasPrefix(lt, "credentials") {
			if strings.Contains(lt, "oauth") || strings.Contains(lt, "api") {
				count++
			}
			continue
		}
		if strings.Contains(lt, "oauth") || strings.Contains(lt, "api key") || strings.Contains(lt, "api_key") || strings.HasPrefix(t, "●") || strings.HasPrefix(t, "•") || strings.HasPrefix(t, "-") {
			count++
		}
	}
	return count
}

func firstCleanLine(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		if idx := strings.LastIndexByte(line, '\r'); idx >= 0 {
			line = line[idx+1:]
		}
		t := strings.TrimSpace(line)
		if t != "" {
			if len(t) > 60 {
				return t[:60]
			}
			return t
		}
	}
	return ""
}

func stripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inEsc := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == 0x1b {
			inEsc = true
			continue
		}
		if inEsc {
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
				inEsc = false
			}
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}
