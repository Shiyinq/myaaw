package agent

import (
	"context"
	"errors"
	"fmt"

	"myaaw/internal/config"
	"myaaw/internal/provider"
)

// ReactConfig holds the loop safety configuration.
type ReactConfig struct {
	MaxIterations    int
	WarningThreshold int
}

// AgentProvider is the public agent surface.
type AgentProvider interface {
	// Run executes the loop and returns the final assistant message.
	Run(modelName string, messages []provider.Message) (provider.Message, error)
	// RunContext is Run with cancellation.
	RunContext(ctx context.Context, modelName string, messages []provider.Message) (provider.Message, error)
	// RunStream is the legacy streaming API. New code should use RunEvents.
	RunStream(modelName string, messages []provider.Message, callback func(provider.Message) error) error
	// RunEvents executes the loop and emits lifecycle events.
	RunEvents(ctx context.Context, cfg RunConfig, history []provider.Message, onEvent func(AgentEvent) error) (RunResult, error)
	// WithExcludedTools returns a copy of the agent with tools excluded.
	WithExcludedTools(tools ...string) AgentProvider
}

type Agent struct {
	Provider      provider.LLMProvider
	Config        ReactConfig
	ExcludedTools []string
}

func NewAgent(prov provider.LLMProvider) AgentProvider {
	return &Agent{
		Provider: prov,
		Config: ReactConfig{
			MaxIterations:    config.AgentMaxIterations,
			WarningThreshold: config.AgentWarningIterations,
		},
	}
}

func (a *Agent) WithExcludedTools(excluded ...string) AgentProvider {
	clone := *a
	clone.ExcludedTools = append([]string(nil), excluded...)
	return &clone
}

func (a *Agent) Run(modelName string, messages []provider.Message) (provider.Message, error) {
	return a.RunContext(context.Background(), modelName, messages)
}

func (a *Agent) RunContext(ctx context.Context, modelName string, messages []provider.Message) (provider.Message, error) {
	cfg := DefaultRunConfig(modelName)
	cfg.ExcludedTools = a.ExcludedTools

	result, err := a.RunEvents(ctx, cfg, messages, nil)
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

// RunStream adapts the event stream to the legacy callback API. Prefer
// RunEvents for new code: it exposes an explicit final message and aggregated
// usage, avoiding intermediate-text accumulation in callers.
func (a *Agent) RunStream(modelName string, messages []provider.Message, callback func(provider.Message) error) error {
	cfg := DefaultRunConfig(modelName)
	cfg.ExcludedTools = a.ExcludedTools

	sawText := false
	sawThought := false
	result, err := a.RunEvents(context.Background(), cfg, messages, func(event AgentEvent) error {
		switch event.Type {
		case EventMessageUpdate:
			if event.Delta != "" {
				sawText = true
				if err := callback(provider.Message{Role: "assistant", Content: event.Delta}); err != nil {
					return err
				}
			}
			if event.ThinkingDelta != "" {
				sawThought = true
				if err := callback(provider.Message{Role: "assistant", Thought: event.ThinkingDelta}); err != nil {
					return err
				}
			}
		case EventToolExecutionStart:
			if err := callback(provider.Message{
				Role: "assistant",
				ToolCalls: []provider.ToolCall{{
					ID:   event.ToolCallID,
					Type: "function",
					Function: provider.FunctionCall{
						Name: event.ToolName,
					},
				}},
			}); err != nil {
				return err
			}
		case EventTraceUpdate:
			if len(event.Trace) > 0 {
				if err := callback(provider.Message{Role: "assistant", Trace: event.Trace}); err != nil {
					return err
				}
			}
		case EventUsage:
			if event.Usage.TotalTokens > 0 {
				if err := callback(provider.Message{Usage: event.Usage}); err != nil {
					return err
				}
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
	if !sawThought && result.Final.Thought != "" {
		if err := callback(provider.Message{Role: "assistant", Thought: result.Final.Thought}); err != nil {
			return err
		}
	}

	if result.StopReason == provider.StopError || result.StopReason == provider.StopAborted {
		if result.Err != nil {
			return result.Err
		}
		if result.Final.ErrorMessage != "" {
			return errors.New(result.Final.ErrorMessage)
		}
		return fmt.Errorf("agent stopped: %s", result.StopReason)
	}
	return nil
}
