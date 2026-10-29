# Config

`myaaw config` validates and prints the active agent's configuration.

## Commands

```bash
myaaw config check
myaaw config dump
myaaw config approvals show
myaaw config approvals set --mode manual --timeout 60
myaaw config approvals allow add filesystem:read_file
myaaw config approvals grants list
```

`config approvals` manages the tool approval policy and runtime grants; see
[Tool Approvals](../agents/approvals.md) for the full reference.

Use `--agent` for another agent:

```bash
myaaw --agent agent-b config check
```

## `config check`

Prints a health report for:

1. **Environment variables** (`.env` / OS):
   `PORT`, `HOST`, `MYAAW_BASE_URL`, `ALLOWED_ORIGINS`, `DB_PATH`, `TAVILY_API_KEY`.
2. **`config.json`**:
   - default LLM provider exists and is present in the `providers` block,
   - transcriber provider and key,
   - Telegram and Discord channel state and tokens.

Example:

```text
Checking Environment Variables (.env / OS):
PORT                      : [OK]
TAVILY_API_KEY            : [EMPTY]

Checking config.json Health:
Default LLM Provider      : [OK] (Active: deepseek)
Channel: Telegram         : [OK] (Mode: polling)
Channel: Discord          : [INACTIVE]
```

This command is informational; it reports issues but does not change files. Use `myaaw onboard`, `provider`, or `channel` to fix them.

## `config dump`

Prints the active configuration with secrets masked:

- Keys containing `KEY`, `TOKEN`, `PASSWORD`, or `SECRET` are shown as `********`.
- Provider API keys are masked.
- Non-sensitive values (`PORT`, `MYAAW_BASE_URL`, bot settings, heartbeat, cron, approvals, etc.) are shown as-is.

Use it when reporting a problem; review the output before sharing it.

## `config approvals`

Reads and writes the approval policy in `config/config.json` and the runtime
grants in `config/approvals.json`.

```bash
myaaw config approvals show [--json]
myaaw config approvals set [--mode manual|off] [--timeout 60] \
                           [--unattended allow|deny] [--subagents inherit|allow|deny]
myaaw config approvals allow list
myaaw config approvals allow add <key>
myaaw config approvals allow remove <key>
myaaw config approvals deny list
myaaw config approvals deny add <key>
myaaw config approvals deny remove <key>
myaaw config approvals grants list [--json]
myaaw config approvals grants revoke <key> [--session <conversation-id>]
myaaw config approvals grants clear
```

### `show`

Prints the **effective** policy — `config.json` merged with the shipped
defaults — followed by the runtime grants:

```text
Approvals
  mode:               manual
  timeout_seconds:    60
  unattended:         allow
  subagents:          inherit
  allow (11):          filesystem:list_directory, filesystem:read_file, ...
  deny (0):

Grants
  bash: always
  playwright_browser_navigate: sessions=09e98570-...
```

`--json` emits the same shape used by `GET /agents/:name/approvals`:

```json
{
  "config": { "mode": "manual", "timeout_seconds": 60, "allow": ["filesystem:read_file", "computer:screenshot"], "deny": [], "unattended": "allow", "subagents": "inherit" },
  "grants": [
    { "key": "bash", "always": true },
    { "key": "filesystem:write_file", "sessions": [{ "id": "...", "created_at": "..." }] }
  ]
}
```

### `set`

Updates only the flags you pass; the rest of the block is preserved (and
defaults are filled in on first write).

```bash
myaaw config approvals set --mode manual --timeout 30
myaaw config approvals set --mode off
myaaw config approvals set --unattended deny --subagents allow
```

Values are validated before writing:

- `mode`: `manual` or `off`
- `timeout`: 1–3600 seconds, applied to **each** approval prompt (on expiry the
  call is denied and the run continues)
- `unattended`: `allow` or `deny`
- `subagents`: `inherit`, `allow`, or `deny`

### `allow` / `deny`

Keys are `tool` or `tool:action` (for example `bash`,
`filesystem:read_file`, `computer:click`). `*` matches everything.

```bash
myaaw config approvals allow add filesystem:read_file
myaaw config approvals allow remove filesystem:read_file
myaaw config approvals deny add bash
myaaw config approvals deny list
```

- In `mode: manual`, every tool needs approval unless it is in `allow`.
- `deny` always refuses and wins over `mode: off`.
- The same key cannot be in both lists; remove it from one first.

`list` prints a heading followed by the entries (and `(empty)` when the list is
empty):

```text
Allow
  filesystem:list_directory
  filesystem:read_file

Deny
  (empty)
```

### `grants`

Grants are created at runtime by the approval prompt (`s` = session,
`a` = always). The store is read-modify-write and written atomically, so
revoking one entry never removes the others.

```bash
myaaw config approvals grants list
myaaw config approvals grants revoke bash
myaaw config approvals grants revoke filesystem:write_file --session 09e98570-...
myaaw config approvals grants clear
```

- `revoke <key>` removes `always` and every session grant for that key.
- `--session` removes only that conversation's grant.
- `clear` empties `config/approvals.json`; the policy is untouched.
- Grants are re-read on each run, so changes apply to the next run without a
  restart. `myaaw chat` loads policy at start, so restart it after `set`,
  `allow`, or `deny`.

See [Tool Approvals](../agents/approvals.md) for the policy semantics, per-channel
prompts, sub-agents, A2A, and API endpoints.

## Files

| Path | Contents |
| :--- | :--- |
| `~/.myaaw/<agent>/.env` | Port, base URL, tokens, API keys. |
| `~/.myaaw/<agent>/config/config.json` | Providers, channels, bot settings, heartbeat, cron defaults, approvals policy. |
| `~/.myaaw/<agent>/config/approvals.json` | Runtime approval grants (allow session / always). |
| `~/.myaaw/<agent>/database/myaaw.db` | Conversations and runtime state. |

## Examples

```bash
# Check the active agent
myaaw config check

# Dump another agent's configuration (secrets masked)
myaaw --agent agent-b config dump

# Show only the config health, then fix a missing provider
myaaw config check
myaaw provider create
myaaw provider default <name>

# Approvals: inspect the effective policy and grants
myaaw config approvals show

# Ask before bash, but let read-only filesystem calls run
myaaw config approvals allow add filesystem:read_file
myaaw config approvals deny add bash

# Revoke an "Always" grant for bash
myaaw config approvals grants revoke bash
```

## Notes

- `config check` reports the presence of keys, not whether they are valid for the remote service. Use `myaaw status` or an actual request to verify connectivity.
- A broken `config.json` is reported as `[BROKEN REF]` when the default provider is missing from the providers block.
- Missing `config.json` suggests running `myaaw onboard`.

## Related

- [Providers](provider.md)
- [Channels](channel.md)
- [Onboarding](onboard.md)
- [Tool Approvals](../agents/approvals.md)
- [Utilities](utilities.md)
