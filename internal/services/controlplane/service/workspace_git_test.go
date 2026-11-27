package service

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"myaaw/internal/agentdir"
)

func setupTestAgentForWorkspace(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := agentdir.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}
	agentName := "default"
	cfgDir, err := agentdir.ConfigDir(agentName)
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write config.json: %v", err)
	}
	homeDir, err := agentdir.Home(agentName)
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(homeDir, "workspace"), 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	return home, agentName
}

func TestWorkspaceBrowseAndMkdir(t *testing.T) {
	home, agentName := setupTestAgentForWorkspace(t)

	projectsDir := filepath.Join(home, "Projects")
	if err := os.MkdirAll(filepath.Join(projectsDir, "existing-empty"), 0o755); err != nil {
		t.Fatalf("mkdir existing-empty: %v", err)
	}
	gitProj := filepath.Join(projectsDir, "git-app")
	if err := os.MkdirAll(filepath.Join(gitProj, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir git-app: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitProj, "package.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	ws := NewWorkspaceService()
	roots, err := ws.Roots(agentName)
	if err != nil {
		t.Fatalf("Roots: %v", err)
	}
	if !roots.AllowHostBrowse || len(roots.Roots) == 0 {
		t.Fatalf("unexpected roots response: %+v", roots)
	}

	browsed, err := ws.Browse(agentName, projectsDir, false)
	if err != nil {
		t.Fatalf("Browse: %v", err)
	}
	if !browsed.CanAttach || len(browsed.Entries) != 2 {
		t.Fatalf("unexpected browse response: %+v", browsed)
	}
	if browsed.Entries[0].Name != "existing-empty" || !browsed.Entries[0].IsEmpty {
		t.Fatalf("expected existing-empty to be empty: %+v", browsed.Entries[0])
	}
	if browsed.Entries[1].Name != "git-app" || !browsed.Entries[1].IsGit {
		t.Fatalf("expected git-app to be git: %+v", browsed.Entries[1])
	}

	created, err := ws.Mkdir(agentName, projectsDir, "fresh-folder")
	if err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if info, err := os.Stat(created); err != nil || !info.IsDir() {
		t.Fatalf("created dir not found: %s (%v)", created, err)
	}
}

func TestConversationWorkingDirProjectFilesAndGit(t *testing.T) {
	home, agentName := setupTestAgentForWorkspace(t)

	repoDir := filepath.Join(home, "my-repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	gitRun := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repoDir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Tester",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Tester",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
		}
	}

	gitRun("init")
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	gitRun("add", "README.md")
	gitRun("commit", "-m", "initial commit")

	// Modify tracked file and add an untracked file.
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatalf("modify README: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "notes.txt"), []byte("new file\n"), 0o644); err != nil {
		t.Fatalf("write notes: %v", err)
	}

	convSvc := NewConversationService()
	created, err := convSvc.Create(agentName, 0, "Repo Chat", repoDir, "")
	if err != nil {
		t.Fatalf("Create conversation: %v", err)
	}
	if !created.WorkingDirExists || !created.HasGit {
		t.Fatalf("expected working dir exists & has_git: %+v", created)
	}

	// FileService scoped to /project
	fileSvc := NewFileService()
	projectRoot, err := convSvc.ResolveWorkingDir(agentName, created.ID, 0)
	if err != nil {
		t.Fatalf("ResolveWorkingDir: %v", err)
	}
	entries, err := fileSvc.ListInProject(agentName, "/project", projectRoot)
	if err != nil {
		t.Fatalf("ListInProject: %v", err)
	}
	foundReadme := false
	for _, e := range entries {
		if e.Name == "README.md" && e.Path == "/project/README.md" {
			foundReadme = true
		}
	}
	if !foundReadme {
		t.Fatalf("expected README.md in /project listing: %+v", entries)
	}
	content, err := fileSvc.ReadInProject(agentName, "/project/README.md", projectRoot)
	if err != nil || !strings.Contains(content.Content, "world") {
		t.Fatalf("ReadInProject README.md = %+v, err=%v", content, err)
	}

	// GitService status, log, diff
	gitSvc := NewGitService(convSvc)
	status, err := gitSvc.Status(agentName, created.ID, 0)
	if err != nil {
		t.Fatalf("Git Status: %v", err)
	}
	if !status.HasGit || status.Clean || len(status.Unstaged) != 1 || len(status.Untracked) != 1 {
		t.Fatalf("unexpected git status: %+v", status)
	}

	commits, err := gitSvc.Log(agentName, created.ID, 0, 10)
	if err != nil || len(commits) != 1 || commits[0].Subject != "initial commit" {
		t.Fatalf("unexpected git log: %+v, err=%v", commits, err)
	}

	diff, err := gitSvc.Diff(agentName, created.ID, 0, "README.md", false, false)
	if err != nil || diff.Additions != 1 || !strings.Contains(diff.Diff, "+world") {
		t.Fatalf("unexpected git diff: %+v, err=%v", diff, err)
	}

	untrackedDiff, err := gitSvc.Diff(agentName, created.ID, 0, "notes.txt", false, true)
	if err != nil || !untrackedDiff.Untracked || !strings.Contains(untrackedDiff.Diff, "+new file") {
		t.Fatalf("unexpected untracked diff: %+v, err=%v", untrackedDiff, err)
	}
}

func TestGitSourceControlActionsAndSecurity(t *testing.T) {
	home, agentName := setupTestAgentForWorkspace(t)

	// Set up a bare local remote and a working repository.
	remoteDir := filepath.Join(home, "remote.git")
	repoDir := filepath.Join(home, "sc-repo")
	if err := os.MkdirAll(remoteDir, 0o755); err != nil {
		t.Fatalf("mkdir remote: %v", err)
	}
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	gitRunDir := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Tester",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Tester",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
		}
	}

	gitRunDir(remoteDir, "init", "--bare")
	gitRunDir(repoDir, "init")
	gitRunDir(repoDir, "config", "user.name", "Tester")
	gitRunDir(repoDir, "config", "user.email", "test@example.com")
	gitRunDir(repoDir, "remote", "add", "origin", remoteDir)

	if err := os.WriteFile(filepath.Join(repoDir, ".gitignore"), []byte("secret.local\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "app.txt"), []byte("v1\n"), 0o644); err != nil {
		t.Fatalf("write app.txt: %v", err)
	}
	gitRunDir(repoDir, "add", ".gitignore", "app.txt")
	gitRunDir(repoDir, "commit", "-m", "init")

	// Create an ignored secret file to verify discard/stash NEVER deletes ignored files.
	ignoredSecret := filepath.Join(repoDir, "secret.local")
	if err := os.WriteFile(ignoredSecret, []byte("TOP_SECRET=1\n"), 0o600); err != nil {
		t.Fatalf("write ignored secret: %v", err)
	}

	convSvc := NewConversationService()
	created, err := convSvc.Create(agentName, 0, "SC Chat", repoDir, "")
	if err != nil {
		t.Fatalf("Create conversation: %v", err)
	}
	gitSvc := NewGitService(convSvc)
	fileSvc := NewFileService()

	// Security: FileService must block tampering with /project/.git
	if _, err := fileSvc.WriteInProject(agentName, "/project/.git/config", "bad", repoDir); err == nil {
		t.Fatal("expected writing /project/.git/config to fail")
	}
	if err := fileSvc.DeleteInProject(agentName, "/project/.git", true, repoDir); err == nil {
		t.Fatal("expected deleting /project/.git to fail")
	}
	if err := fileSvc.RenameInProject(agentName, "/project/app.txt", "/home/escape.txt", repoDir); err == nil {
		t.Fatal("expected cross-root rename to fail")
	}

	// Security: GitService must reject flag injection, traversal, bad refs/hashes
	if _, err := gitSvc.Checkout(agentName, created.ID, 0, "--upload-pack=evil", false, ""); err == nil {
		t.Fatal("expected leading-dash branch name to be rejected")
	}
	if _, err := gitSvc.Checkout(agentName, created.ID, 0, "valid-name", true, "--track"); err == nil {
		t.Fatal("expected leading-dash base ref to be rejected")
	}
	if err := gitSvc.Stage(agentName, created.ID, 0, []string{"../escape.txt"}, false); err == nil {
		t.Fatal("expected traversal path in Stage to be rejected")
	}
	if err := gitSvc.Stage(agentName, created.ID, 0, []string{".git/config"}, false); err == nil {
		t.Fatal("expected .git path in Stage to be rejected")
	}
	if _, err := gitSvc.CommitDetail(agentName, created.ID, 0, "--all"); err == nil {
		t.Fatal("expected invalid commit hash to be rejected")
	}
	if _, err := gitSvc.StashApply(agentName, created.ID, 0, "--invalid"); err == nil {
		t.Fatal("expected invalid stash ref to be rejected")
	}
	if got := sanitizeGitOutput("fatal: https://user:ghp_secret123@github.com/org/repo.git failed"); strings.Contains(got, "ghp_secret123") {
		t.Fatalf("expected credentials scrubbed, got %q", got)
	}

	// Stage, Unstage, Commit
	if err := os.WriteFile(filepath.Join(repoDir, "app.txt"), []byte("v2\n"), 0o644); err != nil {
		t.Fatalf("modify app.txt: %v", err)
	}
	if err := gitSvc.Stage(agentName, created.ID, 0, []string{"app.txt"}, false); err != nil {
		t.Fatalf("Stage: %v", err)
	}
	st, err := gitSvc.Status(agentName, created.ID, 0)
	if err != nil || len(st.Staged) != 1 || len(st.Unstaged) != 0 {
		t.Fatalf("expected 1 staged after Stage: %+v (err=%v)", st, err)
	}
	if err := gitSvc.Unstage(agentName, created.ID, 0, []string{"app.txt"}, false); err != nil {
		t.Fatalf("Unstage: %v", err)
	}
	st, _ = gitSvc.Status(agentName, created.ID, 0)
	if len(st.Staged) != 0 || len(st.Unstaged) != 1 {
		t.Fatalf("expected 1 unstaged after Unstage: %+v", st)
	}
	if err := gitSvc.Stage(agentName, created.ID, 0, nil, true); err != nil {
		t.Fatalf("Stage all: %v", err)
	}
	commit, err := gitSvc.Commit(agentName, created.ID, 0, "-update app to v2", false)
	if err != nil || commit.Hash == "" || commit.Subject != "-update app to v2" {
		t.Fatalf("Commit failed: %+v (err=%v)", commit, err)
	}

	// CommitDetail, CommitDiff, LogPage pagination, and DiffAll
	detail, err := gitSvc.CommitDetail(agentName, created.ID, 0, commit.Hash)
	if err != nil || len(detail.Files) != 1 || detail.Files[0].Path != "app.txt" || detail.Files[0].Additions != 1 {
		t.Fatalf("CommitDetail unexpected: %+v (err=%v)", detail, err)
	}
	cdiff, err := gitSvc.CommitDiff(agentName, created.ID, 0, commit.Hash, "app.txt")
	if err != nil || !strings.Contains(cdiff.Diff, "+v2") {
		t.Fatalf("CommitDiff unexpected: %+v (err=%v)", cdiff, err)
	}
	page1, err := gitSvc.LogPage(agentName, created.ID, 0, 1, 0)
	if err != nil || len(page1.Commits) != 1 || !page1.HasMore {
		t.Fatalf("LogPage page1 unexpected: %+v (err=%v)", page1, err)
	}
	page2, err := gitSvc.LogPage(agentName, created.ID, 0, 1, 1)
	if err != nil || len(page2.Commits) != 1 || page2.HasMore {
		t.Fatalf("LogPage page2 unexpected: %+v (err=%v)", page2, err)
	}
	commitMulti, err := gitSvc.DiffAll(agentName, created.ID, 0, "", commit.Hash)
	if err != nil || len(commitMulti.Files) != 1 || commitMulti.Files[0].Path != "app.txt" {
		t.Fatalf("DiffAll commit unexpected: %+v (err=%v)", commitMulti, err)
	}

	// Push (with setUpstream), Fetch, Pull
	if _, err := gitSvc.Push(agentName, created.ID, 0, true); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if _, err := gitSvc.Fetch(agentName, created.ID, 0); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if _, err := gitSvc.Pull(agentName, created.ID, 0); err != nil {
		t.Fatalf("Pull: %v", err)
	}

	// Branches & Checkout (including create-from base ref)
	if _, err := gitSvc.Checkout(agentName, created.ID, 0, "feat/sc-test", true, ""); err != nil {
		t.Fatalf("Checkout create: %v", err)
	}
	branches, err := gitSvc.Branches(agentName, created.ID, 0)
	if err != nil || branches.Current != "feat/sc-test" || len(branches.Branches) < 2 {
		t.Fatalf("Branches unexpected: %+v (err=%v)", branches, err)
	}
	if _, err := gitSvc.Checkout(agentName, created.ID, 0, "feat/from-base", true, branches.Branches[len(branches.Branches)-1].Name); err != nil {
		t.Fatalf("Checkout create-from: %v", err)
	}
	if _, err := gitSvc.Checkout(agentName, created.ID, 0, "feat/sc-test", false, ""); err != nil {
		t.Fatalf("Checkout switch back: %v", err)
	}

	// Stash Save, List, Pop (ignored secret must stay intact!)
	if err := os.WriteFile(filepath.Join(repoDir, "app.txt"), []byte("v3-wip\n"), 0o644); err != nil {
		t.Fatalf("modify app.txt for stash: %v", err)
	}
	if _, err := gitSvc.StashSave(agentName, created.ID, 0, "wip changes", true); err != nil {
		t.Fatalf("StashSave: %v", err)
	}
	stashes, err := gitSvc.Stashes(agentName, created.ID, 0)
	if err != nil || len(stashes) != 1 {
		t.Fatalf("Stashes unexpected: %+v (err=%v)", stashes, err)
	}
	if _, err := os.Stat(ignoredSecret); err != nil {
		t.Fatalf("ignored secret file was touched by stash: %v", err)
	}
	if _, err := gitSvc.StashPop(agentName, created.ID, 0, stashes[0].Ref); err != nil {
		t.Fatalf("StashPop: %v", err)
	}

	// Discard tracked + untracked must preserve ignored secret file!
	untracked := filepath.Join(repoDir, "temp-untracked.txt")
	if err := os.WriteFile(untracked, []byte("discard me\n"), 0o644); err != nil {
		t.Fatalf("write untracked: %v", err)
	}
	if err := gitSvc.Discard(agentName, created.ID, 0, nil, true, true); err != nil {
		t.Fatalf("Discard all: %v", err)
	}
	if _, err := os.Stat(untracked); !os.IsNotExist(err) {
		t.Fatalf("expected untracked file removed, err=%v", err)
	}
	if _, err := os.Stat(ignoredSecret); err != nil {
		t.Fatalf("ignored secret file must NOT be removed by discard: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(repoDir, "app.txt"))
	if string(data) != "v2\n" {
		t.Fatalf("expected app.txt reverted to v2, got %q", string(data))
	}
}

func TestConversationServiceWorkingDirLockedOnceAttached(t *testing.T) {
	home, agentName := setupTestAgentForWorkspace(t)
	t.Setenv("MYAAW_ALLOWED_WORKSPACES", home)

	dirA := filepath.Join(home, "proj-a")
	dirB := filepath.Join(home, "proj-b")
	for _, d := range []string{dirA, dirB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}

	convSvc := NewConversationService()
	created, err := convSvc.Create(agentName, 0, "Unattached Chat", "", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Attaching an initial folder to an unattached chat succeeds.
	wd, exists, _, err := convSvc.UpdateWorkingDir(agentName, created.ID, 0, dirA)
	if err != nil || wd == "" || !exists {
		t.Fatalf("initial UpdateWorkingDir failed: wd=%q exists=%v err=%v", wd, exists, err)
	}

	// Sending the same directory again is an idempotent no-op.
	if wd2, _, _, err := convSvc.UpdateWorkingDir(agentName, created.ID, 0, dirA); err != nil || wd2 != wd {
		t.Fatalf("idempotent UpdateWorkingDir failed: wd2=%q err=%v", wd2, err)
	}

	// Changing to another directory is rejected with ErrWorkingDirLocked.
	if _, _, _, err := convSvc.UpdateWorkingDir(agentName, created.ID, 0, dirB); !errors.Is(err, ErrWorkingDirLocked) {
		t.Fatalf("expected ErrWorkingDirLocked on change, got %v", err)
	}

	// Detaching (clearing to empty) is also rejected with ErrWorkingDirLocked.
	if _, _, _, err := convSvc.UpdateWorkingDir(agentName, created.ID, 0, ""); !errors.Is(err, ErrWorkingDirLocked) {
		t.Fatalf("expected ErrWorkingDirLocked on clear, got %v", err)
	}
}

func TestConversationServiceListProjectsAndFilterByWorkingDir(t *testing.T) {
	home, agentName := setupTestAgentForWorkspace(t)
	t.Setenv("MYAAW_ALLOWED_WORKSPACES", home)

	dirA := filepath.Join(home, "alpha-app")
	dirB := filepath.Join(home, "beta-api")
	for _, d := range []string{dirA, dirB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	// Mark alpha-app as a Git repo on branch feat/demo so ListProjects returns git_branch.
	if err := os.MkdirAll(filepath.Join(dirA, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirA, ".git", "HEAD"), []byte("ref: refs/heads/feat/demo\n"), 0o644); err != nil {
		t.Fatalf("write .git/HEAD: %v", err)
	}

	convSvc := NewConversationService()
	c1, err := convSvc.Create(agentName, 0, "Alpha 1", dirA, "")
	if err != nil {
		t.Fatalf("Create c1: %v", err)
	}
	c2, err := convSvc.Create(agentName, 0, "Alpha 2", dirA, "")
	if err != nil {
		t.Fatalf("Create c2: %v", err)
	}
	if _, err := convSvc.Create(agentName, 0, "Beta 1", dirB, ""); err != nil {
		t.Fatalf("Create beta: %v", err)
	}
	if _, err := convSvc.Create(agentName, 0, "General Chat", "", ""); err != nil {
		t.Fatalf("Create general: %v", err)
	}

	projects, err := convSvc.ListProjects(agentName, 0)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d (%+v)", len(projects), projects)
	}

	var alpha *ConversationProject
	for i := range projects {
		if projects[i].Name == "alpha-app" {
			alpha = &projects[i]
		}
	}
	if alpha == nil || alpha.ConversationCount != 2 || !alpha.Exists || !alpha.HasGit || alpha.GitBranch != "feat/demo" {
		t.Fatalf("unexpected alpha project: %+v", alpha)
	}
	if alpha.LatestConversationID != c2.ID && alpha.LatestConversationID != c1.ID {
		t.Fatalf("unexpected alpha latest conversation: %+v", alpha)
	}

	filtered, err := convSvc.List(agentName, 0, 50, 0, "", ConversationFilter{WorkingDir: alpha.WorkingDir})
	if err != nil {
		t.Fatalf("List filtered by working_dir: %v", err)
	}
	if filtered.Total != 2 || len(filtered.Conversations) != 2 {
		t.Fatalf("expected 2 filtered conversations for alpha-app, got total=%d len=%d", filtered.Total, len(filtered.Conversations))
	}

	// Verify ListProjectsPage pagination and search filter.
	page1, err := convSvc.ListProjectsPage(agentName, 0, 1, 0, "")
	if err != nil || page1.Total != 2 || len(page1.Projects) != 1 || !page1.HasMore {
		t.Fatalf("ListProjectsPage page1 unexpected: %+v (err=%v)", page1, err)
	}
	page2, err := convSvc.ListProjectsPage(agentName, 0, 1, 1, "")
	if err != nil || page2.Total != 2 || len(page2.Projects) != 1 || page2.HasMore {
		t.Fatalf("ListProjectsPage page2 unexpected: %+v (err=%v)", page2, err)
	}
	searched, err := convSvc.ListProjectsPage(agentName, 0, 10, 0, "beta")
	if err != nil || searched.Total != 1 || len(searched.Projects) != 1 || searched.Projects[0].Name != "beta-api" {
		t.Fatalf("ListProjectsPage search unexpected: %+v (err=%v)", searched, err)
	}
}
