# Providers

LLM provider integrations are stored per agent in `~/.myaaw/<agent>/config/config.json`.

## Commands

```bash
myaaw provider create
myaaw provider list
myaaw provider default [name]
myaaw provider delete [name]
myaaw provider set-model [name]
```

Use `--agent` to manage another agent:

```bash
myaaw --agent agent-b provider list
```

## `provider create`

Interactive setup:

1. Provider type: **OpenAI / OpenAI-compatible** or **Gemini**.
2. Custom name, for example `deepseek`, `local-ollama`, or `gemini-personal`.
3. Base URL (leave empty for the default endpoint).
4. API key (input is hidden).
5. Myaaw fetches the available models and asks for the default model.

The new provider is written to `config/config.json` but does not become the default automatically.

## `provider list`

Prints configured providers, their type, base URL, default model, and which one is the default.

## `provider default [name]`

Sets the active provider. Without a name, an arrow-key picker is shown.

```bash
myaaw provider default deepseek
```

## `provider delete [name]`

Deletes a provider. Without a name, a picker is shown. The default provider cannot be removed without switching first.

## `provider set-model [name]`

Changes the default model for an existing provider. Without a name, a picker is shown, then a model picker.

## Examples

```bash
# Create an OpenAI-compatible provider for a local endpoint
myaaw provider create
# type: openai, name: local-ollama, base URL: http://localhost:11434/v1

# List and switch
myaaw provider list
myaaw provider default local-ollama

# Change the model of a provider
myaaw provider set-model local-ollama
```

## Notes

- Provider and model values are per agent; `agent-b` and `agent-c` can use different providers.
- `myaaw chat` uses the default provider unless you switch with `/models` for that session.
- `myaaw voice` requires the active provider to be Gemini (Multimodal Live). `voice-classic` works with any provider.
- API keys are masked by `myaaw config dump`.
- The server watches `config.json`; most changes apply without a restart. If a running server does not pick up a change, restart it with [Service Management](service.md).

## Related

- [Chat](chat.md)
- [Voice](voice.md)
- [Config](config.md)
- [Multi-Agent Guide](../agents/multi-agent.md)
