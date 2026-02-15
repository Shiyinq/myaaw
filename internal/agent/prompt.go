package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type SystemPromptBuilder struct {
	userID  int64
	homeDir string
}

func NewSystemPromptBuilder(userID int64) *SystemPromptBuilder {
	// Ensure .myaaw exists in home directory
	myaawPath, err := EnsureMyaawConfig()
	homeDir := ""
	if err == nil {
		homeDir = filepath.Join(myaawPath, "home")
	} else {
		fmt.Printf("Error ensuring .myaaw config: %v\n", err)
	}

	return &SystemPromptBuilder{
		userID:  userID,
		homeDir: homeDir,
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
	// Priority: IDENTITY.md > AGENTS.md
	identity := b.readContent("IDENTITY.md")
	if identity == "" {
		identity = b.readContent("AGENTS.md")
	}
	sb.WriteString(identity)
	sb.WriteString("\n\n")

	// 2. User Info (Dynamic)
	sb.WriteString("# User Info\n")
	sb.WriteString(fmt.Sprintf("User ID: %d\n", b.userID))
	// We can add more dynamic user info here if needed (e.g. from DB)
	sb.WriteString("\n")

	// 3. Date & Time
	now := time.Now()
	sb.WriteString("## Date & Time\n")
	sb.WriteString(fmt.Sprintf("Current Date: %s\n", now.Format("Monday, January 02, 2006")))
	sb.WriteString(fmt.Sprintf("Current Time: %s\n", now.Format("15:04:05 MST")))
	sb.WriteString("\n")

	// 4. Runtime
	sb.WriteString("## Runtime\n")
	sb.WriteString(fmt.Sprintf("OS: %s\n", runtime.GOOS))
	sb.WriteString(fmt.Sprintf("Arch: %s\n", runtime.GOARCH))
	sb.WriteString("\n")

	// 5. Workspace / Context Injection
	// Order: SOUL, TOOLS, USER
	files := []string{"SOUL.md", "TOOLS.md", "USER.md"}

	for _, fileName := range files {
		content := b.readContent(fileName)
		if content != "" {
			sb.WriteString(fmt.Sprintf("## %s\n", fileName))
			sb.WriteString(content)
			sb.WriteString("\n\n")
		}
	}

	return strings.TrimSpace(sb.String())
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
