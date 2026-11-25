package runtime

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"

	"myaaw/internal/config"
)

var (
	// ErrUnsupportedRuntimePreset is returned when an unknown or non-installable
	// preset (such as "myaaw" or "custom_acp") is passed to the official installer.
	ErrUnsupportedRuntimePreset = errors.New("unsupported runtime preset")
	// ErrUnsupportedInstallMethod is returned when the requested method ID is not
	// in the compiled-in allowlist for that preset and OS.
	ErrUnsupportedInstallMethod = errors.New("unsupported install method")
	// ErrInstallToolMissing is returned when the prerequisite tool (e.g. brew, npm,
	// bun, curl) is not installed on the host.
	ErrInstallToolMissing = errors.New("required installer tool not found on host")
)

// InstallMethod describes one compiled-in official vendor installation or
// update method for an external runtime CLI.
type InstallMethod struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	CommandPreview string `json:"command_preview"`
	SourceURL      string `json:"source_url"`
	Available      bool   `json:"available"`
	MissingTool    string `json:"missing_tool,omitempty"`
	Recommended    bool   `json:"recommended"`
}

type officialRecipe struct {
	id          string
	label       string
	preview     string
	sourceURL   string
	osList      []string // empty means all OSes
	prereqTools []string // every tool must resolve via ResolveBinary
	execArgs    func(resolvedBin string) (bin string, args []string, usesNPM bool, err error)
	recommended bool
}

// OfficialDocsURL returns the official vendor documentation URL for a preset.
func OfficialDocsURL(presetID string) string {
	switch strings.ToLower(strings.TrimSpace(presetID)) {
	case "antigravity":
		return "https://antigravity.google/docs/getting-started"
	case "claude_code":
		return "https://docs.anthropic.com/en/docs/claude-code"
	case "codex":
		return "https://github.com/openai/codex"
	case "opencode":
		return "https://opencode.ai/docs"
	case "omp":
		return "https://github.com/can1357/oh-my-pi"
	case "pi":
		return "https://pi.dev"
	default:
		return ""
	}
}

func curlBashExec(scriptURL, shellBin string) func(string) (string, []string, bool, error) {
	return func(_ string) (string, []string, bool, error) {
		shPath, err := ResolveBinary(shellBin)
		if err != nil {
			return "", nil, false, fmt.Errorf("%w: %s", ErrInstallToolMissing, shellBin)
		}
		if _, err := ResolveBinary("curl"); err != nil {
			return "", nil, false, fmt.Errorf("%w: curl", ErrInstallToolMissing)
		}
		// Fixed, compiled-in script URL — never interpolated from user input.
		stmt := fmt.Sprintf("set -eo pipefail; curl -fsSL %s | %s", scriptURL, shellBin)
		return shPath, []string{"-c", stmt}, false, nil
	}
}

func powershellExec(scriptURL string) func(string) (string, []string, bool, error) {
	return func(_ string) (string, []string, bool, error) {
		ps, err := ResolveBinary("powershell")
		if err != nil {
			ps, err = ResolveBinary("pwsh")
			if err != nil {
				return "", nil, false, fmt.Errorf("%w: powershell", ErrInstallToolMissing)
			}
		}
		stmt := fmt.Sprintf("irm %s | iex", scriptURL)
		return ps, []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", stmt}, false, nil
	}
}

func toolExec(tool string, fixedArgs ...string) func(string) (string, []string, bool, error) {
	return func(_ string) (string, []string, bool, error) {
		bin, err := ResolveBinary(tool)
		if err != nil {
			return "", nil, false, fmt.Errorf("%w: %s", ErrInstallToolMissing, tool)
		}
		return bin, append([]string(nil), fixedArgs...), tool == "npm", nil
	}
}

func selfSubcommandExec(defaultCmd string, subArgs ...string) func(string) (string, []string, bool, error) {
	return func(resolvedBin string) (string, []string, bool, error) {
		bin := strings.TrimSpace(resolvedBin)
		if bin == "" {
			var err error
			bin, err = ResolveBinary(defaultCmd)
			if err != nil {
				return "", nil, false, fmt.Errorf("%w: %s", ErrInstallToolMissing, defaultCmd)
			}
		}
		return bin, append([]string(nil), subArgs...), false, nil
	}
}

// installCatalog holds the strictly compiled-in allowlist of official vendor
// installation recipes. No user-supplied strings are ever passed here.
var installCatalog = map[string][]officialRecipe{
	"antigravity": {
		{
			id:          "official_script",
			label:       "Official Installer (antigravity.google)",
			preview:     "curl -fsSL https://antigravity.google/cli/install.sh | bash",
			sourceURL:   "https://antigravity.google/cli/install.sh",
			osList:      []string{"darwin", "linux"},
			prereqTools: []string{"curl", "bash"},
			execArgs:    curlBashExec("https://antigravity.google/cli/install.sh", "bash"),
			recommended: true,
		},
		{
			id:          "official_script",
			label:       "Official Installer (antigravity.google)",
			preview:     "irm https://antigravity.google/cli/install.ps1 | iex",
			sourceURL:   "https://antigravity.google/cli/install.ps1",
			osList:      []string{"windows"},
			prereqTools: []string{"powershell"},
			execArgs:    powershellExec("https://antigravity.google/cli/install.ps1"),
			recommended: true,
		},
	},
	"claude_code": {
		{
			id:          "official_script",
			label:       "Official Native Installer (claude.ai)",
			preview:     "curl -fsSL https://claude.ai/install.sh | bash",
			sourceURL:   "https://claude.ai/install.sh",
			osList:      []string{"darwin", "linux"},
			prereqTools: []string{"curl", "bash"},
			execArgs:    curlBashExec("https://claude.ai/install.sh", "bash"),
			recommended: true,
		},
		{
			id:          "official_script",
			label:       "Official Native Installer (claude.ai)",
			preview:     "irm https://claude.ai/install.ps1 | iex",
			sourceURL:   "https://claude.ai/install.ps1",
			osList:      []string{"windows"},
			prereqTools: []string{"powershell"},
			execArgs:    powershellExec("https://claude.ai/install.ps1"),
			recommended: true,
		},
		{
			id:          "brew",
			label:       "Homebrew Cask (claude-code)",
			preview:     "brew install --cask claude-code",
			sourceURL:   "https://docs.anthropic.com/en/docs/claude-code",
			osList:      []string{"darwin", "linux"},
			prereqTools: []string{"brew"},
			execArgs:    toolExec("brew", "install", "--cask", "claude-code"),
		},
		{
			id:          "npm",
			label:       "npm (@anthropic-ai/claude-code)",
			preview:     "npm install -g @anthropic-ai/claude-code@latest",
			sourceURL:   "https://www.npmjs.com/package/@anthropic-ai/claude-code",
			prereqTools: []string{"npm"},
			execArgs:    toolExec("npm", "install", "-g", "@anthropic-ai/claude-code@latest"),
		},
	},
	"codex": {
		{
			id:          "npm",
			label:       "npm (@openai/codex)",
			preview:     "npm install -g @openai/codex@latest",
			sourceURL:   "https://www.npmjs.com/package/@openai/codex",
			prereqTools: []string{"npm"},
			execArgs:    toolExec("npm", "install", "-g", "@openai/codex@latest"),
			recommended: true,
		},
		{
			id:          "official_script",
			label:       "Official Installer (chatgpt.com)",
			preview:     "curl -fsSL https://chatgpt.com/codex/install.sh | sh",
			sourceURL:   "https://chatgpt.com/codex/install.sh",
			osList:      []string{"darwin", "linux"},
			prereqTools: []string{"curl", "sh"},
			execArgs:    curlBashExec("https://chatgpt.com/codex/install.sh", "sh"),
		},
		{
			id:          "official_script",
			label:       "Official Installer (chatgpt.com)",
			preview:     "irm https://chatgpt.com/codex/install.ps1 | iex",
			sourceURL:   "https://chatgpt.com/codex/install.ps1",
			osList:      []string{"windows"},
			prereqTools: []string{"powershell"},
			execArgs:    powershellExec("https://chatgpt.com/codex/install.ps1"),
		},
		{
			id:          "brew",
			label:       "Homebrew Cask (codex)",
			preview:     "brew install --cask codex",
			sourceURL:   "https://github.com/openai/codex",
			osList:      []string{"darwin"},
			prereqTools: []string{"brew"},
			execArgs:    toolExec("brew", "install", "--cask", "codex"),
		},
	},
	"opencode": {
		{
			id:          "official_script",
			label:       "Official Installer (opencode.ai)",
			preview:     "curl -fsSL https://opencode.ai/install | bash",
			sourceURL:   "https://opencode.ai/install",
			osList:      []string{"darwin", "linux"},
			prereqTools: []string{"curl", "bash"},
			execArgs:    curlBashExec("https://opencode.ai/install", "bash"),
			recommended: true,
		},
		{
			id:          "npm",
			label:       "npm (opencode-ai)",
			preview:     "npm install -g opencode-ai@latest",
			sourceURL:   "https://www.npmjs.com/package/opencode-ai",
			prereqTools: []string{"npm"},
			execArgs:    toolExec("npm", "install", "-g", "opencode-ai@latest"),
			recommended: goruntime.GOOS == "windows",
		},
		{
			id:          "brew",
			label:       "Homebrew (anomalyco/tap/opencode)",
			preview:     "brew install anomalyco/tap/opencode",
			sourceURL:   "https://github.com/anomalyco/opencode",
			osList:      []string{"darwin", "linux"},
			prereqTools: []string{"brew"},
			execArgs:    toolExec("brew", "install", "anomalyco/tap/opencode"),
		},
	},
	"omp": {
		{
			id:          "official_script",
			label:       "Official Installer (omp.sh)",
			preview:     "curl -fsSL https://omp.sh/install | sh",
			sourceURL:   "https://omp.sh/install",
			osList:      []string{"darwin", "linux"},
			prereqTools: []string{"curl", "sh"},
			execArgs:    curlBashExec("https://omp.sh/install", "sh"),
			recommended: true,
		},
		{
			id:          "official_script",
			label:       "Official Installer (omp.sh)",
			preview:     "irm https://omp.sh/install.ps1 | iex",
			sourceURL:   "https://omp.sh/install.ps1",
			osList:      []string{"windows"},
			prereqTools: []string{"powershell"},
			execArgs:    powershellExec("https://omp.sh/install.ps1"),
			recommended: true,
		},
		{
			id:          "bun",
			label:       "Bun (@oh-my-pi/pi-coding-agent)",
			preview:     "bun install -g @oh-my-pi/pi-coding-agent",
			sourceURL:   "https://www.npmjs.com/package/@oh-my-pi/pi-coding-agent",
			prereqTools: []string{"bun"},
			execArgs:    toolExec("bun", "install", "-g", "@oh-my-pi/pi-coding-agent"),
		},
		{
			id:          "brew",
			label:       "Homebrew (can1357/tap/omp)",
			preview:     "brew install can1357/tap/omp",
			sourceURL:   "https://github.com/can1357/oh-my-pi",
			osList:      []string{"darwin", "linux"},
			prereqTools: []string{"brew"},
			execArgs:    toolExec("brew", "install", "can1357/tap/omp"),
		},
	},
	"pi": {
		{
			id:          "official_script",
			label:       "Official Installer (pi.dev)",
			preview:     "curl -fsSL https://pi.dev/install.sh | sh",
			sourceURL:   "https://pi.dev/install.sh",
			osList:      []string{"darwin", "linux"},
			prereqTools: []string{"curl", "sh"},
			execArgs:    curlBashExec("https://pi.dev/install.sh", "sh"),
			recommended: true,
		},
		{
			id:          "official_script",
			label:       "Official Installer (pi.dev)",
			preview:     "irm https://pi.dev/install.ps1 | iex",
			sourceURL:   "https://pi.dev/install.ps1",
			osList:      []string{"windows"},
			prereqTools: []string{"powershell"},
			execArgs:    powershellExec("https://pi.dev/install.ps1"),
			recommended: true,
		},
		{
			id:          "npm",
			label:       "npm (@mariozechner/pi-coding-agent)",
			preview:     "npm install -g @mariozechner/pi-coding-agent@latest",
			sourceURL:   "https://www.npmjs.com/package/@mariozechner/pi-coding-agent",
			prereqTools: []string{"npm"},
			execArgs:    toolExec("npm", "install", "-g", "@mariozechner/pi-coding-agent@latest"),
		},
	},
}

func matchesOS(osList []string, targetOS string) bool {
	if len(osList) == 0 {
		return true
	}
	for _, item := range osList {
		if item == targetOS {
			return true
		}
	}
	return false
}

func checkPrereqs(tools []string) (bool, string) {
	for _, tool := range tools {
		if tool == "powershell" {
			if _, err := ResolveBinary("powershell"); err != nil {
				if _, pwshErr := ResolveBinary("pwsh"); pwshErr != nil {
					return false, "powershell"
				}
			}
			continue
		}
		if _, err := ResolveBinary(tool); err != nil {
			return false, tool
		}
	}
	return true, ""
}

// InstallMethodsForPreset returns the official vendor installation methods for
// presetID on the current host OS, annotated with prerequisite tool availability.
func InstallMethodsForPreset(presetID string) []InstallMethod {
	return installMethodsForOS(presetID, goruntime.GOOS)
}

func installMethodsForOS(presetID, targetOS string) []InstallMethod {
	recipes, ok := installCatalog[strings.ToLower(strings.TrimSpace(presetID))]
	if !ok {
		return nil
	}
	out := make([]InstallMethod, 0, len(recipes))
	for _, r := range recipes {
		if !matchesOS(r.osList, targetOS) {
			continue
		}
		avail, missing := checkPrereqs(r.prereqTools)
		out = append(out, InstallMethod{
			ID:             r.id,
			Label:          r.label,
			CommandPreview: r.preview,
			SourceURL:      r.sourceURL,
			Available:      avail,
			MissingTool:    missing,
			Recommended:    r.recommended,
		})
	}
	return out
}

func updateRecipeForPreset(presetID, resolvedBin, targetOS string) *officialRecipe {
	id := strings.ToLower(strings.TrimSpace(presetID))
	switch id {
	case "antigravity":
		return &officialRecipe{
			id:          "self_update",
			label:       "Official Update (agy update)",
			preview:     "agy update",
			sourceURL:   OfficialDocsURL(id),
			execArgs:    selfSubcommandExec("agy", "update"),
			recommended: true,
		}
	case "claude_code":
		return &officialRecipe{
			id:          "self_update",
			label:       "Official Update (claude update)",
			preview:     "claude update",
			sourceURL:   OfficialDocsURL(id),
			execArgs:    selfSubcommandExec("claude", "update"),
			recommended: true,
		}
	case "opencode":
		return &officialRecipe{
			id:          "self_update",
			label:       "Official Update (opencode upgrade)",
			preview:     "opencode upgrade",
			sourceURL:   OfficialDocsURL(id),
			execArgs:    selfSubcommandExec("opencode", "upgrade"),
			recommended: true,
		}
	case "codex":
		if strings.Contains(resolvedBin, "homebrew") || strings.Contains(resolvedBin, "Cellar") || strings.Contains(resolvedBin, "Caskroom") {
			if ok, _ := checkPrereqs([]string{"brew"}); ok {
				return &officialRecipe{
					id:          "brew",
					label:       "Homebrew Upgrade (codex)",
					preview:     "brew upgrade --cask codex",
					sourceURL:   OfficialDocsURL(id),
					prereqTools: []string{"brew"},
					execArgs:    toolExec("brew", "upgrade", "--cask", "codex"),
					recommended: true,
				}
			}
		}
		if ok, _ := checkPrereqs([]string{"npm"}); ok {
			return &officialRecipe{
				id:          "npm",
				label:       "npm Update (@openai/codex@latest)",
				preview:     "npm install -g @openai/codex@latest",
				sourceURL:   "https://www.npmjs.com/package/@openai/codex",
				prereqTools: []string{"npm"},
				execArgs:    toolExec("npm", "install", "-g", "@openai/codex@latest"),
				recommended: true,
			}
		}
	case "omp", "pi":
		if strings.Contains(resolvedBin, ".bun") {
			if ok, _ := checkPrereqs([]string{"bun"}); ok && id == "omp" {
				return &officialRecipe{
					id:          "bun",
					label:       "Bun Update (@oh-my-pi/pi-coding-agent)",
					preview:     "bun install -g @oh-my-pi/pi-coding-agent",
					sourceURL:   OfficialDocsURL(id),
					prereqTools: []string{"bun"},
					execArgs:    toolExec("bun", "install", "-g", "@oh-my-pi/pi-coding-agent"),
					recommended: true,
				}
			}
		}
	}
	// Fall back to the first matching official install recipe for this OS.
	for _, r := range installCatalog[id] {
		if !matchesOS(r.osList, targetOS) {
			continue
		}
		if avail, _ := checkPrereqs(r.prereqTools); avail {
			copyRecipe := r
			return &copyRecipe
		}
	}
	return nil
}

// UpdateMethodForPreset returns the official update command metadata when an
// external runtime is already installed on the host.
func UpdateMethodForPreset(presetID, resolvedBin string) *InstallMethod {
	r := updateRecipeForPreset(presetID, resolvedBin, goruntime.GOOS)
	if r == nil {
		return nil
	}
	avail, missing := checkPrereqs(r.prereqTools)
	return &InstallMethod{
		ID:             r.id,
		Label:          r.label,
		CommandPreview: r.preview,
		SourceURL:      r.sourceURL,
		Available:      avail,
		MissingTool:    missing,
		Recommended:    true,
	}
}

// RunOfficialInstall executes the compiled-in official vendor installer for
// (presetID, methodID) on the current host OS and streams output lines to onLine.
func RunOfficialInstall(ctx context.Context, presetID, methodID string, onLine func(line string)) error {
	id := strings.ToLower(strings.TrimSpace(presetID))
	recipes, ok := installCatalog[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnsupportedRuntimePreset, presetID)
	}
	method := strings.ToLower(strings.TrimSpace(methodID))
	var chosen *officialRecipe
	for i := range recipes {
		r := &recipes[i]
		if !matchesOS(r.osList, goruntime.GOOS) {
			continue
		}
		if method == "" && r.recommended {
			chosen = r
			break
		}
		if r.id == method {
			chosen = r
			break
		}
	}
	if chosen == nil && method == "" {
		for i := range recipes {
			if matchesOS(recipes[i].osList, goruntime.GOOS) {
				chosen = &recipes[i]
				break
			}
		}
	}
	if chosen == nil {
		return fmt.Errorf("%w: %s for %s", ErrUnsupportedInstallMethod, methodID, presetID)
	}
	return executeRecipe(ctx, id, *chosen, "", onLine)
}

// RunOfficialUpdate runs the compiled-in official update recipe for presetID.
func RunOfficialUpdate(ctx context.Context, presetID, resolvedBin string, onLine func(line string)) error {
	id := strings.ToLower(strings.TrimSpace(presetID))
	if _, ok := installCatalog[id]; !ok {
		return fmt.Errorf("%w: %s", ErrUnsupportedRuntimePreset, presetID)
	}
	recipe := updateRecipeForPreset(id, resolvedBin, goruntime.GOOS)
	if recipe == nil {
		return fmt.Errorf("%w: update for %s", ErrUnsupportedInstallMethod, presetID)
	}
	return executeRecipe(ctx, id, *recipe, resolvedBin, onLine)
}

func executeRecipe(ctx context.Context, presetID string, recipe officialRecipe, resolvedBin string, onLine func(line string)) error {
	bin, args, usesNPM, err := recipe.execArgs(resolvedBin)
	if err != nil {
		return err
	}
	emit := func(text string) {
		if onLine != nil && strings.TrimSpace(text) != "" {
			onLine(text)
		}
	}
	emit("$ " + recipe.preview)

	cmd := exec.CommandContext(ctx, bin, args...)
	configureProcessGroup(cmd)
	if home, hErr := os.UserHomeDir(); hErr == nil && home != "" {
		cmd.Dir = home
	}

	env := append([]string(nil), os.Environ()...)
	if usesNPM {
		env = setEnvVar(env, "PATH", npmExecPATH(bin))
	} else {
		env = setEnvVar(env, "PATH", EnrichedPATH())
	}
	env = setEnvVar(env, "NONINTERACTIVE", "1")
	env = setEnvVar(env, "CI", "1")
	env = setEnvVar(env, "NPM_CONFIG_FUND", "false")
	env = setEnvVar(env, "NPM_CONFIG_AUDIT", "false")
	fallbackPrefix := ""
	if usesNPM {
		if fallbackPrefix = writableNPMPrefixFallback(ctx, bin); fallbackPrefix != "" {
			env = setEnvVar(env, "NPM_CONFIG_PREFIX", fallbackPrefix)
			emit(fmt.Sprintf("Using user-writable npm prefix: %s", fallbackPrefix))
		}
	}
	cmd.Env = env

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		_ = pr.Close()
		return fmt.Errorf("start installer: %w", err)
	}

	doneWatch := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			terminateProcessGroup(cmd)
		case <-doneWatch:
		}
	}()

	var scanWG sync.WaitGroup
	scanWG.Add(1)
	go func() {
		defer scanWG.Done()
		scanner := bufio.NewScanner(pr)
		scanner.Buffer(make([]byte, 0, 16*1024), 256*1024)
		for scanner.Scan() {
			emit(scanner.Text())
		}
	}()

	waitErr := cmd.Wait()
	close(doneWatch)
	_ = pw.Close()
	scanWG.Wait()
	_ = pr.Close()

	if ctx.Err() != nil {
		return ctx.Err()
	}
	if waitErr != nil {
		return fmt.Errorf("installer exited with error: %w", waitErr)
	}
	if usesNPM && fallbackPrefix != "" {
		exposeFallbackBinaryToLocalBin(presetID, fallbackPrefix, emit)
	}
	return nil
}

// npmExecPATH prepends npm's own directory to EnrichedPATH so `#!/usr/bin/env node`
// always resolves npm's paired Node binary even under minimal launchd/systemd PATHs.
func npmExecPATH(npmBin string) string {
	enriched := EnrichedPATH()
	binDir := filepath.Dir(strings.TrimSpace(npmBin))
	if binDir == "" || binDir == "." {
		return enriched
	}
	sep := string(os.PathListSeparator)
	if enriched == "" {
		return binDir
	}
	return binDir + sep + enriched
}

// writableNPMPrefixFallback checks whether npm's current global prefix is
// writable by the current user (e.g. nvm, fnm, Homebrew). When it points to a
// root-owned system path like /usr or /usr/local, it falls back to ~/.npm-global
// so global npm installs work cleanly without sudo.
func writableNPMPrefixFallback(ctx context.Context, npmBin string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	probeCmd := exec.CommandContext(ctx, npmBin, "config", "get", "prefix")
	probeCmd.Env = setEnvVar(append([]string(nil), os.Environ()...), "PATH", npmExecPATH(npmBin))
	out, err := probeCmd.Output()
	if err == nil {
		prefix := strings.TrimSpace(string(out))
		if prefix != "" && prefix != "undefined" {
			target := filepath.Join(prefix, "lib", "node_modules")
			if goruntime.GOOS == "windows" {
				target = prefix
			}
			if isDirWritable(target) || isDirWritable(prefix) {
				return ""
			}
		}
	}
	fallback := filepath.Join(home, ".npm-global")
	_ = os.MkdirAll(filepath.Join(fallback, "bin"), 0o755)
	_ = os.MkdirAll(filepath.Join(fallback, "lib", "node_modules"), 0o755)
	return fallback
}

// exposeFallbackBinaryToLocalBin symlinks an npm-fallback CLI binary from
// ~/.npm-global/bin/<cmd> into ~/.local/bin/<cmd> on Unix so the user can also
// run the CLI directly from their interactive terminal without editing PATH.
func exposeFallbackBinaryToLocalBin(presetID, fallbackPrefix string, emit func(string)) {
	if goruntime.GOOS == "windows" || strings.TrimSpace(fallbackPrefix) == "" {
		return
	}
	preset, ok := config.LookupRuntimePreset(presetID)
	if !ok || strings.TrimSpace(preset.DefaultCommand) == "" {
		return
	}
	cmdName := filepath.Base(strings.TrimSpace(preset.DefaultCommand))
	src := filepath.Join(fallbackPrefix, "bin", cmdName)
	if _, err := os.Stat(src); err != nil {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return
	}
	localBinDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(localBinDir, 0o755); err != nil {
		return
	}
	dst := filepath.Join(localBinDir, cmdName)
	if info, lErr := os.Lstat(dst); lErr == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			// Do not overwrite an existing non-symlink executable.
			return
		}
		_ = os.Remove(dst)
	}
	if err := os.Symlink(src, dst); err == nil && emit != nil {
		emit(fmt.Sprintf("Linked %s -> %s", cmdName, dst))
	}
}

func isDirWritable(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	f, err := os.CreateTemp(dir, ".myaaw-perm-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

func setEnvVar(env []string, key, value string) []string {
	prefix := key + "="
	for i, item := range env {
		if strings.HasPrefix(item, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}
