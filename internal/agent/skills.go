package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type SkillMetadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func GetSkillsInstruction() string {
	var sb strings.Builder
	sb.WriteString("\n\n# Agent Skills\n\n")
	sb.WriteString("You have access to the following skills. You can use the 'filesystem' to read file skill and 'bash' tool to execute script if available.\n")
	sb.WriteString("To use a skill, you must first read its documentation in the `~/.myaaw/skills/<skill-name>/SKILL.md` file. \nIf skills need user_id, userId, or userID, you must use User ID from the User Info. \n\n")

	// Walk through the skills directory
	myaawPath, err := EnsureMyaawConfig()
	if err != nil {
		return ""
	}
	skillsDir := filepath.Join(myaawPath, "skills")
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		return ""
	}

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		fmt.Printf("Error reading skills directory: %v\n", err)
		return ""
	}

	for _, entry := range entries {
		if entry.IsDir() {
			skillName := entry.Name()
			skillPath := filepath.Join(skillsDir, skillName, "SKILL.md")

			if _, err := os.Stat(skillPath); err == nil {
				// Read the SKILL.md file
				content, err := os.ReadFile(skillPath)
				if err != nil {
					continue
				}

				// Parse frontmatter
				parts := strings.SplitN(string(content), "---", 3)
				if len(parts) >= 3 {
					var metadata SkillMetadata
					if err := yaml.Unmarshal([]byte(parts[1]), &metadata); err == nil {
						sb.WriteString(fmt.Sprintf("- **%s** (`~/.myaaw/skills/%s`): %s\n", metadata.Name, skillName, metadata.Description))
					}
				}
			}
		}
	}

	return sb.String()
}
