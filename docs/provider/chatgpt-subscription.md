# ChatGPT Subscription

ChatGPT Subscription is Myaaw's independent OAuth provider for eligible ChatGPT plan usage through the public OpenAI Responses API. Its provider type is `openai_chatgpt`, its authentication mode is `oauth`, and its display label is exactly **ChatGPT Subscription**.

This reference describes the current integration: browser authorization, host credentials, provider configuration, control-plane contracts, model discovery, inference, and operational limits. For the shorter command-oriented walkthrough, see [ChatGPT Subscription CLI setup](../cli/chatgpt-subscription.md).

## Contents

1. [Provider and billing boundary](#1-provider-and-billing-boundary)
2. [Setup: dashboard, CLI, and headless use](#2-setup-dashboard-cli-and-headless-use)
3. [Configuration contains references, not tokens](#3-configuration-contains-references-not-tokens)
4. [OAuth lifecycle and identity binding](#4-oauth-lifecycle-and-identity-binding)
5. [Host credential layout and protection](#5-host-credential-layout-and-protection)
6. [Connection metadata versus sign-in attempt status](#6-connection-metadata-versus-sign-in-attempt-status)
7. [Control-plane integration contracts](#7-control-plane-integration-contracts)
8. [Model discovery and picker behavior](#8-model-discovery-and-picker-behavior)
9. [Inference, tools, and continuation](#9-inference-tools-and-continuation)
10. [Preview restrictions and implemented subset](#10-preview-restrictions-and-implemented-subset)
11. [Limits, streaming success, and failures](#11-limits-streaming-success-and-failures)
12. [Sharing, disconnect, deletion, and revocation](#12-sharing-disconnect-deletion-and-revocation)
13. [Backups, migration, and privacy](#13-backups-migration-and-privacy)
14. [Source map and related references](#14-source-map-and-related-references)

## 1. Provider and billing boundary

| Property | ChatGPT Subscription |
| :--- | :--- |
| Config type | `openai_chatgpt` |
| Authentication | Browser OAuth with S256 PKCE; a local `credential_id` selects the saved registration. |
| Model discovery | `GET https://api.openai.com/v1/models` with that registration's access token. |
| Inference | `POST https://api.openai.com/v1/responses`, HTTP/SSE. |
| Billing/eligibility | The selected account/workspace's authorized ChatGPT plan permissions and limits. |
| Endpoint configuration | Fixed official origin; no arbitrary `base_url`. |
| Fallback | No automatic API-key, account, model, or paid-API fallback. |

This is **not** the `openai` API-key provider with an OAuth token pasted into `api_key`. OpenAI/OpenAI-compatible API-key providers and Gemini remain separate integrations. A ChatGPT sign-in does not supply credentials to those providers, and their keys do not repair a failed subscription request. To change provider or billing path, select another configured provider explicitly.

Myaaw neither imports other applications' authentication caches nor calls private ChatGPT `backend-api` endpoints. The grant does not import ChatGPT conversations, memories, or other account context into Myaaw.

Subscription configuration rejects a nonempty API key and custom endpoints. The constructor accepts an omitted endpoint or the exact official `https://api.openai.com` origin after whitespace/trailing-slash normalization; provider normalization clears that redundant official URL. Leave `base_url` absent. In particular, do not supply an OpenAI-compatible gateway or a URL ending in `/v1`: Myaaw appends the API paths itself. OAuth and inference clients do not follow HTTP redirects, preventing credential forwarding to redirect destinations. Endpoint overrides are package-private test facilities, not user configuration.

Sources: [provider implementation](../../internal/provider/chatgpt.go), [provider factory](../../internal/provider/provider.go), [auth normalization](../../internal/onboard/chatgpt.go).

## 2. Setup: dashboard, CLI, and headless use

### Same-machine browser requirement

Initial authorization requires a browser and Myaaw running under the same local OS account on the **same machine**. The auth manager binds an ephemeral IPv4 loopback listener at:

```text
http://127.0.0.1:<ephemeral-port>/auth/callback
```

This is a temporary OAuth listener, not the control plane's port or a public webhook. Keep the initiating process running until sign-in completes; allow the browser to reach that loopback address.

For dashboard sign-in, open the dashboard through `localhost` or a loopback IP on the API host. Connect, poll, and cancel handlers require:

- A loopback socket peer and a loopback raw HTTP Host (`localhost` or an actual loopback IP, not a hostname resolved through DNS).
- Loopback HTTP(S) Origin and Referer when supplied. The frontend proxy rewrites Host but preserves these browser headers, so it cannot make a remote dashboard local.
- A local Referer for browser polling requests that omit Origin. Requests marked with browser Fetch Metadata but lacking both Origin and Referer are rejected. Native same-machine clients without browser metadata may omit both.

Forwarded headers do not establish locality. Remote requests to these login routes receive `403`. Do not disable these checks or strip browser headers to work around them.

A remote browser follows the OAuth redirect to **its own** loopback interface, not the API host's. An ordinary SSH forward for the dashboard/control-plane port does not also forward the dynamically allocated OAuth callback. Use a desktop session on the host for sign-in, then reuse its saved credential for headless processes on that host. There is no implemented device-code flow, remote callback relay, manual code-paste flow, or credential import/export command.

### Dashboard

1. Select an agent on the Providers page, or enter a valid agent name in the new-agent wizard.
2. Choose **ChatGPT Subscription**. The form hides API-key and base-URL inputs.
3. Click **Continue with ChatGPT** and complete authentication and plan consent in the local browser popup. If blocked, use the explicit open-browser button.
4. Wait for the connection result, discover available models, and choose a model slug.
5. Save the provider or submit the onboarding form. Signing in alone does not save the provider reference.

The connection component validates that the sign-in URL has the `https://auth.openai.com` origin, polls approximately every 1.5 seconds while pending, and cancels pending work on navigation/component cleanup. It confirms reconnect and disconnect actions. Its save/discovery gates accept a configured connection in `connected` or `refresh_required` state; discovery can refresh an expired token.

Browser authorization persists the host credential immediately on success, independently of saving the form. Cancelling the onboarding review or leaving the form afterward does not delete that credential. Starting sign-in for a new agent name does not itself create agent files.

### CLI browser setup and reconnect

```sh
# Browser sign-in, live discovery, and interactive model picker:
myaaw --agent myfriend provider connect-chatgpt personal --default

# Browser sign-in with an explicitly selected model slug; no picker:
myaaw --agent myfriend provider connect-chatgpt personal \
  --model MODEL_SLUG_FROM_DISCOVERY --default

# Reauthorize the registration already attached to this provider:
myaaw --agent myfriend provider connect-chatgpt personal \
  --model MODEL_SLUG_FROM_DISCOVERY

# Inspect local metadata or detach this provider:
myaaw --agent myfriend provider status-chatgpt personal
myaaw --agent myfriend provider disconnect-chatgpt personal
```

The optional provider name defaults to `chatgpt`. An existing provider with a different type cannot be overwritten by `connect-chatgpt`. Without a credential flag, the command always performs browser authorization, reusing the provider's saved binding if present. `--default` selects the saved provider as the agent's default; it also becomes default if none is configured.

The CLI invokes the system browser directly with argv, not a shell, and does not print the authorization URL. It waits at most ten minutes; `Ctrl+C` cancels. `myaaw provider create` and interactive `myaaw onboard` also offer **ChatGPT Subscription**, skip key/endpoint prompts, and discover models after authorization. Interactive onboarding does not substitute a built-in subscription model when discovery fails.

Supplying `--model` bypasses the connect command's discovery/picker; it is not proof that the account currently permits that model. Without `--model`, the command attempts live discovery and requires an interactive terminal for selection.

### Headless attachment

After successful browser sign-in, use the printed local credential ID on the same host and OS account:

```sh
# Attach an existing verified record, without opening a browser:
myaaw --agent myfriend provider connect-chatgpt personal \
  --credential-id LOCAL_CREDENTIAL_UUID \
  --model MODEL_SLUG_FROM_DISCOVERY --default

# Create an agent without browser interaction:
myaaw --agent worker onboard --yes \
  --provider openai_chatgpt --provider-name personal \
  --credential-id LOCAL_CREDENTIAL_UUID \
  --model MODEL_SLUG_FROM_DISCOVERY --channel skip
```

`--credential-id` is an attachment flag, not a token and not a sign-in request. Use an actual verified local UUID returned by Myaaw. A pending-only registration ID cannot be attached this way. Headless onboarding requires an explicit model, or the existing model for that same subscription provider; it can also reuse that provider's saved credential reference. Use the normal [onboarding flags](../cli/onboard.md) such as `--force` when reconfiguring an existing agent.

OAuth onboarding ignores unrelated `MYAAW_API_KEY` values. Explicit nonempty `--api-key` and custom `--base-url` values are rejected rather than changing the authentication path.

### Failed-registration recovery

A failed initial attempt may already have received an issued OAuth client ID. If that binding was retained, resume it rather than generating another registration:

```sh
myaaw --agent myfriend provider connect-chatgpt personal \
  --retry-credential-id RECOVERY_CREDENTIAL_UUID \
  --model MODEL_SLUG_FROM_DISCOVERY
```

`--retry-credential-id` calls browser authorization with the retained binding; it is mutually exclusive with `--credential-id`. It accepts saved accounts or pending registrations, including after a restart. Keep the recovery ID printed by the CLI or exposed in attempt status. An ID is assigned before a registration is persisted, so an early failure before a valid issued-client callback may leave nothing recoverable under that ID.

Sources: [CLI commands](../../cmd/myaaw/provider_chatgpt.go), [onboarding browser adapter](../../cmd/myaaw/onboard_chatgpt.go), [CLI onboarding](../../cmd/myaaw/onboard.go), [connection UI](../../webui/src/lib/components/common/ChatGPTConnection.svelte).

## 3. Configuration contains references, not tokens

The selected agent's `~/.myaaw/agents/<agent>/config/config.json` contains a provider reference and model selection:

```json
{
  "default_provider": "personal",
  "providers": {
    "personal": {
      "type": "openai_chatgpt",
      "auth_mode": "oauth",
      "credential_id": "<LOCAL_CREDENTIAL_UUID>",
      "default_model": "<MODEL_SLUG_FROM_DISCOVERY>"
    }
  }
}
```

Angle-bracket values throughout this reference are redacted placeholders, not usable credential IDs or guaranteed model names. Never put access, refresh, or identity tokens, authorization codes, client secrets, or PKCE values in agent config, `.env`, provider API payloads, or source control.

Provider writes normalize OAuth fields and check that the ID resolves to a verified local account record. **Existence validation is not an online eligibility check**: a saved record can still require reauthorization, refresh, or renewed plan permissions. Discovery and inference enforce usable tokens and the required scopes. The low-level provider constructor is lazy and does not inspect the credential store until a request.

On provider update, changing an API-key type to `openai_chatgpt` clears the old key and endpoint and requires a credential. Changing to a non-OAuth type clears OAuth fields. Existing API-key update semantics remain independent.

## 4. OAuth lifecycle and identity binding

The auth package owns authorization and protected storage; the provider owns discovery and inference.

1. `DefaultManager()` opens the host-global store and persists/reuses its opaque `ext_agent_host_id` (`urn:uuid:<UUID>`). Agent selection does not create a new host ID.
2. `Manager.Start(ctx, "")` starts a listener before returning and creates fresh random state, nonce, and S256 PKCE values. It uses `client_id=dynamic_agent_client`, `agent_name_hint=Myaaw`, the stable host ID, and resource `https://api.openai.com/v1`.
3. OpenAI performs browser authentication/consent. For a new registration, the callback must include a valid issued `oaiapp_...` client ID. Myaaw exchanges the code using **that issued ID**, not the bootstrap client ID, together with the verifier and exact redirect URI.
4. The callback validates method, path, Host, and state. Untrusted-state requests do not consume the real attempt; a state-validated callback is redeemed at most once.
5. ID-token signature/JWKS, issuer, audience, expiry, issued-at, subject, nonce, not-before, authorized-party rules, and `at_hash` when present are validated. Required permissions are checked against the actual granted scopes before inference is enabled.
6. Verified credentials are committed to the host store. The caller separately saves a provider reference and selected model.

The requested scope set is:

```text
openid profile email offline_access resource.invoke chatgpt.tokens.use.direct
```

The required inference grant is `openid`, `offline_access`, `resource.invoke`, and `chatgpt.tokens.use.direct`. Identity-only authorization is insufficient. A verified account without the complete grant is retained, but the attempt reports failure and `AccessToken` returns no token for inference. Reauthorize that binding to request the full grant again.

`Manager.Start(ctx, credentialID)` reuses the saved client/subject binding or retained pending registration. Returning authorization may include `id_token_hint` and `login_hint`; verified reauthorization must retain the subject, and a callback cannot replace its client ID. Account identity is **not keyed by email**. Two registrations with the same email or subject remain distinct local UUIDs; to choose another account, create/select a different registration rather than treating reconnect as account switching. Stale commits changed by another refresh/sign-in, or deleted accounts, are rejected.

For integrations calling the package directly, the `Start` context owns the attempt lifetime. Do not use a request context that ends as soon as an HTTP response is returned. `Attempt.Wait(ctx)` cancellation stops only that waiter; `Attempt.Cancel()` stops the attempt. The control plane deliberately uses an attempt-owned lifetime.

### Refresh and rotation

Every model/inference request obtains its token through `Manager.AccessToken(ctx, id)`, which reloads the selected record under a process-shared kernel lock. It normally refreshes within one minute of expiry, respects `earliest_refresh_at`, and never returns an expired token. If refresh is not yet permitted, a still-valid access token may be used; an already expired one produces an error.

Refresh rotation updates the entire record atomically under the lock, coordinating independent CLI/server/control-plane processes using the same store. Once a valid rotation response is received, bounded identity validation and persistence protect the replacement against caller cancellation. A reduced-scope response still saves the replacement refresh token and actual grant before returning a permission error, avoiding reuse of the consumed token.

Confirmed terminal refresh failures such as invalidated, expired, or reused refresh tokens clear the unusable token set while retaining the client/subject/email mapping for reauthorization. Both OAuth string errors and structured `error.code` responses are recognized. Temporary network, rate-limit, and infrastructure errors preserve records; `invalid_client` does not automatically erase tokens. An inference rejection is not itself the auth manager's terminal-refresh cleanup operation.

Sources: [auth contract](../../internal/auth/chatgpt/README.md), [manager](../../internal/auth/chatgpt/manager.go), [attempt/callback](../../internal/auth/chatgpt/attempt.go), [tokens and validation](../../internal/auth/chatgpt/token.go).

## 5. Host credential layout and protection

Credentials belong to the local OS account's Myaaw installation, outside every agent root:

```text
~/.myaaw/credentials/chatgpt/
├── host.json
├── accounts/
│   └── <credential-uuid>.json
├── registrations/
│   └── <recovery-credential-uuid>.json
└── .lock
```

| Record | Contents and lifetime |
| :--- | :--- |
| `host.json` | Stable opaque host identifier; survives process restarts and account deletion. It is an identifier, not an authentication credential. |
| `accounts/<id>.json` | Verified client/subject/email binding, access/refresh/identity tokens, expiry, scopes, and rotation/validation bookkeeping. These JSON files contain secrets; they are not an encrypted keychain. |
| `registrations/<id>.json` | Pending recovery binding: local ID, callback-issued client ID, issuer, and host ID only. No unverified identity, tokens, code, state, nonce, or PKCE verifier. |
| `.lock` | Cross-process serialization for store transactions and rotating refreshes. |

The pending registration is retained **before** code exchange, including when exchange subsequently fails with `invalid_grant`. It is removed after verified credentials are committed. Pending-only IDs return `ErrNotFound` from `Get`/`AccessToken`; they are not connected accounts. Starting with an empty ID always creates an independent registration instead of silently choosing a pending one.

On Unix, directories are `0700`, files are `0600`, writes use atomic replacement, and files plus containing directories are synced. Pinned `os.Root` directory handles prevent path swaps from redirecting operations outside the opened store. Symlinks, special files, hard-linked credential/lock files, and foreign ownership on Unix are rejected before permission repair or reading. These protections do not protect against a compromised owning OS account. On Windows, configure appropriate owner-only ACLs: Go's `0600` mode does not establish Windows ACL isolation.

Sources: [store implementation](../../internal/auth/chatgpt/store.go), [Unix locking/protection](../../internal/auth/chatgpt/store_unix.go), [Windows locking](../../internal/auth/chatgpt/store_windows.go).

## 6. Connection metadata versus sign-in attempt status

These are different state machines. A successful sign-in attempt does not mean the provider reference has been saved, and a saved provider's status is not the status of an active browser attempt.

### Saved provider connection

`GET /agents/:name/providers`, configuration views, and `provider status-chatgpt` expose metadata only. Subscription views include `auth_mode`, `credential_id`, and a `connection` object:

```json
{
  "name": "personal",
  "type": "openai_chatgpt",
  "default_model": "<MODEL_SLUG_FROM_DISCOVERY>",
  "api_key": { "configured": false },
  "auth_mode": "oauth",
  "credential_id": "<LOCAL_CREDENTIAL_UUID>",
  "connection": {
    "configured": true,
    "status": "connected",
    "account_label": "<REDACTED_EMAIL>",
    "expires_at": "2099-01-01T00:00:00Z"
  }
}
```

| Connection status | Local meaning |
| :--- | :--- |
| `disconnected` | No credential reference on the provider. |
| `unavailable` | A reference exists, but the local verified record/store cannot be read, including a pending-only or missing ID. |
| `connected` | The record has access/refresh tokens, required scopes, and a future access-token expiry. |
| `refresh_required` | Tokens/scopes exist, but cached access-token expiry has passed; a request must refresh. |
| `reauthorization_required` | Access/refresh tokens or required scopes are missing; sign in again. |

`connection.configured` means a verified record was readable, not that its grant is currently usable. It remains true for `refresh_required` and `reauthorization_required`. The account label is the saved email; expiry is cached access-token expiry (or `null` if unavailable).

**Status never refreshes tokens or contacts OpenAI to check revocation.** OpenAI does not send disconnection notifications to this integration. A remotely revoked grant can still appear connected until a request or refresh detects the problem.

### Browser attempt

Attempt polling returns `status`, `credential_id`, `email`, and sanitized `error`, never the credential object:

| Attempt status | Meaning |
| :--- | :--- |
| `pending` | Waiting for authorization/callback processing. The credential ID is already allocated; no verified account is implied. |
| `connected` | Authorization, identity/permission validation, and credential persistence succeeded. |
| `failed` | Authorization/validation/storage failed, or the pending attempt was cancelled. There is no separate `cancelled` wire status. |
| `expired` | The attempt deadline elapsed. |

Cancellation does not undo an already completed sign-in, delete a retained recovery binding, or detach a provider. The control plane retains status until two minutes after the attempt's expiry timestamp; restarting it loses in-memory sessions but not saved accounts or pending registration bindings.

Source: [connection views and session service](../../internal/services/controlplane/service/chatgpt.go).

## 7. Control-plane integration contracts

These paths are relative to the control-plane origin, for example `http://localhost:9000`. All require the normal Myaaw control-plane bearer token. That token is **not** the OpenAI OAuth access token. Send JSON with `Content-Type: application/json` where a body is shown.

| Method | Path | Contract |
| :--- | :--- | :--- |
| `POST` | `/agents/:name/providers/chatgpt/connect` | Start browser authorization; body `{}` or `{ "credential_id": "<SAVED_OR_PENDING_UUID>" }`. Returns `200` with session ID, sensitive sign-in URL, and expiry. |
| `GET` | `/agents/:name/providers/chatgpt/connect/:session` | Poll metadata-only attempt status. |
| `DELETE` | `/agents/:name/providers/chatgpt/connect/:session` | Cancel the pending attempt; returns `{ "ok": true }`. |
| `POST` | `/agents/:name/providers/:id/disconnect` | Detach the selected subscription provider; returns `{ "ok": true }`. |
| `POST` | `/providers/models` | Discover using an unsaved provider's credential reference. |
| `GET` | `/agents/:name/providers/:id/models` | Discover using the saved provider. |
| `POST` | `/agents/:name/providers` | Create a provider on an existing agent; returns `201` with `ProviderView`. |
| `PUT` | `/agents/:name/providers/:id` | Update provider fields; returns `200` with `ProviderView`. |
| `POST` | `/agents/:name/providers/default` | Select the default with `{ "name": "personal" }`. |

The extra same-machine restrictions apply to **connect/poll/cancel**, not every provider management route. Disconnect and ordinary provider/model operations still require bearer authentication but do not use the login-locality gate.

### Start, poll, and recover

Initial connect request:

```http
POST /agents/myfriend/providers/chatgpt/connect
Authorization: Bearer <REDACTED_CONTROL_PLANE_TOKEN>
Content-Type: application/json

{}
```

Illustrative response (the real authorization URL must not be logged):

```json
{
  "session_id": "<ATTEMPT_UUID>",
  "authorization_url": "https://auth.openai.com/api/accounts/authorize?<REDACTED_QUERY>",
  "expires_at": "2099-01-01T00:10:00Z"
}
```

Poll `GET /agents/myfriend/providers/chatgpt/connect/<ATTEMPT_UUID>` until terminal. Successful status has this shape:

```json
{
  "status": "connected",
  "credential_id": "<LOCAL_CREDENTIAL_UUID>",
  "email": "<REDACTED_EMAIL>",
  "error": ""
}
```

For reconnect or recoverable failed registration, start another attempt with:

```json
{
  "credential_id": "<SAVED_OR_PENDING_UUID>"
}
```

An empty ID starts a new registration. A nonempty ID reauthorizes that exact binding. Keep the returned recovery credential ID on failure, but do not save it as a provider reference until authorization succeeds.

Sessions are scoped to a validated agent name, including a not-yet-created name for onboarding. Unknown, expired-retention, and cross-agent session lookups return `404`. There is at most one pending attempt per agent and at most 32 retained sessions per control-plane instance; exceeding either returns `429`. Attempts live at most ten minutes. Invalid JSON/config produces `400`; unauthenticated requests produce `401`; login locality violations produce `403`. Login/disconnect handlers set `Cache-Control: no-store`, `Pragma: no-cache`, and `Referrer-Policy: no-referrer`.

### Discover and save

Before saving, send this to `POST /providers/models`:

```json
{
  "type": "openai_chatgpt",
  "auth_mode": "oauth",
  "credential_id": "<LOCAL_CREDENTIAL_UUID>"
}
```

The backend accepts `auth_mode`; the current frontend model-lookup DTO sends `type` and `credential_id` and relies on normalization to supply OAuth mode. Successful discovery returns slugs:

```json
{
  "models": ["<MODEL_SLUG_FROM_DISCOVERY>"],
  "source": "live"
}
```

Then create the provider with `POST /agents/myfriend/providers`:

```json
{
  "name": "personal",
  "type": "openai_chatgpt",
  "auth_mode": "oauth",
  "credential_id": "<LOCAL_CREDENTIAL_UUID>",
  "default_model": "<MODEL_SLUG_FROM_DISCOVERY>"
}
```

Create/update returns the metadata-only provider view, **not just an OK object**, even though the frontend API helper currently types those results as `OKResponse`. Polling success alone does not create a provider or make it default. A first provider becomes default when none exists; otherwise use the default-provider route explicitly.

For a new agent, use `POST /agents/:name/onboard` after sign-in, with the same auth fields inside `provider` but **`model` rather than `default_model`**:

```json
{
  "provider": {
    "name": "personal",
    "type": "openai_chatgpt",
    "auth_mode": "oauth",
    "credential_id": "<LOCAL_CREDENTIAL_UUID>",
    "model": "<MODEL_SLUG_FROM_DISCOVERY>"
  }
}
```

Onboarding requires an explicit or saved subscription model and validates before initialization. See the [Control Plane API](../api/control-plane.md#chatgpt-subscription) for the surrounding auth, onboarding, and config contracts.

Sources: [routes](../../internal/services/controlplane/router.go), [login handlers](../../internal/services/controlplane/handler/chatgpt.go), [provider write handlers](../../internal/services/controlplane/handler/config_write.go), [model service](../../internal/services/controlplane/service/models.go), [onboarding service](../../internal/services/controlplane/service/onboard.go), [frontend API helper](../../webui/src/lib/apis/providers.ts), [frontend DTOs](../../webui/src/lib/types/providers.ts).

## 8. Model discovery and picker behavior

The subscription catalog is account-specific. `ModelCatalog(ctx)` decodes OpenAI's `models` array, keeps entries with `visibility == "list"` and nonblank `slug`, and preserves server ordering. Its internal `ChatGPTModel` retains `slug`, `display_name`, and `visibility`.

**Current UI behavior:** `Models()` projects the catalog to `[]string` slugs. The control plane exposes that slug list, and the dashboard and CLI pickers currently display the slugs themselves, not `display_name`. OpenAI recommends friendly display names, but that presentation is not implemented by these picker contracts. The selected slug is stored and sent as `model`.

Refresh discovery after changing credentials/accounts; do not reuse an API-key catalog or another account's choices. There is no built-in subscription default model. A successful empty catalog is returned as an empty live list; discovery failures return errors instead of `source: "default"` with a fabricated model. A configured/manual model string is not preflighted against the catalog on every write: OpenAI ultimately enforces model eligibility at request time.

## 9. Inference, tools, and continuation

All ChatGPT Subscription inference uses HTTP/SSE Responses, including Myaaw's synchronous `Chat` method, which collects the streaming result internally. It does not use Chat Completions or Codex app-server.

Each request has the following invariants:

```json
{
  "model": "<MODEL_SLUG_FROM_DISCOVERY>",
  "input": [
    {
      "role": "developer",
      "content": [{ "type": "input_text", "text": "<AGENT_INSTRUCTIONS>" }]
    },
    {
      "role": "user",
      "content": [{ "type": "input_text", "text": "<USER_MESSAGE>" }]
    }
  ],
  "store": false,
  "stream": true,
  "include": ["reasoning.encrypted_content"]
}
```

This is an explanatory wire-body example, not an invitation to copy OAuth tokens into an external client.

- `input` is an array containing all context needed for that request. The agent may first apply its normal context transformation, image eviction, and local transcript compaction; this does not mean every historical turn is sent forever.
- `system` messages are mapped to `developer`. Supported message roles are developer, user, and assistant, with tool results represented separately. Developer/user text uses `input_text`; assistant history without native continuation metadata uses `output_text`, including turns imported from another provider and previously stored error messages.
- There is no `previous_response_id` or remote `conversation` linkage. Each tool-loop step resends the required context and continuation items.
- User/tool images become `input_image`: HTTP(S) URLs or PNG/JPEG/WebP/GIF base64 data URLs. Legacy image arrays can be normalized from recognized base64 encodings. Images on developer/assistant messages and unsupported content shapes/types are rejected.

### Local function tools

Myaaw wraps supplied function schemas in a namespace named `myaaw`. Function names must be unique and match `[a-zA-Z0-9_-]{1,64}`; functions default to `strict: false` unless the schema explicitly supplies a value. Other top-level tool types are rejected by this adapter.

A model's completed `function_call` is mapped to a local Myaaw tool call. The agent executes the local tool under the normal Myaaw tool/approval policy, appends a `function_call_output` with the matching `call_id`, and requests the next assistant turn. Multiple calls require their corresponding results before the next non-tool message. Duplicate/missing call IDs, orphan/duplicate results, missing results, invalid argument JSON, unknown tool names, and foreign namespaces produce errors rather than guessing a mapping. Qualified returned names such as `myaaw.lookup` are normalized to the local name.

Local shell, MCP, or computer tools exposed as supported function schemas are distinct from OpenAI-hosted Responses tools. This provider does not turn them into hosted MCP, Code Interpreter, or native computer-use calls.

### Visible thinking summaries

For recognized reasoning families, Myaaw requests `reasoning: {"summary": "auto"}`. It does not set `reasoning.effort`, so the model's default effort remains unchanged. The current conservative name check covers GPT-5/GPT-6 families except `-chat` variants, `o3`/its non-mini variants, and `o4-mini` variants. Older/non-reasoning and unrecognized model names receive no `reasoning` parameter. This local heuristic is not a guarantee of upstream capability or account eligibility; new naming families may require an adapter update.

Visible summary text is read from `response.reasoning_summary_text.delta`/`.done`, `response.reasoning_summary_part.added`/`.done`, reasoning output-item snapshots, and the final `response.completed.output[].summary`. Explicit `response.reasoning_text` deltas/done events remain accepted for compatible streams. Thinking chunks become `Message.Thought` and the normal agent/channel `thought` events, including when the summary appears only at completion. Summary parts are tracked by output/part index, so a repeated terminal snapshot is not displayed twice and two distinct parts with identical text are not collapsed.

The adapter preserves replayable raw summary fields separately from their display projection. Raw-content indices and summary-part indices are tracked separately, and visible chunks preserve stream delivery order even when those event families interleave. A malformed identity/index or a contradictory summary snapshot is rejected instead of appending misleading duplicate text. Partial thinking remains available when a stream is interrupted, but incomplete tool calls are never executed.

A reasoning summary is not the model's raw hidden reasoning. Encrypted continuation bytes and reasoning-token counters are never converted to thinking text. Even with summary enabled, an upstream model/request can return no summary; Myaaw does not fabricate one or increase effort to force its appearance. See [OpenAI reasoning summaries](https://developers.openai.com/api/docs/guides/reasoning#reasoning-summaries).

### Encrypted reasoning continuation

The provider requests `reasoning.encrypted_content` and stores supported output items in assistant `provider_metadata.openai_chatgpt`. Reasoning item IDs, summary arrays, encrypted content when returned, assistant output, and function-call data are preserved for replay. If the terminal snapshot omits ciphertext, the parser can recover it from the corresponding completed output item, not an unfinished added item.

Encrypted content is opaque continuation state, **not displayable thought**. Only explicit visible reasoning text/summaries are surfaced as thinking events separately. Preserve an original assistant turn and its metadata together; the adapter rejects metadata that no longer matches the turn's text/tool calls. Agent request copies filter metadata to the selected provider type, and transformations that change the paired payload discard stale metadata. Local persistence retains continuation state so it can be resumed; `store: false` is not a ban on local conversation storage.

Sources: [request/input/tool mapping](../../internal/provider/chatgpt.go), [stream and continuation parser](../../internal/provider/chatgpt_stream.go), [agent message isolation](../../internal/agent/messages.go), [agent loop](../../internal/agent/engine.go), [agent-loop reference](../agents/agent-loop.md).

## 10. Preview restrictions and implemented subset

OpenAI's ChatGPT plan-usage preview is not the complete paid API surface, and Myaaw's adapter implements a narrower subset still. Eligibility depends on current model, account, workspace policy, consent, and plan availability; a ChatGPT subscription is not a promise that every API model/capability is usable.

| Capability | Current boundary |
| :--- | :--- |
| Text, images, text/refusal output | Implemented by the adapter, subject to model/account support. |
| Namespaced local function tools and tool-result loops | Implemented in the `myaaw` namespace. |
| Opaque encrypted reasoning continuation and token usage | Implemented for supported Responses output items. |
| Native file input / upload / document parsing | **Not implemented by this provider adapter.** OpenAI's preview permits model-supported file inputs, but does not support the Files upload API. Myaaw does not serialize native `input_file` blocks or add a provider-native document parser/upload client. |
| Myaaw chat document attachments | A separate local workflow: files are saved on the agent host and their paths enter an `<attached>` block; local tools can read/extract content. An attachment chip is not evidence of native OpenAI file ingestion. |
| Audio/video input, transcription, native realtime voice | Unsupported by this subscription route/adapter. Separate configured speech services are not supplied by the OAuth grant. |
| OpenAI web search | Official preview availability is model/policy-dependent; this adapter does not expose the native web-search tool type. |
| Image generation, file search, Code Interpreter, native computer use, hosted MCP/connectors, Responses `tool_search` | Unsupported preview tools; not implemented here. Client-side execution does not make Responses `tool_search` supported. |
| Custom tools / `additional_tools` input items | Described by the official preview, but not implemented by this function-only adapter. |
| Background jobs, remote stored conversation state, WebSocket transport | Not implemented by this provider; HTTP requests always use SSE with `store: false`. |

For local attachment behavior, see [Control Plane chat attachments](../api/control-plane.md#attaching-files) and [Agent Context](../agents/context.md).

The current official preview requires omission of `background`, `conversation`, `max_output_tokens`, `max_tool_calls`, `metadata`, `moderation`, `multi_agent`, `prompt`, `prompt_cache_retention`, `safety_identifier`, `temperature`, `top_logprobs`, `top_p`, `truncation`, and `user`. It also requires omitting `previous_response_id` over HTTP and avoiding explicit system message items. Myaaw's request struct sends model, input, store, stream, include, optional namespaced tools, and the optional summary-only reasoning configuration described above; it is not a generic Responses-parameter pass-through. Local `provider_metadata` is not the unsupported top-level wire `metadata` field. Top-level `programmatic_tool_calling` tools are not supported either.

See OpenAI's [Models and inference](https://developers.openai.com/siwc/token-sharing-open-source/models-and-inference) and [Preview limitations](https://developers.openai.com/siwc/token-sharing-open-source/preview-limitations) for upstream policy. Changes upstream do not automatically add serialization or parsing support to Myaaw.

## 11. Limits, streaming success, and failures

### Completion is strict

HTTP `200`, visible text, finished tool-argument deltas, and `[DONE]` alone are **not** successful inference. The parser requires a fully delimited `response.completed` event with a response object, no response error, and no contradictory non-completed status. It validates supported output items, completed tool arguments, IDs, and namespaces before publishing executable tool calls.

`response.failed`, `response.incomplete`, `error`/`response.error`, malformed events, unsupported output items/content, stream-read failures, and EOF without `response.completed` are errors. A terminal JSON payload without its final blank-line SSE delimiter is truncated, not successful. Multiple SSE `data:` lines are joined before JSON parsing. On failure the provider retains partial text but does not expose unfinished calls for execution. One provider lifecycle owner emits the final `StreamDone` with normal/error/aborted outcome and any error.

Usage maps input/output/total tokens plus cached-input and reasoning-token details from the completed response. Token usage is not a live remaining-plan-quota meter or a guarantee of paid-API pricing.

### Bounds and retries

- Browser authorization: maximum ten minutes; control-plane status retention extends two minutes beyond its expiry timestamp.
- Model discovery: 15-second request context, decoding through an 8 MiB reader limit.
- Token/JWKS HTTP client: 30-second timeout; post-rotation identity validation has a bounded 30-second context.
- Inference: the configured stream request timeout, default 180 seconds (`LLM_STREAM_TIMEOUT_SECONDS`, or `bot.stream_timeout_seconds` in config). A shorter caller/run deadline still applies.
- Stream parsing: an 8 MiB scanner-token/line ceiling and a 16 MiB accumulated SSE-data guard per event. Non-200 error bodies are retained up to 1 MiB for structured diagnostics.

**The ChatGPT provider does not automatically retry discovery/inference HTTP requests or replay an interrupted stream.** In particular, it does not use the generic API-key provider retry loop, force-refresh-and-replay a rejected inference request, or retry quota errors after delivering text. This avoids duplicate text, tool actions, or plan consumption. Token refresh before a request is credential maintenance, not an inference retry. Retry deliberately after assessing the failure and any partial output.

### Quotas and structured errors

Limits are controlled by OpenAI and can apply to the account or app registration. A quota/availability failure can arrive in `response.failed` after streaming has begun. There is no automatic switch to another account or paid API billing.

| Error or symptom | Action |
| :--- | :--- |
| `subscription_sharing_usage_limit_exceeded` | Pause requests and check [ChatGPT usage settings](https://chatgpt.com/settings/usage). Do not repeatedly reconnect to bypass a limit. |
| `subscription_sharing_usage_unavailable` | Keep the credential and try later; this is not proof that the registration must be deleted. |
| `subscription_sharing_user_not_eligible` | Check account/workspace eligibility and policy; explicitly choose an eligible registration or another provider. |
| `subscription_sharing_unsupported_capability` | Check model, content, and tool types against the implemented subset and preview restrictions. |
| No Thinking text while reasoning-token usage is nonzero | Install the updated adapter and check that the selected model supports reasoning summaries. Recognized reasoning models request `summary: "auto"`, but tokens/ciphertext alone are not visible summaries and an upstream empty summary stays empty. |
| `refresh_required` | Run discovery/inference to refresh when permitted; status itself will not do so. |
| `reauthorization_required` / confirmed unusable refresh token | Reauthorize the saved registration using the local browser. |
| `unavailable` after restore or service-user change | Sign in under the intended host OS account and update the provider reference. |
| `model_not_found` naming a previous provider's model | Confirm the selected provider's `default_model` is a slug available to the ChatGPT account. Changing provider type clears the dashboard model field; select a model for the new type before saving. An ordinary Telegram turn synchronizes its user's model to the loaded agent default. |
| `invalid_value` for `input_text` in assistant history | Update the Myaaw binary and restart the agent. Assistant text must be serialized as `output_text`; older Telegram or imported-provider history can expose this converter bug even when a new dashboard conversation works. No transcript deletion or OAuth reconnect is required for this serialization fix. |
| Login `403` | Check socket peer, raw Host, browser Origin/Referer, and whether the browser actually runs on the API host. |
| EOF without completion | Treat the partial response as interrupted. Check connectivity/timeout; retry intentionally, not as a continuation success. |

`ChatGPTAPIError` retains HTTP status, code, parameter, request ID, message, and bounded raw body for diagnostics. Stream API errors can report HTTP `200` because the transport succeeded before the event failed. Control-plane model lookup wraps failures in a sanitized configuration error rather than exposing every upstream detail. Do not indiscriminately upload raw bodies or conversation content to logs/support reports.

Sources: [error mapping and discovery](../../internal/provider/chatgpt.go), [SSE parsing](../../internal/provider/chatgpt_stream.go), [timeout helper](../../internal/provider/retry.go), [configuration](../../internal/config/config.go), [OpenAI errors and recovery](https://developers.openai.com/siwc/token-sharing-open-source/errors-and-recovery).

## 12. Sharing, disconnect, deletion, and revocation

Multiple provider entries and agents under the same local installation may reference the same credential UUID. They share that registration's token rotation, permissions, and applicable plan limits, not separate grants per provider name. Reauthorization or a terminal refresh failure therefore affects all references to that record.

| Operation | What it does | What it does not do |
| :--- | :--- | :--- |
| `disconnect-chatgpt` / provider disconnect API | Clears that provider's `credential_id` and any key/endpoint fields; retains its type, OAuth mode, and model. | Does not delete the host credential or revoke remote app access; other references continue to work. |
| Provider deletion | Removes the agent's provider configuration entry. | Does not delete shared host credentials or revoke the grant. |
| `Manager.Delete(id)` | Explicit auth-package operation deleting the local account/pending registration, including a recovery binding under that ID. | Not a public disconnect command/API, not remote revocation, and not the complete official sign-out flow. |
| Manage connected apps in ChatGPT Settings | Lets the user revoke/disconnect the remote app grant. | Does not automatically clear every local config reference or immediately update cached local status. |

The official sign-out contract includes refresh-token revocation before clearing tokens while retaining registration/account mapping. Myaaw's local `Delete` deliberately does not implement or claim those semantics. Before deleting a local record, identify every provider/agent sharing it; those references will become unavailable. Account deletion does not reset `host.json`.

## 13. Backups, migration, and privacy

- **Agent backups are not OAuth credential backups.** The host-global credential store is outside agent roots and archives. Backups can contain `credential_id` references but not the underlying host credential from this store.
- **Restore on another host requires reconnection.** Create a new local sign-in on the destination, discover its eligible models, and replace the restored references. A copied UUID alone is not portable authentication. There is no automatic import from Codex/other clients or Myaaw credential-transfer workflow. Do not clone `host.json` into multiple independent hosts as a migration shortcut; host identity is meant to be stable per host, not shared by unrelated installations.
- **Same host is not enough if the OS account differs.** CLI sign-in and background services must resolve to the same owning user's host store. Protect that account, its filesystem, and any separately managed whole-host backup containing credentials.
- **OAuth secrets stay on the host, not in status responses.** Status exposes local IDs, account labels, expiry, and sanitized failures. Raw `Credential` values include secrets and must never be serialized as views. Authorization URLs can contain identity-token hints: do not put them in telemetry, screenshots, browser-URL support captures, or application logs. The CLI intentionally does not print them.
- **Inference still leaves the machine.** Authorization shares identity/consent metadata with OpenAI. Requests send selected instructions, conversation content, images, function schemas/results, and continuation items to OpenAI under the grant. Local tool output can include sensitive file contents; normal tool approvals and data-minimization practices still matter.
- **`store: false` is not a universal retention/privacy claim.** It disables Responses storage for these requests; it does not mean no network transfer, no OpenAI policy-governed processing, or no local history. Local transcripts, uploaded files, continuation metadata, logs, and optional traces have their own persistence/backup policies. Encrypted reasoning continuation is sensitive opaque provider state, not an OAuth credential.

See [Backup & Restore](../cli/backup.md), [Observability](../agents/observability.md), and the [auth storage contract](../../internal/auth/chatgpt/README.md) before sharing diagnostic material or moving an installation.

## 14. Source map and related references

| Area | Source |
| :--- | :--- |
| OAuth API and storage guarantees | [Auth README](../../internal/auth/chatgpt/README.md), [manager](../../internal/auth/chatgpt/manager.go), [attempt](../../internal/auth/chatgpt/attempt.go), [token handling](../../internal/auth/chatgpt/token.go) |
| Discovery, input mapping, tools, API errors | [ChatGPT provider](../../internal/provider/chatgpt.go) |
| SSE terminal validation and continuation | [ChatGPT stream](../../internal/provider/chatgpt_stream.go) |
| Connection status, session lifecycle, detach | [Control-plane service](../../internal/services/controlplane/service/chatgpt.go), [handlers](../../internal/services/controlplane/handler/chatgpt.go) |
| CLI and onboarding | [Provider commands](../../cmd/myaaw/provider_chatgpt.go), [auth normalization](../../internal/onboard/chatgpt.go), [onboarding service](../../internal/services/controlplane/service/onboard.go) |
| Dashboard contracts | [Connection component](../../webui/src/lib/components/common/ChatGPTConnection.svelte), [Providers page](../../webui/src/routes/app/providers/+page.svelte), [new-agent page](../../webui/src/routes/app/new/+page.svelte), [API helper](../../webui/src/lib/apis/providers.ts) |
| Local regression coverage | [Auth tests](../../internal/auth/chatgpt/chatgpt_test.go), [recovery tests](../../internal/auth/chatgpt/recovery_test.go), [provider tests](../../internal/provider/chatgpt_test.go), [stream tests](../../internal/provider/chatgpt_stream_test.go), [handler tests](../../internal/services/controlplane/handler/chatgpt_test.go) |

The auth/provider regression tests use local test issuers, signed JWTs, and HTTP/SSE fixtures. They are not evidence of a successful live OpenAI login, current account eligibility, or live inference. This reference does not claim such a test.

Related Myaaw documentation:

- [Documentation index](../README.md).
- [ChatGPT Subscription CLI guide](../cli/chatgpt-subscription.md), [Providers](../cli/provider.md), [Onboarding](../cli/onboard.md), and [Config](../cli/config.md).
- [Control Plane API](../api/control-plane.md#chatgpt-subscription), [Agent Context](../agents/context.md), and [Agent Loop](../agents/agent-loop.md).
- [Provider overview](README.md), [Gemini](gemini.md), and [OpenAI-compatible](openai-compatible.md).

Official OpenAI references:

- [ChatGPT plan usage overview](https://developers.openai.com/siwc/token-sharing-open-source).
- [Registration and sign-in](https://developers.openai.com/siwc/token-sharing-open-source/sign-in).
- [Accounts and sessions](https://developers.openai.com/siwc/token-sharing-open-source/profiles-and-sessions).
- [Token reference](https://developers.openai.com/siwc/token-sharing-open-source/token-reference).
- [Models and inference](https://developers.openai.com/siwc/token-sharing-open-source/models-and-inference).
- [Preview limitations](https://developers.openai.com/siwc/token-sharing-open-source/preview-limitations).
- [Errors and recovery](https://developers.openai.com/siwc/token-sharing-open-source/errors-and-recovery).
