# A2A Interop Guide

This guide covers how to connect agents in practice:

1. Myaaw agent ↔ Myaaw agent on the same machine.
2. Myaaw agent ↔ Myaaw agent on different servers.
3. Myaaw agent calling an agent that is **not** Myaaw.
4. An agent that is **not** Myaaw calling a Myaaw agent.

For protocol concepts and internals, see [a2a-protocol.md](a2a-protocol.md). For the day-to-day guide, see [a2a.md](a2a.md).

> In this document, "Myaaw agent" means an agent server built from this project: `myaaw --agent <name> server start`, with its own `~/.myaaw/<name>/` directory, database, tools, and skills.

## 0. The four cases at a glance

| Case | Direction | What you configure | Wire |
| :--- | :--- | :--- | :--- |
| Same machine | Myaaw → Myaaw | Token on B, contact on A | Full (same implementation) |
| Cross server | Myaaw → Myaaw | Token, reachable URL, optional card signing | Full (same implementation) |
| Outbound | Myaaw → external | Contact with the peer's URL and token | `SendMessage` |
| Inbound | External → Myaaw | Token on Myaaw, peer calls `/a2a` | `SendMessage` |

## 1. Common requirements

Every A2A connection needs the same four things.

| Requirement | Detail |
| :--- | :--- |
| **A running agent server** | `myaaw --agent <name> server start`. The A2A routes live on the agent server, not on `myaaw api` and not on `myaaw chat`. |
| **A reachable JSON-RPC endpoint** | `POST http://<host>:<port>/a2a`. Discovery is `GET /.well-known/agent-card.json`. |
| **A credential** | A bearer token in the receiver's `.env`; optionally a named caller with scopes. If every credential variable is empty, the endpoint is **open** and should only be used locally. |
| **A configured agent** | The receiver must have a working LLM provider, because it runs its own agent loop to answer. |

Ports come from `PORT` in `~/.myaaw/<agent>/.env`. In the examples below, the receiver is **B** and the caller is **A**. All JSON-RPC requests need the `A2A-Version: 1.0` header.

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
curl -s http://localhost:8081/.well-known/agent-card.json
```

You should see the Agent Card with `name`, `supportedInterfaces[0].url` (`http://localhost:8081/a2a`), and `skills`.

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

The agent calls the built-in `contact` tool, which sends `SendMessage` to B and returns B's answer.

Inspect what A knows:

```text
{"action":"list"}
{"action":"get","contact":"b"}
```

`get` also fetches B's Agent Card and shows its skills.

### 2.4 Multi-turn (`input-required` / `auth-required`)

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

- `read` → `GetTask`, `ListTasks`, `GetTaskPushNotificationConfig`, `ListTaskPushNotificationConfigs`, `GetExtendedAgentCard`.
- `delegate` → everything `read` allows, plus `SendMessage`, `SendStreamingMessage`, `CancelTask`, `SubscribeToTask`, and the push config write/delete methods.

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

`MYAAW_BASE_URL` must be the address **A can reach**. It becomes the `supportedInterfaces[0].url` of B's Agent Card and the base for `/.well-known/agent-card.json`.

Options:

- Direct: `http://192.168.1.10:8081` on a trusted LAN.
- Reverse proxy: `https://agents.example.com/b` → `http://127.0.0.1:8081`.
- Tunnel: ngrok/Cloudflare Tunnel for testing.

Open the port in the firewall. If you use a reverse proxy, make sure `POST /a2a` and streaming (`text/event-stream`) are both allowed, with buffering disabled for SSE.

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
curl -s https://agents.example.com/b/.well-known/agent-card.json
```

Do not put `localhost` in B's `MYAAW_BASE_URL` when A runs elsewhere.

### 3.3 Signed Agent Cards (optional, recommended across organizations)

Each agent can sign its Agent Card so peers can verify its identity.

On B (receiver), create a key and sign its card:

```bash
openssl genpkey -algorithm ED25519 -out ~/.myaaw/B/a2a_key.pem
chmod 600 ~/.myaaw/B/a2a_key.pem
```

```env
# ~/.myaaw/B/.env
A2A_SIGNING_KEY=/Users/you/.myaaw/B/a2a_key.pem
```

Restart B. Its Agent Card now contains a `signatures` array (the `kid` is in the protected header).

On A (caller), trust B's key. Read B's Agent Card to get its signature, and derive B's public key (base64url Ed25519):

```env
# ~/.myaaw/A/.env
A2A_TRUSTED_KEYS=b:<B-publicKey>
```

Now on A, `contact get` reports `Card signature: verified` when B's key is pinned, otherwise `unverified`.

> Request signing via `X-A2A-Signature` was a Myaaw-specific extension and has been removed in v1.0. A2A v1.0 authenticates requests with bearer tokens only; identity is established by signed Agent Cards.

### 3.4 Common cross-server mistakes

| Mistake | Symptom | Fix |
| :--- | :--- | :--- |
| `MYAAW_BASE_URL` still `localhost` | Card points to the wrong host | Set the reachable URL |
| Contact URL is the Agent Card URL | `HTTP 405` or parse error | Use `/a2a` |
| Token from the wrong agent | `401` | Use the token configured on B |
| Missing `A2A-Version: 1.0` header | `-32009` | Add the header |
| Firewall blocks the port | Connection timeout | Open the port or use a tunnel |
| Proxy buffers SSE | Streaming arrives all at once | Disable buffering for `text/event-stream` |
| B has no provider | Task ends `failed` | Configure a provider on B |

## 4. Myaaw agent calling a non-Myaaw agent

Myaaw's outbound path is the `contact` tool. It sends **`SendMessage`** with a `ROLE_USER` message and text parts, and expects a `SendMessageResponse{task}` result.

> **Binding note:** the `contact` tool speaks the **JSON-RPC** binding only. If the external peer exposes only the REST (`/message:send`) or gRPC (`A2AService.SendMessage`) binding, `contact` cannot call it directly — use a [custom tool](mcp_and_custom_tools.md) or a direct call instead (see §4.5).

### 4.1 What the external peer must provide

| Requirement | Detail |
| :--- | :--- |
| JSON-RPC endpoint | Usually `https://peer.example.com/a2a` |
| Method `SendMessage` | Message carries `role`, `messageId`, and `parts` |
| Result shape | `result.task` with `status.state` (`TASK_STATE_*`), optional `artifacts[].parts[]`, `history[]` |
| Version header | The peer should accept `A2A-Version: 1.0` |
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
  "method": "SendMessage",
  "params": {
    "message": {
      "messageId": "<uuid>",
      "role": "ROLE_USER",
      "parts": [{"text": "..."}]
    }
  }
}
```

When resuming an interrupted task, Myaaw sets `"taskId": "<taskId>"` inside the message.

### 4.4 What Myaaw can read back

- `result.task.status.state` — `TASK_STATE_FAILED` is reported as an error; `TASK_STATE_INPUT_REQUIRED` / `TASK_STATE_AUTH_REQUIRED` are surfaced with `taskId`; `TASK_STATE_CANCELED` is reported; anything else is treated as success.
- `result.task.artifacts[].parts[]` with `text` — preferred.
- `result.task.history[]` with `role` `ROLE_AGENT` and text parts — fallback.

### 4.5 When `contact` is not enough

For a peer with a different method name or response shape, call it directly. The JSON-RPC example:

```bash
curl -s -X POST https://peer.example.com/a2a \
  -H "Authorization: Bearer $PEER_TOKEN" \
  -H "A2A-Version: 1.0" \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"SendMessage",
       "params":{"message":{"role":"ROLE_USER","messageId":"m-1",
       "parts":[{"text":"hello"}]}}}'
```

If the peer only speaks the REST binding:

```bash
curl -s -X POST https://peer.example.com/message:send \
  -H "Authorization: Bearer $PEER_TOKEN" \
  -H "A2A-Version: 1.0" \
  -H 'Content-Type: application/a2a+json' \
  -d '{"message":{"role":"ROLE_USER","messageId":"m-1","parts":[{"text":"hello"}]}}'
```

If the peer only speaks gRPC, use its generated `A2AService` client (the `lf.a2a.v1` service), setting the `a2a-version: 1.0` metadata and a bearer `authorization`.

You can wrap any of these in a custom tool (see [mcp_and_custom_tools.md](mcp_and_custom_tools.md)) if the integration is long-lived.

## 5. A non-Myaaw agent calling a Myaaw agent

A Myaaw agent is a standard A2A v1.0 server that exposes all three standard bindings. Any client that can do JSON-RPC, REST, or gRPC can call it — pick the binding you prefer from the card's `supportedInterfaces` (the first entry is preferred).

### 5.1 Discover

```bash
curl -s https://agents.example.com/b/.well-known/agent-card.json
```

Useful fields:

| Field | Meaning |
| :--- | :--- |
| `supportedInterfaces[]` | One entry per binding: `JSONRPC` (`<base>/a2a`), `HTTP+JSON` (`<base>`), and `GRPC` (`<host>:<port>`, when `A2A_GRPC_PORT` is set) |
| `supportedInterfaces[].protocolBinding` | `JSONRPC` / `HTTP+JSON` / `GRPC` |
| `supportedInterfaces[].protocolVersion` | `1.0` |
| `capabilities.streaming` | `true` |
| `capabilities.pushNotifications` | `true` |
| `skills` | Enabled skills of that agent |
| `securitySchemes` | Present when credentials are configured |
| `signatures` | Present when the card is signed; verify with the peer's pinned public key |

### 5.2 Authenticate

```http
Authorization: Bearer <token>
A2A-Version: 1.0
Content-Type: application/json
```

### 5.3 Send a task

```bash
curl -s -X POST https://agents.example.com/b/a2a \
  -H "Authorization: Bearer $TOKEN" \
  -H "A2A-Version: 1.0" \
  -H 'Content-Type: application/json' \
  -d '{
    "jsonrpc":"2.0",
    "id":1,
    "method":"SendMessage",
    "params":{
      "message":{
        "role":"ROLE_USER",
        "parts":[{"text":"Hello from another agent"}]
      }
    }
  }'
```

### 5.4 Response shape

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "task": {
      "id": "0c4c0c50-...",
      "contextId": "ctx-1",
      "status": {"state": "TASK_STATE_COMPLETED", "timestamp": "..."},
      "history": [
        {"messageId": "…", "role": "ROLE_USER", "parts": [{"text": "Hello"}]},
        {"messageId": "…", "role": "ROLE_AGENT", "parts": [{"text": "Hi"}]}
      ],
      "artifacts": [
        {"artifactId": "…", "name": "response", "parts": [{"text": "Hi"}]}
      ]
    }
  }
}
```

`result.task` is the task; `history` carries the transcript, and `artifacts` carries the final output.

### 5.5 Continue a conversation

Send `contextId` inside the message:

```json
{"jsonrpc":"2.0","id":2,"method":"SendMessage",
 "params":{"message":{"contextId":"ctx-1","role":"ROLE_USER","parts":[{"text":"..."}]}}}
```

The first task with a new `contextId` creates the transcript; later tasks continue it. Contexts are scoped per caller, so another caller cannot reuse them.

### 5.6 Streaming

```bash
curl -N -X POST https://agents.example.com/b/a2a \
  -H "Authorization: Bearer $TOKEN" \
  -H "A2A-Version: 1.0" \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":3,"method":"SendStreamingMessage",
       "params":{"message":{"role":"ROLE_USER","parts":[{"text":"Count to 3"}]}}}'
```

Events (each line is `data: <json>`, a JSON-RPC envelope whose `result` is a `StreamResponse`):

```text
data: {"jsonrpc":"2.0","id":3,"result":{"task":{"id":"...","status":{"state":"TASK_STATE_WORKING"}}}}

data: {"jsonrpc":"2.0","id":3,"result":{"artifactUpdate":{"taskId":"...","artifact":{"artifactId":"…","name":"response","parts":[{"text":"1"}]},"append":true}}}

data: {"jsonrpc":"2.0","id":3,"result":{"artifactUpdate":{"taskId":"...","artifact":{"artifactId":"…","name":"response","parts":[{"text":"1 2 3"}]},"lastChunk":true}}}

data: {"jsonrpc":"2.0","id":3,"result":{"statusUpdate":{"taskId":"...","status":{"state":"TASK_STATE_COMPLETED"},"metadata":{"final":true}}}}
```

### 5.7 Async and push notifications

```json
{
  "jsonrpc":"2.0","id":4,"method":"SendMessage",
  "params":{
    "configuration":{
      "returnImmediately":true,
      "taskPushNotificationConfig":{"url":"https://caller.example.com/hook","token":"hook-secret"}
    },
    "message":{"role":"ROLE_USER","parts":[{"text":"Long job"}]}
  }
}
```

The call returns immediately with `TASK_STATE_SUBMITTED`. When the task finishes, Myaaw POSTs `StreamResponse` events (`application/a2a+json`) to the URL — a `statusUpdate` on each state change plus a final `task` snapshot. A config `token` is sent as `Authorization: Bearer <token>`.

Per-task webhooks can also be stored on the server:

```json
{"jsonrpc":"2.0","id":5,"method":"CreateTaskPushNotificationConfig",
 "params":{"taskId":"<task-id>","url":"https://caller.example.com/hook","token":"hook-secret"}}
```

```json
{"jsonrpc":"2.0","id":6,"method":"ListTaskPushNotificationConfigs","params":{"taskId":"<task-id>"}}
```

```json
{"jsonrpc":"2.0","id":7,"method":"GetTaskPushNotificationConfig","params":{"taskId":"<task-id>","id":"<config-id>"}}
```

```json
{"jsonrpc":"2.0","id":8,"method":"DeleteTaskPushNotificationConfig","params":{"taskId":"<task-id>","id":"<config-id>"}}
```

Stored configs fire on state changes (including terminal states), and deleting one is idempotent.

### 5.8 Ask for more information (`input-required` / `auth-required`)

If the Myaaw agent calls its `a2a_input` tool, the task stops at:

```json
"status": {"state": "TASK_STATE_INPUT_REQUIRED", "message": {"role": "ROLE_AGENT", "parts": [{"text": "Which date?"}]}}
```

Resume with `SendMessage` and the **same task ID** in `message.taskId`:

```json
{"jsonrpc":"2.0","id":5,"method":"SendMessage",
 "params":{"message":{"role":"ROLE_USER","taskId":"<task-id>","parts":[{"text":"2026-10-01"}]}}}
```

`auth-required` works the same way, produced by the `a2a_auth_request` tool. Only the caller that created the task may resume it.

### 5.9 Files

Input file parts use a `raw` part (base64) with `filename` and `mediaType`:

```json
{"raw":"<base64>","filename":"report.pdf","mediaType":"application/pdf"}
```

- Maximum 10 file parts per message.
- Maximum 5 MiB per decoded file.
- `image/*` is passed to the model as an image.
- Other files are written to the task directory and referenced by path.

Files the agent writes under the task's `output/` directory are returned as artifacts (up to 10 files, 1 GiB each): files ≤ 1 MiB as `raw` (base64), larger files as `url` parts pointing to `GET /tasks/{id}/artifacts?path=<name>` (streamed, authenticated as the task owner).

### 5.10 Get, list, and cancel

```json
{"jsonrpc":"2.0","id":6,"method":"GetTask","params":{"id":"<task-id>"}}
{"jsonrpc":"2.0","id":7,"method":"ListTasks","params":{"pageSize":10}}
{"jsonrpc":"2.0","id":8,"method":"CancelTask","params":{"id":"<task-id>"}}
```

`CancelTask` stops the running agent loop and keeps the task in `TASK_STATE_CANCELED`. Canceling a terminal task returns `-32002` (`TaskNotCancelableError`).

### 5.11 Errors and scopes

Standard JSON-RPC codes (`-32700`, `-32600`, `-32601`, `-32602`, `-32603`) plus the A2A-specific codes (each with a `google.rpc.ErrorInfo` in `error.data`):

| Code | Meaning |
| :--- | :--- |
| `-32001` | Task not found, or not owned by this caller |
| `-32002` | Task is not cancelable (terminal) |
| `-32003` | Push notifications not supported |
| `-32004` | Unsupported operation (e.g. resuming a non-interrupted task) |
| `-32005` | Content type not supported |
| `-32006` | Invalid agent response |
| `-32007` | Extended agent card not configured |
| `-32008` | Extension support required |
| `-32009` | Version not supported (wrong `A2A-Version`) |

Scope rules:

- `read` → read methods only.
- `delegate` → write methods plus read.

Task ownership: only the caller that created a task may get, cancel, subscribe to, or resume it. Legacy tasks without a recorded caller remain accessible.

### 5.12 Reliability notes for external callers

| Feature | Status |
| :--- | :--- |
| `SendMessage` | Supported. |
| `message.contextId` | Supported; scoped per caller. |
| `message.taskId` resume | Supported for interrupted tasks. |
| Task `history` | Returned (the transcript); there is no `messages` field. |
| `SubscribeToTask` | Available for streams started by the current process; the buffer is in memory, expires after 10 minutes, and is lost on restart. |
| Push notification configs | Stored in SQLite and survive restarts. |
| Signed Agent Card | Available; JWS over the RFC 8785 canonical card, verified against pinned keys. |
| Extended Agent Card | Available when `A2A_EXTENDED_CARD` is configured; `GetExtendedAgentCard` returns the merged card. |

### 5.13 Choosing a transport binding

The same operations are available over all three bindings; the examples above use JSON-RPC. To use the others:

**HTTP+JSON (REST):** same payloads, resource-based URLs, `Content-Type: application/a2a+json`:

```bash
curl -s -X POST https://agents.example.com/b/message:send \
  -H "Authorization: Bearer $TOKEN" \
  -H "A2A-Version: 1.0" \
  -H 'Content-Type: application/a2a+json' \
  -d '{"message":{"role":"ROLE_USER","parts":[{"text":"Hello"}]}}'

curl -s "https://agents.example.com/b/tasks?pageSize=10" \
  -H "Authorization: Bearer $TOKEN" \
  -H "A2A-Version: 1.0"
```

REST errors use `google.rpc.Status` (`error.code`, `error.status`, `error.details[].@type`).

**gRPC:** use the `lf.a2a.v1.A2AService` client, send `a2a-version: 1.0` (and `authorization: Bearer <token>`) as gRPC metadata. Errors use gRPC `codes` with a `google.rpc.ErrorInfo` detail. The gRPC binding is only present when `A2A_GRPC_PORT` is set on the Myaaw agent.

## 6. Support matrix

| Capability | Myaaw ↔ Myaaw | Myaaw → external | External → Myaaw |
| :--- | :--- | :--- | :--- |
| JSON-RPC binding | Yes | Yes (`contact`) | Yes |
| HTTP+JSON (REST) binding | Yes | Manual/custom tool | Yes |
| gRPC binding | Yes | Manual/custom tool | Yes (when `A2A_GRPC_PORT` set) |
| Agent Card discovery | Yes | Yes | Yes |
| `SendMessage` | Yes | Yes | Yes |
| `SendStreamingMessage` | Yes | Peer-dependent | Yes |
| `GetTask` / `ListTasks` / `CancelTask` | Yes | Peer-dependent | Yes |
| `SubscribeToTask` | Yes | Peer-dependent | Yes (current process) |
| Async + push | Yes | Peer-dependent | Yes |
| Push notification configs | Yes | Peer-dependent | Yes |
| `input-required` / `auth-required` | Yes | Peers vary | Yes |
| Files in/out | Yes | Peers vary | Yes |
| Signed Agent Card | Yes | Verify when key is pinned | Yes |
| Task ownership | Yes | N/A | Yes |
| Per-caller identity | Yes | N/A | Yes |
| Extended Agent Card | Yes | Peer-dependent | Yes |

## 7. Troubleshooting

| Symptom | Likely cause | Fix |
| :--- | :--- | :--- |
| `contact` says `HTTP 401` | Wrong or missing token | Use B's token in A's `contacts.json`; restart B after editing `.env` |
| `contact` says `HTTP 404` | URL is not the JSON-RPC endpoint | Use `<base>/a2a` |
| `contact` says `not found` | Name mismatch | `{"action":"list"}` on A |
| `-32009` | Missing/wrong `A2A-Version` | Send `A2A-Version: 1.0` |
| `-32001` on `GetTask` | Task belongs to another caller | Use the token that created it |
| `-32004` | Resuming a task that is not interrupted | Omit `taskId` to create a new task |
| Task `failed` | Receiver has no provider, or run error | Check `status.message` and receiver logs |
| `Contact completed without output` | Peer returned no artifacts or history | Check the peer response shape |
| `conversation belongs to another user` | A different caller reused the `contextId` | Use a unique context per caller |
| Streaming hangs | Proxy buffers SSE | Disable buffering for `text/event-stream` |
| Request times out | Sync task runs longer than the client timeout | Use `configuration.returnImmediately` with a push URL |

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
- Use HTTPS across hosts; bearer tokens are plaintext over HTTP.
- Pin peer public keys with `A2A_TRUSTED_KEYS` to verify signed Agent Cards.
- Keep the webhook SSRF guard on (`A2A_ALLOW_PRIVATE_WEBHOOKS` is for local testing only).
- Treat `contextId` as a capability: a caller that knows another caller's context cannot use it (Myaaw enforces this), but do not expose IDs unnecessarily.
- Leave the endpoint closed to the public internet unless you control every token.
