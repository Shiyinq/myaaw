package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// EnsureMyaawConfig checks if the .myaaw configuration directory exists in the user's home directory.
// Returns the path to the home .myaaw directory if it exists, otherwise returns an error.
func EnsureMyaawConfig() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	targetDir := filepath.Join(homeDir, ".myaaw")

	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		return "", fmt.Errorf("configuration directory %s not found. Please run 'myaaw onboard' first", targetDir)
	}

	return targetDir, nil
}

// workspaceDirName is the directory inside the agent home where all generated
// projects, cloned repositories, downloads, and build artifacts must be stored.
const workspaceDirName = "workspace"

type SystemPromptBuilder struct {
	userID       int64
	channelName  string
	homeDir      string
	workspaceDir string
	currentDir   string
}

func NewSystemPromptBuilder(userID int64, channelName string) *SystemPromptBuilder {
	// Ensure .myaaw exists in home directory
	myaawPath, err := EnsureMyaawConfig()
	homeDir := ""
	workspaceDir := ""
	if err == nil {
		homeDir = filepath.Join(myaawPath, "home")
		workspaceDir = ensureWorkspace(homeDir)
	} else {
		fmt.Printf("Error ensuring .myaaw config: %v\n", err)
	}

	// Local sessions run inside the user's project directory. Remote channels
	// (Telegram/Discord) use the server process directory, which is not useful
	// project context, so it is intentionally omitted for them.
	currentDir := ""
	if isLocalChannel(channelName) {
		if wd, err := os.Getwd(); err == nil {
			currentDir = wd
		}
	}

	return &SystemPromptBuilder{
		userID:       userID,
		channelName:  channelName,
		homeDir:      homeDir,
		workspaceDir: workspaceDir,
		currentDir:   currentDir,
	}
}

func (b *SystemPromptBuilder) Build() string {
	var sb strings.Builder

	// 0. Bootstrap (Highest Priority)
	bootstrap := b.readContent("BOOTSTRAP.md")
	if bootstrap != "" {
		sb.WriteString(bootstrap)
		sb.WriteString("\n\n")
	}

	// 1. Header & Identity
	// Load IDENTITY.md (Persona) and AGENTS.md (Instructions)
	identity := b.readContent("IDENTITY.md")
	if identity != "" {
		sb.WriteString(identity)
		sb.WriteString("\n\n")
	}

	agents := b.readContent("AGENTS.md")
	if agents != "" {
		sb.WriteString(agents)
		sb.WriteString("\n\n")
	}

	// 2. User Info (Dynamic)
	sb.WriteString("# User Info\n")
	fmt.Fprintf(&sb, "User ID: %d\n", b.userID)
	if b.channelName != "" {
		fmt.Fprintf(&sb, "Channel Platform: %s\n", b.channelName)
	}
	// We can add more dynamic user info here if needed (e.g. from DB)
	sb.WriteString("\n")

	// 3. Date & Time
	now := time.Now()
	sb.WriteString("## Date & Time\n")
	fmt.Fprintf(&sb, "Current Date: %s\n", now.Format("Monday, January 02, 2006"))
	fmt.Fprintf(&sb, "Current Time: %s\n", now.Format("15:04:05 MST"))
	sb.WriteString("\n")

	// 4. Runtime
	sb.WriteString("## Runtime\n")
	fmt.Fprintf(&sb, "OS: %s\n", runtime.GOOS)
	fmt.Fprintf(&sb, "Arch: %s\n", runtime.GOARCH)
	sb.WriteString("\n")

	// 5. Workspace (Dynamic)
	// Expose absolute paths so the agent never has to guess where the workspace
	// is. The behavioral rules live in AGENTS.md.
	if b.workspaceDir != "" {
		projectsDir := filepath.Join(b.workspaceDir, "projects")
		downloadsDir := filepath.Join(b.workspaceDir, "downloads")

		sb.WriteString("## Workspace\n")
		fmt.Fprintf(&sb, "Workspace root: %s\n", b.workspaceDir)
		fmt.Fprintf(&sb, "- Projects: %s/<project-name>/\n", projectsDir)
		fmt.Fprintf(&sb, "- Downloads: %s/\n\n", downloadsDir)
	}

	// 6. Current Directory (Dynamic)
	// Only local CLI sessions have a meaningful project directory.
	if b.currentDir != "" {
		sb.WriteString("## Current Directory\n")
		fmt.Fprintf(&sb, "Myaaw launch directory: %s\n\n", b.currentDir)
	}

	// 7. Workspace / Context Injection
	// Order: SOUL, TOOLS, USER
	files := []string{"SOUL.md", "TOOLS.md", "USER.md"}

	for _, fileName := range files {
		content := b.readContent(fileName)
		if content != "" {
			fmt.Fprintf(&sb, "## %s\n", fileName)
			sb.WriteString(content)
			sb.WriteString("\n\n")
		}
	}

	return strings.TrimSpace(sb.String())
}

// isLocalChannel reports whether the channel runs in the user's local working
// directory and should therefore receive current-directory context.
func isLocalChannel(channelName string) bool {
	switch strings.ToLower(channelName) {
	case "cli", "voice-classic":
		return true
	default:
		return false
	}
}

// ensureWorkspace creates the workspace directory tree and returns its path.
// The workspace is the only location where the agent should create generated
// projects and downloads. It lives inside the allowed filesystem root.
func ensureWorkspace(homeDir string) string {
	if homeDir == "" {
		return ""
	}

	workspaceDir := filepath.Join(homeDir, workspaceDirName)
	dirs := []string{
		workspaceDir,
		filepath.Join(workspaceDir, "projects"),
		filepath.Join(workspaceDir, "downloads"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Printf("Error creating workspace directory %s: %v\n", dir, err)
			return workspaceDir
		}
	}

	return workspaceDir
}

func (b *SystemPromptBuilder) readContent(fileName string) string {
	if b.homeDir == "" {
		return ""
	}
	path := filepath.Join(b.homeDir, fileName)
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(content)
}
