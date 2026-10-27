# Onboarding

`myaaw onboard` is the interactive setup for one agent. On a fresh install it also asks for the agent name, so the data lands in `~/.myaaw/<name>/` instead of a generic `default/` folder.

## Usage

```bash
myaaw onboard                      # first run: asks for the agent name
myaaw --agent agent-b onboard      # configure or reconfigure a specific agent
myaaw onboard                      # with agents present: picker, or create a new one
```

Without `--agent`:

- **No agent yet** → the wizard asks for a name, validates it, and creates the agent.
- **Agents already exist** → a picker lists them, plus `+ Create a new agent`.
- **Only the legacy `default` agent exists** → it is used as is, so existing installs keep working.

## Agent name

The name becomes the folder `~/.myaaw/<name>/` and must match `[A-Za-z0-9][A-Za-z0-9_-]*`, start with a letter or number, and be at most 64 characters. `pids` is reserved, and an existing agent cannot be reused from the name prompt (use `--agent <name> onboard` to reconfigure it).

```text
personal                 ✅
agent-b                  ✅
agent_working            ✅
agent-personal-working   ✅
default                  ⚠️  allowed, but keeps the generic folder name
```

## What it asks

| Step | Purpose |
| :--- | :--- |
| Welcome | Intro screen. |
| Agent | Name prompt (first run) or picker (agents exist). |
| Initialize | Creates `~/.myaaw/<agent>/`, registers the agent, assigns a port, seeds A2A contacts. |
| Provider | Type (OpenAI-compatible or Gemini), config name, base URL, API key, then a model picked from the live model list. |
| Channels | Telegram, Discord, both, or skip. |
| Telegram mode | `polling` or `webhook`, then the bot token. |
| Discord | Bot token, if Discord is enabled. |
| Heartbeat | Channel, destination ID, and interval (`15m`, `30m`, `1h`, ...). |
| Owner | One or more owner IDs (comma separated) used by `bot.type: private`. |
| Review | Summary of everything that will be written, plus warnings. |
| Install | Optional copy of the binary to `~/.local/bin/myaaw`. |

Keys and tokens are masked while typing. `Esc`/`Ctrl+C` cancels; nothing is written until the **Review** step is saved.

## Provider defaults

The base URL prompt is prefilled with the public endpoint for the chosen type:

| Type | Default base URL |
| :--- | :--- |
| OpenAI (or compatible) | `https://api.openai.com` |
| Gemini | `https://generativelanguage.googleapis.com` |

Override it for OpenAI-compatible gateways (`https://api.deepseek.com`, `https://openrouter.ai/api/v1`) or a local Ollama (`http://localhost:11434`). The model list is fetched from that endpoint; if the request fails you can type the model name manually.

## Files written

```text
~/.myaaw/<agent>/
  .env                    # PORT, MYAAW_BASE_URL, ALLOWED_ORIGINS, tuning keys
  config/config.json      # providers (base_url + default_model), channels, heartbeat, bot.owner_ids
  contacts.json           # A2A contacts seeded from the registry
  home/                   # AGENTS.md, SOUL.md, MEMORY.md, workspace
  skills/                 # built-in skill templates
  database/myaaw.db       # created on first use
  logs/
```

`PORT` is assigned once: `8080` for the first agent, then the next free port. Re-running onboarding keeps the existing port, so a running daemon is not moved.

## Re-running onboarding

- Provider and model are replaced with the values you enter.
- Channels and heartbeat are only rewritten when you configure them; choosing **Skip** leaves existing settings untouched, so a working Telegram bot is not disabled by accident.
- Values left over from older templates (`YOUR_*` placeholders) are removed before saving.

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

- For later changes, prefer the focused commands:
  - [Providers](provider.md)
  - [Channels](channel.md)
  - [Webhook](webhook.md)
  - [Config](config.md)
- A Telegram **polling** setup needs no public URL. **Webhook** mode does; register it with `myaaw --agent <name> webhook set <domain>` after onboarding.
- `myaaw voice` (real-time) requires a Gemini provider; `voice-classic` works with any provider.
- Each agent needs its own bot token if you run several agents.
- The install step copies the binary to `~/.local/bin`; if that directory is not in `PATH`, the wizard prints the line to add to your shell profile.
- On Windows the install step is skipped; add the binary to `PATH` manually.

## Related

- [Agent Command](agent.md)
- [Multi-Agent Guide](../agents/multi-agent.md)
- [Service Management](service.md)
- [Providers](provider.md)
- [Channels](channel.md)
