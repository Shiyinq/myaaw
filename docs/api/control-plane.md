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

Management commands: `myaaw api start`, `myaaw api stop`, `myaaw api restart`, `myaaw api status`. Running `myaaw api` without a subcommand is equivalent to `myaaw api run` and starts the API in the foreground.

Store the returned `token` and send it on every other request:

```bash
Authorization: Bearer <token>
```

Credentials live in `~/.myaaw/controlplane.json` with a bcrypt hash and the agent `user_id`, never plaintext:

```json
{
  "username": "myaaw",
  "password_hash": "$2a$...",
  "user_id": 0,
  "created_at": "2026-10-20T00:00:00Z"
}
```

New installs seed `user_id` as `0`. Existing credential files without the field are rewritten with `user_id: 0` the next time `myaaw api` starts. Edit the value and restart the API to change which agent user chat and conversation requests use. Clients do not need to send `user_id`.

## 3. Authentication

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/auth/login` | `{username, password}` → `{token, expires_at, user_id}` |
| `GET` | `/auth/me` | current `{username, user_id}` |
| `POST` | `/auth/logout` | invalidate the current token |
| `POST` | `/auth/change-password` | `{current_password, new_password}` |
| `PUT` | `/auth/username` | `{password, new_username}` |

Tokens are valid for 24 hours. Changing the password invalidates all sessions. Restarting `myaaw api` also clears sessions. `/health` and `/docs/*` are public; every other route requires the bearer token.

The login response includes `user_id` (default `0`):

```json
{
  "token": "...",
  "expires_at": "2026-10-20T20:00:00Z",
  "user_id": 0
}
```

Chat and conversation endpoints use this ID automatically. Clients do not need to send `user_id` at all.

## 4. Agents

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/agents` | list agents with `default`, `running`, `port`, `base_url` |
| `GET` | `/agents/:name` | one agent |
| `POST` | `/agents/:name/onboard` | create or reconfigure an agent with the full setup in one call |
| `DELETE` | `/agents/:name` | delete agent data (default agent is protected) |
| `POST` | `/agents/:name/default` | set registry default |

A frontend keeps one `selectedAgent` value and prefixes every request:

```js
const api = (path) => `http://localhost:9000/agents/${selectedAgent}${path}`;
```

Creating an agent assigns the next free port, writes `.env`, creates the workspace, and registers it. The control plane has no separate create call: `POST /agents/:name/onboard` is the only way to add an agent, so a dashboard never has to chain create + configure.

`POST /agents/:name/onboard` is the dashboard-friendly path: it **creates the agent when the name is free and reconfigures it otherwise**, using the same core as `myaaw onboard` (layout, port, A2A contacts, config, skills, tools, webhook). Omitting a section keeps its current value; on a new agent the omitted parts fall back to the template defaults. Omitted `disable_tools` leaves `tools/tools.json` untouched, and skills are only ever installed, never deleted.

```bash
curl -X POST http://localhost:9000/agents/myfriend/onboard \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{
    "provider": {"type":"openai","name":"deepseek","base_url":"https://api.deepseek.com","api_key":"sk-...","model":"deepseek-chat"},
    "skills": ["time", "notes"],
    "disable_tools": ["bash"],
    "channels": {"telegram_token":"123:abc","telegram_mode":"polling"},
    "heartbeat": {"active":true,"channel":"telegram","to":"123456789","every":"30m"},
    "bot": {"type":"private","owner_ids":["123456789"]}
  }'
```

`201` means the agent was created, `200` means it was reconfigured. The body is `{created, config, warnings}`; `warnings` carries non-fatal issues such as a failed Telegram webhook registration.

## 5. Chat

Chat is proxied to the selected agent server, so it uses that agent's config, SQLite, tools, and `home/`.

Both endpoints record one trace run per message (channel `api`) when
[observability](../agents/observability.md) is enabled on the agent. Read the
traces through `/agents/:name/observability/runs`.

### Non-streaming

```bash
curl -X POST http://localhost:9000/agents/myfriend/chat \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"text":"Check my workspace"}'
```

`user_id` is ignored in the request body. The control plane always sends the ID from `~/.myaaw/controlplane.json` (default `0`), so chat and conversation endpoints stay in the same scope. Direct calls to the agent server's `/api/chat` still default a missing `user_id` to `0`.

To continue an existing conversation, pass its `conversation_id`. Omit it to start a **new conversation** every time; `"NEW"` also forces a new conversation. An explicit ID that does not exist returns `404`; an ID owned by another user returns `403`.

```json
{
  "text": "And the downloads folder?",
  "conversation_id": "conv-id-from-a-previous-response"
}
```

Response:

```json
{
  "text": "There are 3 files.",
  "conversation_id": "conv-id",
  "run_id": "run_2240477b6447ff48",
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
- A retried request adds a `{"kind":"retry","attempt":2}` item; a trimmed transcript adds `{"kind":"compaction","tokens_before","tokens_after","dropped_messages"}`.
- `run_id` is the recorded trace; `GET /agents/:name/observability/runs/:id` reads it. It is absent when observability is disabled, and the streaming `done` event carries the same field.

### Streaming (SSE)

```bash
curl -N -X POST http://localhost:9000/agents/myfriend/chat/stream \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"text":"Check my workspace"}'
```

| Event | Data |
|---|---|
| `thought` | `{"text":"..."}` |
| `chunk` | `{"text":"..."}` (answer delta) |
| `tool` | `{"id","name","arguments"}` |
| `tool_result` | `{"id","name","result","is_error"}` |
| `retry` | `{"attempt":2}` — a provider request was retried |
| `approval_required` | `{"approval_id","tool_name","action","args_summary","subagent"?}` — a gated tool is waiting; `subagent` names the child task when relevant |
| `approval_resolved` | `{"approval_id","decision","reason"}` |
| `compaction` | `{"tokens_before","tokens_after","dropped_messages"}` — the transcript was trimmed |
| `done` | `{"ok":true,"text","conversation_id"?,"run_id"?,"usage"?}` |
| `error` | `{"error":"..."}` followed by `done {"ok":false}` |

Merge consecutive deltas per kind. On `done`, use `done.text` as the authoritative final answer; it can differ from the accumulated deltas when the run stops without normal output.

For streaming requests, an invalid `conversation_id` is reported as an `error` event because the HTTP status is already `200` once the stream starts.

`EventSource` cannot send a POST body, so use `fetch` with `ReadableStream`:

```js
const response = await fetch(`http://localhost:9000/agents/${selectedAgent}/chat/stream`, {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
    "Authorization": `Bearer ${token}`,
  },
  body: JSON.stringify({ text: "Check my workspace" }),
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

### Tool approvals

Approval prompts ride the same run. Streaming chat emits `approval_required`
and `approval_resolved`; non-streaming chat returns
`{"status":"approval_required","approval":{...}}`. Answer with:

```bash
curl -X POST http://localhost:9000/agents/myfriend/approvals/<id>/decision \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"decision":"allow_once"}'
```

Management endpoints:

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/agents/:name/approvals` | effective policy + grants |
| `PATCH` | `/agents/:name/config` | update the `approvals` policy (validated) |
| `DELETE` | `/agents/:name/approvals/grants/:key` | revoke every grant for a key |
| `DELETE` | `/agents/:name/approvals/grants/:key/sessions/:conversation` | revoke one session grant |
| `DELETE` | `/agents/:name/approvals/grants` | clear all grants |

See [Tool Approvals](../agents/approvals.md) for fields, grant scopes, and per-channel behaviour.

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

### Context files

Dedicated endpoints for the agent's identity and memory files. The only allowed root files are:

```text
AGENTS.md  HEARTBEAT.md  IDENTITY.md  MEMORY.md  SOUL.md  TOOLS.md  USER.md
```

Any `.md` file under `home/memory/` (including nested folders) is also allowed. Other `.md` files, such as `BOOTSTRAP.md`, `NOTES.md`, `workspace/README.md`, or `notes/todo.md`, are never listed and cannot be read or updated through this API. Use `/fs` for arbitrary files.

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/agents/:name/context` | list allowed context files |
| `GET` | `/agents/:name/context/file?path=AGENTS.md` | read one file |
| `PUT` | `/agents/:name/context/file` | create or update `{path, content}` |

```bash
# List the allowlisted root files plus every .md file under memory/
curl "http://localhost:9000/agents/myfriend/context" \
  -H "Authorization: Bearer $TOKEN"

# Read memory/RULES.md
curl "http://localhost:9000/agents/myfriend/context/file?path=memory/RULES.md" \
  -H "Authorization: Bearer $TOKEN"

# Update it (missing parent folders under memory/ are created)
curl -X PUT "http://localhost:9000/agents/myfriend/context/file" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"path":"memory/RULES.md","content":"# Updated rules\n"}'
```

List response:

```json
{
  "files": [
    {"name": "AGENTS.md", "path": "AGENTS.md", "folder": "root", "size": 1234, "mod_time": "2026-10-20T00:00:00Z"},
    {"name": "RULES.md", "path": "memory/RULES.md", "folder": "memory", "size": 456, "mod_time": "2026-10-20T00:00:00Z"}
  ]
}
```

Read and write responses add `content`:

```json
{
  "name": "RULES.md",
  "path": "memory/RULES.md",
  "folder": "memory",
  "content": "# Updated rules\n",
  "size": 16,
  "mod_time": "2026-10-20T00:00:00Z"
}
```

Path rules:

- Root files must be one of the seven allowlisted names, exactly.
- `memory/2026/notes.md` is valid; nested memory folders are supported.
- Any other root `.md` file, non-`.md` files, traversal, and absolute paths return `400`.
- A missing allowlisted root file returns `404`; it can still be created with `PUT`.
- Writes create missing parent folders under `memory/` and overwrite existing files.

The same 2 MiB limit applies. Reads still reject binary content with `415`.

## 7. Skills

Skills live in `~/.myaaw/<agent>/skills/<skill>/SKILL.md`. The enabled state follows `skills.json`, then the `SKILL.md` frontmatter, then defaults to enabled. The list is read fresh from disk on every request, so skills added or removed through the terminal appear immediately.

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/agents/:name/skills` | list every skill directory with its state |
| `GET` | `/agents/:name/skills/:skill` | skill detail: `SKILL.md` content + file listing |
| `GET` | `/agents/:name/skills/:skill/file?path=scripts/weather.py` | read one file in the skill (base64 for binary) |
| `PATCH` | `/agents/:name/skills/:skill` | `{enabled}` → writes `skills.json` |
| `DELETE` | `/agents/:name/skills/:skill` | remove the skill folder and its `skills.json` key (idempotent) |
| `DELETE` | `/agents/:name/skills/:skill/config` | remove only the `skills.json` override (folder untouched, idempotent) |

```bash
curl "http://localhost:9000/agents/myfriend/skills" \
  -H "Authorization: Bearer $TOKEN"

curl -X PATCH "http://localhost:9000/agents/myfriend/skills/weather" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"enabled": false}'

curl -X DELETE "http://localhost:9000/agents/myfriend/skills/weather" \
  -H "Authorization: Bearer $TOKEN"
```

List response:

```json
{
  "skills": [
    {"name": "weather", "display_name": "Weather", "description": "Fetch weather", "enabled": false, "source": "config", "parse_ok": true}
  ]
}
```

- `source` is `config` (skills.json), `frontmatter`, or `default`.
- A directory without a parseable `SKILL.md` still appears with `parse_ok: false` so it can be deleted.
- There is no create endpoint: the agent creates skills itself; the list picks them up automatically.
- Empty or missing directories return an empty list. A missing or invalid `skills.json` behaves like the agent loader: everything defaults to enabled, and the next write replaces the file.
- A stale `skills.json` key left behind by a manual folder deletion is ignored by the list (no ghost skills) and can be cleaned with `DELETE`, which succeeds as long as the folder or the key exists.
- `DELETE /skills/:skill/config` clears a toggle written by `PATCH` without deleting the skill. After that, the enabled state falls back to the `SKILL.md` frontmatter `enabled` field, or to the default (enabled) when the frontmatter does not specify one. The skill folder must exist; a missing key still returns `200`.

Skill detail:

```bash
curl "http://localhost:9000/agents/myfriend/skills/weather" \
  -H "Authorization: Bearer $TOKEN"
```

```json
{
  "name": "weather",
  "display_name": "Weather",
  "description": "Fetch weather",
  "enabled": true,
  "source": "default",
  "parse_ok": true,
  "content": "---\nname: Weather\n...\n---\n",
  "files": [
    {"path": "SKILL.md", "size": 123, "mod_time": "..."},
    {"path": "assets/icon.png", "size": 456, "mod_time": "..."},
    {"path": "scripts/weather.py", "size": 789, "mod_time": "..."}
  ]
}
```

- `content` is the raw `SKILL.md` text (capped at 2 MiB, `413` when larger).
- `files` lists every file in the skill directory recursively — scripts, images, and nested folders — sorted by path, capped at 1000 entries. Symlinks are skipped.
- Requesting a deleted skill cleans its stale `skills.json` key automatically and returns `404`.

Reading a file inside a skill:

```bash
curl "http://localhost:9000/agents/myfriend/skills/weather/file?path=scripts/weather.py" \
  -H "Authorization: Bearer $TOKEN"
```

```json
{
  "path": "scripts/weather.py",
  "size": 789,
  "mod_time": "...",
  "binary": false,
  "content": "print('hi')"
}
```

Text files return as plain text. Binary files (images, compiled scripts) return `"binary": true` with base64 content. Paths are relative to the skill directory, capped at 2 MiB (`413`), and cannot escape the skill (`400`).

## 8. Tools

Tools are configured in `~/.myaaw/<agent>/tools/tools.json` with three sections: `builtin_tools`, `custom_tools`, and `mcp_servers`. This API manages only the `tools.json` entries, and their command may point anywhere on disk. Note: the agent server also has a zero-config loader that auto-registers executable files placed directly in the tools folder; those files are not listed or managed by this API. The agent server hot-reloads `tools.json` via a file watcher.

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/agents/:name/tools` | built-in + custom + MCP tools |
| `POST` | `/agents/:name/tools/custom` | register a custom tool |
| `POST` | `/agents/:name/tools/mcp` | register an MCP server |
| `PATCH` | `/agents/:name/tools/builtin/:tool` | `{enabled}` only |
| `PATCH` | `/agents/:name/tools/custom/:tool` | `{enabled?, command?, args?, schema?}` |
| `PATCH` | `/agents/:name/tools/mcp/:tool` | `{enabled?, command?, args?, env?}` |
| `DELETE` | `/agents/:name/tools/custom/:tool` | remove the config entry |
| `DELETE` | `/agents/:name/tools/mcp/:tool` | remove the config entry |

Create a custom tool:

```bash
curl -X POST "http://localhost:9000/agents/myfriend/tools/custom" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"hello_helper","command":"node","args":["/path/script.js"],"enabled":true}'
```

Create an MCP server:

```bash
curl -X POST "http://localhost:9000/agents/myfriend/tools/mcp" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"playwright","command":"npx","args":["-y","@playwright/mcp"],"env":{"TOKEN":"secret"}}'
```

List response:

```json
{
  "builtin": [
    {"name": "bash", "description": "Execute system shell commands", "enabled": true}
  ],
  "custom": [
    {"name": "hello_helper", "command": "node", "args": ["/path/script.js"], "enabled": true}
  ],
  "mcp_servers": [
    {"name": "playwright", "command": "npx", "args": ["-y", "@playwright/mcp"], "enabled": true, "env": {"configured": true, "masked": "********"}}
  ]
}
```

Rules:

- Built-in tools (`bash`, `filesystem`, `cron`, `execute_python` with alias `python`, `provider`, `subagent`, `contact`, `a2a_input`) can only be enabled or disabled.
- Custom tool `name` and MCP server `name` must be unique; duplicates return `409`.
- `schema` is optional for custom tools; omit it to let the loader discover it via `--schema`. Sending `"schema": null` on `PATCH` removes an inline schema. A present inline schema is returned in list/create/patch responses.
- A provided schema must follow the OpenAI function format from the tools docs: valid JSON, `"type": "function"`, and a `function.name` matching the tool name.
- Duplicate names are rejected case-insensitively (`409`), and a custom tool name may not start with a registered MCP server name plus `_` (and vice versa) because the loader prefixes MCP tools as `<server>_<tool>`.
- `PATCH` requires at least one field; an empty body `{}` returns `400`. Deletion has its own endpoints.
- MCP `env` values are accepted on input and always masked on output.
- `DELETE` removes the `tools.json` entry only; it never touches files in the tools folder, and it does not stop a running MCP process (the agent server closes it on reload or restart).
- Missing or invalid `tools.json` behaves like the agent loader: all built-ins enabled and no custom tools or MCP servers. Writes create the file automatically.

## 9. Contacts (A2A)

Contacts are other A2A agents this agent may delegate tasks to. They live in `~/.myaaw/<agent>/contacts.json`; tokens are masked in responses. See [A2A Guide](../agents/protocol/a2a.md) for the protocol details.

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/agents/:name/contacts` | list contacts (tokens masked) |
| `POST` | `/agents/:name/contacts` | add `{name, url, token?, description?}` |
| `DELETE` | `/agents/:name/contacts/:contact` | remove a contact |

```bash
curl -X POST http://localhost:9000/agents/myfriend/contacts \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"research","url":"https://other-host/a2a","token":"remote-token"}'
```

- `name` must be unique (case-insensitive) and free of path separators.
- `url` must be an absolute `http(s)` URL.
- `token` is the remote agent's `A2A_TOKEN`; an empty token means the remote agent is open.
- Contacts are read fresh from disk, so manual edits appear immediately.

## 10. Providers

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/agents/:name/providers` | list providers and the default |
| `POST` | `/agents/:name/providers` | create |
| `PUT` | `/agents/:name/providers/:id` | update fields |
| `DELETE` | `/agents/:name/providers/:id` | delete |
| `POST` | `/agents/:name/providers/default` | set default `{name}` |
| `POST` | `/providers/models` | list models for a provider that is not saved yet |
| `GET` | `/agents/:name/providers/:id/models` | list models for a saved provider (uses its stored key) |

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

### Discovering models

There is no agent yet while the create wizard runs, so the model lookup is not scoped to an agent:

```bash
# before the agent exists
curl -X POST http://localhost:9000/providers/models \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"type":"openai","base_url":"https://api.deepseek.com","api_key":"sk-..."}'

# for a provider that is already saved (key stays server-side)
curl http://localhost:9000/agents/myfriend/providers/openai/models \
  -H "Authorization: Bearer $TOKEN"
```

Both return `{"models": [...], "source": "live"|"default", "warning": "..."}`. If the live call fails or times out, `source` is `default`, `models` contains the built-in default model, and `warning` explains why — so the wizard can always continue.

## 11. Channels

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

## 12. Config

`GET /agents/:name/config` returns providers, channels, bot settings, heartbeat, cron, transcriber, synthesizer, and observability with all secrets masked.

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/agents/:name/config` | read the full config (secrets masked) |
| `PATCH` | `/agents/:name/config` | partial update: only the fields sent change, no key is removed |
| `POST` | `/agents/:name/config/reset` | restore the whole `config.json` to the template (resets providers and channels too) |
| `PATCH` | `/agents/:name/config/reset` | reset only the named bot fields and sections |

`PATCH /config` covers `bot` (`type`, `owner_ids`, and every tuning field), `heartbeat`, `cron.active`, `transcriber`, `synthesizer`, and `observability`. Omitted fields are left untouched, so a key is never deleted:

```bash
curl -X PATCH http://localhost:9000/agents/myfriend/config \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"bot":{"type":"public","max_iterations":50},"heartbeat":{"active":false}}'
```

`PATCH /config/reset` takes the same section names and restores them from the template; `{"bot": []}` resets the whole bot section, while `{"bot": ["type"]}` resets just one field. Providers and channels are not touched by this variant.

`bot.queue_workers` (1–16) sets how many inbound chats the agent processes at once; messages from the same user stay in order. It takes effect after `POST /agents/:name/server/restart`.

### Observability settings

`GET /config` fills in the shipped defaults for any `observability` field the
agent's `config.json` omits, so the response shows what the agent actually runs
with. Overrides in the agent's `.env` (`OBS_*`) are not visible to the control
plane and are not reflected.

| Field | Meaning |
|---|---|
| `enabled` | Record traces at all. |
| `retention_days` | Delete traces older than this when the database opens. `0` keeps everything; omit the field to use the default 30. |
| `max_content_chars` | Byte cap for every stored content blob (default 16384). |
| `store_content` | Store prompts, answers, and tool payloads. `false` keeps metadata and token counts only. |
| `redact_patterns` | Regular expressions masked as `[redacted]` before content is stored or exported. An empty list disables redaction. |
| `tool_prices` | Per-call prices for tools, as `{"<tool>": {"per_call": 0.008}}`. Failed calls are not charged. |
| `model_prices` | Per-model prices that override the built-in table. |

```bash
curl -X PATCH http://localhost:9000/agents/myfriend/config \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{
    "observability": {
      "max_content_chars": 16384,
      "model_prices": {"gemini-3.1-pro-preview": {"input": 1.25, "output": 10}}
    }
  }'
```

The agent reads observability settings when its process starts, so restart it
(`POST /agents/:name/server/restart`) after changing them. This matters most when
turning recording on: the trace database is opened once per process.

## 13. Conversations

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/agents/:name/conversations?search=&limit=&offset=` | paginated conversation list |
| `GET` | `/agents/:name/conversations/:id?limit=&offset=` | conversation with a page of messages |
| `PATCH` | `/agents/:name/conversations/:id` | rename `{title}` |
| `DELETE` | `/agents/:name/conversations/:id` | delete a conversation |

**User scoping.** List, detail, rename, and delete use the `user_id` from `~/.myaaw/controlplane.json` (default `0`). An explicit `?user_id=` query parameter overrides it for administration. A conversation owned by another user returns `403`; a missing ID returns `404`.

**Search.** `search` matches the **title only**, case-insensitive for ASCII, and is applied before pagination. `total` reflects the filtered count.

**Create.** There is no explicit create endpoint: call chat without `conversation_id` and use the `conversation_id` returned in the response.

### List

```bash
curl "http://localhost:9000/agents/myfriend/conversations?limit=20&search=report" \
  -H "Authorization: Bearer $TOKEN"
```

```json
{
  "conversations": [],
  "total": 42,
  "limit": 50,
  "offset": 0,
  "has_more": false
}
```

`limit` defaults to 50; values above 200 fall back to 50. `offset` defaults to 0.

### Detail

```bash
curl "http://localhost:9000/agents/myfriend/conversations/conv-id?limit=20&offset=0" \
  -H "Authorization: Bearer $TOKEN"
```

The detail response adds `messages`, `total_messages`, `limit`, `offset`, and `has_more`. `limit=0` returns all messages; otherwise the maximum page size is 500.

### Rename

```bash
curl -X PATCH http://localhost:9000/agents/myfriend/conversations/conv-id \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"title":"Workspace cleanup"}'
```

```json
{"ok": true, "id": "conv-id", "title": "Workspace cleanup"}
```

The title is trimmed and must not be empty (`400`).

### Delete

```bash
curl -X DELETE http://localhost:9000/agents/myfriend/conversations/conv-id \
  -H "Authorization: Bearer $TOKEN"
```

```json
{"ok": true}
```

Conversations come from the agent's own SQLite database. Legacy traces are migrated on read. If the database does not exist yet, the list is empty.

## 14. Logs

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

## 15. Server control

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/agents/:name/server/status` | running, PID, port, base URL |
| `POST` | `/agents/:name/server/start` | start daemon |
| `POST` | `/agents/:name/server/stop` | stop daemon (idempotent) |
| `POST` | `/agents/:name/server/restart` | restart |

`start` returns `409` when the server is already running. Each daemon uses its own PID file and log directory.

## 16. Cron

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

## 17. Observability

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/agents/:name/observability/summary?days=7` | run totals, error rate, p50/p95 latency, per-step/model/trigger/tool breakdowns, most expensive and slowest run, `cost_complete` and `unpriced_*` flags, `tool_cost_usd`, and `database_bytes` |
| `GET` | `/agents/:name/observability/sessions?limit` | conversations with their turn count, tokens, and cost |
| `GET` | `/agents/:name/observability/sessions/:id` | one session with **all turns, oldest first**, plus per-turn step counts |
| `GET` | `/agents/:name/observability/runs?limit&offset&status&channel&conversation&model&trigger&from&to` | recorded runs, newest first; each run carries `trigger` (`user`, `cron`, `subagent`, or `a2a`) |
| `GET` | `/agents/:name/observability/runs/:id` | one run with flat `spans` and a nested `tree` |
| `DELETE` | `/agents/:name/observability/runs?before=<unix ms>` | delete runs; without `before` deletes all |

```json
{
  "run": { "id": "run_1a2b", "status": "completed", "model": "deepseek-flash", "total_ms": 3200, "model_cost_usd": 0.00042 },
  "spans": [
    { "id": "sp_1", "kind": "step", "name": "Step 1", "step_index": 1, "duration_ms": 1200, "parent_id": null },
    { "id": "sp_2", "kind": "llm", "parent_id": "sp_1", "step_index": 1, "request_index": 1,
      "first_token_ms": 780, "generation_ms": 420, "input_tokens": 2100, "output_tokens": 340,
      "cost_usd": 0.00042, "input_json": "…", "output_json": "…" }
  ],
  "tree": { "id": "run_1a2b", "children": [ { "id": "sp_1", "children": [] } ] }
}
```

Session detail returns every turn of one conversation in chronological order —
`runs[0]` is the first message, `runs[n-1]` the last. Each turn carries every run
field, its `steps`, and its full `spans`, so a trajectory UI renders a whole
session in one request:

```json
{
  "conversation_id": "4e7371da-c190-434e-90d3-e1902f309c82",
  "channel": "cli",
  "turns": 2,
  "started_at": 1761674469000,
  "last_active_at": 1761674512000,
  "input_tokens": 19300,
  "output_tokens": 224,
  "cost_usd": 0.002803,
  "error_count": 0,
  "runs": [
    {
      "id": "run_old",
      "started_at": 1761674469000,
      "ended_at": 1761674471040,
      "channel": "cli",
      "user_id": "1c043f52-…",
      "conversation_id": "4e7371da-…",
      "provider": "deepseek",
      "model": "deepseek-v4-flash",
      "agent": "default",
      "status": "completed",
      "input_tokens": 9600,
      "output_tokens": 143,
      "cache_hit_tokens": 0,
      "model_cost_usd": 0.001409,
      "total_ms": 2040,
      "first_token_ms": 1660,
      "steps": 1,
      "spans": [
        {
          "id": "sp_user", "run_id": "run_old", "kind": "user", "name": "user",
          "started_at": 1761674469000, "ended_at": 1761674469000, "status": "completed",
          "input_json": "halo"
        },
        {
          "id": "sp_step_1", "run_id": "run_old", "kind": "step", "name": "Step 1",
          "step_index": 1, "started_at": 1761674469010, "ended_at": 1761674470000,
          "duration_ms": 990, "status": "completed"
        },
        {
          "id": "sp_llm", "run_id": "run_old", "parent_id": "sp_step_1", "kind": "llm",
          "name": "deepseek-v4-flash", "step_index": 1, "request_index": 1,
          "started_at": 1761674469010, "ended_at": 1761674469980, "duration_ms": 970,
          "first_token_ms": 800, "generation_ms": 170,
          "input_tokens": 9600, "output_tokens": 143, "cost_usd": 0.001409,
          "status": "completed",
          "input_json": "{\"messages\":[{\"role\":\"system\",\"content\":\"…\"},{\"role\":\"user\",\"content\":\"halo\"}]}",
          "output_json": "{\"role\":\"assistant\",\"content\":\"Hai!\",\"thought\":\"…\"}"
        },
        {
          "id": "sp_tool", "run_id": "run_old", "parent_id": "sp_step_1", "kind": "tool",
          "name": "bash", "step_index": 1, "started_at": 1761674469985, "ended_at": 1761674470020,
          "duration_ms": 35, "status": "completed",
          "input_json": "{\"command\":\"pwd\"}",
          "output_json": "/Users/me"
        }
      ]
    },
    { "id": "run_new", "started_at": 1761674512000, "steps": 5, "spans": [ "…" ] }
  ]
}
```

Add `?spans=false` when a client only needs the turn list (faster and much
smaller). Without it, a session with many long turns can be a few megabytes,
because every prompt and tool payload is included.

### Mapping the response to a trajectory UI

`runs[]` in order is Turn 1 … Turn N. Inside a turn, order `spans` by
`started_at` and nest by `parent_id` to build the rows:

| Span | Row |
| :--- | :--- |
| `kind: "system"` | SYSTEM — `input_json` is the assembled prompt **as plain text**, not JSON |
| `kind: "user"` | USER — `input_json` is the inbound message **as plain text** |
| `kind: "compaction"` | COMPACTION — `input_json` is a JSON object with `tokens_before`, `tokens_after`, `budget`, and `messages_dropped` |
| `kind: "step"` | Step header for `step_index` |
| `kind: "llm"` | ASSISTANT — parse `output_json` as JSON for `content` and `thought`; parse `input_json` for the exact `messages` request. Timings come from `first_token_ms` / `generation_ms`. A second `llm` span in the same step is a retry; the first is `status: "retried"` |
| `kind: "tool"` | TOOL — `input_json` is the raw arguments string (usually JSON, but not guaranteed) and `output_json` the raw result text; `status` is `error` when it failed, including a tool blocked by policy |
| `kind: "run"` with a distinct `run_id` | A subagent run; its spans share that `run_id` and point at the tool span via `parent_id` |

Only `llm.input_json`, `llm.output_json`, and `compaction.input_json` are written
as JSON by the recorder. `system`, `user`, and `tool` blobs are raw text, so parse
them only when the tool's own schema says its arguments are JSON.

### Thinking and text previews

The one-line `thinking:` / `text:` preview that `myaaw obs run` prints under an
`llm` row needs no extra endpoint or field. Derive it from the span the run
detail already returns:

1. Parse `output_json` on a `kind: "llm"` span as
   `{ "content": string, "thought": string }`.
2. Render `thought` as `thinking:` and `content` as `text:`, in that order.
3. Skip a field when it is empty. If both are empty, or `output_json` is absent,
   that request produced only tool calls — or content capture was off when it was
   recorded. The API does not distinguish those two cases.
4. For the compact preview, take the first non-empty line, collapse whitespace,
   and cut it at about 100 characters; append `(+N more)` for the remaining
   lines. The expanded view can use the full value, which is capped at
   `observability.max_content_chars` (16384 by default) per blob.

A truncated `llm.output_json` still parses: the marker lives inside `content` or
`thought`, not after the closing brace. If parsing fails, the blob is not a
recorder-written assistant message — check the span kind (a `tool` result is
usually plain text) instead of assuming the cap broke it.

The same rule applies to every `llm` span, including a retry (`request_index >
1`). A step with no content on any of its `llm` spans is a pure tool-call step.

### Why `input_json` and `output_json` are strings

Every span kind stores a different payload: an llm input is a message envelope,
a tool input is arbitrary arguments, a tool result is often plain text, and a
compaction payload is numbers. The recorder writes the redacted, truncated form
into one TEXT column and the API returns that value unchanged. Keeping it a
string keeps the API a faithful view of storage, and a new provider payload never
forces a schema change.

For `llm.output_json` the JSON is still valid after truncation: the
`…[truncated N chars]` marker is inserted into the `content` / `thought` string
before the recorder marshals the object, so a client can always parse it. Tool
payloads are the exception: `input_json` and `output_json` are raw strings, may
not be JSON at all, and their truncation marker is appended as plain text.

The cap is `observability.max_content_chars` (16384 by default), applied when the
span is written, not by the API:

- `llm.input_json`: the whole message envelope is fitted under the cap. Messages
  are shortened by role first, then the oldest ones are dropped (the leading
  system message is never dropped) and `dropped_messages` records how many.
- `llm.output_json`: `content` and `thought` each get half the budget. If the
  marshaled object still exceeds the cap, `thought` is shortened further and
  keeps a readable prefix; it is dropped only when `content` alone already fills
  the cap. `content` is shortened last.
- `tool` spans: arguments (`input_json`) and result (`output_json`) are each cut
  at the cap.

Truncation appends `…[truncated N chars]`, so a stored blob can sit a few bytes
above the cap. `store_content: false` omits both fields entirely. The API adds
no second limit; a large session response is large because it returns every
stored blob (use `?spans=false` for the turn list).

For a duration timeline, offset each span by `span.started_at - run.started_at`
and use `span.ended_at` for its width. `steps` is the number of iteration bars;
`null` `ended_at` means the span is still running.

`404` means the conversation has no recorded runs.

An agent that has never recorded a run returns an empty summary (`runs: 0`) instead of `404`, so a dashboard can render before the first message. See [Observability](../agents/observability.md) for span semantics, pricing, and privacy.

## 18. Endpoint reference

| Group | Endpoints |
|---|---|
| Auth | `POST /auth/login`, `GET /auth/me`, `POST /auth/logout`, `POST /auth/change-password`, `PUT /auth/username` |
| Agents | `GET /agents`, `GET/DELETE /agents/:name`, `POST /agents/:name/onboard`, `POST /agents/:name/default` |
| Chat | `POST /agents/:name/chat`, `POST /agents/:name/chat/stream` |
| Files | `GET/DELETE /agents/:name/fs`, `GET /fs/read`, `GET /fs/search`, `POST /fs/write`, `POST /fs/mkdir`, `POST /fs/rename` |
| Context | `GET /agents/:name/context`, `GET /context/file?path=`, `PUT /context/file` |
| Skills | `GET /agents/:name/skills`, `GET /skills/:skill`, `GET /skills/:skill/file?path=`, `PATCH /skills/:skill`, `DELETE /skills/:skill`, `DELETE /skills/:skill/config` |
| Tools | `GET /agents/:name/tools`, `POST /tools/custom`, `POST /tools/mcp`, `PATCH /tools/builtin/:tool`, `PATCH /tools/custom/:tool`, `PATCH /tools/mcp/:tool`, `DELETE /tools/custom/:tool`, `DELETE /tools/mcp/:tool` |
| Contacts | `GET/POST /agents/:name/contacts`, `DELETE /contacts/:contact` |
| Providers | `GET/POST /providers`, `PUT/DELETE /providers/:id`, `POST /providers/default`, `POST /providers/models`, `GET /providers/:id/models` |
| Channels | `GET /channels`, `PUT/DELETE /channels/:channel` |
| Config | `GET /config`, `PATCH /config`, `POST /config/reset`, `PATCH /config/reset` |
| Conversations | `GET /conversations?search=`, `GET /conversations/:id`, `PATCH /conversations/:id`, `DELETE /conversations/:id` |
| Logs | `GET /logs`, `GET /logs/stream` |
| Server | `GET /server/status`, `POST /server/start`, `POST /server/stop`, `POST /server/restart` |
| Cron | `GET/POST /cron`, `DELETE /cron/:id`, `POST /cron/:id/run`, `GET /cron/:id/history` |
| Observability | `GET /observability/summary`, `GET /observability/sessions`, `GET /observability/sessions/:id`, `GET /observability/runs`, `GET /observability/runs/:id`, `DELETE /observability/runs` |
| System | `GET /health`, `GET /docs/*` |

All `/agents/...` paths are prefixed with `/agents/:name`. `POST /providers/models` is the only provider route outside an agent: it takes explicit credentials so a dashboard can fill the model picker before onboarding.

## 19. Errors

| Status | Meaning |
|---|---|
| `400` | invalid JSON, invalid path, or invalid config |
| `401` | missing, invalid, or expired token |
| `403` | conversation belongs to another user |
| `404` | agent, provider, channel, job, conversation, or log file not found |
| `409` | agent already exists, or server already running |
| `413` | file too large |
| `415` | binary file |
| `502` | agent server is not reachable |

## 20. Security notes

- There is no rate limiting in the first version.
- `/api/*` on the agent servers still has no auth; only the control plane protects access. Do not expose agent ports directly.
- Tools can run Bash, filesystem, and Python operations, so control-plane access is privileged.
- Keep `ALLOWED_ORIGINS` restrictive before serving a browser UI. The control plane reads it directly and allows `Authorization` in CORS preflight requests.
