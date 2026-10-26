# Service Management

Each agent runs its own server daemon. The daemon serves the per-agent HTTP API, channel listeners (Telegram/Discord), cron workers, and A2A endpoints.

## Commands

```bash
myaaw server run       # foreground
myaaw server start     # background daemon
myaaw server stop
myaaw server restart
myaaw server status
```

Aliases:

```bash
myaaw start
myaaw stop
myaaw restart
```

All commands accept the global `--agent` flag:

```bash
myaaw --agent agent-b server restart
```

## Behavior

| Command | What it does |
| :--- | :--- |
| `server run` | Runs the server in the foreground. Useful for debugging; Ctrl+C stops it. |
| `server start` | Registers and starts `myaaw-server-<agent>` through Launchd (macOS) or systemd (Linux). |
| `server stop` | Stops the daemon and unregisters it from the service manager. |
| `server restart` | Stop, wait one second, start again. |
| `server status` | Prints the PID when running, otherwise reports it is stopped. |

`myaaw status` is different: it is a health summary (gateway, SQLite, channels, heartbeat, provider). See [Utilities](utilities.md).

## Ports and logs

- `PORT` comes from `~/.myaaw/<agent>/.env`. Each agent must use a different port.
- Daemon name: `myaaw-server-<agent>`.
- Logs: `~/.myaaw/<agent>/logs/myaaw-server-<agent>.log`.
- PID files: `~/.myaaw/pids/`.
- The agent data directory must exist and be initialized before `server start`.

## After rebuilding

A rebuilt binary does not replace a running process. Restart every agent that should pick up the new code:

```bash
myaaw --agent default server restart
myaaw --agent agent-b server restart
myaaw --agent agent-c server restart
```

Restart them one at a time. Concurrent restarts can race on `agents.json`.

## Examples

```bash
# Debug in the foreground
myaaw --agent agent-b server run

# Start in the background and check it
myaaw --agent agent-b server start
myaaw --agent agent-b server status

# Restart after changing channels or updating the binary
myaaw --agent agent-b server restart
```

## Troubleshooting

| Symptom | Check |
| :--- | :--- |
| `Agent "x" is not initialized` | Run `myaaw onboard` or `myaaw agent create <name>`. |
| Server exits immediately | Read `~/.myaaw/<agent>/logs/myaaw-server-<agent>.log`; usually a missing provider key or an occupied port. |
| Port already in use | Change `PORT` in the agent's `.env`, then restart. |
| Changes not applied | Restart the specific agent; only one agent may have been restarted. |
| `agents.json` race after parallel restarts | Retry the failed restart sequentially. |

## Related

- [Multi-Agent Guide](../agents/multi-agent.md)
- [Control Plane API](../api/control-plane.md)
- [Logs](logs.md)
- [Config](config.md)
