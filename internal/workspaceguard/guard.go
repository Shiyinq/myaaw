package workspaceguard

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"myaaw/internal/agentdir"
)

var (
	ErrEmptyPath     = errors.New("working directory is empty")
	ErrNotDirectory  = errors.New("path is not a directory")
	ErrNotFound      = errors.New("directory not found")
	ErrForbiddenPath = errors.New("directory is not allowed as a working directory")
)

// Root describes one starting directory shown in the workspace directory picker.
type Root struct {
	Label string `json:"label"`
	Path  string `json:"path"`
	Kind  string `json:"kind"` // "agent_workspace", "configured", "host"
}

// HostBrowseEnabled reports whether paths outside the agent's own workspace
// (`~/.myaaw/agents/<name>/home/workspace`) may be browsed or attached.
// Defaults to true unless MYAAW_ALLOW_HOST_DIRS is explicitly disabled (`0`/`false`).
func HostBrowseEnabled() bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv("MYAAW_ALLOW_HOST_DIRS")))
	return raw != "0" && raw != "false" && raw != "no"
}

// ConfiguredWorkspaceRoots parses the optional MYAAW_WORKSPACE_ROOTS path list.
// When non-empty, host directories outside the agent workspace must reside under
// one of these roots.
func ConfiguredWorkspaceRoots() []string {
	raw := strings.TrimSpace(os.Getenv("MYAAW_WORKSPACE_ROOTS"))
	if raw == "" {
		return nil
	}
	parts := filepath.SplitList(raw)
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		p := expandHome(strings.TrimSpace(part))
		if p == "" {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			abs = resolved
		}
		abs = filepath.Clean(abs)
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		out = append(out, abs)
	}
	return out
}

// ExpandHome resolves a leading `~` or `~/` to the OS user's home directory.
func ExpandHome(p string) string {
	return expandHome(p)
}

func expandHome(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			if p == "~" {
				return home
			}
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// ValidateDir verifies that rawPath points to an existing, attachable directory
// for agentName and returns its canonical absolute path.
func ValidateDir(agentName, rawPath string) (string, error) {
	trimmed := expandHome(rawPath)
	if trimmed == "" {
		return "", ErrEmptyPath
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrForbiddenPath, err)
	}
	abs = filepath.Clean(abs)

	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: %s", ErrNotFound, abs)
		}
		return "", fmt.Errorf("%w: %v", ErrForbiddenPath, err)
	}
	resolved = filepath.Clean(resolved)

	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: %s", ErrNotFound, resolved)
		}
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: %s", ErrNotDirectory, resolved)
	}

	if err := checkPathPolicy(agentName, resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

// ValidateCreatableDir verifies that parentDir + name may be created as a new
// working directory for agentName and returns the target absolute path.
func ValidateCreatableDir(agentName, parentRaw, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." ||
		strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("%w: invalid folder name %q", ErrForbiddenPath, name)
	}
	parent, err := ValidateBrowseParent(agentName, parentRaw)
	if err != nil {
		return "", err
	}
	target := filepath.Clean(filepath.Join(parent, name))
	if err := checkPathPolicy(agentName, target); err != nil {
		return "", err
	}
	return target, nil
}

// ValidateBrowseParent allows browsing $HOME itself (so users can navigate into
// ~/Development or ~/Projects) while still blocking system & credential roots
// and enforcing MYAAW_ALLOW_HOST_DIRS / MYAAW_WORKSPACE_ROOTS.
func ValidateBrowseParent(agentName, rawPath string) (string, error) {
	trimmed := expandHome(rawPath)
	if trimmed == "" {
		return "", ErrEmptyPath
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrForbiddenPath, err)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(abs))
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: %s", ErrNotFound, abs)
		}
		return "", fmt.Errorf("%w: %v", ErrForbiddenPath, err)
	}
	resolved = filepath.Clean(resolved)

	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: %s", ErrNotDirectory, resolved)
	}

	// Allow browsing $HOME to pick a subdirectory (attaching $HOME itself is
	// still blocked by ValidateDir).
	if homeDir, err := canonicalUserHome(); err == nil && resolved == homeDir {
		if !HostBrowseEnabled() {
			return "", fmt.Errorf("%w: host directory browsing is disabled", ErrForbiddenPath)
		}
		if configured := ConfiguredWorkspaceRoots(); len(configured) > 0 {
			allowed := false
			for _, root := range configured {
				if IsWithinBase(root, resolved) {
					allowed = true
					break
				}
			}
			if !allowed {
				return "", fmt.Errorf("%w: directory is outside configured MYAAW_WORKSPACE_ROOTS", ErrForbiddenPath)
			}
		}
		return resolved, nil
	}

	if err := checkPathPolicy(agentName, resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

func checkPathPolicy(agentName, target string) error {
	target = filepath.Clean(target)

	// 1. Refuse filesystem root (/ or C:\).
	if filepath.Dir(target) == target {
		return fmt.Errorf("%w: filesystem root cannot be attached", ErrForbiddenPath)
	}

	// 2. Refuse OS user home root itself (subdirectories are allowed below).
	homeDir, homeErr := canonicalUserHome()
	if homeErr == nil && homeDir != "" && target == homeDir {
		return fmt.Errorf("%w: user home directory root cannot be attached directly; pick a subfolder instead", ErrForbiddenPath)
	}

	// 3. Refuse OS system directories.
	if isSystemDir(target) {
		return fmt.Errorf("%w: system directory cannot be attached", ErrForbiddenPath)
	}

	// 4. Refuse known credential / secret stores under $HOME.
	if homeErr == nil && homeDir != "" {
		for _, rel := range []string{
			".ssh",
			".aws",
			".gnupg",
			".kube",
			".claude",
			".codex",
			".gemini",
			filepath.Join(".config", "gcloud"),
			filepath.Join("Library", "Keychains"),
		} {
			denied := filepath.Join(homeDir, rel)
			if IsWithinBase(denied, target) {
				return fmt.Errorf("%w: sensitive credential directory cannot be attached", ErrForbiddenPath)
			}
		}
	}

	// 5. Enforce Myaaw agent isolation under ~/.myaaw.
	agentWorkspace := ""
	if myaawRoot, err := canonicalMyaawRoot(); err == nil && myaawRoot != "" {
		if agentName != "" {
			if ws, wsErr := canonicalAgentWorkspace(agentName); wsErr == nil && ws != "" {
				agentWorkspace = ws
			}
		}
		if IsWithinBase(myaawRoot, target) {
			// Inside ~/.myaaw, ONLY this agent's own home/workspace subtree is attachable.
			if agentWorkspace != "" && IsWithinBase(agentWorkspace, target) {
				return nil
			}
			return fmt.Errorf("%w: only this agent's workspace directory is allowed under ~/.myaaw", ErrForbiddenPath)
		}
	}

	// 6. Outside ~/.myaaw: enforce host-browse toggle and optional root allowlist.
	if !HostBrowseEnabled() {
		return fmt.Errorf("%w: host directories outside the agent workspace are disabled (MYAAW_ALLOW_HOST_DIRS=0)", ErrForbiddenPath)
	}
	if configured := ConfiguredWorkspaceRoots(); len(configured) > 0 {
		for _, root := range configured {
			if IsWithinBase(root, target) {
				return nil
			}
		}
		return fmt.Errorf("%w: directory is outside configured MYAAW_WORKSPACE_ROOTS", ErrForbiddenPath)
	}
	return nil
}

func isSystemDir(target string) bool {
	if runtime.GOOS == "windows" {
		lower := strings.ToLower(filepath.Clean(target))
		for _, prefix := range []string{`c:\windows`, `c:\program files`, `c:\program files (x86)`, `c:\programdata`} {
			if lower == prefix || strings.HasPrefix(lower, prefix+`\`) {
				return true
			}
		}
		return false
	}
	for _, sysRoot := range []string{"/etc", "/var", "/proc", "/sys", "/dev", "/System", "/sbin", "/usr", "/bin", "/private/etc", "/private/var"} {
		if IsWithinBase(sysRoot, target) {
			// Allow macOS /private/var/folders temporary directories (used by tests & user temp dirs).
			if sysRoot == "/private/var" && IsWithinBase("/private/var/folders", target) {
				return false
			}
			return true
		}
	}
	return false
}

// BrowseRoots returns the curated starting directories for the workspace picker.
func BrowseRoots(agentName string) []Root {
	var roots []Root
	seen := make(map[string]struct{})
	add := func(label, rawPath, kind string) {
		p := filepath.Clean(rawPath)
		if p == "" {
			return
		}
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			p = filepath.Clean(resolved)
		}
		if _, ok := seen[p]; ok {
			return
		}
		if info, err := os.Stat(p); err != nil || !info.IsDir() {
			return
		}
		seen[p] = struct{}{}
		roots = append(roots, Root{Label: label, Path: p, Kind: kind})
	}

	if agentName != "" {
		if agentRoot, err := agentdir.AgentRoot(agentName); err == nil && agentRoot != "" {
			ws := filepath.Join(agentRoot, "home", "workspace")
			projects := filepath.Join(ws, "projects")
			_ = os.MkdirAll(projects, 0o755)
			add("Agent Projects", projects, "agent_workspace")
			add("Agent Workspace", ws, "agent_workspace")
		}
	}

	if !HostBrowseEnabled() {
		return roots
	}

	if configured := ConfiguredWorkspaceRoots(); len(configured) > 0 {
		for _, r := range configured {
			add(filepath.Base(r), r, "configured")
		}
		return roots
	}

	if home, err := canonicalUserHome(); err == nil && home != "" {
		for _, candidate := range []struct {
			label string
			rel   string
		}{
			{"Development", "Development"},
			{"Projects", "Projects"},
			{"Code", "Code"},
			{"Workspace", "workspace"},
			{"Desktop", "Desktop"},
			{"Documents", "Documents"},
		} {
			add(candidate.label, filepath.Join(home, candidate.rel), "host")
		}
		add("Home", home, "host")
	}
	return roots
}

// HasGit reports whether dir contains a `.git` directory or worktree pointer file.
func HasGit(dir string) bool {
	if strings.TrimSpace(dir) == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// GitBranch returns the current Git branch name (or short detached commit hash)
// for dir by reading `.git/HEAD` directly, avoiding subprocess overhead.
// Returns "" if dir is not a Git repository or HEAD cannot be read.
func GitBranch(dir string) string {
	if strings.TrimSpace(dir) == "" {
		return ""
	}
	dotGit := filepath.Join(dir, ".git")
	info, err := os.Stat(dotGit)
	if err != nil {
		return ""
	}
	gitDir := dotGit
	if !info.IsDir() {
		raw, err := os.ReadFile(dotGit)
		if err != nil {
			return ""
		}
		line := strings.TrimSpace(string(raw))
		if !strings.HasPrefix(line, "gitdir:") {
			return ""
		}
		target := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
		if target == "" {
			return ""
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(dir, target)
		}
		gitDir = filepath.Clean(target)
	}
	headBytes, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	head := strings.TrimSpace(string(headBytes))
	if head == "" {
		return ""
	}
	if strings.HasPrefix(head, "ref:") {
		ref := strings.TrimSpace(strings.TrimPrefix(head, "ref:"))
		ref = strings.TrimPrefix(ref, "refs/heads/")
		ref = strings.TrimPrefix(ref, "refs/tags/")
		return ref
	}
	if len(head) > 7 {
		return head[:7]
	}
	return head
}

// IsWithinBase reports whether target is base itself or a descendant of base.
func IsWithinBase(base, target string) bool {
	base = filepath.Clean(base)
	target = filepath.Clean(target)
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func canonicalUserHome() (string, error) {
	h, err := os.UserHomeDir()
	if err != nil || h == "" {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(h); err == nil {
		return filepath.Clean(resolved), nil
	}
	return filepath.Clean(h), nil
}

func canonicalMyaawRoot() (string, error) {
	r, err := agentdir.MyaawRoot()
	if err != nil || r == "" {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(r); err == nil {
		return filepath.Clean(resolved), nil
	}
	return filepath.Clean(r), nil
}

func canonicalAgentWorkspace(agentName string) (string, error) {
	root, err := agentdir.AgentRoot(agentName)
	if err != nil || root == "" {
		return "", err
	}
	ws := filepath.Join(root, "home", "workspace")
	if resolved, err := filepath.EvalSymlinks(ws); err == nil {
		return filepath.Clean(resolved), nil
	}
	if resolvedRoot, err := filepath.EvalSymlinks(root); err == nil {
		return filepath.Clean(filepath.Join(resolvedRoot, "home", "workspace")), nil
	}
	return filepath.Clean(ws), nil
}
