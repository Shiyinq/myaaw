package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"myaaw/internal/agent/tools"
	"myaaw/internal/config"
	"myaaw/internal/provider"
)

// TurnDecision lets a FinishTurn hook stop or continue the loop.
type TurnDecision string

const (
	TurnContinue TurnDecision = "continue"
	TurnEnd      TurnDecision = "end"
)

// ToolBlock is returned by BeforeToolCall to prevent a tool from executing.
type ToolBlock struct {
	Block     bool
	Reason    string
	Terminate bool
}

// TurnContext is passed to FinishTurn after a turn's tool results have been
// appended to the transcript.
type TurnContext struct {
	Message     provider.Message
	ToolResults []provider.Message
	Transcript  []provider.Message
	NewMessages []provider.Message
}

// RunConfig controls one agent run. Zero values fall back to the agent's
// defaults and global config.
type RunConfig struct {
	Model            string
	MaxIterations    int
	WarningThreshold int

	ExcludedTools []string
	ToolsContext  *tools.ToolsContext

	// RunTimeout is an optional overall deadline for the whole run.
	RunTimeout time.Duration
	// ToolTimeout is an optional per-tool deadline. Zero disables the outer
	// deadline (individual tools may still impose their own).
	ToolTimeout time.Duration
	// MaxToolOutputBytes truncates tool output before it enters the transcript.
	// Zero uses the configured default.
	MaxToolOutputBytes int
	// ContextMaxTokens enables transcript compaction before each request.
	// Zero uses the configured default.
	ContextMaxTokens int
	// RepeatToolCallLimit warns the model after the same tool+arguments pair
	// has been executed this many times. Zero uses the configured default.
	RepeatToolCallLimit int

	// TransformContext runs immediately before compaction. Use it to inject
	// or prune context at the transcript level.
	TransformContext func(ctx context.Context, messages []provider.Message) []provider.Message
	// BeforeToolCall may block a tool call (approval policy).
	BeforeToolCall func(ctx context.Context, call ToolCallInput, args json.RawMessage) *ToolBlock
	// AfterToolCall may override or redact a tool result.
	AfterToolCall func(ctx context.Context, call ToolCallInput, result *ToolResult) *ToolResult
	// FinishTurn may end the run after a completed turn.
	FinishTurn func(ctx context.Context, turn TurnContext) TurnDecision
}

// DefaultRunConfig builds a run config from global configuration.
func DefaultRunConfig(model string) RunConfig {
	cfg := RunConfig{
		Model:               model,
		MaxIterations:       config.AgentMaxIterations,
		WarningThreshold:    config.AgentWarningIterations,
		MaxToolOutputBytes:  config.AgentMaxToolOutputBytes,
		ContextMaxTokens:    config.AgentContextMaxTokens,
		RepeatToolCallLimit: config.AgentRepeatToolCallLimit,
		RunTimeout:          time.Duration(config.AgentRunTimeoutSeconds) * time.Second,
		ToolTimeout:         time.Duration(config.AgentToolTimeoutSeconds) * time.Second,
	}

	// Configuration-based tool policy. Callers can override this with their own
	// BeforeToolCall hook (for example, an interactive approval flow).
	if len(config.AgentBlockedTools) > 0 {
		blocked := append([]string(nil), config.AgentBlockedTools...)
		cfg.BeforeToolCall = func(_ context.Context, call ToolCallInput, _ json.RawMessage) *ToolBlock {
			for _, name := range blocked {
				if name == call.Name {
					return &ToolBlock{
						Block:  true,
						Reason: fmt.Sprintf("tool %q is disabled by policy (AGENT_BLOCKED_TOOLS / bot.blocked_tools)", call.Name),
					}
				}
			}
			return nil
		}
	}

	return cfg
}

func (a *Agent) applyDefaults(cfg RunConfig) RunConfig {
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = a.Config.MaxIterations
	}
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = 100
	}
	if cfg.WarningThreshold <= 0 {
		cfg.WarningThreshold = a.Config.WarningThreshold
	}
	if cfg.WarningThreshold <= 0 || cfg.WarningThreshold >= cfg.MaxIterations {
		cfg.WarningThreshold = cfg.MaxIterations - 10
		if cfg.WarningThreshold < 1 {
			cfg.WarningThreshold = 1
		}
	}
	if cfg.ExcludedTools == nil {
		cfg.ExcludedTools = a.ExcludedTools
	}
	if cfg.MaxToolOutputBytes <= 0 {
		cfg.MaxToolOutputBytes = config.AgentMaxToolOutputBytes
	}
	if cfg.ContextMaxTokens <= 0 {
		cfg.ContextMaxTokens = config.AgentContextMaxTokens
	}
	if cfg.RepeatToolCallLimit == 0 {
		cfg.RepeatToolCallLimit = config.AgentRepeatToolCallLimit
	}
	if cfg.RunTimeout == 0 {
		cfg.RunTimeout = time.Duration(config.AgentRunTimeoutSeconds) * time.Second
	}
	return cfg
}
