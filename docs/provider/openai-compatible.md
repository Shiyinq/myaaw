# OpenAI-compatible

Myaaw's `openai` provider connects to OpenAI's API and services that implement the **Chat Completions** protocol. It authenticates using an API key and supports configurable endpoints, making it the integration for services such as DeepSeek, OpenRouter, and compatible local servers.

This is separate from [ChatGPT Subscription](chatgpt-subscription.md): a consumer ChatGPT subscription does not fund ordinary OpenAI API-key requests. Do not paste a ChatGPT OAuth token into this provider's `api_key` field.

Related: [Provider index](README.md) · [Gemini](gemini.md) · [Provider CLI](../cli/provider.md) · [Onboarding](../cli/onboard.md) · [Control Plane API](../api/control-plane.md).

## Contents

1. [Protocol and authentication](#1-protocol-and-authentication)
2. [Base URLs: Myaaw appends `/v1`](#2-base-urls-myaaw-appends-v1)
3. [Setup](#3-setup)
4. [Model discovery and its limits](#4-model-discovery-and-its-limits)
5. [Chat requests, tools, and images](#5-chat-requests-tools-and-images)
6. [Billing, credentials, and endpoint trust](#6-billing-credentials-and-endpoint-trust)
7. [Troubleshooting](#7-troubleshooting)

## 1. Protocol and authentication

| Property | Current behavior |
| :--- | :--- |
| Provider type | `openai`, including when the service is not OpenAI. |
| Authentication | `Authorization: Bearer <api_key>` for chat and model listing. |
| Default base URL | `https://api.openai.com`. |
| Chat endpoint | `POST <base_url>/v1/chat/completions`. |
| Discovery endpoint | `GET <base_url>/v1/models`. |
| Streaming | OpenAI-compatible `data:` SSE, with text, reasoning, tool calls, and usage handling. |
| Built-in default model | `gpt-4o`; a source default, not a guarantee of account/model availability. |
| OAuth | Not used. Select `openai_chatgpt` for ChatGPT Subscription. |

Both request types send `Content-Type: application/json`. The adapter does not expose arbitrary authentication headers, Azure deployment parameters, organization/project headers, a client certificate, or per-provider request-body overrides. A service requiring `api-key` rather than Bearer authentication or deployment-specific URLs is not automatically compatible.

A service implementing only Responses, legacy text completions, Anthropic Messages, or the native Gemini API will not work simply because its base URL is configurable. Compatibility is about the endpoints, authentication, request fields, response schema, and streaming format—not just accepting an OpenAI SDK.

## 2. Base URLs: Myaaw appends `/v1`

**Do not include the final `/v1`, `/models`, or `/chat/completions` in `base_url`.** Myaaw appends `/v1/models` and `/v1/chat/completions` itself. Use a scheme, authority, and any gateway prefix before that suffix, without a trailing slash, query string, or fragment.

| Service example | Myaaw `base_url` | Resulting chat URL |
| :--- | :--- | :--- |
| OpenAI API | `https://api.openai.com` | `https://api.openai.com/v1/chat/completions` |
| DeepSeek preset | `https://api.deepseek.com` | `https://api.deepseek.com/v1/chat/completions` |
| OpenRouter | `https://openrouter.ai/api` | `https://openrouter.ai/api/v1/chat/completions` |
| Local Ollama compatibility API | `http://localhost:11434` | `http://localhost:11434/v1/chat/completions` |
| Gateway with a routing prefix | `https://gateway.example/team-a` | `https://gateway.example/team-a/v1/chat/completions` |

These examples describe URL construction, not a live compatibility or model-availability certification. Check the service's current documentation. If it documents `https://service.example/v1` as an SDK `base_url`, configure `https://service.example` in **Myaaw**, because the two clients construct paths differently.

For example, configuring `https://openrouter.ai/api/v1` in Myaaw would request `/api/v1/v1/chat/completions`. Do not compensate by editing the model name; correct the endpoint.

A local URL is resolved from the machine/process running Myaaw. In a container, `localhost` refers to that container, not automatically the host's model server. The dashboard performs discovery through the control plane, so its `localhost` is also server-side, not the browser's machine.

## 3. Setup

### Interactive onboarding and provider creation

For a new agent, run `myaaw onboard` and choose **OpenAI (or compatible)**, or the **DeepSeek** onboarding preset. The DeepSeek preset stores `type: "openai"`, `base_url: "https://api.deepseek.com"`, and initially suggests `deepseek-chat`; it is not another protocol adapter.

For an existing agent:

```bash
myaaw --agent personal provider create
# Type: OpenAI (or OpenAI Compatible)
# Custom name: openai-api
# Base URL: https://api.openai.com
# API key: enter at the hidden prompt
# Select an available chat model

myaaw --agent personal provider default openai-api
myaaw --agent personal provider list
myaaw --agent personal provider set-model openai-api
```

The integration name (`openai-api`, `deepseek`, or `local-model`) is a per-agent config-map key. It does not change the wire type or automatically configure an endpoint. The first provider becomes the default automatically; later additions require an explicit default switch.

CLI provider creation/model selection uses live discovery, with manual model entry when discovery fails or returns no models. Interactive onboarding also allows manual selection. Manual entry is useful for gateways without a model-list endpoint but does not validate access or tool compatibility.

After setup, a small smoke request is:

```bash
myaaw --agent personal chat -m "Reply with a short greeting."
```

That is a request for you to run, not a claim that a live API test has been performed. It can consume the selected service's quota or incur charges.

### Headless onboarding

Provide `MYAAW_API_KEY` through your shell/CI secret mechanism before running the command. Prefer that environment input to a literal key in shell history or `--api-key` process arguments.

```bash
# OpenAI API; MYAAW_API_KEY must already contain an OpenAI API key.
myaaw onboard --yes --agent personal \
  --provider openai --provider-name openai-api \
  --base-url https://api.openai.com \
  --model gpt-4o --channel skip
```

For another service, supply its key and change the endpoint/model explicitly:

```bash
# MYAAW_API_KEY must contain a DeepSeek key for this example.
myaaw onboard --yes --agent research \
  --provider openai --provider-name deepseek \
  --base-url https://api.deepseek.com \
  --model deepseek-chat --channel skip
```

Models are examples, not guaranteed current offerings. Headless API-key onboarding can reuse an applicable saved key/model or use the built-in default model; it does not verify upstream inference before saving. Reconfiguring an initialized agent requires `--force`. `--channel skip` preserves channel settings rather than disabling channels. See the [headless onboarding rules](../cli/onboard.md#headless-mode-ci-scripts).

`MYAAW_API_KEY` is an onboarding input. After setup the stored provider entry supplies the key; changing that environment variable alone does not replace an already selected JSON provider. Legacy runtime `.env` settings are covered in the [configuration reference](../cli/config.md).

### Dashboard

1. Select the agent and open **Providers** (`/app/providers`), or configure a provider in the new-agent wizard.
2. Choose `openai`, enter an integration name, the correctly formed base URL, and the service's API key.
3. Use **Discover models**, check the discovery result/warning, and select a chat-capable model. Use manual entry if the service does not supply a suitable catalog.
4. Save the provider. Set it as the default if it should be used for new requests.

When editing, a blank API-key field **keeps the stored key**; it does not clear or revoke it. An omitted key also preserves it through the provider update API. A nonempty replacement changes the stored key.

**Discovery while editing with a blank key uses the entire saved provider**, not unsaved base-URL changes. To test new endpoint credentials before saving, provide the intended key explicitly, or save and then discover through that provider. Only submit a key to an endpoint you trust.

### Per-agent configuration

Merge this fragment into `~/.myaaw/personal/config/config.json`; do not overwrite unrelated channels, approvals, speech settings, or other providers:

```json
{
  "default_provider": "openai-api",
  "providers": {
    "openai-api": {
      "type": "openai",
      "base_url": "https://api.openai.com",
      "api_key": "REPLACE_WITH_YOUR_OPENAI_API_KEY",
      "default_model": "gpt-4o"
    },
    "deepseek": {
      "type": "openai",
      "base_url": "https://api.deepseek.com",
      "api_key": "REPLACE_WITH_YOUR_DEEPSEEK_API_KEY",
      "default_model": "deepseek-chat"
    }
  }
}
```

Leave `auth_mode` and `credential_id` absent for API-key providers. Do not set `auth_mode: "oauth"` for `type: "openai"`. Keep the intended service/model explicit when hand-editing: `gpt-4o` is not a sensible universal default for DeepSeek or local servers.

### Local servers without key enforcement

A local server must expose the OpenAI-compatible endpoints above and provide a loaded model that supports the intended features. The `openai` adapter sends a Bearer header even if the configured key is empty; it does not negotiate an alternative local authentication scheme.

Headless API-key onboarding requires a nonempty key. If a trusted local server explicitly ignores authentication and accepts a dummy value, a non-secret placeholder can satisfy that setup requirement. Do not use this as a workaround for a server that actually requires authentication. Use its real credential where applicable.

Using `type: "openai"` with Ollama's compatibility API is different from the separately registered native `ollama` adapter. This guide and the standard provider form describe the OpenAI-compatible path.

## 4. Model discovery and its limits

The adapter sends `GET <base_url>/v1/models` with a 15-second timeout and expects this shape:

```json
{
  "object": "list",
  "data": [
    { "id": "service-specific-chat-model", "object": "model" }
  ]
}
```

It returns `data[].id` in the server's order. It does not paginate, infer model capabilities, filter out embedding/audio/non-chat models, or check that a returned model supports tools/images. Use the exact model ID required by the service, including any service-specific prefix; do not add Gemini's `models/` prefix generically.

### Control-plane discovery

| Method | Endpoint | Behavior |
| :--- | :--- | :--- |
| `POST` | `/providers/models` | Accepts `type`, `base_url`, and `api_key` before a provider is saved. |
| `GET` | `/agents/:name/providers/:id/models` | Uses that saved provider's endpoint/key without returning its plaintext credential. |

For example, the body for unsaved-provider discovery is:

```json
{
  "type": "openai",
  "base_url": "https://api.deepseek.com",
  "api_key": "REPLACE_WITH_YOUR_DEEPSEEK_API_KEY"
}
```

The endpoints belong to **Myaaw**, not the upstream service. Use a Myaaw control-plane bearer token to call them; the body key is a separate upstream credential. When going through the dashboard's same-origin proxy, the paths have an `/api` prefix. See [Control Plane API](../api/control-plane.md#10-providers) for management authentication and CRUD requests.

A successful result uses `source: "live"`. For API-key providers, a failed lookup or an empty list falls back to the built-in default:

```json
{
  "models": ["gpt-4o"],
  "source": "default",
  "warning": "Model discovery failed; illustrative message, not a successful upstream lookup."
}
```

Inspect `source` as well as `warning`: an empty successful list can trigger fallback without a warning. HTTP 200 from Myaaw or seeing `gpt-4o` in a picker is **not proof of upstream authentication, availability, or compatibility**. The fallback is not automatic runtime failover to another model/service. ChatGPT Subscription deliberately does not use this fallback behavior.

## 5. Chat requests, tools, and images

### Wire request

Streaming requests contain these adapter-managed fields:

```json
{
  "model": "service-specific-chat-model",
  "messages": [{ "role": "user", "content": "Hello" }],
  "stream": true,
  "stream_options": { "include_usage": true }
}
```

When tools are available, Myaaw adds `tools` and `tool_choice: "auto"`. Tool definitions use the Chat Completions function shape; argument JSON is serialized as a string in assistant tool calls. The agent executes permitted local tools and sends subsequent `tool` messages paired with their call IDs. The provider does not execute hosted tools by itself.

System messages stay system messages in this adapter. History/context is constructed locally and sent in `messages`; this integration neither imports ChatGPT conversation history nor creates a persistent Responses conversation.

`ProviderConfig` does not offer temperature, top-p, max-token, JSON-schema output, custom-header, or arbitrary request-parameter fields. A custom endpoint does not expose every feature in the upstream API. Models requiring different roles/parameters or Responses-only behavior may need another adapter rather than a model-name change.

### Streaming and reasoning

- Text is read from Chat Completions choice deltas; function names/argument chunks are accumulated into complete tool calls.
- `reasoning_content`, when the service supplies it, is kept separate from visible answer text. This is optional service output, not guaranteed access to hidden reasoning.
- Only the first choice is used. The adapter handles common finish reasons and tolerates some compatible-server variations.
- Usage maps prompt, completion, total, reasoning, and cached-input token counts when reported. DeepSeek's `prompt_cache_hit_tokens` spelling is recognized. Counts and configured cost estimates are not an upstream invoice.
- Some gateways reject `stream_options.include_usage`. This adapter always sends it when streaming through `type: "openai"`; there is no provider-level toggle. Check gateway compatibility instead of assuming a valid key is the problem.

Streaming has a configurable total budget (`LLM_STREAM_TIMEOUT_SECONDS`, default 180 seconds) and a 60-second idle watchdog. The shared retry budget uses `LLM_REQUEST_RETRIES` (default one retry) for transport failures and selected transient statuses: 408, 425, 429, 500, 502, 503, and 504. It does not replay a stream after text/thinking output has reached the caller. Non-streaming chat has a 120-second request timeout and shares the bounded HTTP retry policy. Model discovery is a separate 15-second request, not an inference health check.

The Chat Completions parser is tolerant of malformed frames and accepts common stream termination variants. Do not apply the subscription provider's strict `response.completed` requirement to this protocol. Proxy buffering, malformed SSE, and dropped connections still require diagnosis; a partial answer is not evidence of a complete tool round.

### Images, files, and speech

- Vision content uses Chat Completions `image_url` content items. The normal bot-service path builds these blocks for incoming user images. Whether remote/data image URLs are accepted depends on the selected endpoint and model.
- The converter forwards `Message.Content` blocks; it does not independently convert the separate legacy `Message.Images` slice into OpenAI content items. Clients/tool integration must produce a representation this adapter actually sends.
- Ordinary uploaded files are stored locally and referenced in Myaaw's `<attached>` block. Their paths do not grant the remote service access to the filesystem; enabled/approved tools must read and extract content as needed.
- The adapter does not automatically call OpenAI Files, Assistants, hosted Code Interpreter/file search, image generation, audio transcription, TTS, or Realtime APIs.
- `myaaw voice-classic` can use an OpenAI-compatible LLM alongside **separate** configured speech providers. `myaaw voice` is the Gemini Live path, not an OpenAI Realtime client. See [Voice](../cli/voice.md).

## 6. Billing, credentials, and endpoint trust

The API key identifies the chosen upstream account/project/service. API billing, model access, quotas, regional restrictions, and data policies are governed by that service, not by Myaaw's provider name. Multiple agents using the same key may share quota. Tools and scheduled jobs can generate additional model turns; bounded retries can also consume provider resources.

API keys are stored in agent configuration, not the ChatGPT host OAuth store. Provider responses mask keys, and current config writers use atomic replacement with owner-only file permissions on Unix. This is **not encryption at rest**. Restrict OS access, protect `.env`, and review copies and archives. Unlike the excluded host OAuth store, agent backups can contain API keys in configuration; treat them as secrets.

A custom base URL sends the Bearer key, system/context text, history, images, and tool results to that endpoint. Use HTTPS for non-loopback services and trust the gateway operator. Do not reuse an OpenAI key at an unrelated endpoint unless that forwarding is intentional and trusted. Authentication errors and gateway responses may contain sensitive operational details; redact diagnostics before sharing them.

Revoking an API key is done with the upstream service. Removing the Myaaw provider only removes local configuration; it does not revoke remote access or remove keys copied into other agents/backups.

## 7. Troubleshooting

| Symptom | Checks |
| :--- | :--- |
| 404 or `/v1/v1/...` | Remove the final `/v1` from Myaaw's base URL; verify gateway routing prefixes and both endpoint paths. |
| 401/403 | Verify the service/key pairing, project permissions, model access, key restrictions, and Bearer authentication support. A ChatGPT subscription is not an API key. |
| Discovery shows `gpt-4o` for a non-OpenAI service | Check `source`/`warning`; this can be a default fallback, not a live catalog. Choose the service's actual model ID. |
| Missing model or `/models` unsupported | The adapter does not paginate or synthesize an upstream catalog. Enter a confirmed chat model manually if the service supports inference without listing. |
| Model listed but chat/tools fail | Model discovery does not filter capabilities. Confirm Chat Completions, system-message support, streaming, function tools, and exact request fields. |
| 400 mentioning `stream_options` | The gateway may not implement usage-enabled streams. No provider-config switch currently suppresses that field. |
| 429 or transient 5xx | Inspect the service's quota/billing/status and local concurrency. Retries are bounded; they do not extend quota. |
| Stream stalls or cuts off | Check reverse-proxy buffering/timeouts, network reliability, the idle watchdog, and total request budget. Do not blindly repeat side-effecting tool work. |
| New endpoint seems ignored in the edit form | Blank-key discovery uses saved settings. Supply intended credentials explicitly or save the endpoint before rediscovering. |
| Local server unreachable | Resolve the address from the Myaaw/control-plane host or container; browser reachability alone is insufficient. |
| No image/audio/file capability despite a compatible endpoint | Check the actual input representation and model support. This adapter does not automatically enable native upload/audio/hosted tools. |

## Implementation sources

- [OpenAI request/message/model types and API-key adapter](../../internal/provider/openai.go)
- [Chat Completions HTTP and SSE implementation](../../internal/provider/openai_stream.go)
- [Factories, explicit provider configuration, and defaults](../../internal/provider/provider.go)
- [HTTP retry policy](../../internal/provider/retry.go) and [stream retry/idle handling](../../internal/provider/stream_retry.go)
- [CLI provider management](../../cmd/myaaw/provider.go), [onboarding](../../cmd/myaaw/onboard.go), and [preset definitions](../../internal/onboard/capabilities.go)
- [Control-plane discovery/fallback](../../internal/services/controlplane/service/models.go) and [provider CRUD/secret views](../../internal/services/controlplane/service/config.go)
- [Agent configuration and private writes](../../internal/config/config.go)
- [Attachment context helpers](../../internal/provider/attachments.go) and [conversation input handling](../../internal/services/bot/service/conversation.go)
- [API-key wire-compatibility regression tests](../../internal/provider/chatgpt_test.go)
