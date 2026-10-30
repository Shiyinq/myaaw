# Myaaw Documentation

Start with the [main README](../README.md) for installation and a feature overview. This directory is grouped by topic: agents, CLI, API, then generated references.

## Agents

| Document | Description |
| :--- | :--- |
| [Multi-Agent Guide](agents/multi-agent.md) | Registry, agent commands, per-agent config, channels, ports, and servers. |
| [Agent Loop](agents/agent-loop.md) | `RunEvents` engine, transcript v2, compaction, tools, and retries. |
| [Agent Skills](agents/skills.md) | `SKILL.md` skills, discovery, prompt injection, and data isolation. |
| [MCP & Custom Tools](agents/mcp-and-custom-tools.md) | MCP servers, custom CLI tools, `tools.json`, and hot reloading. |
| [Observability](agents/observability.md) | Traces, token usage, model cost, retention, and OTLP export. |
| [Tool Approvals](agents/approvals.md) | Approval policy, grants, channel/API flows, and CLI/control-plane management. |
| [Computer Use](agents/computer-use.md) | macOS desktop automation: screenshots, mouse/keyboard, permissions, and safety. |

### Agent protocol (A2A)

| Document | Description |
| :--- | :--- |
| [A2A Guide](agents/protocol/a2a.md) | Enable the endpoint, call another agent, and manage contacts. |
| [A2A Protocol](agents/protocol/a2a-protocol.md) | Data model, task lifecycle, methods, streaming, and storage. |
| [A2A Interop](agents/protocol/a2a-interop.md) | Local and cross-server connections, plus non-Myaaw peers. |
| [A2A Spec Compliance](agents/protocol/a2a-spec-compliance.md) | Validation against the A2A v1.0 specification. |

## CLI

| Document | Description |
| :--- | :--- |
| [CLI Reference](cli/README.md) | Global flags, command groups, and conventions. |
| [Onboarding](cli/onboard.md) | Agent name, provider, channels, webhook, heartbeat; interactive or headless. |
| [Agent](cli/agent.md) | `agent list/create/use/delete`, name rules, and what create writes. |
| [Chat](cli/chat.md) | Interactive TUI, one-shot messages, sessions, models, and file attachments. |
| [Voice](cli/voice.md) | Real-time voice and the classic STT/TTS pipeline. |
| [Service Management](cli/service.md) | Server daemon lifecycle, status, logs, and ports. |
| [API](cli/api.md) | `api run/start/stop/restart/status`, `--terminal`, daemon name, and ports. |
| [Cron](cli/cron.md) | Scheduled jobs: cron expressions, intervals, one-time runs, and history. |
| [Backup & Restore](cli/backup.md) | Per-agent archives, database snapshots, and verified restore. |
| [Providers](cli/provider.md) | LLM provider integrations and models. |
| [Channels](cli/channel.md) | Telegram and Discord setup. |
| [Webhook](cli/webhook.md) | Telegram webhook management. |
| [Logs](cli/logs.md) | Interactive log viewer and filtering. |
| [Config](cli/config.md) | Validate and inspect configuration. |
| [Utilities](cli/utilities.md) | `status`, `doctor`, `codesign`, `computer`, `version`, `update`, and shell completion. |

## API

| Document | Description |
| :--- | :--- |
| [API](api/api.md) | Per-agent HTTP API for chat and SSE streaming. |
| [Control Plane API](api/control-plane.md) | Unified multi-agent API used by web, desktop, and mobile clients. |
| [Web Terminal](api/terminal.md) | Browser shell into the API host: enablement, lifecycle, and security. |

## Generated references

- [Control plane Swagger](controlplane/)
- [Server Swagger](swagger/)
