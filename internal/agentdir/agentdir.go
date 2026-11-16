// Package agentdir owns the on-disk layout for isolated Myaaw agents.
// It intentionally depends only on the standard library so config, tools, and
// command packages can all use it without import cycles.
package agentdir

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"myaaw"
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
	// AgentColor is the palette key the dashboard draws the agent with. It is an
	// id from AgentColors, never an asset path: the registry stays unaware of how
	// the frontend renders it.
	AgentColor string `json:"agent_color,omitempty"`
}

// AgentColors are the Bloub palette keys, in the order the picker shows them.
// They match the file names the dashboard ships, and `encre` is the ink default.
var AgentColors = []string{
	"encre",
	"brun",
	"rouge",
	"orange",
	"ambre",
	"vert",
	"turquoise",
	"bleu",
	"violet",
	"rose",
	"gris",
	"creme",
}

// DefaultAgentColor is used whenever an agent has no colour of its own: agents
// registered before colours existed stay untouched on disk and read as ink.
const DefaultAgentColor = "encre"

// ValidAgentColor reports whether key is one of the palette ids.
func ValidAgentColor(key string) bool {
	for _, color := range AgentColors {
		if color == key {
			return true
		}
	}
	return false
}

// AgentColorOrDefault resolves a stored value, folding empty and unknown keys
// back onto DefaultAgentColor so callers never have to guess.
func AgentColorOrDefault(key string) string {
	if ValidAgentColor(key) {
		return key
	}
	return DefaultAgentColor
}

// Registry is stored in ~/.myaaw/config/agents.json.
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

// SetActive validates and selects the current process agent. It also exports
// MYAAW_AGENT so child processes (tool subprocesses, shell commands) resolve the
// same agent instead of falling back to the registry default.
func SetActive(name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	activeName = name
	activeSet = true
	_ = os.Setenv("MYAAW_AGENT", name)
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

// AgentRoot returns ~/.myaaw/agents/<name>.
func AgentRoot(name string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	root, err := MyaawRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "agents", name), nil
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

func Home(name string) (string, error) { return agentPath(name, "home") }

// UserDir is the root for content owned by the person using the agent, such as
// files they upload from chat. It is separate from home so agent-authored
// context never mixes with user-provided data.
func UserDir(name string) (string, error)   { return agentPath(name, "user") }
func ConfigDir(name string) (string, error) { return agentPath(name, "config") }
func ConfigPath(name string) (string, error) {
	return agentPath(name, filepath.Join("config", "config.json"))
}
func EnvPath(name string) (string, error)   { return agentPath(name, ".env") }
func SkillsDir(name string) (string, error) { return agentPath(name, "skills") }
func JobsDir(name string) (string, error)   { return agentPath(name, "jobs") }
func LogsDir(name string) (string, error)   { return agentPath(name, "logs") }

// CacheDir holds regenerable artifacts (preview builds, temporary output) that
// may be deleted at any time.
func CacheDir(name string) (string, error) { return agentPath(name, "cache") }
func CronDir(name string) (string, error)  { return agentPath(name, "cron") }
func ToolsDir(name string) (string, error) { return agentPath(name, "tools") }
func TelegramOffsetPath(name string) (string, error) {
	return agentPath(name, filepath.Join("database", "telegram_offset"))
}
func ContactsPath(name string) (string, error) { return agentPath(name, "contacts.json") }
func DatabaseDir(name string) (string, error)  { return agentPath(name, "database") }
func DatabasePath(name string) (string, error) {
	return agentPath(name, filepath.Join("database", "myaaw.db"))
}

// ObservabilityPath is the SQLite file that stores traces, token usage, and
// model cost for one agent.
func ObservabilityPath(name string) (string, error) {
	return agentPath(name, filepath.Join("database", "observability.db"))
}

func ActiveHome() (string, error)         { return activePath("home") }
func ActiveUserDir() (string, error)      { return activePath("user") }
func ActiveConfigPath() (string, error)   { return activePath(filepath.Join("config", "config.json")) }
func ActiveEnvPath() (string, error)      { return activePath(".env") }
func ActiveSkillsDir() (string, error)    { return activePath("skills") }
func ActiveJobsDir() (string, error)      { return activePath("jobs") }
func ActiveLogsDir() (string, error)      { return activePath("logs") }
func ActiveCronDir() (string, error)      { return activePath("cron") }
func ActiveToolsDir() (string, error)     { return activePath("tools") }
func ActiveContactsPath() (string, error) { return activePath("contacts.json") }

// ApprovalsPath returns the approval grant store path. Grants live under
// config/ next to config.json; a legacy root-level approvals.json is migrated
// lazily so existing grants are preserved.
func ApprovalsPath(name string) (string, error) {
	target, err := agentPath(name, filepath.Join("config", "approvals.json"))
	if err != nil {
		return "", err
	}
	legacy, legacyErr := agentPath(name, "approvals.json")
	if legacyErr == nil {
		if _, statErr := os.Stat(target); os.IsNotExist(statErr) {
			if _, err := os.Stat(legacy); err == nil {
				if mkErr := os.MkdirAll(filepath.Dir(target), 0o755); mkErr == nil {
					_ = os.Rename(legacy, target)
				}
			}
		}
	}
	return target, nil
}

func ActiveApprovalsPath() (string, error) { return ApprovalsPath(Active()) }
func ActiveDatabaseDir() (string, error)   { return activePath("database") }
func ActiveDatabasePath() (string, error)  { return activePath(filepath.Join("database", "myaaw.db")) }
func ActiveObservabilityPath() (string, error) {
	return activePath(filepath.Join("database", "observability.db"))
}

// PIDDir is shared by all agents so daemon files cannot collide by accident.
func PIDDir() (string, error) {
	root, err := MyaawRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "pids"), nil
}

// GlobalConfigDir returns ~/.myaaw/config, the shared folder for registry,
// control-plane credentials, and the model data tables.
func GlobalConfigDir() (string, error) {
	root, err := MyaawRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "config"), nil
}

// GlobalConfigPath returns ~/.myaaw/config/<elem>.
func GlobalConfigPath(elem string) (string, error) {
	dir, err := GlobalConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, elem), nil
}

// RegistryPath returns ~/.myaaw/config/agents.json.
func RegistryPath() (string, error) {
	return GlobalConfigPath("agents.json")
}

// legacyRegistryPath is where agents.json lived before the global config folder.
func legacyRegistryPath() (string, error) {
	root, err := MyaawRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "agents.json"), nil
}

// LoadRegistry reads agents.json, returning an initialized registry when the
// file does not exist yet. A registry left in the old root location is still
// read so the first command after an upgrade does not see an empty fleet.
func LoadRegistry() (*Registry, error) {
	paths := make([]string, 0, 2)
	if path, err := RegistryPath(); err == nil {
		paths = append(paths, path)
	}
	if path, err := legacyRegistryPath(); err == nil {
		paths = append(paths, path)
	}

	var firstErr error
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			if firstErr == nil {
				firstErr = err
			}
			continue
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
	if firstErr != nil {
		return nil, firstErr
	}
	return &Registry{Default: DefaultAgentName, Agents: map[string]AgentInfo{}}, nil
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

	// A unique temp file keeps two processes from clobbering each other's write,
	// and on Unix the rename replaces the target atomically. Removing the target
	// first would leave a window where agents.json does not exist, and a reader in
	// that window sees an empty registry — which is how agents disappear from the
	// list while their directories are still there.
	temp, err := os.CreateTemp(filepath.Dir(path), "agents-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	if _, err := temp.Write(append(data, '\n')); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Chmod(tempPath, 0o644); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	// Windows cannot reliably replace an existing file with Rename.
	if runtime.GOOS == "windows" {
		_ = os.Remove(path)
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

// AgentNameTaken reports whether a name already belongs to an agent. A name is
// taken when the agent is initialized, when its directory exists (even a
// leftover from a failed run), or when another registry entry differs only by
// case. Case-insensitive matches keep the registry consistent with how A2A
// contacts are compared.
func AgentNameTaken(name string) (bool, error) {
	if err := ValidateName(name); err != nil {
		return false, err
	}
	if AgentInitialized(name) {
		return true, nil
	}

	root, err := AgentRoot(name)
	if err != nil {
		return false, err
	}
	if _, statErr := os.Stat(root); statErr == nil {
		return true, nil
	} else if !os.IsNotExist(statErr) {
		return false, statErr
	}

	registry, err := LoadRegistry()
	if err != nil {
		return false, err
	}
	for existing := range registry.Agents {
		if existing == name || !strings.EqualFold(existing, name) {
			continue
		}
		existingRoot, err := AgentRoot(existing)
		if err != nil {
			continue
		}
		if _, statErr := os.Stat(existingRoot); statErr == nil {
			return true, nil
		}
	}
	return false, nil
}

// CreatedAtUTC returns the current instant as an RFC3339 UTC timestamp, the
// format stored for a newly created agent's registry entry.
func CreatedAtUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
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
		registry.Agents[name] = AgentInfo{CreatedAt: CreatedAtUTC()}
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

// SetAgentColor stores the palette key an agent is drawn with. A key that is not
// in AgentColors is rejected rather than coerced, so a typo cannot silently ship
// as ink.
func SetAgentColor(name, color string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	if !ValidAgentColor(color) {
		return fmt.Errorf("agent colour %q is not in the palette", color)
	}
	registry, err := LoadRegistry()
	if err != nil {
		return err
	}
	entry, exists := registry.Agents[name]
	if !exists {
		return fmt.Errorf("agent %q is not registered", name)
	}
	entry.AgentColor = color
	registry.Agents[name] = entry
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

// EnsureLayout creates ~/.myaaw/agents and the shared ~/.myaaw/config folder,
// migrates global state from the old root layout, seeds the model data tables,
// and registers the agent directories that already exist. It never creates an
// agent itself: onboarding names the first one and the registry default points
// at it. It is safe to call repeatedly.
func EnsureLayout() error {
	root, err := MyaawRoot()
	if err != nil {
		return err
	}

	configDir, err := GlobalConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return err
	}

	// The registry and credentials must exist before any reader looks for them.
	if err := migrateGlobalState(root, configDir); err != nil {
		return err
	}
	if err := seedGlobalDataFiles(configDir); err != nil {
		log.Printf("Warning: could not seed global model data: %v", err)
	}

	agentsRoot := filepath.Join(root, "agents")
	if err := os.MkdirAll(agentsRoot, 0o755); err != nil {
		return err
	}

	registry, err := LoadRegistry()
	if err != nil {
		return err
	}

	if registry.Default == "" {
		registry.Default = DefaultAgentName
	}
	registerExistingAgents(agentsRoot, registry)
	return SaveRegistry(registry)
}

// SeedGlobalDataFiles copies the embedded model data into the global config
// folder when a file is missing. It is exported so onboarding can ensure the
// folder exists even when EnsureLayout is not part of the flow.
func SeedGlobalDataFiles() error {
	dir, err := GlobalConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return seedGlobalDataFiles(dir)
}

var globalDataFiles = []string{
	"model_capabilities.json",
	"model_prices.json",
	"tool_prices.json",
}

// migrateGlobalState moves agents.json and controlplane.json into the global
// config folder when the new file is missing. A file already in the new place
// always wins; the old copy is left untouched. The same filesystem rename is
// preferred, with a copy fallback for exotic layouts.
func migrateGlobalState(root, configDir string) error {
	for _, name := range []string{"agents.json", "controlplane.json"} {
		target := filepath.Join(configDir, name)
		if _, err := os.Stat(target); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}

		legacy := filepath.Join(root, name)
		if _, err := os.Stat(legacy); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if err := moveGlobalFile(legacy, target); err != nil {
			return fmt.Errorf("move %s: %w", name, err)
		}
		log.Printf("Moved %s to %s", name, target)
	}
	return nil
}

func moveGlobalFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, data, info.Mode().Perm()); err != nil {
		return err
	}
	return os.Remove(src)
}

// seedGlobalDataFiles copies the embedded model tables without overwriting an
// existing global file, so edits survive upgrades.
func seedGlobalDataFiles(dir string) error {
	for _, name := range globalDataFiles {
		target := filepath.Join(dir, name)
		if _, err := os.Stat(target); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		data, err := myaaw.DefaultMyaawDir.ReadFile(path.Join(".myaaw", "config", name))
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", name, err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// registerExistingAgents adds every initialized agent directory under the
// agents folder that the registry does not know about. The directories are the
// source of truth: a registry that was lost, truncated, or written before an
// agent existed must not hide agents whose config and home are still on disk.
func registerExistingAgents(agentsRoot string, registry *Registry) {
	entries, err := os.ReadDir(agentsRoot)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || ValidateName(name) != nil {
			continue
		}
		if _, exists := registry.Agents[name]; exists {
			continue
		}
		agentRoot := filepath.Join(agentsRoot, name)
		if !agentInitialized(agentRoot) {
			continue
		}
		created := CreatedAtUTC()
		if info, statErr := entry.Info(); statErr == nil {
			created = info.ModTime().UTC().Format(time.RFC3339)
		}
		registry.Agents[name] = AgentInfo{CreatedAt: created}
	}
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

// RewriteLegacyPaths updates text files under agentRoot so references to the
// shared ~/.myaaw paths (as written in the embedded template) point at the
// agent's own folder under ~/.myaaw/agents/<name>. It runs for every new agent,
// which is what keeps the template files free of per-agent edits.
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
			[2]string{"~/.myaaw/" + component, "~/.myaaw/agents/" + name + "/" + component},
			[2]string{filepath.Join(root, component), filepath.Join(root, "agents", name, component)},
			[2]string{`".myaaw", "` + component + `"`, `".myaaw", "agents", "` + name + `", "` + component + `"`},
			[2]string{`'.myaaw', '` + component + `'`, `'.myaaw', 'agents', '` + name + `', '` + component + `'`},
		)
	}
	replacements = append(replacements,
		[2]string{"~/.myaaw/.env", "~/.myaaw/agents/" + name + "/.env"},
		[2]string{filepath.Join(root, ".env"), filepath.Join(root, "agents", name, ".env")},
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
