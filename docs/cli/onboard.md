# Onboarding

`myaaw onboard` is the interactive setup for one agent. On a fresh install it also asks for the agent name, so the data lands in `~/.myaaw/<name>/` instead of a generic `default/` folder.

## Usage

```bash
myaaw onboard                      # first run: asks for the agent name
myaaw --agent agent-b onboard      # configure or reconfigure a specific agent
myaaw agent create agent-b         # create agent-b, then run this same wizard
myaaw onboard                      # with agents present: picker, or create a new one
```

Without `--agent`:

- **No agent yet** → the wizard asks for a name, validates it, and creates the agent.
- **Agents already exist** → a picker lists them, plus `+ Create a new agent`.
- **Only the legacy `default` agent exists** → it is listed in the picker and can be reconfigured, so existing installs keep working.

`myaaw agent create <name>` runs this same flow with the agent name already chosen (no prompt, no picker, starts at the provider step) and accepts every flag below. It still writes nothing until Review is saved.

## Agent name

The name becomes the folder `~/.myaaw/<name>/` and must match `[A-Za-z0-9][A-Za-z0-9_-]*`, start with a letter or number, and be at most 64 characters. `pids` is reserved.

A name is rejected when it is already taken. “Taken” means:

- the agent is already initialized,
- the directory `~/.myaaw/<name>/` exists (even a leftover from an interrupted run),
- another agent differs only by case (`agent-b` vs `Agent-B`), because the filesystem on macOS/Windows is case-insensitive and A2A contacts are matched case-insensitively.

Use `--agent <name> onboard` to reconfigure an existing agent instead of creating a new one.

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
| Initialize | Shows the target directory. Nothing is written yet. |
| Provider | Type (OpenAI-compatible or Gemini), config name, base URL, API key, then a model picked from the live model list. |
| Skills | Multi-select built-in skills. **Nothing is selected by default**, so no skill folder is created unless you choose it. |
| Built-in tools | Multi-select built-in tools. **All are enabled by default**; press `Space` on a tool to disable it. |
| Channels | Telegram, Discord, both, or skip. |
| Telegram mode | `polling` or `webhook`. Choosing webhook asks for the public URL next. |
| Telegram token | Bot token, masked while typing. |
| Webhook URL | Only for webhook mode: the public `https://` base URL. Telegram receives updates at `<url>/webhook/telegram`, and Myaaw registers it right after saving. |
| Discord | Bot token, if Discord is enabled. |
| Heartbeat | Channel, destination ID, and interval (`15m`, `30m`, `1h`, ...). |
| Owner | One or more owner IDs (comma separated) used by `bot.type: private`. |
| Review | Summary of everything that will be written, plus warnings. |
| Install | Optional copy of the binary to `~/.local/bin/myaaw`. |
| Restart | Offered only when this agent's daemon is already running, so the new config is loaded immediately. |

Keys and tokens are masked while typing. `Esc` goes back one step so answers are kept; `Ctrl+C` cancels. **Nothing is written until you save from Review** — layout, `.env`, port, registry entry, A2A wiring, and `config.json` are all created at that point. Cancelling at any step (or **Cancel** on Review) leaves no agent behind.

## Provider defaults

The base URL prompt is prefilled with the public endpoint for the chosen type:

| Type | Default base URL |
| :--- | :--- |
| OpenAI (or compatible) | `https://api.openai.com` |
| Gemini | `https://generativelanguage.googleapis.com` |

Override it for OpenAI-compatible gateways (`https://api.deepseek.com`, `https://openrouter.ai/api/v1`) or a local Ollama (`http://localhost:11434`). The model list is fetched from that endpoint; if the request fails you can type the model name manually. When no model is chosen, the built-in default for that provider type is stored instead.

## Webhook mode

Telegram webhook mode needs a public **HTTPS** URL (an ngrok URL works). After the wizard saves, Myaaw calls Telegram's `setWebhook` so updates start flowing to:

```text
https://<your-domain>/webhook/telegram
```

- The URL is stored as `MYAAW_BASE_URL` in `.env`.
- If the call fails (DNS, certificate, Telegram error), onboarding still completes and prints a warning; fix the URL and run `myaaw --agent <name> webhook set <domain>`.
- If you leave the URL empty, nothing is registered and the wizard prints the command to run later.
- Polling mode ignores this entirely and needs no public URL.

## Headless mode (CI, scripts)

Every wizard question has a flag. Passing `--yes` (or running without a TTY) skips the TUI:

```bash
myaaw onboard --yes \
  --agent agent-b \
  --provider openai --provider-name deepseek \
  --base-url https://api.deepseek.com \
  --model deepseek-chat \
  --channel telegram --telegram-token "$TG_TOKEN" --telegram-mode polling \
  --owner 123456789 \
  --heartbeat-channel telegram --heartbeat-to 123456789 --heartbeat-every 30m
```

| Flag | Wizard step |
| :--- | :--- |
| `--agent <name>` | Agent name (required in headless mode; `agent create <name>` supplies it) |
| `--yes` | Skip every prompt (implied when stdin is not a TTY) |
| `--provider openai\|gemini` | Provider type (required) |
| `--provider-name` | Provider name stored in `config.json` |
| `--base-url` | Provider base URL |
| `--api-key` | API key (prefer `MYAAW_API_KEY`) |
| `--model` | Default model |
| `--channel telegram\|discord\|both\|skip` | Channel choice |
| `--telegram-mode polling\|webhook` | Telegram mode |
| `--telegram-token` | Telegram bot token (prefer `TELEGRAM_BOT_TOKEN`) |
| `--webhook-url` | Public `https://` URL (webhook mode) |
| `--discord-token` | Discord bot token (prefer `DISCORD_BOT_TOKEN`) |
| `--heartbeat-channel`, `--heartbeat-to`, `--heartbeat-every` | Heartbeat |
| `--owner` | Owner IDs, comma separated |
| `--skills time,notes` | Skills to install, comma separated (`all` selects every built-in skill). Default: none |
| `--disable-tools bash,cron` | Built-in tools to disable, comma separated (`all` disables every one). Default: none |
| `--port` | Explicit server port instead of the first free one |
| `--install` | Copy the binary to `~/.local/bin` (off by default in headless mode) |
| `--restart` | Restart the agent daemon after writing, if it is running |
| `--force` | Allow reconfiguring an existing agent |
| `--json` | Print a machine-readable summary (implies `--yes`) |

Rules:

- Secrets should come from the environment: `MYAAW_API_KEY`, `TELEGRAM_BOT_TOKEN`, `DISCORD_BOT_TOKEN`, `MYAAW_WEBHOOK_URL`.
- `--channel` defaults to `skip`, which leaves existing channel settings untouched.
- `--skills` and `--disable-tools` take one or more names separated by commas; spaces after commas are fine. Invalid names exit non-zero and print the valid list.
- `--port` is rejected if another agent already uses it or another process is listening on it.
- Missing required values exit non-zero with a message that names the flag, instead of hanging on a prompt.
- Re-running without `--force` on an initialized agent is refused.
- A leftover `~/.myaaw/<name>/` directory also requires `--force`, so a failed run cannot silently overwrite data.

```bash
# Docker / CI
MYAAW_API_KEY="$KEY" myaaw onboard --yes --agent ci \
  --provider gemini --model models/gemini-2.5-flash --json
```

## Skills and built-in tools

Both lists come from the binary, so there is nothing to download.

- **Skills default to none.** Only the ones you select are copied to `~/.myaaw/<agent>/skills/<name>/`, and an unselected skill is not created at all. The folder is what enables a skill, so you can also add or remove skills later by copying a folder.
- **Tools default to all enabled.** Deselecting one writes `false` for it in `tools/tools.json` (`builtin_tools`); the rest stay `true`. Custom tools and MCP servers in the same file are preserved.
- On a reconfigure, the wizard pre-selects the skills already installed and the tools currently enabled, so pressing Enter through those steps keeps the current state.
- Headless updates behave the same way: omitting `--disable-tools` keeps the tools that are already disabled, while passing it explicitly (even empty) rewrites the set. Omitting `--skills` installs nothing and never removes an installed skill.
- Skills are never deleted by onboarding; removing a folder is still a manual action.

```bash
# headless: install one skill and disable bash
myaaw onboard --yes --agent agent-b --provider openai --model gpt-4o \
  --skills time --disable-tools bash
```

### Multiple skills or tools

Both flags accept a comma-separated list in any order; the list is normalised to the canonical order.

| Flag | Valid names |
| :--- | :--- |
| `--skills` | `calendar`, `cashflow`, `converter`, `notes`, `scraping`, `tavily`, `time`, `weather` |
| `--disable-tools` | `a2a_auth_request`, `a2a_input`, `bash`, `contact`, `cron`, `execute_python`, `filesystem`, `provider`, `subagent` |

```bash
# new agent: install three skills, keep every tool enabled
myaaw onboard --yes --agent agent-b \
  --provider gemini --model models/gemini-2.5-flash \
  --skills time,notes,calendar

# new agent: install all skills, disable three tools
myaaw onboard --yes --agent agent-c \
  --provider openai --model gpt-4o \
  --skills all \
  --disable-tools bash,cron,provider

# update an existing agent: add two skills, disable four tools
myaaw onboard --yes --agent agent-b --force \
  --provider gemini --model models/gemini-2.5-flash \
  --skills weather,tavily \
  --disable-tools bash,cron,provider,subagent
```

`--skills` and `--disable-tools` can be used independently, but headless mode always needs `--provider`:

```bash
# update an existing agent: only change the disabled tools
myaaw onboard --yes --agent agent-b --force \
  --provider gemini --model models/gemini-2.5-flash \
  --disable-tools bash

# re-enable every tool (explicit empty value)
myaaw onboard --yes --agent agent-b --force \
  --provider gemini --model models/gemini-2.5-flash \
  --disable-tools ""

# add the missing skill without touching the disabled tools
myaaw onboard --yes --agent agent-b --force \
  --provider gemini --model models/gemini-2.5-flash \
  --skills converter
```

Notes:

- Names are matched **case-insensitively**, and `python` is accepted as an alias for `execute_python`.
- Anything unknown is rejected before the agent is created or touched:

  ```text
  $ myaaw onboard --yes --agent agent-b --provider openai --skills skil-time
  Error: --skills: unknown name "skil-time" (valid: calendar, cashflow, converter, notes, scraping, tavily, time, weather)
  ```

  The command exits with status 1 and creates no directory, registry entry, or contacts. The same applies to `--disable-tools`.
- The interactive wizard cannot produce this error: skills and tools are toggled from the built-in list, not typed.
- Headless mode always requires `--provider`, and it rewrites the default provider entry with the values you pass. To keep the provider exactly as it is, use the interactive wizard and press Enter through the provider steps; there, skills and tools can be changed on their own.
- Adding a skill that is already installed is a no-op; existing folders are never overwritten.
- There is no flag to remove a skill. Delete `~/.myaaw/<agent>/skills/<name>/` manually, because onboarding will not delete user data.

## Files written

```text
~/.myaaw/<agent>/
  .env                    # PORT, MYAAW_BASE_URL, ALLOWED_ORIGINS, A2A_TOKEN, tuning keys
  config/config.json      # providers (base_url + default_model), channels, heartbeat, bot.owner_ids
  contacts.json           # A2A contacts for the other agents (never the agent itself)
  home/                   # AGENTS.md, SOUL.md, MEMORY.md, workspace
  skills/<name>/          # only the skills selected during onboarding
  tools/tools.json        # builtin_tools map (false = disabled)
  database/myaaw.db       # created on first use
  logs/
```

`PORT` is assigned once: the first agent (whatever its name) gets `8080`, the next ones get the lowest free port. Re-running onboarding keeps the existing port, so a running daemon is not moved. An explicit `--port` is rejected when another agent already owns it or something else is listening on it, and port assignment is serialized through `~/.myaaw/.ports.lock` so two onboarding runs cannot pick the same number.

## A2A wiring

Onboarding also joins the agent to the local A2A mesh, the same way `myaaw agent create` does:

- registers the agent in `agents.json` (and fixes the default when it pointed at a non-initialized folder),
- generates `A2A_TOKEN` in `.env` if it is missing, and never rotates an existing token,
- adds every other agent to this agent's `contacts.json`,
- adds this agent to every other agent's `contacts.json`,
- creates `contacts.json` even when this is the first agent,
- never writes the agent itself as a contact: self entries are skipped, and a stale self entry is pruned on the next run (`agent-b` and `Agent-B` count as the same agent).

If any of this fails, onboarding still completes and shows a warning (`A2A contacts were not seeded: ...`).

## Re-running onboarding

- Provider and model are replaced with the values you enter.
- Nothing is written until you save **Review**, so cancelling a reconfigure leaves the existing `config.json` and `.env` untouched.
- Channels and heartbeat are only rewritten when you configure them; choosing **Skip** leaves existing settings untouched, so a working Telegram bot is not disabled by accident.
- Values left over from older templates (`YOUR_*` placeholders) are removed before saving.
- If the agent daemon is running, the wizard offers to restart it at the end; `--restart` does the same in headless mode.
- A2A contacts are refreshed, so a newly added agent appears in the other agents' contact lists.

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
- A Telegram **polling** setup needs no public URL. **Webhook** mode asks for one during onboarding and registers it automatically; see [Webhook](webhook.md) to change it later.
- `myaaw voice` (real-time) requires a Gemini provider; `voice-classic` works with any provider.
- Each agent needs its own bot token if you run several agents.
- The install step copies the binary to `~/.local/bin`; if that directory is not in `PATH`, the wizard prints the line to add to your shell profile.
- The interactive wizard does not install automatically on Windows; copy the binary to a directory on `PATH` yourself. Headless `--install` attempts the copy and reports failures as a warning.

## Related

- [Agent Command](agent.md)
- [Multi-Agent Guide](../agents/multi-agent.md)
- [Service Management](service.md)
- [Providers](provider.md)
- [Channels](channel.md)
