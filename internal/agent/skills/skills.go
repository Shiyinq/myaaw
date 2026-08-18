package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"myaaw/internal/agent"

	"gopkg.in/yaml.v3"
)

type SkillMetadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Enabled     *bool  `yaml:"enabled"` // nil = not specified, defaults to enabled
}

// skillEntry is a single enabled skill discovered in the skills directory.
type skillEntry struct {
	dirName     string
	displayName string
	description string
}

// GetSkillsInstruction scans ~/.myaaw/skills/ and returns a compact summary of
// all enabled skills for injection into the agent's system prompt. Skills
// disabled via ~/.myaaw/skills/skills.json or the SKILL.md frontmatter
// (`enabled: false`) are skipped.
func GetSkillsInstruction() string {
	myaawPath, err := agent.EnsureMyaawConfig()
	if err != nil {
		return ""
	}
	skillsDir := filepath.Join(myaawPath, "skills")
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		return ""
	}
	return buildSkillsInstruction(skillsDir, loadSkillsFilter(skillsDir))
}

// buildSkillsInstruction generates the Agent Skills prompt section from the
// given skills directory, skipping skills disabled by the filter.
func buildSkillsInstruction(skillsDir string, filter map[string]bool) string {
	skills, err := listEnabledSkills(skillsDir, filter)
	if err != nil {
		fmt.Printf("Error reading skills directory: %v\n", err)
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\n# Agent Skills\n\n")
	sb.WriteString("You have access to the following skills. You can use the 'filesystem' to read file skill and 'bash' tool to execute script if available.\n")
	sb.WriteString("To use a skill, you must first read its documentation in the `~/.myaaw/skills/<skill-name>/SKILL.md` file. \nIf skills need user_id, userId, or userID, you must use User ID from the User Info. \n\n")

	for _, skill := range skills {
		sb.WriteString(fmt.Sprintf("- **%s** (`~/.myaaw/skills/%s`): %s\n", skill.displayName, skill.dirName, skill.description))
	}

	return sb.String()
}

// listEnabledSkills walks a skills directory and returns the enabled skills
// (respecting both skills.json and the SKILL.md frontmatter `enabled` field).
// Directories without a parseable SKILL.md frontmatter are skipped.
func listEnabledSkills(skillsDir string, filter map[string]bool) ([]skillEntry, error) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, err
	}

	var skills []skillEntry
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirName := entry.Name()

		// Read and parse the SKILL.md frontmatter
		content, err := os.ReadFile(filepath.Join(skillsDir, dirName, "SKILL.md"))
		if err != nil {
			continue
		}
		parts := strings.SplitN(string(content), "---", 3)
		if len(parts) < 3 {
			continue
		}
		var metadata SkillMetadata
		if err := yaml.Unmarshal([]byte(parts[1]), &metadata); err != nil {
			continue
		}

		if !isSkillEnabled(dirName, filter, metadata.Enabled) {
			continue
		}

		skills = append(skills, skillEntry{dirName: dirName, displayName: metadata.Name, description: metadata.Description})
	}

	return skills, nil
}

// loadSkillsFilter reads <skillsDir>/skills.json and returns a map of
// skill directory name -> enabled. Returns nil (all skills enabled) when the
// file is missing or invalid.
func loadSkillsFilter(skillsDir string) map[string]bool {
	configPath := filepath.Join(skillsDir, "skills.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Printf("Warning: Failed to read %s: %v\n", configPath, err)
		}
		return nil
	}

	var filter map[string]bool
	if err := json.Unmarshal(data, &filter); err != nil {
		fmt.Printf("Warning: Failed to parse %s: %v\n", configPath, err)
		return nil
	}
	return filter
}

// isSkillEnabled reports whether a skill is enabled. Priority:
//  1. skills.json, when the skill is explicitly listed there.
//  2. The SKILL.md frontmatter `enabled` field (nil = not specified).
//  3. Default to enabled when the skill is omitted everywhere.
func isSkillEnabled(name string, filter map[string]bool, frontmatterEnabled *bool) bool {
	if filter != nil {
		if enabled, exists := filter[name]; exists {
			return enabled
		}
	}
	if frontmatterEnabled != nil {
		return *frontmatterEnabled
	}
	return true
}

// IsSkillEnabled reports whether the named skill is enabled according to
// ~/.myaaw/skills/skills.json. Used by callers that need to validate a skill
// independently of prompt generation.
func IsSkillEnabled(name string) bool {
	myaawPath, err := agent.EnsureMyaawConfig()
	if err != nil {
		return true
	}
	skillsDir := filepath.Join(myaawPath, "skills")
	return isSkillEnabled(name, loadSkillsFilter(skillsDir), nil)
}
