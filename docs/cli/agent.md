# Agent Command

`myaaw agent` manages the isolated agents stored under `~/.myaaw/<agent>/`. This page covers the command surface only; the registry, directory layout, per-agent ports, and isolation rules are in the [Multi-Agent Guide](../agents/multi-agent.md).

## Commands

```bash
myaaw agent list
myaaw agent create <name>
myaaw agent use [name]
myaaw agent delete [name] [--yes]
```

`agent` is a global command, so it does not use the `--agent` flag. To configure the agent you just created, run `myaaw --agent <name> onboard`.

## Agent names

Names must match `[A-Za-z0-9][A-Za-z0-9_-]*`, start with a letter or number, and be at most 64 characters. `pids` is reserved.

```text
agent-b                  ✅
agent_working            ✅
agent-personal-working   ✅
my-agent-2024            ✅
-bad                     ❌ starts with a dash
bad name                 ❌ spaces are not allowed
pids                     ❌ reserved for ~/.myaaw/pids
```

Prefer lowercase. macOS filesystems are case-insensitive by default, so `Agent-B` and `agent-b` resolve to the same directory. Avoid `global`, which is not reserved by the validator but collides with the shared registry, logs, and PID directories.

## `agent list`

Prints every initialized agent, one per line, and marks the registry default:

```text
  agent-b (default)
  agent-c
```

If nothing is initialized it prints `No agents registered. Run 'myaaw onboard' to create one.`

## `agent create <name>`

Alias for onboarding that pre-fills the agent name. It runs the same implementation as `myaaw onboard`, so there is no second setup path:

```bash
myaaw agent create agent-b
```

What happens:

1. Validates the name and refuses names that are already taken (existing agent, leftover directory, or case-only duplicate).
2. Opens the onboarding wizard **starting at the provider step** — no name prompt, no picker.
3. Asks for provider, model, skills, built-in tools, channels, access type, heartbeat, and owner.
3. When you save the **Review** step, it creates `~/.myaaw/<name>/` from the embedded template, assigns a unique port, writes `.env`, registers the agent in `agents.json`, wires A2A (`A2A_TOKEN`, contacts in both directions, self excluded), and writes `config.json`.

The global install question is only shown for the **first agent** on the machine; later agents skip it.

Cancelling with `Ctrl+C` before Review leaves nothing behind; no directory, registry entry, or contacts are created.

It accepts the same flags as [`myaaw onboard`](onboard.md#headless-mode-ci-scripts), so it can also run headless:

```bash
myaaw agent create agent-b --yes \
  --provider gemini --model models/gemini-2.5-flash \
  --channel telegram --telegram-token "$TG_TOKEN" --owner 123456789
```

To reconfigure an existing agent, use `myaaw --agent <name> onboard` instead; `agent create` never overwrites an existing agent.

## `agent use [name]`

Sets the default agent. Without a name it opens an arrow-key picker.

```bash
myaaw agent use agent-b
```

The name must already exist; otherwise the command prints `Agent "<name>" does not exist.`

## `agent delete [name] [--yes]`

Deletes an agent and everything under its directory. Without a name it opens a picker that excludes the reserved `default` agent and the current default agent.

```bash
myaaw agent delete agent-c
myaaw agent delete agent-c --yes   # skip the confirmation prompt
```

Steps performed:

1. Refuses `default` and the current default agent. Switch first with `myaaw agent use <name>`.
2. Stops the agent daemon (`myaaw-server-<agent>`) if it is running, so it cannot recreate `tools/` or `logs/` during deletion.
3. Removes `~/.myaaw/<agent>/`, retrying briefly while the stopped daemon releases files.
4. Unregisters the name from `~/.myaaw/agents.json`.

Deletion is not reversible and does not create an archive. Back up first:

```bash
myaaw backup --agent agent-c
myaaw agent delete agent-c
```

See [Backup & Restore](backup.md) for restoring that archive later, including under a different agent name.

## Examples

```bash
# Create, configure, and switch
myaaw agent create agent-b
myaaw --agent agent-b onboard
myaaw agent use agent-b

# List and delete
myaaw agent list
myaaw backup --agent agent-c
myaaw agent delete agent-c --yes
```

## Related

- [Multi-Agent Guide](../agents/multi-agent.md) — registry, selecting an agent, per-agent providers, channels, ports, and servers.
- [Onboarding](onboard.md)
- [Backup & Restore](backup.md)
- [Service Management](service.md)
