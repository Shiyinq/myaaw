# A2A Interop Guide

This guide covers how to connect agents in practice:

1. Myaaw agent ↔ Myaaw agent on the same machine.
2. Myaaw agent ↔ Myaaw agent on different servers.
3. Myaaw agent calling an agent that is **not** Myaaw.
4. An agent that is **not** Myaaw calling a Myaaw agent.

For protocol concepts and internals, see [a2a-protocol.md](a2a-protocol.md). For the day-to-day guide, see [a2a.md](a2a.md).

> In this document, "Myaaw agent" means an agent server built from this project: `myaaw --agent <name> server start`, with its own `~/.myaaw/<name>/` directory, database, tools, and skills.

## 0. The four cases at a glance

| Case | Direction | What you configure | Wire compatibility |
| :--- | :--- | :--- | :--- |
| Same machine | Myaaw → Myaaw | Token on B, contact on A | Full (same implementation) |
| Cross server | Myaaw → Myaaw | Token, reachable URL, optional signing | Full (same implementation) |
| Outbound | Myaaw → external | Contact with the peer's URL and token | Basic `tasks/send` only |
| Inbound | External → Myaaw | Token on Myaaw, peer calls `/a2a` | `tasks/send` and `message/send` alias |

## 1. Common requirements

Every A2A connection needs the same four things.

| Requirement | Detail |
| :--- | :--- |
| **A running agent server** | `myaaw --agent <name> server start`. The A2A routes live on the agent server, not on `myaaw api` and not on `myaaw chat`. |
| **A reachable JSON-RPC endpoint** | `POST http://<host>:<port>/a2a`. Discovery is `GET /.well-known/agent.json` (alias `/.well-known/agent-card.json`). |
| **A credential** | A bearer token in the receiver's `.env`; optionally a named caller with scopes. If every credential variable is empty, the endpoint is **open** and should only be used locally. |
| **A configured agent** | The receiver must have a working LLM provider, because it runs its own agent loop to answer. |

Ports come from `PORT` in `~/.myaaw/<agent>/.env`. In the examples below, the receiver is **B** and the caller is **A**.

## 2. Myaaw ↔ Myaaw on the same machine

This is the simplest setup and works offline.

### 2.1 Prepare the receiver (B)

Generate a token:

```bash
openssl rand -hex 24
```

Add it to B's environment and restart the server:

```bash
# ~/.myaaw/B/.env
A2A_TOKEN=<token-for-A>
PORT=8081
MYAAW_BASE_URL=http://localhost:8081
```

```bash
myaaw --agent B server start
```

Verify:

```bash
curl -s http://localhost:8081/.well-known/agent.json
```

You should see the Agent Card with `name`, `url` (`http://localhost:8081/a2a`), and `skills`.

### 2.2 Register B on the caller (A)

Create `~/.myaaw/A/contacts.json`:

```json
[
  {
    "name": "b",
    "url": "http://localhost:8081/a2a",
    "token": "<token-for-A>",
    "description": "Agent B on this machine"
  }
]
```

Rules:

- `url` must be the **JSON-RPC endpoint** (`/a2a`), not the Agent Card URL.
- `name` is local to A; it is what you say in chat.
- The file should be mode `0600`.

Alternatively, use the control plane:

```bash
curl -X POST http://localhost:9000/agents/A/contacts \
  -H "Authorization: Bearer <control-plane-session>" \
  -H 'Content-Type: application/json' \
  -d '{"name":"b","url":"http://localhost:8081/a2a","token":"<token-for-A>"}'
```

### 2.3 Use it

```bash
myaaw --agent A chat -m "Tanya agent b: siapa kamu? Balas satu kalimat."
```

The agent calls the built-in `contact` tool, which sends `tasks/send` to B and returns B's answer.

Inspect what A knows:

```text
{"action":"list"}
{"action":"get","contact":"b"}
```

`get` also fetches B's Agent Card and shows its skills.

### 2.4 Multi-turn (`input-required`)

If B needs more information, `contact` returns:

```text
[input-required taskId=7f2c...]
Tanggal berapa?
```

A answers with the same task ID:

```text
{"action":"send","contact":"b","taskId":"7f2c...","message":"1 Oktober 2026"}
```

Without `taskId`, a new task is created.

### 2.5 Several callers with separate identities

On B, replace the shared token with named callers:

```env
# ~/.myaaw/B/.env
A2A_CALLERS=alice:<token-alice>:delegate,bob:<token-bob>:read
```

Each entry is `name:token[:read|delegate]`. Scopes:

- `read` → only `tasks/get`.
- `delegate` → `tasks/send`, `tasks/sendSubscribe`, `tasks/cancel`, and read.

Each name gets its own synthetic local user (starting at `-1001`), its own transcript namespace, and task ownership. In each caller's `contacts.json`, put the token that B assigned to that caller.

## 3. Myaaw ↔ Myaaw across servers

The steps are the same as section 2, plus network and identity details.

### 3.1 Make B reachable

On B:

```env
# ~/.myaaw/B/.env
PORT=8081
MYAAW_BASE_URL=https://agents.example.com/b
A2A_TOKEN=<token-for-A>
```

`MYAAW_BASE_URL` must be the address **A can reach**. It becomes the `url` field of B's Agent Card and the base for `/.well-known/agent.json`.

Options:

- Direct: `http://192.168.1.10:8081` on a trusted LAN.
- Reverse proxy: `https://agents.example.com/b` → `http://127.0.0.1:8081`.
- Tunnel: ngrok/Cloudflare Tunnel for testing.

Open the port in the firewall. If you use a reverse proxy, make sure `POST /a2a` and `POST /a2a` streaming (`text/event-stream`) are both allowed, with buffering disabled for SSE.

### 3.2 Register B on A

```json
{
  "name": "b-remote",
  "url": "https://agents.example.com/b/a2a",
  "token": "<token-for-A>"
}
```

Verify from A's machine:

```bash
curl -s https://agents.example.com/b/.well-known/agent.json
```

Do not put `localhost` in B's `MYAAW_BASE_URL` when A runs elsewhere.

### 3.3 Signed requests (optional, recommended across organizations)

B can require proof of key possession in addition to the bearer token.

On A (caller), create a key and enable signing:

```bash
openssl genpkey -algorithm ED25519 -out ~/.myaaw/A/a2a_key.pem
chmod 600 ~/.myaaw/A/a2a_key.pem
```

```env
# ~/.myaaw/A/.env
A2A_SIGNING_KEY=/Users/you/.myaaw/A/a2a_key.pem
```

Restart A. Its Agent Card now contains `publicKey` and `keyId`.

On B (receiver), trust A's key. Read A's Agent Card to get its `publicKey`:

```bash
curl -s http://<A-host>:<A-port>/.well-known/agent.json
```

```env
# ~/.myaaw/B/.env
A2A_TRUSTED_KEYS=alice:<A-publicKey>
A2A_REQUIRE_SIGNATURE=true
```

Restart B. Now:

- A signs every `contact` request with `X-A2A-Signature` (compact JWS, `EdDSA`) over the raw body.
- B rejects requests with a missing, unknown, or invalid signature when `A2A_REQUIRE_SIGNATURE=true`.
- A present-but-invalid signature is always rejected, even when the flag is false.
- B's Agent Card is signed too. On A, `contact get` reports `Card signature: verified` when B's key is in A's `A2A_TRUSTED_KEYS`, otherwise `unverified`.

The signature header is a Myaaw extension. It is optional for Myaaw ↔ Myaaw but external agents will not understand it; keep `A2A_REQUIRE_SIGNATURE=false` for those.

### 3.4 Common cross-server mistakes

| Mistake | Symptom | Fix |
| :--- | :--- | :--- |
| `MYAAW_BASE_URL` still `localhost` | Card points to the wrong host | Set the reachable URL |
| Contact URL is the Agent Card URL | `HTTP 405` or parse error | Use `/a2a` |
| Token from the wrong agent | `401` | Use the token configured on B |
| Firewall blocks the port | Connection timeout | Open the port or use a tunnel |
| Proxy buffers SSE | Streaming arrives all at once | Disable buffering for `text/event-stream` |
| B has no provider | Task ends `failed` | Configure a provider on B |

## 4. Myaaw agent calling a non-Myaaw agent

Myaaw's outbound path is the `contact` tool. It sends **`message/send`** first (with `kind`, `messageId`, and text parts) and falls back to `tasks/send` when the peer answers `-32601`. It expects a JSON-RPC result shaped like a task.

### 4.1 What the external peer must provide

| Requirement | Detail |
| :--- | :--- |
| JSON-RPC endpoint | Usually `https://peer.example.com/a2a` |
| Method `message/send` | Tried first; the message carries `kind`, `messageId`, and `parts` |
| Method `tasks/send` | Automatic fallback when the peer answers `-32601` (unknown method) |
| Result shape | `result.status.state`, optional `result.artifacts[].parts[]`, `result.messages[]`, or `result.history[]` |
| Auth | Whatever the peer expects; Myaaw sends `Authorization: Bearer <token>` from `contacts.json` |

Myaaw never retries on network or validation errors, so a task is not submitted twice.

### 4.2 Register the peer

```json
[
  {
    "name": "peer",
    "url": "https://peer.example.com/a2a",
    "token": "<peer-token>",
    "description": "External A2A agent"
  }
]
```

### 4.3 What Myaaw sends

```json
{
  "jsonrpc": "2.0",
  "id": "<uuid>",
  "method": "message/send",
  "params": {
    "message": {
      "kind": "message",
      "messageId": "<uuid>",
      "role": "user",
      "parts": [{"kind": "text", "text": "..."}]
    }
  }
}
```

When resuming an `input-required` task, Myaaw also sends `"id": "<taskId>"` at the params level and `"taskId": "<taskId>"` inside the message, so both early-style and newer servers can continue it.

If the peer answers `-32601`, Myaaw retries once with `tasks/send` using the same message shape. It does not retry on HTTP errors, timeouts, or validation failures.

### 4.4 What Myaaw can read back

- `result.status.state` — `failed` is reported as an error; `input-required` is surfaced with `taskId`; `canceled` is reported; anything else is treated as success.
- `result.artifacts[].parts[]` with `kind: "text"` — preferred.
- `result.messages[]` with `role: "agent"` and text parts — fallback.
- `result.history[]` with `role: "agent"` — fallback for newer peers that no longer return `messages`.

### 4.5 Signature caveat

Myaaw can sign outbound requests with `X-A2A-Signature`, but that is a Myaaw extension. External peers will ignore it. Verify the peer's own auth requirements (some use OAuth2 or mTLS) before relying on `contact`.

### 4.6 When `contact` is not enough

For a peer with a different method name or response shape, call it directly:

```bash
curl -s -X POST https://peer.example.com/a2a \
  -H "Authorization: Bearer $PEER_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"message/send",
       "params":{"message":{"role":"user","messageId":"m-1",
       "parts":[{"kind":"text","text":"hello"}]}}}'
```

You can wrap this in a custom tool (see [mcp_and_custom_tools.md](mcp_and_custom_tools.md)) if the integration is long-lived.

## 5. A non-Myaaw agent calling a Myaaw agent

A Myaaw agent is a standard-ish A2A server. Any client that can do HTTP and JSON-RPC can call it.

### 5.1 Discover

```bash
curl -s https://agents.example.com/b/.well-known/agent.json
```

The alias `/.well-known/agent-card.json` serves the same document.

Useful fields:

| Field | Meaning |
| :--- | :--- |
| `url` | JSON-RPC endpoint to call |
| `capabilities.streaming` | `true` |
| `capabilities.pushNotifications` | `true` |
| `skills` | Enabled skills of that agent |
| `securitySchemes` | Present when credentials are configured |
| `publicKey` / `keyId` | Present when `A2A_SIGNING_KEY` is configured |
| `signatures` | Present when the card is signed; verify with the peer's pinned public key |

### 5.2 Authenticate

```http
Authorization: Bearer <token>
Content-Type: application/json
```

If `A2A_REQUIRE_SIGNATURE=true`, also send `X-A2A-Signature` (Myaaw's JWS format). Most external clients should be given a deployment where this flag is `false`.

### 5.3 Send a task

```bash
curl -s -X POST https://agents.example.com/b/a2a \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "jsonrpc":"2.0",
    "id":1,
    "method":"tasks/send",
    "params":{
      "message":{
        "role":"user",
        "parts":[{"kind":"text","text":"Hello from another agent"}]
      }
    }
  }'
```

The `message/send` alias is accepted:

```json
{"jsonrpc":"2.0","id":1,"method":"message/send","params":{"message":{...}}}
```

Note: Myaaw reads `contextId` from the **params** level, not from inside `message`. A strict client that puts `contextId` inside `message` will get a new context.

### 5.4 Response shape

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "id": "0c4c0c50-...",
    "contextId": "ctx-1",
    "status": {"state": "completed", "timestamp": "..."},
    "messages": [
      {"role": "user", "parts": [{"kind": "text", "text": "Hello"}]},
      {"role": "agent", "parts": [{"kind": "text", "text": "Hi"}]}
    ],
    "artifacts": [
      {"name": "response", "parts": [{"kind": "text", "text": "Hi"}]}
    ]
  }
}
```

Myaaw uses `messages`, not `history`. A strict client that only reads `history` will not see the transcript, but `artifacts` still contains the final text.

### 5.5 Continue a conversation

Send `contextId` at the params level:

```json
{"jsonrpc":"2.0","id":2,"method":"tasks/send",
 "params":{"contextId":"ctx-1","message":{...}}}
```

The first task with a new `contextId` creates the transcript; later tasks continue it. Contexts are scoped per caller, so another caller cannot reuse them.

### 5.6 Streaming

```bash
curl -N -X POST https://agents.example.com/b/a2a \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":3,"method":"tasks/sendSubscribe",
       "params":{"message":{"role":"user","parts":[{"kind":"text","text":"Count to 3"}]}}}'
```

Events:

```text
event: status
data: {"kind":"status","taskId":"...","status":{"state":"working"}}

event: artifact
data: {"kind":"artifact","taskId":"...","artifact":{"name":"response","parts":[{"kind":"text","text":"1"}]},"append":true}

event: artifact
data: {"kind":"artifact","taskId":"...","artifact":{"name":"response","parts":[{"kind":"text","text":"1 2 3"}]},"lastChunk":true}

event: status
data: {"kind":"status","taskId":"...","status":{"state":"completed"},"final":true}
```

`async: true` is rejected for this method. The `message/stream` alias is accepted.

### 5.7 Async and push notifications

```json
{
  "jsonrpc":"2.0","id":4,"method":"tasks/send",
  "params":{
    "async":true,
    "pushNotificationUrl":"https://caller.example.com/hook",
    "message":{"role":"user","parts":[{"kind":"text","text":"Long job"}]}
  }
}
```

The call returns immediately with `status.state = "submitted"`. When the task finishes, Myaaw POSTs the full Task JSON to the URL with `X-Myaaw-Task-ID`.

Per-task webhooks can also be stored on the server:

```json
{"jsonrpc":"2.0","id":5,"method":"tasks/pushNotificationConfig/set",
 "params":{"id":"<task-id>","pushNotificationConfig":{"url":"https://caller.example.com/hook","token":"hook-secret"}}}
```

```json
{"jsonrpc":"2.0","id":6,"method":"tasks/pushNotificationConfig/list","params":{"id":"<task-id>"}}
```

`get` accepts an optional `pushNotificationConfigId`; `delete` requires it. Stored configs fire on `completed`, `failed`, `input-required`, and `canceled`, and a config `token` is sent as `Authorization: Bearer <token>`.

### 5.8 Ask for more information (`input-required`)

If the Myaaw agent calls its `a2a_input` tool, the task stops at:

```json
"status": {"state": "input-required", "message": {"role": "agent", "parts": [{"kind": "text", "text": "Which date?"}]}}
```

Resume with `tasks/send` and the **same task ID**:

```json
{"jsonrpc":"2.0","id":5,"method":"tasks/send",
 "params":{"id":"<task-id>","message":{"role":"user","parts":[{"kind":"text","text":"2026-10-01"}]}}}
```

Only the caller that created the task may resume it.

### 5.9 Files

Input file parts must use base64 `bytes`:

```json
{"kind":"file","file":{"name":"report.pdf","mimeType":"application/pdf","bytes":"<base64>"}}
```

- Maximum 10 file parts per message.
- Maximum 5 MiB per decoded file.
- `image/*` is passed to the model as an image.
- Other files are written to the task directory and referenced by path.
- `file.uri` is rejected.

Files the agent writes under the task's `output/` directory are returned as `file` artifacts (base64, up to 10 files, 5 MiB each).

### 5.10 Get and cancel

```json
{"jsonrpc":"2.0","id":6,"method":"tasks/get","params":{"id":"<task-id>"}}
{"jsonrpc":"2.0","id":7,"method":"tasks/cancel","params":{"id":"<task-id>"}}
```

`tasks/cancel` stops the running agent loop and keeps the task in `canceled` state. Canceling a terminal task returns it unchanged.

### 5.11 Errors and scopes

| Code | Meaning |
| :--- | :--- |
| `-32700` | Parse error |
| `-32600` | Invalid JSON-RPC request |
| `-32601` | Unknown method |
| `-32602` | Invalid params |
| `-32603` | Internal error |
| `-32001` | Task not found, or not owned by this caller |
| `-32002` | Task exists and is not resumable |
| `-32003` | Caller lacks the required scope (HTTP 403) |

Scope rules:

- `read` → `tasks/get`.
- `delegate` → `tasks/send`, `tasks/sendSubscribe`, `tasks/cancel`, plus read.

Task ownership: only the caller that created a task may get, cancel, or resume it. Legacy tasks without a recorded caller remain accessible.

### 5.12 Reliability notes for external callers

| Feature | Status |
| :--- | :--- |
| `message/send` with `messageId` | Accepted, but `messageId` is ignored |
| `contextId` inside `message` | Ignored; put it at params level |
| Task field `history` | Not returned; Myaaw returns `messages` |
| `tasks/resubscribe` | Available for streams started by the current process; the buffer is in memory and expires after 10 minutes, and is lost on restart |
| `tasks/pushNotificationConfig/*` | Available; configs are stored in SQLite and survive restarts |
| Signed Agent Card | Available; JWS over the canonical card, verified against pinned keys |
| `X-A2A-Signature` | Myaaw-specific request header; not part of the core spec |

## 6. Support matrix

| Capability | Myaaw ↔ Myaaw | Myaaw → external | External → Myaaw |
| :--- | :--- | :--- | :--- |
| Agent Card discovery | Yes | Yes | Yes |
| `tasks/send` | Yes | Yes (outbound only) | Yes |
| `message/send` | Yes (alias) | Yes (preferred, `tasks/send` fallback) | Yes (alias) |
| `tasks/get` / `cancel` | Yes | Yes (manual) | Yes |
| Streaming | Yes | Yes (manual) | Yes |
| Async + push | Yes (manual/API) | Yes (manual) | Yes |
| Push notification configs | Yes | N/A | Yes |
| Resubscribe | Yes | N/A | Yes (current process) |
| `input-required` | Yes | Peers vary | Yes |
| Files in/out | Yes | Peers vary | Yes |
| JWS request signing | Yes | Not understood by peers | Myaaw-specific |
| Signed Agent Card | Yes | Verify when key is pinned | Yes |
| Task ownership | Yes | N/A | Yes |
| Per-caller identity | Yes | N/A | Yes |

## 7. Troubleshooting

| Symptom | Likely cause | Fix |
| :--- | :--- | :--- |
| `contact` says `HTTP 401` | Wrong or missing token | Use B's token in A's `contacts.json`; restart B after editing `.env` |
| `contact` says `HTTP 404` | URL is not the JSON-RPC endpoint | Use `<base>/a2a` |
| `contact` says `not found` | Name mismatch | `{"action":"list"}` on A |
| `-32001` on `tasks/get` | Task belongs to another caller | Use the token that created it |
| `-32002` | Resuming a task that is not `input-required` | Omit `id` to create a new task |
| Task `failed` | Receiver has no provider, or run error | Check `status.message` and receiver logs |
| `Contact completed without output` | Peer returned no artifacts, messages, or history | Check the peer response shape |
| `conversation belongs to another user` | A different caller reused the `contextId` | Use a unique context per caller |
| Streaming hangs | Proxy buffers SSE | Disable buffering for `text/event-stream` |
| Request times out | Sync task runs longer than the client timeout | Use `async: true` with a push URL |

Logs:

```bash
tail -f ~/.myaaw/<agent>/logs/myaaw-server-<agent>.log
```

Inspect tasks:

```bash
sqlite3 ~/.myaaw/<agent>/database/myaaw.db \
  "select id, caller, status, context_id from a2a_tasks order by created_at desc limit 10;"
```

## 8. Security checklist

- Use one token per caller (`A2A_CALLERS`) instead of one shared `A2A_TOKEN`.
- Never commit `~/.myaaw/<agent>/.env` or `contacts.json`.
- Keep `contacts.json` and signing keys at mode `0600`.
- Use HTTPS across hosts; bearer tokens and signature headers are plaintext over HTTP.
- Set `A2A_REQUIRE_SIGNATURE=true` only when every caller is Myaaw and has a trusted key.
- Treat `contextId` as a capability: a caller that knows another caller's context cannot use it (Myaaw enforces this), but do not expose IDs unnecessarily.
- Leave the endpoint closed to the public internet unless you control every token.
