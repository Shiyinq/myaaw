# Webhook

`myaaw webhook` manages the Telegram bot webhook. It is only meaningful when the agent's Telegram mode is `webhook`.

## Commands

```bash
myaaw webhook info
myaaw webhook set [domain]
myaaw webhook delete
```

Use `--agent` if the Telegram token belongs to another agent:

```bash
myaaw --agent agent-b webhook info
```

## `webhook info`

Calls Telegram's `getWebhookInfo` and prints the current URL, pending updates, and last error. Requires a configured Telegram bot token.

## `webhook set [domain]`

Registers:

```text
https://<domain>/webhook/telegram
```

- With an argument:

  ```bash
  myaaw webhook set https://agent-b.example.com
  ```

- Without an argument, Myaaw prompts for the domain or ngrok URL.
- Trailing slashes are trimmed before `/webhook/telegram` is appended.
- The URL must use `https://`; Telegram rejects plain HTTP and raw IPs.

You usually do not need this by hand: `myaaw onboard` asks for the public URL when Telegram mode is `webhook` and registers it right after saving. Use `webhook set` to change it later.

Telegram requires a public **HTTPS** URL with a valid certificate. A raw IP or plain HTTP URL will be rejected by Telegram.

## `webhook delete`

Removes the webhook from Telegram. Useful before switching the agent back to polling mode.

## Mode interaction

- `channel create` or `channel edit` selects `polling` or `webhook`.
- In **polling** mode, the server clears any existing webhook on startup so polling can work.
- In **webhook** mode, the server expects incoming POSTs at `/webhook/telegram`. Onboarding registers the URL for you when you provide one; otherwise run `webhook set`.
- On startup the server itself only calls `setWebhook` when `NGROK_ACTIVE=true` and `NGROK_AUTH_TOKEN` are set, because it needs a public URL to advertise.
- Do not use both modes for the same bot token at once.

## Examples

```bash
# Check what Telegram currently has
myaaw --agent agent-b webhook info

# Point the bot at a public domain
myaaw --agent agent-b webhook set https://agent-b.example.com

# Remove it and switch back to polling
myaaw --agent agent-b webhook delete
myaaw --agent agent-b channel edit
myaaw --agent agent-b server restart
```

## Notes

- The command uses the agent's Telegram bot token from `.env` / config. If the token is missing, it prints an error and exits.
- Webhook changes take effect immediately on Telegram's side; restart the agent server only when you also changed the channel mode.
- No effect on Discord.

## Related

- [Channels](channel.md)
- [Service Management](service.md)
- [Config](config.md)
