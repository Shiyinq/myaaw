# CLI Reference

`myaaw` is a single binary that manages agents, chats, channels, scheduled jobs, backups, and servers. Run `myaaw <command> --help` for the exact flags of any command.

## Global flags

| Flag | Description |
| :--- | :--- |
| `--agent <name>` | Target a specific agent. Without it, the registry default is used. |
| `-v`, `--verbose` | Enable verbose logging. |
| `-h`, `--help` | Show help for a command. |

The `--agent` flag is persistent, so these are equivalent:

```bash
myaaw --agent agent-b backup
myaaw backup --agent agent-b
```

Agent names must match `[A-Za-z0-9][A-Za-z0-9_-]*` and be at most 64 characters. See [Agent names](../agents/multi-agent.md#2-agent-names).

## Commands by group

### Getting started

| Command | Document |
| :--- | :--- |
| `onboard` | [Onboarding](onboard.md) |

### Chat & voice

| Command | Description | Document |
| :--- | :--- | :--- |
| `chat` | Interactive TUI or one-shot message | [Chat](chat.md) |
| `voice` | Real-time voice with Gemini Live | [Voice](voice.md) |
| `voice-classic` | STT → agent → TTS pipeline | [Voice](voice.md) |

### Agents & automation

| Command | Description | Document |
| :--- | :--- | :--- |
| `agent list` | List initialized agents | [Agent Command](agent.md#agent-list) |
| `agent create <name>` | Create an agent and run the onboarding wizard | [Agent Command](agent.md#agent-create-name) |
| `agent use [name]` | Set the default agent | [Agent Command](agent.md#agent-use-name) |
| `agent delete [name]` | Stop the daemon and delete all data | [Agent Command](agent.md#agent-delete-name---yes) |
| `cron list` | List scheduled jobs | [Cron](cron.md) |
| `cron add` | Create a cron, interval, or one-time job | [Cron](cron.md) |
| `cron remove <id>` | Delete a job | [Cron](cron.md) |
| `cron run <id>` | Run a job now | [Cron](cron.md) |
| `cron history [id]` | Show execution history | [Cron](cron.md) |

`agent create` only scaffolds the directory. Configure the new agent with `myaaw --agent <name> onboard`, or run `myaaw onboard` without a name to create and configure in one flow.

### Channels & integrations

| Command | Description | Document |
| :--- | :--- | :--- |
| `provider` | Create, list, default, delete, and set the model of LLM providers | [Providers](provider.md) |
| `channel` | Create, list, edit, and delete Telegram/Discord channels | [Channels](channel.md) |
| `webhook` | Telegram webhook management | [Webhook](webhook.md) |

### Monitoring & diagnostics

| Command | Description | Document |
| :--- | :--- | :--- |
| `status` | Health summary for the active agent | [Utilities](utilities.md#myaaw-status) |
| `logs` | Interactive log viewer or filtered stream | [Logs](logs.md) |
| `observability` | Inspect traces, token usage, and cost | [Observability](../agents/observability.md#6-cli) |
| `doctor` | Check environment readiness (permissions, dependencies) | [Utilities](utilities.md#myaaw-doctor) |

### Settings & desktop

| Command | Description | Document |
| :--- | :--- | :--- |
| `config check` | Validate `.env` and `config.json` | [Config](config.md) |
| `config dump` | Print configuration with secrets masked | [Config](config.md) |
| `computer status\|enable\|disable` | Desktop automation gate (macOS only) | [Utilities](utilities.md#myaaw-computer) |
| `codesign setup\|status\|remove` | Local macOS code-signing identity | [Utilities](utilities.md#myaaw-codesign) |

### Maintenance

| Command | Description | Document |
| :--- | :--- | :--- |
| `backup` | Write a `tar.gz` per agent | [Backup](backup.md) |
| `restore` | Verify and restore an archive | [Backup](backup.md#restore) |
| `update` | Check for and install updates | [Utilities](utilities.md#myaaw-update) |
| `version` | Print build information | [Utilities](utilities.md#myaaw-version) |
| `completion` | Generate shell completion | [Utilities](utilities.md#myaaw-completion) |

### Service management

| Command | Description | Document |
| :--- | :--- | :--- |
| `server run` | Run the agent server in the foreground | [Service](service.md) |
| `server start` | Start the agent daemon | [Service](service.md) |
| `server stop` | Stop and unregister the daemon | [Service](service.md) |
| `server restart` | Restart the daemon | [Service](service.md) |
| `server status` | Check daemon status | [Service](service.md) |
| `api` | Unified control plane API | [API Command](api.md) |

`start`, `stop`, and `restart` are aliases for `server start`, `server stop`, and `server restart`.

## Conventions

- **Per-agent everything.** Config, `.env`, database, memory, skills, logs, and the server port belong to one agent under `~/.myaaw/<agent>/`. Use `--agent` or `agent use` to switch.
- **Daemons are per agent.** Each agent runs `myaaw-server-<agent>` on its own `PORT`. Rebuilding the binary does not change a running daemon; restart each agent to pick up new code.
- **Secrets stay local.** `.env`, `config/config.json`, `contacts.json`, and `controlplane.json` contain credentials. They are never printed unmasked by `config dump`.
- **Exit codes.** Success is `0`. Command errors print a single `Error: ...` line and exit non-zero.
- **Machine-readable output.** `backup` and `restore` accept `--json`.
- **Shell completion.** `myaaw completion bash|zsh|fish|powershell`.

## Data layout

```text
~/.myaaw/
  agents.json               # registry
  controlplane.json         # control plane credentials
  pids/                     # daemon pid files (shared)
  <agent>/
    .env
    config/config.json
    contacts.json
    database/myaaw.db
    database/observability.db
    home/
    skills/
    tools/
    cron/
    jobs/
    logs/
```

See [Multi-Agent](../agents/multi-agent.md) for the full directory reference.
