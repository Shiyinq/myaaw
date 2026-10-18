package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"myaaw/internal/agent/tools"
	"myaaw/internal/provider"
)

// ToolCallInput is the normalized form of a provider tool call.
type ToolCallInput struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// ToolResult is the structured result of a tool call.
type ToolResult struct {
	Content           []provider.ContentItem
	Details           any
	StructuredContent any
	IsError           bool
	Terminate         bool
	Usage             provider.Usage
	DurationMs        int64
}

// Tool is the agent-facing tool contract. Existing tools are wrapped into
// this interface by buildTools.
type Tool interface {
	Name() string
	Schema() json.RawMessage
	ExecutionMode() tools.ExecutionMode
	Execute(ctx context.Context, call ToolCallInput, onUpdate func(ToolResult)) ToolResult
}

// legacyTool adapts an existing tools.ToolsFactory to the Tool interface.
type legacyTool struct {
	name     string
	factory  tools.ToolsFactory
	schema   json.RawMessage
	toolsCtx *tools.ToolsContext
	mode     tools.ExecutionMode
}

func (t *legacyTool) Name() string { return t.name }

func (t *legacyTool) Schema() json.RawMessage { return t.schema }

func (t *legacyTool) ExecutionMode() tools.ExecutionMode { return t.mode }

func (t *legacyTool) Execute(ctx context.Context, call ToolCallInput, onUpdate func(ToolResult)) (result ToolResult) {
	started := time.Now()
	defer func() {
		result.DurationMs = time.Since(started).Milliseconds()
		if recovered := recover(); recovered != nil {
			result = ToolResult{
				Content:    []provider.ContentItem{{Type: "text", Text: fmt.Sprintf("Error: tool %q panicked: %v", t.name, recovered)}},
				IsError:    true,
				DurationMs: result.DurationMs,
			}
		}
	}()

	var output string
	if contextTool, ok := t.factory.(tools.ContextTool); ok {
		output = contextTool.CallToolContext(ctx, string(call.Arguments), t.toolsCtx)
	} else {
		output = t.factory.CallTool(string(call.Arguments), t.toolsCtx)
	}

	result = ToolResult{
		Content:    []provider.ContentItem{{Type: "text", Text: output}},
		IsError:    looksLikeToolError(output),
		DurationMs: time.Since(started).Milliseconds(),
	}
	return result
}

// buildTools materializes the registry for one run.
func buildTools(excluded []string, toolsCtx *tools.ToolsContext) []Tool {
	factories := tools.GetFactories(excluded...)
	result := make([]Tool, 0, len(factories))
	for _, named := range factories {
		schema := json.RawMessage(named.Factory.ToolDefinition())
		mode := tools.ExecutionSequential
		if aware, ok := named.Factory.(tools.ExecutionModeAware); ok {
			mode = aware.ExecutionMode()
		}
		result = append(result, &legacyTool{
			name:     named.Name,
			factory:  named.Factory,
			schema:   schema,
			toolsCtx: toolsCtx,
			mode:     mode,
		})
	}
	return result
}

func toolSchemas(toolsList []Tool) []map[string]interface{} {
	schemas := make([]map[string]interface{}, 0, len(toolsList))
	for _, tool := range toolsList {
		var schema map[string]interface{}
		if err := json.Unmarshal(tool.Schema(), &schema); err != nil {
			continue
		}
		schemas = append(schemas, schema)
	}
	return schemas
}

func toolMap(toolsList []Tool) map[string]Tool {
	result := make(map[string]Tool, len(toolsList))
	for _, tool := range toolsList {
		result[tool.Name()] = tool
	}
	return result
}

// validateToolArguments performs lightweight JSON-schema validation covering
// required properties and primitive types. Full JSON-schema validation could
// be added later without changing the executor contract.
func validateToolArguments(schema json.RawMessage, args json.RawMessage) error {
	var definition struct {
		Function struct {
			Parameters struct {
				Properties map[string]struct {
					Type string `json:"type"`
				} `json:"properties"`
				Required []string `json:"required"`
			} `json:"parameters"`
		} `json:"function"`
	}
	if err := json.Unmarshal(schema, &definition); err != nil {
		return nil
	}
	params := definition.Function.Parameters
	if len(params.Properties) == 0 && len(params.Required) == 0 {
		return nil
	}

	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(args, &object); err != nil {
		return fmt.Errorf("arguments must be a JSON object: %w", err)
	}

	for _, required := range params.Required {
		if _, ok := object[required]; !ok {
			return fmt.Errorf("missing required argument %q", required)
		}
	}
	for key, raw := range object {
		property, ok := params.Properties[key]
		if !ok || property.Type == "" {
			continue
		}
		if !matchesJSONType(raw, property.Type) {
			return fmt.Errorf("argument %q must be of type %s", key, property.Type)
		}
	}
	return nil
}

func matchesJSONType(raw json.RawMessage, expected string) bool {
	trimmed := strings.TrimSpace(string(raw))
	switch expected {
	case "string":
		return strings.HasPrefix(trimmed, "\"")
	case "number":
		var v float64
		return json.Unmarshal(raw, &v) == nil
	case "integer":
		var v int64
		return json.Unmarshal(raw, &v) == nil
	case "boolean":
		return trimmed == "true" || trimmed == "false"
	case "object":
		return strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")
	case "array":
		return strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")
	case "null":
		return trimmed == "null"
	default:
		return true
	}
}

func errorToolResult(message string) ToolResult {
	return ToolResult{
		Content: []provider.ContentItem{{Type: "text", Text: message}},
		IsError: true,
	}
}

func looksLikeToolError(output string) bool {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(lower, "error") ||
		strings.HasPrefix(lower, "failed") ||
		strings.Contains(lower, "not available") ||
		strings.Contains(lower, "blocked for security")
}

// ToolResultText flattens text content into a single string.
func ToolResultText(result ToolResult) string {
	var parts []string
	for _, item := range result.Content {
		if item.Type == "text" && item.Text != "" {
			parts = append(parts, item.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func truncateToolResult(result *ToolResult, maxBytes int) {
	if maxBytes <= 0 {
		return
	}
	for i := range result.Content {
		if result.Content[i].Type != "text" {
			continue
		}
		text := result.Content[i].Text
		if len(text) <= maxBytes {
			continue
		}
		cut := maxBytes
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		result.Content[i].Text = text[:cut] + fmt.Sprintf(
			"\n\n... [Output truncated at %d bytes. Use the filesystem tool or a more specific command to read the rest.] ...",
			maxBytes,
		)
	}
}

type preparedToolCall struct {
	index  int
	call   ToolCallInput
	tool   Tool
	result *ToolResult
}

// executeToolCalls executes one assistant turn's tool calls. Results are
// returned in the same order as the calls. Execution is parallel unless any
// call targets a tool that requires sequential execution (the default for
// legacy tools).
func executeToolCalls(
	ctx context.Context,
	calls []ToolCallInput,
	registry map[string]Tool,
	cfg RunConfig,
	emit func(AgentEvent) error,
) ([]ToolResult, bool) {
	results := make([]ToolResult, len(calls))
	if len(calls) == 0 {
		return results, false
	}

	var emitMu sync.Mutex
	safeEmit := func(event AgentEvent) error {
		if emit == nil {
			return nil
		}
		emitMu.Lock()
		defer emitMu.Unlock()
		return emit(event)
	}

	prepared := make([]preparedToolCall, 0, len(calls))
	parallel := true
	for i, call := range calls {
		if ctx.Err() != nil {
			result := errorToolResult("Operation aborted")
			results[i] = result
			safeEmit(AgentEvent{Type: EventToolExecutionStart, ToolCallID: call.ID, ToolName: call.Name})
			safeEmit(AgentEvent{Type: EventToolExecutionEnd, ToolCallID: call.ID, ToolName: call.Name, ToolResult: &result})
			continue
		}

		tool, ok := registry[call.Name]
		if !ok {
			result := errorToolResult(fmt.Sprintf("Error: tool %q not available", call.Name))
			results[i] = result
			safeEmit(AgentEvent{Type: EventToolExecutionStart, ToolCallID: call.ID, ToolName: call.Name})
			safeEmit(AgentEvent{Type: EventToolExecutionEnd, ToolCallID: call.ID, ToolName: call.Name, ToolResult: &result})
			continue
		}
		if err := validateToolArguments(tool.Schema(), call.Arguments); err != nil {
			result := errorToolResult(fmt.Sprintf("Error: invalid arguments for %q: %v", call.Name, err))
			results[i] = result
			safeEmit(AgentEvent{Type: EventToolExecutionStart, ToolCallID: call.ID, ToolName: call.Name})
			safeEmit(AgentEvent{Type: EventToolExecutionEnd, ToolCallID: call.ID, ToolName: call.Name, ToolResult: &result})
			continue
		}
		if cfg.BeforeToolCall != nil {
			block := cfg.BeforeToolCall(ctx, call, call.Arguments)
			if block != nil && block.Block {
				reason := block.Reason
				if reason == "" {
					reason = fmt.Sprintf("Tool call %q was blocked by policy", call.Name)
				}
				result := errorToolResult("Error: " + reason)
				result.Terminate = block.Terminate
				results[i] = result
				safeEmit(AgentEvent{Type: EventToolExecutionStart, ToolCallID: call.ID, ToolName: call.Name})
				safeEmit(AgentEvent{Type: EventToolExecutionEnd, ToolCallID: call.ID, ToolName: call.Name, ToolResult: &result})
				continue
			}
		}

		if tool.ExecutionMode() == tools.ExecutionSequential {
			parallel = false
		}
		prepared = append(prepared, preparedToolCall{index: i, call: call, tool: tool})
	}

	if len(prepared) == 0 {
		return results, allTerminate(results)
	}

	run := func(p preparedToolCall) {
		result := runPreparedToolCall(ctx, p, cfg, safeEmit)
		results[p.index] = result
	}

	if !parallel {
		for _, p := range prepared {
			if ctx.Err() != nil {
				results[p.index] = errorToolResult("Operation aborted")
				continue
			}
			run(p)
		}
	} else {
		var wg sync.WaitGroup
		wg.Add(len(prepared))
		for _, p := range prepared {
			p := p
			go func() {
				defer wg.Done()
				run(p)
			}()
		}
		wg.Wait()
	}

	return results, allTerminate(results)
}

func runPreparedToolCall(
	parentCtx context.Context,
	prepared preparedToolCall,
	cfg RunConfig,
	emit func(AgentEvent) error,
) (result ToolResult) {
	started := time.Now()
	defer func() {
		if recovered := recover(); recovered != nil {
			result = errorToolResult(fmt.Sprintf("Error: tool %q panicked: %v", prepared.call.Name, recovered))
		}
		result.DurationMs = time.Since(started).Milliseconds()
		_ = emit(AgentEvent{
			Type:       EventToolExecutionEnd,
			ToolCallID: prepared.call.ID,
			ToolName:   prepared.call.Name,
			ToolResult: &result,
		})
	}()

	_ = emit(AgentEvent{
		Type:       EventToolExecutionStart,
		ToolCallID: prepared.call.ID,
		ToolName:   prepared.call.Name,
	})

	ctx := parentCtx
	cancel := func() {}
	if cfg.ToolTimeout > 0 {
		ctx, cancel = context.WithTimeout(parentCtx, cfg.ToolTimeout)
	}
	defer cancel()

	onUpdate := func(partial ToolResult) {
		_ = emit(AgentEvent{
			Type:       EventToolExecutionUpdate,
			ToolCallID: prepared.call.ID,
			ToolName:   prepared.call.Name,
			ToolResult: &partial,
		})
	}

	result = prepared.tool.Execute(ctx, prepared.call, onUpdate)
	if cfg.AfterToolCall != nil {
		if override := cfg.AfterToolCall(ctx, prepared.call, &result); override != nil {
			result = *override
		}
	}
	if ctx.Err() != nil && !result.IsError {
		result.IsError = true
		if result.Content == nil {
			result.Content = []provider.ContentItem{{Type: "text", Text: "Error: operation aborted"}}
		}
	}
	truncateToolResult(&result, cfg.MaxToolOutputBytes)
	return result
}

func allTerminate(results []ToolResult) bool {
	if len(results) == 0 {
		return false
	}
	for _, result := range results {
		if !result.Terminate {
			return false
		}
	}
	return true
}
