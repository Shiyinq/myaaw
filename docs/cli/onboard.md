# Onboarding

`myaaw onboard` is the interactive first-run setup. It creates the agent layout, writes `.env` and `config/config.json`, and optionally installs the binary on your `PATH`.

## Usage

```bash
myaaw onboard
myaaw --agent agent-b onboard
```

Without `--agent`, the default agent (`default`) is configured.

## What it asks

| Step | Purpose |
| :--- | :--- |
| Welcome / home | Confirms `~/.myaaw/<agent>` and creates the directory layout. |
| Provider | Choose the LLM provider type and enter name, base URL, API key, and model. |
| Channels | Telegram and/or Discord. |
| Telegram mode | `polling` or `webhook`, then the bot token. |
| Discord token | Bot token, if Discord is enabled. |
| Heartbeat | Optional scheduled self-check: channel, target ID, interval. |
| Owner | Owner/chat ID used for notifications. |
| Install | Optionally install the `myaaw` binary globally. |

The wizard is a full-screen terminal UI. Use `↑`/`↓` to choose, `Enter` to confirm, and `Esc`/`Ctrl+C` to leave.

## Files written

```text
~/.myaaw/<agent>/
  .env                    # PORT, base URL, API keys, bot tokens
  config/config.json      # providers, channels, heartbeat, bot settings
  home/                   # AGENTS.md, SOUL.md, MEMORY.md, workspace
  skills/                 # built-in skill templates
  database/myaaw.db       # created on first use
  logs/
```

## After onboarding

```bash
# Check what was written
myaaw --agent agent-b config check

# Start the agent server
myaaw --agent agent-b server start

# Chat
myaaw --agent agent-b chat
```

## Notes

- Onboarding is intended for initial setup. For later changes, prefer the focused commands:
  - [Providers](provider.md)
  - [Channels](channel.md)
  - [Webhook](webhook.md)
  - [Config](config.md)
- A Telegram **polling** setup does not need a public URL. A **webhook** setup does; see [Webhook](webhook.md).
- Each agent needs its own bot token if you run several agents.
- If you skip global install, run commands with `go run ./cmd/myaaw` or install the binary manually.

## Related

- [Multi-Agent Guide](../agents/multi-agent.md)
- [Service Management](service.md)
- [Providers](provider.md)
- [Channels](channel.md)
