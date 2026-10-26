# A2A Protocol and Myaaw Implementation

This document describes the Agent2Agent (A2A) protocol model and how Myaaw implements it, following the [A2A Specification v1.0](https://a2a-protocol.org/latest/specification/). For a quick start, see [a2a.md](a2a.md); for connection recipes, see [a2a-interop.md](a2a-interop.md); the implementation validation is in [a2a-spec-compliance.md](a2a-spec-compliance.md).

## 1. Why A2A

Agents need a standard way to delegate work to each other — regardless of framework, language, or host. A2A defines agent discovery (an Agent Card), a task model, and a JSON-RPC 2.0 method surface. Myaaw implements the JSON-RPC binding of A2A v1.0 on both sides: as a **server** (each agent exposes `/a2a`) and as a **client** (the `contact` tool).

## 2. Protocol model

### 2.1 Discovery: the Agent Card

The Agent Card is the self-describing manifest served at `/.well-known/agent-card.json`. Key fields:

| Field | Meaning |
| :--- | :--- |
| `name`, `description` | Human-readable identity and purpose. |
| `supportedInterfaces` | Ordered list of `AgentInterface` entries; the first is preferred. Each has `url`, `protocolBinding` (`JSONRPC`), `protocolVersion` (`1.0`), and optional `tenant`. |
| `version` | The A2A protocol version this agent speaks (`1.0`). |
| `provider` | Optional `{url, organization}` from `A2A_PROVIDER_URL` / `A2A_PROVIDER_ORGANIZATION`. |
| `documentationUrl`, `iconUrl` | Optional metadata. |
| `capabilities` | `streaming`, `pushNotifications`, and `extendedAgentCard` (when configured). |
| `securitySchemes` | A map of named `SecurityScheme` objects; Myaaw uses `{"bearer": {"httpAuthSecurityScheme": {"scheme": "bearer"}}}` when auth is configured. |
| `securityRequirements` | An array binding operations/skills to schemes; Myaaw emits `[{"schemes": {"bearer": {}}}]`. |
| `defaultInputModes` / `defaultOutputModes` | Media types across all skills. |
| `skills` | Array of `AgentSkill` (`id`, `name`, `description`, `tags`, optional `examples`/`inputModes`/`outputModes`/`securityRequirements`). |
| `signatures` | Optional JWS signatures (see 2.9). |

### 2.2 Tasks and states

A task is the unit of work. Its wire state is the `TASK_STATE_*` enum:

| Wire state | Internal storage | Meaning |
| :--- | :--- | :--- |
| `TASK_STATE_SUBMITTED` | `submitted` | Accepted, queued (async). |
| `TASK_STATE_WORKING` | `working` | Running. |
| `TASK_STATE_INPUT_REQUIRED` | `input-required` | Waiting for the caller (interrupted). |
| `TASK_STATE_AUTH_REQUIRED` | `auth-required` | Waiting for authentication (interrupted). |
| `TASK_STATE_COMPLETED` | `completed` | Terminal success. |
| `TASK_STATE_FAILED` | `failed` | Terminal failure. |
| `TASK_STATE_CANCELED` | `canceled` | Terminal cancellation. |
| `TASK_STATE_REJECTED` | `rejected` | Terminal rejection. |
| `TASK_STATE_UNSPECIFIED` | `unspecified` | Unknown. |

Myaaw stores the lowercase internal form in SQLite and converts at the marshaling boundary, so the database does not change when the wire enum changes.

### 2.3 Messages, parts, and artifacts

A `Message` has `messageId`, `role` (`ROLE_USER` / `ROLE_AGENT`), `parts`, and optional `contextId`, `taskId`, `metadata`, `extensions`, and `referenceTaskIds`.

A `Part` is member-discriminated — exactly one of `text`, `raw`, `url`, or `data`, plus optional `filename` and `mediaType`:

```json
{"text": "Hello"}
{"raw": "<base64>", "filename": "report.txt", "mediaType": "text/plain"}
{"url": "https://example.com/file.pdf"}
{"data": {"key": "value"}}
```

An `Artifact` has a required `artifactId` plus `name`, `description`, `parts`, and `metadata`.

On the wire the task transcript is the `history` field (never `messages`). Roles are `ROLE_*`, not lowercase.

### 2.4 JSON-RPC surface

`POST /a2a`, JSON-RPC 2.0, with the `A2A-Version: 1.0` header (or query parameter). Methods:

| Method | Purpose |
| :--- | :--- |
| `SendMessage` | Create and run a task (blocking until terminal or interrupted, or return immediately with `configuration.returnImmediately`). Idempotent per `(caller, messageId)`: retrying the same message returns its existing task. |
| `SendStreamingMessage` | Like `SendMessage` but emits `StreamResponse` events as `data:` lines. |
| `GetTask` | Fetch one task (`id`, optional `historyLength`). |
| `ListTasks` | Cursor-paginated list with filters. |
| `CancelTask` | Cancel a non-terminal task. |
| `SubscribeToTask` | Replay a running/recent stream from the in-memory buffer. |
| `CreateTaskPushNotificationConfig` | Register a webhook for a task. |
| `GetTaskPushNotificationConfig` | Fetch one webhook config. |
| `ListTaskPushNotificationConfigs` | List webhook configs for a task. |
| `DeleteTaskPushNotificationConfig` | Delete a webhook config (idempotent). |
| `GetExtendedAgentCard` | Fetch the authenticated extended card (when configured). |

`SendMessage` returns a `SendMessageResponse` oneof — `{"task": {...}}` or `{"message": {...}}`. The other task methods return the task (or a `ListTasksResponse` / config) directly.

### 2.5 Streaming

Streaming uses the `StreamResponse` oneof:

```json
{"task": {...}}
{"message": {...}}
{"statusUpdate": {"taskId": "...", "contextId": "...", "status": {...}, "metadata": {"final": true}}}
{"artifactUpdate": {"taskId": "...", "artifact": {...}, "append": true, "lastChunk": false}}
```

`SendStreamingMessage` emits the task first, then artifact updates (`append` for deltas, `lastChunk` for the authoritative artifact), then a final `statusUpdate` with `metadata.final: true`. `SubscribeToTask` replays buffered events (retained 10 minutes) and forwards live ones.

### 2.6 Async and push notifications

`SendMessage` with `configuration.returnImmediately: true` returns a `submitted` task and runs it in the background. `configuration.taskPushNotificationConfig` (or the push config CRUD methods) registers a webhook. The webhook receives `StreamResponse` events (`Content-Type: application/a2a+json`): a `statusUpdate` on each state change, plus a final `task` snapshot for terminal states. Deliveries retry with exponential backoff (at-least-once).

### 2.7 Multi-turn

A task can be interrupted by the agent (via `a2a_input` → `input-required`, or `a2a_auth_request` → `auth-required`). The caller resumes with `SendMessage` and `message.taskId` set to the original task id. Myaaw infers `contextId` from the task id and rejects a mismatched `contextId`+`taskId` pair.

### 2.8 Authentication and scopes

Authentication is bearer-token based, declared in the card via `securitySchemes` / `securityRequirements`. Myaaw accepts three credential sources:

- `A2A_TOKEN` — one token, identity `default`, delegate scope.
- `A2A_TOKENS` — several tokens, identity `default`, delegate scope.
- `A2A_CALLERS` — `name:token[:scope|scope]` entries with `read` or `delegate` scopes.

`delegate` implies `read`. Read-scoped callers may only call read methods; write methods return HTTP 403.

### 2.9 Signed Agent Cards

Agent Cards may be signed with JWS (`EdDSA`). Myaaw canonicalizes the card with RFC 8785 (JCS) — excluding the `signatures` field — and signs with a protected header of `{"alg":"EdDSA","typ":"JOSE","kid":"..."}`. Verification honors the protected header and requires the `kid` to resolve in `A2A_TRUSTED_KEYS`.

### 2.10 REST (HTTP+JSON) binding

Myaaw also exposes the same operations over HTTP+JSON (spec §11):

| Method + path | Operation |
| :--- | :--- |
| `POST /message:send` | `SendMessage` |
| `POST /message:stream` | `SendStreamingMessage` (SSE `data:` `StreamResponse` lines) |
| `GET /tasks/{id}` | `GetTask` |
| `GET /tasks` | `ListTasks` (camelCase query params) |
| `POST /tasks/{id}:cancel` | `CancelTask` |
| `POST /tasks/{id}:subscribe` | `SubscribeToTask` (SSE) |
| `POST /tasks/{id}/pushNotificationConfigs` | `CreateTaskPushNotificationConfig` |
| `GET /tasks/{id}/pushNotificationConfigs/{configId}` | `GetTaskPushNotificationConfig` |
| `GET /tasks/{id}/pushNotificationConfigs` | `ListTaskPushNotificationConfigs` |
| `DELETE /tasks/{id}/pushNotificationConfigs/{configId}` | `DeleteTaskPushNotificationConfig` |
| `GET /extendedAgentCard` | `GetExtendedAgentCard` |

REST requests use `Content-Type: application/a2a+json` and the same `A2A-Version: 1.0` header. Errors use `google.rpc.Status` (`error.code`, `error.status`, `error.message`, `error.details[]` with `@type`/`reason`/`domain`), mapping A2A error types to HTTP statuses per §5.4.

### 2.11 gRPC binding

Myaaw exposes the `lf.a2a.v1.A2AService` service (the official proto) with all 11 operations, over the shared `Service`. The gRPC binding is enabled by setting `A2A_GRPC_PORT` (or `A2A_GRPC_ADDR`); the Agent Card then advertises a `GRPC` entry in `supportedInterfaces`.

| Operation | gRPC method |
| :--- | :--- |
| `SendMessage` | unary `SendMessageRequest` → `SendMessageResponse` |
| `SendStreamingMessage` | server-streaming → `StreamResponse` |
| `GetTask` | unary → `Task` |
| `ListTasks` | unary → `ListTasksResponse` |
| `CancelTask` | unary → `Task` |
| `SubscribeToTask` | server-streaming → `StreamResponse` |
| `CreateTaskPushNotificationConfig` | unary → `TaskPushNotificationConfig` |
| `GetTaskPushNotificationConfig` | unary → `TaskPushNotificationConfig` |
| `ListTaskPushNotificationConfigs` | unary → `ListTaskPushNotificationConfigsResponse` |
| `DeleteTaskPushNotificationConfig` | unary → `google.protobuf.Empty` |
| `GetExtendedAgentCard` | unary → `AgentCard` |

Service parameters are carried in gRPC metadata: `a2a-version` (required, `1.0`), `authorization` (bearer token), and `a2a-extensions`. Errors map to gRPC `codes` with a `google.rpc.ErrorInfo` detail (`reason`, `domain: a2a-protocol.org`) per §5.4. TLS is enabled with `A2A_GRPC_TLS_CERT` / `A2A_GRPC_TLS_KEY`; without it the listener uses plaintext.

## 3. Implementation map

| Concern | Location |
| :--- | :--- |
| Types, states, JSON-RPC envelopes | `internal/services/a2a/types.go` |
| JCS canonicalization (RFC 8785) | `internal/services/a2a/jcs.go` |
| Task execution, files, streaming, push | `internal/services/a2a/service.go` |
| SQLite persistence, resume, stale recovery, `ListTasks` pagination | `internal/services/a2a/store.go` |
| HTTP handlers, version gate, scope checks, caching | `internal/services/a2a/handler.go` |
| HTTP+JSON (REST) binding | `internal/services/a2a/rest_router.go` |
| gRPC binding | `internal/services/a2a/grpc/` (imports `github.com/a2aproject/a2a-go/v2/a2apb/v1`) |
| Caller credentials and scopes | `internal/services/a2a/auth.go` |
| Card JWS signing and verification | `internal/services/a2a/cardsign.go` |
| Key loading and trusted-key parsing | `internal/services/a2a/signing.go` |
| Route mounting | `internal/services/a2a/router.go`, called from `internal/services/bot/router.go` |
| Caller-side tool | `internal/agent/tools/contact/contact.go` |
| `input-required` tool | `internal/agent/tools/a2ainput/a2ainput.go` |
| `auth-required` tool | `internal/agent/tools/a2aauth/a2aauth.go` |

## 4. Request flows

### 4.1 Synchronous task

`SendMessage` → `prepareTask` (validates parts, allocates/loads a task, stores it) → `runBot` (sets `MYAAW_A2A_TASK_DIR`, calls the agent loop) → `completeTask` / `inputRequired` / `authRequired` / `failTask` → response with `SendMessageResponse{task}`.

`prepareTask` first reserves `(caller, messageId)` in `a2a_message_ids`. If the message was already accepted, it returns that task without running the agent again, so a client retry (timeout, dropped connection) cannot duplicate side effects.

### 4.2 Streaming task

`SendStreamingMessage` → `prepareTask` → `startStream` → emit the task → run with a streaming callback that emits `artifactUpdate` deltas → on completion emit the final artifacts and a `statusUpdate` with `final: true`.

### 4.3 Async task with push notification

`SendMessage` with `returnImmediately` → `prepareTask` (stores `submitted`, registers the push config) → `go runTask` → on each state change `notifyTask` posts `StreamResponse` events to the webhook(s) with retry/backoff.

### 4.4 Agent-to-agent through the contact tool

`contact send` → load `contacts.json` → `SendMessage` on the remote URL with the bearer token and `A2A-Version: 1.0` → parse `SendMessageResponse{task}` → return artifact text (or an `[input-required taskId=…]` header). Each send carries one `messageId` that is reused across transient retries, so the remote deduplicates them; a timeout is recovered with a `returnImmediately` retry of the same message. File artifacts (`raw` and `url` parts) are downloaded into the caller's `home/workspace/downloads/<taskId>/` and reported with their paths and sizes.

### 4.5 Resuming an interrupted task

`SendMessage` with `message.taskId` → `prepareTask` loads the existing task, verifies the caller and interrupted state, appends the new user message, and re-runs.

## 5. Storage

| Table | Columns | Purpose |
| :--- | :--- | :--- |
| `a2a_tasks` | `id`, `context_id`, `status` (lowercase), `status_message`, `messages` (JSON), `artifacts` (JSON), `attempts`, `push_url`, `caller`, `created_at`, `updated_at` | Task persistence. |
| `a2a_callers` | `name`, `user_id` (negative), `created_at` | Per-caller synthetic local users. |
| `a2a_push_configs` | `task_id`, `id`, `url`, `token`, `created_at` | Webhook configs. |
| `a2a_message_ids` | `caller`, `message_id`, `task_id`, `created_at` | `SendMessage` idempotency; rows older than 30 days are pruned on startup. |

- `ListTasks` uses cursor pagination over `(updated_at DESC, id DESC)` backed by `idx_a2a_tasks_updated`.
- On startup, `ResumePending` re-runs `submitted`/`working` tasks (up to three attempts); exhausted tasks are failed. Stale tasks not resumable are failed by `RecoverStale`.
- Timestamps are UTC with millisecond precision on the wire.

## 6. Contact registry and the `contact` tool

Contacts live in `~/.myaaw/<agent>/contacts.json`:

```json
[
  {"name": "research", "url": "https://other-host/a2a", "token": "…", "description": "Research agent"}
]
```

The `contact` tool is a pure v1.0 client: it reads `supportedInterfaces` for discovery, always sends `A2A-Version: 1.0`, calls `SendMessage`, and parses `SendMessageResponse{task}`, `TASK_STATE_*`, `history`, and `artifactId`.

## 7. Security model

- Bearer tokens (constant-time comparison) with per-caller scopes.
- Signed Agent Cards verified against `A2A_TRUSTED_KEYS`.
- Webhook SSRF guard (loopback/private/link-local/shared-address ranges rejected unless `A2A_ALLOW_PRIVATE_WEBHOOKS=true`).
- Per-caller rate limiting (`A2A_RATE_LIMIT` / `A2A_RATE_BURST`).
- Audit logging of caller, method, and task state transitions (no credentials).
- Task ownership: read/cancel/resume are limited to the creating caller.

## 8. Compatibility and deliberate deviations

- **v1.0 only.** Legacy names (`tasks/*`, `message/send`, lowercase states, `kind` parts, `/.well-known/agent.json`, `async`, `X-A2A-Signature`, `tasks/resubscribe`) are removed, not aliased.
- **Internal storage stays lowercase**; the `TASK_STATE_*` enum is a wire-boundary concern.
- **Per-caller synthetic users** (`a2a:<caller>`, IDs from `-1001`) are an authorization/persistence internal, allowed by the spec's extension points.
- **`SubscribeToTask` buffer is in-memory** and expires after 10 minutes (documented; the spec allows implementation-defined replay).
- **Webhook DNS names are checked by name only**; resolution happens at delivery, a documented SSRF caveat.

## 9. Operations and troubleshooting

| Symptom | Check |
| :--- | :--- |
| `-32009` on every request | Missing `A2A-Version: 1.0` header. |
| `401 unauthorized` | Bearer token missing or wrong. |
| `403` on a write method | Caller lacks `delegate` scope. |
| `-32001` on `GetTask` | Task id missing, wrong, or owned by another caller. |
| `-32002` on `CancelTask` | Task already in a terminal state. |
| `-32004` on resume | `taskId` is not in an interrupted state. |
| `-32007` on `GetExtendedAgentCard` | `A2A_EXTENDED_CARD` points at a missing file. |
| Webhook rejected | URL points at a private/loopback address (see SSRF guard). |
| No card signature | `A2A_SIGNING_KEY` not set or key not loadable. |

## 10. Roadmap

- JSON-RPC binding: complete.
- HTTP+JSON (REST) binding: complete (`/message:send`, `/message:stream`, `/tasks…`, push configs, `/extendedAgentCard`).
- gRPC binding: complete (`lf.a2a.v1.A2AService`, 11 operations, via the official `a2a-go/v2` generated types).
- See [a2a-spec-compliance.md](a2a-spec-compliance.md) for the section-by-section implementation validation.
