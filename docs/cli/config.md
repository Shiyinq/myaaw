# Config

`myaaw config` validates and prints the active agent's configuration.

## Commands

```bash
myaaw config check
myaaw config dump
```

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
- Non-sensitive values (`PORT`, `MYAAW_BASE_URL`, bot settings, heartbeat, cron, etc.) are shown as-is.

Use it when reporting a problem; review the output before sharing it.

## Files

| Path | Contents |
| :--- | :--- |
| `~/.myaaw/<agent>/.env` | Port, base URL, tokens, API keys. |
| `~/.myaaw/<agent>/config/config.json` | Providers, channels, bot settings, heartbeat, cron defaults. |
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
```

## Notes

- `config check` reports the presence of keys, not whether they are valid for the remote service. Use `myaaw status` or an actual request to verify connectivity.
- A broken `config.json` is reported as `[BROKEN REF]` when the default provider is missing from the providers block.
- Missing `config.json` suggests running `myaaw onboard`.

## Related

- [Providers](provider.md)
- [Channels](channel.md)
- [Onboarding](onboard.md)
- [Utilities](utilities.md)
