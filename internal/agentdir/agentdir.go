// Package agentdir owns the on-disk layout for isolated Myaaw agents.
// It intentionally depends only on the standard library so config, tools, and
// command packages can all use it without import cycles.
package agentdir

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultAgentName is used when no agent is selected.
	DefaultAgentName = "default"
	// MaxNameLength keeps names usable as directory names and CLI arguments.
	MaxNameLength = 64
)

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// AgentInfo describes a registered agent. Paths are derived from the name, so
// only metadata that must survive is stored here.
type AgentInfo struct {
	CreatedAt string `json:"created_at"`
	Path      string `json:"path,omitempty"`
}

// Registry is stored in ~/.myaaw/agents.json.
type Registry struct {
	Default string               `json:"default"`
	Agents  map[string]AgentInfo `json:"agents"`
}

// ValidateName enforces the agent naming rules.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("agent name cannot be empty")
	}
	if len(name) > MaxNameLength {
		return fmt.Errorf("agent name must be at most %d characters", MaxNameLength)
	}
	if !namePattern.MatchString(name) {
		return fmt.Errorf("agent name %q is invalid: use only letters, numbers, '_' and '-', starting with a letter or number", name)
	}
	if strings.EqualFold(name, "pids") {
		return fmt.Errorf("agent name %q is reserved", name)
	}
	return nil
}

var (
	activeName = DefaultAgentName
	activeSet  bool
)

// Active returns the current process agent name. Before the CLI resolves a
// flag, MYAAW_AGENT lets daemon children select an agent during package init.
func Active() string {
	if activeSet {
		return activeName
	}
	if name := strings.TrimSpace(os.Getenv("MYAAW_AGENT")); name != "" {
		if ValidateName(name) == nil {
			return name
		}
	}
	return activeName
}

// SetActive validates and selects the current process agent.
func SetActive(name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	activeName = name
	activeSet = true
	return nil
}

// ResolveActive chooses the agent from the flag, environment, or registry and
// stores it as the process active agent.
func ResolveActive(flag string) (string, error) {
	name := strings.TrimSpace(flag)
	if name == "" {
		name = strings.TrimSpace(os.Getenv("MYAAW_AGENT"))
	}
	if name == "" {
		if registry, err := LoadRegistry(); err == nil && registry.Default != "" {
			name = registry.Default
		}
	}
	if name == "" {
		name = DefaultAgentName
	}
	if err := SetActive(name); err != nil {
		return "", err
	}
	return name, nil
}

// MyaawRoot returns ~/.myaaw.
func MyaawRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get user home directory: %w", err)
	}
	return filepath.Join(home, ".myaaw"), nil
}

// AgentRoot returns ~/.myaaw/<name>.
func AgentRoot(name string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	root, err := MyaawRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, name), nil
}

// ActiveRoot returns the root directory of the active agent.
func ActiveRoot() (string, error) {
	return AgentRoot(Active())
}

func agentPath(name, elem string) (string, error) {
	root, err := AgentRoot(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, elem), nil
}

func activePath(elem string) (string, error) {
	return agentPath(Active(), elem)
}

func Home(name string) (string, error)      { return agentPath(name, "home") }
func ConfigDir(name string) (string, error) { return agentPath(name, "config") }
func ConfigPath(name string) (string, error) {
	return agentPath(name, filepath.Join("config", "config.json"))
}
func EnvPath(name string) (string, error)      { return agentPath(name, ".env") }
func SkillsDir(name string) (string, error)    { return agentPath(name, "skills") }
func JobsDir(name string) (string, error)      { return agentPath(name, "jobs") }
func LogsDir(name string) (string, error)      { return agentPath(name, "logs") }
func CronDir(name string) (string, error)      { return agentPath(name, "cron") }
func ToolsDir(name string) (string, error)     { return agentPath(name, "tools") }
func ContactsPath(name string) (string, error) { return agentPath(name, "contacts.json") }
func DatabaseDir(name string) (string, error)  { return agentPath(name, "database") }
func DatabasePath(name string) (string, error) {
	return agentPath(name, filepath.Join("database", "myaaw.db"))
}

func ActiveHome() (string, error)         { return activePath("home") }
func ActiveConfigPath() (string, error)   { return activePath(filepath.Join("config", "config.json")) }
func ActiveEnvPath() (string, error)      { return activePath(".env") }
func ActiveSkillsDir() (string, error)    { return activePath("skills") }
func ActiveJobsDir() (string, error)      { return activePath("jobs") }
func ActiveLogsDir() (string, error)      { return activePath("logs") }
func ActiveCronDir() (string, error)      { return activePath("cron") }
func ActiveToolsDir() (string, error)     { return activePath("tools") }
func ActiveContactsPath() (string, error) { return activePath("contacts.json") }
func ActiveDatabaseDir() (string, error)  { return activePath("database") }
func ActiveDatabasePath() (string, error) { return activePath(filepath.Join("database", "myaaw.db")) }

// PIDDir is shared by all agents so daemon files cannot collide by accident.
func PIDDir() (string, error) {
	root, err := MyaawRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "pids"), nil
}

// RegistryPath returns ~/.myaaw/agents.json.
func RegistryPath() (string, error) {
	root, err := MyaawRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "agents.json"), nil
}

// LoadRegistry reads agents.json, returning an initialized registry when the
// file does not exist yet.
func LoadRegistry() (*Registry, error) {
	path, err := RegistryPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Registry{Default: DefaultAgentName, Agents: map[string]AgentInfo{}}, nil
		}
		return nil, err
	}

	var registry Registry
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if registry.Agents == nil {
		registry.Agents = map[string]AgentInfo{}
	}
	if registry.Default == "" {
		registry.Default = DefaultAgentName
	}
	return &registry, nil
}

// SaveRegistry writes agents.json atomically.
func SaveRegistry(registry *Registry) error {
	if registry.Agents == nil {
		registry.Agents = map[string]AgentInfo{}
	}
	if registry.Default == "" {
		registry.Default = DefaultAgentName
	}

	path, err := RegistryPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(registry, "", "    ")
	if err != nil {
		return err
	}

	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, append(data, '\n'), 0o644); err != nil {
		return err
	}
	// Windows cannot reliably replace an existing file with Rename.
	_ = os.Remove(path)
	return os.Rename(tempPath, path)
}

// RegisterAgent adds an agent to the registry if missing.
func RegisterAgent(name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	registry, err := LoadRegistry()
	if err != nil {
		return err
	}
	if _, exists := registry.Agents[name]; !exists {
		registry.Agents[name] = AgentInfo{CreatedAt: time.Now().Format("2006-01-02")}
	}
	if registry.Default == "" {
		registry.Default = name
	}
	return SaveRegistry(registry)
}

// ListAgents returns sorted registered agent names.
func ListAgents() ([]string, error) {
	registry, err := LoadRegistry()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(registry.Agents))
	for name := range registry.Agents {
		if AgentInitialized(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// SetDefault updates the registry default agent.
func SetDefault(name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	registry, err := LoadRegistry()
	if err != nil {
		return err
	}
	if _, exists := registry.Agents[name]; !exists {
		return fmt.Errorf("agent %q is not registered", name)
	}
	registry.Default = name
	return SaveRegistry(registry)
}

// UnregisterAgent removes an agent from the registry. It does not delete files.
func UnregisterAgent(name string) error {
	registry, err := LoadRegistry()
	if err != nil {
		return err
	}
	delete(registry.Agents, name)
	if registry.Default == name {
		registry.Default = DefaultAgentName
	}
	return SaveRegistry(registry)
}

// AgentExists reports whether the agent directory exists.
func AgentExists(name string) bool {
	root, err := AgentRoot(name)
	if err != nil {
		return false
	}
	info, err := os.Stat(root)
	return err == nil && info.IsDir()
}

// AgentInitialized reports whether the agent has its required home and config
// files. A directory containing only log files is not considered initialized.
func AgentInitialized(name string) bool {
	root, err := AgentRoot(name)
	if err != nil {
		return false
	}
	return agentInitialized(root)
}

// EnsureLayout creates ~/.myaaw and migrates a legacy single-agent layout into
// ~/.myaaw/default. It is safe to call repeatedly.
func EnsureLayout() error {
	root, err := MyaawRoot()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}

	registry, err := LoadRegistry()
	if err != nil {
		return err
	}

	defaultRoot := filepath.Join(root, DefaultAgentName)
	if legacyLayoutExists(root) && !agentInitialized(defaultRoot) {
		if err := migrateLegacyLayout(root, defaultRoot); err != nil {
			return err
		}
	}

	if _, exists := registry.Agents[DefaultAgentName]; !exists {
		registry.Agents[DefaultAgentName] = AgentInfo{CreatedAt: time.Now().Format("2006-01-02")}
	}
	if registry.Default == "" {
		registry.Default = DefaultAgentName
	}
	return SaveRegistry(registry)
}

func migrateLegacyLayout(root, defaultRoot string) error {
	legacyDirs := []string{"home", "config", "database", "skills", "logs", "jobs", "cron", "tools"}

	if err := os.MkdirAll(defaultRoot, 0o755); err != nil {
		return err
	}

	for _, name := range legacyDirs {
		src := filepath.Join(root, name)
		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		}
		dst := filepath.Join(defaultRoot, name)
		if err := moveOrMergeDir(src, dst); err != nil {
			return fmt.Errorf("migrate %s: %w", src, err)
		}
	}

	legacyEnv := filepath.Join(root, ".env")
	if _, err := os.Stat(legacyEnv); err == nil {
		dstEnv := filepath.Join(defaultRoot, ".env")
		if _, err := os.Stat(dstEnv); os.IsNotExist(err) {
			if err := os.Rename(legacyEnv, dstEnv); err != nil {
				return fmt.Errorf("migrate %s: %w", legacyEnv, err)
			}
		} else {
			if err := os.Rename(legacyEnv, filepath.Join(defaultRoot, ".env.legacy")); err != nil {
				return fmt.Errorf("migrate %s: %w", legacyEnv, err)
			}
		}
	}

	return RewriteLegacyPaths(defaultRoot, DefaultAgentName)
}

func legacyLayoutExists(root string) bool {
	legacyDirs := []string{"home", "config", "database", "skills", "logs", "jobs", "cron", "tools"}
	for _, name := range legacyDirs {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return true
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".env")); err == nil {
		return true
	}
	return false
}

func agentInitialized(root string) bool {
	if _, err := os.Stat(filepath.Join(root, "home", "AGENTS.md")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(root, "config", "config.json")); err == nil {
		return true
	}
	return false
}

func moveOrMergeDir(src, dst string) error {
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		return os.Rename(src, dst)
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if _, err := os.Stat(dstPath); os.IsNotExist(err) {
			if err := os.Rename(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		if entry.IsDir() {
			if err := moveOrMergeDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		// Preserve legacy duplicates instead of silently dropping them.
		legacyPath := dstPath + ".legacy"
		if _, err := os.Stat(legacyPath); os.IsNotExist(err) {
			if err := os.Rename(srcPath, legacyPath); err != nil {
				return err
			}
		}
	}

	// Remove src when all contents have been moved.
	if remaining, err := os.ReadDir(src); err == nil && len(remaining) == 0 {
		return os.Remove(src)
	}
	return nil
}

// RewriteLegacyPaths updates text files under agentRoot so references to the
// old shared ~/.myaaw paths point at the agent-specific location.
func RewriteLegacyPaths(agentRoot, name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	root, err := MyaawRoot()
	if err != nil {
		return err
	}

	components := []string{"home", "config", "database", "skills", "logs", "jobs", "cron", "tools"}
	replacements := make([][2]string, 0, len(components)*4+2)
	for _, component := range components {
		replacements = append(replacements,
			[2]string{"~/.myaaw/" + component, "~/.myaaw/" + name + "/" + component},
			[2]string{filepath.Join(root, component), filepath.Join(root, name, component)},
			[2]string{`".myaaw", "` + component + `"`, `".myaaw", "` + name + `", "` + component + `"`},
			[2]string{`'.myaaw', '` + component + `'`, `'.myaaw', '` + name + `', '` + component + `'`},
		)
	}
	replacements = append(replacements,
		[2]string{"~/.myaaw/.env", "~/.myaaw/" + name + "/.env"},
		[2]string{filepath.Join(root, ".env"), filepath.Join(root, name, ".env")},
	)

	return filepath.WalkDir(agentRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".md", ".txt", ".json", ".yaml", ".yml", ".toml", ".env", ".sh", ".py":
		default:
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		updated := string(content)
		for _, replacement := range replacements {
			updated = strings.ReplaceAll(updated, replacement[0], replacement[1])
		}
		if updated == string(content) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		return os.WriteFile(path, []byte(updated), info.Mode().Perm())
	})
}

// SuggestedPort returns a port that is not used by any registered agent.
// The default agent keeps 8080; new agents get the next free port.
// SuggestedPort returns the lowest port that is not used by another registered
// agent. The caller's own assignment is ignored so reusing a value does not look
// like a conflict.
func SuggestedPort(name string) int {
	used := map[int]bool{}
	if registry, err := LoadRegistry(); err == nil {
		for agentName := range registry.Agents {
			if agentName == name {
				continue
			}
			if port := readPort(agentName); port > 0 {
				used[port] = true
			}
		}
	}

	for port := 8080; port <= 65535; port++ {
		if !used[port] {
			return port
		}
	}
	return 8080
}

func readPort(name string) int {
	envPath, err := EnvPath(name)
	if err != nil {
		return 0
	}
	data, err := os.ReadFile(envPath)
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "PORT=") {
			continue
		}
		port, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "PORT=")))
		if err == nil {
			return port
		}
	}
	return 0
}

// UpsertEnvValue updates or appends KEY=value in an env file.
func UpsertEnvValue(path, key, value string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), key+"=") {
			lines[i] = key + "=" + value
			found = true
			break
		}
	}
	if !found {
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		lines = append(lines, key+"="+value, "")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}
