package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"myaaw/internal/agentdir"
)

var (
	ErrToolNotFound      = errors.New("tool not found")
	ErrToolNameTaken     = errors.New("tool name already exists")
	ErrInvalidToolName   = errors.New("invalid tool name")
	ErrUnknownBuiltin    = errors.New("unknown builtin tool")
	ErrInvalidToolConfig = errors.New("invalid tool config")
)

// builtinToolDescriptions mirrors the built-in tool table in the tools docs.
var builtinToolDescriptions = map[string]string{
	"bash":           "Execute system shell commands",
	"filesystem":     "Read, write, search, and manage local files",
	"cron":           "Schedule one-time or recurring tasks and reminders",
	"execute_python": "Dynamic Python code execution in isolated venv",
	"provider":       "Change or switch LLM providers dynamically",
	"subagent":       "Spawn autonomous subagents for task delegation",
}

// builtinToolOrder is the canonical listing order for built-in tools.
var builtinToolOrder = []string{"bash", "filesystem", "cron", "execute_python", "provider", "subagent"}

type toolsConfig struct {
	BuiltinTools map[string]bool   `json:"builtin_tools,omitempty"`
	CustomTools  []customToolEntry `json:"custom_tools"`
	MCPServers   []mcpServerEntry  `json:"mcp_servers"`
}

type customToolEntry struct {
	Name    string          `json:"name"`
	Command string          `json:"command"`
	Args    []string        `json:"args,omitempty"`
	Schema  json.RawMessage `json:"schema,omitempty"`
	Enabled *bool           `json:"enabled,omitempty"`
}

type mcpServerEntry struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Enabled *bool             `json:"enabled,omitempty"`
}

// BuiltinToolView is one built-in tool with its effective state.
type BuiltinToolView struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
}

// CustomToolView is one custom tool entry from tools.json.
type CustomToolView struct {
	Name    string          `json:"name"`
	Command string          `json:"command"`
	Args    []string        `json:"args,omitempty"`
	Schema  json.RawMessage `json:"schema,omitempty"`
	Enabled bool            `json:"enabled"`
}

// MCPServerView is one MCP server entry from tools.json with its env masked.
type MCPServerView struct {
	Name    string      `json:"name"`
	Command string      `json:"command"`
	Args    []string    `json:"args,omitempty"`
	Enabled bool        `json:"enabled"`
	Env     SecretState `json:"env"`
}

// ToolsView is the complete tools listing. Only tools.json entries are shown;
// files inside the tools directory are storage, not active tools.
type ToolsView struct {
	Builtin    []BuiltinToolView `json:"builtin"`
	Custom     []CustomToolView  `json:"custom"`
	MCPServers []MCPServerView   `json:"mcp_servers"`
}

// CustomToolInput creates or replaces one custom_tools entry.
type CustomToolInput struct {
	Name    string          `json:"name"`
	Command string          `json:"command"`
	Args    []string        `json:"args,omitempty"`
	Schema  json.RawMessage `json:"schema,omitempty"`
	Enabled *bool           `json:"enabled,omitempty"`
}

// CustomToolPatch updates fields of one custom_tools entry. Nil fields are
// left untouched; Schema can be JSON null to remove an inline schema.
type CustomToolPatch struct {
	Enabled *bool            `json:"enabled,omitempty"`
	Command *string          `json:"command,omitempty"`
	Args    *[]string        `json:"args,omitempty"`
	Schema  *json.RawMessage `json:"schema,omitempty"`
}

// MCPServerInput creates one mcp_servers entry.
type MCPServerInput struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Enabled *bool             `json:"enabled,omitempty"`
}

// MCPServerPatch updates fields of one mcp_servers entry.
type MCPServerPatch struct {
	Enabled *bool              `json:"enabled,omitempty"`
	Command *string            `json:"command,omitempty"`
	Args    *[]string          `json:"args,omitempty"`
	Env     *map[string]string `json:"env,omitempty"`
}

// ToolsService manages an agent's tools.json. Built-in tools are toggled
// through builtin_tools; custom tools and MCP servers live in their arrays.
type ToolsService struct{}

func NewToolsService() *ToolsService {
	return &ToolsService{}
}

// List reads tools.json fresh from disk. A missing or invalid file behaves
// like the agent loader: all built-ins enabled and no custom tools or MCP
// servers.
func (s *ToolsService) List(agentName string) (ToolsView, error) {
	cfg, err := loadToolsConfig(agentName)
	if err != nil {
		return ToolsView{}, err
	}

	view := ToolsView{
		Builtin:    make([]BuiltinToolView, 0, len(builtinToolOrder)),
		Custom:     make([]CustomToolView, 0, len(cfg.CustomTools)),
		MCPServers: make([]MCPServerView, 0, len(cfg.MCPServers)),
	}
	for _, name := range builtinToolOrder {
		view.Builtin = append(view.Builtin, BuiltinToolView{
			Name:        name,
			Description: builtinToolDescriptions[name],
			Enabled:     builtinEnabled(name, cfg.BuiltinTools),
		})
	}
	for _, entry := range cfg.CustomTools {
		view.Custom = append(view.Custom, customToolView(entry))
	}
	for _, entry := range cfg.MCPServers {
		view.MCPServers = append(view.MCPServers, mcpServerView(entry))
	}
	sort.Slice(view.Custom, func(i, j int) bool { return view.Custom[i].Name < view.Custom[j].Name })
	sort.Slice(view.MCPServers, func(i, j int) bool { return view.MCPServers[i].Name < view.MCPServers[j].Name })
	return view, nil
}

// ToggleBuiltin enables or disables one built-in tool via builtin_tools.
func (s *ToolsService) ToggleBuiltin(agentName, name string, enabled bool) error {
	if err := validateBuiltinName(name); err != nil {
		return err
	}
	cfg, err := loadToolsConfig(agentName)
	if err != nil {
		return err
	}
	if cfg.BuiltinTools == nil {
		cfg.BuiltinTools = make(map[string]bool)
	}
	cfg.BuiltinTools[name] = enabled
	return saveToolsConfig(agentName, cfg)
}

// CreateCustom appends one entry to custom_tools.
func (s *ToolsService) CreateCustom(agentName string, in CustomToolInput) (CustomToolView, error) {
	cfg, err := loadToolsConfig(agentName)
	if err != nil {
		return CustomToolView{}, err
	}
	if err := validateCustomInput(in.Name, in.Command); err != nil {
		return CustomToolView{}, err
	}
	for _, entry := range cfg.CustomTools {
		if strings.EqualFold(entry.Name, in.Name) {
			return CustomToolView{}, fmt.Errorf("%w: %s", ErrToolNameTaken, in.Name)
		}
	}
	for _, server := range cfg.MCPServers {
		if strings.HasPrefix(in.Name, server.Name+"_") {
			return CustomToolView{}, fmt.Errorf("%w: %s collides with MCP server %q prefix", ErrInvalidToolName, in.Name, server.Name)
		}
	}
	entry := customToolEntry{
		Name:    in.Name,
		Command: in.Command,
		Args:    in.Args,
		Schema:  normalizeSchema(in.Schema),
		Enabled: in.Enabled,
	}
	if err := validateCustomSchema(entry.Name, entry.Schema); err != nil {
		return CustomToolView{}, err
	}
	cfg.CustomTools = append(cfg.CustomTools, entry)
	if err := saveToolsConfig(agentName, cfg); err != nil {
		return CustomToolView{}, err
	}
	return customToolView(entry), nil
}

// PatchCustom updates fields of one custom_tools entry.
func (s *ToolsService) PatchCustom(agentName, name string, patch CustomToolPatch) (CustomToolView, error) {
	cfg, err := loadToolsConfig(agentName)
	if err != nil {
		return CustomToolView{}, err
	}
	idx, entry, err := findCustomTool(cfg, name)
	if err != nil {
		return CustomToolView{}, err
	}
	if patch.Command != nil {
		entry.Command = strings.TrimSpace(*patch.Command)
		if entry.Command == "" {
			return CustomToolView{}, fmt.Errorf("%w: command must not be empty", ErrInvalidToolConfig)
		}
	}
	if patch.Args != nil {
		entry.Args = *patch.Args
	}
	if patch.Schema != nil {
		entry.Schema = normalizeSchema(*patch.Schema)
		if err := validateCustomSchema(entry.Name, entry.Schema); err != nil {
			return CustomToolView{}, err
		}
	}
	if patch.Enabled != nil {
		entry.Enabled = patch.Enabled
	}
	cfg.CustomTools[idx] = entry
	if err := saveToolsConfig(agentName, cfg); err != nil {
		return CustomToolView{}, err
	}
	return customToolView(entry), nil
}

// DeleteCustom removes one custom_tools entry. Files in the tools directory
// are never touched.
func (s *ToolsService) DeleteCustom(agentName, name string) error {
	cfg, err := loadToolsConfig(agentName)
	if err != nil {
		return err
	}
	idx, _, err := findCustomTool(cfg, name)
	if err != nil {
		return err
	}
	cfg.CustomTools = append(cfg.CustomTools[:idx], cfg.CustomTools[idx+1:]...)
	return saveToolsConfig(agentName, cfg)
}

// CreateMCP appends one entry to mcp_servers.
func (s *ToolsService) CreateMCP(agentName string, in MCPServerInput) (MCPServerView, error) {
	cfg, err := loadToolsConfig(agentName)
	if err != nil {
		return MCPServerView{}, err
	}
	if err := validateMCPInput(in.Name, in.Command); err != nil {
		return MCPServerView{}, err
	}
	for _, entry := range cfg.MCPServers {
		if strings.EqualFold(entry.Name, in.Name) {
			return MCPServerView{}, fmt.Errorf("%w: %s", ErrToolNameTaken, in.Name)
		}
	}
	for _, tool := range cfg.CustomTools {
		if strings.HasPrefix(tool.Name, in.Name+"_") {
			return MCPServerView{}, fmt.Errorf("%w: MCP server name %q collides with custom tool %q", ErrInvalidToolName, in.Name, tool.Name)
		}
	}
	entry := mcpServerEntry{
		Name:    in.Name,
		Command: in.Command,
		Args:    in.Args,
		Env:     in.Env,
		Enabled: in.Enabled,
	}
	cfg.MCPServers = append(cfg.MCPServers, entry)
	if err := saveToolsConfig(agentName, cfg); err != nil {
		return MCPServerView{}, err
	}
	return mcpServerView(entry), nil
}

// PatchMCP updates fields of one mcp_servers entry. A provided env map
// replaces the previous one entirely.
func (s *ToolsService) PatchMCP(agentName, name string, patch MCPServerPatch) (MCPServerView, error) {
	cfg, err := loadToolsConfig(agentName)
	if err != nil {
		return MCPServerView{}, err
	}
	idx, entry, err := findMCPServer(cfg, name)
	if err != nil {
		return MCPServerView{}, err
	}
	if patch.Command != nil {
		entry.Command = strings.TrimSpace(*patch.Command)
		if entry.Command == "" {
			return MCPServerView{}, fmt.Errorf("%w: command must not be empty", ErrInvalidToolConfig)
		}
	}
	if patch.Args != nil {
		entry.Args = *patch.Args
	}
	if patch.Env != nil {
		entry.Env = *patch.Env
	}
	if patch.Enabled != nil {
		entry.Enabled = patch.Enabled
	}
	cfg.MCPServers[idx] = entry
	if err := saveToolsConfig(agentName, cfg); err != nil {
		return MCPServerView{}, err
	}
	return mcpServerView(entry), nil
}

// DeleteMCP removes one mcp_servers entry. It does not stop a running server
// process; the agent server closes it on the next reload or restart.
func (s *ToolsService) DeleteMCP(agentName, name string) error {
	cfg, err := loadToolsConfig(agentName)
	if err != nil {
		return err
	}
	idx, _, err := findMCPServer(cfg, name)
	if err != nil {
		return err
	}
	cfg.MCPServers = append(cfg.MCPServers[:idx], cfg.MCPServers[idx+1:]...)
	return saveToolsConfig(agentName, cfg)
}

func customToolView(entry customToolEntry) CustomToolView {
	return CustomToolView{
		Name:    entry.Name,
		Command: entry.Command,
		Args:    entry.Args,
		Schema:  normalizeSchema(entry.Schema),
		Enabled: entry.Enabled == nil || *entry.Enabled,
	}
}

func mcpServerView(entry mcpServerEntry) MCPServerView {
	view := MCPServerView{
		Name:    entry.Name,
		Command: entry.Command,
		Args:    entry.Args,
		Enabled: entry.Enabled == nil || *entry.Enabled,
	}
	if len(entry.Env) > 0 {
		view.Env = SecretState{Configured: true, Masked: "********"}
	}
	return view
}

func findCustomTool(cfg *toolsConfig, name string) (int, customToolEntry, error) {
	for i, entry := range cfg.CustomTools {
		if entry.Name == name {
			return i, entry, nil
		}
	}
	return 0, customToolEntry{}, ErrToolNotFound
}

func findMCPServer(cfg *toolsConfig, name string) (int, mcpServerEntry, error) {
	for i, entry := range cfg.MCPServers {
		if entry.Name == name {
			return i, entry, nil
		}
	}
	return 0, mcpServerEntry{}, ErrToolNotFound
}

func validateBuiltinName(name string) error {
	if _, ok := builtinToolDescriptions[name]; ok {
		return nil
	}
	if name == "python" { // alias handled by the loader
		return nil
	}
	return fmt.Errorf("%w: %s", ErrUnknownBuiltin, name)
}

func validateCustomInput(name, command string) error {
	return validateManagedToolName(name, command)
}

func validateMCPInput(name, command string) error {
	return validateManagedToolName(name, command)
}

func validateManagedToolName(name, command string) error {
	if name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidToolName)
	}
	if strings.ContainsAny(name, "/\\\x00") {
		return fmt.Errorf("%w: %s", ErrInvalidToolName, name)
	}
	for _, reserved := range builtinToolOrder {
		if strings.EqualFold(reserved, name) {
			return fmt.Errorf("%w: %s is a built-in tool name", ErrInvalidToolName, name)
		}
	}
	if strings.EqualFold(name, "python") {
		return fmt.Errorf("%w: %s is a built-in tool name", ErrInvalidToolName, name)
	}
	if strings.TrimSpace(command) == "" {
		return fmt.Errorf("%w: command is required", ErrInvalidToolConfig)
	}
	return nil
}

// validateCustomSchema enforces the OpenAI function schema format documented
// for custom tools: valid JSON with type "function" and a function.name that
// matches the tool name.
func validateCustomSchema(name string, raw json.RawMessage) error {
	if raw == nil {
		return nil
	}
	var schema struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		return fmt.Errorf("%w: schema must be valid JSON", ErrInvalidToolConfig)
	}
	if schema.Type != "function" {
		return fmt.Errorf("%w: schema type must be \"function\"", ErrInvalidToolConfig)
	}
	if schema.Function.Name == "" {
		return fmt.Errorf("%w: schema function.name is required", ErrInvalidToolConfig)
	}
	if schema.Function.Name != name {
		return fmt.Errorf("%w: schema function.name %q must match tool name %q", ErrInvalidToolConfig, schema.Function.Name, name)
	}
	return nil
}

// normalizeSchema converts JSON null to nil so a cleared schema falls back to
// the loader's --schema discovery.
func normalizeSchema(raw json.RawMessage) json.RawMessage {
	trimmed := strings.TrimSpace(string(raw))
	if raw == nil || trimmed == "" || trimmed == "null" {
		return nil
	}
	return raw
}

func builtinEnabled(name string, filter map[string]bool) bool {
	if filter == nil {
		return true
	}
	if enabled, ok := filter[name]; ok {
		return enabled
	}
	switch name {
	case "execute_python":
		if enabled, ok := filter["python"]; ok {
			return enabled
		}
	case "python":
		if enabled, ok := filter["execute_python"]; ok {
			return enabled
		}
	}
	return true
}

// loadToolsConfig returns an empty config for a missing or invalid tools.json,
// matching the agent loader's defaults.
func loadToolsConfig(agentName string) (*toolsConfig, error) {
	if err := agentdirValidateAgent(agentName); err != nil {
		return nil, err
	}
	dir, err := agentdir.ToolsDir(agentName)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "tools.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return &toolsConfig{}, nil
		}
		return nil, err
	}
	var cfg toolsConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return &toolsConfig{}, nil
	}
	if cfg.BuiltinTools == nil {
		cfg.BuiltinTools = make(map[string]bool)
	}
	return &cfg, nil
}

func saveToolsConfig(agentName string, cfg *toolsConfig) error {
	if err := agentdirValidateAgent(agentName); err != nil {
		return err
	}
	dir, err := agentdir.ToolsDir(agentName)
	if err != nil {
		return err
	}
	if cfg.BuiltinTools == nil {
		cfg.BuiltinTools = make(map[string]bool)
	}
	if cfg.CustomTools == nil {
		cfg.CustomTools = []customToolEntry{}
	}
	if cfg.MCPServers == nil {
		cfg.MCPServers = []mcpServerEntry{}
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "tools.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
