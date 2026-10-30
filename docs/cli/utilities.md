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

## `myaaw computer`

Enable or disable the built-in desktop automation tool for an agent (macOS
only):

```bash
myaaw computer status    # feature gate, registry gate, platform, effective state
myaaw computer enable
myaaw computer disable
```

The built-in tool is **macOS-only**: on Linux/Windows it returns
`COMPUTER_UNSUPPORTED_PLATFORM` for every action, so use an MCP driver instead
(see [Computer Use](../agents/computer-use.md)).

Two gates must allow the tool: `bot.computer_use.enabled` in `config.json`
(written by this command) and `builtin_tools.computer` in `tools.json`
(absent means enabled). Restart `myaaw api` and the agent servers after
changing it.

`myaaw computer enable` also checks the local "Myaaw Agent" signing identity
(creating it when missing) and prints the macOS permission checklist:
Screen Recording, Accessibility, and Automation → System Events. Verify with
`myaaw doctor`.

## `myaaw codesign`

macOS only. The local "Myaaw Agent" certificate gives the binary a stable code
identity, so macOS permission grants (Screen Recording, Accessibility,
Automation) survive rebuilds and updates.

```bash
myaaw codesign setup    # one time: create the local signing identity
myaaw codesign status   # identity + the running binary's signature
myaaw codesign remove   # delete the identity + trust settings (confirms first)
```

`remove` asks for confirmation; pass `--yes` to skip it. Use
`MYAAW_CODESIGN_IDENTITY` to target a different certificate name. Existing
binaries stay signed after removal, but future builds/updates stop being
signed automatically (grant reset per update) until `setup` is run again.

On non-macOS platforms every subcommand is a **no-op**: it prints that
codesigning is macOS-only and exits 0, so scripts can call it unconditionally
(no keychain, files, or elevation are touched).

`install.sh` and `myaaw update` re-sign automatically once the identity exists.
No source checkout is required for `codesign setup`. See
[Computer Use](../agents/computer-use.md#2-macos-permissions) for the full
permission model.

## `myaaw doctor`

Checks environment readiness for Myaaw features, starting with desktop
automation (computer use):

| Check | What it verifies |
| :--- | :--- |
| Launched from | Which app macOS attributes the permission request to (terminal or `myaaw` binary) |
| Feature enabled | `MYAAW_COMPUTER_USE` / `bot.computer_use.enabled` |
| Operating system | macOS (the darwin backend) |
| cliclick | Whether the recommended input driver is installed |
| Screen Recording permission | Takes one screenshot to a temp file and deletes it |
| System Events access | Automation (Apple Events) to System Events; fixes error `-1743` |
| Desktop size | Logical desktop size in points |

Accessibility cannot be probed without synthesizing input, so `doctor` does
not test it directly — grant it to the same app for `cliclick` to move the
mouse and type. Screen Recording needs a **relaunch** of the app after
toggling; Automation applies immediately. See [Computer Use](../agents/computer-use.md#2-macos-permissions)
for the three panes and the `tccutil reset` recovery.

```bash
myaaw doctor
```

The probe is **non-destructive**: it never moves the pointer or sends input.
On Linux/Windows every check collapses into a single unsupported-platform
result that points at the cua-driver MCP route. See [Computer Use](../agents/computer-use.md)
for what each permission is used for.

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
