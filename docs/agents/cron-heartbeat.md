# Cron and Heartbeat

How Myaaw runs work on a schedule and delivers it into the right conversation.

Both features share one endpoint and one target model. The difference is what
starts the run:

| Feature | Trigger | Configured in | Output |
| :--- | :--- | :--- | :--- |
| **Heartbeat** | fixed interval (`every`) | `config.json` → `heartbeat` | one message per tick, or silent on `HEARTBEAT_OK` |
| **Cron** | cron expression, interval, or one specific time | `jobs.json` (agent cron dir) | one message per run, recorded in cron history |

---

## 1. Heartbeat

`config.json`:

```json
"heartbeat": {
  "active": true,
  "every": "20m",
  "channel": "telegram",
  "to": "-1001234567890",
  "thread_id": "77",
  "session_id": "",
  "chat_type": ""
}
```

- `active` turns the ticker on; `every` is a Go duration (`20m`, `1h30m`).
- `thread_id` targets a Telegram forum topic, `session_id` pins an exact
  conversation, and `chat_type` (`dm`/`guild`) distinguishes a Discord DM from a
  channel. They follow the rules in §3.
- Every tick posts the built-in heartbeat prompt to `POST /heartbeat` with
  `trigger: "heartbeat"`.
- The prompt asks the model to read `HEARTBEAT.md`, do what is needed, and reply
  with exactly `HEARTBEAT_OK` when there is nothing to say. `HEARTBEAT_OK` is
  never stored and never delivered.
- The config is re-read on every tick and hot-reloaded when `config.json`
  changes.

## 2. Cron

Jobs live in `<agent>/cron/jobs.json`:

```json
{
  "id": "b946503f-…",
  "name": "Water reminder",
  "agent_id": "default",
  "schedule": { "kind": "at", "expr": "2026-11-04T16:50:48+07:00", "tz": "" },
  "payload": { "kind": "agentTurn", "content": "💧 Time to drink water!" },
  "delivery": {
    "mode": "announce",
    "channel": "api",
    "to": "111222333444555666",
    "thread_id": "",
    "session_id": "af53cc52-2196-42ae-bc00-9cde4a9a853c",
    "chat_type": ""
  },
  "enabled": true,
  "created_at": "2026-11-04T16:48:48+07:00",
  "updated_at": "2026-11-04T16:48:48+07:00"
}
```

| Field | Meaning |
| :--- | :--- |
| `schedule.kind` | `cron` (5-field expression), `every` (duration), or `at` (absolute time) |
| `schedule.tz` | timezone for a cron expression, e.g. `Asia/Jakarta` |
| `payload.content` | the prompt sent to the agent |
| `delivery` | where the answer goes (see §3) |
| `agent_id` | the agent that owns the job; must match the directory it lives in |

A relative `at` (for example `10m`) is converted to an absolute timestamp when
the job is created, so a reload does not restart the timer.

Jobs can be managed from three places:

- the agent's `cron` tool (the model schedules a reminder while chatting),
- `myaaw cron add/list/remove/run/history`,
- editing `jobs.json` directly (the scheduler watches the file).

---

## 3. Delivery target

A target resolves to exactly one conversation session.

| Surface | `channel` | `to` | `thread_id` | `session_id` | `chat_type` |
| :--- | :--- | :--- | :--- | :--- | :--- |
| Telegram DM | `telegram` | user id | — | optional | — |
| Telegram group | `telegram` | chat id (negative) | — | optional | — |
| Telegram forum topic | `telegram` | chat id | topic id | optional | — |
| Discord DM | `discord` | recipient user id | — | optional | `dm` |
| Discord channel | `discord` | channel id | — | optional | `guild` (default) |
| Discord thread | `discord` | thread channel id | — | optional | `guild` (default) |
| Web (API) | `api` | user id | — | **required** | — |
| Terminal (CLI) | `cli` | `0` or user id | — | **required** | — |

Rules:

- `to` is the platform id. A Telegram topic is **not** a standalone id: it needs
  both the chat id (`to`) and `message_thread_id` (`thread_id`).
- `session_id` pins the exact conversation and wins over scope. It is mandatory
  for `api`/`cli`, because a dashboard or terminal chat has no platform chat id.
  For Telegram/Discord it is optional and only needed to target a specific
  (for example older) conversation.
- `chat_type` exists only for Discord. A user id and a channel id are both
  17–19 digit snowflakes, and Discord needs a different API call for each: open a
  DM channel for a user, or post into a channel. `dm` means `to` is a recipient
  user id; `guild` (the default when omitted) means `to` is a channel or thread
  id. For a DM, `to` is the **user id**, not the DM channel id: the chat context
  shows both (`ChatID` is the DM channel). If the two are mixed up, the adapter
  looks the id up and delivers to the channel it finds, logging a warning.
- Telegram ignores `chat_type`: the room kind comes from the chat id (`-…` is a
  group, a positive id is a DM) and the topic from `thread_id`. The `cron` tool
  and `myaaw cron add` do not store it for Telegram, and a stale value in an old
  job is overridden at run time.
- Without `session_id`, the scope is derived exactly like an inbound message:
  `tg:<chat>`, `tg:<chat>:<thread>`, or `dc:<channel>`.

### Scheduling across chats

A job may target any chat on the same platform, not just the one it was created
in. The caller supplies the target ids, because a model cannot discover another
chat's id from the current conversation:

| From → to | What to send |
| :--- | :--- |
| Telegram DM → group | `channel: telegram`, `to: <group chat id>` |
| Telegram DM → forum topic | `channel: telegram`, `to: <group chat id>`, `thread_id: <topic id>` |
| Any chat → a specific older session | add `session_id` (Telegram/Discord) |
| Discord DM → guild channel/thread | `channel: discord`, `to: <channel or thread id>` |
| Discord DM → another user's DM | `channel: discord`, `to: <user id>`, `chat_type: dm` |
| Any chat → web/terminal session | `channel: api`/`cli`, `to: <user id>`, `session_id` (mandatory) |

Room fields are **not** inherited when the target is a different chat: the tool
only fills `thread_id`/`session_id`/`chat_type` from the current conversation
when the job addresses that same conversation. Without a pinned session the run
joins the target chat's active session, exactly like a live message.

### Unpinned runs and sessions

An empty `session_id` does **not** mean "start a new chat". The run resolves its
conversation the same way a live message does:

- the target scope already has an active conversation → the turn is appended to
  it (same history, no new session);
- the scope has no active conversation yet (a group, topic, or channel the bot
  has never talked in, or a user with no open DM) → **one new session is
  created** for that scope and reused from then on;
- `/new` rotates the active conversation for that scope, so later unpinned runs
  follow the newest one. Pin `session_id` only when a job must stay in one
  specific conversation.

The scope that is looked up is the one derived from the target: `tg:<chat>`,
`tg:<chat>:<thread>`, `dc:<channel>`, or the per-user scope for DMs. A Telegram
topic therefore has its own session, separate from the group's General topic,
and a Discord thread has its own, separate from the parent channel.

---

## 4. What happens during a scheduled run

```
scheduler / heartbeat tick
  → POST /heartbeat {prompt, channel, to, thread_id, session_id, chat_type, trigger}
  → lock the conversation (session id, or channel:to:thread)
  → ProcessHeartbeat resolves the session, owner, names, and RawMeta
  → the agent runs with the same system prompt and the <userinfo> block
  → the turn is saved into that conversation
  → publish conversation.updated (SSE subscribers re-read the chat)
  → the reply is delivered through the channel adapter
```

Details:

- **Session join.** The run writes into the target chat's conversation, so the
  agent sees the same history as a live message and the dashboard shows one
  session with the right badge. A new conversation is created only when the
  target has no active one yet (see §3).
- **Owner.** A pinned session keeps its owner. A DM belongs to the target user.
  A shared chat uses `bot.owner_ids[0]`, so no conversation row is created under
  a chat or channel id.
- **Names.** `chat_scopes` supplies the group/server and topic/channel names for
  the dashboard badge and `<userinfo>`.
- **Delivery.**
  - Telegram uses `chat id` + `message_thread_id`.
  - Discord uses the channel/thread id, or opens a DM channel for a user id.
  - API/CLI have no outbound adapter; the turn is saved in the pinned session
    and appears when the dashboard or terminal reads it.
- **Run log.** Each run appends one line to `cron/runs/<job id>.jsonl` with
  `ts`, `status`, `result`, and a snapshot of the job (`name`, `message`,
  `channel`, `to`, `thread_id`, `session_id`, `chat_type`). A one-time job is
  deleted right after it runs, so the snapshot is what lets
  `myaaw cron history` and the dashboard's **Recent runs** tab name the job and
  show where it went; entries written by older versions carry only the first
  three fields. `myaaw cron history <id>` resolves the id from the run files as
  well as `jobs.json`, so a removed job's runs stay readable. The control plane
  serves the log newest first in pages
  (`GET /agents/:name/cron/history?limit=&offset=`).
- **Trigger marker.** The turn is stored inside a model-only
  `<trigger kind="cron">` block (`internal/provider/triggers.go`): the rules come
  first, the scheduled content follows. The model reads the block as the reason it
  woke up; the dashboard and CLI hide it and show a badge instead — the badge is
  the whole message and cannot be expanded. A heartbeat has no content, so its
  tick prompt is joined to the rules inside the same block. Turns stored before
  the block existed have no marker and show as plain text.
- **Serialized with live chats.** Every turn takes the same per-conversation
  lock, whether it came from a chat request or a scheduled run. A cron that fires
  while a chat is still generating waits for that chat to finish, so the two
  turns cannot read the same transcript and overwrite each other.
- **Live update.** Once the turn is persisted the agent publishes
  `conversation.updated` (SSE, `GET /api/events`, proxied by the control plane at
  `GET /agents/:name/events`). Clients re-read the conversation instead of
  polling; `HEARTBEAT_OK` turns are not persisted, so they emit nothing.
- **Errors.** A failed delivery is logged; it never fails the run itself.
- **Heartbeat silence.** `HEARTBEAT_OK` is neither stored nor delivered.

---

## 5. Creating a job from a chat

The agent's `cron` tool defaults the delivery target to the **current session**,
so a simple request needs no ids:

> "remind me to eat in 5 minutes"

becomes a job with:

- `channel` = the current channel,
- `to` = the current chat id (or user id on DM/API/CLI),
- `thread_id` = the current topic (Telegram),
- `session_id` = the current conversation id,
- `chat_type` = the current room type (Discord only),
- `agent_id` = the active agent.

Cross-chat requests ("send it to the Telegram group", "greet the Discord
general channel") need the target id from the user, because the model cannot
know another chat's id from the current one. Give the ids and the job is
created with exactly those; the current chat's topic, session, and chat type are
not copied over (see §3).

`myaaw cron add` exposes the same fields:

```sh
myaaw cron add \
  --name "Water reminder" \
  --at 10m \
  --message "Time to drink water" \
  --channel api \
  --to 111222333444555666 \
  --session-id af53cc52-2196-42ae-bc00-9cde4a9a853c
```

`--agent` defaults to the active agent. Its value also decides where the job is
stored: `--agent myfriend` always writes `~/.myaaw/myfriend/cron/jobs.json`, never the
registry default's file, so `agent_id` and the directory cannot disagree.

The tool also reads back what it created. `list` with an `id` returns that job's
definition — schedule, prompt, and delivery target — and `list` alone keeps the
table. `history` on its own lists recent runs across every job, and `limit` pages
that list; `history` with a `job_id` returns that job's runs in full — time,
status, result, prompt, and delivery target — which is what lets the agent answer
"did the reminder go out, and where?". A job id is not paged, so `limit` is
dropped for it. All of it maps to `myaaw cron list` / `myaaw cron history`, so
the CLI and the agent never disagree about what a job or run says.

---

## 6. Multi-agent isolation

- Each agent has its own `cron/jobs.json` and its own scheduler that posts to its
  own server URL.
- The store directory and `agent_id` are resolved from the same value: the
  `--agent` flag when given, otherwise the process active agent
  (`agentdir.CronDir(agent)`). A job can never be written with an `agent_id` that
  does not match its directory.
- The `cron` tool binds its `myaaw cron` subprocess explicitly: it passes
  `--agent <active>` and exports `MYAAW_AGENT`, so the child cannot fall back to
  the registry default. `agentdir.SetActive` also exports `MYAAW_AGENT`, which
  covers any other tool that spawns `myaaw` or a shell command.
- A `session_id` is meaningful only inside the agent that owns the conversation.
  A job must never carry a session id from another agent: `ProcessHeartbeat`
  loads the conversation from this agent's database and rejects an unknown id.
- Heartbeat config is per agent (`<agent>/config/config.json`), so its target is
  always agent-local.

### Setting the heartbeat target

Interactive onboarding asks only for channel and destination, so it keeps any
saved `thread_id`/`session_id`/`chat_type` when the destination has not changed.
To set them explicitly:

```sh
myaaw onboard --agent default \
  --heartbeat-channel telegram \
  --heartbeat-to -1001234567890 \
  --heartbeat-thread 77 \
  --heartbeat-every 20m
```

`--heartbeat-session <conversation-id>` pins an exact conversation, and
`--heartbeat-chat-type dm` targets a Discord DM. The control-plane
`PATCH /agents/{name}/config` accepts the same three keys under `heartbeat`.

## 7. Compatibility

- Existing jobs/config without `thread_id`/`session_id`/`chat_type` keep working:
  the scope is derived from `channel` + `to`, and Telegram messages land in
  General as before.
- Older `jobs.json` files that recorded `agent_id: "main"` remain valid on load;
  new jobs use the real agent name.
- No database schema change is required.

## 8. Troubleshooting

| Symptom | Check |
| :--- | :--- |
| Reminder lands in General instead of a topic | `thread_id` is missing; set `delivery.thread_id` (Telegram). |
| Web/CLI reminder starts a new chat | `session_id` is missing; it is required for `api`/`cli`. |
| Reminder appears under a fake user | the target resolved no owner; check `bot.owner_ids`. |
| A scheduled run created a new session | the target scope had no active conversation yet (first message in that group, topic, or channel). Expected; later runs reuse it. |
| A scheduled run landed in the newest session after `/new` | the job has no `session_id`, so it follows the active session. Pin `session_id` to keep it in one conversation. |
| Job runs but no message | `HEARTBEAT_OK` was returned, the channel has no adapter (api/cli), or delivery failed (check the log). |
| `agent_id` is `main` in `jobs.json` | older job written before the active-agent fix; recreate or edit it. |
| Job runs on the wrong agent | `agent_id`/directory mismatch; recreate from the correct agent. Jobs created by a server that was running an older binary may sit in another agent's `jobs.json`; recreate them after restarting the agent. |
