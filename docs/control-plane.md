# Control Plane API Guide

`myaaw api` exposes one unified HTTP API for web, desktop, and mobile clients. A client only needs one base URL and one token; every operation is scoped to an agent through the path, so switching agents means switching the `:name` segment.

```text
GET  /agents                          → agent picker data
POST /agents/myfriend/chat            → chat with agent myfriend
GET  /agents/myfriend/fs?path=/home   → browse that agent's home
GET  /agents/myfriend/logs/stream     → tail that agent's logs
```

## 1. Runtime model

```text
myaaw server --agent default     → agent server :8080
myaaw server --agent myfriend    → agent server :8081
myaaw api start --port 9000     → control plane :9000 (daemon)
```

- Agent servers keep running chat, tools, SQLite, channels, and the agent loop.
- The control plane handles auth, management, filesystem, logs, and proxies chat to the agent server.
- Swagger UI: `http://localhost:9000/docs/`.

## 2. Quick start

```bash
# Start the agents you want to use
myaaw server --agent default start
myaaw server --agent myfriend start

# Start the unified API in the background
myaaw api start --port 9000
myaaw api status

# Or run it in the foreground
myaaw api run --port 9000

# Login (first run seeds these credentials)
curl -X POST http://localhost:9000/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"myaaw","password":"myaawadmin"}'
```

The API daemon uses:

```text
PID:  ~/.myaaw/pids/myaaw-api.pid
Log:  ~/.myaaw/logs/myaaw-api.log
```

Management commands: `myaaw api start`, `myaaw api stop`, `myaaw api restart`, `myaaw api status`.

Store the returned `token` and send it on every other request:

```bash
Authorization: Bearer <token>
```

Credentials live in `~/.myaaw/controlplane.json` with a bcrypt hash, never plaintext.

## 3. Authentication

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/auth/login` | `{username, password}` → `{token, expires_at}` |
| `GET` | `/auth/me` | current username |
| `POST` | `/auth/logout` | invalidate the current token |
| `POST` | `/auth/change-password` | `{current_password, new_password}` |
| `PUT` | `/auth/username` | `{password, new_username}` |

Tokens are valid for 24 hours. Changing the password invalidates all sessions. Restarting `myaaw api` also clears sessions. `/health` and `/docs/*` are public; every other route requires the bearer token.

## 4. Agents

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/agents` | list agents with `default`, `running`, `port`, `base_url` |
| `GET` | `/agents/:name` | one agent |
| `POST` | `/agents` | create `{name}` from the embedded template |
| `DELETE` | `/agents/:name` | delete agent data (default agent is protected) |
| `POST` | `/agents/:name/default` | set registry default |

A frontend keeps one `selectedAgent` value and prefixes every request:

```js
const api = (path) => `http://localhost:9000/agents/${selectedAgent}${path}`;
```

Creating an agent assigns the next free port, writes `.env`, creates the workspace, and registers it. The new agent still needs provider and channel configuration.

## 5. Chat

Chat is proxied to the selected agent server, so it uses that agent's config, SQLite, tools, and `home/`.

### Non-streaming

```bash
curl -X POST http://localhost:9000/agents/myfriend/chat \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"user_id":12345,"text":"Check my workspace"}'
```

Response:

```json
{
  "text": "There are 3 files.",
  "timeline": [
    {"kind": "thought", "text": "I should list the workspace first."},
    {"kind": "tool", "id": "call_1", "name": "bash", "arguments": "{\"command\":\"ls\"}", "result": "a.txt\nb.txt\nc.txt"},
    {"kind": "text", "text": "There are 3 files."}
  ],
  "usage": {"prompt_tokens": 120, "completion_tokens": 30, "total_tokens": 150}
}
```

- `text` is the authoritative final answer.
- `timeline` is the ordered thought/tool/text sequence; the last `text` item equals `text`.
- `tool_result` results are attached to their `tool` item.

### Streaming (SSE)

```bash
curl -N -X POST http://localhost:9000/agents/myfriend/chat/stream \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"user_id":12345,"text":"Check my workspace"}'
```

| Event | Data |
|---|---|
| `thought` | `{"text":"..."}` |
| `chunk` | `{"text":"..."}` (answer delta) |
| `tool` | `{"id","name","arguments"}` |
| `tool_result` | `{"id","name","result","is_error"}` |
| `done` | `{"ok":true,"text","usage"?}` |
| `error` | `{"error":"..."}` followed by `done {"ok":false}` |

Merge consecutive deltas per kind. On `done`, use `done.text` as the authoritative final answer; it can differ from the accumulated deltas when the run stops without normal output.

`EventSource` cannot send a POST body, so use `fetch` with `ReadableStream`:

```js
const response = await fetch(`http://localhost:9000/agents/${selectedAgent}/chat/stream`, {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
    "Authorization": `Bearer ${token}`,
  },
  body: JSON.stringify({ user_id: 12345, text: "Check my workspace" }),
});

const reader = response.body.getReader();
const decoder = new TextDecoder();
let buffer = "";

const handleEvent = (block) => {
  const lines = block.split(/\r?\n/);
  let event = "message";
  const data = [];
  for (const line of lines) {
    if (line.startsWith("event:")) event = line.slice(6).trim();
    else if (line.startsWith("data:")) data.push(line.slice(5).trimStart());
  }
  if (data.length > 0) console.log(event, JSON.parse(data.join("\n")));
};

while (true) {
  const { value, done } = await reader.read();
  if (done) break;
  buffer += decoder.decode(value, { stream: true });

  const blocks = buffer.split(/\r?\n\r?\n/);
  buffer = blocks.pop() ?? "";
  blocks.forEach(handleEvent);
}
buffer += decoder.decode();
if (buffer.trim() !== "") handleEvent(buffer);
```

## 6. Filesystem

Virtual roots are `/home`, `/skills`, and `/jobs`. Paths outside those roots, other agents, traversal, and symlink escapes are rejected.

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/agents/:name/fs?path=/home` | list a directory (`path` empty → the three roots) |
| `GET` | `/agents/:name/fs/read?path=/home/AGENTS.md` | read a text file |
| `POST` | `/agents/:name/fs/write` | write `{path, content}` |
| `POST` | `/agents/:name/fs/mkdir` | create `{path}` |
| `POST` | `/agents/:name/fs/rename` | `{from, to}` |
| `DELETE` | `/agents/:name/fs?path=...&recursive=true` | delete a file or directory |
| `GET` | `/agents/:name/fs/search?q=milk&path=/home` | search text |

Editing an agent's instructions:

```bash
# Read
curl "http://localhost:9000/agents/myfriend/fs/read?path=/home/AGENTS.md" \
  -H "Authorization: Bearer $TOKEN"

# Write
curl -X POST "http://localhost:9000/agents/myfriend/fs/write" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"path":"/home/AGENTS.md","content":"# Updated instructions\n"}'
```

Limits: reads and writes are capped at 2 MiB, binary files return `415`, and the three roots cannot be deleted or renamed.

## 7. Providers

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/agents/:name/providers` | list providers and the default |
| `POST` | `/agents/:name/providers` | create |
| `PUT` | `/agents/:name/providers/:id` | update fields |
| `DELETE` | `/agents/:name/providers/:id` | delete |
| `POST` | `/agents/:name/providers/default` | set default `{name}` |

```json
{
  "name": "openai",
  "type": "openai",
  "base_url": "https://api.openai.com",
  "api_key": "sk-...",
  "default_model": "gpt-4o"
}
```

Rules:

- API keys are write-only. Responses contain `{"configured": true, "masked": "********"}`.
- An empty `api_key` on update keeps the existing key.
- Deleting the default provider moves the default to another provider when one remains.

## 8. Channels

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/agents/:name/channels` | list Telegram/Discord status |
| `PUT` | `/agents/:name/channels/:channel` | create or update |
| `DELETE` | `/agents/:name/channels/:channel` | disable |

```json
{
  "active": true,
  "token": "BOT_TOKEN",
  "mode": "polling",
  "ngrok_active": false,
  "ngrok_auth_token": ""
}
```

- `channel` must be `telegram` or `discord`.
- Tokens are masked in responses; an empty token keeps the current one.
- `DELETE` disables the channel and keeps its token, matching the CLI.
- Restart the agent server after changing tokens.

## 9. Config

`GET /agents/:name/config` returns providers, channels, bot settings, heartbeat, cron, transcriber, and synthesizer with all secrets masked.

## 10. Conversations

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/agents/:name/conversations?user_id=&limit=` | recent conversations |
| `GET` | `/agents/:name/conversations/:id` | full transcript |

Conversations come from the agent's own SQLite database. Legacy traces are migrated on read. If the database does not exist yet, the list is empty.

## 11. Logs

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/agents/:name/logs` | list log files |
| `GET` | `/agents/:name/logs/stream?file=app.log` | tail over SSE |

Stream events:

```text
event: ready
data: {"file":"app.log"}

event: line
data: {"line":"..."}
```

The stream sends the last 64 KiB, then polls every 500 ms for new lines. File names are validated; paths are rejected.

`EventSource` cannot set the `Authorization` header. Use the `fetch` streaming pattern from the chat section, or proxy the request through a same-origin endpoint that injects the header.

## 12. Server control

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/agents/:name/server/status` | running, PID, port, base URL |
| `POST` | `/agents/:name/server/start` | start daemon |
| `POST` | `/agents/:name/server/stop` | stop daemon (idempotent) |
| `POST` | `/agents/:name/server/restart` | restart |

`start` returns `409` when the server is already running. Each daemon uses its own PID file and log directory.

## 13. Cron

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/agents/:name/cron` | list jobs |
| `POST` | `/agents/:name/cron` | create a job |
| `DELETE` | `/agents/:name/cron/:id` | delete (full ID or prefix) |
| `POST` | `/agents/:name/cron/:id/run` | run now via `/heartbeat` |
| `GET` | `/agents/:name/cron/:id/history?limit=` | execution history |

```json
{
  "name": "Morning reminder",
  "schedule_kind": "cron",
  "expr": "0 7 * * *",
  "tz": "Asia/Jakarta",
  "message": "Good morning!",
  "channel": "telegram",
  "to": "123456789",
  "enabled": true
}
```

`schedule_kind` is `cron`, `every`, or `at`. Running a job requires the agent server to be up; results are written to the job history.

## 14. Endpoint reference

| Group | Endpoints |
|---|---|
| Auth | `POST /auth/login`, `GET /auth/me`, `POST /auth/logout`, `POST /auth/change-password`, `PUT /auth/username` |
| Agents | `GET/POST /agents`, `GET/DELETE /agents/:name`, `POST /agents/:name/default` |
| Chat | `POST /agents/:name/chat`, `POST /agents/:name/chat/stream` |
| Files | `GET/DELETE /agents/:name/fs`, `GET /fs/read`, `GET /fs/search`, `POST /fs/write`, `POST /fs/mkdir`, `POST /fs/rename` |
| Providers | `GET/POST /providers`, `PUT/DELETE /providers/:id`, `POST /providers/default` |
| Channels | `GET /channels`, `PUT/DELETE /channels/:channel` |
| Config | `GET /config` |
| Conversations | `GET /conversations`, `GET /conversations/:id` |
| Logs | `GET /logs`, `GET /logs/stream` |
| Server | `GET /server/status`, `POST /server/start`, `POST /server/stop`, `POST /server/restart` |
| Cron | `GET/POST /cron`, `DELETE /cron/:id`, `POST /cron/:id/run`, `GET /cron/:id/history` |
| System | `GET /health`, `GET /docs/*` |

All `/agents/...` paths are prefixed with `/agents/:name`.

## 15. Errors

| Status | Meaning |
|---|---|
| `400` | invalid JSON, invalid path, or invalid config |
| `401` | missing, invalid, or expired token |
| `404` | agent, provider, channel, job, conversation, or log file not found |
| `409` | agent already exists, or server already running |
| `413` | file too large |
| `415` | binary file |
| `502` | agent server is not reachable |

## 16. Security notes

- There is no rate limiting in the first version.
- `/api/*` on the agent servers still has no auth; only the control plane protects access. Do not expose agent ports directly.
- Tools can run Bash, filesystem, and Python operations, so control-plane access is privileged.
- Keep `ALLOWED_ORIGINS` restrictive before serving a browser UI. The control plane reads it directly and allows `Authorization` in CORS preflight requests.
