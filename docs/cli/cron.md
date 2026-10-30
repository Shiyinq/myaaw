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
| `--channel` | yes | Delivery channel: `telegram` or `discord`. |
| `--to` | yes | Target chat/channel ID. |
| `--cron` | one schedule | Standard cron expression, for example `0 7 * * *`. |
| `--every` | one schedule | Interval, for example `30m` or `2h`. |
| `--at` | one schedule | Absolute time (`2026-10-27T10:00:00`) or relative (`10m`). |
| `--tz` | no | Timezone, for example `Asia/Jakarta`. |
| `--agent` | no | Job `AgentID` value (defaults to `main`). This is the job field, not the CLI target agent. |

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

- Jobs live in `~/.myaaw/<agent>/cron/jobs.json`.
- Run history is stored under `~/.myaaw/<agent>/cron/runs/` as JSONL files.
- The agent **server must be running** for jobs to execute. See [Service Management](service.md).
- A job runs an agent turn, then announces the result to the configured channel and target.
- `--at` with a relative duration is converted to an absolute timestamp at creation time.

## Related

- [Service Management](service.md)
- [Channels](channel.md)
- [Agent Loop](../agents/agent-loop.md)
