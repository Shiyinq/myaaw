# Service Management

Each agent runs its own server daemon. The daemon serves the per-agent HTTP API, channel listeners (Telegram/Discord), cron workers, and A2A endpoints.

On top of the per-agent daemons, the `start`, `stop`, and `restart` commands orchestrate the whole stack: the selected agent servers, the control plane (`myaaw api`), and the dashboard (`myaaw webui`) when it is installed.

## Whole-stack commands

```bash
myaaw start            # start the default agent, the API, and the WebUI
myaaw start --agent agent-b
myaaw start --all-agent

myaaw stop             # same targets, on the way down
myaaw restart          # stop, wait one second, start again
myaaw status           # API + WebUI on top, then the agent report
```

| Invocation | Agent servers | API | WebUI (if installed) |
| :--- | :--- | :--- | :--- |
| `myaaw start` | default agent | yes | yes |
| `myaaw start --agent agent-b` | `agent-b` only | yes | yes |
| `myaaw start --all-agent` | every registered agent | yes | yes |

- The API and the WebUI are always included; only the set of agents changes.
- `--all-agent` takes precedence over `--agent` and is accepted only by `start`, `stop`, `restart`, and `status`.
- Targets run in order: agents → API → WebUI. `stop` and `restart` reverse that order, and `restart` waits one second between the two phases.
- The WebUI is skipped with `not installed` when `~/.myaaw/webui/current/build/index.js` is missing.
- Start waits up to five seconds per daemon for a live PID. If a daemon exits right after registration, that target is reported as failed with the path to its log instead of a false success.
- A failure does not stop the remaining targets; the command exits non-zero at the end.

Daemon control stays available per agent through `myaaw server start|stop|restart|status`, which only touches the agent named by `--agent` (or the default) and does not touch the API or the WebUI.

## Status

`myaaw status` prints the API and the dashboard first, then the agent report:

```text
Myaaw Service Status
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Services
  API      Running (PID: 1234)  http://localhost:9000/docs/
  WebUI    Running (PID: 1235)  http://127.0.0.1:5050
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Agent: default (default)
  Gateway      OPERATIONAL
  Server       Running (PID: 1236, Port: 8080)
  SQLite DB    Online (Path: ~/.myaaw/agents/default/database/myaaw.db)
  Telegram     Configured (polling)
  Discord      Configured
  Heartbeat    Disabled
  Provider     openai
  Streaming    Enabled
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

- Without flags the report covers the default agent. `--agent <name>` covers one agent instead.
- `--all-agent` prints the full block for every registered agent. Each block is read from that agent's own `.env` and `config.json`, so several agents can be reported in one process without loading the active agent's configuration.
- The WebUI row shows `not installed` on machines that never installed a bundle.
- This is a read-only summary; it never starts or stops anything.

## `server` commands

```bash
myaaw server run       # foreground
myaaw server start     # background daemon
myaaw server stop
myaaw server restart
myaaw server status
```

All `server` commands accept the global `--agent` flag. `server start`, `server stop`, `server restart`, and `server status` also accept `--all-agent`:

```bash
myaaw --agent agent-b server restart
myaaw server status --all-agent
```

`--all-agent` takes precedence over `--agent`. `server run` is a foreground single-agent process and does not accept it. Output always names the agent, so a mixed report is unambiguous:

```text
Agent server status
  agent "default"  running (PID: 1234, Port: 8080)
  agent "agent-b"  running (PID: 1235, Port: 8082)
  agent "agent-c"  stopped
  agent "agent-d"  running (PID: 1236, Port: 8083)
```

## Behavior

| Command | What it does |
| :--- | :--- |
| `server run` | Runs the server in the foreground. Useful for debugging; Ctrl+C stops it. |
| `server start` | Registers and starts `myaaw-server-<agent>` through Launchd (macOS) or systemd (Linux). `--all-agent` starts every registered agent, one at a time, naming each agent in the output. |
| `server stop` | Stops the daemon and unregisters it from the service manager. `--all-agent` stops every registered agent. |
| `server restart` | Stop, wait one second, start again. `--all-agent` restarts every registered agent. |
| `server status` | Prints a line per agent with its name, PID, and port. `--all-agent` reports every registered agent. |

## Ports and logs

- `PORT` comes from `~/.myaaw/agents/<agent>/.env`. Each agent must use a different port.
- Daemon name: `myaaw-server-<agent>`.
- Logs: `~/.myaaw/agents/<agent>/logs/myaaw-server-<agent>.log`.
- PID files: `~/.myaaw/pids/`.
- The agent data directory must exist and be initialized before `server start`.

## After rebuilding

A rebuilt binary does not replace a running process. Restart every agent that should pick up the new code:

```bash
myaaw restart --all-agent
```

That stops and starts every agent sequentially. To restart one agent only, use `myaaw restart --agent <name>` (which also restarts the API and the WebUI), or `myaaw server restart` for the daemon alone.

## Examples

```bash
# Debug in the foreground
myaaw --agent agent-b server run

# Start one agent in the background and check it
myaaw --agent agent-b server start
myaaw --agent agent-b server status

# Bring up the whole stack, then check every agent
myaaw start
myaaw status --all-agent

# Restart the default stack after changing channels or updating the binary
myaaw restart
```

## Troubleshooting

| Symptom | Check |
| :--- | :--- |
| `Agent "x" is not initialized` | Run `myaaw onboard` or `myaaw agent create <name>`. |
| Server exits immediately | Read `~/.myaaw/agents/<agent>/logs/myaaw-server-<agent>.log`; usually a missing provider key or an occupied port. `myaaw start` reports this target as failed with the log path. |
| Port already in use | Change `PORT` in the agent's `.env`, then restart. |
| Changes not applied | Restart the specific agent; only one agent may have been restarted. |
| `agents.json` race after parallel restarts | Retry the failed restart sequentially; `--all-agent` already does this one agent at a time. |
| WebUI reports stopped | Check `myaaw webui status` and `~/.myaaw/logs/myaaw-webui.log`. Node.js 22+ must be on the startup PATH; `myaaw webui start` pins the binary it resolved. |

## Related

- [Multi-Agent Guide](../agents/multi-agent.md)
- [Control Plane API](../api/control-plane.md)
- [WebUI Command](webui.md)
- [Logs](logs.md)
- [Config](config.md)
