package config

import (
	"fmt"
	"strings"
	"time"
)

// Runtime driver types.
const (
	RuntimeTypeMyaaw      = "myaaw"
	RuntimeTypeACP        = "acp"
	RuntimeTypeClaudeCode = "claude_code"
	RuntimeTypeOMP        = "omp"
	RuntimeTypePi         = "pi"
	RuntimeTypeCodex      = "codex"
)

// Runtime authentication modes.
const (
	AuthSourceCLILogin = "cli_login"
	AuthSourceAPIKey   = "api_key"
)

// Runtime tool permission routing modes.
const (
	PermissionModeBridge   = "bridge"
	PermissionModeAuto     = "auto"
	PermissionModeReadOnly = "read_only"
)

// DefaultRuntimeIdleTimeoutSeconds controls how long an idle warm CLI
// subprocess stays alive per conversation before being reaped.
const DefaultRuntimeIdleTimeoutSeconds = 900

// RuntimeConfig configures which agent execution engine runs turns for this
// agent. When omitted or set to type "myaaw", the built-in Myaaw ReAct engine
// is used with the configured LLM provider.
type RuntimeConfig struct {
	// Type selects the protocol driver: myaaw, acp, claude_code, omp, pi, codex.
	Type string `json:"type,omitempty"`
	// Preset identifies the user-facing catalog entry (e.g. "myaaw",
	// "claude_code", "codex", "antigravity", "opencode", "deepseek_harness",
	// "pi", "omp", "custom_acp").
	Preset string `json:"preset,omitempty"`
	// Command and Args override the preset default binary and arguments.
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	// WorkingDir overrides the subprocess cwd. Empty resolves to
	// ~/.myaaw/agents/<name>/home/workspace.
	WorkingDir string `json:"working_dir,omitempty"`
	// AdditionalDirs grants read/write access to extra host directories
	// alongside WorkingDir and the agent's own home/skills/user dirs.
	AdditionalDirs []string `json:"additional_dirs,omitempty"`
	// Model optionally overrides the external CLI's default model.
	Model string `json:"model,omitempty"`
	// ThinkingLevel optionally overrides the external CLI's reasoning/thinking
	// effort level (values are discovered dynamically from each CLI).
	ThinkingLevel string `json:"thinking_level,omitempty"`
	// SessionMode optionally overrides the external CLI's native session/agent
	// mode (for example ACP modes like "default", "accept-edits", "auto", "plan",
	// or "build", discovered dynamically from the CLI).
	SessionMode string `json:"session_mode,omitempty"`
	// AuthSource declares how the external CLI authenticates: "cli_login" or
	// "api_key".
	AuthSource string `json:"auth_source,omitempty"`
	// APIKeyEnv names the environment variable injected when AuthSource is
	// "api_key" (for example ANTHROPIC_API_KEY, GEMINI_API_KEY, OPENAI_API_KEY).
	APIKeyEnv string `json:"api_key_env,omitempty"`
	// APIKey is an optional secret injected into APIKeyEnv for the subprocess.
	APIKey string `json:"api_key,omitempty"`
	// PermissionMode controls tool approval routing: "bridge" (default), "auto",
	// or "read_only".
	PermissionMode string `json:"permission_mode,omitempty"`
	// MCPBridge attaches Myaaw ecosystem tools (cron, a2a_contact,
	// extra_messages, computer, custom tools) as an MCP server. Nil defaults to true.
	MCPBridge *bool `json:"mcp_bridge,omitempty"`
	// IdleTimeoutSeconds bounds how long an idle warm subprocess stays alive.
	IdleTimeoutSeconds *int `json:"idle_timeout_seconds,omitempty"`
}

// RuntimeCapabilities describes what a runtime supports so the service,
// onboarding flows, and dashboard can adapt cleanly.
type RuntimeCapabilities struct {
	NeedsLLMProvider      bool `json:"needs_llm_provider"`
	ManagesCoreTools      bool `json:"manages_core_tools"`
	SupportsApprovals     bool `json:"supports_approvals"`
	SupportsModelOverride bool `json:"supports_model_override"`
	SupportsImages        bool `json:"supports_images"`
	SupportsMCPBridge     bool `json:"supports_mcp_bridge"`
	SupportsCLIAuth       bool `json:"supports_cli_auth"`
}

// RuntimePreset describes one selectable engine in the CLI wizard and dashboard.
type RuntimePreset struct {
	ID               string              `json:"id"`
	Label            string              `json:"label"`
	Description      string              `json:"description"`
	Type             string              `json:"type"`
	DefaultCommand   string              `json:"default_command,omitempty"`
	DefaultArgs      []string            `json:"default_args,omitempty"`
	DefaultAPIKeyEnv string              `json:"default_api_key_env,omitempty"`
	DefaultAuth      string              `json:"default_auth,omitempty"`
	Capabilities     RuntimeCapabilities `json:"capabilities"`
	ToSNotice        string              `json:"tos_notice,omitempty"`
}

var runtimePresetCatalog = []RuntimePreset{
	{
		ID:          "myaaw",
		Label:       "Myaaw (Built-in)",
		Description: "Myaaw's native ReAct loop paired with an OpenAI-compatible, Gemini, or ChatGPT provider.",
		Type:        RuntimeTypeMyaaw,
		Capabilities: RuntimeCapabilities{
			NeedsLLMProvider:      true,
			ManagesCoreTools:      false,
			SupportsApprovals:     true,
			SupportsModelOverride: true,
			SupportsImages:        true,
			SupportsMCPBridge:     false,
			SupportsCLIAuth:       false,
		},
	},
	// --- Popular official vendor CLIs ---
	{
		ID:               "claude_code",
		Label:            "Claude Code (claude)",
		Description:      "Runs the official Claude Code CLI over bidirectional stream-json with bridged approvals.",
		Type:             RuntimeTypeClaudeCode,
		DefaultCommand:   "claude",
		DefaultAPIKeyEnv: "ANTHROPIC_API_KEY",
		DefaultAuth:      AuthSourceCLILogin,
		Capabilities: RuntimeCapabilities{
			NeedsLLMProvider:      false,
			ManagesCoreTools:      true,
			SupportsApprovals:     true,
			SupportsModelOverride: true,
			SupportsImages:        true,
			SupportsMCPBridge:     true,
			SupportsCLIAuth:       true,
		},
		ToSNotice: "Uses your unmodified local claude binary. Use private mode for personal Pro/Max logins, or ANTHROPIC_API_KEY for public/24x7 automation.",
	},
	{
		ID:               "codex",
		Label:            "OpenAI Codex CLI (codex)",
		Description:      "Runs the official Codex CLI via codex app-server JSON-RPC with bridged approvals.",
		Type:             RuntimeTypeCodex,
		DefaultCommand:   "codex",
		DefaultArgs:      []string{"app-server"},
		DefaultAPIKeyEnv: "OPENAI_API_KEY",
		DefaultAuth:      AuthSourceCLILogin,
		Capabilities: RuntimeCapabilities{
			NeedsLLMProvider:      false,
			ManagesCoreTools:      true,
			SupportsApprovals:     true,
			SupportsModelOverride: true,
			SupportsImages:        true,
			SupportsMCPBridge:     true,
			SupportsCLIAuth:       true,
		},
		ToSNotice: "Uses your unmodified local codex binary. Use private mode on your own machine for ChatGPT logins, or OPENAI_API_KEY for VPS/public/unattended workloads.",
	},
	{
		ID:               "antigravity",
		Label:            "Google Antigravity (agy)",
		Description:      "Runs the official Antigravity CLI over Agent Client Protocol (agy acp).",
		Type:             RuntimeTypeACP,
		DefaultCommand:   "agy",
		DefaultArgs:      []string{"acp", "--disable-telemetry"},
		DefaultAPIKeyEnv: "GEMINI_API_KEY",
		DefaultAuth:      AuthSourceCLILogin,
		Capabilities: RuntimeCapabilities{
			NeedsLLMProvider:      false,
			ManagesCoreTools:      true,
			SupportsApprovals:     true,
			SupportsModelOverride: true,
			SupportsImages:        true,
			SupportsMCPBridge:     true,
			SupportsCLIAuth:       true,
		},
		ToSNotice: "Uses your unmodified local agy binary via ACP. Use private mode for personal CLI logins, or GEMINI_API_KEY for public/unattended workloads.",
	},
	// --- Open-source coding CLIs ---
	{
		ID:               "opencode",
		Label:            "OpenCode (opencode)",
		Description:      "Runs OpenCode over Agent Client Protocol (opencode acp).",
		Type:             RuntimeTypeACP,
		DefaultCommand:   "opencode",
		DefaultArgs:      []string{"acp"},
		DefaultAPIKeyEnv: "OPENAI_API_KEY",
		DefaultAuth:      AuthSourceAPIKey,
		Capabilities: RuntimeCapabilities{
			NeedsLLMProvider:      false,
			ManagesCoreTools:      true,
			SupportsApprovals:     true,
			SupportsModelOverride: true,
			SupportsImages:        true,
			SupportsMCPBridge:     true,
			SupportsCLIAuth:       false,
		},
		ToSNotice: "Configure standard API keys or local models in OpenCode.",
	},
	{
		ID:               "deepseek_harness",
		Label:            "DeepSeek Harness (dsh)",
		Description:      "Runs DeepSeek Harness over Agent Client Protocol (dsh --profile acp).",
		Type:             RuntimeTypeACP,
		DefaultCommand:   "dsh",
		DefaultArgs:      []string{"--profile", "acp"},
		DefaultAPIKeyEnv: "DEEPSEEK_API_KEY",
		DefaultAuth:      AuthSourceCLILogin,
		Capabilities: RuntimeCapabilities{
			NeedsLLMProvider:      false,
			ManagesCoreTools:      true,
			SupportsApprovals:     true,
			SupportsModelOverride: true,
			SupportsImages:        true,
			SupportsMCPBridge:     true,
			SupportsCLIAuth:       true,
		},
		ToSNotice: "Uses DeepSeek Harness (Desktop bundled CLI or npm @deepseek-ai/dsh, Node >= 24.9). Use private mode for personal DeepSeek Account logins, or DEEPSEEK_API_KEY for public/unattended workloads.",
	},
	{
		ID:               "pi",
		Label:            "Pi Coding Agent (pi)",
		Description:      "Runs the Pi coding agent in JSONL RPC mode (pi --mode rpc) with per-agent session files.",
		Type:             RuntimeTypePi,
		DefaultCommand:   "pi",
		DefaultArgs:      []string{"--mode", "rpc"},
		DefaultAPIKeyEnv: "OPENAI_API_KEY",
		DefaultAuth:      AuthSourceAPIKey,
		Capabilities: RuntimeCapabilities{
			NeedsLLMProvider:      false,
			ManagesCoreTools:      true,
			SupportsApprovals:     false,
			SupportsModelOverride: true,
			SupportsImages:        false,
			SupportsMCPBridge:     true,
			SupportsCLIAuth:       false,
		},
		ToSNotice: "Configure standard API keys in pi. Avoid unofficial consumer-OAuth plugins.",
	},
	{
		ID:               "omp",
		Label:            "Oh My Pi (omp)",
		Description:      "Runs the Oh My Pi coding agent in JSONL RPC mode (omp --mode rpc) with per-agent session files.",
		Type:             RuntimeTypeOMP,
		DefaultCommand:   "omp",
		DefaultArgs:      []string{"--mode", "rpc"},
		DefaultAPIKeyEnv: "OPENAI_API_KEY",
		DefaultAuth:      AuthSourceAPIKey,
		Capabilities: RuntimeCapabilities{
			NeedsLLMProvider:      false,
			ManagesCoreTools:      true,
			SupportsApprovals:     false,
			SupportsModelOverride: true,
			SupportsImages:        false,
			SupportsMCPBridge:     true,
			SupportsCLIAuth:       false,
		},
		ToSNotice: "Configure standard API keys (OpenAI, Anthropic, Gemini, OpenRouter, DeepSeek, or local Ollama) in omp. Avoid unofficial consumer-OAuth plugins.",
	},
	// --- Custom ACP ---
	{
		ID:             "custom_acp",
		Label:          "Custom ACP Agent",
		Description:    "Connect any Agent Client Protocol (ACP) compliant CLI binary over JSON-RPC stdio.",
		Type:           RuntimeTypeACP,
		DefaultCommand: "",
		DefaultArgs:    nil,
		DefaultAuth:    AuthSourceAPIKey,
		Capabilities: RuntimeCapabilities{
			NeedsLLMProvider:      false,
			ManagesCoreTools:      true,
			SupportsApprovals:     true,
			SupportsModelOverride: true,
			SupportsImages:        true,
			SupportsMCPBridge:     true,
			SupportsCLIAuth:       true,
		},
	},
}

// RuntimePresets returns a defensive copy of the built-in runtime catalog.
func RuntimePresets() []RuntimePreset {
	out := make([]RuntimePreset, len(runtimePresetCatalog))
	for i, preset := range runtimePresetCatalog {
		if len(preset.DefaultArgs) > 0 {
			preset.DefaultArgs = append([]string(nil), preset.DefaultArgs...)
		}
		out[i] = preset
	}
	return out
}

// LookupRuntimePreset finds a runtime preset by id (case-insensitive).
func LookupRuntimePreset(id string) (RuntimePreset, bool) {
	needle := strings.ToLower(strings.TrimSpace(id))
	for _, preset := range runtimePresetCatalog {
		if preset.ID == needle {
			if len(preset.DefaultArgs) > 0 {
				preset.DefaultArgs = append([]string(nil), preset.DefaultArgs...)
			}
			return preset, true
		}
	}
	return RuntimePreset{}, false
}

// CapabilitiesForRuntime resolves the static capabilities of a runtime config.
func CapabilitiesForRuntime(raw *RuntimeConfig) RuntimeCapabilities {
	merged := MergeRuntime(raw)
	if preset, ok := LookupRuntimePreset(merged.Preset); ok {
		return preset.Capabilities
	}
	if merged.Type == RuntimeTypeMyaaw {
		preset, _ := LookupRuntimePreset("myaaw")
		return preset.Capabilities
	}
	for _, preset := range runtimePresetCatalog {
		if preset.Type == merged.Type {
			return preset.Capabilities
		}
	}
	preset, _ := LookupRuntimePreset("custom_acp")
	return preset.Capabilities
}

// DefaultRuntime returns the shipped default runtime (native Myaaw engine).
func DefaultRuntime() RuntimeConfig {
	mcpBridge := true
	idle := DefaultRuntimeIdleTimeoutSeconds
	return RuntimeConfig{
		Type:               RuntimeTypeMyaaw,
		Preset:             "myaaw",
		PermissionMode:     PermissionModeBridge,
		MCPBridge:          &mcpBridge,
		IdleTimeoutSeconds: &idle,
	}
}

// NormalizeRuntime trims strings and lowercases enum fields.
func NormalizeRuntime(in RuntimeConfig) RuntimeConfig {
	out := in
	out.Type = strings.ToLower(strings.TrimSpace(out.Type))
	out.Preset = strings.ToLower(strings.TrimSpace(out.Preset))
	out.Command = strings.TrimSpace(out.Command)
	out.WorkingDir = strings.TrimSpace(out.WorkingDir)
	out.Model = strings.TrimSpace(out.Model)
	out.ThinkingLevel = strings.TrimSpace(out.ThinkingLevel)
	out.SessionMode = strings.TrimSpace(out.SessionMode)
	out.AuthSource = strings.ToLower(strings.TrimSpace(out.AuthSource))
	out.APIKeyEnv = strings.TrimSpace(out.APIKeyEnv)
	out.APIKey = strings.TrimSpace(out.APIKey)
	out.PermissionMode = strings.ToLower(strings.TrimSpace(out.PermissionMode))

	if len(out.Args) > 0 {
		cleaned := make([]string, 0, len(out.Args))
		for _, arg := range out.Args {
			if trimmed := strings.TrimSpace(arg); trimmed != "" {
				cleaned = append(cleaned, trimmed)
			}
		}
		out.Args = cleaned
	}
	if len(out.AdditionalDirs) > 0 {
		seen := make(map[string]struct{}, len(out.AdditionalDirs))
		cleaned := make([]string, 0, len(out.AdditionalDirs))
		for _, dir := range out.AdditionalDirs {
			trimmed := strings.TrimSpace(dir)
			if trimmed == "" {
				continue
			}
			if _, ok := seen[trimmed]; ok {
				continue
			}
			seen[trimmed] = struct{}{}
			cleaned = append(cleaned, trimmed)
		}
		out.AdditionalDirs = cleaned
	}
	return out
}

// MergeRuntime resolves unset fields from presets and defaults.
func MergeRuntime(raw *RuntimeConfig) RuntimeConfig {
	merged := DefaultRuntime()
	if raw == nil {
		return merged
	}
	n := NormalizeRuntime(*raw)
	if n.Preset != "" {
		merged.Preset = n.Preset
		if preset, ok := LookupRuntimePreset(n.Preset); ok {
			merged.Type = preset.Type
			merged.Command = preset.DefaultCommand
			merged.Args = append([]string(nil), preset.DefaultArgs...)
			merged.APIKeyEnv = preset.DefaultAPIKeyEnv
			merged.AuthSource = preset.DefaultAuth
		}
	}
	if n.Type != "" {
		merged.Type = n.Type
		if n.Preset == "" {
			switch n.Type {
			case RuntimeTypeMyaaw:
				merged.Preset = "myaaw"
			case RuntimeTypeClaudeCode:
				if preset, ok := LookupRuntimePreset("claude_code"); ok {
					merged.Preset = preset.ID
					merged.Command = preset.DefaultCommand
					merged.APIKeyEnv = preset.DefaultAPIKeyEnv
					merged.AuthSource = preset.DefaultAuth
				}
			case RuntimeTypeOMP:
				if preset, ok := LookupRuntimePreset("omp"); ok {
					merged.Preset = preset.ID
					merged.Command = preset.DefaultCommand
					merged.Args = append([]string(nil), preset.DefaultArgs...)
					merged.APIKeyEnv = preset.DefaultAPIKeyEnv
					merged.AuthSource = preset.DefaultAuth
				}
			case RuntimeTypePi:
				if preset, ok := LookupRuntimePreset("pi"); ok {
					merged.Preset = preset.ID
					merged.Command = preset.DefaultCommand
					merged.Args = append([]string(nil), preset.DefaultArgs...)
					merged.APIKeyEnv = preset.DefaultAPIKeyEnv
					merged.AuthSource = preset.DefaultAuth
				}
			case RuntimeTypeCodex:
				if preset, ok := LookupRuntimePreset("codex"); ok {
					merged.Preset = preset.ID
					merged.Command = preset.DefaultCommand
					merged.Args = append([]string(nil), preset.DefaultArgs...)
					merged.APIKeyEnv = preset.DefaultAPIKeyEnv
					merged.AuthSource = preset.DefaultAuth
				}
			case RuntimeTypeACP:
				merged.Preset = "custom_acp"
				merged.AuthSource = AuthSourceAPIKey
			}
		}
	}
	if merged.Type == "" {
		merged.Type = RuntimeTypeMyaaw
	}
	if merged.Preset == "" {
		merged.Preset = merged.Type
	}
	if n.Command != "" {
		merged.Command = n.Command
	}
	if len(n.Args) > 0 {
		merged.Args = append([]string(nil), n.Args...)
	}
	if n.WorkingDir != "" {
		merged.WorkingDir = n.WorkingDir
	}
	if len(n.AdditionalDirs) > 0 {
		merged.AdditionalDirs = append([]string(nil), n.AdditionalDirs...)
	}
	if n.Model != "" {
		merged.Model = n.Model
	}
	if n.ThinkingLevel != "" {
		merged.ThinkingLevel = n.ThinkingLevel
	}
	if n.SessionMode != "" {
		merged.SessionMode = n.SessionMode
	}
	if n.AuthSource != "" {
		merged.AuthSource = n.AuthSource
	}
	if n.APIKeyEnv != "" {
		merged.APIKeyEnv = n.APIKeyEnv
	}
	if n.APIKey != "" {
		merged.APIKey = n.APIKey
	}
	if n.PermissionMode != "" {
		merged.PermissionMode = n.PermissionMode
	}
	if n.MCPBridge != nil {
		v := *n.MCPBridge
		merged.MCPBridge = &v
	}
	if n.IdleTimeoutSeconds != nil && *n.IdleTimeoutSeconds > 0 {
		v := *n.IdleTimeoutSeconds
		merged.IdleTimeoutSeconds = &v
	}
	if merged.Type != RuntimeTypeMyaaw && merged.AuthSource == "" {
		if merged.APIKey != "" {
			merged.AuthSource = AuthSourceAPIKey
		} else {
			merged.AuthSource = AuthSourceCLILogin
		}
	}
	return merged
}

// IsExternal reports whether the resolved runtime uses an external CLI driver.
func (r RuntimeConfig) IsExternal() bool {
	return strings.TrimSpace(r.Type) != "" && strings.ToLower(strings.TrimSpace(r.Type)) != RuntimeTypeMyaaw
}

// MCPBridgeEnabled reports whether Myaaw ecosystem tools should be bridged via MCP.
func (r RuntimeConfig) MCPBridgeEnabled() bool {
	if r.MCPBridge == nil {
		return true
	}
	return *r.MCPBridge
}

// IdleTimeout returns the subprocess idle timeout duration.
func (r RuntimeConfig) IdleTimeout() time.Duration {
	if r.IdleTimeoutSeconds != nil && *r.IdleTimeoutSeconds > 0 {
		return time.Duration(*r.IdleTimeoutSeconds) * time.Second
	}
	return DefaultRuntimeIdleTimeoutSeconds * time.Second
}

// ValidateRuntime checks enum values, binary requirements, and account-safety
// ToS guardrails.
func ValidateRuntime(raw *RuntimeConfig, bot *GlobalBotConfig) error {
	if raw == nil {
		return nil
	}
	merged := MergeRuntime(raw)
	switch merged.Type {
	case RuntimeTypeMyaaw, RuntimeTypeACP, RuntimeTypeClaudeCode, RuntimeTypeOMP, RuntimeTypePi, RuntimeTypeCodex:
	default:
		return fmt.Errorf("runtime.type must be one of myaaw, acp, claude_code, omp, pi, or codex")
	}

	if merged.Preset != "" {
		if _, ok := LookupRuntimePreset(merged.Preset); !ok {
			return fmt.Errorf("runtime.preset %q is not a recognized runtime preset", merged.Preset)
		}
	}

	if merged.Type == RuntimeTypeMyaaw {
		return nil
	}

	if merged.Command == "" {
		return fmt.Errorf("runtime.command is required for runtime %q", merged.Preset)
	}

	switch merged.AuthSource {
	case "", AuthSourceCLILogin, AuthSourceAPIKey:
	default:
		return fmt.Errorf("runtime.auth_source must be %q or %q", AuthSourceCLILogin, AuthSourceAPIKey)
	}

	switch merged.PermissionMode {
	case "", PermissionModeBridge, PermissionModeAuto, PermissionModeReadOnly:
	default:
		return fmt.Errorf("runtime.permission_mode must be %q, %q, or %q", PermissionModeBridge, PermissionModeAuto, PermissionModeReadOnly)
	}

	if merged.IdleTimeoutSeconds != nil && (*merged.IdleTimeoutSeconds < 30 || *merged.IdleTimeoutSeconds > 86400) {
		return fmt.Errorf("runtime.idle_timeout_seconds must be between 30 and 86400")
	}

	// Account safety / ToS guardrail: personal CLI logins must not power public
	// multi-user bots (account sharing / subscription proxying).
	if merged.AuthSource == AuthSourceCLILogin && bot != nil && strings.EqualFold(strings.TrimSpace(bot.Type), "public") {
		return fmt.Errorf("runtime with auth_source %q cannot use bot.type \"public\"; switch bot.type to \"private\" or configure auth_source %q with an official API key", AuthSourceCLILogin, AuthSourceAPIKey)
	}

	return nil
}

// RuntimeWarnings returns non-fatal operational and ToS warnings for an agent's
// runtime and schedule configuration.
func RuntimeWarnings(raw *RuntimeConfig, hb *HeartbeatConfig) []string {
	merged := MergeRuntime(raw)
	if !merged.IsExternal() {
		return nil
	}
	var warnings []string
	if merged.AuthSource == AuthSourceCLILogin && hb != nil && hb.Active {
		if d, err := time.ParseDuration(strings.TrimSpace(hb.Every)); err == nil && d > 0 && d < 15*time.Minute {
			warnings = append(warnings, fmt.Sprintf("heartbeat.every (%s) is high-frequency for auth_source \"cli_login\"; use an official API key (auth_source \"api_key\") for unattended high-frequency automation", hb.Every))
		}
	}
	return warnings
}

// EffectiveRuntime returns the active process's merged runtime configuration.
func EffectiveRuntime() RuntimeConfig {
	return MergeRuntime(&Runtime)
}

// IsExternalRuntime reports whether the active process uses an external CLI runtime.
func IsExternalRuntime() bool {
	return EffectiveRuntime().IsExternal()
}
