package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"myaaw/internal/agent"
	"myaaw/internal/config"
	"myaaw/internal/provider"
)

// acpRuntime implements the Agent Client Protocol (ACP) over JSON-RPC 2.0 stdio,
// supporting Google Antigravity (`agy acp`), OpenCode (`opencode acp`),
// Claude/Codex ACP adapters, and custom ACP servers.
type acpRuntime struct {
	externalBase

	poolMu   sync.Mutex
	sessions map[string]*acpProcess
	stopReap chan struct{}
	closed   bool
}

type acpProcess struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	writeMu   sync.Mutex
	nextID    atomic.Int64
	sessionID string
	lastUsed  time.Time
	alive     atomic.Bool

	pendingMu sync.Mutex
	pending   map[int64]chan jsonRPCMessage

	turnMu         sync.RWMutex
	onNotification func(method string, params json.RawMessage)
	onRequest      func(id json.RawMessage, method string, params json.RawMessage)
}

type jsonRPCMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func newACPRuntime(cfg config.RuntimeConfig) Runtime {
	r := &acpRuntime{
		externalBase: newExternalBase(cfg),
		sessions:     make(map[string]*acpProcess),
		stopReap:     make(chan struct{}),
	}
	go r.reapLoop()
	return r
}

func (r *acpRuntime) WithExcludedTools(tools ...string) agent.AgentProvider {
	return r
}

func (r *acpRuntime) Run(modelName string, messages []provider.Message) (provider.Message, error) {
	return r.RunContext(context.Background(), modelName, messages)
}

func (r *acpRuntime) RunContext(ctx context.Context, modelName string, messages []provider.Message) (provider.Message, error) {
	return runContextAdapter(ctx, r, modelName, messages)
}

func (r *acpRuntime) RunStream(modelName string, messages []provider.Message, callback func(provider.Message) error) error {
	return runStreamAdapter(r, modelName, messages, callback)
}

func (r *acpRuntime) Close() error {
	r.poolMu.Lock()
	if !r.closed {
		r.closed = true
		close(r.stopReap)
	}
	procs := make([]*acpProcess, 0, len(r.sessions))
	for k, p := range r.sessions {
		procs = append(procs, p)
		delete(r.sessions, k)
	}
	r.poolMu.Unlock()

	for _, p := range procs {
		p.kill()
	}
	r.closeBridge()
	return nil
}

func (r *acpRuntime) reapLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	idle := r.cfg.IdleTimeout()
	for {
		select {
		case <-r.stopReap:
			return
		case <-ticker.C:
			now := time.Now()
			var stale []*acpProcess
			r.poolMu.Lock()
			for k, p := range r.sessions {
				if !p.alive.Load() || now.Sub(p.lastUsed) > idle {
					stale = append(stale, p)
					delete(r.sessions, k)
				}
			}
			r.poolMu.Unlock()
			for _, p := range stale {
				p.kill()
			}
		}
	}
}

func (r *acpRuntime) RunEvents(
	ctx context.Context,
	cfg agent.RunConfig,
	history []provider.Message,
	onEvent func(agent.AgentEvent) error,
) (agent.RunResult, error) {
	if cfg.RunTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.RunTimeout)
		defer cancel()
	}

	acc := newTurnAccumulator(ctx, cfg, history, onEvent)
	releaseBridge := r.activateBridge(cfg, acc)
	defer releaseBridge()

	convKey := cfg.ConversationID
	if convKey == "" {
		convKey = acc.runID
	}

	matchingRuntime := cfg.ExternalRuntime == "" || cfg.ExternalRuntime == r.DisplayProvider()
	wantResume := cfg.ExternalSessionID != "" && matchingRuntime && !cfg.Rewind

	proc, resumed, err := r.acquireSession(ctx, convKey, cfg, wantResume)
	if err != nil {
		return acc.Finish(provider.StopError, err)
	}
	if cfg.OnExternalSession != nil && proc.sessionID != "" {
		cfg.OnExternalSession(r.DisplayProvider(), proc.sessionID)
	}

	systemPrompt, userText, images := BuildTurnInput(history, resumed, cfg.Rewind)
	promptText := userText
	if !resumed && systemPrompt != "" {
		promptText = "<system_instructions>\n" + systemPrompt + "\n</system_instructions>\n\n" + userText
	}

	// Build ACP prompt content blocks (text + optional inline images).
	blocks := make([]map[string]any, 0, 1+len(images))
	if promptText != "" {
		blocks = append(blocks, map[string]any{"type": "text", "text": promptText})
	}
	for _, img := range images {
		blocks = append(blocks, map[string]any{
			"type":     "image",
			"data":     img,
			"mimeType": "image/jpeg",
		})
	}
	if len(blocks) == 0 {
		blocks = append(blocks, map[string]any{"type": "text", "text": ""})
	}

	proc.setHandlers(
		func(method string, params json.RawMessage) {
			if method == "session/update" {
				handleACPUpdate(params, acc)
			}
		},
		func(id json.RawMessage, method string, params json.RawMessage) {
			if method == "session/request_permission" {
				go r.handleACPPermission(ctx, cfg, proc, id, params)
				return
			}
			_ = proc.sendError(id, -32601, "method not supported by myaaw acp client: "+method)
		},
	)
	defer proc.setHandlers(nil, nil)

	// Run session/prompt asynchronously so ctx cancellation triggers two-stage
	// cancel: ACP session/cancel first, then process-group kill after 2s.
	type promptOutcome struct {
		res json.RawMessage
		err error
	}
	doneCh := make(chan promptOutcome, 1)
	go func() {
		res, callErr := proc.call(ctx, "session/prompt", map[string]any{
			"sessionId": proc.sessionID,
			"prompt":    blocks,
		})
		doneCh <- promptOutcome{res: res, err: callErr}
	}()

	var promptRes json.RawMessage
	var runErr error
	select {
	case out := <-doneCh:
		promptRes = out.res
		runErr = out.err
	case <-ctx.Done():
		runErr = ctx.Err()
		_ = proc.notify("session/cancel", map[string]any{"sessionId": proc.sessionID})
		select {
		case <-doneCh:
		case <-time.After(2 * time.Second):
			r.evictSession(convKey, proc)
			proc.kill()
		}
	}

	if runErr != nil {
		if ctx.Err() != nil {
			return acc.Finish(provider.StopAborted, ctx.Err())
		}
		if !proc.alive.Load() {
			r.evictSession(convKey, proc)
		}
		return acc.Finish(provider.StopError, runErr)
	}

	// Parse optional usage / stopReason from session/prompt result.
	var resultMeta struct {
		StopReason string `json:"stopReason"`
		Usage      *struct {
			InputTokens       int `json:"inputTokens"`
			OutputTokens      int `json:"outputTokens"`
			TotalTokens       int `json:"totalTokens"`
			CachedReadTokens  int `json:"cachedReadTokens"`
			ThoughtTokens     int `json:"thoughtTokens"`
			PromptTokens      int `json:"prompt_tokens"`
			CompletionTokens  int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if len(promptRes) > 0 && json.Unmarshal(promptRes, &resultMeta) == nil && resultMeta.Usage != nil {
		u := resultMeta.Usage
		promptTok := u.InputTokens
		if promptTok == 0 {
			promptTok = u.PromptTokens
		}
		compTok := u.OutputTokens
		if compTok == 0 {
			compTok = u.CompletionTokens
		}
		acc.AddUsage(provider.Usage{
			PromptTokens:      promptTok,
			CompletionTokens:  compTok,
			TotalTokens:       u.TotalTokens,
			CachedInputTokens: u.CachedReadTokens,
			ThoughtsTokens:    u.ThoughtTokens,
		})
	}

	proc.lastUsed = time.Now()
	return acc.Finish(provider.StopNormal, nil)
}

func (r *acpRuntime) acquireSession(ctx context.Context, convKey string, cfg agent.RunConfig, wantResume bool) (*acpProcess, bool, error) {
	r.poolMu.Lock()
	existing := r.sessions[convKey]
	if existing != nil && (!existing.alive.Load() || cfg.Rewind) {
		delete(r.sessions, convKey)
		r.poolMu.Unlock()
		existing.kill()
		existing = nil
	} else {
		r.poolMu.Unlock()
	}

	if existing != nil && wantResume && (existing.sessionID == cfg.ExternalSessionID || cfg.ExternalSessionID == "") {
		existing.lastUsed = time.Now()
		return existing, true, nil
	}

	proc, err := r.spawnACPProcess()
	if err != nil {
		return nil, false, err
	}

	if _, err := proc.call(ctx, "initialize", map[string]any{
		"protocolVersion": 1,
		"clientCapabilities": map[string]any{
			"fs": map[string]any{
				"readTextFile":  false,
				"writeTextFile": false,
			},
			"terminal": false,
		},
		"clientInfo": map[string]any{
			"name":    "myaaw",
			"version": "1.0.0",
		},
	}); err != nil {
		proc.kill()
		return nil, false, fmt.Errorf("acp initialize: %w", err)
	}

	cwd := r.WorkingDir()
	mcpSpecs := BuildMCPServers(r.cfg.MCPBridgeEnabled(), r.bridgeURL(), r.bridgeToken())
	acpMCP := make([]map[string]any, 0, len(mcpSpecs))
	for _, srv := range mcpSpecs {
		envList := make([]map[string]string, 0, len(srv.Env))
		for k, v := range srv.Env {
			envList = append(envList, map[string]string{"name": k, "value": v})
		}
		acpMCP = append(acpMCP, map[string]any{
			"name":    srv.Name,
			"command": srv.Command,
			"args":    srv.Args,
			"env":     envList,
		})
	}

	resumed := false
	var sessionMeta acpSessionMeta
	if wantResume && cfg.ExternalSessionID != "" {
		resumeParams := map[string]any{
			"sessionId":  cfg.ExternalSessionID,
			"cwd":        cwd,
			"mcpServers": acpMCP,
		}
		if raw, err := proc.call(ctx, "session/load", resumeParams); err == nil {
			_ = json.Unmarshal(raw, &sessionMeta)
			proc.sessionID = cfg.ExternalSessionID
			resumed = true
		} else if raw, rErr := proc.call(ctx, "session/resume", resumeParams); rErr == nil {
			// DeepSeek Harness (`@deepseek-ai/dsh-acp`) implements ACP `session/resume`
			// instead of `session/load` for persistent session restoration without replay.
			_ = json.Unmarshal(raw, &sessionMeta)
			proc.sessionID = cfg.ExternalSessionID
			resumed = true
		} else {
			log.Printf("[ACP] session/load (%v) and session/resume (%v) failed for %s; creating fresh session", err, rErr, cfg.ExternalSessionID)
		}
	}

	if !resumed {
		raw, err := proc.call(ctx, "session/new", map[string]any{
			"cwd":        cwd,
			"mcpServers": acpMCP,
		})
		if err != nil {
			proc.kill()
			return nil, false, fmt.Errorf("acp session/new: %w", err)
		}
		if err := json.Unmarshal(raw, &sessionMeta); err != nil || strings.TrimSpace(sessionMeta.SessionID) == "" {
			proc.kill()
			return nil, false, fmt.Errorf("acp session/new returned invalid sessionId: %s", string(raw))
		}
		proc.sessionID = sessionMeta.SessionID
	}

	r.applyACPSessionOptions(ctx, proc, sessionMeta)

	proc.lastUsed = time.Now()
	r.poolMu.Lock()
	if old := r.sessions[convKey]; old != nil && old != proc {
		go old.kill()
	}
	r.sessions[convKey] = proc
	r.poolMu.Unlock()
	return proc, resumed, nil
}

func (r *acpRuntime) applyACPSessionOptions(ctx context.Context, proc *acpProcess, meta acpSessionMeta) {
	modelConfigID := "model"
	thinkingConfigID := "thought_level"
	modeConfigID := "mode"
	for _, co := range meta.ConfigOptions {
		cat := strings.ToLower(strings.TrimSpace(co.Category))
		id := strings.ToLower(strings.TrimSpace(co.ID))
		switch {
		case cat == "model" || id == "model":
			modelConfigID = co.ID
		case cat == "thought_level" || id == "thought_level" || strings.Contains(id, "effort") || strings.Contains(id, "thinking") || strings.Contains(id, "reasoning"):
			thinkingConfigID = co.ID
		case cat == "mode" || id == "mode":
			modeConfigID = co.ID
		}
	}

	if model := strings.TrimSpace(r.cfg.Model); model != "" {
		if _, err := proc.call(ctx, "session/set_config_option", map[string]any{
			"sessionId": proc.sessionID,
			"configId":  modelConfigID,
			"value":     model,
		}); err != nil {
			_, _ = proc.call(ctx, "session/set_model", map[string]any{
				"sessionId": proc.sessionID,
				"modelId":   model,
			})
		}
	}
	if level := strings.TrimSpace(r.cfg.ThinkingLevel); level != "" {
		_, _ = proc.call(ctx, "session/set_config_option", map[string]any{
			"sessionId": proc.sessionID,
			"configId":  thinkingConfigID,
			"value":     level,
		})
	}
	if mode := strings.TrimSpace(r.cfg.SessionMode); mode != "" {
		if _, err := proc.call(ctx, "session/set_config_option", map[string]any{
			"sessionId": proc.sessionID,
			"configId":  modeConfigID,
			"value":     mode,
		}); err != nil {
			_, _ = proc.call(ctx, "session/set_mode", map[string]any{
				"sessionId": proc.sessionID,
				"modeId":    mode,
			})
		}
	}
}

func (r *acpRuntime) evictSession(convKey string, proc *acpProcess) {
	r.poolMu.Lock()
	if r.sessions[convKey] == proc {
		delete(r.sessions, convKey)
	}
	r.poolMu.Unlock()
}

func (r *acpRuntime) spawnACPProcess() (*acpProcess, error) {
	bin, err := ResolveBinary(r.cfg.Command)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, r.cfg.Args...)
	cmd.Dir = r.WorkingDir()
	cmd.Env = BuildSubprocessEnv(r.cfg, r.bridgeURL(), r.bridgeToken())
	configureProcessGroup(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("acp stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("acp stdout pipe: %w", err)
	}
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", r.cfg.Command, err)
	}

	p := &acpProcess{
		cmd:      cmd,
		stdin:    stdin,
		lastUsed: time.Now(),
		pending:  make(map[int64]chan jsonRPCMessage),
	}
	p.alive.Store(true)

	if stderr != nil {
		go func() {
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				log.Printf("[ACP:%s stderr] %s", r.cfg.Preset, scanner.Text())
			}
		}()
	}
	go p.readLoop(stdout)
	return p, nil
}

func (p *acpProcess) readLoop(stdout io.Reader) {
	defer func() {
		p.alive.Store(false)
		_ = p.cmd.Wait()
		p.pendingMu.Lock()
		for id, ch := range p.pending {
			close(ch)
			delete(p.pending, id)
		}
		p.pendingMu.Unlock()
	}()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var msg jsonRPCMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		switch {
		case msg.Method != "" && len(msg.ID) == 0:
			p.turnMu.RLock()
			fn := p.onNotification
			p.turnMu.RUnlock()
			if fn != nil {
				fn(msg.Method, msg.Params)
			}
		case msg.Method != "" && len(msg.ID) > 0:
			p.turnMu.RLock()
			fn := p.onRequest
			p.turnMu.RUnlock()
			if fn != nil {
				fn(msg.ID, msg.Method, msg.Params)
			}
		case len(msg.ID) > 0:
			var id int64
			if json.Unmarshal(msg.ID, &id) == nil {
				p.pendingMu.Lock()
				ch, ok := p.pending[id]
				if ok {
					delete(p.pending, id)
				}
				p.pendingMu.Unlock()
				if ok {
					ch <- msg
				}
			}
		}
	}
}

func (p *acpProcess) setHandlers(
	onNotif func(method string, params json.RawMessage),
	onReq func(id json.RawMessage, method string, params json.RawMessage),
) {
	p.turnMu.Lock()
	p.onNotification = onNotif
	p.onRequest = onReq
	p.turnMu.Unlock()
}

func (p *acpProcess) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if !p.alive.Load() {
		return nil, errors.New("acp process is not running")
	}
	id := p.nextID.Add(1)
	ch := make(chan jsonRPCMessage, 1)
	p.pendingMu.Lock()
	p.pending[id] = ch
	p.pendingMu.Unlock()

	frame, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		p.pendingMu.Lock()
		delete(p.pending, id)
		p.pendingMu.Unlock()
		return nil, err
	}

	if err := p.writeLine(frame); err != nil {
		p.pendingMu.Lock()
		delete(p.pending, id)
		p.pendingMu.Unlock()
		return nil, err
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			return nil, errors.New("acp subprocess closed unexpectedly")
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("acp error (%d): %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-ctx.Done():
		p.pendingMu.Lock()
		delete(p.pending, id)
		p.pendingMu.Unlock()
		return nil, ctx.Err()
	}
}

func (p *acpProcess) notify(method string, params any) error {
	frame, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return err
	}
	return p.writeLine(frame)
}

func (p *acpProcess) sendResult(id json.RawMessage, result any) error {
	frame, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
	if err != nil {
		return err
	}
	return p.writeLine(frame)
}

func (p *acpProcess) sendError(id json.RawMessage, code int, message string) error {
	frame, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
	if err != nil {
		return err
	}
	return p.writeLine(frame)
}

func (p *acpProcess) writeLine(line []byte) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	if p.stdin == nil {
		return errors.New("acp stdin closed")
	}
	_, err := p.stdin.Write(append(line, '\n'))
	return err
}

func (p *acpProcess) kill() {
	p.alive.Store(false)
	p.writeMu.Lock()
	if p.stdin != nil {
		_ = p.stdin.Close()
	}
	p.writeMu.Unlock()
	terminateProcessGroup(p.cmd)
}

func handleACPUpdate(raw json.RawMessage, acc *turnAccumulator) {
	var envelope struct {
		Update struct {
			SessionUpdate string `json:"sessionUpdate"`
			Content       *struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			ToolCallID string `json:"toolCallId"`
			Title      string `json:"title"`
			Kind       string `json:"kind"`
			Status     string `json:"status"`
			RawInput   any    `json:"rawInput"`
			RawOutput  any    `json:"rawOutput"`
		} `json:"update"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return
	}
	u := envelope.Update
	switch u.SessionUpdate {
	case "agent_message_chunk":
		if u.Content != nil && u.Content.Text != "" {
			acc.AddTextDelta(u.Content.Text)
		}
	case "agent_thought_chunk":
		if u.Content != nil && u.Content.Text != "" {
			acc.AddThinkingDelta(u.Content.Text)
		}
	case "tool_call":
		name := u.Title
		if name == "" {
			name = u.Kind
		}
		acc.StartTool(u.ToolCallID, name, marshalArgsJSON(u.RawInput))
	case "tool_call_update":
		name := u.Title
		if name == "" {
			name = u.Kind
		}
		output := extractACPOutput(u.RawOutput, u.Content)
		switch strings.ToLower(u.Status) {
		case "completed":
			acc.EndTool(u.ToolCallID, name, output, false)
		case "failed", "error", "cancelled", "canceled":
			acc.EndTool(u.ToolCallID, name, output, true)
		default:
			if output != "" {
				acc.UpdateTool(u.ToolCallID, name, output, false)
			}
		}
	}
}

func extractACPOutput(rawOutput any, content *struct {
	Type string `json:"type"`
	Text string `json:"text"`
}) string {
	if rawOutput != nil {
		switch v := rawOutput.(type) {
		case string:
			return v
		default:
			if b, err := json.Marshal(v); err == nil {
				return string(b)
			}
		}
	}
	if content != nil && content.Text != "" {
		return content.Text
	}
	return ""
}

func (r *acpRuntime) handleACPPermission(ctx context.Context, cfg agent.RunConfig, proc *acpProcess, id json.RawMessage, raw json.RawMessage) {
	var req struct {
		ToolCall struct {
			ToolCallID string `json:"toolCallId"`
			Title      string `json:"title"`
			Kind       string `json:"kind"`
			RawInput   any    `json:"rawInput"`
		} `json:"toolCall"`
		Options []struct {
			OptionID string `json:"optionId"`
			Name     string `json:"name"`
			Kind     string `json:"kind"`
		} `json:"options"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		_ = proc.sendError(id, -32602, "invalid permission request")
		return
	}
	toolName := req.ToolCall.Title
	if toolName == "" {
		toolName = req.ToolCall.Kind
	}
	allowed, _ := r.evaluatePermission(ctx, cfg, "acp", toolName, req.ToolCall.RawInput)

	if ctx.Err() != nil {
		_ = proc.sendResult(id, map[string]any{
			"outcome": map[string]any{"outcome": "cancelled"},
		})
		return
	}

	chosen := ""
	for _, opt := range req.Options {
		k := strings.ToLower(opt.Kind)
		if allowed && (k == "allow_once" || k == "allow_always" || strings.HasPrefix(k, "allow")) {
			chosen = opt.OptionID
			break
		}
		if !allowed && (k == "reject_once" || k == "reject_always" || strings.HasPrefix(k, "reject") || strings.HasPrefix(k, "deny")) {
			chosen = opt.OptionID
			break
		}
	}
	if chosen == "" && len(req.Options) > 0 {
		if allowed {
			chosen = req.Options[0].OptionID
		} else {
			chosen = req.Options[len(req.Options)-1].OptionID
		}
	}
	if chosen == "" {
		_ = proc.sendResult(id, map[string]any{
			"outcome": map[string]any{"outcome": "cancelled"},
		})
		return
	}
	_ = proc.sendResult(id, map[string]any{
		"outcome": map[string]any{
			"outcome":  "selected",
			"optionId": chosen,
		},
	})
}
