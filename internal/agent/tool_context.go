package agent

import "context"

type toolCallContextKey struct{}

// ToolCallIDFromContext returns the identifier of the tool call currently
// executing, or "". Tools that start nested work (a subagent) use it to attach
// their trace to the call that spawned them.
func ToolCallIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(toolCallContextKey{}).(string)
	return id
}

func withToolCallID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, toolCallContextKey{}, id)
}
