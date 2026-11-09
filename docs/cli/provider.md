# Providers

LLM provider integrations are stored per agent in `~/.myaaw/agents/<agent>/config/config.json`.

For protocol, authentication, endpoint, capability, and troubleshooting details, see the [Provider Integration Guide](../provider/README.md): [Gemini](../provider/gemini.md), [OpenAI-compatible](../provider/openai-compatible.md), and [ChatGPT Subscription](../provider/chatgpt-subscription.md).

## Commands

```bash
myaaw provider create
myaaw provider list
myaaw provider default [name]
myaaw provider delete [name]
myaaw provider set-model [name]
myaaw provider connect-chatgpt [name] [--model MODEL] [--default]
myaaw provider status-chatgpt [name]
myaaw provider disconnect-chatgpt [name]
```

Use `--agent` to manage another agent:

```bash
myaaw --agent agent-b provider list
```

## `provider create`

Interactive setup:

1. Provider type: **OpenAI / OpenAI-compatible**, **Gemini**, or **ChatGPT Subscription**.
2. Custom name, for example `deepseek`, `local-ollama`, or `gemini-personal`.
3. Base URL (leave empty for the default endpoint: `https://api.openai.com` or `https://generativelanguage.googleapis.com`).
4. API key (input is hidden).
5. Myaaw fetches the available models and asks for the default model.

The new provider is written to `config/config.json`; the first provider becomes the default automatically.

For **ChatGPT Subscription**, the wizard skips the base URL/API key and opens the local system browser for OAuth, then discovers models. See [ChatGPT Subscription](chatgpt-subscription.md) for headless `--credential-id` attachment, failed-registration recovery, local-browser requirements, credential privacy, quotas, and reference-only disconnect semantics.

## `provider list`

Prints configured provider names, their type, default model, and which one is the default.

## `provider default [name]`

Sets the active provider. Without a name, an arrow-key picker is shown.

```bash
myaaw provider default deepseek
```

## `provider delete [name]`

Deletes a provider. Without a name, a picker is shown. Deleting the default selects another saved provider when one exists, or clears the default otherwise. This does not revoke upstream API keys or delete shared ChatGPT OAuth credentials.

## `provider set-model [name]`

Changes the default model for an existing provider. Without a name, a picker is shown, then a model picker.

## Examples

```bash
# Create an OpenAI-compatible provider for a local endpoint
myaaw provider create
# type: openai, name: local-ollama, base URL: http://localhost:11434
# Myaaw appends /v1/chat/completions and /v1/models; do not include /v1 here.

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

- [Provider Integration Guide](../provider/README.md)
- [Chat](chat.md)
- [Voice](voice.md)
- [Config](config.md)
- [Multi-Agent Guide](../agents/multi-agent.md)
