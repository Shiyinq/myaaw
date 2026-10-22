# A2A Protocol v1.0 — Implementation Validation

This document validates Myaaw's implementation against the [Agent2Agent (A2A) Protocol Specification v1.0](https://a2a-protocol.org/latest/specification/). It replaces the earlier phased plan; the migration is complete. Each spec section is checked with a status and the evidence (code + tests) that proves it.

> **Scope:** Myaaw implements the three standard bindings — **JSON-RPC** (`POST /a2a`), **HTTP+JSON/REST**, and **gRPC** (`lf.a2a.v1.A2AService`) — over one shared service. The legacy surface (lowercase states, `tasks/*` methods, `kind`-discriminated parts, `X-A2A-Signature`, `/.well-known/agent.json`) was removed, not aliased.

**How to verify:** `go build ./... && go vet ./... && go test ./...` — all green. A2A-specific tests live in `internal/services/a2a/*_test.go`, `internal/services/a2a/grpc/*_test.go`, and `internal/agent/tools/contact/*_test.go`.

Legend: ✅ implemented · ⚠️ implemented with a documented deviation · ➖ not applicable (no normative requirement for a server — sections 1, 2, 12, and 14 are either introductory or administrative).

## Summary

| Spec section | Status |
| :--- | :--- |
| [1. Introduction](#1-introduction) | ➖ |
| [2. Terminology](#2-terminology) | ➖ |
| [3. A2A Protocol Operations](#3-a2a-protocol-operations) | ✅ |
| [4. Protocol Data Model](#4-protocol-data-model) | ✅ |
| [5. Protocol Binding Requirements and Interoperability](#5-protocol-binding-requirements-and-interoperability) | ✅ |
| [6. Common Workflows & Examples](#6-common-workflows--examples) | ✅ |
| [7. Authentication and Authorization](#7-authentication-and-authorization) | ✅ |
| [8. Agent Discovery: The Agent Card](#8-agent-discovery-the-agent-card) | ✅ |
| [9. JSON-RPC Protocol Binding](#9-json-rpc-protocol-binding) | ✅ |
| [10. gRPC Protocol Binding](#10-grpc-protocol-binding) | ✅ |
| [11. HTTP+JSON/REST Protocol Binding](#11-httpjsonrest-protocol-binding) | ✅ |
| [12. Custom Binding Guidelines](#12-custom-binding-guidelines) | ➖ |
| [13. Security Considerations](#13-security-considerations) | ⚠️ |
| [14. IANA Considerations](#14-iana-considerations) | ➖ |

---

## 1. Introduction

No normative server requirements. ➖

## 2. Terminology

No normative server requirements. ➖

## 3. A2A Protocol Operations

### 3.1 Core Operations (11 methods)

All 11 operations are implemented in all three bindings.

| Operation | JSON-RPC (`handler.go`) | REST (`rest_router.go`) | gRPC (`grpc/server.go`) |
| :--- | :--- | :--- | :--- |
| Send Message | ✅ | ✅ `POST /message:send` | ✅ |
| Send Streaming Message | ✅ | ✅ `POST /message:stream` | ✅ |
| Get Task | ✅ | ✅ `GET /tasks/{id}` | ✅ |
| List Tasks | ✅ | ✅ `GET /tasks` | ✅ |
| Cancel Task | ✅ | ✅ `POST /tasks/{id}:cancel` | ✅ |
| Subscribe to Task | ✅ | ✅ `POST /tasks/{id}:subscribe` | ✅ |
| Create Push Notification Config | ✅ | ✅ | ✅ |
| Get Push Notification Config | ✅ | ✅ | ✅ |
| List Push Notification Configs | ✅ | ✅ | ✅ |
| Delete Push Notification Config | ✅ | ✅ | ✅ |
| Get Extended Agent Card | ✅ | ✅ `GET /extendedAgentCard` | ✅ |

Evidence: `TestRPCSendAndGet`, `TestRPCStreaming`, `TestRPCListTasks`, `TestRPCCancelTask`, `TestRPCPushNotificationConfig`, `TestRestSendAndGetTask`, `TestRestPushConfigCRUD`, `TestGRPCSendAndGetTask`, `TestGRPCStreaming`.

### 3.2 Operation Parameter Objects

- `SendMessageRequest` (tenant, message, configuration, metadata) — `types.go`.
- `SendMessageConfiguration` (`acceptedOutputModes`, `taskPushNotificationConfig`, `historyLength`, `returnImmediately`) — `types.go`.
- `StreamResponse` oneof (`task` / `message` / `statusUpdate` / `artifactUpdate`) — `types.go`.

### 3.2.4 History Length Semantics

`trimHistory` enforces: omitted = keep all, `0` = omit history, `n` = keep last `n`. Evidence: `TestRPCListTasks` (historyLength 0 omits history).

### 3.2.6 Service Parameters

`A2A-Version` is validated strictly (only `1.0`; anything else → `VersionNotSupportedError`, `-32009`). Carried as an HTTP header/query param (JSON-RPC + REST) and gRPC metadata `a2a-version`. `A2A-Extensions` is accepted but no protocol extensions are implemented (see [deviations](#known-deviations)). Evidence: `TestRPCVersionGate`, `TestRestVersionRequired`, `TestGRPCVersionRequired`.

### 3.3 Operation Semantics

- **3.3.1 Idempotency:** `DeleteTaskPushNotificationConfig` is idempotent (returns success for a missing config).
- **3.3.2 Error Handling:** §5.4 codes plus a `google.rpc.ErrorInfo` detail (`reason`, `domain: a2a-protocol.org`) — `handler.go`/`grpc/errors.go`. Evidence: `TestRPCFailedTaskAndMissingTask`, `TestRestErrorShape`, `TestGRPCErrorMapping`.
- **3.3.3 Asynchronous Processing:** `configuration.returnImmediately` returns a `submitted` task and runs in the background. Evidence: `TestSendMessageAsyncPushNotification`.
- **3.3.4 Capability Validation:** `GetExtendedAgentCard` returns `UnsupportedOperationError` when the capability is absent. Evidence: `TestRPCExtendedCardNotSupported`.

### 3.4 Multi-Turn Interactions

`message.taskId` resumes an interrupted task; `contextId` is inferred from the task and a mismatched `contextId`+`taskId` is rejected (`ErrContextMismatch`). `input-required` (tool `a2a_input`) and `auth-required` (tool `a2a_auth_request`) states are supported. Evidence: `TestRPCInboundMessageTaskIDResume`, `TestSendMessageInputRequiredAndResume`, `TestContactToolInputRequiredResume`.

### 3.5 Task Update Delivery Mechanisms

Streaming (`SendStreamingMessage`) and push notifications. Webhook payload is a `StreamResponse` (`statusUpdate` on each change + a final `task` snapshot for terminal states), `Content-Type: application/a2a+json`, with retry/backoff (at-least-once). Evidence: `TestRPCStreaming`, `TestRestStreaming`, `TestGRPCStreaming`, `TestNotifyTaskSendsConfiguredWebhooks`.

## 4. Protocol Data Model

- **4.1 Core Objects:** `Task`, `TaskStatus`, `TaskState` (`TASK_STATE_*`), `Message`, `Role` (`ROLE_*`), `Part` (member-discriminated `text`/`raw`/`url`/`data` + `filename`/`mediaType`), `Artifact` (`artifactId` required) — `types.go`. Evidence: `TestRPCResponseShape` (asserts `history` not `messages`, `ROLE_*` roles, `messageId`).
- **4.2 Streaming Events:** `TaskStatusUpdateEvent`, `TaskArtifactUpdateEvent` with `contextId` — `types.go`.
- **4.3 Push Notification Objects:** `TaskPushNotificationConfig`, `AuthenticationInfo`, `StreamResponse` payload — `types.go`.
- **4.4 Agent Discovery Objects:** `AgentCard`, `AgentProvider`, `AgentCapabilities`, `AgentExtension`, `AgentSkill` (with `tags`), `AgentInterface`, `AgentCardSignature` — `types.go`.
- **4.5 Security Objects:** `SecurityScheme` oneof with `APIKeySecurityScheme`, `HTTPAuthSecurityScheme`, `OAuth2SecurityScheme`, `OpenIDConnectSecurityScheme`, `MutualTLSSecurityScheme` — `types.go`. The server emits the bearer `httpAuthSecurityScheme` only; the other kinds are defined for card (re)construction.

State conversion happens at the marshaling boundary (`WireState`/`StateFromWire`); storage stays lowercase (`store.go`).

## 5. Protocol Binding Requirements and Interoperability

- **5.1 Functional equivalence:** one shared `a2a.Service` serves all three bindings, so every operation is functionally identical. `SetupService`/`MountRouter`/`grpc.Serve` share the instance (`router.go`, `bot/router.go`).
- **5.2 Protocol selection:** the card declares `JSONRPC`, `HTTP+JSON`, and `GRPC` in `supportedInterfaces` (`service.go`). Evidence: `TestCard`.
- **5.3 Method mapping:** the operation → method/endpoint table above matches §5.3.
- **5.4 Error code mappings:** JSON-RPC `-32001…-32009`, gRPC `codes` (`NOT_FOUND`, `FAILED_PRECONDITION`, `INVALID_ARGUMENT`, `INTERNAL`), HTTP statuses (`404`/`400`/`500`) — `handler.go`, `grpc/errors.go`, `rest_router.go`.
- **5.5 JSON field naming:** camelCase fields and `SCREAMING_SNAKE_CASE` enum values (verified by golden tests).
- **5.6.1 Timestamps:** UTC, millisecond precision (`nowTimestamp` emits `…sssZ`; `store.go` reads RFC3339Nano).
- **5.7 Field presence:** optional fields use `omitempty`; JCS canonicalization excludes unset optionals and the `signatures` field. Evidence: `TestJCSCanonicalizationRFC8785Vector`, `TestCanonicalCardDropsSignaturesAndWhitespace`.
- **5.8 Custom binding URI:** ➖ (Myaaw implements only the standard bindings).

## 6. Common Workflows & Examples

Covered end-to-end by the test suite: basic task (`TestRPCSendAndGet`, `TestRestSendAndGetTask`, `TestGRPCSendAndGetTask`), streaming, multi-turn, version negotiation error, list/pagination (`TestTaskStoreListTasks`), push notifications, file exchange (`TestSendMessageAcceptsFileParts`, `TestSendMessageCollectsOutputFiles`), structured data (`data` parts), and extended agent card (`TestRPCExtendedCard`).

## 7. Authentication and Authorization

- Bearer tokens with constant-time comparison (`auth.go`, `subtle.ConstantTimeCompare`).
- Credential sources: `A2A_TOKEN`, `A2A_TOKENS`, `A2A_CALLERS` (`name:token[:read|delegate]`).
- Scopes: `read` (read methods) and `delegate` (writes + read). Evidence: `TestRPCReadOnlyScope`, `TestAuthenticateScopes`.
- Signed Agent Cards verified against `A2A_TRUSTED_KEYS` (`cardsign.go`, `contact get`).

## 8. Agent Discovery: The Agent Card

- `name`, `description`, `version` (`1.0`), `supportedInterfaces` (ordered, first preferred), `capabilities` (`streaming`, `pushNotifications`, `extendedAgentCard`), `defaultInputModes`/`defaultOutputModes`, `skills` (with required `tags`), `securitySchemes` (map) + `securityRequirements`, `provider`/`documentationUrl`/`iconUrl` (config-driven) — `service.go`.
- Caching: `ETag` + `If-None-Match` → `304`, `Cache-Control: no-cache`. Evidence: `TestCardCaching`.
- Signed cards: RFC 8785 (JCS) canonicalization, protected header `{"alg":"EdDSA","typ":"JOSE","kid":…}`. Evidence: `TestSignAndVerifyCard`, `TestCardEndpointSignsCard`.

## 9. JSON-RPC Protocol Binding

`POST /a2a`, JSON-RPC 2.0 envelopes, PascalCase method names, `A2A-Version` header gate, `SendMessageResponse` oneof, `StreamResponse` `data:` lines, §5.4 error codes with `error.data` `ErrorInfo`. Evidence: `handler.go` + `handler_test.go`.

## 10. gRPC Protocol Binding

`lf.a2a.v1.A2AService` with all 11 operations, implemented over the shared service. Uses the **official generated types** from `github.com/a2aproject/a2a-go/v2` (`a2apb/v1`), so the wire contract is the normative `specification/a2a.proto`. Service parameters travel as gRPC metadata (`a2a-version`, `authorization`, `a2a-extensions`); errors map to `codes` with a `google.rpc.ErrorInfo` detail. Evidence: `grpc/server.go` + `grpc/server_test.go` (`TestGRPCSendAndGetTask`, `TestGRPCErrorMapping`, `TestGRPCVersionRequired`, `TestGRPCStreaming`, `TestGRPCSubscribeTerminalFails`).

## 11. HTTP+JSON/REST Protocol Binding

§11.3 endpoints (`/message:send`, `/message:stream`, `/tasks…`, push config CRUD, `/extendedAgentCard`), `application/a2a+json`, camelCase query parameters, `google.rpc.Status` errors with `error.details[].@type`. Evidence: `rest_router.go` + `rest_test.go`.

## 12. Custom Binding Guidelines

➖ Myaaw does not define a custom protocol binding.

## 13. Security Considerations

- **13.1 Data access / authorization scoping:** task ownership (`OwnsTask`), per-caller scopes, per-caller synthetic users (`a2a:<caller>`, IDs from `-1001`) so callers do not share user `0`. Evidence: `TestRPCTaskOwnership`, `TestRPCPushConfigOwnership`, `TestSendMessageUsesPerCallerUserIDs`.
- **13.2 Push notification security:** webhook SSRF guard (loopback/private/link-local/CGNAT rejected unless `A2A_ALLOW_PRIVATE_WEBHOOKS=true`), bearer auth on delivery, retry/backoff. Evidence: `TestWebhookURLSSRFGuard`.
- **13.3 Extended Agent Card access control:** read-scoped, only when configured. Evidence: `TestRPCExtendedCardNotSupported`, `TestRPCExtendedCardNotConfigured`.
- **13.4 General security:** rate limiting per caller (`A2A_RATE_LIMIT`/`A2A_RATE_BURST`, HTTP/gRPC 429/`ResourceExhausted`), audit logging, constant-time token comparison, optional gRPC TLS. Evidence: `TestRPCRateLimit`.

⚠️ See [deviations](#known-deviations) for the TLS and webhook-resolution notes.

## 14. IANA Considerations

➖ This section contains the **registration templates** that the A2A Protocol Working Group submits to IANA for the media type `application/a2a+json`, the HTTP headers `A2A-Version`/`A2A-Extensions`, and the well-known URI `/.well-known/agent-card.json`. Registration itself is a one-time administrative action by the protocol authors, **not** a per-server obligation, so there is no code requirement here.

The templates do embed a few *behavioral* statements, and Myaaw satisfies them: UTF-8 JSON, `A2A-Version` validation with `VersionNotSupportedError`, the card at `/.well-known/agent-card.json` returning an `AgentCard`, and enforced authentication/authorization (§7). The SSRF consideration is met because the only server-side outbound fetch is webhooks (guarded in §13.2); `url` parts in messages are not fetched by the server, only passed to the agent as references.

---

## Known deviations

These are deliberate, documented trade-offs; none break protocol interoperability.

| Area | Deviation | Rationale |
| :--- | :--- | :--- |
| `A2A-Extensions` | Accepted but not enforced; no protocol extensions implemented. `ExtensionSupportRequiredError` (`-32008`) exists but is unused. | Myaaw exposes no extension URIs. |
| `SubscribeToTask` replay | Buffer is in-memory, expires after 10 minutes, and handlers forward only live events (the buffered snapshot is not replayed). | Documented §2.8 behavior; keeps no durable stream store. |
| Push notification delivery | In-process retry with backoff (at-least-once); not a durable queue, so deliveries are not replayed across restart. | Spec permits at-least-once; durability is a deployment concern. |
| gRPC TLS | Plaintext unless `A2A_GRPC_TLS_CERT`/`A2A_GRPC_TLS_KEY` are set. | TLS termination may also be handled by a proxy. |
| Webhook DNS resolution | Hostnames are validated by name only; IP resolution happens at delivery, not config time. | Avoids a DNS dependency at config time; SSRF guard still blocks literal private/loopback ranges. |
| Card signing numbers | JCS number serialization is exercised on the RFC 8785 vector; Agent Cards themselves carry no numbers. | Documented in §12 of the original plan. |
