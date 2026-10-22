# Agent2Agent (A2A) Guide

Myaaw agents can delegate tasks to other agents using the [Agent2Agent (A2A) protocol v1.0](https://a2a-protocol.org/latest/specification/). Each agent server exposes an Agent Card for discovery and a JSON-RPC 2.0 endpoint for tasks. A built-in `contact` tool lets the agent call agents listed in its contact book.

> For the protocol model, task lifecycle, storage, request flows, security boundaries, and implementation details, see [a2a-protocol.md](a2a-protocol.md). For connection recipes (same machine, cross server, Myaaw to external, external to Myaaw), see [a2a-interop.md](a2a-interop.md). The section-by-section implementation validation is in [a2a-spec-compliance.md](a2a-spec-compliance.md).

Implemented:

- `SendMessage` (blocking), `SendMessage` with `configuration.returnImmediately` (async), and `SendStreamingMessage` (SSE `data:` lines).
- `GetTask`, `ListTasks`, `CancelTask`, and `SubscribeToTask`.
- `TASK_STATE_*` states, `ROLE_*` roles, and member-discriminated parts (`text`/`raw`/`url`/`data`).
- Multi-turn via `message.taskId` with `TASK_STATE_INPUT_REQUIRED` and `TASK_STATE_AUTH_REQUIRED`.
- Push notifications to a webhook (`StreamResponse` payload, `application/a2a+json`, retry with backoff).
- Push notification config CRUD: `CreateTaskPushNotificationConfig`, `GetTaskPushNotificationConfig`, `ListTaskPushNotificationConfigs`, `DeleteTaskPushNotificationConfig`.
- Text, data, URL, and raw file input parts (images are passed to the model; other files are written to disk).
- File output artifacts from the task `output/` directory.
- Per-caller bearer tokens and scopes via `A2A_TOKEN`, `A2A_TOKENS`, and `A2A_CALLERS`.
- Signed Agent Cards (JWS `EdDSA` over an RFC 8785 JCS canonical form, `typ:"JOSE"`).
- An authenticated extended Agent Card via `A2A_EXTENDED_CARD` and `GetExtendedAgentCard`.
- Webhook SSRF guard, per-caller rate limiting, and an audit log.
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
| `read` | `GetTask`, `ListTasks`, `GetTaskPushNotificationConfig`, `ListTaskPushNotificationConfigs`, `GetExtendedAgentCard` |
| `delegate` | Everything `read` allows, plus `SendMessage`, `SendStreamingMessage`, `CancelTask`, `SubscribeToTask`, and the push config write/delete methods |

Plain `A2A_TOKEN` / `A2A_TOKENS` entries get delegate access under the shared `default` identity. `A2A_CALLERS` entries look like `name:token` or `name:token:read|delegate`; an entry without a scope list gets delegate. A caller that lacks the required scope receives HTTP 403.

### Signed Agent Cards

Generate an Ed25519 key and point the agent at it:

```bash
openssl genpkey -algorithm ED25519 -out ~/.myaaw/<agent>/a2a_key.pem
chmod 600 ~/.myaaw/<agent>/a2a_key.pem

# ~/.myaaw/<agent>/.env
A2A_SIGNING_KEY=/Users/you/.myaaw/<agent>/a2a_key.pem
```

The Agent Card then publishes a `signatures` array. To trust a peer, copy its key (the peer's public key is `base64url`-encoded; you can derive it from the same `a2a_key.pem`, or the peer can give it to you):

```bash
A2A_TRUSTED_KEYS=peer-name:base64url-public-key
```

`contact get` reports `Card signature: verified` (or `unverified`) against `A2A_TRUSTED_KEYS`.

## 2. Agent Card (discovery)

```bash
curl http://localhost:8080/.well-known/agent-card.json
```

```json
{
  "name": "myfriend",
  "description": "Myaaw agent myfriend",
  "supportedInterfaces": [
    {
      "url": "http://localhost:8080/a2a",
      "protocolBinding": "JSONRPC",
      "protocolVersion": "1.0"
    },
    {
      "url": "http://localhost:8080",
      "protocolBinding": "HTTP+JSON",
      "protocolVersion": "1.0"
    }
  ],
  "version": "1.0",
  "capabilities": {"streaming": true, "pushNotifications": true},
  "defaultInputModes": ["text/plain", "application/json"],
  "defaultOutputModes": ["text/plain"],
  "skills": [
    {"id": "weather", "name": "Weather", "description": "Fetch weather", "tags": ["weather"]}
  ],
  "securitySchemes": {
    "bearer": {"httpAuthSecurityScheme": {"scheme": "bearer"}}
  },
  "securityRequirements": [
    {"schemes": {"bearer": {}}}
  ],
  "signatures": [
    {"protected": "base64url-header", "signature": "base64url-signature"}
  ]
}
```

- `securitySchemes` and `securityRequirements` appear only when authentication is configured.
- `signatures` appears only when `A2A_SIGNING_KEY` is configured. The protected header carries `alg`, `typ:"JOSE"`, and `kid`.
- `capabilities.extendedAgentCard` is set when `A2A_EXTENDED_CARD` is configured.
- `provider`, `documentationUrl`, and `iconUrl` are driven by `A2A_PROVIDER_URL` / `A2A_PROVIDER_ORGANIZATION`, `A2A_DOCUMENTATION_URL`, and `A2A_ICON_URL`.
- The card never contains secrets. It supports `ETag` / `If-None-Match` → `304 Not Modified`.

## 3. Send a task

All calls go to `POST /a2a` with a JSON-RPC 2.0 envelope and the `A2A-Version: 1.0` header.

```bash
curl -X POST http://localhost:8080/a2a \
  -H "Authorization: Bearer $A2A_TOKEN" \
  -H "A2A-Version: 1.0" \
  -H 'Content-Type: application/json' \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "SendMessage",
    "params": {
      "message": {
        "role": "ROLE_USER",
        "parts": [{"text": "Rangkum file AGENTS.md"}]
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
    "task": {
      "id": "4f4c...",
      "contextId": "e2b1...",
      "status": {"state": "TASK_STATE_COMPLETED", "timestamp": "2026-10-22T09:00:00.000Z"},
      "history": [
        {"messageId": "…", "role": "ROLE_USER", "parts": [{"text": "Rangkum file AGENTS.md"}]},
        {"messageId": "…", "role": "ROLE_AGENT", "parts": [{"text": "…"}]}
      ],
      "artifacts": [
        {"artifactId": "…", "name": "response", "parts": [{"text": "…"}]}
      ]
    }
  }
}
```

- `contextId` is the agent's conversation ID. Pass it back in `message.contextId` to continue the same conversation.
- A failed run returns `TASK_STATE_FAILED` with an explanation in `status.message`.
- Tasks are stored in the agent's SQLite database (`a2a_tasks`), so `GetTask` works after the call or after a reconnect.
- `historyLength` in `configuration` controls how much history is returned: omitted keeps everything, `0` omits history, and `n` keeps the last `n` messages.

### Async tasks and push notifications

Set `configuration.returnImmediately` to return immediately while the task runs in the background. Optionally set `configuration.taskPushNotificationConfig` to receive updates over HTTP POST.

```bash
curl -X POST http://localhost:8080/a2a \
  -H "Authorization: Bearer $A2A_TOKEN" \
  -H "A2A-Version: 1.0" \
  -H 'Content-Type: application/json' \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "SendMessage",
    "params": {
      "configuration": {
        "returnImmediately": true,
        "taskPushNotificationConfig": {"url": "https://your-host/a2a-callback", "token": "hook-token"}
      },
      "message": {"role": "ROLE_USER", "parts": [{"text": "Riset panjang..."}]}
    }
  }'
```

The response is a task in `TASK_STATE_SUBMITTED` (then `TASK_STATE_WORKING`). Poll `GetTask` or wait for the callback. The callback receives `StreamResponse` events (`application/a2a+json`): a `statusUpdate` for each state change and, on terminal states, a final `task` snapshot. Webhook deliveries are retried with exponential backoff (at-least-once), so receivers should be idempotent. Async runs are serialized per agent so two long tasks do not race the agent loop. If the server restarts while a task is `submitted`/`working`, it is marked `failed` on startup instead of hanging forever.

### Input-required and auth-required (multi-turn)

The agent can pause a task and ask the caller for more information with the built-in `a2a_input` tool, or request authentication with `a2a_auth_request`. The task becomes `TASK_STATE_INPUT_REQUIRED` or `TASK_STATE_AUTH_REQUIRED` with the request in `status.message`. Resume by calling `SendMessage` again with the **same task id** in `message.taskId`:

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "SendMessage",
  "params": {
    "message": {"role": "ROLE_USER", "taskId": "4f4c...", "parts": [{"text": "Jakarta"}]}
  }
}
```

The existing `contextId` is reused, so the agent continues the same conversation. A `taskId` that is not in an interrupted state returns error `-32004` (`UnsupportedOperationError`), and a mismatched `contextId` returns an invalid-params error.

### Output artifacts

Before each run, the message receives a note with the task output directory:

```text
[A2A] Save files for the caller under ~/.myaaw/<agent>/jobs/a2a/<taskId>/output; they will be returned as artifacts.
```

Files written there are returned as artifacts, together with the text artifact. Small files (≤ 1 MiB) are inlined as `raw` parts (base64); larger files are returned as `url` parts pointing to `GET /tasks/{id}/artifacts?path=<name>` and streamed on demand, so a large file (for example a video) does not have to fit in a single JSON response. Limits: 10 files per task, up to 1 GiB per file. Symlinks are skipped.

The `contact` tool downloads file artifacts (both `raw` and `url` parts) into the caller's `~/.myaaw/<agent>/home/workspace/downloads/<taskId>/` and reports each saved path and size.

### File parts

```json
{
  "parts": [
    {"raw": "<base64>", "filename": "photo.jpg", "mediaType": "image/jpeg"}
  ]
}
```

- `mediaType: image/*` parts are passed to the model as images.
- Other `raw` parts are written to `~/.myaaw/<agent>/jobs/a2a/<taskId>/` and their absolute path is appended to the message text, so the agent can read them with its filesystem tool.
- `url` parts are passed through as file references; `data` parts are encoded as JSON.
- Limits: 10 files per message, 5 MiB per file.

### Streaming (`SendStreamingMessage`)

Use `SendStreamingMessage` to receive status and artifact updates over SSE (`data:` lines):

```bash
curl -N -X POST http://localhost:8080/a2a \
  -H "Authorization: Bearer $A2A_TOKEN" \
  -H "A2A-Version: 1.0" \
  -H 'Content-Type: application/json' \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "SendStreamingMessage",
    "params": {
      "message": {"role": "ROLE_USER", "parts": [{"text": "Rangkum file AGENTS.md"}]}
    }
  }'
```

```text
data: {"jsonrpc":"2.0","id":1,"result":{"task":{"id":"4f4c...","status":{"state":"TASK_STATE_WORKING", ...}}}}

data: {"jsonrpc":"2.0","id":1,"result":{"artifactUpdate":{"taskId":"4f4c...","artifact":{"artifactId":"…","name":"response","parts":[{"text":"Ringkasan "}]},"append":true}}}

data: {"jsonrpc":"2.0","id":1,"result":{"artifactUpdate":{"taskId":"4f4c...","artifact":{"artifactId":"…","name":"response","parts":[{"text":"Ringkasan lengkap..."}]},"lastChunk":true}}}

data: {"jsonrpc":"2.0","id":1,"result":{"statusUpdate":{"taskId":"4f4c...","status":{"state":"TASK_STATE_COMPLETED", ...},"metadata":{"final":true}}}}
```

Rules:

- The first event is the task itself, then artifact updates, then a final status update with `metadata.final: true`.
- `append: true` means the text is a delta to append.
- The final artifact (`lastChunk: true`, no `append`) replaces the accumulated deltas with the authoritative answer.
- The task is still persisted, so `GetTask` works after the stream or after a reconnect.

### Subscribe to an existing stream (`SubscribeToTask`)

`SubscribeToTask` replays a running or recently finished stream from an in-memory buffer (retained for 10 minutes):

```json
{"jsonrpc": "2.0", "id": 2, "method": "SubscribeToTask", "params": {"id": "TASK_ID"}}
```

Subscribing to a task in a terminal state returns `-32004` (`UnsupportedOperationError`).

### Get / list / cancel

```bash
# Get a task
curl -X POST http://localhost:8080/a2a \
  -H "Authorization: Bearer $A2A_TOKEN" -H "A2A-Version: 1.0" \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":2,"method":"GetTask","params":{"id":"TASK_ID"}}'

# List tasks (optional filters: contextId, status, pageSize, pageToken, statusTimestampAfter, includeArtifacts)
curl -X POST http://localhost:8080/a2a \
  -H "Authorization: Bearer $A2A_TOKEN" -H "A2A-Version: 1.0" \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":2,"method":"ListTasks","params":{"pageSize":10}}'

# Cancel a non-terminal task
curl -X POST http://localhost:8080/a2a \
  -H "Authorization: Bearer $A2A_TOKEN" -H "A2A-Version: 1.0" \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":3,"method":"CancelTask","params":{"id":"TASK_ID"}}'
```

Cancelling a terminal task returns `-32002` (`TaskNotCancelableError`).

### Error codes

Standard JSON-RPC codes (`-32700` parse, `-32600` invalid request, `-32601` method not found, `-32602` invalid params, `-32603` internal) plus the A2A-specific codes, which also carry a `google.rpc.ErrorInfo` detail in `error.data`:

| Code | Error type |
| :--- | :--- |
| `-32001` | `TaskNotFoundError` |
| `-32002` | `TaskNotCancelableError` |
| `-32003` | `PushNotificationNotSupportedError` |
| `-32004` | `UnsupportedOperationError` |
| `-32005` | `ContentTypeNotSupportedError` |
| `-32006` | `InvalidAgentResponseError` |
| `-32007` | `ExtendedAgentCardNotConfiguredError` |
| `-32008` | `ExtensionSupportRequiredError` |
| `-32009` | `VersionNotSupportedError` |

### REST (HTTP+JSON) binding

The same operations are also available over the REST binding (content type `application/a2a+json`):

| Method + path | Purpose |
| :--- | :--- |
| `POST /message:send` | Send a message (`SendMessageRequest` body, `SendMessageResponse` result). |
| `POST /message:stream` | Stream a message (SSE `data:` lines carrying `StreamResponse`). |
| `GET /tasks` | List tasks (`contextId`, `status`, `pageSize`, `pageToken`, `historyLength`, `statusTimestampAfter`, `includeArtifacts` as query params). |
| `GET /tasks/{id}` | Get a task (`historyLength` query param). |
| `POST /tasks/{id}:cancel` | Cancel a task. |
| `POST /tasks/{id}:subscribe` | Subscribe to a task stream (SSE). |
| `POST /tasks/{id}/pushNotificationConfigs` | Create a push config. |
| `GET /tasks/{id}/pushNotificationConfigs/{configId}` | Get a push config. |
| `GET /tasks/{id}/pushNotificationConfigs` | List push configs. |
| `DELETE /tasks/{id}/pushNotificationConfigs/{configId}` | Delete a push config. |
| `GET /extendedAgentCard` | Get the authenticated extended card. |

REST errors use the `google.rpc.Status` shape (`error.code`, `error.status`, `error.message`, and `error.details[]` with `@type` and `reason`/`domain`).

### gRPC binding

A gRPC server exposing `lf.a2a.v1.A2AService` (the official proto, all 11 operations) is available when you set a port:

```bash
# ~/.myaaw/<agent>/.env
A2A_GRPC_PORT=8082
# optional: full bind address (overrides A2A_GRPC_PORT)
# A2A_GRPC_ADDR=0.0.0.0:8082
# optional: TLS
# A2A_GRPC_TLS_CERT=/path/to/server.crt
# A2A_GRPC_TLS_KEY=/path/to/server.key
```

Clients send the `a2a-version: 1.0` metadata header and, when auth is configured, an `authorization` bearer token. Errors use gRPC `codes` plus a `google.rpc.ErrorInfo` detail. When enabled, the Agent Card advertises a `GRPC` entry in `supportedInterfaces`.

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
| `send` | Delegate a task and return the final answer | `contact`, `message`, optional `taskId`, `async` |
| `status` | Poll a task started with `async: true` | `contact`, `taskId` |
| `list` | Show configured contacts (tokens are masked) | - |
| `get` | Show one contact plus its Agent Card capabilities and signature status | `contact` |
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

Aliases are accepted for convenience: `send_message`, `poll` (for `status`), `list_contacts`, `get_contact`, `add_contact`.

For long tasks, set `async: true` on `send`: it returns `[async taskId=…]` immediately (the remote receives `configuration.returnImmediately`), and `status` polls it with `GetTask` until it completes.

On `send` the tool:

1. Looks up the contact by name.
2. Calls `SendMessage` on the contact's URL with its bearer token and the `A2A-Version: 1.0` header.
3. Waits for the task to finish.
4. Returns the artifact text as the tool result, so the local agent can continue its loop.

When the remote agent asks for more information (`input-required`) or authentication (`auth-required`), the result starts with a machine-readable header:

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

Without `taskId`, each `send` starts a new task. A `taskId` that is no longer waiting for input is rejected with a readable error. A canceled remote task is reported as canceled. A failed task or unknown contact is returned as a readable error string to the model; unknown contacts also report the available names.

`get` fetches the remote `/.well-known/agent-card.json` (5 second timeout) and reports its skills and signature status; when the remote agent is offline it falls back to the local contact metadata. `add` validates the name and URL the same way as the control-plane API, writes `contacts.json` atomically with mode `0600`, and never echoes the token back. Calls without `action` that include `contact` and `message` are still treated as `send` for backwards compatibility.

### Scope of the `contact` tool

The server implements the full A2A v1.0 surface; the tool is a delegation client and exposes only the part that fits an LLM tool call.

| A2A v1.0 capability | Server | `contact` tool |
| --- | :---: | --- |
| Agent Card (public) | yes | yes (`get`) |
| Extended Agent Card | yes | no |
| `SendMessage` | yes | yes (`send`) |
| `SendStreamingMessage` | yes | no |
| `GetTask` | yes | yes (`status`) |
| `ListTasks` | yes | no |
| `CancelTask` | yes | no |
| `SubscribeToTask` | yes | no |
| Push notification configs | yes | no |
| File artifacts (`raw` / `url`) | yes | yes (auto-download) |

This gap is by design rather than a protocol limitation:

- Streaming and subscribe need a long-lived connection, while a tool call is one-shot request/response. A `watch` action would block the agent turn for the entire task; polling `status` lets the agent interleave other work.
- Push configs require an HTTP receiver in the calling agent plus a mapping from webhook to agent and chat session. That is new infrastructure, not just another action.
- `CancelTask` and `ListTasks` have no consumer in the delegation flow yet: the caller keeps the `taskId` returned by `send`, so enumeration is rarely needed. Cancel is the cheapest next addition when a task must be aborted.

All of them remain available over JSON-RPC, REST, and gRPC, so interop with other A2A implementations is unaffected. The planned order for extending the tool is `cancel` and `tasks`, then a standalone progress endpoint (not a blocking `watch` action), and push last as a separate feature.

## 6. Security notes

- `A2A_TOKEN`, `A2A_TOKENS`, `A2A_CALLERS`, and `contacts.json` contain secrets. Use file permissions `0600`; never commit them.
- The Agent Card is public metadata and never includes tokens. The `kid` in a signature is a public key identifier, safe to publish.
- Tokens are compared in constant time.
- Use HTTPS for contacts across hosts; bearer tokens travel in clear text over plain HTTP.
- Prefer `A2A_CALLERS` with one token per caller so access can be revoked per agent and scopes can be narrowed.
- `A2A_TRUSTED_KEYS` verifies signed Agent Cards; `contact get` reports the result.
- Webhook URLs are validated against loopback, private, link-local, and shared-address ranges (SSRF guard). Set `A2A_ALLOW_PRIVATE_WEBHOOKS=true` only for local testing.
- Rate limiting per caller is enabled with `A2A_RATE_LIMIT` (requests/second) and `A2A_RATE_BURST`; exceeding the limit returns HTTP 429.
- A2A transcripts use a synthetic local user per calling agent: `a2a:<caller>` with IDs starting at `-1001`. They do not appear in the terminal or default control-plane conversation list, and each caller gets an isolated conversation namespace.
- `GetTask`, `CancelTask`, `SubscribeToTask`, push config operations, and interrupted-task resume are limited to the caller that created the task. Legacy tasks without a recorded caller remain readable.
- The `add` action lets the agent register a new endpoint. A prompt-injected model could add an attacker-controlled URL and send data to it. Block the tool (`AGENT_BLOCKED_TOOLS=contact`) or keep `contacts.json` read-only for agents that must not manage contacts.

## 7. Limitations / next phase

- Async retries happen on restart only, capped at three attempts.
- `SubscribeToTask` buffers live in memory and expire after 10 minutes.
- DNS names for webhooks are validated by name only; resolution is left to the delivery client (a documented SSRF limitation).
- The `contact` tool covers `send`, `status`, artifact download, and contact management only; cancel, task listing, streaming, subscribe, extended card, and push configs are server-only for now. See the coverage table in [section 5](#scope-of-the-contact-tool).
- Contact HTTP calls use a 10 minute timeout, including artifact downloads. A very large artifact over a slow link may exceed it.

All three protocol bindings (JSON-RPC, HTTP+JSON/REST, and gRPC) are implemented.
