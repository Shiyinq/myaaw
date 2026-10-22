# Agent2Agent (A2A) Guide

Myaaw agents can delegate tasks to other agents using a subset of the [Agent2Agent protocol](https://a2a-protocol.org/). Each agent server exposes an Agent Card for discovery and a JSON-RPC 2.0 endpoint for tasks. A built-in `contact` tool lets the agent call agents listed in its contact book.

> For the protocol model, task lifecycle, storage, request flows, security boundaries, and implementation details, see [a2a-protocol.md](a2a-protocol.md). For connection recipes (same machine, cross server, Myaaw to external, external to Myaaw), see [a2a-interop.md](a2a-interop.md).

Implemented today:

- Synchronous `tasks/send`, async `tasks/send` (`async: true`), and streaming `tasks/sendSubscribe` (SSE).
- Newer method aliases `message/send` and `message/stream`.
- Outbound `message/send` with an automatic `tasks/send` fallback for older peers.
- `tasks/get`, `tasks/cancel`, and `input-required` multi-turn tasks.
- Push notifications to a webhook URL for async tasks.
- Stored push notification configs: `tasks/pushNotificationConfig/set|get|list|delete`.
- `tasks/resubscribe` for replaying a running or recently finished stream.
- Text, structured data, and file input parts (images are passed to the model; other files are written to disk).
- File output artifacts from `output/`.
- Per-caller tokens and scopes via `A2A_TOKENS` and `A2A_CALLERS`.
- JWS (EdDSA) request signing and trusted peer keys.
- Signed Agent Cards: a JWS over the canonical card document (JCS-compatible for the card's data types).
- Cancellation that stops the running agent loop.
- Async tasks resume after a restart (up to three attempts).

## 1. Enable the endpoint

Set a bearer token in the agent's `.env` and restart the server:

```bash
# ~/.myaaw/<agent>/.env
A2A_TOKEN=change-me-to-a-random-string

# Optional: one shared token per caller
A2A_TOKENS=token-for-agent-a,token-for-agent-b

# Optional: named callers with scopes (delegate implies read)
A2A_CALLERS=monitor:token-for-monitor:read,research:token-for-research:delegate
```

Generate a token with:

```bash
openssl rand -hex 32
```

If `A2A_TOKEN`, `A2A_TOKENS`, and `A2A_CALLERS` are all empty, the A2A endpoint is open (useful for local development only).

### Scopes

| Scope | Allows |
| :--- | :--- |
| `read` | `tasks/get` |
| `delegate` | `tasks/send`, `tasks/sendSubscribe`, `tasks/cancel`, plus everything `read` allows |

Plain `A2A_TOKEN` / `A2A_TOKENS` entries get delegate access under the shared `default` identity. `A2A_CALLERS` entries look like `name:token` or `name:token:read|delegate`; an entry without a scope list gets delegate. A caller that lacks the required scope receives HTTP 403 with JSON-RPC error `-32003`.

### Signed requests (JWS)

Generate an Ed25519 key and point the agent at it:

```bash
openssl genpkey -algorithm ED25519 -out ~/.myaaw/<agent>/a2a_key.pem
chmod 600 ~/.myaaw/<agent>/a2a_key.pem

# ~/.myaaw/<agent>/.env
A2A_SIGNING_KEY=/Users/you/.myaaw/<agent>/a2a_key.pem
```

The Agent Card then publishes `publicKey` (base64url) and `keyId`. Outgoing `contact` calls carry a compact JWS (`EdDSA`) in the `X-A2A-Signature` header over the raw request body. To trust a peer, copy its `publicKey` from its Agent Card:

```bash
A2A_TRUSTED_KEYS=peer-name:base64url-public-key
A2A_REQUIRE_SIGNATURE=true
```

With `A2A_REQUIRE_SIGNATURE=false` (default), unsigned bearer-only callers still work. A present-but-invalid or unknown signature is always rejected. When the key is trusted, the peer's configured name becomes the caller identity for that request.

## 2. Agent Card (discovery)

```bash
curl http://localhost:8080/.well-known/agent.json
# alias: /.well-known/agent-card.json
```

```json
{
  "name": "myfriend",
  "description": "Myaaw agent myfriend",
  "url": "http://localhost:8080/a2a",
  "version": "1.0.0",
  "capabilities": {"streaming": true, "pushNotifications": true},
  "defaultInputModes": ["text", "file"],
  "defaultOutputModes": ["text"],
  "skills": [
    {"id": "weather", "name": "Weather", "description": "Fetch weather"}
  ],
  "securitySchemes": [{"type": "http", "scheme": "bearer"}],
  "publicKey": "base64url-ed25519-public-key",
  "keyId": "short-key-id",
  "signatures": [
    {"protected": "base64url-header", "signature": "base64url-signature"}
  ]
}
```

`publicKey`, `keyId`, and `signatures` only appear when `A2A_SIGNING_KEY` is configured. The card never contains secrets.

## 3. Send a task

All calls go to `POST /a2a` with a JSON-RPC 2.0 envelope.

```bash
curl -X POST http://localhost:8080/a2a \
  -H "Authorization: Bearer $A2A_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tasks/send",
    "params": {
      "message": {
        "role": "user",
        "parts": [{"kind": "text", "text": "Rangkum file AGENTS.md"}]
      }
    }
  }'
```

Response:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "id": "4f4c...",
    "contextId": "e2b1...",
    "status": {"state": "completed", "timestamp": "2026-10-21T09:00:00Z"},
    "messages": [
      {"role": "user", "parts": [{"kind": "text", "text": "Rangkum file AGENTS.md"}]},
      {"role": "agent", "parts": [{"kind": "text", "text": "..."}]}
    ],
    "artifacts": [
      {"name": "response", "parts": [{"kind": "text", "text": "..."}]}
    ]
  }
}
```

- `contextId` is the agent's conversation ID. Pass it back in `params.contextId` to continue the same conversation.
- A failed run returns `status.state = "failed"` with an explanation in `status.message`.
- Tasks are stored in the agent's SQLite database (`a2a_tasks`), so `tasks/get` works after the call or after a reconnect.

### Async tasks and push notifications

Add `"async": true` to return immediately while the task runs in the background. Optionally set `pushNotificationUrl` to receive the final task over HTTP POST.

```bash
curl -X POST http://localhost:8080/a2a \
  -H "Authorization: Bearer $A2A_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tasks/send",
    "params": {
      "async": true,
      "pushNotificationUrl": "https://your-host/a2a-callback",
      "message": {"role": "user", "parts": [{"kind": "text", "text": "Riset panjang..."}]}
    }
  }'
```

The response is a task in `submitted` (then `working`) state. Poll `tasks/get` or wait for the callback. The callback receives the final Task JSON with header `X-Myaaw-Task-ID`. Async runs are serialized per agent so two long tasks do not race the agent loop. If the server restarts while a task is `submitted`/`working`, it is marked `failed` on startup instead of hanging forever.

### Input-required (multi-turn)

The agent can pause a task and ask the caller for missing information by calling the built-in `a2a_input` tool:

```json
{"question": "Which city should I use?"}
```

The task becomes `input-required` with the question in `status.message`. Resume it by calling `tasks/send` again with the **same task id** and the answer:

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tasks/send",
  "params": {
    "id": "4f4c...",
    "message": {"role": "user", "parts": [{"kind": "text", "text": "Jakarta"}]}
  }
}
```

The existing `contextId` is reused, so the agent continues the same conversation. Calling `tasks/send` with an id that is not in `input-required` returns error `-32002`.

### Output artifacts

Before each run, the message receives a note with the task output directory:

```text
[A2A] Save files for the caller under ~/.myaaw/<agent>/jobs/a2a/<taskId>/output; they will be returned as artifacts.
```

Files written there are returned as `file` artifacts (base64 + MIME type), together with the text artifact. Limits: 10 files per task, 5 MiB per file. Symlinks are skipped.

### File parts

```json
{
  "kind": "file",
  "file": {"name": "photo.jpg", "mimeType": "image/jpeg", "bytes": "<base64>"}
}
```

- `image/*` parts are passed to the model as images.
- Other files are written to `~/.myaaw/<agent>/jobs/a2a/<taskId>/` and their absolute path is appended to the message text, so the agent can read them with its filesystem tool.
- Limits: 10 files per message, 5 MiB per file. `file.uri` is not supported; send base64 bytes.

### Streaming (`tasks/sendSubscribe`)

Use `tasks/sendSubscribe` to receive status and text-artifact updates over SSE:

```bash
curl -N -X POST http://localhost:8080/a2a \
  -H "Authorization: Bearer $A2A_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tasks/sendSubscribe",
    "params": {
      "message": {
        "role": "user",
        "parts": [{"kind": "text", "text": "Rangkum file AGENTS.md"}]
      }
    }
  }'
```

```text
event: status
data: {"kind":"status","taskId":"4f4c...","status":{"state":"working","timestamp":"..."},"final":false}

event: artifact
data: {"kind":"artifact","taskId":"4f4c...","artifact":{"name":"response","parts":[{"kind":"text","text":"Ringkasan "}]},"append":true}

event: artifact
data: {"kind":"artifact","taskId":"4f4c...","artifact":{"name":"response","parts":[{"kind":"text","text":"Ringkasan lengkap..."}]},"lastChunk":true}

event: status
data: {"kind":"status","taskId":"4f4c...","status":{"state":"completed","timestamp":"..."},"final":true}
```

Rules:

- `append: true` means the text is a delta to append.
- The final artifact (`lastChunk: true`, no `append`) replaces the accumulated deltas with the authoritative answer.
- `final: true` closes the run. A failure emits `state: "failed"` with `status.message` and `final: true`.
- The task is still persisted, so `tasks/get` works after the stream or after a reconnect.

### Get / cancel

```bash
# Get a task
curl -X POST http://localhost:8080/a2a \
  -H "Authorization: Bearer $A2A_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tasks/get","params":{"id":"TASK_ID"}}'

# Cancel a non-terminal task
curl -X POST http://localhost:8080/a2a \
  -H "Authorization: Bearer $A2A_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":3,"method":"tasks/cancel","params":{"id":"TASK_ID"}}'
```

### Error codes

| Code | Meaning |
|---|---|
| `-32700` | parse error |
| `-32600` | invalid request |
| `-32601` | method not found |
| `-32602` | invalid params |
| `-32603` | internal error |
| `-32001` | task not found |
| `-32002` | task already exists |

## 4. Contacts

A contact is another agent this agent may call. Contacts live in:

```text
~/.myaaw/<agent>/contacts.json
```

```json
[
  {
    "name": "research",
    "url": "https://other-host/a2a",
    "token": "token-from-the-other-agent",
    "description": "Research agent"
  }
]
```

`token` is the remote agent's `A2A_TOKEN`. It is stored only in this file and is masked by the control-plane API.

Manage contacts through the control plane:

```bash
# List (tokens masked)
curl http://localhost:9000/agents/myfriend/contacts \
  -H "Authorization: Bearer $TOKEN"

# Create
curl -X POST http://localhost:9000/agents/myfriend/contacts \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"research","url":"https://other-host/a2a","token":"remote-token"}'

# Delete
curl -X DELETE http://localhost:9000/agents/myfriend/contacts/research \
  -H "Authorization: Bearer $TOKEN"
```

## 5. The `contact` tool

The built-in `contact` tool manages the contact book and delegates tasks:

| Action | Purpose | Parameters |
| --- | --- | --- |
| `send` | Delegate a task and return the final answer | `contact`, `message`, optional `taskId` |
| `list` | Show configured contacts (tokens are masked) | - |
| `get` | Show one contact plus its Agent Card capabilities | `contact` |
| `add` | Register a new contact | `contact`, `url`, optional `token`, `description` |

Delegate a task:

```json
{
  "action": "send",
  "contact": "research",
  "message": "Cari referensi terbaru tentang A2A protocol"
}
```

Discover what is available:

```json
{"action": "list"}
```

```json
{"action": "get", "contact": "research"}
```

Register a contact from chat (only when the user explicitly provides the URL):

```json
{
  "action": "add",
  "contact": "research",
  "url": "https://other-host/a2a",
  "token": "remote-token",
  "description": "Research agent"
}
```

Aliases are accepted for convenience: `send_message`, `list_contacts`, `get_contact`, `add_contact`.

On `send` the tool:

1. Looks up the contact by name.
2. Calls `tasks/send` on the contact's URL with its token.
3. Waits for the task to finish.
4. Returns the artifact text as the tool result, so the local agent can continue its loop.

When the remote agent asks for more information (`input-required`), the result starts with a machine-readable header:

```text
[input-required taskId=7f2c...]
Tanggal berapa?
```

Answer with the same task ID:

```json
{
  "action": "send",
  "contact": "research",
  "taskId": "7f2c...",
  "message": "1 Oktober 2026"
}
```

Without `taskId`, each `send` starts a new task. A `taskId` that is no longer waiting for input is rejected with a readable error. A canceled remote task is reported as canceled.

A failed task or unknown contact is returned as a readable error string to the model. Unknown contacts also report the available names.

`get` fetches the remote `/.well-known/agent.json` (5 second timeout) and reports its skills; when the remote agent is offline it falls back to the local contact metadata. `add` validates the name and URL the same way as the control-plane API, writes `contacts.json` atomically with mode `0600`, and never echoes the token back. Calls without `action` that include `contact` and `message` are still treated as `send` for backwards compatibility.

## 6. Security notes

- `A2A_TOKEN`, `A2A_TOKENS`, `A2A_CALLERS`, and `contacts.json` contain secrets. Use file permissions `0600`; never commit them.
- The Agent Card is public metadata and never includes tokens. `publicKey` is a public key, safe to publish.
- Tokens are compared in constant time.
- Use HTTPS for contacts across hosts; bearer tokens and signature headers travel in clear text over plain HTTP.
- Prefer `A2A_CALLERS` with one token per caller so access can be revoked per agent and scopes can be narrowed.
- `A2A_REQUIRE_SIGNATURE=true` makes a valid JWS mandatory, but a token is still required when tokens are configured.
- A2A transcripts use a synthetic local user per calling agent: `a2a:<caller>` with IDs starting at `-1001`. They do not appear in the terminal or default control-plane conversation list, and each caller gets an isolated conversation namespace.
- `tasks/get`, `tasks/cancel`, and `input-required` resume are limited to the caller that created the task. Legacy tasks without a recorded caller remain readable.
- Signed Agent Cards can be verified with `A2A_TRUSTED_KEYS`; `contact get` reports `Card signature: verified` or `unverified`.
- The `add` action lets the agent register a new endpoint. A prompt-injected model could add an attacker-controlled URL and send data to it. Block the tool (`AGENT_BLOCKED_TOOLS=contact`) or keep `contacts.json` read-only for agents that must not manage contacts.

## 7. Limitations / next phase

- Async retries happen on restart only, capped at three attempts.
- `tasks/resubscribe` buffers live in memory and expire after 10 minutes.
- Signed-card canonicalization matches JCS for the card's current data types; it is not a general JCS implementation.
- Alignment with future A2A spec revisions.
