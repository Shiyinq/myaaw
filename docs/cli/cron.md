# Cron

`myaaw cron` manages scheduled agent turns. Jobs are stored per agent and executed by the agent server.

## Commands

```bash
myaaw cron list [id]
myaaw cron add ...
myaaw cron remove <id>
myaaw cron run <id>
myaaw cron history [id]
```

Use the global `--agent` flag to manage another agent's jobs:

```bash
myaaw --agent agent-b cron list
```

## `cron list [id]`

Without an id it prints every job with its ID, schedule, target channel, and
status. Pass a job id (or its prefix) to read every field the job stores, one per
line:

```bash
myaaw cron list 33333333
```

```text
ID        33333333-4444-4555-8666-777777777777
Name      Water reminder
Agent     default
Schedule  at · 2026-11-05T15:13:10+07:00
Payload   agentTurn
Prompt    Time to drink water
Mode      announce
Channel   telegram
To        -1001234567890
Topic     77
Session   0f4e8b3a-1111-4222-8333-944455556666
Status    enabled
Created   2026-11-05T14:13:00+07:00
Updated   2026-11-05T14:13:00+07:00
```

The timestamps are printed as stored. Empty fields are left out, so a target
without a topic simply has no `Topic` line, and `Chat type` only appears for a
Discord job. A prefix that matches more than one job lists the candidates instead
of guessing, and a job whose runs you want to read is `cron history <id>`.

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
| `--to` | yes | Target id: user id (`api`/`cli`/Discord DM), chat id (Telegram), or channel id (Discord guild or thread). |
| `--thread-id` | no | Telegram forum topic (`message_thread_id`). A topic needs both `--to` (chat id) and this. |
| `--session-id` | no | Pin the exact conversation id. Required for `api`/`cli`, optional for Telegram/Discord; `cron add` rejects `api`/`cli` without it. |
| `--chat-type` | no | `dm` or `guild` (Discord only). Tells a DM recipient user id from a guild channel id; ignored for other channels. |
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

Lists recent runs, newest first. Without an id it covers every job, including
one-time jobs that were removed after they ran.

```bash
myaaw cron history
myaaw cron history <job-id>
myaaw cron history --limit 50
```

| Flag | Default | Description |
| :--- | :--- | :--- |
| `--limit` | `10` | Number of runs in the list. A job id ignores it: that job's runs are always shown in full. |

Without an id the output is one row per run: time, short job id, job name,
status, and result. Pass a job id (or a prefix) to read every field the run
stores, one per line:

```text
History for job: Water reminder (11111111-2222-4333-8444-555555555555)
Time       2026-11-05T13:18:52.513049+07:00
Job ID     11111111-2222-4333-8444-555555555555
Job        Water reminder
Status     success
Result     Job executed successfully
Prompt     Time to drink water.
Channel    telegram
To         -1001234567890
Topic      77
Session    0f4e8b3a-1111-4222-8333-944455556666
```

The block prints the timestamp exactly as the log stores it (RFC3339 with its
original offset), so it can be matched to a line in `cron/runs/<job id>.jsonl`.
Runs recorded before the log kept a job snapshot only show the time, status, and
result, with a note saying so.

An id still resolves after the job is gone: a one-time job is removed once it
runs, but its run file stays, and the id is matched against the log as well as
`jobs.json`.

The agent's `cron` tool has the same split: `history` alone lists runs, and
`history` with a `job_id` returns that job's runs in full. `limit` pages the list
only; the tool never sends it alongside a `job_id`.

## Behavior

- Jobs live in `~/.myaaw/<agent>/cron/jobs.json`; `--agent` decides both the directory and the job's `agent_id`.
- Run history is stored under `~/.myaaw/<agent>/cron/runs/` as JSONL files, one line per run: `ts`, `status`, `result`, and a snapshot of the job (name, prompt, channel, target, topic, session). The snapshot keeps the history readable after a one-time job is removed.
- The agent **server must be running** for jobs to execute. See [Service Management](service.md).
- A job runs an agent turn, then announces the result to the configured channel and target.
- A job may target another chat than the one it was created in; pass that chat's ids. `--chat-type` is only written for Discord.
- Without `--session-id` on Telegram/Discord, the job joins the target room's **active** session (a new one is created only if that room has none) and follows `/new` rotations. See [Cron & Heartbeat](../agents/cron-heartbeat.md#unpinned-runs-and-sessions).
- `api`/`cli` targets have no outbound adapter: the turn is saved in the pinned session and appears the next time the dashboard or terminal reads it, so they require `--session-id`.
- `--at` with a relative duration is converted to an absolute timestamp at creation time.

## Related

- [Service Management](service.md)
- [Channels](channel.md)
- [Agent Loop](../agents/agent-loop.md)
