# Utilities

Small commands for health, build info, updates, and shell integration.

## `myaaw status`

Prints a health summary for the active agent:

| Section | What it shows |
| :--- | :--- |
| Gateway / Server | Whether `myaaw-server-<agent>` is running, its PID and port. |
| SQLite DB | Whether the database file is reachable, and its path. |
| Channels | Telegram (and mode) and Discord configured state. |
| Heartbeat | Whether the scheduled self-check is active, its interval and target. |
| LLM Provider | Active provider and streaming setting. |

```bash
myaaw status
myaaw --agent agent-b status
```

This is a read-only summary. For daemon control, use [Service Management](service.md).

## `myaaw version`

Prints:

```text
Myaaw CLI Version: <version>
Git Commit:        <hash>
Build Date:        <timestamp>
Go Version:        <version>
```

Versions injected at build time show the commit; a `dev` version means the binary was built without release metadata.

## `myaaw update`

Checks GitHub releases for a newer version, prompts before downloading, installs the new binary, and attempts to restart the service.

```bash
myaaw update
```

There is a hidden `--auto-confirm` flag used by automated installs. After an update, restart every agent that should use the new binary:

```bash
myaaw --agent default server restart
myaaw --agent agent-b server restart
```

The update command only updates the binary; agent data and configuration are untouched.

## `myaaw completion`

Generates shell completion scripts through Cobra:

```bash
myaaw completion bash
myaaw completion zsh
myaaw completion fish
myaaw completion powershell
```

Install the output according to your shell, for example:

```bash
# zsh
myaaw completion zsh > "${fpath[1]}/_myaaw"

# bash
myaaw completion bash > /usr/local/etc/bash_completion.d/myaaw
```

Restart the shell or source the file afterwards.

## Related

- [Service Management](service.md)
- [Config](config.md)
- [Logs](logs.md)
- [CLI Reference](README.md)
