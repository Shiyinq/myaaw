# Cron Tool

The Cron tool allows the agent to managing scheduled jobs within the Myaaw system. It supports listing, adding, removing, running, and viewing execution history of jobs.

## Function Name

`cron`

## Parameters

| Parameter | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `action` | `string` | **Yes** | The action to perform: `list`, `add`, `remove`, `run`, `history`. |
| `name` | `string` | Yes (for `add`) | Name of the job. |
| `cron` | `string` | No | Cron expression (e.g., `0 7 * * *`). |
| `every` | `string` | No | Interval duration (e.g., `1h30m`). |
| `at` | `string` | No | Specific time or delay (e.g., `10m` or `2023-01-01T10:00:00`). |
| `message` | `string` | Yes (for `add`) | The content/prompt to send. |
| `channel` | `string` | Yes (for `add`) | Target channel (`telegram`, `discord`, `terminal`). |
| `to` | `string` | Yes (for `add`) | Target recipient ID (User ID from the related channel). |
| `tz` | `string` | No | Timezone (e.g., `Asia/Jakarta`). |
| `agent` | `string` | No | Agent ID to handle the job (default: `main`). |
| `id` | `string` | Yes (for `remove`, `run`) | The Job ID. |
| `job_id` | `string` | No (for `history`) | The Job ID to limit history to specific job. |
| `limit` | `number` | No (for `history`) | Limit number of history entries (default: 10). |

**Note:** For `add` action, you must provide exactly one of `cron`, `every`, or `at`.

## Usage Examples

### List Jobs

```json
{
  "action": "list"
}
```

### Add a Recurring Job (Cron)

Schedule a morning greeting every day at 7 AM Jakarta time.

```json
{
  "action": "add",
  "name": "Morning Greeting",
  "cron": "0 7 * * *",
  "message": "Selamat pagi! Tolong cek agenda hari ini.",
  "channel": "telegram",
  "to": "123456789",
  "tz": "Asia/Jakarta"
}
```

### Add an Interval Job

Run a check every 2 hours.

```json
{
  "action": "add",
  "name": "System Check",
  "every": "2h",
  "message": "Check system status",
  "channel": "telegram",
  "to": "123456789"
}
```

### Add a One-time Job

Remind me in 30 minutes.

```json
{
  "action": "add",
  "name": "Reminder",
  "at": "30m",
  "message": "Time to take a break!",
  "channel": "telegram",
  "to": "123456789"
}
```

### Remove a Job

```json
{
  "action": "remove",
  "id": "job-123456"
}
```

### Run a Job Manually

Trigger a job execution immediately.

```json
{
  "action": "run",
  "id": "job-123456"
}
```

### View History

View global history (last 5 runs):

```json
{
  "action": "history",
  "limit": 5
}
```

View history for specific job:

```json
{
  "action": "history",
  "job_id": "job-123456",
  "limit": 10
}
```
