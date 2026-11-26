package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"myaaw/internal/agent"
	"myaaw/internal/config"
	"myaaw/internal/provider"
)

// codexRuntime drives the official OpenAI Codex CLI via `codex app-server`
// (JSON-RPC 2.0 over stdio), supporting persistent threads, streaming deltas,
// and bidirectional tool/file approval bridging.
type codexRuntime struct {
	externalBase

	poolMu   sync.Mutex
	sessions map[string]*acpProcess // reuses the generic JSON-RPC 2.0 stdio transport
	stopReap chan struct{}
	closed   bool
}

func newCodexRuntime(cfg config.RuntimeConfig) Runtime {
	r := &codexRuntime{
		externalBase: newExternalBase(cfg),
		sessions:     make(map[string]*acpProcess),
		stopReap:     make(chan struct{}),
	}
	go r.reapLoop()
	return r
}

func (r *codexRuntime) WithExcludedTools(tools ...string) agent.AgentProvider { return r }

func (r *codexRuntime) Run(modelName string, messages []provider.Message) (provider.Message, error) {
	return r.RunContext(context.Background(), modelName, messages)
}

func (r *codexRuntime) RunContext(ctx context.Context, modelName string, messages []provider.Message) (provider.Message, error) {
	return runContextAdapter(ctx, r, modelName, messages)
}

func (r *codexRuntime) RunStream(modelName string, messages []provider.Message, callback func(provider.Message) error) error {
	return runStreamAdapter(r, modelName, messages, callback)
}

func (r *codexRuntime) Close() error {
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

func (r *codexRuntime) reapLoop() {
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

func (r *codexRuntime) RunEvents(
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

	proc, resumed, err := r.acquireThread(ctx, convKey, cfg, wantResume)
	if err != nil {
		return acc.Finish(provider.StopError, err)
	}
	if cfg.OnExternalSession != nil && proc.sessionID != "" {
		cfg.OnExternalSession(r.DisplayProvider(), proc.sessionID)
	}

	systemPrompt, userText, _ := BuildTurnInput(history, resumed, cfg.Rewind)
	promptText := userText
	if !resumed && systemPrompt != "" {
		promptText = "<system_instructions>\n" + systemPrompt + "\n</system_instructions>\n\n" + userText
	}

	turnDone := make(chan error, 1)
	finishOnce := sync.Once{}
	signalDone := func(err error) {
		finishOnce.Do(func() { turnDone <- err })
	}

	var activeTurnID string
	var turnIDMu sync.Mutex
	turnState := newCodexTurnState(acc)

	proc.setHandlers(
		func(method string, params json.RawMessage) {
			turnState.handleNotification(method, params, signalDone)
		},
		func(id json.RawMessage, method string, params json.RawMessage) {
			switch method {
			case "item/commandExecution/requestApproval", "item/fileChange/requestApproval", "item/mcpToolCall/requestApproval":
				go func() {
					var req struct {
						Command any    `json:"command"`
						Reason  any    `json:"reason"`
						Tool    string `json:"tool"`
						Server  string `json:"server"`
					}
					_ = json.Unmarshal(params, &req)
					toolName := "command"
					switch {
					case strings.Contains(method, "fileChange"):
						toolName = "file_change"
					case strings.Contains(method, "mcpToolCall"):
						toolName = strings.TrimSpace(req.Tool)
						if toolName == "" {
							toolName = "mcp_tool"
						}
						if srv := strings.TrimSpace(req.Server); srv != "" {
							toolName = "mcp:" + srv + ":" + toolName
						}
					}
					allowed, _ := r.evaluatePermission(ctx, cfg, "codex", toolName, params)
					decision := "accept"
					if !allowed {
						decision = "decline"
					}
					_ = proc.sendResult(id, map[string]any{"decision": decision})
				}()
			case "mcpServer/elicitation/request":
				go func() {
					var req struct {
						ServerName string         `json:"serverName"`
						Mode       string         `json:"mode"`
						Message    string         `json:"message"`
						URL        string         `json:"url"`
						Meta       map[string]any `json:"_meta"`
					}
					_ = json.Unmarshal(params, &req)
					serverName := strings.TrimSpace(req.ServerName)
					if serverName == "" {
						serverName = "mcp"
					}
					toolName := "mcp:" + serverName
					for _, k := range []string{"tool_name", "toolName", "codex_tool_name", "tool"} {
						if v, ok := req.Meta[k].(string); ok && strings.TrimSpace(v) != "" {
							toolName = "mcp:" + serverName + ":" + strings.TrimSpace(v)
							break
						}
					}
					summary := map[string]any{
						"server":  serverName,
						"message": strings.TrimSpace(req.Message),
					}
					if strings.TrimSpace(req.URL) != "" {
						summary["url"] = strings.TrimSpace(req.URL)
					}
					allowed, _ := r.evaluatePermission(ctx, cfg, "codex", toolName, summary)
					if allowed {
						var content any = map[string]any{}
						if strings.EqualFold(strings.TrimSpace(req.Mode), "url") {
							content = nil
						}
						_ = proc.sendResult(id, map[string]any{
							"action":  "accept",
							"content": content,
							"_meta":   nil,
						})
					} else {
						_ = proc.sendResult(id, map[string]any{
							"action":  "decline",
							"content": nil,
							"_meta":   nil,
						})
					}
				}()
			case "item/permissions/requestApproval":
				go func() {
					var req struct {
						Reason      string          `json:"reason"`
						Permissions json.RawMessage `json:"permissions"`
					}
					_ = json.Unmarshal(params, &req)
					allowed, _ := r.evaluatePermission(ctx, cfg, "codex", "permissions", params)
					var granted any = map[string]any{}
					if allowed && len(req.Permissions) > 0 && string(req.Permissions) != "null" {
						granted = req.Permissions
					}
					_ = proc.sendResult(id, map[string]any{
						"permissions": granted,
						"scope":       "turn",
					})
				}()
			default:
				_ = proc.sendError(id, -32601, "unsupported codex client method: "+method)
			}
		},
	)
	defer proc.setHandlers(nil, nil)

	startParams := map[string]any{
		"threadId": proc.sessionID,
		"input": []map[string]any{
			{"type": "text", "text": promptText},
		},
	}
	if model := strings.TrimSpace(r.cfg.Model); model != "" {
		startParams["model"] = model
	}
	effort, summary := codexReasoningParams(r.cfg.ThinkingLevel)
	if effort != "" {
		startParams["effort"] = effort
	}
	if summary != "" {
		startParams["summary"] = summary
	}

	rawTurn, err := proc.call(ctx, "turn/start", startParams)
	if err != nil {
		if ctx.Err() != nil {
			return acc.Finish(provider.StopAborted, ctx.Err())
		}
		return acc.Finish(provider.StopError, fmt.Errorf("codex turn/start: %w", err))
	}
	var turnResp struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if json.Unmarshal(rawTurn, &turnResp) == nil {
		turnIDMu.Lock()
		activeTurnID = turnResp.Turn.ID
		turnIDMu.Unlock()
	}

	var runErr error
	select {
	case runErr = <-turnDone:
	case <-ctx.Done():
		runErr = ctx.Err()
		turnIDMu.Lock()
		tid := activeTurnID
		turnIDMu.Unlock()
		if tid != "" {
			_, _ = proc.call(context.Background(), "turn/interrupt", map[string]any{
				"threadId": proc.sessionID,
				"turnId":   tid,
			})
		}
		select {
		case <-turnDone:
		case <-time.After(2 * time.Second):
			r.evictThread(convKey, proc)
			proc.kill()
		}
	}

	if runErr != nil {
		if ctx.Err() != nil {
			return acc.Finish(provider.StopAborted, ctx.Err())
		}
		return acc.Finish(provider.StopError, runErr)
	}
	proc.lastUsed = time.Now()
	return acc.Finish(provider.StopNormal, nil)
}

func (r *codexRuntime) acquireThread(ctx context.Context, convKey string, cfg agent.RunConfig, wantResume bool) (*acpProcess, bool, error) {
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

	effort, summary := codexReasoningParams(r.cfg.ThinkingLevel)
	args := codexSubprocessArgs(r.cfg, true)
	mcpSpecs := BuildMCPServers(r.cfg.MCPBridgeEnabled(), r.bridgeURL(), r.bridgeToken())
	args = append(args, BuildCodexMCPArgs(mcpSpecs)...)

	bin, err := ResolveBinary(r.cfg.Command)
	if err != nil {
		return nil, false, err
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = r.WorkingDir()
	cmd.Env = BuildSubprocessEnv(r.cfg, r.bridgeURL(), r.bridgeToken())
	configureProcessGroup(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, false, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, false, err
	}
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return nil, false, fmt.Errorf("start %s: %w", r.cfg.Command, err)
	}

	proc := &acpProcess{
		cmd:      cmd,
		stdin:    stdin,
		lastUsed: time.Now(),
		pending:  make(map[int64]chan jsonRPCMessage),
	}
	proc.alive.Store(true)
	if stderr != nil {
		go func() {
			sc := bufio.NewScanner(stderr)
			for sc.Scan() {
				log.Printf("[Codex stderr] %s", sc.Text())
			}
		}()
	}
	go proc.readLoop(stdout)

	if _, err := proc.call(ctx, "initialize", map[string]any{
		"clientInfo": map[string]any{"name": "myaaw", "version": "1.0.0"},
	}); err != nil {
		proc.kill()
		return nil, false, fmt.Errorf("codex initialize: %w", err)
	}
	_ = proc.notify("initialized", map[string]any{})

	resumed := false
	if wantResume && cfg.ExternalSessionID != "" {
		if _, err := proc.call(ctx, "thread/resume", map[string]any{
			"threadId": cfg.ExternalSessionID,
		}); err == nil {
			proc.sessionID = cfg.ExternalSessionID
			resumed = true
		} else {
			log.Printf("[Codex] thread/resume failed for %s (%v); starting fresh thread", cfg.ExternalSessionID, err)
		}
	}

	if !resumed {
		approvalPolicy := "on-request"
		if r.cfg.PermissionMode == config.PermissionModeAuto {
			approvalPolicy = "never"
		}
		threadParams := map[string]any{
			"cwd":            r.WorkingDir(),
			"approvalPolicy": approvalPolicy,
		}
		if model := strings.TrimSpace(r.cfg.Model); model != "" {
			threadParams["model"] = model
		}
		threadCfg := map[string]any{}
		if effort != "" {
			threadCfg["model_reasoning_effort"] = effort
		}
		if summary != "" {
			threadCfg["model_reasoning_summary"] = summary
		}
		if len(threadCfg) > 0 {
			threadParams["config"] = threadCfg
		}
		raw, err := proc.call(ctx, "thread/start", threadParams)
		if err != nil {
			proc.kill()
			return nil, false, fmt.Errorf("codex thread/start: %w", err)
		}
		var resp struct {
			Thread struct {
				ID string `json:"id"`
			} `json:"thread"`
			ThreadID string `json:"threadId"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			proc.kill()
			return nil, false, fmt.Errorf("parse codex thread/start: %w", err)
		}
		tid := resp.Thread.ID
		if tid == "" {
			tid = resp.ThreadID
		}
		if tid == "" {
			proc.kill()
			return nil, false, errors.New("codex thread/start returned empty thread id")
		}
		proc.sessionID = tid
	}

	proc.lastUsed = time.Now()
	r.poolMu.Lock()
	if old := r.sessions[convKey]; old != nil && old != proc {
		go old.kill()
	}
	r.sessions[convKey] = proc
	r.poolMu.Unlock()
	return proc, resumed, nil
}

func (r *codexRuntime) evictThread(convKey string, proc *acpProcess) {
	r.poolMu.Lock()
	if r.sessions[convKey] == proc {
		delete(r.sessions, convKey)
	}
	r.poolMu.Unlock()
}

// codexReasoningParams converts Myaaw's thinking_level setting into Codex
// app-server effort and summary values. Matches Myaaw's native ChatGPT
// provider (`summary: "auto"`) so Codex requests reasoning summaries instead
// of leaving `model_reasoning_summary` unset (`summary: []`).
func codexReasoningParams(thinkingLevel string) (effort string, summary string) {
	lvl := strings.ToLower(strings.TrimSpace(thinkingLevel))
	if lvl == "off" || lvl == "none" {
		return "none", "none"
	}
	return lvl, "auto"
}

type codexTurnState struct {
	acc                *turnAccumulator
	activeAgentMsgID   string
	activeReasoningID  string
	streamedAgentMsgs  map[string]bool
	streamedReasonings map[string]bool
}

func newCodexTurnState(acc *turnAccumulator) *codexTurnState {
	return &codexTurnState{
		acc:                acc,
		streamedAgentMsgs:  make(map[string]bool),
		streamedReasonings: make(map[string]bool),
	}
}

func (s *codexTurnState) handleNotification(method string, params json.RawMessage, signalDone func(error)) {
	switch method {
	case "item/agentMessage/delta":
		var d struct {
			ItemID string `json:"itemId"`
			Delta  string `json:"delta"`
		}
		if json.Unmarshal(params, &d) == nil && d.Delta != "" {
			if d.ItemID != "" && s.activeAgentMsgID != "" && d.ItemID != s.activeAgentMsgID {
				s.acc.SplitAssistantMessage()
			}
			if d.ItemID != "" {
				s.activeAgentMsgID = d.ItemID
				s.streamedAgentMsgs[d.ItemID] = true
			}
			s.acc.AddTextDelta(d.Delta)
		}

	case "item/reasoning/summaryPartAdded":
		var d struct {
			ItemID       string `json:"itemId"`
			SummaryIndex int    `json:"summaryIndex"`
		}
		if json.Unmarshal(params, &d) == nil && d.SummaryIndex > 0 && s.streamedReasonings[d.ItemID] {
			s.acc.AddThinkingSeparator()
		}

	case "item/reasoning/textDelta", "item/reasoning/summaryTextDelta":
		var d struct {
			ItemID string `json:"itemId"`
			Delta  string `json:"delta"`
		}
		if json.Unmarshal(params, &d) == nil && d.Delta != "" {
			if d.ItemID != "" && s.activeReasoningID != "" && d.ItemID != s.activeReasoningID {
				s.acc.AddThinkingSeparator()
			}
			if d.ItemID != "" {
				s.activeReasoningID = d.ItemID
				s.streamedReasonings[d.ItemID] = true
			}
			s.acc.AddThinkingDelta(d.Delta)
		}

	case "item/started":
		var ev struct {
			Item struct {
				ID         string `json:"id"`
				Type       string `json:"type"`
				Command    any    `json:"command"`
				Changes    any    `json:"changes"`
				Server     string `json:"server"`
				Tool       string `json:"tool"`
				Args       any    `json:"arguments"`
				Query      string `json:"query"`
				Prompt     string `json:"prompt"`
				Path       string `json:"path"`
				DurationMs int64  `json:"durationMs"`
			} `json:"item"`
		}
		if json.Unmarshal(params, &ev) == nil {
			switch ev.Item.Type {
			case "commandExecution":
				s.activeAgentMsgID = ""
				s.acc.StartTool(ev.Item.ID, "command", marshalArgsJSON(ev.Item.Command))
			case "fileChange":
				s.activeAgentMsgID = ""
				args := "{}"
				if ev.Item.Changes != nil {
					args = marshalArgsJSON(ev.Item.Changes)
				}
				s.acc.StartTool(ev.Item.ID, "file_change", args)
			case "mcpToolCall":
				s.activeAgentMsgID = ""
				s.acc.StartTool(ev.Item.ID, codexToolName(ev.Item.Tool, ev.Item.Server, "mcp_tool"), marshalArgsJSON(ev.Item.Args))
			case "dynamicToolCall":
				s.activeAgentMsgID = ""
				s.acc.StartTool(ev.Item.ID, codexToolName(ev.Item.Tool, "", "tool"), marshalArgsJSON(ev.Item.Args))
			case "webSearch":
				s.activeAgentMsgID = ""
				args := "{}"
				if strings.TrimSpace(ev.Item.Query) != "" {
					args = marshalArgsJSON(map[string]any{"query": ev.Item.Query})
				}
				s.acc.StartTool(ev.Item.ID, "web_search", args)
			case "collabAgentToolCall":
				s.activeAgentMsgID = ""
				args := "{}"
				if strings.TrimSpace(ev.Item.Prompt) != "" {
					args = marshalArgsJSON(map[string]any{"prompt": ev.Item.Prompt})
				}
				s.acc.StartTool(ev.Item.ID, codexToolName(ev.Item.Tool, "", "agent"), args)
			case "imageView":
				s.activeAgentMsgID = ""
				args := "{}"
				if strings.TrimSpace(ev.Item.Path) != "" {
					args = marshalArgsJSON(map[string]any{"path": ev.Item.Path})
				}
				s.acc.StartTool(ev.Item.ID, "image_view", args)
			case "imageGeneration":
				s.activeAgentMsgID = ""
				s.acc.StartTool(ev.Item.ID, "image_generation", "{}")
			case "sleep":
				s.activeAgentMsgID = ""
				args := "{}"
				if ev.Item.DurationMs > 0 {
					args = marshalArgsJSON(map[string]any{"duration_ms": ev.Item.DurationMs})
				}
				s.acc.StartTool(ev.Item.ID, "sleep", args)
			}
		}

	case "item/completed":
		var ev struct {
			Item struct {
				ID               string   `json:"id"`
				Type             string   `json:"type"`
				Status           string   `json:"status"`
				Success          *bool    `json:"success"`
				Text             string   `json:"text"`
				Summary          []string `json:"summary"`
				Content          []string `json:"content"`
				AggregatedOutput string   `json:"aggregatedOutput"`
				Result           any      `json:"result"`
				Results          any      `json:"results"`
				ContentItems     any      `json:"contentItems"`
				Output           any      `json:"output"`
				Query            string   `json:"query"`
				Path             string   `json:"path"`
				SavedPath        string   `json:"savedPath"`
				Failure          string   `json:"failure"`
				Name             string   `json:"name"`
				Server           string   `json:"server"`
				Tool             string   `json:"tool"`
				DurationMs       int64    `json:"durationMs"`
				Error            *struct {
					Message string `json:"message"`
				} `json:"error"`
			} `json:"item"`
		}
		if json.Unmarshal(params, &ev) == nil {
			isErr := strings.EqualFold(ev.Item.Status, "failed") ||
				strings.EqualFold(ev.Item.Status, "declined") ||
				ev.Item.Error != nil ||
				(ev.Item.Success != nil && !*ev.Item.Success) ||
				strings.TrimSpace(ev.Item.Failure) != ""

			switch ev.Item.Type {
			case "reasoning":
				if !s.streamedReasonings[ev.Item.ID] {
					if text := extractCodexReasoningText(ev.Item.Summary, ev.Item.Content); text != "" {
						if ev.Item.ID != "" && s.activeReasoningID != "" && ev.Item.ID != s.activeReasoningID {
							s.acc.AddThinkingSeparator()
						}
						if ev.Item.ID != "" {
							s.activeReasoningID = ev.Item.ID
							s.streamedReasonings[ev.Item.ID] = true
						}
						s.acc.AddThinkingDelta(text)
					}
				}
			case "agentMessage":
				if !s.streamedAgentMsgs[ev.Item.ID] && ev.Item.Text != "" {
					if ev.Item.ID != "" && s.activeAgentMsgID != "" && ev.Item.ID != s.activeAgentMsgID {
						s.acc.SplitAssistantMessage()
					}
					if ev.Item.ID != "" {
						s.activeAgentMsgID = ev.Item.ID
						s.streamedAgentMsgs[ev.Item.ID] = true
					}
					s.acc.AddTextDelta(ev.Item.Text)
				}
			case "commandExecution":
				s.acc.EndTool(ev.Item.ID, "command", ev.Item.AggregatedOutput, isErr)
			case "fileChange":
				s.acc.EndTool(ev.Item.ID, "file_change", ev.Item.Status, isErr)
			case "mcpToolCall":
				output := extractPiToolText(ev.Item.Result)
				if ev.Item.Error != nil && strings.TrimSpace(ev.Item.Error.Message) != "" {
					output = ev.Item.Error.Message
				}
				s.acc.EndTool(ev.Item.ID, codexToolName(ev.Item.Tool, ev.Item.Server, "mcp_tool"), output, isErr)
			case "dynamicToolCall":
				output := extractCodexContentItems(ev.Item.ContentItems)
				s.acc.EndTool(ev.Item.ID, codexToolName(ev.Item.Tool, "", "tool"), output, isErr)
			case "webSearch":
				output := extractPiToolText(ev.Item.Results)
				if strings.TrimSpace(output) == "" {
					output = ev.Item.Query
				}
				s.acc.EndTool(ev.Item.ID, "web_search", output, isErr)
			case "collabAgentToolCall":
				s.acc.EndTool(ev.Item.ID, codexToolName(ev.Item.Tool, "", "agent"), ev.Item.Status, isErr)
			case "imageView":
				s.acc.EndTool(ev.Item.ID, "image_view", ev.Item.Path, isErr)
			case "imageGeneration":
				output := strings.TrimSpace(ev.Item.SavedPath)
				if strings.TrimSpace(ev.Item.Failure) != "" {
					output = ev.Item.Failure
				}
				if output == "" {
					output = ev.Item.Status
				}
				s.acc.EndTool(ev.Item.ID, "image_generation", output, isErr)
			case "functionCallOutput":
				s.acc.EndTool(ev.Item.ID, codexToolName(ev.Item.Name, "", "tool"), extractPiToolText(ev.Item.Output), isErr)
			case "sleep":
				output := "Sleep completed."
				if ev.Item.DurationMs > 0 {
					output = fmt.Sprintf("Slept %dms", ev.Item.DurationMs)
				}
				s.acc.EndTool(ev.Item.ID, "sleep", output, isErr)
			}
		}

	case "thread/tokenUsage/updated":
		var ev struct {
			TokenUsage struct {
				Last struct {
					InputTokens           int `json:"inputTokens"`
					OutputTokens          int `json:"outputTokens"`
					CachedInputTokens     int `json:"cachedInputTokens"`
					ReasoningOutputTokens int `json:"reasoningOutputTokens"`
					TotalTokens           int `json:"totalTokens"`
				} `json:"last"`
			} `json:"tokenUsage"`
		}
		if json.Unmarshal(params, &ev) == nil {
			u := ev.TokenUsage.Last
			s.acc.AddUsage(provider.Usage{
				PromptTokens:      u.InputTokens,
				CompletionTokens:  u.OutputTokens,
				CachedInputTokens: u.CachedInputTokens,
				ThoughtsTokens:    u.ReasoningOutputTokens,
				TotalTokens:       u.TotalTokens,
			})
		}

	case "turn/completed":
		var ev struct {
			Turn struct {
				Status string `json:"status"`
				Error  *struct {
					Message string `json:"message"`
				} `json:"error"`
			} `json:"turn"`
		}
		if json.Unmarshal(params, &ev) == nil && ev.Turn.Error != nil && ev.Turn.Error.Message != "" {
			signalDone(errors.New(ev.Turn.Error.Message))
			return
		}
		signalDone(nil)
	}
}

func codexToolName(tool, server, fallback string) string {
	if t := strings.TrimSpace(tool); t != "" {
		return t
	}
	if s := strings.TrimSpace(server); s != "" {
		return s
	}
	return fallback
}

func extractCodexReasoningText(summary, content []string) string {
	join := func(items []string) string {
		parts := make([]string, 0, len(items))
		for _, item := range items {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				parts = append(parts, trimmed)
			}
		}
		return strings.Join(parts, "\n\n")
	}
	if text := join(summary); text != "" {
		return text
	}
	return join(content)
}

func extractCodexContentItems(v any) string {
	if v == nil {
		return ""
	}
	if arr, ok := v.([]any); ok {
		parts := make([]string, 0, len(arr))
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				if text, ok := m["text"].(string); ok && text != "" {
					parts = append(parts, text)
				}
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	return extractPiToolText(v)
}
