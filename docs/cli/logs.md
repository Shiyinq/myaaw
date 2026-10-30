# Logs

`myaaw logs` streams files from the active agent's log directory.

## Usage

```bash
myaaw logs
myaaw logs <filter>
```

## Modes

| Mode | Behavior |
| :--- | :--- |
| No argument | Interactive picker (arrow keys + enter) listing `.log` files. |
| `filter` | Streams every log whose filename contains the filter. |
| `help` | Prints the command help. |

## Flags

| Flag | Default | Description |
| :--- | :--- | :--- |
| `-f`, `--follow` | `true` | Follow the file like `tail -f`. |
| `-n`, `--tail` | `10` | Number of lines to show before following. |

## Examples

```bash
myaaw logs                 # interactive menu
myaaw logs chat            # every file with "chat" in its name
myaaw logs -n 100          # last 100 lines
myaaw logs error -f=false  # search "error" logs without following
myaaw --agent agent-b logs server
```

On macOS/Linux, `-f=false` disables following. Press Ctrl+C to stop a followed stream.

## Log directory

```text
~/.myaaw/<agent>/logs/
```

Typical files:

| File | Contents |
| :--- | :--- |
| `myaaw-server-<agent>.log` | Server daemon stdout/stderr. |
| `myaaw-chat-cli.log` | CLI chat activity. |
| `tools.log` | External tool loading and tool diagnostics. |
| `skills.log` | Skill discovery and refresh. |
| `voice-transcript-*.log` | Voice session transcripts. |
| `frames/`, `audio/` | Optional voice artifacts. |

The log directory is created when the agent first logs. If it does not exist yet, the command prints a message instead of an error.

## Notes

- Use `--agent` to read another agent's logs.
- Daemon startup failures are easiest to diagnose in `myaaw-server-<agent>.log`.

## Related

- [Service Management](service.md)
- [Multi-Agent Guide](../agents/multi-agent.md)
- [Config](config.md)
