# REST API Guide

This guide explains how to call the Myaaw REST API and how to consume its responses, both non-streaming and streaming (Server-Sent Events).

> This document covers the per-agent API served by each agent server. For one unified API across multiple agents, see [control-plane.md](control-plane.md).

## 1. Overview

Every running agent exposes its own HTTP server. The API uses the same agent loop, SQLite database, `home/`, tools, and provider as `myaaw chat`, Telegram, and Discord.

```text
myaaw --agent default server start     → http://localhost:8080
myaaw --agent myfriend server start    → http://localhost:8081
```

The port comes from `PORT` in `~/.myaaw/<agent>/.env`. There is no `agent` field in requests: the port determines the agent.

| Endpoint | Method | Purpose |
|---|---|---|
| `/api/chat` | `POST` | Chat, single JSON response |
| `/api/chat/stream` | `POST` | Chat, Server-Sent Events |
| `/api/approvals/:id` | `POST` | Answer a pending tool approval |
| `/docs/index.html` | `GET` | Swagger UI |
| `/webhook/telegram` | `POST` | Internal: Telegram webhook mode |
| `/webhook/bot` | `POST` | Internal: queue consumer |
| `/heartbeat` | `POST` | Internal: cron and heartbeat |

Use only `/api/chat` and `/api/chat/stream` from your application.

## 2. Security

The `/api/*` routes have **no authentication**:

- no API key, no Bearer token, no owner allowlist;
- `BOT_TYPE=private` and `OWNER_IDS` are enforced only for Telegram/Discord webhooks;
- CORS is configurable through `ALLOWED_ORIGINS`, but CORS does not protect the endpoint;
- the server listens on `:PORT` (all interfaces), not only loopback.

Anyone who can reach the port can run the agent and its tools. Keep the server on a trusted network, bind it behind a reverse proxy with authentication, or firewall the port.

## 3. Request body

```json
{
  "user_id": 12345,
  "text": "Check my workspace",
  "images": ["<base64-image>"],
  "conversation_id": "conv-id-or-NEW"
}
```

| Field | Type | Required | Notes |
|---|---|---|---|
| `user_id` | integer | yes | must be non-zero; identifies the user and conversation owner |
| `text` | string | yes | must not be empty |
| `images` | string[] | no | base64 image data; only the Gemini provider consumes them |
| `conversation_id` | string | no | continue a specific conversation; omit it to start a new session (`"NEW"` also starts a new session) |

`user_id` maps to a row in the agent's SQLite database. Omit `conversation_id` and every request starts a **new conversation**. To continue a session, pass the `conversation_id` returned by the previous response. An explicit ID that does not exist returns `404`; an ID owned by another user returns `403`.

## 4. Non-streaming: `POST /api/chat`

```bash
curl -X POST http://localhost:8080/api/chat \
  -H 'Content-Type: application/json' \
  -d '{"user_id":12345,"text":"Check my workspace"}'
```

### Response

```json
{
  "text": "There are 3 files.",
  "conversation_id": "conv-id",
  "timeline": [
    {
      "kind": "thought",
      "text": "I should list the workspace first."
    },
    {
      "kind": "tool",
      "id": "call_1",
      "name": "bash",
      "arguments": "{\"command\":\"ls\"}",
      "result": "a.txt\nb.txt\nc.txt"
    },
    {
      "kind": "text",
      "text": "There are 3 files."
    }
  ],
  "usage": {
    "prompt_tokens": 120,
    "completion_tokens": 30,
    "total_tokens": 150
  }
}
```

| Field | Meaning |
|---|---|
| `text` | authoritative final answer from the agent loop |
| `conversation_id` | conversation used for this run; pass it to continue the session |
| `timeline` | ordered steps: `thought`, `tool`, `text` |
| `usage` | token usage; omitted when `total_tokens` is 0 |

### Timeline items

| `kind` | Fields | Meaning |
|---|---|---|
| `thought` | `text` | reasoning deltas, merged into one item |
| `text` | `text` | assistant text, merged into one item; the last one is the final answer |
| `tool` | `id`, `name`, `arguments`, `result`, `is_error` | tool call; the result is attached to the same item |
| `tool_result` | `id`, `name`, `result`, `is_error` | only used when a result arrives without a matching tool item |

`text` always equals the last `kind: "text"` item because the timeline is finalized with the authoritative answer. If the run stops without normal output (for example, maximum iterations), that fallback message is what `text` contains.

## 5. Streaming: `POST /api/chat/stream`

```bash
curl -N -X POST http://localhost:8080/api/chat/stream \
  -H 'Content-Type: application/json' \
  -d '{"user_id":12345,"text":"Check my workspace"}'
```

The response is `text/event-stream`. Each event is:

```text
event: <name>
data: <json>

```

### Events

| Event | Data | Meaning |
|---|---|---|
| `thought` | `{"text": "..."}` | reasoning delta |
| `chunk` | `{"text": "..."}` | answer text delta |
| `tool` | `{"id","name","arguments"}` | tool execution started |
| `tool_result` | `{"id","name","result","is_error"}` | tool execution finished |
| `approval_required` | `{"approval_id","tool_name","action","args_summary","args","subagent"?}` — a gated tool is waiting; `args` is the full JSON and `subagent` names the child task when relevant |
| `approval_resolved` | `{"approval_id","decision","reason"}` | the decision was applied (or the request timed out) |
| `done` | `{"ok":true,"text":"...","conversation_id":"...","usage":{...}}` | run finished; `conversation_id` and `usage` are optional |
| `error` | `{"error":"..."}` | run failed; followed by `done` with `ok:false` |

The `chunk` name is kept for compatibility. Treat it as `text`.

### Example sequence

```text
event: thought
data: {"text":"I should list the workspace first."}

event: tool
data: {"arguments":"{\"command\":\"ls\"}","id":"call_1","name":"bash"}

event: tool_result
data: {"id":"call_1","is_error":false,"name":"bash","result":"a.txt\nb.txt\nc.txt"}

event: chunk
data: {"text":"There are "}

event: chunk
data: {"text":"3 files."}

event: done
data: {"ok":true,"text":"There are 3 files.","usage":{"completion_tokens":30,"prompt_tokens":120,"total_tokens":150}}
```

### How to consume

1. Merge consecutive `thought` deltas into one thinking block.
2. Merge consecutive `chunk` deltas into one text block.
3. On `tool`, create a running tool item using `id`, `name`, and `arguments`.
4. On `tool_result`, finish the matching item with `id`, `result`, and `is_error`.
5. On `done` with `ok:true`, use `done.text` as the authoritative final answer:
   - replace the last text block with `done.text`, or
   - append it when no text delta was streamed (for example, a max-iterations fallback).
6. On `error`, show `error`; the following `done` has `ok:false`.

`done.text` is authoritative because streamed deltas can be incomplete even when the run succeeds. This is the same finalization the TUI and Telegram timelines perform.

### JavaScript example

`EventSource` cannot send a POST body, so read the stream with `fetch`:

```js
const response = await fetch("http://localhost:8080/api/chat/stream", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
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
  if (data.length > 0) {
    const payload = JSON.parse(data.join("\n"));
    console.log(event, payload);
  }
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

When the active policy gates a tool, the run pauses and asks the caller.

**Streaming:** handle `approval_required`, show the prompt, then POST a decision:

```bash
curl -X POST http://localhost:8080/api/approvals/<approval_id> \
  -H 'Content-Type: application/json' \
  -d '{"decision":"allow_once"}'
```

**Non-streaming:** the endpoint returns HTTP 200 with

```json
{
  "status": "approval_required",
  "approval": {
    "id": "...",
    "run_id": "...",
    "tool_name": "bash",
    "action": "",
    "args_summary": "{\"command\":\"ls\"}",
    "args": "{\"command\":\"ls\"}",
    "subagent": "download-x-photo"
  }
}
```

`subagent` is present only when a background sub-agent raised the request.

The same decision call then blocks until the resumed run completes and returns
the final chat response (or the next `approval_required`). Decisions:
`allow_once`, `allow_session`, `allow_always`, `deny`. The body must also carry
the run's `user_id` (`{"decision":"allow_once","user_id":7}` or the
`X-Myaaw-User-ID` header); a missing user id returns `400`, a different user
`403`, and an unknown/expired id `404`. Without an answer the call is denied
after `approvals.timeout_seconds`.

`POST /api/approvals/:id` returns `404` for an unknown or already-expired ID.
Paused non-streaming runs live in memory, so a daemon restart drops them and the
run is denied.

See [Tool Approvals](../agents/approvals.md) for policy fields and grant scopes.

## 6. Mapping to the terminal and Telegram timelines

The API mirrors the channel timelines:

| Timeline | `thought` | text | tool lifecycle | final answer |
|---|---|---|---|---|
| CLI | `thinking` item | `text` item | `tool` item | last `text` item |
| Telegram | `thinking` bubble | `text` bubble | tool bubble | last `text` bubble |
| Discord | `thinking` item | `text` item | tool item | last `text` item |
| API | `kind: "thought"` | `kind: "text"` / `chunk` | `kind: "tool"` | `text` / `done.text` |

There is no separate trace field. Tool lifecycle events already carry the action, arguments, and result.

## 7. Errors

### Non-streaming

| Status | Body | Cause |
|---|---|---|
| `400` | `{"error":"..."}` | invalid JSON, missing `user_id`, or empty `text` |
| `403` | `{"error":"..."}` | `conversation_id` belongs to another user |
| `404` | `{"error":"..."}` | `conversation_id` not found |
| `500` | `{"error":"..."}` | provider or agent failure |

### Streaming

```text
event: error
data: {"error":"..."}

event: done
data: {"ok":false}
```

For streaming requests, an invalid `conversation_id` is reported as an `error` event because the HTTP status is already `200` once the stream starts.

## 8. Limitations

- No authentication or per-key rate limiting.
- This per-agent API has no conversation listing endpoint; use the control plane API for history and pagination.
- Images are only consumed by the Gemini provider, as raw base64 without a `data:` prefix.
- The API can execute tools (Bash, filesystem, Python, skills), so treat access to the port as privileged.
