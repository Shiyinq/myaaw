package service

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// GitActionResult returns human-readable CLI output from a Git action.
type GitActionResult struct {
	Output string `json:"output,omitempty"`
}

// GitBranchEntry describes one local branch in the repository.
type GitBranchEntry struct {
	Name     string `json:"name"`
	Current  bool   `json:"current"`
	Upstream string `json:"upstream,omitempty"`
	Subject  string `json:"subject,omitempty"`
}

// GitBranchesResponse lists local and remote branches for the branch switcher.
type GitBranchesResponse struct {
	Current  string           `json:"current,omitempty"`
	Detached bool             `json:"detached,omitempty"`
	Branches []GitBranchEntry `json:"branches"`
	Remotes  []string         `json:"remotes"`
}

// GitStashEntry describes one entry in `git stash list`.
type GitStashEntry struct {
	Ref     string `json:"ref"`
	Message string `json:"message"`
	Date    string `json:"date"`
}

// GitCommitFileEntry describes one file touched by a historical commit.
type GitCommitFileEntry struct {
	Path      string `json:"path"`
	OrigPath  string `json:"orig_path,omitempty"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Binary    bool   `json:"binary,omitempty"`
}

// GitCommitDetailResponse returns full commit metadata and changed files.
type GitCommitDetailResponse struct {
	Commit GitCommitEntry       `json:"commit"`
	Body   string               `json:"body,omitempty"`
	Files  []GitCommitFileEntry `json:"files"`
}

// GitMultiDiffResponse aggregates multiple file diffs for the VS Code-style
// "Open All Changes" multi-diff editor view.
type GitMultiDiffResponse struct {
	Scope     string            `json:"scope"`
	Commit    string            `json:"commit,omitempty"`
	Files     []GitDiffResponse `json:"files"`
	Total     int               `json:"total"`
	Truncated bool              `json:"truncated,omitempty"`
}

const (
	maxMultiDiffFiles      = 80
	maxMultiDiffTotalBytes = 2 << 20 // 2 MiB cumulative cap across all files
)

// Stage adds specific paths (or all changes when all == true) to the Git index.
func (s *GitService) Stage(agentName, convID string, userID int, paths []string, all bool) error {
	dir, err := s.resolveGitDir(agentName, convID, userID)
	if err != nil {
		return err
	}
	unlock := s.lockRepo(dir)
	defer unlock()

	if all {
		_, err = runGitWrite(dir, "", gitCommandTimeout, "add", "-A")
		return err
	}
	cleanPaths, err := validateGitBatchPaths(dir, paths)
	if err != nil {
		return err
	}
	args := append([]string{"add", "--"}, cleanPaths...)
	_, err = runGitWrite(dir, "", gitCommandTimeout, args...)
	return err
}

// Unstage removes specific paths (or all staged paths when all == true) from the index.
func (s *GitService) Unstage(agentName, convID string, userID int, paths []string, all bool) error {
	dir, err := s.resolveGitDir(agentName, convID, userID)
	if err != nil {
		return err
	}
	unlock := s.lockRepo(dir)
	defer unlock()

	unborn := hasUnbornHead(dir)
	if all {
		if unborn {
			_, err = runGitWrite(dir, "", gitCommandTimeout, "rm", "--cached", "-r", "--quiet", ".")
			return err
		}
		_, err = runGitWrite(dir, "", gitCommandTimeout, "reset", "HEAD", "--")
		return err
	}
	cleanPaths, err := validateGitBatchPaths(dir, paths)
	if err != nil {
		return err
	}
	if unborn {
		args := append([]string{"rm", "--cached", "-r", "--quiet", "--"}, cleanPaths...)
		_, err = runGitWrite(dir, "", gitCommandTimeout, args...)
		return err
	}
	args := append([]string{"reset", "HEAD", "--"}, cleanPaths...)
	_, err = runGitWrite(dir, "", gitCommandTimeout, args...)
	return err
}

// Discard reverts unstaged working-tree edits or removes explicitly specified
// untracked files. It never touches .gitignore'd files (such as .env).
func (s *GitService) Discard(agentName, convID string, userID int, paths []string, all, includeUntracked bool) error {
	dir, err := s.resolveGitDir(agentName, convID, userID)
	if err != nil {
		return err
	}
	unlock := s.lockRepo(dir)
	defer unlock()

	if all {
		if _, err := runGitWrite(dir, "", gitCommandTimeout, "checkout", "--", "."); err != nil {
			// Fresh repos with no tracked files can report pathspec mismatch;
			// ignore only when HEAD does not exist yet.
			if !hasUnbornHead(dir) {
				return err
			}
		}
		if includeUntracked {
			// Only remove non-ignored untracked files (--exclude-standard).
			out, err := runGit(dir, "ls-files", "--others", "--exclude-standard", "-z")
			if err != nil {
				return err
			}
			for _, raw := range strings.Split(out, "\x00") {
				if raw == "" {
					continue
				}
				_, abs, pathErr := validateGitRelPath(dir, raw)
				if pathErr != nil {
					continue
				}
				if rmErr := os.RemoveAll(abs); rmErr != nil && !os.IsNotExist(rmErr) {
					return rmErr
				}
			}
		}
		return nil
	}

	if len(paths) == 0 || len(paths) > maxGitBatchPaths {
		return fmt.Errorf("%w: paths are required", ErrInvalidGitArg)
	}

	var tracked []string
	var untrackedAbs []string
	for _, raw := range paths {
		rel, abs, err := validateGitRelPath(dir, raw)
		if err != nil {
			return err
		}
		statusOut, err := runGit(dir, "status", "--porcelain=v1", "--", rel)
		if err != nil {
			return err
		}
		trimmed := strings.TrimRight(statusOut, "\n")
		if strings.HasPrefix(trimmed, "?? ") {
			untrackedAbs = append(untrackedAbs, abs)
		} else {
			tracked = append(tracked, rel)
		}
	}

	if len(tracked) > 0 {
		args := append([]string{"checkout", "--"}, tracked...)
		if _, err := runGitWrite(dir, "", gitCommandTimeout, args...); err != nil {
			return err
		}
	}
	for _, abs := range untrackedAbs {
		if err := os.RemoveAll(abs); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// Commit creates a commit (or amends HEAD when amend == true) using stdin so
// arbitrary commit messages can never be parsed as CLI flags.
func (s *GitService) Commit(agentName, convID string, userID int, message string, amend bool) (GitCommitEntry, error) {
	dir, err := s.resolveGitDir(agentName, convID, userID)
	if err != nil {
		return GitCommitEntry{}, err
	}
	msg, err := validateGitMessage(message, true)
	if err != nil {
		return GitCommitEntry{}, err
	}

	unlock := s.lockRepo(dir)
	defer unlock()

	args := []string{"commit", "--file=-"}
	if amend {
		args = append(args, "--amend")
	}
	if _, err := runGitWrite(dir, msg+"\n", gitCommandTimeout, args...); err != nil {
		return GitCommitEntry{}, err
	}

	out, err := runGit(dir, "log", "-n1", "--format=%H%x1f%h%x1f%s%x1f%an%x1f%aI")
	if err != nil {
		return GitCommitEntry{}, err
	}
	parts := strings.Split(strings.TrimSpace(out), "\x1f")
	if len(parts) < 5 {
		return GitCommitEntry{}, nil
	}
	return GitCommitEntry{
		Hash:      parts[0],
		ShortHash: parts[1],
		Subject:   parts[2],
		Author:    parts[3],
		Date:      parts[4],
	}, nil
}

// Branches returns local branches and remote tracking branch names.
func (s *GitService) Branches(agentName, convID string, userID int) (GitBranchesResponse, error) {
	dir, err := s.resolveGitDir(agentName, convID, userID)
	if err != nil {
		return GitBranchesResponse{}, err
	}

	statusOut, err := runGit(dir, "status", "--porcelain=v2", "--branch")
	if err != nil {
		return GitBranchesResponse{}, err
	}
	st := parseGitStatusV2(statusOut)

	resp := GitBranchesResponse{
		Current:  st.Branch,
		Detached: st.Detached,
		Branches: []GitBranchEntry{},
		Remotes:  []string{},
	}

	localOut, err := runGit(dir, "for-each-ref",
		"--sort=-committerdate",
		"--format=%(HEAD)%09%(refname:short)%09%(upstream:short)%09%(contents:subject)",
		"refs/heads",
	)
	if err != nil {
		return GitBranchesResponse{}, err
	}
	for _, line := range strings.Split(strings.TrimRight(localOut, "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) < 2 {
			continue
		}
		entry := GitBranchEntry{
			Current: strings.TrimSpace(parts[0]) == "*",
			Name:    parts[1],
		}
		if len(parts) > 2 {
			entry.Upstream = parts[2]
		}
		if len(parts) > 3 {
			entry.Subject = parts[3]
		}
		resp.Branches = append(resp.Branches, entry)
	}

	remoteOut, err := runGit(dir, "for-each-ref", "--format=%(refname:short)", "refs/remotes")
	if err == nil {
		for _, line := range strings.Split(strings.TrimRight(remoteOut, "\n"), "\n") {
			name := strings.TrimSpace(line)
			if name == "" || strings.HasSuffix(name, "/HEAD") {
				continue
			}
			resp.Remotes = append(resp.Remotes, name)
		}
	}

	return resp, nil
}

// Checkout switches to an existing branch or creates and checks out a new one,
// optionally starting from a validated base branch, remote branch, or commit hash.
func (s *GitService) Checkout(agentName, convID string, userID int, branch string, create bool, fromRef string) (GitStatusResponse, error) {
	dir, err := s.resolveGitDir(agentName, convID, userID)
	if err != nil {
		return GitStatusResponse{}, err
	}
	name, err := validateBranchName(branch)
	if err != nil {
		return GitStatusResponse{}, err
	}

	var startPoint string
	if create && strings.TrimSpace(fromRef) != "" {
		startPoint, err = validateBranchName(fromRef)
		if err != nil {
			return GitStatusResponse{}, fmt.Errorf("%w: invalid base ref", ErrInvalidGitArg)
		}
	}

	unlock := s.lockRepo(dir)
	defer unlock()

	if create {
		args := []string{"checkout", "-b", name}
		if startPoint != "" {
			args = append(args, startPoint)
		}
		if _, err := runGitWrite(dir, "", gitCommandTimeout, args...); err != nil {
			return GitStatusResponse{}, err
		}
	} else {
		if _, err := runGitWrite(dir, "", gitCommandTimeout, "checkout", name); err != nil {
			return GitStatusResponse{}, err
		}
	}

	out, err := runGit(dir, "status", "--porcelain=v2", "--branch", "-uall")
	if err != nil {
		return GitStatusResponse{}, err
	}
	return parseGitStatusV2(out), nil
}

// Fetch fetches remote refs without interactive prompts.
func (s *GitService) Fetch(agentName, convID string, userID int) (GitActionResult, error) {
	dir, err := s.resolveGitDir(agentName, convID, userID)
	if err != nil {
		return GitActionResult{}, err
	}
	unlock := s.lockRepo(dir)
	defer unlock()

	out, err := runGitWrite(dir, "", gitNetworkTimeout, "fetch", "--prune")
	if err != nil {
		return GitActionResult{}, err
	}
	return GitActionResult{Output: strings.TrimSpace(out)}, nil
}

// Pull fast-forwards the current branch from its upstream without opening an editor.
func (s *GitService) Pull(agentName, convID string, userID int) (GitActionResult, error) {
	dir, err := s.resolveGitDir(agentName, convID, userID)
	if err != nil {
		return GitActionResult{}, err
	}
	unlock := s.lockRepo(dir)
	defer unlock()

	out, err := runGitWrite(dir, "", gitNetworkTimeout, "pull", "--ff-only")
	if err != nil {
		return GitActionResult{}, err
	}
	return GitActionResult{Output: strings.TrimSpace(out)}, nil
}

// Push pushes the current branch to its configured upstream (or origin when
// setUpstream is true and no upstream is configured yet).
func (s *GitService) Push(agentName, convID string, userID int, setUpstream bool) (GitActionResult, error) {
	dir, err := s.resolveGitDir(agentName, convID, userID)
	if err != nil {
		return GitActionResult{}, err
	}
	unlock := s.lockRepo(dir)
	defer unlock()

	args := []string{"push"}
	if setUpstream {
		statusOut, err := runGit(dir, "status", "--porcelain=v2", "--branch")
		if err != nil {
			return GitActionResult{}, err
		}
		st := parseGitStatusV2(statusOut)
		if st.Upstream == "" && !st.Detached && st.Branch != "" {
			branch, err := validateBranchName(st.Branch)
			if err != nil {
				return GitActionResult{}, err
			}
			args = []string{"push", "-u", "origin", branch}
		}
	}

	out, err := runGitWrite(dir, "", gitNetworkTimeout, args...)
	if err != nil {
		return GitActionResult{}, err
	}
	return GitActionResult{Output: strings.TrimSpace(out)}, nil
}

// Stashes lists stashed changes in the repository.
func (s *GitService) Stashes(agentName, convID string, userID int) ([]GitStashEntry, error) {
	dir, err := s.resolveGitDir(agentName, convID, userID)
	if err != nil {
		return nil, err
	}

	out, err := runGit(dir, "stash", "list", "--format=%gd%x1f%gs%x1f%aI")
	if err != nil {
		return nil, err
	}
	entries := []GitStashEntry{}
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "\x1f", 3)
		if len(parts) < 2 {
			continue
		}
		date := ""
		if len(parts) > 2 {
			date = parts[2]
		}
		entries = append(entries, GitStashEntry{
			Ref:     parts[0],
			Message: parts[1],
			Date:    date,
		})
	}
	return entries, nil
}

// StashSave creates a new stash entry. Never uses --all so ignored files (.env)
// are never stashed or deleted.
func (s *GitService) StashSave(agentName, convID string, userID int, message string, includeUntracked bool) (GitActionResult, error) {
	dir, err := s.resolveGitDir(agentName, convID, userID)
	if err != nil {
		return GitActionResult{}, err
	}
	msg, err := validateGitMessage(message, false)
	if err != nil {
		return GitActionResult{}, err
	}

	unlock := s.lockRepo(dir)
	defer unlock()

	args := []string{"stash", "push"}
	if includeUntracked {
		args = append(args, "-u")
	}
	if msg != "" {
		args = append(args, "-m", msg)
	}
	out, err := runGitWrite(dir, "", gitCommandTimeout, args...)
	if err != nil {
		return GitActionResult{}, err
	}
	return GitActionResult{Output: strings.TrimSpace(out)}, nil
}

// StashApply applies a validated stash ref without removing it.
func (s *GitService) StashApply(agentName, convID string, userID int, ref string) (GitActionResult, error) {
	return s.runStashOp(agentName, convID, userID, "apply", ref)
}

// StashPop applies and removes a validated stash ref.
func (s *GitService) StashPop(agentName, convID string, userID int, ref string) (GitActionResult, error) {
	return s.runStashOp(agentName, convID, userID, "pop", ref)
}

// StashDrop permanently drops a validated stash ref.
func (s *GitService) StashDrop(agentName, convID string, userID int, ref string) (GitActionResult, error) {
	return s.runStashOp(agentName, convID, userID, "drop", ref)
}

func (s *GitService) runStashOp(agentName, convID string, userID int, op, ref string) (GitActionResult, error) {
	dir, err := s.resolveGitDir(agentName, convID, userID)
	if err != nil {
		return GitActionResult{}, err
	}
	validRef, err := validateStashRef(ref)
	if err != nil {
		return GitActionResult{}, err
	}

	unlock := s.lockRepo(dir)
	defer unlock()

	out, err := runGitWrite(dir, "", gitCommandTimeout, "stash", op, validRef)
	if err != nil {
		return GitActionResult{}, err
	}
	return GitActionResult{Output: strings.TrimSpace(out)}, nil
}

// CommitDetail returns metadata and changed files for a single commit hash.
func (s *GitService) CommitDetail(agentName, convID string, userID int, hash string) (GitCommitDetailResponse, error) {
	dir, err := s.resolveGitDir(agentName, convID, userID)
	if err != nil {
		return GitCommitDetailResponse{}, err
	}
	validHash, err := validateCommitHash(hash)
	if err != nil {
		return GitCommitDetailResponse{}, err
	}

	metaOut, err := runGit(dir, "show", "--no-patch", "--format=%H%x1f%h%x1f%s%x1f%an%x1f%aI%x1f%b", validHash)
	if err != nil {
		return GitCommitDetailResponse{}, err
	}
	parts := strings.SplitN(strings.TrimRight(metaOut, "\n"), "\x1f", 6)
	if len(parts) < 5 {
		return GitCommitDetailResponse{}, ErrPathNotFound
	}
	body := ""
	if len(parts) > 5 {
		body = strings.TrimSpace(parts[5])
	}
	commit := GitCommitEntry{
		Hash:      parts[0],
		ShortHash: parts[1],
		Subject:   parts[2],
		Author:    parts[3],
		Date:      parts[4],
	}

	statusOut, err := runGit(dir, "diff-tree", "--no-commit-id", "--name-status", "-r", "--root", "-M", validHash)
	if err != nil {
		return GitCommitDetailResponse{}, err
	}
	numstatOut, _ := runGit(dir, "diff-tree", "--no-commit-id", "--numstat", "-r", "--root", "-M", validHash)

	type statInfo struct {
		add    int
		del    int
		binary bool
	}
	stats := make(map[string]statInfo)
	for _, line := range strings.Split(strings.TrimRight(numstatOut, "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) < 3 {
			continue
		}
		p := cols[len(cols)-1]
		if cols[0] == "-" || cols[1] == "-" {
			stats[p] = statInfo{binary: true}
			continue
		}
		add, _ := strconv.Atoi(cols[0])
		del, _ := strconv.Atoi(cols[1])
		stats[p] = statInfo{add: add, del: del}
	}

	files := []GitCommitFileEntry{}
	for _, line := range strings.Split(strings.TrimRight(statusOut, "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) < 2 {
			continue
		}
		code := cols[0]
		if len(code) > 1 {
			code = code[:1]
		}
		entry := GitCommitFileEntry{Status: code}
		if (code == "R" || code == "C") && len(cols) >= 3 {
			entry.OrigPath = cols[1]
			entry.Path = cols[2]
		} else {
			entry.Path = cols[1]
		}
		if st, ok := stats[entry.Path]; ok {
			entry.Additions = st.add
			entry.Deletions = st.del
			entry.Binary = st.binary
		}
		files = append(files, entry)
	}

	return GitCommitDetailResponse{
		Commit: commit,
		Body:   body,
		Files:  files,
	}, nil
}

// CommitDiff returns the unified diff for one file within a historical commit.
func (s *GitService) CommitDiff(agentName, convID string, userID int, hash, relPath string) (GitDiffResponse, error) {
	dir, err := s.resolveGitDir(agentName, convID, userID)
	if err != nil {
		return GitDiffResponse{}, err
	}
	validHash, err := validateCommitHash(hash)
	if err != nil {
		return GitDiffResponse{}, err
	}
	rel, _, err := validateGitRelPath(dir, relPath)
	if err != nil {
		return GitDiffResponse{}, err
	}

	out, err := runGit(dir, "show", "--no-color", "--unified=3", "--format=", validHash, "--", rel)
	if err != nil {
		return GitDiffResponse{}, err
	}
	resp := GitDiffResponse{
		Path:   rel,
		Commit: validHash,
	}
	return finalizeDiffResponse(resp, out), nil
}

// DiffAll aggregates diffs for a scope ("changes", "staged", "all") or a
// specific commit hash, capped safely by file count and cumulative bytes.
func (s *GitService) DiffAll(agentName, convID string, userID int, scope, commitHash string) (GitMultiDiffResponse, error) {
	dir, err := s.resolveGitDir(agentName, convID, userID)
	if err != nil {
		return GitMultiDiffResponse{}, err
	}

	if strings.TrimSpace(commitHash) != "" {
		validHash, err := validateCommitHash(commitHash)
		if err != nil {
			return GitMultiDiffResponse{}, err
		}
		detail, err := s.CommitDetail(agentName, convID, userID, validHash)
		if err != nil {
			return GitMultiDiffResponse{}, err
		}
		res := GitMultiDiffResponse{
			Scope:  "commit",
			Commit: validHash,
			Files:  make([]GitDiffResponse, 0, min(len(detail.Files), maxMultiDiffFiles)),
			Total:  len(detail.Files),
		}
		totalBytes := 0
		for i, f := range detail.Files {
			if i >= maxMultiDiffFiles || totalBytes >= maxMultiDiffTotalBytes {
				res.Truncated = true
				break
			}
			rel, _, pathErr := validateGitRelPath(dir, f.Path)
			if pathErr != nil {
				continue
			}
			out, diffErr := runGit(dir, "show", "--no-color", "--unified=3", "--format=", validHash, "--", rel)
			if diffErr != nil {
				continue
			}
			entry := finalizeDiffResponse(GitDiffResponse{Path: rel, Commit: validHash}, out)
			totalBytes += len(entry.Diff)
			res.Files = append(res.Files, entry)
		}
		return res, nil
	}

	normScope := strings.ToLower(strings.TrimSpace(scope))
	if normScope == "" {
		normScope = "changes"
	}
	switch normScope {
	case "changes", "staged", "all":
	default:
		return GitMultiDiffResponse{}, fmt.Errorf("%w: unsupported diff scope", ErrInvalidGitArg)
	}

	statusOut, err := runGit(dir, "status", "--porcelain=v2", "--branch", "-uall")
	if err != nil {
		return GitMultiDiffResponse{}, err
	}
	st := parseGitStatusV2(statusOut)

	type diffTarget struct {
		path      string
		staged    bool
		untracked bool
	}
	var targets []diffTarget
	if normScope == "staged" || normScope == "all" {
		for _, item := range st.Staged {
			targets = append(targets, diffTarget{path: item.Path, staged: true})
		}
	}
	if normScope == "changes" || normScope == "all" {
		for _, item := range st.Unstaged {
			targets = append(targets, diffTarget{path: item.Path, staged: false})
		}
		for _, item := range st.Untracked {
			targets = append(targets, diffTarget{path: item.Path, untracked: true})
		}
	}

	res := GitMultiDiffResponse{
		Scope: normScope,
		Files: make([]GitDiffResponse, 0, min(len(targets), maxMultiDiffFiles)),
		Total: len(targets),
	}
	totalBytes := 0
	for i, target := range targets {
		if i >= maxMultiDiffFiles || totalBytes >= maxMultiDiffTotalBytes {
			res.Truncated = true
			break
		}
		rel, abs, pathErr := validateGitRelPath(dir, target.path)
		if pathErr != nil {
			continue
		}
		var entry GitDiffResponse
		if target.untracked {
			uEntry, uErr := buildUntrackedDiff(abs, GitDiffResponse{Path: rel, Untracked: true})
			if uErr != nil {
				continue
			}
			entry = uEntry
		} else {
			args := []string{"diff", "--no-color", "--unified=3"}
			if target.staged {
				args = append(args, "--cached")
			}
			args = append(args, "--", rel)
			out, dErr := runGit(dir, args...)
			if dErr != nil {
				continue
			}
			entry = finalizeDiffResponse(GitDiffResponse{Path: rel, Staged: target.staged}, out)
		}
		totalBytes += len(entry.Diff)
		res.Files = append(res.Files, entry)
	}
	return res, nil
}

func validateGitBatchPaths(dir string, paths []string) ([]string, error) {
	if len(paths) == 0 || len(paths) > maxGitBatchPaths {
		return nil, fmt.Errorf("%w: paths are required", ErrInvalidGitArg)
	}
	clean := make([]string, 0, len(paths))
	for _, raw := range paths {
		rel, _, err := validateGitRelPath(dir, raw)
		if err != nil {
			return nil, err
		}
		clean = append(clean, rel)
	}
	return clean, nil
}

func hasUnbornHead(dir string) bool {
	_, err := runGit(dir, "rev-parse", "--verify", "HEAD")
	return err != nil
}
