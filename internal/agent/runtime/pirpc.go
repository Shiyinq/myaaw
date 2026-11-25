package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"myaaw/internal/agent"
	"myaaw/internal/agentdir"
	"myaaw/internal/config"
	"myaaw/internal/provider"
)

// piRPCRuntime drives Pi (`pi --mode rpc`) and Oh My Pi (`omp --mode rpc`)
// over their newline-delimited JSONL RPC protocol on stdio, storing session
// files cleanly under ~/.myaaw/agents/<name>/runtime/sessions/.
type piRPCRuntime struct {
	externalBase

	poolMu   sync.Mutex
	sessions map[string]*piProcess
	stopReap chan struct{}
	closed   bool
}

type piProcess struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	writeMu  sync.Mutex
	alive    atomic.Bool
	lastUsed time.Time
	seeded   bool

	turnMu  sync.RWMutex
	onEvent func(line []byte)
}

func newPiRPCRuntime(cfg config.RuntimeConfig) Runtime {
	r := &piRPCRuntime{
		externalBase: newExternalBase(cfg),
		sessions:     make(map[string]*piProcess),
		stopReap:     make(chan struct{}),
	}
	go r.reapLoop()
	return r
}

func (r *piRPCRuntime) WithExcludedTools(tools ...string) agent.AgentProvider { return r }

func (r *piRPCRuntime) Run(modelName string, messages []provider.Message) (provider.Message, error) {
	return r.RunContext(context.Background(), modelName, messages)
}

func (r *piRPCRuntime) RunContext(ctx context.Context, modelName string, messages []provider.Message) (provider.Message, error) {
	return runContextAdapter(ctx, r, modelName, messages)
}

func (r *piRPCRuntime) RunStream(modelName string, messages []provider.Message, callback func(provider.Message) error) error {
	return runStreamAdapter(r, modelName, messages, callback)
}

func (r *piRPCRuntime) Close() error {
	r.poolMu.Lock()
	if !r.closed {
		r.closed = true
		close(r.stopReap)
	}
	procs := make([]*piProcess, 0, len(r.sessions))
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

func (r *piRPCRuntime) reapLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	idle := r.cfg.IdleTimeout()
	for {
		select {
		case <-r.stopReap:
			return
		case <-ticker.C:
			now := time.Now()
			var stale []*piProcess
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

func (r *piRPCRuntime) RunEvents(
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

	sessionFile := ""
	if sessionsDir, err := agentdir.ActiveRuntimeSessionsDir(); err == nil && sessionsDir != "" {
		_ = os.MkdirAll(sessionsDir, 0o700)
		sessionFile = filepath.Join(sessionsDir, convKey+".jsonl")
		if cfg.Rewind {
			_ = os.Remove(sessionFile)
		}
	}

	matchingRuntime := cfg.ExternalRuntime == "" || cfg.ExternalRuntime == r.DisplayProvider()
	wantResume := cfg.ExternalSessionID != "" && matchingRuntime && !cfg.Rewind

	proc, resumed, err := r.acquireProcess(convKey, sessionFile, cfg.Rewind, wantResume)
	if err != nil {
		return acc.Finish(provider.StopError, err)
	}
	if cfg.OnExternalSession != nil {
		sid := sessionFile
		if sid == "" {
			sid = convKey
		}
		cfg.OnExternalSession(r.DisplayProvider(), sid)
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

	proc.setEventHandler(func(line []byte) {
		var ev struct {
			Type              string `json:"type"`
			Success           *bool  `json:"success"`
			Error             string `json:"error"`
			Command           string `json:"command"`
			ToolCallID        string `json:"toolCallId"`
			ToolName          string `json:"toolName"`
			Args              any    `json:"args"`
			Result            any    `json:"result"`
			PartialResult     any    `json:"partialResult"`
			IsError           bool   `json:"isError"`
			AssistantMsgEvent *struct {
				Type  string `json:"type"`
				Delta string `json:"delta"`
			} `json:"assistantMessageEvent"`
			Message *struct {
				Usage *struct {
					Input       int `json:"input"`
					Output      int `json:"output"`
					CacheRead   int `json:"cacheRead"`
					TotalTokens int `json:"totalTokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &ev) != nil {
			return
		}
		switch ev.Type {
		case "response":
			if ev.Success != nil && !*ev.Success && ev.Command == "prompt" {
				msg := strings.TrimSpace(ev.Error)
				if msg == "" {
					msg = "pi rpc prompt failed"
				}
				signalDone(errors.New(msg))
			}
		case "message_update":
			if ev.AssistantMsgEvent == nil {
				continueEvent := false
				_ = continueEvent
				return
			}
			switch ev.AssistantMsgEvent.Type {
			case "text_delta":
				acc.AddTextDelta(ev.AssistantMsgEvent.Delta)
			case "thinking_delta":
				acc.AddThinkingDelta(ev.AssistantMsgEvent.Delta)
			}
		case "tool_execution_start":
			acc.StartTool(ev.ToolCallID, ev.ToolName, marshalArgsJSON(ev.Args))
		case "tool_execution_update":
			acc.UpdateTool(ev.ToolCallID, ev.ToolName, extractPiToolText(ev.PartialResult), false)
		case "tool_execution_end":
			acc.EndTool(ev.ToolCallID, ev.ToolName, extractPiToolText(ev.Result), ev.IsError)
		case "message_end":
			if ev.Message != nil && ev.Message.Usage != nil {
				u := ev.Message.Usage
				acc.AddUsage(provider.Usage{
					PromptTokens:      u.Input,
					CompletionTokens:  u.Output,
					CachedInputTokens: u.CacheRead,
					TotalTokens:       u.TotalTokens,
				})
			}
		case "agent_end":
			signalDone(nil)
		}
	})
	defer proc.setEventHandler(nil)

	if err := proc.writeJSON(map[string]any{
		"type":    "prompt",
		"message": promptText,
	}); err != nil {
		r.evictProcess(convKey, proc)
		proc.kill()
		return acc.Finish(provider.StopError, fmt.Errorf("write %s rpc prompt: %w", r.cfg.Type, err))
	}

	var runErr error
	select {
	case runErr = <-turnDone:
	case <-ctx.Done():
		runErr = ctx.Err()
		_ = proc.writeJSON(map[string]any{"type": "abort"})
		select {
		case <-turnDone:
		case <-time.After(2 * time.Second):
			r.evictProcess(convKey, proc)
			proc.kill()
		}
	}

	if runErr != nil {
		if ctx.Err() != nil {
			return acc.Finish(provider.StopAborted, ctx.Err())
		}
		if !proc.alive.Load() {
			r.evictProcess(convKey, proc)
		}
		return acc.Finish(provider.StopError, runErr)
	}

	proc.seeded = true
	proc.lastUsed = time.Now()
	return acc.Finish(provider.StopNormal, nil)
}

func (r *piRPCRuntime) acquireProcess(convKey, sessionFile string, rewind, wantResume bool) (*piProcess, bool, error) {
	r.poolMu.Lock()
	existing := r.sessions[convKey]
	if existing != nil && (!existing.alive.Load() || rewind) {
		delete(r.sessions, convKey)
		r.poolMu.Unlock()
		existing.kill()
		existing = nil
	} else {
		r.poolMu.Unlock()
	}

	if existing != nil && wantResume {
		existing.lastUsed = time.Now()
		return existing, existing.seeded, nil
	}

	args := append([]string(nil), r.cfg.Args...)
	if len(args) == 0 {
		args = []string{"--mode", "rpc"}
	}
	if model := strings.TrimSpace(r.cfg.Model); model != "" {
		args = append(args, "--model", model)
	}
	if thinking := strings.TrimSpace(r.cfg.ThinkingLevel); thinking != "" {
		args = append(args, "--thinking", thinking)
	}
	if sessionFile != "" {
		args = append(args, "--session", sessionFile)
	}
	if r.cfg.Type == config.RuntimeTypePi {
		mcpSpecs := BuildMCPServers(r.cfg.MCPBridgeEnabled(), r.bridgeURL(), r.bridgeToken())
		if _, err := SyncPiProjectMCPConfig(r.WorkingDir(), mcpSpecs); err != nil {
			log.Printf("[Pi] sync .pi/mcp.json: %v", err)
		}
		if len(mcpSpecs) > 0 && !hasArgFlag(args, "--approve", "-a") {
			args = append(args, "--approve")
		}
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

	p := &piProcess{
		cmd:      cmd,
		stdin:    stdin,
		lastUsed: time.Now(),
	}
	p.alive.Store(true)
	if sessionFile != "" && wantResume {
		if info, err := os.Stat(sessionFile); err == nil && info.Size() > 0 {
			p.seeded = true
		}
	}

	if stderr != nil {
		go func() {
			sc := bufio.NewScanner(stderr)
			for sc.Scan() {
				log.Printf("[%s stderr] %s", r.cfg.Type, sc.Text())
			}
		}()
	}

	go func() {
		defer func() {
			p.alive.Store(false)
			_ = cmd.Wait()
		}()
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64*1024), 8*1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || !strings.HasPrefix(line, "{") {
				continue
			}
			p.turnMu.RLock()
			fn := p.onEvent
			p.turnMu.RUnlock()
			if fn != nil {
				fn([]byte(line))
			}
		}
	}()

	r.poolMu.Lock()
	if old := r.sessions[convKey]; old != nil && old != p {
		go old.kill()
	}
	r.sessions[convKey] = p
	r.poolMu.Unlock()
	return p, p.seeded, nil
}

func (r *piRPCRuntime) evictProcess(convKey string, proc *piProcess) {
	r.poolMu.Lock()
	if r.sessions[convKey] == proc {
		delete(r.sessions, convKey)
	}
	r.poolMu.Unlock()
}

func (p *piProcess) setEventHandler(fn func([]byte)) {
	p.turnMu.Lock()
	p.onEvent = fn
	p.turnMu.Unlock()
}

func (p *piProcess) writeJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	if p.stdin == nil {
		return errors.New("pi rpc stdin closed")
	}
	_, err = p.stdin.Write(append(data, '\n'))
	return err
}

func (p *piProcess) kill() {
	p.alive.Store(false)
	p.writeMu.Lock()
	if p.stdin != nil {
		_ = p.stdin.Close()
	}
	p.writeMu.Unlock()
	terminateProcessGroup(p.cmd)
}

func extractPiToolText(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	if m, ok := v.(map[string]any); ok {
		if content, ok := m["content"].([]any); ok {
			var parts []string
			for _, item := range content {
				if block, ok := item.(map[string]any); ok {
					if t, ok := block["text"].(string); ok && t != "" {
						parts = append(parts, t)
					}
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, "\n")
			}
		}
	}
	return marshalArgsJSON(v)
}

func hasArgFlag(args []string, flags ...string) bool {
	for _, a := range args {
		for _, f := range flags {
			if a == f {
				return true
			}
		}
	}
	return false
}
