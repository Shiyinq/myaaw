package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"myaaw/internal/agent/skills"
	"myaaw/internal/agentdir"
	"myaaw/internal/onboard"

	myaaw "myaaw"
)

var (
	ErrSkillNotFound    = errors.New("skill not found")
	ErrInvalidSkillName = errors.New("invalid skill name")
)

// SkillService manages an agent's skills directory and skills.json.
type SkillService struct{}

func NewSkillService() *SkillService {
	return &SkillService{}
}

// SkillFile is one file inside a skill directory.
type SkillFile struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time,omitempty"`
}

// SkillDetail is one skill with its SKILL.md content and file listing.
type SkillDetail struct {
	Name        string      `json:"name"`
	DisplayName string      `json:"display_name"`
	Description string      `json:"description"`
	Enabled     bool        `json:"enabled"`
	Source      string      `json:"source"`
	ParseOK     bool        `json:"parse_ok"`
	Content     string      `json:"content"`
	Files       []SkillFile `json:"files"`
}

// SkillFileContent is one file inside a skill. Binary files are returned as
// base64 with Binary set.
type SkillFileContent struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time,omitempty"`
	Binary  bool   `json:"binary"`
	Content string `json:"content"`
}

// maxSkillFiles caps the number of files reported in a skill detail so one
// huge directory cannot inflate a response without bound.
const maxSkillFiles = 1000

// Get returns one skill's metadata, SKILL.md content, and recursive file
// listing. When the folder is missing but a stale skills.json key remains,
// the key is removed automatically and ErrSkillNotFound is returned.
func (s *SkillService) Get(agentName, name string) (SkillDetail, error) {
	if err := validateSkillName(name); err != nil {
		return SkillDetail{}, err
	}
	if err := agentdirValidateAgent(agentName); err != nil {
		return SkillDetail{}, err
	}
	dir, err := agentdir.SkillsDir(agentName)
	if err != nil {
		return SkillDetail{}, err
	}

	skillDir := filepath.Join(dir, name)
	info, statErr := os.Stat(skillDir)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			if cleanupErr := s.removeStaleKey(dir, name); cleanupErr != nil {
				return SkillDetail{}, cleanupErr
			}
			return SkillDetail{}, ErrSkillNotFound
		}
		return SkillDetail{}, statErr
	}
	if !info.IsDir() {
		return SkillDetail{}, fmt.Errorf("%w: %s is not a skill directory", ErrInvalidSkillName, name)
	}

	detail := SkillDetail{
		Name:        name,
		DisplayName: name,
		Enabled:     true,
		Source:      skills.SkillSourceDefault,
		Files:       []SkillFile{},
	}

	list, err := skills.ListSkills(dir)
	if err != nil {
		return SkillDetail{}, err
	}
	for _, skill := range list {
		if skill.Name == name {
			detail.Name = skill.Name
			detail.DisplayName = skill.DisplayName
			detail.Description = skill.Description
			detail.Enabled = skill.Enabled
			detail.Source = skill.Source
			detail.ParseOK = skill.ParseOK
			break
		}
	}

	skillMDPath := filepath.Join(skillDir, "SKILL.md")
	if mdInfo, mdErr := os.Stat(skillMDPath); mdErr == nil && mdInfo.Mode().IsRegular() {
		if mdInfo.Size() > maxReadSize {
			return SkillDetail{}, ErrPathTooLarge
		}
		data, readErr := os.ReadFile(skillMDPath)
		if readErr != nil {
			return SkillDetail{}, readErr
		}
		detail.Content = string(data)
	}

	files, err := listSkillFiles(skillDir)
	if err != nil {
		return SkillDetail{}, err
	}
	detail.Files = files
	return detail, nil
}

// ReadFile returns one file inside a skill. Text files come back as plain
// text; binary files (images, compiled scripts) as base64 with Binary set.
func (s *SkillService) ReadFile(agentName, skillName, relPath string) (SkillFileContent, error) {
	if err := validateSkillName(skillName); err != nil {
		return SkillFileContent{}, err
	}
	if err := agentdirValidateAgent(agentName); err != nil {
		return SkillFileContent{}, err
	}
	dir, err := agentdir.SkillsDir(agentName)
	if err != nil {
		return SkillFileContent{}, err
	}
	skillDir := filepath.Join(dir, skillName)
	if err := requireSkillDir(dir, skillName); err != nil {
		return SkillFileContent{}, err
	}

	cleaned, err := normalizeSkillFilePath(relPath)
	if err != nil {
		return SkillFileContent{}, err
	}
	abs := filepath.Join(skillDir, filepath.FromSlash(cleaned))
	if err := ensureSymlinksInside(skillDir, abs); err != nil {
		return SkillFileContent{}, err
	}

	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return SkillFileContent{}, ErrPathNotFound
		}
		return SkillFileContent{}, err
	}
	if !info.Mode().IsRegular() {
		return SkillFileContent{}, fmt.Errorf("%w: %s is not a file", ErrInvalidPath, cleaned)
	}
	if info.Size() > maxReadSize {
		return SkillFileContent{}, ErrPathTooLarge
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return SkillFileContent{}, err
	}

	content := SkillFileContent{
		Path:    cleaned,
		Size:    info.Size(),
		ModTime: info.ModTime().UTC().Format(time.RFC3339),
	}
	if looksBinary(data) {
		content.Binary = true
		content.Content = base64.StdEncoding.EncodeToString(data)
	} else {
		content.Content = string(data)
	}
	return content, nil
}

// normalizeSkillFilePath limits a relative path to the skill directory.
func normalizeSkillFilePath(rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return "", fmt.Errorf("%w: path is required", ErrInvalidPath)
	}
	if strings.ContainsAny(rel, "\\:\x00") {
		return "", fmt.Errorf("%w: path contains unsupported characters", ErrInvalidPath)
	}
	cleaned := path.Clean(rel)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("%w: path escapes the skill directory", ErrInvalidPath)
	}
	return cleaned, nil
}

// RemoveConfig removes the skills.json override for a skill without touching
// the skill folder, so its enabled state falls back to the SKILL.md
// frontmatter or the default. The folder must exist; a missing key is not an
// error.
func (s *SkillService) RemoveConfig(agentName, name string) error {
	if err := validateSkillName(name); err != nil {
		return err
	}
	if err := agentdirValidateAgent(agentName); err != nil {
		return err
	}
	dir, err := agentdir.SkillsDir(agentName)
	if err != nil {
		return err
	}
	if err := requireSkillDir(dir, name); err != nil {
		return err
	}

	filter, err := readSkillsFilter(dir)
	if err != nil {
		return err
	}
	if filter == nil {
		return nil
	}
	if _, exists := filter[name]; !exists {
		return nil
	}
	delete(filter, name)
	return writeSkillsFilter(dir, filter)
}

// removeStaleKey deletes a skills.json key without touching the filesystem.
func (s *SkillService) removeStaleKey(dir, name string) error {
	filter, err := readSkillsFilter(dir)
	if err != nil {
		return err
	}
	if filter == nil {
		return nil
	}
	if _, exists := filter[name]; !exists {
		return nil
	}
	delete(filter, name)
	return writeSkillsFilter(dir, filter)
}

func listSkillFiles(skillDir string) ([]SkillFile, error) {
	files := make([]SkillFile, 0, 16)
	walkErr := filepath.WalkDir(skillDir, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry == nil {
			return nil
		}
		if entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel, relErr := filepath.Rel(skillDir, filePath)
		if relErr != nil {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil
		}
		files = append(files, SkillFile{
			Path:    filepath.ToSlash(rel),
			Size:    info.Size(),
			ModTime: info.ModTime().UTC().Format(time.RFC3339),
		})
		if len(files) >= maxSkillFiles {
			return fs.SkipAll
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// List reads the skills directory fresh from disk. A missing directory is
// treated as empty, so deletions made outside the API are always reflected.
func (s *SkillService) List(agentName string) ([]skills.Skill, error) {
	if err := agentdirValidateAgent(agentName); err != nil {
		return nil, err
	}
	dir, err := agentdir.SkillsDir(agentName)
	if err != nil {
		return nil, err
	}
	return skills.ListSkills(dir)
}

// Install copies one embedded built-in skill into the agent's skills directory
// and records it as enabled in skills.json. Installing a skill that is already
// present only ensures the flag, so retrying is safe.
func (s *SkillService) Install(agentName, name string) (SkillDetail, error) {
	if err := validateSkillName(name); err != nil {
		return SkillDetail{}, err
	}
	if err := agentdirValidateAgent(agentName); err != nil {
		return SkillDetail{}, err
	}
	if !isBuiltinSkill(name) {
		return SkillDetail{}, fmt.Errorf("%w: %q is not a built-in skill", ErrInvalidSkillName, name)
	}
	if err := onboard.InstallSkills(agentName, []string{name}); err != nil {
		return SkillDetail{}, err
	}
	return s.Get(agentName, name)
}

// isBuiltinSkill reports whether name matches one of the embedded templates.
func isBuiltinSkill(name string) bool {
	for _, skill := range onboard.BuiltinSkillNames() {
		if skill == name {
			return true
		}
	}
	return false
}

// BuiltinDetail returns one embedded template without installing it, so a
// skill can be inspected before it is copied into the agent.
func (s *SkillService) BuiltinDetail(name string) (SkillDetail, error) {
	if err := validateSkillName(name); err != nil {
		return SkillDetail{}, err
	}
	if !isBuiltinSkill(name) {
		return SkillDetail{}, ErrSkillNotFound
	}

	root := path.Join(".myaaw", "skills", name)
	detail := SkillDetail{
		Name:        name,
		DisplayName: name,
		Enabled:     false,
		Source:      "builtin",
		Files:       []SkillFile{},
	}
	if data, readErr := myaaw.DefaultMyaawDir.ReadFile(path.Join(root, "SKILL.md")); readErr == nil {
		detail.ParseOK = true
		detail.Content = string(data)
		detail.Description = onboard.FrontmatterDescription(string(data))
	}
	files, err := listEmbedFiles(myaaw.DefaultMyaawDir, root)
	if err != nil {
		return SkillDetail{}, err
	}
	detail.Files = files
	return detail, nil
}

// BuiltinFile returns one file from an embedded template.
func (s *SkillService) BuiltinFile(name, relPath string) (SkillFileContent, error) {
	if err := validateSkillName(name); err != nil {
		return SkillFileContent{}, err
	}
	if !isBuiltinSkill(name) {
		return SkillFileContent{}, ErrSkillNotFound
	}
	cleaned, err := normalizeSkillFilePath(relPath)
	if err != nil {
		return SkillFileContent{}, err
	}

	root := path.Join(".myaaw", "skills", name)
	abs := path.Join(root, cleaned)
	info, err := fs.Stat(myaaw.DefaultMyaawDir, abs)
	if err != nil || !info.Mode().IsRegular() {
		return SkillFileContent{}, ErrPathNotFound
	}
	if info.Size() > maxReadSize {
		return SkillFileContent{}, ErrPathTooLarge
	}
	data, err := myaaw.DefaultMyaawDir.ReadFile(abs)
	if err != nil {
		return SkillFileContent{}, ErrPathNotFound
	}

	content := SkillFileContent{Path: cleaned, Size: info.Size()}
	if looksBinary(data) {
		content.Binary = true
		content.Content = base64.StdEncoding.EncodeToString(data)
	} else {
		content.Content = string(data)
	}
	return content, nil
}

func listEmbedFiles(fsys fs.FS, root string) ([]SkillFile, error) {
	files := make([]SkillFile, 0, 16)
	walkErr := fs.WalkDir(fsys, root, func(p string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry == nil || entry.IsDir() {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil
		}
		rel := strings.TrimPrefix(p, root+"/")
		files = append(files, SkillFile{Path: rel, Size: info.Size()})
		if len(files) >= maxSkillFiles {
			return fs.SkipAll
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// SetEnabled writes the skill's enabled flag to skills.json, creating the
// file (and directory) when missing while preserving other keys.
func (s *SkillService) SetEnabled(agentName, name string, enabled bool) error {
	if err := validateSkillName(name); err != nil {
		return err
	}
	if err := agentdirValidateAgent(agentName); err != nil {
		return err
	}
	dir, err := agentdir.SkillsDir(agentName)
	if err != nil {
		return err
	}
	if err := requireSkillDir(dir, name); err != nil {
		return err
	}

	filter, err := readSkillsFilter(dir)
	if err != nil {
		return err
	}
	if filter == nil {
		filter = make(map[string]bool)
	}
	filter[name] = enabled
	return writeSkillsFilter(dir, filter)
}

// Delete removes a skill directory and its skills.json entry. Either one is
// enough to succeed, so a folder deleted manually through the terminal can
// still have its stale skills.json key cleaned up here. It returns
// ErrSkillNotFound only when neither the folder nor the key exists.
func (s *SkillService) Delete(agentName, name string) error {
	if err := validateSkillName(name); err != nil {
		return err
	}
	if err := agentdirValidateAgent(agentName); err != nil {
		return err
	}
	dir, err := agentdir.SkillsDir(agentName)
	if err != nil {
		return err
	}

	skillDir := filepath.Join(dir, name)
	info, statErr := os.Stat(skillDir)
	if statErr != nil && !os.IsNotExist(statErr) {
		return statErr
	}
	folderExists := statErr == nil
	if folderExists && !info.IsDir() {
		return fmt.Errorf("%w: %s is not a skill directory", ErrInvalidSkillName, name)
	}

	filter, err := readSkillsFilter(dir)
	if err != nil {
		return err
	}
	_, keyExists := filter[name]
	if !folderExists && !keyExists {
		return ErrSkillNotFound
	}

	if folderExists {
		if err := os.RemoveAll(skillDir); err != nil {
			return err
		}
	}
	if keyExists {
		delete(filter, name)
		return writeSkillsFilter(dir, filter)
	}
	return nil
}

func requireSkillDir(dir, name string) error {
	info, err := os.Stat(filepath.Join(dir, name))
	if err != nil {
		if os.IsNotExist(err) {
			return ErrSkillNotFound
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: %s is not a skill directory", ErrInvalidSkillName, name)
	}
	return nil
}

func validateSkillName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidSkillName)
	}
	if strings.ContainsAny(name, "/\\\x00") || name == "." || name == ".." {
		return fmt.Errorf("%w: %s", ErrInvalidSkillName, name)
	}
	return nil
}

// readSkillsFilter returns nil when skills.json is missing or invalid,
// matching the agent loader's default behavior.
func readSkillsFilter(dir string) (map[string]bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, "skills.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var filter map[string]bool
	if err := json.Unmarshal(data, &filter); err != nil {
		return nil, nil
	}
	return filter, nil
}

func writeSkillsFilter(dir string, filter map[string]bool) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(filter, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "skills.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
