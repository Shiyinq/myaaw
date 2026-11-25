package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"

	"myaaw/internal/agent"
	"myaaw/internal/config"
	"myaaw/internal/provider"
)

// claudeRuntime drives the official Claude Code CLI via bidirectional
// `--input-format stream-json --output-format stream-json --verbose
// --include-partial-messages --permission-prompt-tool stdio`.
type claudeRuntime struct {
	externalBase
}

func newClaudeRuntime(cfg config.RuntimeConfig) Runtime {
	return &claudeRuntime{
		externalBase: newExternalBase(cfg),
	}
}

func (r *claudeRuntime) WithExcludedTools(tools ...string) agent.AgentProvider { return r }

func (r *claudeRuntime) Run(modelName string, messages []provider.Message) (provider.Message, error) {
	return r.RunContext(context.Background(), modelName, messages)
}

func (r *claudeRuntime) RunContext(ctx context.Context, modelName string, messages []provider.Message) (provider.Message, error) {
	return runContextAdapter(ctx, r, modelName, messages)
}

func (r *claudeRuntime) RunStream(modelName string, messages []provider.Message, callback func(provider.Message) error) error {
	return runStreamAdapter(r, modelName, messages, callback)
}

func (r *claudeRuntime) Close() error {
	r.closeBridge()
	return nil
}

func (r *claudeRuntime) RunEvents(
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

	matchingRuntime := cfg.ExternalRuntime == "" || cfg.ExternalRuntime == r.DisplayProvider()
	resuming := cfg.ExternalSessionID != "" && matchingRuntime && !cfg.Rewind
	systemPrompt, userText, images := BuildTurnInput(history, resuming, cfg.Rewind)

	args := []string{
		"-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
		"--include-partial-messages",
		"--permission-prompt-tool", "stdio",
	}
	if len(r.cfg.Args) > 0 {
		args = append(args, r.cfg.Args...)
	}
	if resuming {
		args = append(args, "--resume", cfg.ExternalSessionID)
	}
	if systemPrompt != "" {
		args = append(args, "--append-system-prompt", systemPrompt)
	}
	if model := strings.TrimSpace(r.cfg.Model); model != "" {
		args = append(args, "--model", model)
	}
	if effort := strings.TrimSpace(r.cfg.ThinkingLevel); effort != "" {
		args = append(args, "--effort", effort)
	}
	if mode := strings.TrimSpace(r.cfg.SessionMode); mode != "" {
		args = append(args, "--permission-mode", mode)
	}
	for _, dir := range AllowedDirs(r.cfg) {
		args = append(args, "--add-dir", dir)
	}

	mcpSpecs := BuildMCPServers(r.cfg.MCPBridgeEnabled(), r.bridgeURL(), r.bridgeToken())
	if mcpFile, err := WriteMCPConfigFile(mcpSpecs); err == nil && mcpFile != "" {
		args = append(args, "--mcp-config", mcpFile)
	}

	bin, err := ResolveBinary(r.cfg.Command)
	if err != nil {
		return acc.Finish(provider.StopError, err)
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = r.WorkingDir()
	cmd.Env = BuildSubprocessEnv(r.cfg, r.bridgeURL(), r.bridgeToken())
	configureProcessGroup(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return acc.Finish(provider.StopError, fmt.Errorf("claude stdin pipe: %w", err))
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return acc.Finish(provider.StopError, fmt.Errorf("claude stdout pipe: %w", err))
	}
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return acc.Finish(provider.StopError, fmt.Errorf("start %s: %w", r.cfg.Command, err))
	}

	var writeMu sync.Mutex
	writeJSON := func(v any) error {
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		writeMu.Lock()
		defer writeMu.Unlock()
		_, err = stdin.Write(append(data, '\n'))
		return err
	}

	if stderr != nil {
		go func() {
			sc := bufio.NewScanner(stderr)
			for sc.Scan() {
				log.Printf("[Claude stderr] %s", sc.Text())
			}
		}()
	}

	// Cancel watcher kills the whole process group when the run is stopped.
	stopWatch := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			terminateProcessGroup(cmd)
		case <-stopWatch:
		}
	}()
	defer close(stopWatch)

	// Send the user message payload via stream-json stdin.
	var content any = userText
	if len(images) > 0 {
		blocks := make([]map[string]any, 0, len(images)+1)
		if userText != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": userText})
		}
		for _, img := range images {
			blocks = append(blocks, map[string]any{
				"type": "image",
				"source": map[string]any{
					"type":       "base64",
					"media_type": "image/jpeg",
					"data":       img,
				},
			})
		}
		content = blocks
	}
	if err := writeJSON(map[string]any{
		"type": "user",
		"message": map[string]any{
			"role":    "user",
			"content": content,
		},
	}); err != nil {
		terminateProcessGroup(cmd)
		return acc.Finish(provider.StopError, fmt.Errorf("write claude prompt: %w", err))
	}

	var (
		sawDelta       bool
		activeToolID   string
		activeToolName string
		activeToolArgs strings.Builder
		resultErr      error
	)

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var frame map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			continue
		}
		var msgType string
		_ = json.Unmarshal(frame["type"], &msgType)

		switch msgType {
		case "system":
			var sys struct {
				Subtype   string `json:"subtype"`
				SessionID string `json:"session_id"`
			}
			if json.Unmarshal([]byte(line), &sys) == nil && sys.SessionID != "" && cfg.OnExternalSession != nil {
				cfg.OnExternalSession(r.DisplayProvider(), sys.SessionID)
			}

		case "stream_event":
			var ev struct {
				Event struct {
					Type         string `json:"type"`
					ContentBlock *struct {
						Type string `json:"type"`
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"content_block"`
					Delta *struct {
						Type        string `json:"type"`
						Text        string `json:"text"`
						Thinking    string `json:"thinking"`
						PartialJSON string `json:"partial_json"`
					} `json:"delta"`
				} `json:"event"`
			}
			if json.Unmarshal([]byte(line), &ev) != nil {
				continue
			}
			switch ev.Event.Type {
			case "content_block_start":
				if ev.Event.ContentBlock != nil && ev.Event.ContentBlock.Type == "tool_use" {
					activeToolID = ev.Event.ContentBlock.ID
					activeToolName = ev.Event.ContentBlock.Name
					activeToolArgs.Reset()
				}
			case "content_block_delta":
				if ev.Event.Delta == nil {
					continue
				}
				switch ev.Event.Delta.Type {
				case "text_delta":
					if ev.Event.Delta.Text != "" {
						sawDelta = true
						acc.AddTextDelta(ev.Event.Delta.Text)
					}
				case "thinking_delta":
					if ev.Event.Delta.Thinking != "" {
						acc.AddThinkingDelta(ev.Event.Delta.Thinking)
					}
				case "input_json_delta":
					activeToolArgs.WriteString(ev.Event.Delta.PartialJSON)
				}
			case "content_block_stop":
				if activeToolName != "" {
					acc.StartTool(activeToolID, activeToolName, activeToolArgs.String())
					activeToolID = ""
					activeToolName = ""
					activeToolArgs.Reset()
				}
			}

		case "assistant":
			var asst struct {
				SessionID string `json:"session_id"`
				Message   struct {
					Content []struct {
						Type     string `json:"type"`
						Text     string `json:"text"`
						Thinking string `json:"thinking"`
						ID       string `json:"id"`
						Name     string `json:"name"`
						Input    any    `json:"input"`
					} `json:"content"`
				} `json:"message"`
			}
			if json.Unmarshal([]byte(line), &asst) != nil {
				continue
			}
			if asst.SessionID != "" && cfg.OnExternalSession != nil {
				cfg.OnExternalSession(r.DisplayProvider(), asst.SessionID)
			}
			for _, block := range asst.Message.Content {
				switch block.Type {
				case "thinking":
					if !sawDelta && block.Thinking != "" {
						acc.AddThinkingDelta(block.Thinking)
					}
				case "text":
					if !sawDelta && block.Text != "" {
						acc.AddTextDelta(block.Text)
					}
				case "tool_use":
					acc.StartTool(block.ID, block.Name, marshalArgsJSON(block.Input))
				}
			}
			sawDelta = false

		case "user":
			var usr struct {
				Message struct {
					Content []struct {
						Type      string `json:"type"`
						ToolUseID string `json:"tool_use_id"`
						Content   any    `json:"content"`
						IsError   bool   `json:"is_error"`
					} `json:"content"`
				} `json:"message"`
			}
			if json.Unmarshal([]byte(line), &usr) != nil {
				continue
			}
			for _, block := range usr.Message.Content {
				if block.Type == "tool_result" {
					acc.EndTool(block.ToolUseID, "", extractClaudeToolContent(block.Content), block.IsError)
				}
			}

		case "control_request":
			var req struct {
				RequestID string `json:"request_id"`
				Subtype   string `json:"subtype"`
				Request   struct {
					Subtype  string `json:"subtype"`
					ToolName string `json:"tool_name"`
					Input    any    `json:"input"`
				} `json:"request"`
				Data struct {
					Tool  string `json:"tool"`
					Input any    `json:"input"`
				} `json:"data"`
			}
			if json.Unmarshal([]byte(line), &req) == nil && req.RequestID != "" {
				toolName := req.Request.ToolName
				input := req.Request.Input
				if toolName == "" {
					toolName = req.Data.Tool
					input = req.Data.Input
				}
				allowed, reason := r.evaluatePermission(ctx, cfg, "claude", toolName, input)
				behavior := "allow"
				if !allowed {
					behavior = "deny"
				}
				_ = writeJSON(map[string]any{
					"type":       "control_response",
					"request_id": req.RequestID,
					"decision":   behavior,
					"response": map[string]any{
						"subtype":    "success",
						"request_id": req.RequestID,
						"response": map[string]any{
							"behavior": behavior,
							"message":  reason,
						},
					},
				})
			}

		case "result":
			var res struct {
				SessionID string `json:"session_id"`
				IsError   bool   `json:"is_error"`
				Result    string `json:"result"`
				Usage     *struct {
					InputTokens              int `json:"input_tokens"`
					OutputTokens             int `json:"output_tokens"`
					CacheReadInputTokens     int `json:"cache_read_input_tokens"`
					CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
				} `json:"usage"`
			}
			if json.Unmarshal([]byte(line), &res) == nil {
				if res.SessionID != "" && cfg.OnExternalSession != nil {
					cfg.OnExternalSession(r.DisplayProvider(), res.SessionID)
				}
				if res.Usage != nil {
					acc.AddUsage(provider.Usage{
						PromptTokens:      res.Usage.InputTokens + res.Usage.CacheCreationInputTokens + res.Usage.CacheReadInputTokens,
						CompletionTokens:  res.Usage.OutputTokens,
						CachedInputTokens: res.Usage.CacheReadInputTokens,
					})
				}
				if res.IsError {
					msg := strings.TrimSpace(res.Result)
					if msg == "" {
						msg = "claude code reported an error"
					}
					resultErr = fmt.Errorf("%s", msg)
				} else if res.Result != "" && acc.curText.Len() == 0 && acc.lastFinal.Role == "" {
					acc.AddTextDelta(res.Result)
				}
			}
			// Turn complete; close stdin so claude exits cleanly.
			writeMu.Lock()
			_ = stdin.Close()
			writeMu.Unlock()
		}
	}

	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return acc.Finish(provider.StopAborted, ctx.Err())
	}
	if resultErr != nil {
		return acc.Finish(provider.StopError, resultErr)
	}
	if waitErr != nil && acc.lastFinal.Role == "" && acc.curText.Len() == 0 {
		return acc.Finish(provider.StopError, fmt.Errorf("claude exited: %w", waitErr))
	}
	return acc.Finish(provider.StopNormal, nil)
}

func extractClaudeToolContent(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case []any:
		var parts []string
		for _, item := range val {
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
	return marshalArgsJSON(v)
}
