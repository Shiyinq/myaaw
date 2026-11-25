package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"myaaw/internal/agent"
	"myaaw/internal/config"
	"myaaw/internal/provider"
)

// externalBase holds metadata and convenience wrappers shared by every external
// CLI runtime driver.
type externalBase struct {
	cfg           config.RuntimeConfig
	excludedTools []string
	bridge        *TurnBridgeServer
}

func newExternalBase(cfg config.RuntimeConfig) externalBase {
	bridge, _ := NewTurnBridgeServer()
	return externalBase{
		cfg:    cfg,
		bridge: bridge,
	}
}

func (b *externalBase) Type() string            { return b.cfg.Type }
func (b *externalBase) Preset() string          { return b.cfg.Preset }
func (b *externalBase) DisplayProvider() string { return runtimeDisplayProvider(b.cfg) }
func (b *externalBase) DisplayModel() string    { return runtimeDisplayModel(b.cfg) }
func (b *externalBase) WorkingDir() string      { return ResolveWorkingDir(b.cfg) }
func (b *externalBase) Capabilities() config.RuntimeCapabilities {
	return config.CapabilitiesForRuntime(&b.cfg)
}
func (b *externalBase) IsExternal() bool { return true }

func (b *externalBase) closeBridge() {
	if b.bridge != nil {
		_ = b.bridge.Close()
	}
}

func (b *externalBase) bridgeURL() string {
	if b.bridge == nil {
		return ""
	}
	return b.bridge.URL()
}

func (b *externalBase) bridgeToken() string {
	if b.bridge == nil {
		return ""
	}
	return b.bridge.Token()
}

func (b *externalBase) activateBridge(cfg agent.RunConfig, acc *turnAccumulator) func() {
	if b.bridge == nil {
		return func() {}
	}
	return b.bridge.SetActive(&TurnSideEffects{
		ToolsContext:    cfg.ToolsContext,
		OnExtraMessages: acc.AddExtraMessages,
		OnAttachments:   acc.AddAttachments,
	})
}

// evaluatePermission routes an external agent's tool-permission request through
// runtime.permission_mode and Myaaw's interactive approvals.Gate.
func (b *externalBase) evaluatePermission(ctx context.Context, cfg agent.RunConfig, prefix, toolName string, rawArgs any) (bool, string) {
	mode := b.cfg.PermissionMode
	if mode == "" {
		mode = config.PermissionModeBridge
	}
	switch mode {
	case config.PermissionModeAuto:
		return true, "auto-approved by runtime permission_mode"
	case config.PermissionModeReadOnly:
		lower := strings.ToLower(strings.TrimSpace(toolName))
		if strings.Contains(lower, "read") || strings.Contains(lower, "list") || strings.Contains(lower, "search") || strings.Contains(lower, "grep") || strings.Contains(lower, "glob") || strings.Contains(lower, "view") {
			return true, "read-only tool allowed"
		}
		return false, "blocked by runtime permission_mode=read_only"
	}

	// Default "bridge" mode: forward to Myaaw's approvals.Gate if attached.
	if cfg.ApprovalGate == nil {
		return true, ""
	}
	key := strings.TrimSpace(toolName)
	if key == "" {
		key = "tool"
	}
	if prefix != "" && !strings.Contains(key, ":") {
		key = prefix + ":" + key
	}
	argsStr := marshalArgsJSON(rawArgs)
	outcome := cfg.ApprovalGate.Check(ctx, key, json.RawMessage(argsStr))
	return outcome.Allowed, outcome.Reason
}

// runContextAdapter implements Run/RunContext on top of RunEvents.
func runContextAdapter(ctx context.Context, p agent.AgentProvider, modelName string, messages []provider.Message) (provider.Message, error) {
	cfg := agent.DefaultRunConfig(modelName)
	result, err := p.RunEvents(ctx, cfg, messages, nil)
	if err != nil {
		return result.Final, err
	}
	if result.StopReason == provider.StopError || result.StopReason == provider.StopAborted {
		if result.Err != nil {
			return result.Final, result.Err
		}
		if result.Final.ErrorMessage != "" {
			return result.Final, errors.New(result.Final.ErrorMessage)
		}
		return result.Final, fmt.Errorf("agent stopped: %s", result.StopReason)
	}
	return result.Final, nil
}

// runStreamAdapter implements legacy RunStream on top of RunEvents.
func runStreamAdapter(p agent.AgentProvider, modelName string, messages []provider.Message, callback func(provider.Message) error) error {
	cfg := agent.DefaultRunConfig(modelName)
	sawText := false
	result, err := p.RunEvents(context.Background(), cfg, messages, func(ev agent.AgentEvent) error {
		switch ev.Type {
		case agent.EventMessageUpdate:
			if ev.Delta != "" {
				sawText = true
				return callback(provider.Message{Role: "assistant", Content: ev.Delta})
			}
			if ev.ThinkingDelta != "" {
				return callback(provider.Message{Role: "assistant", Thought: ev.ThinkingDelta})
			}
		case agent.EventToolExecutionStart:
			return callback(provider.Message{
				Role: "assistant",
				ToolCalls: []provider.ToolCall{{
					ID:       ev.ToolCallID,
					Type:     "function",
					Function: provider.FunctionCall{Name: ev.ToolName},
				}},
			})
		case agent.EventTraceUpdate:
			if len(ev.Trace) > 0 {
				return callback(provider.Message{Role: "assistant", Trace: ev.Trace})
			}
		case agent.EventUsage:
			if ev.Usage.TotalTokens > 0 {
				return callback(provider.Message{Usage: ev.Usage})
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !sawText {
		if text := provider.MessageText(result.Final); text != "" {
			if err := callback(provider.Message{Role: "assistant", Content: text}); err != nil {
				return err
			}
		}
	}
	return nil
}
