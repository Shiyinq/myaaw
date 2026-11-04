# Cron

`myaaw cron` manages scheduled agent turns. Jobs are stored per agent and executed by the agent server.

## Commands

```bash
myaaw cron list
myaaw cron add ...
myaaw cron remove <id>
myaaw cron run <id>
myaaw cron history [id]
```

Use the global `--agent` flag to manage another agent's jobs:

```bash
myaaw --agent agent-b cron list
```

## `cron list`

Prints every job with its ID, schedule, target channel, and status.

## `cron add`

```bash
myaaw cron add \
  --name "Morning greeting" \
  --cron "0 7 * * *" \
  --message "Selamat pagi!" \
  --channel telegram \
  --to 123456789
```

| Flag | Required | Description |
| :--- | :--- | :--- |
| `--name` | yes | Job name. |
| `--message` | yes | Prompt sent to the agent when the job runs. |
| `--channel` | yes | Delivery channel: `telegram`, `discord`, `api`, or `cli`. |
| `--to` | yes | Target id: user id (`api`/`cli`/Telegram DM), chat id (Telegram), or channel id (Discord). |
| `--thread-id` | no | Telegram forum topic (`message_thread_id`). A topic needs both `--to` (chat id) and this. |
| `--session-id` | no | Pin the exact conversation id. Required for `api`/`cli`, optional for Telegram/Discord. |
| `--chat-type` | no | `dm` or `guild` (Discord), to tell a DM from a channel. |
| `--cron` | one schedule | Standard cron expression, for example `0 7 * * *`. |
| `--every` | one schedule | Interval, for example `30m` or `2h`. |
| `--at` | one schedule | Absolute time (`2026-10-27T10:00:00`) or relative (`10m`). |
| `--tz` | no | Timezone, for example `Asia/Jakarta`. |
| `--agent` | no | Agent that owns the job (defaults to the active agent). It also selects the store, so `--agent myfriend` always writes `~/.myaaw/myfriend/cron/jobs.json` and sets `agent_id` to `myfriend`. |

Exactly one of `--cron`, `--every`, or `--at` must be provided. The command prints the job ID after creation.

Examples:

```bash
# Every 2 hours
myaaw cron add --name "Hydration" --every "2h" --message "Drink water!" \
  --channel telegram --to 123456789

# One-time reminder in 30 minutes
myaaw cron add --name "Meeting" --at "30m" --message "Meeting starts soon" \
  --channel telegram --to 123456789

# Daily at 07:00 Jakarta time
myaaw cron add --name "Morning" --cron "0 7 * * *" --tz "Asia/Jakarta" \
  --message "Selamat pagi!" --channel telegram --to 123456789

# Into a Telegram forum topic
myaaw cron add --name "Standup" --cron "0 9 * * 1-5" \
  --message "Standup time" --channel telegram --to -1001234567890 --thread-id 77

# Into an exact web conversation
myaaw cron add --name "Review" --at "1h" --message "Review the draft" \
  --channel api --to 111222333444555666 --session-id 0f4e8b3a-1111-4222-8333-944455556666
```

## `cron remove <id>`

Deletes the job. Copy the ID from `cron list`.

## `cron run <id>`

Triggers the job immediately without changing its schedule.

## `cron history [id]`

Shows recent executions. Without an ID, shows history across jobs.

```bash
myaaw cron history
myaaw cron history <job-id> --limit 50
```

| Flag | Default | Description |
| :--- | :--- | :--- |
| `--limit` | `10` | Number of entries to show. |

## Behavior

- Jobs live in `~/.myaaw/<agent>/cron/jobs.json`; `--agent` decides both the directory and the job's `agent_id`.
- Run history is stored under `~/.myaaw/<agent>/cron/runs/` as JSONL files.
- The agent **server must be running** for jobs to execute. See [Service Management](service.md).
- A job runs an agent turn, then announces the result to the configured channel and target.
- `api`/`cli` targets have no outbound adapter: the turn is saved in the pinned session and appears the next time the dashboard or terminal reads it, so they require `--session-id`.
- `--at` with a relative duration is converted to an absolute timestamp at creation time.

## Related

- [Service Management](service.md)
- [Channels](channel.md)
- [Agent Loop](../agents/agent-loop.md)
