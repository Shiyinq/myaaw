package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	agentruntime "myaaw/internal/agent/runtime"
	"myaaw/internal/config"
)

var (
	// ErrRuntimeManagementDisabled is returned when host runtime management was
	// explicitly disabled via MYAAW_RUNTIME_MANAGEMENT=0.
	ErrRuntimeManagementDisabled = errors.New("external runtime host management is disabled on this server")
	// ErrRuntimeNotFound is returned when an unknown runtime preset ID is given.
	ErrRuntimeNotFound = errors.New("runtime preset not found")
	// ErrRuntimeBusy is returned when an install or update job is already running
	// for the requested runtime preset.
	ErrRuntimeBusy = errors.New("an installation or update is already in progress for this runtime")
)

type storedRuntimeDefaults struct {
	Preset         string
	Command        string
	Args           []string
	Model          string
	ThinkingLevel  string
	SessionMode    string
	AuthSource     string
	APIKeyEnv      string
	APIKey         string
	PermissionMode string
	MCPBridge      bool
	UpdatedAt      string
}

// RuntimeDefaultsView is the public read representation of global defaults for
// one external runtime preset, with any saved API key masked.
type RuntimeDefaultsView struct {
	Preset         string      `json:"preset"`
	Command        string      `json:"command,omitempty"`
	Args           []string    `json:"args,omitempty"`
	Model          string      `json:"model,omitempty"`
	ThinkingLevel  string      `json:"thinking_level,omitempty"`
	SessionMode    string      `json:"session_mode,omitempty"`
	AuthSource     string      `json:"auth_source,omitempty"`
	APIKeyEnv      string      `json:"api_key_env,omitempty"`
	APIKey         SecretState `json:"api_key"`
	PermissionMode string      `json:"permission_mode,omitempty"`
	MCPBridge      bool        `json:"mcp_bridge"`
	UpdatedAt      string      `json:"updated_at,omitempty"`
}

// RuntimeDefaultsInput updates global defaults for one external runtime preset.
type RuntimeDefaultsInput struct {
	Command        string   `json:"command,omitempty"`
	Args           []string `json:"args,omitempty"`
	Model          string   `json:"model,omitempty"`
	ThinkingLevel  string   `json:"thinking_level,omitempty"`
	SessionMode    string   `json:"session_mode,omitempty"`
	AuthSource     string   `json:"auth_source,omitempty"`
	APIKeyEnv      string   `json:"api_key_env,omitempty"`
	APIKey         string   `json:"api_key,omitempty"`
	ClearAPIKey    bool     `json:"clear_api_key,omitempty"`
	PermissionMode string   `json:"permission_mode,omitempty"`
	MCPBridge      *bool    `json:"mcp_bridge,omitempty"`
	ApplyToAgents  []string `json:"apply_to_agents,omitempty"`
}

// ManagedRuntimeView combines preset metadata, host installation detection,
// official install/update recipes, non-intrusive CLI auth status, saved global
// defaults, and the list of Myaaw agents currently using the runtime.
type ManagedRuntimeView struct {
	agentruntime.DetectedRuntime
	DocsURL        string                       `json:"docs_url,omitempty"`
	InstallMethods []agentruntime.InstallMethod `json:"install_methods"`
	UpdateMethod   *agentruntime.InstallMethod  `json:"update_method,omitempty"`
	Auth           agentruntime.AuthStatusInfo  `json:"auth"`
	Defaults       RuntimeDefaultsView          `json:"defaults"`
	UsedByAgents   []string                     `json:"used_by_agents"`
}

// ManagedRuntimesResponse is returned by GET /runtimes/manage.
type ManagedRuntimesResponse struct {
	ManagementEnabled bool                 `json:"management_enabled"`
	Runtimes          []ManagedRuntimeView `json:"runtimes"`
}

// RuntimeManageService owns global runtime defaults, official host installation
// jobs, and non-intrusive CLI authentication status probes.
type RuntimeManageService struct {
	cpdb    *ControlPlaneDB
	agents  *AgentService
	enabled bool
	pty     *RuntimeAuthPTYService

	jobMu      sync.Mutex
	installing map[string]bool
}

// NewRuntimeManageService builds a RuntimeManageService backed by cpdb.
func NewRuntimeManageService(cpdb *ControlPlaneDB, agents *AgentService) *RuntimeManageService {
	rawFlag := strings.ToLower(strings.TrimSpace(os.Getenv("MYAAW_RUNTIME_MANAGEMENT")))
	enabled := rawFlag != "0" && rawFlag != "false" && rawFlag != "off"
	svc := &RuntimeManageService{
		cpdb:       cpdb,
		agents:     agents,
		enabled:    enabled,
		installing: make(map[string]bool),
	}
	svc.pty = NewRuntimeAuthPTYService(svc)
	return svc
}

// Enabled reports whether host CLI installation and interactive login are enabled.
func (s *RuntimeManageService) Enabled() bool {
	return s != nil && s.enabled
}

// PTY returns the scoped interactive CLI authentication PTY service.
func (s *RuntimeManageService) PTY() *RuntimeAuthPTYService {
	if s == nil {
		return nil
	}
	return s.pty
}

// List returns the full management catalog for all external runtime presets.
// When probeAuth is true, installed CLIs are probed concurrently for login status.
func (s *RuntimeManageService) List(ctx context.Context, probeAuth, refresh bool) (ManagedRuntimesResponse, error) {
	detected := agentruntime.Discover(ctx)
	defaultsMap, err := s.loadAllDefaults()
	if err != nil {
		return ManagedRuntimesResponse{}, err
	}
	usedMap := s.agentsByPreset()

	external := make([]agentruntime.DetectedRuntime, 0, len(detected))
	for _, rt := range detected {
		if rt.ID == config.RuntimeTypeMyaaw {
			continue
		}
		external = append(external, rt)
	}

	out := make([]ManagedRuntimeView, len(external))
	var wg sync.WaitGroup
	for i, rt := range external {
		idx := i
		entry := rt
		def, hasDef := defaultsMap[entry.ID]
		if !hasDef {
			def = defaultStoredDefaults(entry.RuntimePreset)
		}
		// If the user configured a custom binary command in global defaults,
		// resolve installation & version against that command.
		if customCmd := strings.TrimSpace(def.Command); customCmd != "" {
			if path, rErr := agentruntime.ResolveBinary(customCmd); rErr == nil {
				entry.Installed = true
				entry.BinaryPath = path
				entry.Version = probeCustomVersion(ctx, path)
			} else {
				entry.Installed = false
				entry.BinaryPath = ""
				entry.Version = ""
			}
		}

		methods := agentruntime.InstallMethodsForPreset(entry.ID)
		if methods == nil {
			methods = []agentruntime.InstallMethod{}
		}
		var updateMethod *agentruntime.InstallMethod
		if entry.Installed {
			updateMethod = agentruntime.UpdateMethodForPreset(entry.ID, entry.BinaryPath)
		}
		used := usedMap[entry.ID]
		if used == nil {
			used = []string{}
		}

		view := ManagedRuntimeView{
			DetectedRuntime: entry,
			DocsURL:         agentruntime.OfficialDocsURL(entry.ID),
			InstallMethods:  methods,
			UpdateMethod:    updateMethod,
			Auth:            agentruntime.AuthActionMetadata(entry.ID),
			Defaults:        toDefaultsView(def),
			UsedByAgents:    used,
		}
		if def.APIKey != "" {
			view.Auth.Status = agentruntime.AuthStatusAPIKeyConfigured
			view.Auth.Detail = def.APIKeyEnv
		}
		out[idx] = view

		if probeAuth && entry.Installed {
			wg.Add(1)
			go func(slot int, preset config.RuntimePreset, binPath string, saved storedRuntimeDefaults) {
				defer wg.Done()
				out[slot].Auth = agentruntime.ProbeAuthStatusCached(
					ctx,
					preset,
					binPath,
					saved.APIKeyEnv,
					saved.APIKey != "",
					refresh,
				)
			}(idx, entry.RuntimePreset, entry.BinaryPath, def)
		}
	}
	wg.Wait()

	return ManagedRuntimesResponse{
		ManagementEnabled: s.Enabled(),
		Runtimes:          out,
	}, nil
}

// GetOne returns the refreshed ManagedRuntimeView (including live auth probe)
// for a single external runtime preset.
func (s *RuntimeManageService) GetOne(ctx context.Context, presetID string) (ManagedRuntimeView, error) {
	id := strings.ToLower(strings.TrimSpace(presetID))
	preset, ok := config.LookupRuntimePreset(id)
	if !ok || preset.ID == config.RuntimeTypeMyaaw {
		return ManagedRuntimeView{}, ErrRuntimeNotFound
	}
	agentruntime.InvalidateAuthStatusCache(preset.ID)
	res, err := s.List(ctx, false, false)
	if err != nil {
		return ManagedRuntimeView{}, err
	}
	for _, item := range res.Runtimes {
		if item.ID == preset.ID {
			if item.Installed {
				rawDef, _ := s.RawDefaults(preset.ID)
				item.Auth = agentruntime.ProbeAuthStatusCached(
					ctx,
					preset,
					item.BinaryPath,
					rawDef.APIKeyEnv,
					rawDef.APIKey != "",
					true,
				)
			}
			return item, nil
		}
	}
	return ManagedRuntimeView{}, ErrRuntimeNotFound
}

// SaveDefaults creates or updates the global default configuration for presetID,
// and optionally propagates the defaults to selected existing agents.
func (s *RuntimeManageService) SaveDefaults(ctx context.Context, presetID string, in RuntimeDefaultsInput) (ManagedRuntimeView, error) {
	id := strings.ToLower(strings.TrimSpace(presetID))
	preset, ok := config.LookupRuntimePreset(id)
	if !ok || preset.ID == config.RuntimeTypeMyaaw {
		return ManagedRuntimeView{}, ErrRuntimeNotFound
	}
	if s == nil || s.cpdb == nil {
		return ManagedRuntimeView{}, errors.New("control plane database unavailable")
	}

	existing, hasExisting := s.RawDefaults(preset.ID)
	if !hasExisting {
		existing = defaultStoredDefaults(preset)
	}

	authSource := strings.ToLower(strings.TrimSpace(in.AuthSource))
	if authSource == "" {
		authSource = existing.AuthSource
	}
	if authSource != config.AuthSourceCLILogin && authSource != config.AuthSourceAPIKey {
		return ManagedRuntimeView{}, fmt.Errorf("%w: auth_source must be cli_login or api_key", ErrInvalidConfig)
	}

	permMode := strings.ToLower(strings.TrimSpace(in.PermissionMode))
	if permMode == "" {
		permMode = existing.PermissionMode
	}
	if permMode != config.PermissionModeBridge && permMode != config.PermissionModeAuto && permMode != config.PermissionModeReadOnly {
		return ManagedRuntimeView{}, fmt.Errorf("%w: permission_mode must be bridge, auto, or read_only", ErrInvalidConfig)
	}

	apiKey := existing.APIKey
	if in.ClearAPIKey {
		apiKey = ""
	} else if trimmedKey := strings.TrimSpace(in.APIKey); trimmedKey != "" {
		apiKey = trimmedKey
	}

	apiKeyEnv := strings.TrimSpace(in.APIKeyEnv)
	if apiKeyEnv == "" {
		apiKeyEnv = preset.DefaultAPIKeyEnv
	}

	mcpBridge := existing.MCPBridge
	if in.MCPBridge != nil {
		mcpBridge = *in.MCPBridge
	}

	cleanArgs := make([]string, 0, len(in.Args))
	for _, a := range in.Args {
		if t := strings.TrimSpace(a); t != "" {
			cleanArgs = append(cleanArgs, t)
		}
	}
	argsBytes, err := json.Marshal(cleanArgs)
	if err != nil {
		return ManagedRuntimeView{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	mcpInt := 0
	if mcpBridge {
		mcpInt = 1
	}

	if _, err := s.cpdb.DB().Exec(
		`INSERT INTO runtime_defaults (
			preset, command, args_json, model, thinking_level, session_mode,
			auth_source, api_key_env, api_key, permission_mode, mcp_bridge, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(preset) DO UPDATE SET
			command         = excluded.command,
			args_json       = excluded.args_json,
			model           = excluded.model,
			thinking_level  = excluded.thinking_level,
			session_mode    = excluded.session_mode,
			auth_source     = excluded.auth_source,
			api_key_env     = excluded.api_key_env,
			api_key         = excluded.api_key,
			permission_mode = excluded.permission_mode,
			mcp_bridge      = excluded.mcp_bridge,
			updated_at      = excluded.updated_at`,
		preset.ID,
		strings.TrimSpace(in.Command),
		string(argsBytes),
		strings.TrimSpace(in.Model),
		strings.TrimSpace(in.ThinkingLevel),
		strings.TrimSpace(in.SessionMode),
		authSource,
		apiKeyEnv,
		apiKey,
		permMode,
		mcpInt,
		now,
	); err != nil {
		return ManagedRuntimeView{}, fmt.Errorf("save runtime defaults: %w", err)
	}

	if len(in.ApplyToAgents) > 0 {
		saved, _ := s.RawDefaults(preset.ID)
		s.applyDefaultsToAgents(preset, saved, in.ApplyToAgents)
	}

	return s.GetOne(ctx, preset.ID)
}

// Install runs the compiled-in official vendor installer for (presetID, methodID)
// and returns the refreshed runtime view when finished.
func (s *RuntimeManageService) Install(ctx context.Context, presetID, methodID string, onLine func(line string)) (ManagedRuntimeView, error) {
	if !s.Enabled() {
		return ManagedRuntimeView{}, ErrRuntimeManagementDisabled
	}
	id := strings.ToLower(strings.TrimSpace(presetID))
	preset, ok := config.LookupRuntimePreset(id)
	if !ok || preset.ID == config.RuntimeTypeMyaaw {
		return ManagedRuntimeView{}, ErrRuntimeNotFound
	}
	if !s.acquireJob(preset.ID) {
		return ManagedRuntimeView{}, ErrRuntimeBusy
	}
	defer s.releaseJob(preset.ID)

	if err := agentruntime.RunOfficialInstall(ctx, preset.ID, methodID, onLine); err != nil {
		return ManagedRuntimeView{}, err
	}
	return s.GetOne(ctx, preset.ID)
}

// Update runs the compiled-in official vendor update command for presetID.
func (s *RuntimeManageService) Update(ctx context.Context, presetID string, onLine func(line string)) (ManagedRuntimeView, error) {
	if !s.Enabled() {
		return ManagedRuntimeView{}, ErrRuntimeManagementDisabled
	}
	id := strings.ToLower(strings.TrimSpace(presetID))
	preset, ok := config.LookupRuntimePreset(id)
	if !ok || preset.ID == config.RuntimeTypeMyaaw {
		return ManagedRuntimeView{}, ErrRuntimeNotFound
	}
	if !s.acquireJob(preset.ID) {
		return ManagedRuntimeView{}, ErrRuntimeBusy
	}
	defer s.releaseJob(preset.ID)

	current, err := s.GetOne(ctx, preset.ID)
	if err != nil {
		return ManagedRuntimeView{}, err
	}
	if err := agentruntime.RunOfficialUpdate(ctx, preset.ID, current.BinaryPath, onLine); err != nil {
		return ManagedRuntimeView{}, err
	}
	return s.GetOne(ctx, preset.ID)
}

// RawDefaults returns the unmasked stored defaults for internal server use
// (seeding new agents on onboard and injecting saved global keys during option probes).
func (s *RuntimeManageService) RawDefaults(presetID string) (storedRuntimeDefaults, bool) {
	if s == nil || s.cpdb == nil {
		return storedRuntimeDefaults{}, false
	}
	id := strings.ToLower(strings.TrimSpace(presetID))
	preset, ok := config.LookupRuntimePreset(id)
	if !ok {
		return storedRuntimeDefaults{}, false
	}
	var row storedRuntimeDefaults
	var argsJSON string
	var mcpInt int
	err := s.cpdb.DB().QueryRow(
		`SELECT preset, command, args_json, model, thinking_level, session_mode,
		        auth_source, api_key_env, api_key, permission_mode, mcp_bridge, updated_at
		 FROM runtime_defaults WHERE preset = ?`,
		preset.ID,
	).Scan(
		&row.Preset,
		&row.Command,
		&argsJSON,
		&row.Model,
		&row.ThinkingLevel,
		&row.SessionMode,
		&row.AuthSource,
		&row.APIKeyEnv,
		&row.APIKey,
		&row.PermissionMode,
		&mcpInt,
		&row.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) || err != nil {
		return storedRuntimeDefaults{}, false
	}
	_ = json.Unmarshal([]byte(argsJSON), &row.Args)
	row.MCPBridge = mcpInt != 0
	return row, true
}

func (s *RuntimeManageService) loadAllDefaults() (map[string]storedRuntimeDefaults, error) {
	out := make(map[string]storedRuntimeDefaults)
	if s == nil || s.cpdb == nil {
		return out, nil
	}
	rows, err := s.cpdb.DB().Query(
		`SELECT preset, command, args_json, model, thinking_level, session_mode,
		        auth_source, api_key_env, api_key, permission_mode, mcp_bridge, updated_at
		 FROM runtime_defaults`,
	)
	if err != nil {
		return nil, fmt.Errorf("query runtime_defaults: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var row storedRuntimeDefaults
		var argsJSON string
		var mcpInt int
		if err := rows.Scan(
			&row.Preset,
			&row.Command,
			&argsJSON,
			&row.Model,
			&row.ThinkingLevel,
			&row.SessionMode,
			&row.AuthSource,
			&row.APIKeyEnv,
			&row.APIKey,
			&row.PermissionMode,
			&mcpInt,
			&row.UpdatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(argsJSON), &row.Args)
		row.MCPBridge = mcpInt != 0
		out[row.Preset] = row
	}
	return out, rows.Err()
}

func (s *RuntimeManageService) agentsByPreset() map[string][]string {
	out := make(map[string][]string)
	if s == nil || s.agents == nil {
		return out
	}
	list, err := s.agents.List()
	if err != nil {
		return out
	}
	for _, ag := range list {
		preset := strings.TrimSpace(ag.RuntimePreset)
		if preset == "" {
			preset = strings.TrimSpace(ag.RuntimeType)
		}
		if preset == "" || preset == config.RuntimeTypeMyaaw {
			continue
		}
		out[preset] = append(out[preset], ag.Name)
	}
	return out
}

func (s *RuntimeManageService) applyDefaultsToAgents(preset config.RuntimePreset, def storedRuntimeDefaults, names []string) {
	for _, rawName := range names {
		name := strings.TrimSpace(rawName)
		cfg, err := loadAgentConfig(name)
		if err != nil || cfg == nil || cfg.Runtime == nil {
			continue
		}
		merged := config.MergeRuntime(cfg.Runtime)
		if merged.Preset != preset.ID {
			continue
		}
		// Never apply cli_login to a public bot.
		if def.AuthSource == config.AuthSourceCLILogin && cfg.Bot.Type == "public" {
			continue
		}
		if def.Command != "" {
			merged.Command = def.Command
		}
		if len(def.Args) > 0 {
			merged.Args = append([]string(nil), def.Args...)
		}
		merged.Model = def.Model
		merged.ThinkingLevel = def.ThinkingLevel
		merged.SessionMode = def.SessionMode
		if def.AuthSource != "" {
			merged.AuthSource = def.AuthSource
		}
		if def.APIKeyEnv != "" {
			merged.APIKeyEnv = def.APIKeyEnv
		}
		if def.APIKey != "" {
			merged.APIKey = def.APIKey
		}
		if def.PermissionMode != "" {
			merged.PermissionMode = def.PermissionMode
		}
		mcpVal := def.MCPBridge
		merged.MCPBridge = &mcpVal
		if config.ValidateRuntime(&merged, &cfg.Bot) != nil {
			continue
		}
		cfg.Runtime = &merged
		_ = saveAgentConfig(name, cfg)
	}
}

func (s *RuntimeManageService) acquireJob(presetID string) bool {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	if s.installing[presetID] {
		return false
	}
	s.installing[presetID] = true
	return true
}

func (s *RuntimeManageService) releaseJob(presetID string) {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	delete(s.installing, presetID)
}

func defaultStoredDefaults(preset config.RuntimePreset) storedRuntimeDefaults {
	authSource := preset.DefaultAuth
	if authSource == "" {
		authSource = config.AuthSourceCLILogin
	}
	return storedRuntimeDefaults{
		Preset:         preset.ID,
		Command:        preset.DefaultCommand,
		Args:           append([]string(nil), preset.DefaultArgs...),
		AuthSource:     authSource,
		APIKeyEnv:      preset.DefaultAPIKeyEnv,
		PermissionMode: config.PermissionModeBridge,
		MCPBridge:      true,
	}
}

func toDefaultsView(d storedRuntimeDefaults) RuntimeDefaultsView {
	return RuntimeDefaultsView{
		Preset:         d.Preset,
		Command:        d.Command,
		Args:           d.Args,
		Model:          d.Model,
		ThinkingLevel:  d.ThinkingLevel,
		SessionMode:    d.SessionMode,
		AuthSource:     d.AuthSource,
		APIKeyEnv:      d.APIKeyEnv,
		APIKey:         secretState(d.APIKey),
		PermissionMode: d.PermissionMode,
		MCPBridge:      d.MCPBridge,
		UpdatedAt:      d.UpdatedAt,
	}
}

func probeCustomVersion(ctx context.Context, binaryPath string) string {
	subCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	for _, rt := range agentruntime.Discover(subCtx) {
		if rt.BinaryPath == binaryPath && rt.Version != "" {
			return rt.Version
		}
	}
	return ""
}
