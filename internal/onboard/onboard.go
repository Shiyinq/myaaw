// Package onboard contains the agent-creation and configuration core shared by
// the `myaaw onboard` / `agent create` CLI and the control-plane API. The
// interactive TUI lives in cmd/myaaw and builds an Input, then hands it to this
// package so every entry point applies the exact same logic.
package onboard

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	myaaw "myaaw"
	"myaaw/internal/agent/tools"
	"myaaw/internal/agentdir"
	"myaaw/internal/config"
)

// Input is the neutral, flag-agnostic description of what to write. Empty
// fields mean "leave the current value alone", matching the wizard's keep
// semantics.
type Input struct {
	ProviderName       string
	ProviderType       string
	ProviderBaseURL    string
	ProviderAPIKey     string
	ProviderModel      string
	TelegramToken      string
	TelegramMode       string
	TelegramWebhookURL string
	DiscordToken       string
	DisableTelegram    bool
	DisableDiscord     bool
	BotType            string
	Heartbeat          bool
	HeartbeatSet       bool
	HeartbeatChannel   string
	HeartbeatTo        string
	HeartbeatEvery     string
	OwnerIDs           []string
	OwnersSet          bool
	Skills             []string
	DisabledTools      []string
	UpdateTools        bool
}

// Initialize creates the agent layout, registers it, assigns a port, and wires
// A2A. It is safe to call on an existing agent: tokens and ports are preserved.
// setActive controls whether the calling process switches its active agent; the
// control-plane API passes false because it always targets explicit names.
func Initialize(name string, setActive bool) (a2aWarning error, err error) {
	root, err := agentdir.AgentRoot(name)
	if err != nil {
		return nil, err
	}
	if err := setupHome(root); err != nil {
		return nil, fmt.Errorf("create agent layout: %w", err)
	}
	if err := agentdir.RewriteLegacyPaths(root, name); err != nil {
		return nil, fmt.Errorf("rewrite legacy paths: %w", err)
	}
	if err := agentdir.RegisterAgent(name); err != nil {
		return nil, fmt.Errorf("register agent: %w", err)
	}
	if setActive {
		if err := agentdir.SetActive(name); err != nil {
			return nil, err
		}
	}
	if err := ensureDefaultAgent(name); err != nil {
		return nil, fmt.Errorf("set default agent: %w", err)
	}
	if err := PrepareEnv(name, 0); err != nil {
		return nil, err
	}

	// Ensure the A2A token exists and every agent appears in the others'
	// contacts. Existing tokens are preserved, so re-running is safe.
	if contactErr := agentdir.EnsureA2AContacts(name); contactErr != nil {
		a2aWarning = contactErr
	}
	if err := ensureWorkspace(name); err != nil {
		return nil, fmt.Errorf("prepare workspace: %w", err)
	}
	return a2aWarning, nil
}

// Apply merges the input into config.json and returns the written config plus
// non-fatal warnings (for example a failed webhook registration).
func Apply(name string, in Input, port int) (*config.Config, []string, error) {
	// Initialize already assigned a port. Only re-run PrepareEnv when the caller
	// explicitly wants a different one, so a normal run does not take the port
	// lock twice.
	if port > 0 {
		if err := PrepareEnv(name, port); err != nil {
			return nil, nil, err
		}
	}
	root, err := agentdir.AgentRoot(name)
	if err != nil {
		return nil, nil, err
	}
	configPath := filepath.Join(root, "config", "config.json")

	cfg, err := LoadConfig(configPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("read config.json: %w", err)
	}
	if cfg == nil {
		cfg = &config.Config{}
	}
	SanitizeConfig(cfg)

	merged := MergeConfig(cfg, in)
	if err := SaveConfig(configPath, merged); err != nil {
		return nil, nil, fmt.Errorf("write config.json: %w", err)
	}

	if err := InstallSkills(name, in.Skills); err != nil {
		return nil, nil, err
	}
	if in.UpdateTools {
		if err := SaveToolSelection(name, in.DisabledTools); err != nil {
			return nil, nil, err
		}
	}

	var warnings []string
	if in.TelegramToken != "" && in.TelegramMode == "webhook" {
		publicURL := strings.TrimRight(strings.TrimSpace(in.TelegramWebhookURL), "/")
		if publicURL == "" {
			warnings = append(warnings, fmt.Sprintf("webhook mode is enabled but no public URL was set; run `myaaw --agent %s webhook set <domain>`", name))
		} else {
			if envPath, envErr := agentdir.EnvPath(name); envErr == nil {
				_ = agentdir.UpsertEnvValue(envPath, "MYAAW_BASE_URL", publicURL)
				// Contacts advertise MYAAW_BASE_URL, so refresh them after the change.
				if contactErr := agentdir.EnsureA2AContacts(name); contactErr != nil {
					warnings = append(warnings, "A2A contacts were not refreshed for the public URL: "+contactErr.Error())
				}
			}
			if regErr := RegisterTelegramWebhook(in.TelegramToken, publicURL); regErr != nil {
				warnings = append(warnings, "Telegram webhook was not registered: "+regErr.Error())
			}
		}
	}
	return merged, warnings, nil
}

// MergeConfig applies the input to an existing config without mutating base.
func MergeConfig(base *config.Config, in Input) *config.Config {
	cfg := &config.Config{Heartbeat: config.HeartbeatConfig{}}
	if base != nil {
		*cfg = *base
	}

	// Copy the mutable parts so merging never mutates the caller's config.
	providers := make(map[string]config.ProviderConfig, len(cfg.Providers))
	for name, p := range cfg.Providers {
		providers[name] = p
	}
	cfg.Providers = providers
	if cfg.Channels.Telegram != nil {
		tg := *cfg.Channels.Telegram
		cfg.Channels.Telegram = &tg
	}
	if cfg.Channels.Discord != nil {
		dc := *cfg.Channels.Discord
		cfg.Channels.Discord = &dc
	}
	if in.ProviderName != "" {
		cfg.Providers[in.ProviderName] = config.ProviderConfig{
			Type:         in.ProviderType,
			BaseURL:      in.ProviderBaseURL,
			APIKey:       in.ProviderAPIKey,
			DefaultModel: in.ProviderModel,
		}
		cfg.DefaultProvider = in.ProviderName
	}

	if in.TelegramToken != "" {
		tg := cfg.Channels.Telegram
		if tg == nil {
			tg = &config.ChannelConfig{}
		}
		tg.Active = true
		tg.Token = in.TelegramToken
		tg.Mode = in.TelegramMode
		cfg.Channels.Telegram = tg
	}
	if in.DiscordToken != "" {
		dc := cfg.Channels.Discord
		if dc == nil {
			dc = &config.ChannelConfig{}
		}
		dc.Active = true
		dc.Token = in.DiscordToken
		cfg.Channels.Discord = dc
	}
	// Turning a channel off keeps its token so it can be re-enabled later; the
	// adapters only start when active is true.
	if in.DisableTelegram && cfg.Channels.Telegram != nil {
		cfg.Channels.Telegram.Active = false
	}
	if in.DisableDiscord && cfg.Channels.Discord != nil {
		cfg.Channels.Discord.Active = false
	}
	if in.BotType != "" {
		cfg.Bot.Type = in.BotType
	}

	if in.HeartbeatSet {
		if in.Heartbeat {
			cfg.Heartbeat = config.HeartbeatConfig{
				Active:  true,
				Channel: in.HeartbeatChannel,
				To:      in.HeartbeatTo,
				Every:   in.HeartbeatEvery,
			}
		} else {
			cfg.Heartbeat.Active = false
		}
	}

	// OwnersSet records that the owner step was answered, so clearing every ID
	// removes the saved list instead of silently keeping it.
	if in.OwnersSet {
		cfg.Bot.OwnerIDs = append([]string{}, in.OwnerIDs...)
	}

	return cfg
}

const templateHeartbeatTo = "123456789"

// SanitizeConfig strips values that shipped with the old template so a fresh
// install never starts a bot with a fake token or blocks every real user.
func SanitizeConfig(cfg *config.Config) {
	if cfg == nil {
		return
	}
	for name, p := range cfg.Providers {
		if IsTemplateValue(p.APIKey) {
			delete(cfg.Providers, name)
			if cfg.DefaultProvider == name {
				cfg.DefaultProvider = ""
			}
		}
	}
	if cfg.Channels.Telegram != nil && IsTemplateValue(cfg.Channels.Telegram.Token) {
		cfg.Channels.Telegram.Token = ""
		cfg.Channels.Telegram.Active = false
	}
	if cfg.Channels.Discord != nil && IsTemplateValue(cfg.Channels.Discord.Token) {
		cfg.Channels.Discord.Token = ""
		cfg.Channels.Discord.Active = false
	}
	// Drop empty, unused channel blocks so new agents start with a clean file.
	if cfg.Channels.Telegram != nil && !cfg.Channels.Telegram.Active && strings.TrimSpace(cfg.Channels.Telegram.Token) == "" {
		cfg.Channels.Telegram = nil
	}
	if cfg.Channels.Discord != nil && !cfg.Channels.Discord.Active && strings.TrimSpace(cfg.Channels.Discord.Token) == "" {
		cfg.Channels.Discord = nil
	}
	if IsTemplateValue(cfg.Heartbeat.To) || cfg.Heartbeat.To == templateHeartbeatTo {
		cfg.Heartbeat.Active = false
		cfg.Heartbeat.To = ""
	}
	if IsTemplateValue(cfg.Transcriber.APIKey) {
		cfg.Transcriber = config.TranscriberConfig{}
	}
	if IsTemplateValue(cfg.Synthesizer.APIKey) {
		cfg.Synthesizer = config.SynthesizerConfig{}
	}

	owners := cfg.Bot.OwnerIDs[:0]
	for _, id := range cfg.Bot.OwnerIDs {
		if IsTemplateValue(id) {
			continue
		}
		owners = append(owners, id)
	}
	cfg.Bot.OwnerIDs = owners
}

// IsTemplateValue reports whether a value is a leftover YOUR_* placeholder.
func IsTemplateValue(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), "YOUR_")
}

// ValidateTelegramToken rejects empty or obviously malformed bot tokens.
func ValidateTelegramToken(token string) error {
	if token == "" {
		return fmt.Errorf("Telegram bot token is required")
	}
	if !strings.Contains(token, ":") {
		return fmt.Errorf("Telegram bot token looks invalid (expected <bot-id>:<secret>)")
	}
	return nil
}

// ValidateWebhookURL requires a public https base URL without the webhook path.
func ValidateWebhookURL(value string) error {
	if value == "" {
		return fmt.Errorf("a public https:// URL is required for webhook mode")
	}
	if !strings.HasPrefix(value, "https://") {
		return fmt.Errorf("Telegram requires an https:// URL, got %q", value)
	}
	if strings.HasSuffix(value, "/webhook/telegram") {
		return fmt.Errorf("enter only the base URL, without the /webhook/telegram path")
	}
	return nil
}

// ValidateChatID validates a numeric Telegram/Discord chat ID.
func ValidateChatID(value string) error {
	if value == "" {
		return fmt.Errorf("chat ID is required")
	}
	digits := strings.TrimPrefix(value, "-")
	if digits == "" {
		return fmt.Errorf("chat ID must be numeric")
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return fmt.Errorf("chat ID %q must be numeric", value)
		}
	}
	return nil
}

// ParseOwnerIDs splits a comma-separated owner list, de-duplicating entries.
func ParseOwnerIDs(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	seen := make(map[string]bool)
	var ids []string
	for _, part := range strings.Split(raw, ",") {
		id := strings.TrimSpace(part)
		if id == "" {
			continue
		}
		if err := ValidateChatID(id); err != nil {
			return nil, err
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
}

// ParseOnboardSelection validates a comma-separated list (or "all") against the
// known names and returns them in canonical order.
func ParseOnboardSelection(raw string, known []string, flag string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if strings.EqualFold(raw, "all") {
		return append([]string{}, known...), nil
	}

	lookup := make(map[string]string, len(known)+1)
	for _, name := range known {
		lookup[strings.ToLower(name)] = name
	}
	if _, ok := lookup["execute_python"]; ok {
		lookup["python"] = "execute_python"
	}

	chosen := make(map[string]bool)
	for _, part := range strings.Split(raw, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		canonical, ok := lookup[strings.ToLower(name)]
		if !ok {
			return nil, fmt.Errorf("%s: unknown name %q (valid: %s)", flag, name, strings.Join(known, ", "))
		}
		chosen[canonical] = true
	}

	selected := make([]string, 0, len(chosen))
	for _, name := range known {
		if chosen[name] {
			selected = append(selected, name)
		}
	}
	return selected, nil
}

// MergeOwnerIDs concatenates owner ID groups, dropping duplicates.
func MergeOwnerIDs(groups ...[]string) []string {
	seen := make(map[string]bool)
	var merged []string
	for _, group := range groups {
		for _, id := range group {
			if !seen[id] {
				seen[id] = true
				merged = append(merged, id)
			}
		}
	}
	return merged
}

// InstallSkills copies only the chosen skill templates. Existing folders are
// never overwritten, and unselected skills are not created.
func InstallSkills(name string, selected []string) error {
	skillsDir, err := agentdir.SkillsDir(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return err
	}

	known := make(map[string]bool)
	for _, skill := range BuiltinSkillNames() {
		known[skill] = true
	}

	for _, skill := range selected {
		if !known[skill] {
			return fmt.Errorf("unknown skill %q", skill)
		}
		dest := filepath.Join(skillsDir, skill)
		if _, statErr := os.Stat(dest); statErr == nil {
			continue
		}
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return err
		}
		src := path.Join(".myaaw", "skills", skill)
		if err := extractEmbedDir(myaaw.DefaultMyaawDir, src, dest); err != nil {
			return fmt.Errorf("install skill %q: %w", skill, err)
		}
	}
	return nil
}

// SaveToolSelection writes builtin_tools to tools/tools.json while preserving
// custom_tools and mcp_servers.
func SaveToolSelection(name string, disabled []string) error {
	names := BuiltinToolNames()
	known := make(map[string]bool, len(names))
	builtin := make(map[string]bool, len(names))
	for _, tool := range names {
		known[tool] = true
		builtin[tool] = true
	}
	for _, tool := range disabled {
		if !known[tool] {
			return fmt.Errorf("unknown built-in tool %q", tool)
		}
		builtin[tool] = false
	}

	toolsDir, err := agentdir.ToolsDir(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		return err
	}
	configPath := filepath.Join(toolsDir, "tools.json")

	raw := map[string]any{}
	if data, readErr := os.ReadFile(configPath); readErr == nil {
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("read tools.json: %w", err)
		}
	}
	raw["builtin_tools"] = builtin

	data, err := json.MarshalIndent(raw, "", "    ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(configPath, append(data, '\n'))
}

// ExistingDisabledTools returns the built-in tools currently disabled in
// tools/tools.json, in registry order.
func ExistingDisabledTools(agentName string) []string {
	dir, err := agentdir.ToolsDir(agentName)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(dir, "tools.json"))
	if err != nil {
		return nil
	}
	var cfg struct {
		BuiltinTools map[string]bool `json:"builtin_tools"`
	}
	if json.Unmarshal(data, &cfg) != nil {
		return nil
	}

	enabled := make(map[string]bool, len(cfg.BuiltinTools))
	for name, value := range cfg.BuiltinTools {
		enabled[CanonicalToolName(name)] = value
	}

	disabled := make([]string, 0, len(cfg.BuiltinTools))
	for _, tool := range BuiltinToolNames() {
		if value, ok := enabled[tool]; ok && !value {
			disabled = append(disabled, tool)
		}
	}
	return disabled
}

// CanonicalToolName maps the alias accepted by the runtime filter to the name
// used by the registry.
func CanonicalToolName(name string) string {
	if name == "python" {
		return "execute_python"
	}
	return name
}

// BuiltinToolNames lists the tools registered in the built-in registry.
func BuiltinToolNames() []string {
	return tools.GetBuiltinToolNames()
}

// BuiltinSkillNames lists the skill templates embedded in the binary.
func BuiltinSkillNames() []string {
	entries, err := myaaw.DefaultMyaawDir.ReadDir(".myaaw/skills")
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

// LoadConfig reads a config.json file.
func LoadConfig(path string) (*config.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// SaveConfig writes config.json atomically.
func SaveConfig(path string, cfg *config.Config) error {
	data, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, append(data, '\n'))
}

func setupHome(targetDir string) error {
	homeReady := false
	configReady := false
	if _, err := os.Stat(filepath.Join(targetDir, "home", "AGENTS.md")); err == nil {
		homeReady = true
	}
	if _, err := os.Stat(filepath.Join(targetDir, "config", "config.json")); err == nil {
		configReady = true
	}
	if homeReady && configReady {
		return ensurePriceFiles(targetDir)
	}
	// Skills are opt-in: the template copy skips them and onboarding installs
	// only the selected ones.
	if err := extractEmbedDirExcept(myaaw.DefaultMyaawDir, ".myaaw", targetDir, "skills"); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(targetDir, "skills"), 0o755); err != nil {
		return err
	}
	return ensurePriceFiles(targetDir)
}

// ensurePriceFiles adds the price tables to an agent that does not have them
// yet. New agents are seeded from the embedded template; older agents with
// inline prices get their own values written to the files so nothing is lost.
// Existing files are never overwritten.
func ensurePriceFiles(targetDir string) error {
	dir := filepath.Join(targetDir, "config")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	var inline *config.Config
	if data, err := os.ReadFile(filepath.Join(dir, "config.json")); err == nil {
		var cfg config.Config
		if json.Unmarshal(data, &cfg) == nil {
			inline = &cfg
		}
	}

	modelPath := filepath.Join(dir, "model_prices.json")
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		if inline != nil && inline.Observability != nil && len(inline.Observability.ModelPrices) > 0 {
			if err := config.WriteModelPricesFile(dir, inline.Observability.ModelPrices); err != nil {
				return err
			}
		} else if err := writeEmbeddedPriceFile(dir, "model_prices.json"); err != nil {
			return err
		}
	}

	toolPath := filepath.Join(dir, "tool_prices.json")
	if _, err := os.Stat(toolPath); os.IsNotExist(err) {
		if inline != nil && inline.Observability != nil && len(inline.Observability.ToolPrices) > 0 {
			if err := config.WriteToolPricesFile(dir, inline.Observability.ToolPrices); err != nil {
				return err
			}
		} else if err := writeEmbeddedPriceFile(dir, "tool_prices.json"); err != nil {
			return err
		}
	}
	return nil
}

func writeEmbeddedPriceFile(dir, name string) error {
	data, err := myaaw.DefaultMyaawDir.ReadFile(".myaaw/config/" + name)
	if err != nil {
		return err
	}
	return WriteFileAtomic(filepath.Join(dir, name), data)
}

func ensureEnvFile(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, []byte(myaaw.DefaultEnvExample), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func ensureDefaultAgent(name string) error {
	registry, err := agentdir.LoadRegistry()
	if err != nil {
		return err
	}
	if registry.Default != "" && agentdir.AgentInitialized(registry.Default) {
		return nil
	}
	return agentdir.SetDefault(name)
}

// ensureWorkspace creates the named agent's workspace directories without
// depending on the process-wide active agent.
func ensureWorkspace(name string) error {
	homeDir, err := agentdir.Home(name)
	if err != nil {
		return err
	}
	for _, dir := range []string{
		filepath.Join(homeDir, "workspace"),
		filepath.Join(homeDir, "workspace", "projects"),
		filepath.Join(homeDir, "workspace", "downloads"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// WriteFileAtomic writes data through a temp file in the same directory.
func WriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpPath)
	}

	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		// Windows cannot reliably replace an existing file with Rename.
		_ = os.Remove(path)
		if retryErr := os.Rename(tmpPath, path); retryErr != nil {
			os.Remove(tmpPath)
			return retryErr
		}
	}
	return nil
}

func extractEmbedDir(embeddedFS embed.FS, src string, dest string) error {
	return extractEmbedDirExcept(embeddedFS, src, dest)
}

func extractEmbedDirExcept(embeddedFS embed.FS, src string, dest string, skip ...string) error {
	skipSet := make(map[string]bool, len(skip))
	for _, name := range skip {
		skipSet[name] = true
	}

	entries, err := embeddedFS.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() && skipSet[entry.Name()] {
			continue
		}
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		if entry.IsDir() {
			if err := os.MkdirAll(destPath, 0o755); err != nil {
				return err
			}
			if err := extractEmbedDir(embeddedFS, srcPath, destPath); err != nil {
				return err
			}
			continue
		}
		data, err := embeddedFS.ReadFile(srcPath)
		if err != nil {
			return err
		}
		if err := os.WriteFile(destPath, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
