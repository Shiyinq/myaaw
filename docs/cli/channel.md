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
- **`telegram_offset` is the bot's bookmark in Telegram's update stream.** Telegram gives every event it delivers (a message, an edit, a button press) an increasing `update_id`, shared across all users of one bot. While polling, the agent asks for updates from a given position, and asking from a higher position also confirms everything before it. The file keeps that position (`update_id + 1`) in `~/.myaaw/<agent>/state/telegram_offset`, so after a restart the bot continues where it left off instead of fetching and answering updates it already handled. It holds a single number: no message content, no chat ID, no token.
- **Deleting `telegram_offset` resets the bookmark to zero.** Telegram only keeps updates it has not confirmed yet (and never for more than 24 hours), so a normal restart re-fetches nothing. If the agent crashed with an update still pending, that update is delivered again and produces a duplicate answer. Do not edit the number by hand: a larger value skips updates, a smaller one replays them.
- **Discord does not replay messages sent while the agent was down.** The gateway session can only be resumed while the process is alive; after a restart, messages sent during the downtime are gone and have to be re-sent by the user. The transcript of a turn that was already running is still saved, so the conversation keeps its context.

## Message processing

Inbound Telegram and Discord messages go through an in-memory queue before the agent sees them:

- **Concurrency:** `bot.queue_workers` in `config.json` (or `QUEUE_WORKERS` in `.env`) sets how many chats run at the same time. Default `4`, range `1–16`.
- **Ordering:** messages from the same user are always processed in order, one at a time. Different users run in parallel, so one long answer no longer blocks everybody else.
- **Restart required:** the worker pool is created at startup. After changing the value, run `myaaw server restart`.
- **Heartbeat and cron** do not use this queue; they run independently.
- **The queue is in memory** (buffer of 1000). Messages still waiting when the process dies are lost, and Telegram's polling offset is already advanced by then, so they are not re-delivered. The transcript of a turn that already started is still checkpointed to the conversation.

Setting `queue_workers` to `1` restores the old serial behavior: every user waits for the previous message, in arrival order.

## Related

- [Webhook](webhook.md)
- [Service Management](service.md)
- [Multi-Agent Guide](../agents/multi-agent.md)
- [Utilities](utilities.md)
