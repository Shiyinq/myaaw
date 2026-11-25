package runtime

import (
	"context"

	"myaaw/internal/agent"
	"myaaw/internal/config"
	"myaaw/internal/provider"
)

type nativeRuntime struct {
	agent.AgentProvider
	llm provider.LLMProvider
}

func newNativeRuntime(llm provider.LLMProvider) Runtime {
	return &nativeRuntime{
		AgentProvider: agent.NewAgent(llm),
		llm:           llm,
	}
}

func (n *nativeRuntime) Type() string            { return config.RuntimeTypeMyaaw }
func (n *nativeRuntime) Preset() string          { return "myaaw" }
func (n *nativeRuntime) DisplayProvider() string { return n.llm.ProviderName() }
func (n *nativeRuntime) DisplayModel() string    { return n.llm.DefaultModel("") }
func (n *nativeRuntime) WorkingDir() string      { return ResolveWorkingDir(config.DefaultRuntime()) }
func (n *nativeRuntime) Capabilities() config.RuntimeCapabilities {
	return config.CapabilitiesForRuntime(nil)
}
func (n *nativeRuntime) IsExternal() bool { return false }
func (n *nativeRuntime) Close() error     { return nil }

func (n *nativeRuntime) WithExcludedTools(tools ...string) agent.AgentProvider {
	return &nativeRuntime{
		AgentProvider: n.AgentProvider.WithExcludedTools(tools...),
		llm:           n.llm,
	}
}

// RunContext forwards to the wrapped native agent.
func (n *nativeRuntime) RunContext(ctx context.Context, modelName string, messages []provider.Message) (provider.Message, error) {
	return n.AgentProvider.RunContext(ctx, modelName, messages)
}
