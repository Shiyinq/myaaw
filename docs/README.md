# Myaaw Documentation

Start with the [main README](../README.md) for installation and a feature overview. This directory is grouped by topic: agents, provider integrations, CLI, API, channels, then generated references.

## Agents

| Document | Description |
| :--- | :--- |
| [Multi-Agent Guide](agents/multi-agent.md) | Registry, agent commands, per-agent config, channels, ports, and servers. |
| [Agent Loop](agents/agent-loop.md) | `RunEvents` engine, transcript v2, compaction, tools, and retries. |
| [Agent Context](agents/context.md) | What the agent knows per turn, the `<userinfo>`/`<attached>` blocks, and prompt-cache safety. |
| [Agent Attachments](agents/attachments.md) | Delivering agent-produced files: the `@attach` contract, per-surface previews, validation and security, persistence, and rendering. |
| [Thinking Levels](agents/thinking.md) | Reasoning effort per provider: catalog data, resolver rules, and wire shapes for Gemini, OpenAI, DeepSeek, and ChatGPT. |
| [Cron & Heartbeat](agents/cron-heartbeat.md) | Scheduled jobs, heartbeat ticks, delivery targets, session pinning, and multi-agent isolation. |
| [Agent Skills](agents/skills.md) | `SKILL.md` skills, discovery, prompt injection, and data isolation. |
| [MCP & Custom Tools](agents/mcp-and-custom-tools.md) | MCP servers, custom CLI tools, `tools.json`, and hot reloading. |
| [Observability](agents/observability.md) | Traces, token and context usage, model cost, retention, and OTLP export. |
| [Tool Approvals](agents/approvals.md) | Approval policy, grants, channel/API flows, and CLI/control-plane management. |
| [Computer Use](agents/computer-use.md) | macOS desktop automation: screenshots, mouse/keyboard, permissions, and safety. |

### Agent protocol (A2A)

| Document | Description |
| :--- | :--- |
| [A2A Guide](agents/protocol/a2a.md) | Enable the endpoint, call another agent, and manage contacts. |
| [A2A Protocol](agents/protocol/a2a-protocol.md) | Data model, task lifecycle, methods, streaming, and storage. |
| [A2A Interop](agents/protocol/a2a-interop.md) | Local and cross-server connections, plus non-Myaaw peers. |
| [A2A Spec Compliance](agents/protocol/a2a-spec-compliance.md) | Validation against the A2A v1.0 specification. |

## Provider integrations

| Document | Description |
| :--- | :--- |
| [Provider Integration Guide](provider/README.md) | Comparison, per-agent configuration, authentication/billing boundaries, model discovery, and security. |
| [Gemini](provider/gemini.md) | Native REST chat, API keys, model filters, tools/thought signatures, images, and separate Live/STT/TTS configuration. |
| [OpenAI-compatible](provider/openai-compatible.md) | OpenAI/DeepSeek/gateways/local servers, base URLs, Chat Completions, model discovery, and compatibility limits. |
| [ChatGPT Subscription](provider/chatgpt-subscription.md) | OAuth lifecycle, credential rotation/storage, setup/API contracts, Responses streaming, limitations, quotas, and recovery. |

## CLI

| Document | Description |
| :--- | :--- |
| [CLI Reference](cli/README.md) | Global flags, command groups, and conventions. |
| [Onboarding](cli/onboard.md) | Agent name, provider, channels, webhook, heartbeat; interactive or headless. |
| [Agent](cli/agent.md) | `agent list/create/use/delete`, name rules, and what create writes. |
| [Chat](cli/chat.md) | Interactive TUI, one-shot messages, sessions, models, and file attachments. |
| [Voice](cli/voice.md) | Real-time voice and the classic STT/TTS pipeline. |
| [Service Management](cli/service.md) | Server daemon lifecycle, status, logs, and ports. |
| [API](cli/api.md) | `api run/start/stop/restart/status`, `--host`, `--terminal`, daemon name, and ports. |
| [WebUI](cli/webui.md) | `webui install/run/start/stop/update/uninstall/open`, port, and control-plane URL. |
| [Cron](cli/cron.md) | Scheduled jobs: cron expressions, intervals, one-time runs, and history. |
| [Backup & Restore](cli/backup.md) | Per-agent archives, database snapshots, and verified restore. |
| [Providers](cli/provider.md) | LLM provider integrations and models. |
| [ChatGPT Subscription](cli/chatgpt-subscription.md) | Local browser OAuth, headless credential references, reconnect/disconnect, privacy, and plan quotas. |
| [Channels](cli/channel.md) | Telegram and Discord setup. |
| [Webhook](cli/webhook.md) | Telegram webhook management. |
| [Logs](cli/logs.md) | Interactive log viewer and filtering. |
| [Config](cli/config.md) | Validate and inspect configuration. |
| [Utilities](cli/utilities.md) | `status`, `doctor`, `codesign`, `computer`, `version`, `update`, and shell completion. |

## API

| Document | Description |
| :--- | :--- |
| [API](api/api.md) | Per-agent HTTP API for chat, conversation events, and SSE streaming. |
| [Control Plane API](api/control-plane.md) | Unified multi-agent API used by web, desktop, and mobile clients (`--host` defaults to loopback). |
| [Web Terminal](api/terminal.md) | Browser shell into the API host: enablement, lifecycle, and security. |

## WebUI

| Document | Description |
| :--- | :--- |
| [Security](webui/security.md) | Threat model, loopback, transport, tokens, CORS, terminal, and tunnel vs SSH. |
| [Remote Connection](webui/remote-connection.md) | Server URL, Tailscale/Cloudflare/ngrok/SSH, and the direct (Option A) connection. |

## Channels

| Document | Description |
| :--- | :--- |
| [Channels](channels/README.md) | How each conversation channel handles incoming messages. |
| [Telegram](channels/telegram.md) | Message flow, group/topic rules, sessions, implemented vs notes. |
| [Discord](channels/discord.md) | Message flow, per-channel sessions, intents, permissions, implemented vs notes. |

## Generated references

- [Control plane Swagger](controlplane/)
- [Server Swagger](swagger/)
