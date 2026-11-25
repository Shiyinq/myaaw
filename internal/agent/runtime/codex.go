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

	proc.setHandlers(
		func(method string, params json.RawMessage) {
			switch method {
			case "item/agentMessage/delta":
				var d struct {
					Delta string `json:"delta"`
				}
				if json.Unmarshal(params, &d) == nil {
					acc.AddTextDelta(d.Delta)
				}
			case "item/reasoning/textDelta", "item/reasoning/summaryTextDelta":
				var d struct {
					Delta string `json:"delta"`
				}
				if json.Unmarshal(params, &d) == nil {
					acc.AddThinkingDelta(d.Delta)
				}
			case "item/started":
				var ev struct {
					Item struct {
						ID      string `json:"id"`
						Type    string `json:"type"`
						Command any    `json:"command"`
						Tool    string `json:"tool"`
						Args    any    `json:"arguments"`
					} `json:"item"`
				}
				if json.Unmarshal(params, &ev) == nil {
					switch ev.Item.Type {
					case "commandExecution":
						acc.StartTool(ev.Item.ID, "command", marshalArgsJSON(ev.Item.Command))
					case "fileChange":
						acc.StartTool(ev.Item.ID, "file_change", "{}")
					case "mcpToolCall":
						acc.StartTool(ev.Item.ID, ev.Item.Tool, marshalArgsJSON(ev.Item.Args))
					}
				}
			case "item/completed":
				var ev struct {
					Item struct {
						ID               string `json:"id"`
						Type             string `json:"type"`
						Status           string `json:"status"`
						AggregatedOutput string `json:"aggregatedOutput"`
						Result           any    `json:"result"`
						Tool             string `json:"tool"`
					} `json:"item"`
				}
				if json.Unmarshal(params, &ev) == nil {
					isErr := strings.EqualFold(ev.Item.Status, "failed") || strings.EqualFold(ev.Item.Status, "declined")
					switch ev.Item.Type {
					case "commandExecution":
						acc.EndTool(ev.Item.ID, "command", ev.Item.AggregatedOutput, isErr)
					case "fileChange":
						acc.EndTool(ev.Item.ID, "file_change", ev.Item.Status, isErr)
					case "mcpToolCall":
						acc.EndTool(ev.Item.ID, ev.Item.Tool, marshalArgsJSON(ev.Item.Result), isErr)
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
					acc.AddUsage(provider.Usage{
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
		},
		func(id json.RawMessage, method string, params json.RawMessage) {
			switch method {
			case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
				go func() {
					var req struct {
						Command any `json:"command"`
						Reason  any `json:"reason"`
					}
					_ = json.Unmarshal(params, &req)
					toolName := "command"
					if strings.Contains(method, "fileChange") {
						toolName = "file_change"
					}
					allowed, _ := r.evaluatePermission(ctx, cfg, "codex", toolName, params)
					decision := "accept"
					if !allowed {
						decision = "decline"
					}
					_ = proc.sendResult(id, map[string]any{"decision": decision})
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
	if effort := strings.TrimSpace(r.cfg.ThinkingLevel); effort != "" {
		startParams["effort"] = effort
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

	args := append([]string(nil), r.cfg.Args...)
	if len(args) == 0 {
		args = []string{"app-server"}
	}
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
		raw, err := proc.call(ctx, "thread/start", map[string]any{
			"cwd":            r.WorkingDir(),
			"approvalPolicy": approvalPolicy,
		})
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
