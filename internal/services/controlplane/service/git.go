package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"myaaw/internal/workspaceguard"
)

const (
	gitCommandTimeout = 10 * time.Second
	gitNetworkTimeout = 45 * time.Second
	maxGitDiffBytes   = 512 * 1024  // 512 KiB
	maxGitOutputBytes = 2 << 20     // 2 MiB stdout/stderr cap
	maxGitMessageSize = 16 * 1024   // 16 KiB commit/stash message cap
	maxGitBatchPaths  = 500
)

var (
	ErrNoWorkingDir   = errors.New("conversation has no attached working directory")
	ErrNotGitRepo     = errors.New("working directory is not a git repository")
	ErrInvalidGitArg  = errors.New("invalid git argument")
	gitCommitHashRe   = regexp.MustCompile(`^[0-9a-fA-F]{4,40}$`)
	gitStashRefRe     = regexp.MustCompile(`^stash@\{[0-9]+\}$`)
	gitURLCredsRe     = regexp.MustCompile(`(https?://)[^/\s@]+@`)
)

// GitChangeEntry represents one changed, staged, or untracked path in git status.
type GitChangeEntry struct {
	Path     string `json:"path"`
	OrigPath string `json:"orig_path,omitempty"`
	Status   string `json:"status"` // "M" | "A" | "D" | "R" | "C" | "U" | "?"
}

// GitStatusResponse describes the git state of a conversation's working directory.
type GitStatusResponse struct {
	HasGit    bool             `json:"has_git"`
	Branch    string           `json:"branch,omitempty"`
	HeadOID   string           `json:"head_oid,omitempty"`
	Upstream  string           `json:"upstream,omitempty"`
	Ahead     int              `json:"ahead"`
	Behind    int              `json:"behind"`
	Detached  bool             `json:"detached,omitempty"`
	Clean     bool             `json:"clean"`
	Staged    []GitChangeEntry `json:"staged"`
	Unstaged  []GitChangeEntry `json:"unstaged"`
	Untracked []GitChangeEntry `json:"untracked"`
}

// GitCommitEntry is one commit returned by git log.
type GitCommitEntry struct {
	Hash      string `json:"hash"`
	ShortHash string `json:"short_hash"`
	Subject   string `json:"subject"`
	Author    string `json:"author"`
	Date      string `json:"date"`
}

// GitLogPageResponse wraps a paginated slice of commits for infinite scrolling.
type GitLogPageResponse struct {
	Commits []GitCommitEntry `json:"commits"`
	Offset  int              `json:"offset"`
	Limit   int              `json:"limit"`
	HasMore bool             `json:"has_more"`
}

// GitDiffResponse is the unified diff (or untracked text preview) for one file.
type GitDiffResponse struct {
	Path      string `json:"path"`
	Commit    string `json:"commit,omitempty"`
	Staged    bool   `json:"staged"`
	Untracked bool   `json:"untracked"`
	Binary    bool   `json:"binary"`
	Truncated bool   `json:"truncated,omitempty"`
	Diff      string `json:"diff"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// GitService provides Git inspection and Source Control actions for a
// conversation's attached working directory.
type GitService struct {
	conversations *ConversationService
	mu            sync.Mutex
	repoLocks     map[string]*sync.Mutex
}

func NewGitService(conversations *ConversationService) *GitService {
	return &GitService{
		conversations: conversations,
		repoLocks:     make(map[string]*sync.Mutex),
	}
}

// lockRepo serializes mutating git commands per repository path so rapid UI
// clicks do not collide on .git/index.lock.
func (s *GitService) lockRepo(dir string) func() {
	key := canonicalPath(dir)
	s.mu.Lock()
	m, ok := s.repoLocks[key]
	if !ok {
		m = &sync.Mutex{}
		s.repoLocks[key] = m
	}
	s.mu.Unlock()
	m.Lock()
	return m.Unlock
}

// resolveGitDir validates conversation ownership, working_dir guard, and that
// the resolved directory is a Git repository.
func (s *GitService) resolveGitDir(agentName, convID string, userID int) (string, error) {
	dir, err := s.conversations.ResolveWorkingDir(agentName, convID, userID)
	if err != nil {
		return "", err
	}
	if dir == "" {
		return "", ErrNoWorkingDir
	}
	if !workspaceguard.HasGit(dir) {
		return "", ErrNotGitRepo
	}
	return dir, nil
}

// Status returns branch, ahead/behind, staged, unstaged, and untracked entries.
func (s *GitService) Status(agentName, convID string, userID int) (GitStatusResponse, error) {
	dir, err := s.conversations.ResolveWorkingDir(agentName, convID, userID)
	if err != nil {
		return GitStatusResponse{}, err
	}
	if dir == "" {
		return GitStatusResponse{}, ErrNoWorkingDir
	}
	if !workspaceguard.HasGit(dir) {
		return GitStatusResponse{
			HasGit:    false,
			Staged:    []GitChangeEntry{},
			Unstaged:  []GitChangeEntry{},
			Untracked: []GitChangeEntry{},
		}, nil
	}

	out, err := runGit(dir, "status", "--porcelain=v2", "--branch", "-uall")
	if err != nil {
		return GitStatusResponse{}, err
	}
	return parseGitStatusV2(out), nil
}

// Log returns recent commits in the conversation's attached git repository.
func (s *GitService) Log(agentName, convID string, userID, limit int) ([]GitCommitEntry, error) {
	page, err := s.LogPage(agentName, convID, userID, limit, 0)
	if err != nil {
		return nil, err
	}
	return page.Commits, nil
}

// LogPage returns one paginated slice of commits with has_more for infinite scroll.
func (s *GitService) LogPage(agentName, convID string, userID, limit, offset int) (GitLogPageResponse, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	if offset < 0 {
		offset = 0
	}
	if offset > 100000 {
		offset = 100000
	}
	dir, err := s.conversations.ResolveWorkingDir(agentName, convID, userID)
	if err != nil {
		return GitLogPageResponse{}, err
	}
	if dir == "" {
		return GitLogPageResponse{}, ErrNoWorkingDir
	}
	if !workspaceguard.HasGit(dir) {
		return GitLogPageResponse{
			Commits: []GitCommitEntry{},
			Offset:  offset,
			Limit:   limit,
			HasMore: false,
		}, nil
	}

	// Fetch limit+1 commits to know accurately whether another page exists.
	args := []string{
		"log",
		fmt.Sprintf("--skip=%d", offset),
		fmt.Sprintf("-n%d", limit+1),
		"--format=%H%x1f%h%x1f%s%x1f%an%x1f%aI",
	}
	out, err := runGit(dir, args...)
	if err != nil {
		// Unborn branch (fresh `git init` with 0 commits) returns exit 128.
		if isUnbornGitErr(err) {
			return GitLogPageResponse{
				Commits: []GitCommitEntry{},
				Offset:  offset,
				Limit:   limit,
				HasMore: false,
			}, nil
		}
		return GitLogPageResponse{}, err
	}

	commits := make([]GitCommitEntry, 0, limit+1)
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\x1f")
		if len(parts) < 5 {
			continue
		}
		commits = append(commits, GitCommitEntry{
			Hash:      parts[0],
			ShortHash: parts[1],
			Subject:   parts[2],
			Author:    parts[3],
			Date:      parts[4],
		})
	}
	hasMore := len(commits) > limit
	if hasMore {
		commits = commits[:limit]
	}
	return GitLogPageResponse{
		Commits: commits,
		Offset:  offset,
		Limit:   limit,
		HasMore: hasMore,
	}, nil
}

// Diff returns the unified diff for one relative path inside the attached git
// repository. When untracked is true, it synthesizes an all-additions preview
// up to 512 KiB.
func (s *GitService) Diff(agentName, convID string, userID int, relPath string, staged, untracked bool) (GitDiffResponse, error) {
	dir, err := s.resolveGitDir(agentName, convID, userID)
	if err != nil {
		return GitDiffResponse{}, err
	}

	rel, abs, err := validateGitRelPath(dir, relPath)
	if err != nil {
		return GitDiffResponse{}, err
	}

	resp := GitDiffResponse{
		Path:      rel,
		Staged:    staged,
		Untracked: untracked,
	}

	if untracked {
		return buildUntrackedDiff(abs, resp)
	}

	args := []string{"diff", "--no-color", "--unified=3"}
	if staged {
		args = append(args, "--cached")
	}
	args = append(args, "--", rel)

	out, err := runGit(dir, args...)
	if err != nil {
		return GitDiffResponse{}, err
	}
	return finalizeDiffResponse(resp, out), nil
}

func finalizeDiffResponse(resp GitDiffResponse, out string) GitDiffResponse {
	if strings.Contains(out, "Binary files ") && strings.Contains(out, " differ") {
		resp.Binary = true
		resp.Diff = out
		return resp
	}
	if len(out) > maxGitDiffBytes {
		out = out[:maxGitDiffBytes]
		resp.Truncated = true
	}
	resp.Diff = out
	resp.Additions, resp.Deletions = countDiffLines(out)
	return resp
}

func buildUntrackedDiff(abs string, resp GitDiffResponse) (GitDiffResponse, error) {
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return GitDiffResponse{}, ErrPathNotFound
		}
		return GitDiffResponse{}, err
	}
	if info.IsDir() {
		return GitDiffResponse{}, fmt.Errorf("%w: path is a directory", ErrInvalidPath)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return GitDiffResponse{}, err
	}
	if looksBinary(data) {
		resp.Binary = true
		return resp, nil
	}
	truncated := false
	if len(data) > maxGitDiffBytes {
		data = data[:maxGitDiffBytes]
		truncated = true
	}
	lines := strings.Split(string(data), "\n")
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n", resp.Path, len(lines))
	for _, l := range lines {
		sb.WriteByte('+')
		sb.WriteString(l)
		sb.WriteByte('\n')
	}
	resp.Diff = sb.String()
	resp.Additions = len(lines)
	resp.Truncated = truncated
	return resp, nil
}

type gitExecOpts struct {
	timeout  time.Duration
	stdin    string
	readOnly bool
}

type limitedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if l.limit <= 0 {
		return l.buf.Write(p)
	}
	remaining := l.limit - l.buf.Len()
	if remaining <= 0 {
		l.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = l.buf.Write(p[:remaining])
		l.truncated = true
		return len(p), nil
	}
	return l.buf.Write(p)
}

func (l *limitedBuffer) String() string {
	return l.buf.String()
}

func runGit(dir string, args ...string) (string, error) {
	return runGitWithOptions(dir, gitExecOpts{
		timeout:  gitCommandTimeout,
		readOnly: true,
	}, args...)
}

func runGitWrite(dir string, stdin string, timeout time.Duration, args ...string) (string, error) {
	if timeout <= 0 {
		timeout = gitCommandTimeout
	}
	return runGitWithOptions(dir, gitExecOpts{
		timeout:  timeout,
		stdin:    stdin,
		readOnly: false,
	}, args...)
}

func runGitWithOptions(dir string, opts gitExecOpts, args ...string) (string, error) {
	timeout := opts.timeout
	if timeout <= 0 {
		timeout = gitCommandTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Disable interactive credential prompts and force C locale so output
	// parsing is deterministic and remote commands never hang on a TTY.
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	env := append(os.Environ(),
		"LC_ALL=C",
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
		"GIT_ASKPASS=",
	)
	if opts.readOnly {
		env = append(env, "GIT_OPTIONAL_LOCKS=0")
	}
	cmd.Env = env
	if opts.stdin != "" {
		cmd.Stdin = strings.NewReader(opts.stdin)
	}

	stdout := &limitedBuffer{limit: maxGitOutputBytes}
	stderr := &limitedBuffer{limit: 64 * 1024}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", firstGitSubcmd(args), sanitizeGitOutput(msg))
	}
	return sanitizeGitOutput(stdout.String()), nil
}

func firstGitSubcmd(args []string) string {
	if len(args) == 0 {
		return "command"
	}
	return args[0]
}

func sanitizeGitOutput(raw string) string {
	return gitURLCredsRe.ReplaceAllString(raw, "${1}***@")
}

func isUnbornGitErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "does not have any commits yet") ||
		strings.Contains(msg, "bad default revision") ||
		strings.Contains(msg, "ambiguous argument 'HEAD'")
}

// validateGitRelPath validates a repo-relative path, blocks traversal, NUL
// bytes, .git metadata mutation, and symlink escape outside the repository.
func validateGitRelPath(dir, raw string) (string, string, error) {
	if strings.ContainsRune(raw, '\x00') {
		return "", "", fmt.Errorf("%w: path contains NUL byte", ErrInvalidPath)
	}
	rel := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", "", fmt.Errorf("%w: invalid git path", ErrInvalidPath)
	}
	cleaned := filepath.ToSlash(filepath.Clean(rel))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
		return "", "", fmt.Errorf("%w: invalid git path", ErrInvalidPath)
	}
	for _, seg := range strings.Split(cleaned, "/") {
		if seg == ".git" || seg == ".." {
			return "", "", fmt.Errorf("%w: protected git path", ErrInvalidPath)
		}
	}
	abs := filepath.Join(dir, filepath.FromSlash(cleaned))
	if err := ensureSymlinksInside(dir, abs); err != nil {
		return "", "", err
	}
	return cleaned, abs, nil
}

// validateBranchName enforces safe git branch names and prevents option/flag
// injection (e.g. names starting with '-').
func validateBranchName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || len(name) > 120 {
		return "", fmt.Errorf("%w: invalid branch name", ErrInvalidGitArg)
	}
	if strings.HasPrefix(name, "-") || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") ||
		strings.HasSuffix(name, ".") || strings.HasSuffix(name, ".lock") ||
		strings.Contains(name, "..") || strings.Contains(name, "@{") ||
		strings.Contains(name, "//") || strings.Contains(name, "\\") {
		return "", fmt.Errorf("%w: invalid branch name", ErrInvalidGitArg)
	}
	for _, r := range name {
		if r <= 0x20 || r == 0x7f || strings.ContainsRune("~^:?*[", r) {
			return "", fmt.Errorf("%w: invalid branch name", ErrInvalidGitArg)
		}
	}
	return name, nil
}

func validateCommitHash(raw string) (string, error) {
	hash := strings.TrimSpace(raw)
	if !gitCommitHashRe.MatchString(hash) {
		return "", fmt.Errorf("%w: invalid commit hash", ErrInvalidGitArg)
	}
	return hash, nil
}

func validateStashRef(raw string) (string, error) {
	ref := strings.TrimSpace(raw)
	if ref == "" {
		return "stash@{0}", nil
	}
	if !gitStashRefRe.MatchString(ref) {
		return "", fmt.Errorf("%w: invalid stash reference", ErrInvalidGitArg)
	}
	return ref, nil
}

func validateGitMessage(raw string, required bool) (string, error) {
	if strings.ContainsRune(raw, '\x00') {
		return "", fmt.Errorf("%w: message contains NUL byte", ErrInvalidGitArg)
	}
	msg := strings.TrimSpace(raw)
	if required && msg == "" {
		return "", fmt.Errorf("%w: message is required", ErrInvalidGitArg)
	}
	if len(msg) > maxGitMessageSize {
		return "", fmt.Errorf("%w: message is too long", ErrInvalidGitArg)
	}
	return msg, nil
}

func parseGitStatusV2(raw string) GitStatusResponse {
	res := GitStatusResponse{
		HasGit:    true,
		Staged:    []GitChangeEntry{},
		Unstaged:  []GitChangeEntry{},
		Untracked: []GitChangeEntry{},
	}

	for _, line := range strings.Split(strings.TrimRight(raw, "\n"), "\n") {
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "# branch.oid "):
			res.HeadOID = strings.TrimSpace(strings.TrimPrefix(line, "# branch.oid "))
		case strings.HasPrefix(line, "# branch.head "):
			head := strings.TrimSpace(strings.TrimPrefix(line, "# branch.head "))
			if head == "(detached)" {
				res.Detached = true
				if len(res.HeadOID) >= 7 {
					res.Branch = res.HeadOID[:7]
				} else {
					res.Branch = "HEAD"
				}
			} else {
				res.Branch = head
			}
		case strings.HasPrefix(line, "# branch.upstream "):
			res.Upstream = strings.TrimSpace(strings.TrimPrefix(line, "# branch.upstream "))
		case strings.HasPrefix(line, "# branch.ab "):
			parts := strings.Fields(strings.TrimPrefix(line, "# branch.ab "))
			if len(parts) >= 2 {
				res.Ahead, _ = strconv.Atoi(strings.TrimPrefix(parts[0], "+"))
				res.Behind, _ = strconv.Atoi(strings.TrimPrefix(parts[1], "-"))
			}
		case strings.HasPrefix(line, "1 "):
			// 1 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <path>
			fields := strings.SplitN(line, " ", 9)
			if len(fields) == 9 && len(fields[1]) == 2 {
				xy := fields[1]
				p := fields[8]
				if xy[0] != '.' {
					res.Staged = append(res.Staged, GitChangeEntry{Path: p, Status: string(xy[0])})
				}
				if xy[1] != '.' {
					res.Unstaged = append(res.Unstaged, GitChangeEntry{Path: p, Status: string(xy[1])})
				}
			}
		case strings.HasPrefix(line, "2 "):
			// 2 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <X><score> <path>\t<origPath>
			fields := strings.SplitN(line, " ", 10)
			if len(fields) == 10 && len(fields[1]) == 2 {
				xy := fields[1]
				paths := strings.SplitN(fields[9], "\t", 2)
				p := paths[0]
				orig := ""
				if len(paths) == 2 {
					orig = paths[1]
				}
				if xy[0] != '.' {
					res.Staged = append(res.Staged, GitChangeEntry{Path: p, OrigPath: orig, Status: string(xy[0])})
				}
				if xy[1] != '.' {
					res.Unstaged = append(res.Unstaged, GitChangeEntry{Path: p, OrigPath: orig, Status: string(xy[1])})
				}
			}
		case strings.HasPrefix(line, "u "):
			// Unmerged entry: u <XY> <sub> <m1> <m2> <m3> <mW> <h1> <h2> <h3> <path>
			fields := strings.SplitN(line, " ", 11)
			if len(fields) == 11 {
				res.Unstaged = append(res.Unstaged, GitChangeEntry{Path: fields[10], Status: "U"})
			}
		case strings.HasPrefix(line, "? "):
			p := strings.TrimPrefix(line, "? ")
			if p != "" {
				res.Untracked = append(res.Untracked, GitChangeEntry{Path: p, Status: "?"})
			}
		}
	}

	res.Clean = len(res.Staged) == 0 && len(res.Unstaged) == 0 && len(res.Untracked) == 0
	return res
}

func countDiffLines(diff string) (additions, deletions int) {
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "@@") {
			continue
		}
		if strings.HasPrefix(line, "+") {
			additions++
		} else if strings.HasPrefix(line, "-") {
			deletions++
		}
	}
	return additions, deletions
}
