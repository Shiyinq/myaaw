package filesystem

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myaaw/internal/agentdir"
)

func TestConfigureAllowedDirectoriesIncludesMyaawDirs(t *testing.T) {
	agentRoot := t.TempDir()
	cwd := t.TempDir()

	restore := saveAllowedConfig()
	t.Cleanup(restore)

	projectRoot := configureAllowedDirectories(agentRoot, cwd)
	if projectRoot != "" {
		t.Fatalf("projectRoot = %q, want empty", projectRoot)
	}

	wantBase := filepath.Join(agentRoot, "home")
	if defaultBaseDir != wantBase {
		t.Errorf("defaultBaseDir = %q, want %q", defaultBaseDir, wantBase)
	}

	for _, want := range []string{
		filepath.Join(agentRoot, "home"),
		filepath.Join(agentRoot, "skills"),
		filepath.Join(agentRoot, "jobs"),
	} {
		if !containsPath(allowedDirectories, want) {
			t.Errorf("allowedDirectories missing %q: %v", want, allowedDirectories)
		}
	}
}

func TestConfigureAllowedDirectoriesAddsProjectRoot(t *testing.T) {
	agentRoot := t.TempDir()
	projectRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectRoot, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(projectRoot, "src", "components")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	restore := saveAllowedConfig()
	t.Cleanup(restore)

	gotRoot := configureAllowedDirectories(agentRoot, nested)
	if gotRoot != projectRoot {
		t.Fatalf("projectRoot = %q, want %q", gotRoot, projectRoot)
	}
	if defaultBaseDir != nested {
		t.Errorf("defaultBaseDir = %q, want launch directory %q", defaultBaseDir, nested)
	}
	if !containsPath(allowedDirectories, projectRoot) {
		t.Errorf("allowedDirectories missing project root %q: %v", projectRoot, allowedDirectories)
	}

	newFile := filepath.Join(projectRoot, "src", "main.go")
	if _, err := isPathAllowed(newFile, allowedDirectories); err != nil {
		t.Errorf("expected project file %q to be allowed: %v", newFile, err)
	}
}

func TestConfigureAllowedDirectoriesDoesNotUseHomeAsProject(t *testing.T) {
	agentRoot := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv("USERPROFILE", userHome)
	if err := os.Mkdir(filepath.Join(userHome, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(userHome, "random", "folder")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	restore := saveAllowedConfig()
	t.Cleanup(restore)

	projectRoot := configureAllowedDirectories(agentRoot, nested)
	if projectRoot != "" {
		t.Fatalf("projectRoot = %q, want empty for home directory", projectRoot)
	}
	if defaultBaseDir != filepath.Join(agentRoot, "home") {
		t.Errorf("defaultBaseDir = %q, want agent home", defaultBaseDir)
	}
	if containsPath(allowedDirectories, userHome) {
		t.Error("home directory must not be added as a project root")
	}
}

func TestReadMultipleFilesValidatesEachPath(t *testing.T) {
	base := t.TempDir()
	fileA := filepath.Join(base, "a.txt")
	fileB := filepath.Join(base, "b.txt")
	if err := os.WriteFile(fileA, []byte("alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileB, []byte("beta"), 0o644); err != nil {
		t.Fatal(err)
	}

	restore := saveAllowedConfig()
	t.Cleanup(restore)
	allowedDirectories = []string{base}
	defaultBaseDir = base

	pathsJSON, err := json.Marshal([]string{fileA, fileB})
	if err != nil {
		t.Fatal(err)
	}
	argsJSON, err := json.Marshal(FileSystemArgs{
		ToolName: "read_multiple_files",
		Path:     string(pathsJSON),
	})
	if err != nil {
		t.Fatal(err)
	}

	out := NewFileSystemTool().CallTool(string(argsJSON), nil)
	if strings.Contains(out, "Security error") {
		t.Fatalf("valid multi-file read was rejected: %s", out)
	}
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") {
		t.Fatalf("unexpected read_multiple_files output: %s", out)
	}
}

func TestSetProjectDirectoryAddsLaunchDirectory(t *testing.T) {
	homeDir := t.TempDir()
	launchDir := t.TempDir()

	restore := saveAllowedConfig()
	t.Cleanup(restore)
	configureAllowedDirectories(homeDir, t.TempDir())

	if err := SetProjectDirectory(launchDir); err != nil {
		t.Fatalf("SetProjectDirectory() error = %v", err)
	}
	if defaultBaseDir != launchDir {
		t.Errorf("defaultBaseDir = %q, want %q", defaultBaseDir, launchDir)
	}
	if !containsPath(allowedDirectories, launchDir) {
		t.Errorf("allowedDirectories missing launch directory %q", launchDir)
	}

	if _, err := isPathAllowed(filepath.Join(launchDir, "notes.md"), allowedDirectories); err != nil {
		t.Errorf("expected launch file to be allowed: %v", err)
	}
}

func TestSetProjectDirectoryRejectsHome(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	restore := saveAllowedConfig()
	t.Cleanup(restore)
	configureAllowedDirectories(homeDir, t.TempDir())

	if err := SetProjectDirectory(homeDir); err == nil {
		t.Fatal("expected home directory to be rejected as a project root")
	}
	if containsPath(allowedDirectories, homeDir) {
		t.Error("home directory must not be added to allowedDirectories")
	}
}

func TestSetProjectDirectoryRejectsAnotherAgent(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	alphaRoot := filepath.Join(homeDir, ".myaaw", "alpha")
	betaRoot := filepath.Join(homeDir, ".myaaw", "beta")
	if err := os.MkdirAll(alphaRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(betaRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	oldActive := agentdir.Active()
	if err := agentdir.SetActive("beta"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = agentdir.SetActive(oldActive) })

	restore := saveAllowedConfig()
	t.Cleanup(restore)
	configureAllowedDirectories(betaRoot, t.TempDir())

	if err := SetProjectDirectory(alphaRoot); err == nil {
		t.Fatal("expected another agent's directory to be rejected")
	}
}

func TestConfigureAllowedDirectoriesKeepsJobsAccessible(t *testing.T) {
	agentRoot := t.TempDir()
	cwd := t.TempDir()
	jobsDir := filepath.Join(agentRoot, "jobs")
	if err := os.MkdirAll(jobsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	restore := saveAllowedConfig()
	t.Cleanup(restore)

	configureAllowedDirectories(agentRoot, cwd)

	jobLog := filepath.Join(jobsDir, "job_123.log")
	if _, err := isPathAllowed(jobLog, allowedDirectories); err != nil {
		t.Errorf("expected job log %q to be allowed: %v", jobLog, err)
	}
}

func TestIsProjectDirMarkers(t *testing.T) {
	markers := []string{
		"go.mod",
		"package.json",
		"pyproject.toml",
		"requirements.txt",
		"Cargo.toml",
		"composer.json",
		"Gemfile",
		"pom.xml",
		"build.gradle",
		"build.gradle.kts",
		"CMakeLists.txt",
		".sln",
		".git",
	}

	for _, marker := range markers {
		t.Run(marker, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, marker)
			if marker == ".git" {
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(path, []byte("marker"), 0o644); err != nil {
				t.Fatal(err)
			}

			if !isProjectDir(dir) {
				t.Errorf("isProjectDir(%q) = false, want true", dir)
			}
		})
	}

	t.Run("csproj", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "App.csproj"), []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
		if !isProjectDir(dir) {
			t.Errorf("isProjectDir(%q) = false, want true", dir)
		}
	})
}

func TestIsProjectDirIgnoresEnvOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRET=1"), 0o644); err != nil {
		t.Fatal(err)
	}

	if isProjectDir(dir) {
		t.Error(".env alone must not mark a directory as a project")
	}
}

func TestDetectProjectRootWalksUp(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "src", "package")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	if got := detectProjectRoot(nested); got != root {
		t.Fatalf("detectProjectRoot(%q) = %q, want %q", nested, got, root)
	}
}

func TestIsPathAllowedRejectsPrefixSibling(t *testing.T) {
	base := t.TempDir()
	allowedFile := filepath.Join(base, "ok.txt")
	if err := os.WriteFile(allowedFile, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := isPathAllowed(allowedFile, []string{base}); err != nil {
		t.Fatalf("expected %q to be allowed: %v", allowedFile, err)
	}

	evilDir := base + "-evil"
	if err := os.MkdirAll(evilDir, 0o755); err != nil {
		t.Fatal(err)
	}
	evilFile := filepath.Join(evilDir, "secret.txt")
	if err := os.WriteFile(evilFile, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := isPathAllowed(evilFile, []string{base}); err == nil {
		t.Fatalf("expected prefix sibling %q to be rejected", evilFile)
	}
}

func TestIsPathAllowedRejectsParentTraversal(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	escape := filepath.Join(base, "..", filepath.Base(outside), "secret.txt")
	if _, err := isPathAllowed(escape, []string{base}); err == nil {
		t.Fatalf("expected traversal path %q to be rejected", escape)
	}
}

func TestIsPathAllowedRejectsSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(base, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	for _, target := range []string{
		filepath.Join(link, "secret.txt"),
		filepath.Join(link, "new.txt"),
	} {
		if _, err := isPathAllowed(target, []string{base}); err == nil {
			t.Errorf("expected symlink escape %q to be rejected", target)
		}
	}
}

func TestIsPathAllowedRejectsBrokenSymlink(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()

	link := filepath.Join(base, "broken")
	if err := os.Symlink(filepath.Join(outside, "missing.txt"), link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	if _, err := isPathAllowed(link, []string{base}); err == nil {
		t.Fatalf("expected broken symlink %q to be rejected", link)
	}
}

func TestIsPathAllowedAllowsNewFileInBase(t *testing.T) {
	base := t.TempDir()
	// Resolve the base so the assertion matches what isPathAllowed returns on
	// platforms where temp roots are symlinked (macOS /var -> /private/var).
	resolvedBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", base, err)
	}

	newFile := filepath.Join(base, "new", "file.txt")
	got, err := isPathAllowed(newFile, []string{base})
	if err != nil {
		t.Fatalf("expected new file %q to be allowed: %v", newFile, err)
	}
	if !isWithinBase(resolvedBase, got) {
		t.Errorf("resolved path %q is not within %q", got, resolvedBase)
	}
}

func saveAllowedConfig() func() {
	oldAllowed := allowedDirectories
	oldBase := defaultBaseDir
	return func() {
		allowedDirectories = oldAllowed
		defaultBaseDir = oldBase
	}
}

func containsPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}
