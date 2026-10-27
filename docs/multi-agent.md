# Multi-Agent Guide

This guide explains how to run multiple isolated Myaaw agents on one machine: how to create them, configure providers and channels, run their servers, and understand the per-agent filesystem layout.

## 1. Overview

Myaaw supports multiple isolated agents under a single home directory:

```text
~/.myaaw/
├── agents.json
├── pids/
├── default/
└── myfriend/
```

Each agent has its own:

- config and `.env`
- identity, memory, and workspace
- SQLite database
- skills, jobs, logs, cron, and tools
- LLM providers
- Telegram/Discord channels
- server process, port, and daemon name

The core model is:

> **One process = one agent.**

A running server or CLI session selects one agent at startup. It does not switch agents while running.

## 2. Agent names

Agent names are used as directory names and CLI arguments. Valid names follow:

```text
^[A-Za-z0-9][A-Za-z0-9_-]*$
```

Rules:

- only letters, numbers, `_`, and `-`
- must start with a letter or number
- no spaces, dots, slashes, or other special characters
- maximum 64 characters
- case-sensitive
- `pids` is reserved

Examples:

```text
valid:   default, myfriend, friend_2, agent-3, team1
invalid: my friend, ../admin, my.friend, my/friend, -agent, _agent, pids
```

## 3. Directory layout

```text
~/.myaaw/
├── agents.json
├── pids/
│   ├── myaaw-server-default.pid
│   └── myaaw-server-myfriend.pid
├── default/
│   ├── config/config.json
│   ├── .env
│   ├── home/
│   │   ├── AGENTS.md
│   │   ├── MEMORY.md
│   │   ├── SOUL.md
│   │   ├── TOOLS.md
│   │   ├── USER.md
│   │   ├── memory/
│   │   └── workspace/
│   │       ├── projects/
│   │       └── downloads/
│   ├── skills/
│   ├── logs/
│   ├── jobs/
│   ├── cron/
│   ├── tools/
│   └── database/
│       ├── myaaw.db
│       ├── calendar/
│       ├── cashflow/
│       └── notes/
└── myfriend/
    └── ...same structure
```

`~/.myaaw/agents.json` stores the registry:

```json
{
  "default": "default",
  "agents": {
    "default": {
      "created_at": "2026-10-19"
    },
    "myfriend": {
      "created_at": "2026-10-19"
    }
  }
}
```

## 4. First install and migration

### Fresh install

```bash
myaaw onboard
```

This creates the default agent:

```text
~/.myaaw/default/
```

To initialize a specific agent directly:

```bash
myaaw --agent myfriend onboard
```

### Legacy layout migration

On the first command after upgrading, Myaaw migrates the old single-agent layout:

```text
~/.myaaw/home
~/.myaaw/config
~/.myaaw/database
~/.myaaw/skills
~/.myaaw/logs
~/.myaaw/jobs
~/.myaaw/cron
~/.myaaw/tools
~/.myaaw/.env
```

into:

```text
~/.myaaw/default/
```

Legacy path references in text files and skill scripts are rewritten to the new agent-specific paths. Duplicate log files are preserved as `.legacy`.

The migration moves directories; it does not create a full backup. Back up `~/.myaaw` before the first run of a new binary if the data matters.

## 5. Selecting an agent

### In `myaaw chat`

```bash
myaaw chat
```

If more than one initialized agent exists, Myaaw shows an arrow-key picker:

```text
Select an agent
  > myfriend (default)
    default
```

Use `↑`/`↓` and `Enter`.

For one-shot messages:

```bash
myaaw chat -m "hello" --agent myfriend
```

Without `--agent`, the registry default is used.

### With the global flag

```bash
myaaw --agent myfriend <command>
```

Examples:

```bash
myaaw --agent myfriend status
myaaw --agent myfriend provider list
myaaw --agent myfriend channel list
myaaw --agent myfriend logs
```

### With environment variables

```bash
MYAAW_AGENT=myfriend myaaw chat
```

If both `--agent` and `MYAAW_AGENT` are present, `--agent` wins.

### Selection fallback

If the registry default points to a missing agent but another initialized agent exists, Myaaw selects the first initialized agent so commands can still run.

## 6. Managing agents

### List

```bash
myaaw agent list
```

### Create

```bash
myaaw agent create myfriend
```

This copies the embedded template into `~/.myaaw/myfriend/`, assigns the next free port, creates `.env`, rewrites legacy paths, and registers the agent.

It also **auto-wires A2A contacts**: the new agent gets a generated `A2A_TOKEN`, its `contacts.json` is populated with every other initialized agent (name, URL, and that agent's `A2A_TOKEN`), and the new agent is added to each other agent's `contacts.json` with the new token. Agents can therefore delegate to each other immediately after restart — no manual `contacts.json` edits. Agents without an `A2A_TOKEN` (open mode) are added as contacts without a token.

Delegation from chat goes through the `contact` tool, which covers `send`, `status` (async polling), artifact download, and contact management. Other A2A v1.0 operations (cancel, task listing, streaming, subscribe, extended card, push configs) are available on the server but not exposed to the agent; see the coverage table in the [A2A guide](a2a.md#scope-of-the-contact-tool).

The new agent is not automatically configured with real API keys or bot tokens.

### Set default

```bash
myaaw agent use
```

Shows an arrow-key picker. Or pass a name:

```bash
myaaw agent use myfriend
```

### Delete

```bash
myaaw agent delete
```

Shows an arrow-key picker and confirmation. Or:

```bash
myaaw agent delete myfriend
```

Deletion removes the agent directory and its data. The current default agent cannot be deleted.

Use `--yes` to skip the confirmation prompt when a name is provided:

```bash
myaaw agent delete myfriend --yes
```

### Onboard a specific agent

```bash
myaaw --agent myfriend onboard
```

This loads or creates `~/.myaaw/myfriend/` and runs the provider/channel setup wizard for that agent.

## 7. Providers per agent

Provider configuration is stored in:

```text
~/.myaaw/<agent>/config/config.json
```

Commands:

```bash
myaaw --agent myfriend provider create
myaaw --agent myfriend provider list
myaaw --agent myfriend provider default deepseek
myaaw --agent myfriend provider delete
```

`provider create` asks for:

- provider type
- provider name
- base URL
- API key
- default model

The first provider becomes the default for that agent.

Example config:

```json
{
  "default_provider": "deepseek",
  "providers": {
    "deepseek": {
      "type": "openai",
      "base_url": "https://api.deepseek.com",
      "api_key": "sk-...",
      "default_model": "deepseek-chat"
    }
  }
}
```

## 8. Channels per agent

Channel configuration is stored in the same per-agent config file.

Commands:

```bash
myaaw --agent myfriend channel create
myaaw --agent myfriend channel list
myaaw --agent myfriend channel edit
myaaw --agent myfriend channel delete
```

Telegram example:

```json
{
  "channels": {
    "telegram": {
      "active": true,
      "token": "BOT_TOKEN_MYFRIEND",
      "mode": "polling"
    }
  }
}
```

Rules:

- Each running agent should use a different Telegram/Discord bot token.
- Two agents using the same token will compete for the same Telegram updates.
- Restart the agent server after changing channel tokens:

```bash
myaaw server --agent myfriend restart
```

## 9. Servers per agent

Each agent runs its own Fiber server:

```bash
myaaw server --agent myfriend start
myaaw server --agent myfriend status
myaaw server --agent myfriend restart
myaaw server --agent myfriend stop
```

`myaaw start`, `myaaw stop`, and `myaaw restart` are aliases for the active agent.

Without `--agent`, the registry default is used.

### Ports

Ports are read from `PORT` in:

```text
~/.myaaw/<agent>/.env
```

New agents created with `myaaw agent create` receive the next free port, for example:

```text
default  → 8080
myfriend → 8081
```

`MYAAW_BASE_URL` is also set for the agent, for example:

```env
PORT=8081
MYAAW_BASE_URL=http://localhost:8081
```

### Daemon names and logs

Each agent has its own daemon name:

```text
myaaw-server-default
myaaw-server-myfriend
```

PID files:

```text
~/.myaaw/pids/myaaw-server-myfriend.pid
```

Server logs:

```text
~/.myaaw/myfriend/logs/
```

## 10. Per-agent resources

| Resource | Path |
|---|---|
| Config | `~/.myaaw/<agent>/config/config.json` |
| Environment | `~/.myaaw/<agent>/.env` |
| Home / identity | `~/.myaaw/<agent>/home` |
| Memory | `~/.myaaw/<agent>/home/MEMORY.md` and `memory/` |
| Workspace | `~/.myaaw/<agent>/home/workspace` |
| Skills | `~/.myaaw/<agent>/skills` |
| Jobs | `~/.myaaw/<agent>/jobs` |
| Logs | `~/.myaaw/<agent>/logs` |
| Cron | `~/.myaaw/<agent>/cron` |
| Tools | `~/.myaaw/<agent>/tools` |
| SQLite | `~/.myaaw/<agent>/database/myaaw.db` |
| Skill data | `~/.myaaw/<agent>/database/<skill>/` (skills may also write under `config/`) |
| PID files | `~/.myaaw/pids/` (shared directory, per-agent files) |
| Registry | `~/.myaaw/agents.json` (shared) |

Back up an agent with `myaaw backup --agent <name>`; the archive includes `config/*` and `database/*` generically, so data from any skill is covered. Restore it with `myaaw restore <archive>`. See the [Backup Guide](backup.md).

## 11. Filesystem isolation

The filesystem tool for an active agent allows:

```text
~/.myaaw/<active-agent>/home
~/.myaaw/<active-agent>/skills
~/.myaaw/<active-agent>/jobs
+ the current project directory (local CLI sessions only)
```

Paths under another agent's directory are rejected.

Local CLI sessions also inject the launch directory as `## Current Directory` and register it as a project. Telegram/Discord sessions do not receive the server process working directory.

## 12. Skill data isolation

Bash and Python subprocesses receive:

```text
MYAAW_AGENT=<agent-name>
MYAAW_AGENT_ROOT=<absolute-agent-root>
```

Built-in skills such as `calendar`, `cashflow`, and `notes` use `MYAAW_AGENT_ROOT` to store data:

```text
~/.myaaw/<agent>/database/<skill>/
```

If the environment variable is absent, scripts fall back to:

```text
~/.myaaw/default/database/<skill>/
```

`MYAAW_AGENT` is available for custom skills and debugging. Agent identity variables are applied last, so Bash tool arguments cannot override them.

## 13. Development workflow

The development consumer scripts build the binary once and run one agent:

```bash
./start-consumer-myaaw.sh            # agent default
./start-consumer-myaaw.sh myfriend   # agent myfriend

./stop-consumer-myaaw.sh             # agent default
./stop-consumer-myaaw.sh myfriend    # agent myfriend
```

Files:

```text
tmp/myaaw
tmp/consumer-myaaw-<agent>.pid
tmp/consumer-myaaw-<agent>.log
```

You do not need to install the binary multiple times for multiple agents.

## 14. Troubleshooting

### `Agent "default" is not initialized`

Run:

```bash
myaaw onboard
```

or create/choose an agent:

```bash
myaaw agent create myfriend
myaaw agent use myfriend
```

### Server fails to start

Check:

- Is the port already in use?
- Does another agent use the same `PORT`?
- Is the Telegram token already used by another running agent?

### Wrong provider/channel is used

Check the active agent:

```bash
myaaw --agent <name> status
myaaw --agent <name> config check
```

Check the config path:

```text
~/.myaaw/<agent>/config/config.json
```

### Wrong calendar/cashflow/notes data

Each agent writes to its own database directory. If data appears in the wrong place, check whether the skill was run manually without `MYAAW_AGENT_ROOT`.

### Agent cannot access the current project

The current directory is registered only for local `myaaw chat` and `myaaw voice-classic` sessions. Confirm that the directory is not the user home or another agent's directory.

## 15. Limitations

- Bash and Python are not OS-sandboxed. They can access other paths according to OS permissions. Filesystem-tool isolation does not restrict Bash/Python.
- Migration moves data; it does not create a full backup.
- The agent picker is used for interactive `myaaw chat`; one-shot messages use the default agent unless `--agent` is passed.
- `agents.json` writes are atomic, but concurrent `agent create/use/delete` operations are not file-locked.
- Skills that use hardcoded global paths may need to be updated to use `MYAAW_AGENT_ROOT`.

## 16. Command reference

| Command | Purpose |
|---|---|
| `myaaw chat` | Interactive chat with agent picker |
| `myaaw chat --agent <name>` | Chat with a specific agent |
| `myaaw chat -m "..." --agent <name>` | One-shot message |
| `myaaw agent list` | List initialized agents |
| `myaaw agent create <name>` | Create an agent |
| `myaaw agent use [name]` | Set default agent |
| `myaaw agent delete [name]` | Delete an agent |
| `myaaw --agent <name> onboard` | Configure a specific agent |
| `myaaw --agent <name> provider ...` | Manage providers |
| `myaaw --agent <name> channel ...` | Manage channels |
| `myaaw server --agent <name> start` | Start an agent server |
| `myaaw server --agent <name> status` | Check server status |
| `myaaw --agent <name> status` | Check agent configuration and DB |
| `myaaw --agent <name> logs` | View that agent's logs |
| `myaaw --agent <name> config check` | Validate that agent's config |
