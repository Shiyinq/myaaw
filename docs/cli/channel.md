# Channels

Channels connect an agent to messaging platforms. Telegram and Discord are built in.

## Commands

```bash
myaaw channel list
myaaw channel create
myaaw channel edit
myaaw channel delete
```

All are interactive. Use `--agent` to target another agent:

```bash
myaaw --agent agent-b channel list
```

## `channel list`

Shows the active state for each channel:

```text
--- Configured Channels for agent "agent-b" ---
Telegram : Active (Mode: polling)
Discord  : Inactive
```

## `channel create`

Prompts for the channel type and its credentials.

**Telegram**

- Bot token from [@BotFather](https://t.me/BotFather).
- Mode:
  - `polling`: no public URL required; the agent calls Telegram.
  - `webhook`: requires a public HTTPS URL; use [Webhook](webhook.md) to register it.
- Owner/chat ID used for heartbeat and notifications.

**Discord**

- Bot token from the Discord developer portal.
- The bot must be invited to the server/channel you want to use.

## `channel edit`

Lets you change an existing channel's token or mode without deleting it.

## `channel delete`

Disables and removes the channel configuration. It does not delete the bot on the platform.

## Examples

```bash
# See what is active
myaaw --agent agent-b channel list

# Add Telegram in polling mode
myaaw --agent agent-b channel create

# Edit or remove
myaaw --agent agent-b channel edit
myaaw --agent agent-b channel delete
```

## Notes

- Channel tokens are stored in `~/.myaaw/<agent>/config/config.json`.
- Use a **different bot token for each agent**. Two agents cannot poll the same bot token reliably.
- Restart the agent server after changing channels so the listener is recreated:

  ```bash
  myaaw --agent agent-b server restart
  ```

- In webhook mode, the server registers `https://<your-domain>/webhook/telegram`. Use `myaaw webhook set <domain>` or configure it in onboarding.
- Discord runs through a gateway listener and does not need a public URL.
- `myaaw status` shows whether each channel is configured.

## Related

- [Webhook](webhook.md)
- [Service Management](service.md)
- [Multi-Agent Guide](../agents/multi-agent.md)
- [Utilities](utilities.md)
