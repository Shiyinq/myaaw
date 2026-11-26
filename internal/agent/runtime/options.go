package runtime

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"myaaw/internal/config"
)

// RuntimeOptionItem describes one selectable option discovered dynamically
// from an external runtime CLI (model, thinking/reasoning level, session mode,
// or Myaaw permission routing mode).
type RuntimeOptionItem struct {
	Value          string   `json:"value"`
	Label          string   `json:"label"`
	Description    string   `json:"description,omitempty"`
	Group          string   `json:"group,omitempty"`
	ContextWindow  int      `json:"context_window,omitempty"`
	Thinking       bool     `json:"thinking,omitempty"`
	Vision         bool     `json:"vision,omitempty"`
	ThinkingLevels []string `json:"thinking_levels,omitempty"`
}

// RuntimeOptionGroup holds one category of dynamic options returned by
// GET /agents/:name/runtime/options.
type RuntimeOptionGroup struct {
	ConfigID    string              `json:"config_id,omitempty"`
	Current     string              `json:"current,omitempty"`
	Options     []RuntimeOptionItem `json:"options"`
	AllowCustom bool                `json:"allow_custom,omitempty"`
}

// RuntimeOptions is the live capability and option discovery payload for an
// external CLI runtime.
type RuntimeOptions struct {
	RuntimeType   string             `json:"runtime_type"`
	RuntimePreset string             `json:"runtime_preset"`
	Models        RuntimeOptionGroup `json:"models"`
	Thinking      RuntimeOptionGroup `json:"thinking"`
	SessionModes  RuntimeOptionGroup `json:"session_modes"`
	ApprovalModes RuntimeOptionGroup `json:"approval_modes"`
	Warning       string             `json:"warning,omitempty"`
}

type cachedRuntimeOptions struct {
	preset    string
	fetchedAt time.Time
	value     RuntimeOptions
}

type runtimeOptionsInflight struct {
	done  chan struct{}
	value RuntimeOptions
}

var (
	runtimeOptionsCacheMu sync.Mutex
	runtimeOptionsCache   = make(map[string]cachedRuntimeOptions)
	runtimeOptionsRunning = make(map[string]*runtimeOptionsInflight)
	runtimeOptionsTTL     = 5 * time.Minute
)

// InvalidateRuntimeOptionsCache drops cached runtime option discovery entries
// for presetID (or every preset when presetID is empty) so installs, updates,
// and login/logout transitions always probe fresh models and auth state.
func InvalidateRuntimeOptionsCache(presetID string) {
	id := strings.ToLower(strings.TrimSpace(presetID))
	runtimeOptionsCacheMu.Lock()
	defer runtimeOptionsCacheMu.Unlock()
	if id == "" {
		runtimeOptionsCache = make(map[string]cachedRuntimeOptions)
		return
	}
	for k, entry := range runtimeOptionsCache {
		if entry.preset == id {
			delete(runtimeOptionsCache, k)
		}
	}
}

func runtimeOptionsCacheKey(cfg config.RuntimeConfig) string {
	cmdKey := strings.TrimSpace(cfg.Command)
	if resolved, err := ResolveBinary(cmdKey); err == nil && resolved != "" {
		cmdKey = resolved
	}
	keyFingerprint := ""
	if cfg.AuthSource == config.AuthSourceAPIKey && cfg.APIKey != "" {
		sum := sha256.Sum256([]byte(cfg.APIKeyEnv + ":" + cfg.APIKey))
		keyFingerprint = hex.EncodeToString(sum[:8])
	}
	return fmt.Sprintf("%s|%s|%s|%s|%s",
		cfg.Type, cfg.Preset, cmdKey, strings.Join(cfg.Args, "\x00"), keyFingerprint)
}

func cloneRuntimeOptions(in RuntimeOptions) RuntimeOptions {
	out := in
	if len(in.Models.Options) > 0 {
		out.Models.Options = make([]RuntimeOptionItem, len(in.Models.Options))
		for i, item := range in.Models.Options {
			if len(item.ThinkingLevels) > 0 {
				item.ThinkingLevels = append([]string(nil), item.ThinkingLevels...)
			}
			out.Models.Options[i] = item
		}
	}
	if len(in.Thinking.Options) > 0 {
		out.Thinking.Options = append([]RuntimeOptionItem(nil), in.Thinking.Options...)
	}
	if len(in.SessionModes.Options) > 0 {
		out.SessionModes.Options = append([]RuntimeOptionItem(nil), in.SessionModes.Options...)
	}
	if len(in.ApprovalModes.Options) > 0 {
		out.ApprovalModes.Options = append([]RuntimeOptionItem(nil), in.ApprovalModes.Options...)
	}
	return out
}

// cleanupProbeProcess tears down a one-shot probe subprocess asynchronously so
// the caller can return immediately after parsing the JSON-RPC response without
// blocking on the 300ms SIGTERM grace period.
func cleanupProbeProcess(stdin io.Closer, cmd *exec.Cmd) {
	go func() {
		if stdin != nil {
			_ = stdin.Close()
		}
		terminateProcessGroup(cmd)
		_ = cmd.Wait()
	}()
}

// DiscoverRuntimeOptionsCached returns cached live runtime options unless
// refresh is true or the 5-minute TTL expired. Cache entries are shared across
// agents, onboarding probes, and auth status checks for the same runtime binary
// and arguments, and concurrent callers coalesce onto a single CLI probe.
func DiscoverRuntimeOptionsCached(
	ctx context.Context,
	agentName string,
	raw config.RuntimeConfig,
	workingDir string,
	refresh bool,
) RuntimeOptions {
	_ = agentName
	cfg := config.MergeRuntime(&raw)
	if !cfg.IsExternal() {
		return DiscoverRuntimeOptions(ctx, cfg, workingDir)
	}
	cacheKey := runtimeOptionsCacheKey(cfg)

	runtimeOptionsCacheMu.Lock()
	if !refresh {
		if entry, ok := runtimeOptionsCache[cacheKey]; ok && time.Since(entry.fetchedAt) < runtimeOptionsTTL {
			val := cloneRuntimeOptions(entry.value)
			runtimeOptionsCacheMu.Unlock()
			return applyConfiguredCurrents(val, cfg)
		}
	}
	if inflight, ok := runtimeOptionsRunning[cacheKey]; ok {
		runtimeOptionsCacheMu.Unlock()
		select {
		case <-inflight.done:
			return applyConfiguredCurrents(cloneRuntimeOptions(inflight.value), cfg)
		case <-ctx.Done():
			return DiscoverRuntimeOptions(ctx, cfg, workingDir)
		}
	}
	inflight := &runtimeOptionsInflight{done: make(chan struct{})}
	runtimeOptionsRunning[cacheKey] = inflight
	runtimeOptionsCacheMu.Unlock()

	opts := DiscoverRuntimeOptions(ctx, cfg, workingDir)
	runtimeOptionsCacheMu.Lock()
	if opts.Warning == "" {
		runtimeOptionsCache[cacheKey] = cachedRuntimeOptions{
			preset:    strings.ToLower(strings.TrimSpace(cfg.Preset)),
			fetchedAt: time.Now(),
			value:     cloneRuntimeOptions(opts),
		}
	}
	inflight.value = cloneRuntimeOptions(opts)
	delete(runtimeOptionsRunning, cacheKey)
	close(inflight.done)
	runtimeOptionsCacheMu.Unlock()
	return applyConfiguredCurrents(opts, cfg)
}

// DiscoverRuntimeOptions probes the configured external CLI runtime live to
// discover its available models, thinking/reasoning levels, native session
// modes, and Myaaw approval routing modes without hardcoded model lists.
func DiscoverRuntimeOptions(ctx context.Context, raw config.RuntimeConfig, workingDir string) RuntimeOptions {
	cfg := config.MergeRuntime(&raw)
	if strings.TrimSpace(workingDir) == "" {
		workingDir = ResolveWorkingDir(cfg)
	}

	opts := RuntimeOptions{
		RuntimeType:   cfg.Type,
		RuntimePreset: cfg.Preset,
		Models:        RuntimeOptionGroup{Options: []RuntimeOptionItem{}},
		Thinking:      RuntimeOptionGroup{Options: []RuntimeOptionItem{}},
		SessionModes:  RuntimeOptionGroup{Options: []RuntimeOptionItem{}},
		ApprovalModes: RuntimeOptionGroup{Options: []RuntimeOptionItem{}},
	}

	if !cfg.IsExternal() {
		return opts
	}

	caps := config.CapabilitiesForRuntime(&cfg)
	if caps.SupportsApprovals {
		opts.ApprovalModes = RuntimeOptionGroup{
			Current: cfg.PermissionMode,
			Options: []RuntimeOptionItem{
				{
					Value:       config.PermissionModeAuto,
					Label:       "Full Access",
					Description: "Auto-approve external CLI tool calls without prompting.",
				},
				{
					Value:       config.PermissionModeBridge,
					Label:       "Approval",
					Description: "Route external CLI permission prompts through Myaaw approvals.",
				},
				{
					Value:       config.PermissionModeReadOnly,
					Label:       "Read Only",
					Description: "Only allow read, list, search, and inspection tools.",
				},
			},
		}
	}

	opts.Models.AllowCustom = caps.SupportsModelOverride

	switch cfg.Type {
	case config.RuntimeTypeACP:
		probeACPOptions(ctx, cfg, workingDir, &opts)
	case config.RuntimeTypePi, config.RuntimeTypeOMP:
		probePiRPCOptions(ctx, cfg, workingDir, &opts)
	case config.RuntimeTypeCodex:
		probeCodexOptions(ctx, cfg, workingDir, &opts)
	case config.RuntimeTypeClaudeCode:
		probeClaudeOptions(ctx, cfg, workingDir, &opts)
	}

	return applyConfiguredCurrents(opts, cfg)
}

func applyConfiguredCurrents(opts RuntimeOptions, cfg config.RuntimeConfig) RuntimeOptions {
	if opts.Models.Options == nil {
		opts.Models.Options = []RuntimeOptionItem{}
	}
	if opts.Thinking.Options == nil {
		opts.Thinking.Options = []RuntimeOptionItem{}
	}
	if opts.SessionModes.Options == nil {
		opts.SessionModes.Options = []RuntimeOptionItem{}
	}
	if opts.ApprovalModes.Options == nil {
		opts.ApprovalModes.Options = []RuntimeOptionItem{}
	}
	if len(opts.ApprovalModes.Options) > 0 {
		if cfg.PermissionMode != "" {
			opts.ApprovalModes.Current = cfg.PermissionMode
		} else {
			opts.ApprovalModes.Current = config.PermissionModeBridge
		}
	}

	// If a specific model is selected and the runtime reports per-model thinking
	// metadata (e.g. Pi's thinkingLevelMap or Codex's supportedReasoningEfforts),
	// refine Thinking.Options dynamically for that model.
	hasPerModelThinking := false
	for _, m := range opts.Models.Options {
		if len(m.ThinkingLevels) > 0 {
			hasPerModelThinking = true
			break
		}
	}
	selectedModel := strings.TrimSpace(cfg.Model)
	if selectedModel == "" {
		selectedModel = opts.Models.Current
	}
	if selectedModel != "" && hasPerModelThinking {
		for _, m := range opts.Models.Options {
			if m.Value == selectedModel {
				items := make([]RuntimeOptionItem, 0, len(m.ThinkingLevels))
				for _, lvl := range m.ThinkingLevels {
					items = append(items, RuntimeOptionItem{
						Value: lvl,
						Label: formatLevelLabel(lvl),
					})
				}
				opts.Thinking.Options = items
				break
			}
		}
	}
	return opts
}

type acpConfigOptionValue struct {
	Value       string `json:"value"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type acpConfigOptionEntry struct {
	Value       string                 `json:"value"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Group       string                 `json:"group"`
	Options     []acpConfigOptionValue `json:"options"`
}

type acpConfigOption struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Category     string                 `json:"category"`
	Type         string                 `json:"type"`
	CurrentValue string                 `json:"currentValue"`
	Options      []acpConfigOptionEntry `json:"options"`
}

type acpSessionMeta struct {
	SessionID     string            `json:"sessionId"`
	ConfigOptions []acpConfigOption `json:"configOptions"`
	Models        *struct {
		CurrentModelID  string `json:"currentModelId"`
		AvailableModels []struct {
			ModelID     string `json:"modelId"`
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"availableModels"`
	} `json:"models"`
	Modes *struct {
		CurrentModeID  string `json:"currentModeId"`
		AvailableModes []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"availableModes"`
	} `json:"modes"`
}

func probeACPOptions(parent context.Context, cfg config.RuntimeConfig, workingDir string, opts *RuntimeOptions) {
	ctx, cancel := context.WithTimeout(parent, 12*time.Second)
	defer cancel()

	bin, err := ResolveBinary(cfg.Command)
	if err != nil {
		opts.Warning = err.Error()
		return
	}

	cmd := exec.Command(bin, acpSubprocessArgs(cfg)...)
	cmd.Dir = workingDir
	cmd.Env = BuildSubprocessEnv(cfg, "", "")
	configureProcessGroup(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		opts.Warning = err.Error()
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		opts.Warning = err.Error()
		return
	}
	if err := cmd.Start(); err != nil {
		opts.Warning = fmt.Sprintf("start %s: %v", cfg.Command, err)
		return
	}
	defer cleanupProbeProcess(stdin, cmd)

	writeReq := func(id int, method string, params any) error {
		data, err := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"method":  method,
			"params":  params,
		})
		if err != nil {
			return err
		}
		_, err = stdin.Write(append(data, '\n'))
		return err
	}

	if err := writeReq(1, "initialize", map[string]any{
		"protocolVersion":    1,
		"clientCapabilities": map[string]any{},
		"clientInfo":         map[string]any{"name": "myaaw", "version": "1.0.0"},
	}); err != nil {
		opts.Warning = err.Error()
		return
	}

	type lineRes struct {
		line string
		eof  bool
	}
	lines := make(chan lineRes, 16)
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for sc.Scan() {
			lines <- lineRes{line: strings.TrimSpace(sc.Text())}
		}
		lines <- lineRes{eof: true}
	}()

	for {
		select {
		case <-ctx.Done():
			opts.Warning = "timed out probing ACP runtime options"
			return
		case res := <-lines:
			if res.eof {
				return
			}
			if !strings.HasPrefix(res.line, "{") {
				continue
			}
			var msg jsonRPCMessage
			if json.Unmarshal([]byte(res.line), &msg) != nil || len(msg.ID) == 0 {
				continue
			}
			var id int
			if json.Unmarshal(msg.ID, &id) != nil {
				continue
			}
			if msg.Error != nil {
				opts.Warning = msg.Error.Message
				return
			}
			switch id {
			case 1:
				if err := writeReq(2, "session/new", map[string]any{
					"cwd":        workingDir,
					"mcpServers": []any{},
				}); err != nil {
					opts.Warning = err.Error()
					return
				}
			case 2:
				var meta acpSessionMeta
				if err := json.Unmarshal(msg.Result, &meta); err != nil {
					opts.Warning = err.Error()
					return
				}
				applyACPSessionMeta(meta, opts)
				return
			}
		}
	}
}

func applyACPSessionMeta(meta acpSessionMeta, opts *RuntimeOptions) {
	for _, co := range meta.ConfigOptions {
		cat := strings.ToLower(strings.TrimSpace(co.Category))
		id := strings.ToLower(strings.TrimSpace(co.ID))
		items := make([]RuntimeOptionItem, 0, len(co.Options))
		for _, o := range co.Options {
			if len(o.Options) > 0 {
				groupLabel := strings.TrimSpace(o.Name)
				if groupLabel == "" {
					groupLabel = strings.TrimSpace(o.Group)
				}
				for _, sub := range o.Options {
					val := strings.TrimSpace(sub.Value)
					if val == "" {
						continue
					}
					label := strings.TrimSpace(sub.Name)
					if label == "" {
						label = val
					}
					items = append(items, RuntimeOptionItem{
						Value:       val,
						Label:       label,
						Description: strings.TrimSpace(sub.Description),
						Group:       groupLabel,
					})
				}
				continue
			}
			val := strings.TrimSpace(o.Value)
			if val == "" {
				continue
			}
			label := strings.TrimSpace(o.Name)
			if label == "" {
				label = val
			}
			group := ""
			if idx := strings.IndexByte(val, '/'); idx > 0 {
				group = val[:idx]
			}
			items = append(items, RuntimeOptionItem{
				Value:       val,
				Label:       label,
				Description: strings.TrimSpace(o.Description),
				Group:       group,
			})
		}
		switch {
		case cat == "model" || id == "model":
			opts.Models.ConfigID = co.ID
			opts.Models.Current = co.CurrentValue
			opts.Models.Options = items
		case cat == "thought_level" || id == "thought_level" || strings.Contains(id, "effort") || strings.Contains(id, "thinking") || strings.Contains(id, "reasoning"):
			opts.Thinking.ConfigID = co.ID
			opts.Thinking.Current = co.CurrentValue
			opts.Thinking.Options = items
		case cat == "mode" || id == "mode":
			opts.SessionModes.ConfigID = co.ID
			opts.SessionModes.Current = co.CurrentValue
			opts.SessionModes.Options = items
		}
	}

	if len(opts.Models.Options) == 0 && meta.Models != nil {
		opts.Models.Current = meta.Models.CurrentModelID
		for _, m := range meta.Models.AvailableModels {
			val := strings.TrimSpace(m.ModelID)
			if val == "" {
				continue
			}
			label := strings.TrimSpace(m.Name)
			if label == "" {
				label = val
			}
			group := ""
			if idx := strings.IndexByte(val, '/'); idx > 0 {
				group = val[:idx]
			}
			opts.Models.Options = append(opts.Models.Options, RuntimeOptionItem{
				Value:       val,
				Label:       label,
				Description: strings.TrimSpace(m.Description),
				Group:       group,
			})
		}
	}

	if len(opts.SessionModes.Options) == 0 && meta.Modes != nil {
		opts.SessionModes.Current = meta.Modes.CurrentModeID
		for _, m := range meta.Modes.AvailableModes {
			val := strings.TrimSpace(m.ID)
			if val == "" {
				continue
			}
			label := strings.TrimSpace(m.Name)
			if label == "" {
				label = val
			}
			opts.SessionModes.Options = append(opts.SessionModes.Options, RuntimeOptionItem{
				Value:       val,
				Label:       label,
				Description: strings.TrimSpace(m.Description),
			})
		}
	}
}

func probePiRPCOptions(parent context.Context, cfg config.RuntimeConfig, workingDir string, opts *RuntimeOptions) {
	ctx, cancel := context.WithTimeout(parent, 6*time.Second)
	defer cancel()

	bin, err := ResolveBinary(cfg.Command)
	if err != nil {
		opts.Warning = err.Error()
		return
	}

	args := append([]string(nil), cfg.Args...)
	if len(args) == 0 {
		args = []string{"--mode", "rpc"}
	}
	args = append(args, "--no-session")

	cmd := exec.Command(bin, args...)
	cmd.Dir = workingDir
	cmd.Env = BuildSubprocessEnv(cfg, "", "")
	configureProcessGroup(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		opts.Warning = err.Error()
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		opts.Warning = err.Error()
		return
	}
	if err := cmd.Start(); err != nil {
		opts.Warning = fmt.Sprintf("start %s: %v", cfg.Command, err)
		return
	}
	defer cleanupProbeProcess(stdin, cmd)

	_, _ = stdin.Write([]byte(`{"id":"1","type":"get_state"}` + "\n"))
	_, _ = stdin.Write([]byte(`{"id":"2","type":"get_available_models"}` + "\n"))

	type lineRes struct {
		line string
		eof  bool
	}
	lines := make(chan lineRes, 16)
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for sc.Scan() {
			lines <- lineRes{line: strings.TrimSpace(sc.Text())}
		}
		lines <- lineRes{eof: true}
	}()

	gotState := false
	gotModels := false
	for !gotState || !gotModels {
		select {
		case <-ctx.Done():
			if !gotModels {
				opts.Warning = "timed out probing Pi RPC options"
			}
			return
		case res := <-lines:
			if res.eof {
				return
			}
			if !strings.HasPrefix(res.line, "{") {
				continue
			}
			var frame struct {
				ID      string          `json:"id"`
				Type    string          `json:"type"`
				Command string          `json:"command"`
				Success bool            `json:"success"`
				Data    json.RawMessage `json:"data"`
			}
			if json.Unmarshal([]byte(res.line), &frame) != nil || frame.Type != "response" {
				continue
			}
			switch frame.Command {
			case "get_state":
				gotState = true
				if frame.Success {
					var st struct {
						Model *struct {
							ID       string `json:"id"`
							Provider string `json:"provider"`
						} `json:"model"`
						ThinkingLevel string `json:"thinkingLevel"`
					}
					if json.Unmarshal(frame.Data, &st) == nil {
						if st.Model != nil && st.Model.ID != "" {
							opts.Models.Current = formatPiModelValue(st.Model.Provider, st.Model.ID)
						}
						if st.ThinkingLevel != "" {
							opts.Thinking.Current = st.ThinkingLevel
						}
					}
				}
			case "get_available_models":
				gotModels = true
				if frame.Success {
					var avail struct {
						Models []struct {
							ID               string            `json:"id"`
							Name             string            `json:"name"`
							Provider         string            `json:"provider"`
							Reasoning        bool              `json:"reasoning"`
							ContextWindow    int               `json:"contextWindow"`
							Input            []string          `json:"input"`
							ThinkingLevelMap map[string]any    `json:"thinkingLevelMap"`
							Extra            map[string]string `json:"-"`
						} `json:"models"`
					}
					if json.Unmarshal(frame.Data, &avail) == nil {
						for _, m := range avail.Models {
							if strings.TrimSpace(m.ID) == "" {
								continue
							}
							val := formatPiModelValue(m.Provider, m.ID)
							label := strings.TrimSpace(m.Name)
							if label == "" {
								label = m.ID
							}
							hasVision := false
							for _, in := range m.Input {
								if strings.EqualFold(in, "image") {
									hasVision = true
									break
								}
							}
							levels := extractPiThinkingLevels(m.Reasoning, m.ThinkingLevelMap)
							opts.Models.Options = append(opts.Models.Options, RuntimeOptionItem{
								Value:          val,
								Label:          label,
								Group:          strings.TrimSpace(m.Provider),
								ContextWindow:  m.ContextWindow,
								Thinking:       m.Reasoning,
								Vision:         hasVision,
								ThinkingLevels: levels,
							})
						}
					}
				}
			}
		}
	}
}

func formatPiModelValue(provider, id string) string {
	p := strings.TrimSpace(provider)
	m := strings.TrimSpace(id)
	if p == "" || strings.Contains(m, "/") {
		return m
	}
	return p + "/" + m
}

func extractPiThinkingLevels(reasoning bool, levelMap map[string]any) []string {
	if !reasoning {
		return nil
	}
	canonicalOrder := []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}
	found := map[string]bool{}
	for k, v := range levelMap {
		if v == nil {
			continue
		}
		if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
			continue
		}
		found[strings.ToLower(strings.TrimSpace(k))] = true
	}
	if len(found) == 0 {
		return nil
	}
	found["off"] = true
	out := make([]string, 0, len(found))
	for _, lvl := range canonicalOrder {
		if found[lvl] {
			out = append(out, lvl)
			delete(found, lvl)
		}
	}
	var extras []string
	for k := range found {
		extras = append(extras, k)
	}
	sort.Strings(extras)
	return append(out, extras...)
}

func probeCodexOptions(parent context.Context, cfg config.RuntimeConfig, workingDir string, opts *RuntimeOptions) {
	ctx, cancel := context.WithTimeout(parent, 6*time.Second)
	defer cancel()

	bin, err := ResolveBinary(cfg.Command)
	if err != nil {
		opts.Warning = err.Error()
		return
	}

	args := codexSubprocessArgs(cfg, false)
	cmd := exec.Command(bin, args...)
	cmd.Dir = workingDir
	cmd.Env = BuildSubprocessEnv(cfg, "", "")
	configureProcessGroup(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		opts.Warning = err.Error()
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		opts.Warning = err.Error()
		return
	}
	if err := cmd.Start(); err != nil {
		opts.Warning = fmt.Sprintf("start %s: %v", cfg.Command, err)
		return
	}
	defer cleanupProbeProcess(stdin, cmd)

	writeReq := func(id int, method string, params any) error {
		data, err := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"method":  method,
			"params":  params,
		})
		if err != nil {
			return err
		}
		_, err = stdin.Write(append(data, '\n'))
		return err
	}

	if err := writeReq(1, "initialize", map[string]any{
		"clientInfo": map[string]any{"name": "myaaw", "version": "1.0.0"},
	}); err != nil {
		opts.Warning = err.Error()
		return
	}

	type lineRes struct {
		line string
		eof  bool
	}
	lines := make(chan lineRes, 16)
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for sc.Scan() {
			lines <- lineRes{line: strings.TrimSpace(sc.Text())}
		}
		lines <- lineRes{eof: true}
	}()

	for {
		select {
		case <-ctx.Done():
			opts.Warning = "timed out probing Codex options"
			return
		case res := <-lines:
			if res.eof {
				return
			}
			if !strings.HasPrefix(res.line, "{") {
				continue
			}
			var msg jsonRPCMessage
			if json.Unmarshal([]byte(res.line), &msg) != nil || len(msg.ID) == 0 {
				continue
			}
			var id int
			if json.Unmarshal(msg.ID, &id) != nil {
				continue
			}
			if msg.Error != nil {
				opts.Warning = msg.Error.Message
				return
			}
			switch id {
			case 1:
				if err := writeReq(2, "model/list", map[string]any{}); err != nil {
					opts.Warning = err.Error()
					return
				}
			case 2:
				var list struct {
					Data []struct {
						ID                        string `json:"id"`
						Model                     string `json:"model"`
						DisplayName               string `json:"displayName"`
						Description               string `json:"description"`
						IsDefault                 bool   `json:"isDefault"`
						DefaultReasoningEffort    string `json:"defaultReasoningEffort"`
						SupportedReasoningEfforts []struct {
							ReasoningEffort string `json:"reasoningEffort"`
							Effort          string `json:"effort"`
						} `json:"supportedReasoningEfforts"`
					} `json:"data"`
				}
				if json.Unmarshal(msg.Result, &list) == nil {
					for _, m := range list.Data {
						val := strings.TrimSpace(m.Model)
						if val == "" {
							val = strings.TrimSpace(m.ID)
						}
						if val == "" {
							continue
						}
						label := strings.TrimSpace(m.DisplayName)
						if label == "" {
							label = val
						}
						var levels []string
						for _, e := range m.SupportedReasoningEfforts {
							lvl := strings.TrimSpace(e.ReasoningEffort)
							if lvl == "" {
								lvl = strings.TrimSpace(e.Effort)
							}
							if lvl != "" {
								levels = append(levels, lvl)
							}
						}
						if m.IsDefault && opts.Models.Current == "" {
							opts.Models.Current = val
							if m.DefaultReasoningEffort != "" {
								opts.Thinking.Current = m.DefaultReasoningEffort
							}
						}
						opts.Models.Options = append(opts.Models.Options, RuntimeOptionItem{
							Value:          val,
							Label:          label,
							Description:    strings.TrimSpace(m.Description),
							Thinking:       len(levels) > 0,
							ThinkingLevels: levels,
						})
					}
				}
				return
			}
		}
	}
}

var helpChoicesRe = regexp.MustCompile(`choices:\s*([^)]+)`)

func probeClaudeOptions(parent context.Context, cfg config.RuntimeConfig, workingDir string, opts *RuntimeOptions) {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()

	bin, err := ResolveBinary(cfg.Command)
	if err != nil {
		opts.Warning = err.Error()
		return
	}

	cmd := exec.CommandContext(ctx, bin, "--help")
	cmd.Dir = workingDir
	cmd.Env = BuildSubprocessEnv(cfg, "", "")
	raw, err := cmd.CombinedOutput()
	if err != nil && len(raw) == 0 {
		opts.Warning = fmt.Sprintf("probe %s --help: %v", cfg.Command, err)
		return
	}

	text := string(raw)
	if efforts := parseFlagChoices(text, "--effort"); len(efforts) > 0 {
		for _, v := range efforts {
			opts.Thinking.Options = append(opts.Thinking.Options, RuntimeOptionItem{
				Value: v,
				Label: formatLevelLabel(v),
			})
		}
	}
	if modes := parseFlagChoices(text, "--permission-mode"); len(modes) > 0 {
		for _, v := range modes {
			opts.SessionModes.Options = append(opts.SessionModes.Options, RuntimeOptionItem{
				Value: v,
				Label: formatLevelLabel(v),
			})
		}
	}
}

func parseFlagChoices(helpText, flag string) []string {
	lines := strings.Split(helpText, "\n")
	for i, line := range lines {
		if !strings.Contains(line, flag) {
			continue
		}
		block := line
		for j := i + 1; j < len(lines) && j <= i+3; j++ {
			next := strings.TrimSpace(lines[j])
			if strings.HasPrefix(next, "-") {
				break
			}
			block += " " + next
		}
		if match := helpChoicesRe.FindStringSubmatch(block); len(match) > 1 {
			parts := strings.Split(match[1], ",")
			var out []string
			for _, p := range parts {
				cleaned := strings.Trim(strings.TrimSpace(p), `"'`)
				if cleaned != "" {
					out = append(out, cleaned)
				}
			}
			return out
		}
		// Also support parenthesized pipe lists like (low|medium|high|max).
		if start := strings.IndexByte(block, '('); start >= 0 {
			if end := strings.IndexByte(block[start:], ')'); end > 0 {
				inside := block[start+1 : start+end]
				if strings.Contains(inside, "|") {
					parts := strings.Split(inside, "|")
					var out []string
					for _, p := range parts {
						cleaned := strings.TrimSpace(p)
						if cleaned != "" {
							out = append(out, cleaned)
						}
					}
					return out
				}
			}
		}
	}
	return nil
}

func formatLevelLabel(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "off", "none":
		return "Off"
	case "xhigh":
		return "XHigh"
	default:
		clean := strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(v), "_", " "), "-", " ")
		parts := strings.Fields(clean)
		for i, p := range parts {
			if len(p) > 0 {
				parts[i] = strings.ToUpper(p[:1]) + p[1:]
			}
		}
		return strings.Join(parts, " ")
	}
}
