package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"myaaw/internal/agentdir"
	"myaaw/internal/attachment"
)

const (
	// ProjectMaxFiles and ProjectMaxBytes bound what the preview service will
	// inspect or copy. Larger projects stay download-only.
	ProjectMaxFiles = 500
	ProjectMaxBytes = 50 << 20
	// ProjectPreviewTTL is how long a cached build stays usable.
	ProjectPreviewTTL = 15 * time.Minute
	// projectBuildTimeout bounds npm install plus the build command.
	projectBuildTimeout = 5 * time.Minute
	projectLogLimit     = 64 << 10
	// projectCacheSchema invalidates caches produced by an older build strategy
	// (for example absolute asset paths from before the relative-base fix).
	projectCacheSchema = "v2"
)

var (
	ErrProjectNotDirectory = errors.New("path is not a project directory")
	ErrProjectNotBuildable = errors.New("project type has no build step")
	ErrProjectBuilding     = errors.New("project is already building")
	ErrProjectNotReady     = errors.New("project preview is not ready")
)

// ProjectStatus is the state a dashboard needs to render one project card.
type ProjectStatus struct {
	Path         string `json:"path"`
	Type         string `json:"type"`
	State        string `json:"state"`
	Entry        string `json:"entry,omitempty"`
	URL          string `json:"url,omitempty"`
	ExpiresAt    int64  `json:"expires_at,omitempty"`
	Logs         string `json:"logs,omitempty"`
	Error        string `json:"error,omitempty"`
	FileCount    int    `json:"file_count,omitempty"`
	Size         int64  `json:"size,omitempty"`
	DownloadOnly bool   `json:"download_only,omitempty"`
}

type projectBuild struct {
	status    ProjectStatus
	output    string
	updatedAt time.Time
}

// ProjectService inspects project directories and manages sandboxed preview
// builds. One build runs per project; a second request observes the first.
type ProjectService struct {
	files *FileService

	mu     sync.Mutex
	builds map[string]*projectBuild
}

func NewProjectService(files *FileService) *ProjectService {
	return &ProjectService{files: files, builds: make(map[string]*projectBuild)}
}

// Status classifies a project and reports its preview state.
func (s *ProjectService) Status(agentName, virtualPath string) (ProjectStatus, error) {
	abs, cleaned, err := s.resolveProject(agentName, virtualPath)
	if err != nil {
		return ProjectStatus{}, err
	}

	projectType := attachment.DetectProjectType(abs)
	count, size, over := measureProject(abs)
	status := ProjectStatus{
		Path:      cleaned,
		Type:      projectType,
		State:     "idle",
		FileCount: count,
		Size:      size,
	}
	if over {
		status.State = "failed"
		status.DownloadOnly = true
		status.Error = fmt.Sprintf("Project is larger than %d files or %d MB; download-only.", ProjectMaxFiles, ProjectMaxBytes>>20)
		return status, nil
	}

	switch {
	case projectType == attachment.ProjectStatic:
		entry, ok := staticEntry(abs)
		if !ok {
			status.State = "failed"
			status.DownloadOnly = true
			status.Error = "No index.html or HTML entry point found."
			return status, nil
		}
		status.Entry = entry
		status.State = "ready"
	case attachment.Buildable(projectType):
		s.mu.Lock()
		build := s.builds[s.buildKey(agentName, cleaned)]
		s.mu.Unlock()
		if build == nil {
			status.State = "idle"
			return status, nil
		}
		if build.status.State == "ready" && time.Since(build.updatedAt) > ProjectPreviewTTL {
			status.State = "idle"
			return status, nil
		}
		status.State = build.status.State
		status.Entry = build.status.Entry
		status.Logs = build.status.Logs
		status.Error = build.status.Error
		if status.State == "ready" {
			status.Entry = "index.html"
		}
	default:
		status.State = "failed"
		status.DownloadOnly = true
		status.Error = "This project has no preview entry point."
	}
	return status, nil
}

// StartBuild launches a bounded build for a built project. It returns the
// current status plus whether this call started the build.
func (s *ProjectService) StartBuild(agentName, virtualPath string) (ProjectStatus, bool, error) {
	abs, cleaned, err := s.resolveProject(agentName, virtualPath)
	if err != nil {
		return ProjectStatus{}, false, err
	}
	projectType := attachment.DetectProjectType(abs)
	if !attachment.Buildable(projectType) {
		return ProjectStatus{}, false, ErrProjectNotBuildable
	}
	if count, size, over := measureProject(abs); over {
		return ProjectStatus{
			Path:         cleaned,
			Type:         projectType,
			State:        "failed",
			FileCount:    count,
			Size:         size,
			DownloadOnly: true,
			Error:        fmt.Sprintf("Project is larger than %d files or %d MB; download-only.", ProjectMaxFiles, ProjectMaxBytes>>20),
		}, false, nil
	}

	key := s.buildKey(agentName, cleaned)
	s.mu.Lock()
	if existing := s.builds[key]; existing != nil {
		if existing.status.State == "building" || (existing.status.State == "ready" && time.Since(existing.updatedAt) < ProjectPreviewTTL) {
			status := existing.status
			s.mu.Unlock()
			return status, false, nil
		}
	}
	status := ProjectStatus{Path: cleaned, Type: projectType, State: "building", Entry: "index.html"}
	s.builds[key] = &projectBuild{status: status, updatedAt: time.Now()}
	s.mu.Unlock()

	go s.runBuild(agentName, cleaned, abs, projectType, key)
	return status, true, nil
}

// PreviewBase returns the directory that serves the project: the project
// directory for static sites, or the cached build output for built projects.
func (s *ProjectService) PreviewBase(agentName, virtualPath string) (string, error) {
	abs, cleaned, err := s.resolveProject(agentName, virtualPath)
	if err != nil {
		return "", err
	}
	projectType := attachment.DetectProjectType(abs)
	if !attachment.Buildable(projectType) {
		return abs, nil
	}

	s.mu.Lock()
	build := s.builds[s.buildKey(agentName, cleaned)]
	s.mu.Unlock()
	if build == nil || build.status.State == "idle" {
		return "", ErrProjectNotReady
	}
	if build.status.State == "building" {
		return "", ErrProjectBuilding
	}
	if build.status.State == "failed" || build.output == "" {
		return "", ErrProjectNotReady
	}
	if time.Since(build.updatedAt) > ProjectPreviewTTL {
		return "", ErrProjectNotReady
	}
	return build.output, nil
}

// PreviewFile resolves a relative preview path under a project. It rejects
// traversal, hidden entries, dependency trees, and symlinks that escape the
// served root.
func (s *ProjectService) PreviewFile(agentName, virtualPath, rel string) (string, error) {
	base, err := s.PreviewBase(agentName, virtualPath)
	if err != nil {
		return "", err
	}

	rel = path.Clean(strings.TrimPrefix(strings.ReplaceAll(rel, "\\", "/"), "/"))
	if rel == "." || rel == "" {
		rel = "index.html"
	}
	if rel == ".." || strings.HasPrefix(rel, "../") || path.IsAbs(rel) {
		return "", fmt.Errorf("%w: invalid preview path", ErrInvalidPath)
	}
	for _, segment := range strings.Split(rel, "/") {
		if strings.HasPrefix(segment, ".") || segment == "node_modules" {
			return "", fmt.Errorf("%w: invalid preview path", ErrInvalidPath)
		}
	}

	abs := filepath.Join(base, filepath.FromSlash(rel))
	if !isWithin(canonicalPath(base), canonicalPath(abs)) {
		return "", fmt.Errorf("%w: preview path escapes the project", ErrInvalidPath)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", ErrPathNotFound
	}
	if info.IsDir() {
		abs = filepath.Join(abs, "index.html")
		info, err = os.Stat(abs)
		if err != nil {
			return "", ErrPathNotFound
		}
	}
	if info.IsDir() {
		return "", ErrPathNotFound
	}
	return abs, nil
}

// staticEntry picks index.html, or the first top-level HTML file, as the entry.
func staticEntry(dir string) (string, bool) {
	if info, err := os.Stat(filepath.Join(dir, "index.html")); err == nil && !info.IsDir() {
		return "index.html", true
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*.html"))
	sort.Strings(matches)
	if len(matches) == 0 {
		return "", false
	}
	return filepath.Base(matches[0]), true
}

func (s *ProjectService) resolveProject(agentName, virtualPath string) (string, string, error) {
	if strings.TrimSpace(virtualPath) == "" {
		return "", "", fmt.Errorf("%w: project path is required", ErrInvalidPath)
	}
	abs, cleaned, _, err := s.files.resolve(agentName, virtualPath)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", ErrPathNotFound
		}
		return "", "", err
	}
	if !info.IsDir() {
		return "", "", ErrProjectNotDirectory
	}
	return abs, cleaned, nil
}

func (s *ProjectService) buildKey(agentName, virtualPath string) string {
	return agentName + "\x00" + virtualPath
}

// measureProject counts files and bytes while ignoring dependency and VCS
// directories. It stops early once a cap is crossed, so a huge tree never costs
// a full walk.
func measureProject(dir string) (count int, size int64, over bool) {
	_ = filepath.WalkDir(dir, func(current string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if current != dir && skipProjectDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		info, statErr := entry.Info()
		if statErr != nil {
			return nil
		}
		count++
		size += info.Size()
		if count > ProjectMaxFiles || size > ProjectMaxBytes {
			over = true
			return io.EOF
		}
		return nil
	})
	return count, size, over
}

func skipProjectDir(name string) bool {
	switch name {
	case ".git", "node_modules", "dist", "build", "out", ".next", ".svelte-kit", ".cache", "vendor":
		return true
	default:
		return strings.HasPrefix(name, ".") && name != "."
	}
}

// treeHash fingerprints a project's file names, sizes, and mtimes so a rebuild
// can reuse a recent cache entry.
func treeHash(dir string) string {
	hash := sha256.New()
	_ = filepath.WalkDir(dir, func(current string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if current != dir && skipProjectDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		info, statErr := entry.Info()
		if statErr != nil {
			return nil
		}
		rel, relErr := filepath.Rel(dir, current)
		if relErr != nil {
			return nil
		}
		fmt.Fprintf(hash, "%s\x00%d\x00%d\n", filepath.ToSlash(rel), info.Size(), info.ModTime().UnixNano())
		return nil
	})
	return hex.EncodeToString(hash.Sum(nil))
}

func (s *ProjectService) runBuild(agentName, virtualPath, projectAbs, projectType, key string) {
	workDir, err := os.MkdirTemp("", "myaaw-project-*")
	if err != nil {
		s.finishBuild(key, ProjectStatus{State: "failed", Error: err.Error()}, "")
		return
	}
	defer os.RemoveAll(workDir)

	var logs bytes.Buffer
	appendLog := func(format string, args ...any) {
		fmt.Fprintf(&logs, format+"\n", args...)
		if logs.Len() > projectLogLimit {
			trimmed := logs.Bytes()[logs.Len()-projectLogLimit:]
			logs.Reset()
			logs.Write(trimmed)
		}
		s.setBuildLogs(key, logs.String())
	}

	cacheRoot, err := projectCacheDir(agentName)
	if err != nil {
		s.finishBuild(key, ProjectStatus{State: "failed", Error: err.Error()}, "")
		return
	}
	hash := treeHash(projectAbs)
	cacheDir := filepath.Join(cacheRoot, hash+"-"+projectCacheSchema)
	s.cleanupCache(cacheRoot)
	if isFreshCache(cacheDir) {
		s.finishBuild(key, ProjectStatus{State: "ready", Entry: "index.html"}, cacheDir)
		return
	}

	// A daemon started by launchd/systemd often has a minimal PATH, so npm is
	// resolved explicitly instead of relying on the service environment.
	npmPath, err := npmExecutable()
	if err != nil {
		s.finishBuild(key, ProjectStatus{State: "failed", Error: err.Error()}, "")
		return
	}

	appendLog("Preparing %s project in %s", projectType, workDir)
	if err := copyProject(projectAbs, workDir); err != nil {
		s.finishBuild(key, ProjectStatus{State: "failed", Error: err.Error()}, "")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), projectBuildTimeout)
	defer cancel()
	run := func(args ...string) bool {
		appendLog("$ %s %s", filepath.Base(npmPath), strings.Join(args, " "))
		cmd := exec.CommandContext(ctx, npmPath, args...)
		cmd.Dir = workDir
		cmd.Env = envWithPath(filepath.Dir(npmPath))
		output, runErr := cmd.CombinedOutput()
		if len(output) > 0 {
			if logs.Len()+len(output) > projectLogLimit {
				output = output[len(output)-(projectLogLimit/2):]
			}
			logs.Write(output)
			s.setBuildLogs(key, logs.String())
		}
		if runErr != nil {
			appendLog("✗ %v", runErr)
			return false
		}
		return true
	}

	if _, statErr := os.Stat(filepath.Join(workDir, "node_modules")); statErr != nil {
		npmArgs := []string{"install", "--ignore-scripts", "--no-audit", "--no-fund"}
		if _, lockErr := os.Stat(filepath.Join(workDir, "package-lock.json")); lockErr == nil {
			npmArgs = []string{"ci", "--ignore-scripts", "--no-audit", "--no-fund"}
		}
		if !run(npmArgs...) {
			s.finishBuild(key, ProjectStatus{State: "failed", Error: "Dependency install failed. See logs.", Logs: logs.String()}, "")
			return
		}
	}
	buildArgs := []string{"run", "build", "--if-present"}
	if usesViteRelativeBase(workDir) {
		// Vite's default base is '/', which breaks under the tokenized preview
		// path; relative assets keep HTML and lazy chunks inside the project.
		buildArgs = append(buildArgs, "--", "--base=./")
	}
	if !run(buildArgs...) {
		s.finishBuild(key, ProjectStatus{State: "failed", Error: "Build command failed. See logs.", Logs: logs.String()}, "")
		return
	}

	outputDir, err := findBuildOutput(workDir)
	if err != nil {
		s.finishBuild(key, ProjectStatus{State: "failed", Error: err.Error(), Logs: logs.String()}, "")
		return
	}
	if err := os.RemoveAll(cacheDir); err != nil {
		s.finishBuild(key, ProjectStatus{State: "failed", Error: err.Error()}, "")
		return
	}
	if err := copyDir(outputDir, cacheDir); err != nil {
		_ = os.RemoveAll(cacheDir)
		s.finishBuild(key, ProjectStatus{State: "failed", Error: err.Error(), Logs: logs.String()}, "")
		return
	}
	// Frameworks that ignore a relative base still emit root-absolute asset URLs.
	// Rewriting them keeps the preview working under the tokenized path.
	if err := rewritePreviewPaths(cacheDir); err != nil {
		_ = os.RemoveAll(cacheDir)
		s.finishBuild(key, ProjectStatus{State: "failed", Error: err.Error(), Logs: logs.String()}, "")
		return
	}
	appendLog("✓ preview ready (%s)", outputDir)
	s.finishBuild(key, ProjectStatus{State: "ready", Entry: "index.html", Logs: logs.String()}, cacheDir)
}

func (s *ProjectService) finishBuild(key string, status ProjectStatus, output string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	build := s.builds[key]
	if build == nil {
		return
	}
	build.status = status
	build.output = output
	build.updatedAt = time.Now()
}

func (s *ProjectService) setBuildLogs(key, logs string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if build := s.builds[key]; build != nil && build.status.State == "building" {
		build.status.Logs = logs
		build.updatedAt = time.Now()
	}
}

func (s *ProjectService) cleanupCache(cacheRoot string) {
	entries, err := os.ReadDir(cacheRoot)
	if err != nil {
		return
	}
	now := time.Now()
	for _, entry := range entries {
		info, statErr := entry.Info()
		if statErr != nil || now.Sub(info.ModTime()) <= ProjectPreviewTTL {
			continue
		}
		_ = os.RemoveAll(filepath.Join(cacheRoot, entry.Name()))
	}
}

func projectCacheDir(agentName string) (string, error) {
	dir, err := agentdir.CacheDir(agentName)
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "projects")
	return dir, os.MkdirAll(dir, 0o700)
}

func isFreshCache(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "index.html"))
	if err != nil || info.IsDir() {
		return false
	}
	return time.Since(info.ModTime()) < ProjectPreviewTTL
}

// npmExecutable finds npm even when the control plane runs from a daemon with a
// minimal PATH. MYAAW_NPM wins, then the inherited PATH, then the usual
// package-manager and per-user install locations.
func npmExecutable() (string, error) {
	if override := strings.TrimSpace(os.Getenv("MYAAW_NPM")); override != "" {
		if resolved, err := exec.LookPath(override); err == nil {
			return resolved, nil
		}
		if isExecutableFile(override) {
			return override, nil
		}
		return "", fmt.Errorf("MYAAW_NPM=%q is not an executable file", override)
	}
	if resolved, err := exec.LookPath("npm"); err == nil {
		return resolved, nil
	}

	home, _ := os.UserHomeDir()
	var patterns []string
	if home != "" {
		patterns = []string{
			filepath.Join(home, ".nvm", "versions", "node", "*", "bin", "npm"),
			filepath.Join(home, ".fnm", "node-versions", "*", "installation", "bin", "npm"),
			filepath.Join(home, ".local", "share", "mise", "installs", "node", "*", "bin", "npm"),
			filepath.Join(home, ".asdf", "installs", "nodejs", "*", "bin", "npm"),
		}
	}
	var matches []string
	for _, pattern := range patterns {
		found, _ := filepath.Glob(pattern)
		matches = append(matches, found...)
	}
	// Newest installed version wins when several version-manager installs exist.
	sort.SliceStable(matches, func(i, j int) bool {
		return nodeVersionRank(matches[i]) > nodeVersionRank(matches[j])
	})

	// Version-manager installs come first: their bin directory also contains the
	// matching node, which npm scripts (vite, svelte-kit, next) need. A stale
	// system npm without a sibling node is only a last resort.
	globalCandidates := []string{
		"/opt/homebrew/bin/npm",
		"/usr/local/bin/npm",
		"/usr/local/opt/node/bin/npm",
		"/opt/local/bin/npm",
	}
	if home != "" {
		globalCandidates = append(globalCandidates,
			filepath.Join(home, ".local", "bin", "npm"),
			filepath.Join(home, ".npm-global", "bin", "npm"),
			filepath.Join(home, "Library", "pnpm", "npm"),
			filepath.Join(home, ".volta", "bin", "npm"),
		)
	}

	ordered := append(append([]string{}, matches...), globalCandidates...)
	nodeName := "node"
	if runtime.GOOS == "windows" {
		nodeName = "node.exe"
	}
	for _, candidate := range ordered {
		if isExecutableFile(candidate) && isExecutableFile(filepath.Join(filepath.Dir(candidate), nodeName)) {
			return candidate, nil
		}
	}
	for _, candidate := range ordered {
		if isExecutableFile(candidate) {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("npm was not found: install Node.js or set MYAAW_NPM to the npm executable")
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode().Perm()&0o111 != 0
}

// nodeVersionRank turns a path containing vX.Y.Z into X*1e9+Y*1e6+Z*1e3 so nvm
// and fnm installs sort newest-first.
func nodeVersionRank(path string) int {
	for _, segment := range strings.Split(filepath.ToSlash(path), "/") {
		if len(segment) < 2 || segment[0] != 'v' || segment[1] < '0' || segment[1] > '9' {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(segment, "v"), ".")
		rank := 0
		for i := 0; i < 3; i++ {
			rank *= 1000
			if i >= len(parts) {
				continue
			}
			value, err := strconv.Atoi(parts[i])
			if err != nil {
				continue
			}
			rank += value
		}
		return rank
	}
	return 0
}

// envWithPath prepends npm's directory to PATH, replacing any existing PATH
// entry so the child sees exactly one value. npm's shebang and the scripts it
// spawns (vite, svelte-kit, next) need node from the same directory.
func envWithPath(dir string) []string {
	prefix := dir + string(os.PathListSeparator)
	env := os.Environ()
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, entry := range env {
		if len(entry) >= 5 && strings.EqualFold(entry[:5], "PATH=") {
			out = append(out, entry[:5]+prefix+entry[5:])
			replaced = true
			continue
		}
		out = append(out, entry)
	}
	if !replaced {
		out = append(out, "PATH="+prefix)
	}
	return out
}

// findBuildOutput picks the conventional output directory that contains a
// previewable entry point.
func findBuildOutput(workDir string) (string, error) {
	candidates := []string{
		"dist", "build", "out",
		".svelte-kit/output/client",
		".svelte-kit/output/prerendered/pages",
		".next/out",
	}
	for _, rel := range candidates {
		dir := filepath.Join(workDir, filepath.FromSlash(rel))
		if info, err := os.Stat(filepath.Join(dir, "index.html")); err == nil && !info.IsDir() {
			return dir, nil
		}
	}
	return "", fmt.Errorf("Build finished but no dist/build/out directory with index.html was found")
}

// usesViteRelativeBase reports whether the build script is a plain Vite build.
// SvelteKit and Next own their base paths, so the flag is skipped there.
func usesViteRelativeBase(dir string) bool {
	if !hasProjectGlob(dir, "vite.config.*") {
		return false
	}
	if hasProjectGlob(dir, "svelte.config.*") || hasProjectGlob(dir, "next.config.*") {
		return false
	}
	return true
}

func hasProjectGlob(dir, pattern string) bool {
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	return err == nil && len(matches) > 0
}

var (
	htmlAbsoluteRefRE = regexp.MustCompile(`(?i)\b(src|href|poster|action)(\s*=\s*)(["'])/([^/"'])`)
	htmlRootRefRE     = regexp.MustCompile(`(?i)\b(href|action)(\s*=\s*)(["'])/(["'])`)
	cssAbsoluteURLRE  = regexp.MustCompile(`(?i)url\(\s*(["']?)/([^/"'])`)
	cssImportRE       = regexp.MustCompile(`(?i)(@import\s+["'])/([^/"'])`)
)

// rewritePreviewPaths turns root-absolute asset references into document-relative
// ones. A preview is served under /agents/<name>/project/preview/<token>/, so
// href="/assets/app.js" would otherwise request the control-plane root.
func rewritePreviewPaths(root string) error {
	return filepath.WalkDir(root, func(current string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		switch strings.ToLower(filepath.Ext(current)) {
		case ".html", ".htm", ".css":
		default:
			return nil
		}

		rel, relErr := filepath.Rel(root, current)
		if relErr != nil {
			return relErr
		}
		depth := strings.Count(filepath.ToSlash(rel), "/")
		prefix := strings.Repeat("../", depth)
		if prefix == "" {
			prefix = "./"
		}

		data, readErr := os.ReadFile(current)
		if readErr != nil {
			return readErr
		}
		content := string(data)
		rewritten := htmlAbsoluteRefRE.ReplaceAllString(content, "${1}${2}${3}"+prefix+"${4}")
		rewritten = htmlRootRefRE.ReplaceAllString(rewritten, "${1}${2}${3}"+prefix+"${4}")
		rewritten = cssAbsoluteURLRE.ReplaceAllString(rewritten, "url(${1}"+prefix+"${2}")
		rewritten = cssImportRE.ReplaceAllString(rewritten, "${1}"+prefix+"${2}")
		if rewritten == content {
			return nil
		}

		mode := os.FileMode(0o644)
		if info, statErr := entry.Info(); statErr == nil {
			mode = info.Mode().Perm()
		}
		return os.WriteFile(current, []byte(rewritten), mode)
	})
}

func copyProject(src, dst string) error {
	var files int
	var size int64
	return filepath.WalkDir(src, func(current string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(src, current)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		if entry.IsDir() {
			if skipProjectDir(entry.Name()) {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		info, statErr := entry.Info()
		if statErr != nil {
			return statErr
		}
		files++
		size += info.Size()
		if files > ProjectMaxFiles || size > ProjectMaxBytes {
			return fmt.Errorf("project exceeds %d files or %d MB", ProjectMaxFiles, ProjectMaxBytes>>20)
		}
		return copyFile(current, filepath.Join(dst, rel), info.Mode())
	})
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(current string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(src, current)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		if entry.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		info, statErr := entry.Info()
		if statErr != nil {
			return statErr
		}
		return copyFile(current, filepath.Join(dst, rel), info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// ContentTypeForPreview keeps browsers honest: an unknown extension is served
// as octet-stream instead of being sniffed.
func ContentTypeForPreview(file string) string {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	}
	if value := mime.TypeByExtension(filepath.Ext(file)); value != "" {
		return value
	}
	return "application/octet-stream"
}
