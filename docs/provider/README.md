# Provider integrations

Myaaw offers three primary provider integrations in its onboarding and dashboard: **Gemini**, **OpenAI-compatible**, and **ChatGPT Subscription**. These guides describe the actual adapters, configuration, authentication, supported workflows, and current limitations—not every feature advertised by an upstream API.

## Guides

| Guide | What it covers |
| :--- | :--- |
| [Gemini](gemini.md) | Native Gemini REST chat, API keys, model filters, tools/thought signatures, inline images, and the separate Live/STT/TTS paths. |
| [OpenAI-compatible](openai-compatible.md) | OpenAI API keys, DeepSeek/gateways/local servers, exact base-URL construction, Chat Completions streaming, model discovery, tools, and compatibility limits. |
| [ChatGPT Subscription](chatgpt-subscription.md) | Browser OAuth, credential storage/rotation, same-host login, account-specific models, Responses streaming, quotas, and shared-reference disconnect/recovery. |

For command syntax, use the [Provider CLI](../cli/provider.md) and [Onboarding](../cli/onboard.md). The [Control Plane API](../api/control-plane.md) documents management requests; [Chat](../cli/chat.md) covers using the selected integration.

## 1. Choose the right integration

| Property | Gemini | OpenAI-compatible | ChatGPT Subscription |
| :--- | :--- | :--- | :--- |
| Stored `type` | `gemini` | `openai` | `openai_chatgpt` |
| Credential | Gemini API key | The chosen service's API key | Browser OAuth registration selected by `credential_id` |
| `auth_mode` | Omit | Omit | `oauth` |
| Upstream protocol | Native Gemini `generateContent` | OpenAI Chat Completions | Public OpenAI Responses |
| Default origin | `https://generativelanguage.googleapis.com` | `https://api.openai.com` | Fixed `https://api.openai.com` |
| Custom endpoint | Must implement native Gemini paths/auth | Must implement the expected Chat Completions paths/auth | No arbitrary endpoint |
| Upstream stream | Incremental JSON array | Chat Completions SSE | Responses SSE; must finish with `response.completed` |
| Model IDs | Full resources such as `models/gemini-2.5-flash` | Service-specific `data[].id` | Account-specific `models[].slug` |
| Local function tools | Supported for recognized model families | Requires compatible endpoint/model | Namespaced local function tools |
| Main usage entitlement | Google API project/plan/quota | Selected service's API account/quota | Eligible ChatGPT plan authorization/limits |
| Initial browser login | No | No | Yes, on the Myaaw host |
| Credential location | Agent config or legacy environment | Agent config or legacy environment | Host-global protected store; agent config holds a reference |

**Subscriptions and API keys are not interchangeable.** A Gemini app subscription is not a Gemini API key. A ChatGPT subscription is not an OpenAI API key. Myaaw does not silently change billing paths or sign in to another account when the selected integration fails.

DeepSeek is an onboarding preset for `type: "openai"`, not a fourth wire protocol. OpenRouter and local servers also use `openai` only when they meet that adapter's compatibility requirements. Native `groq` and `ollama` factories also exist in the codebase, but they are not additional choices in the standard three-type provider form; their complete integrations are outside these guides.

## 2. Provider name, type, model, and default

These values have different jobs:

| Value | Meaning |
| :--- | :--- |
| Provider **name** | A local key within one agent's `providers` map, such as `work-google`, `deepseek`, or `personal-subscription`. |
| Provider **type** | Selects Myaaw's protocol/auth adapter: `gemini`, `openai`, or `openai_chatgpt`. |
| `base_url` | The upstream origin/routing prefix, where allowed; not the dashboard/control-plane address. |
| `default_model` | The upstream model ID used when no explicit model is supplied. It does not imply availability or grant access. |
| `default_provider` | The local provider name selected for that agent. It is not a model ID or provider type. |
| `credential_id` | A local reference to a verified ChatGPT OAuth registration; not an access token or API key. |

Each agent keeps its configuration at:

```text
~/.myaaw/<agent>/config/config.json
```

For example, one agent can have all three integrations available:

```json
{
  "default_provider": "google-chat",
  "providers": {
    "google-chat": {
      "type": "gemini",
      "base_url": "https://generativelanguage.googleapis.com",
      "api_key": "REPLACE_WITH_YOUR_GEMINI_API_KEY",
      "default_model": "models/gemini-2.5-flash"
    },
    "api-chat": {
      "type": "openai",
      "base_url": "https://api.openai.com",
      "api_key": "REPLACE_WITH_YOUR_OPENAI_API_KEY",
      "default_model": "gpt-4o"
    },
    "personal-subscription": {
      "type": "openai_chatgpt",
      "auth_mode": "oauth",
      "credential_id": "REPLACE_WITH_A_VERIFIED_LOCAL_CREDENTIAL_UUID",
      "default_model": "REPLACE_WITH_AN_AVAILABLE_MODEL_SLUG"
    }
  }
}
```

This is a **merge example**, not a complete replacement for an existing config. Model names are illustrative; confirm them using the account/project catalog. OAuth references are created by actual local browser sign-in, not by inventing a UUID. Never paste OAuth tokens into config.

Separate agents can use different defaults, models, and keys. They may deliberately share an upstream API key or a host ChatGPT credential, which can also mean shared quota. Per-agent configuration is not an OS sandbox or a separate upstream billing account.

## 3. Common setup workflow

### New agent

Run `myaaw onboard` or use the dashboard's create-agent wizard. Choose the integration, authorize it, select an appropriate model, and review before saving.

- API-key providers ask for an endpoint and key.
- ChatGPT Subscription starts a same-machine browser flow and saves only the resulting credential reference in agent config.
- API-key onboarding writes provider credentials when the review is saved. Successful OAuth sign-in writes host credentials before the agent review is saved; cancelling review does not delete that account record.
- Headless setup uses API-key input for `gemini`/`openai`, or a previously verified local `--credential-id` for `openai_chatgpt`. It cannot perform an interactive OAuth login by itself.

### Existing agent

```bash
myaaw --agent personal provider create
myaaw --agent personal provider list
myaaw --agent personal provider default google-chat
myaaw --agent personal provider set-model google-chat
```

For browser OAuth and account metadata:

```bash
myaaw --agent personal provider connect-chatgpt personal-subscription --default
myaaw --agent personal provider status-chatgpt personal-subscription
myaaw --agent personal provider disconnect-chatgpt personal-subscription
```

The dashboard's Providers page offers the equivalent management controls. The first provider becomes default automatically. Adding more providers does not enable automatic fallback or multi-provider routing.

Provider/model config changes are watched by the agent service and picked up for subsequent requests. This is not a promise to migrate an in-flight turn mid-stream. If a service has not picked up a saved change, inspect its logs and use the [service commands](../cli/service.md) to restart it.

## 4. Endpoint and model pitfalls

### Let each adapter append its own paths

- **OpenAI-compatible:** store `https://api.openai.com`, **not** `https://api.openai.com/v1`. For OpenRouter use `https://openrouter.ai/api`, not `/api/v1`. Myaaw appends `/v1/...`.
- **Gemini:** store `https://generativelanguage.googleapis.com`, not `/v1beta/models`. Myaaw appends `/v1beta/<model>:...`; use full `models/...` model resources.
- **ChatGPT Subscription:** omit `base_url`; only the official endpoint is supported. OAuth tokens must not be forwarded to gateways.

A working URL in another SDK is not necessarily the right `base_url` for Myaaw. See each guide's endpoint table before configuring a gateway.

### Treat discovery as a catalog, not an inference test

Myaaw's control-plane discovery returns `models`, `source`, and an optional `warning`.

- `source: "live"` means the adapter obtained a usable live catalog, not that every model supports chat/tools/images or has available quota.
- For API-key providers, a failed request or empty list can produce a built-in model with `source: "default"`. An empty list can trigger that fallback without a warning. Check `source`, not merely the HTTP status or presence of a model.
- ChatGPT Subscription has no fabricated default model fallback. It requires the selected account's available model slug; failed discovery is an error.
- CLI discovery and headless onboarding have different behaviors. Headless API-key setup can save an untested model/key combination; an interactive manual entry is also not a health check.

The repository defaults are `gpt-4o` for OpenAI and `models/gemini-3-flash-preview` for Gemini. They are code defaults, not current upstream availability guarantees. The subscription provider has no built-in model default.

## 5. Shared tool, attachment, and voice boundaries

All three chat integrations feed Myaaw's own agent loop. A provider can request local functions, but the enabled tool set, approval policy, OS permissions, and installed dependencies decide whether they execute. Native hosted search, code execution, uploads, or computer-use APIs are not automatically enabled by a provider's advertised capabilities.

Full conversation context and tool results can be sent upstream for each model turn. Ordinary uploaded files are stored locally and described by paths in an `<attached>` block; the model needs permitted tools to read/extract them. Inline images use provider-specific conversion. “Local attachment” does not mean extracted content can never leave the machine.

Voice is configured separately:

- `myaaw voice` uses **Gemini Live** and its own session/model rather than the normal agent tool loop.
- `myaaw voice-classic` uses a chosen chat provider with separately configured STT and TTS. ChatGPT Subscription does not supply speech API credentials, and selecting a Gemini chat provider does not automatically fill speech settings.

See [Gemini's voice section](gemini.md#voice-transcription-and-synthesis), [Voice](../cli/voice.md), [Agent Loop](../agents/agent-loop.md), and [Tool Approvals](../agents/approvals.md).

## 6. Security, privacy, and backups

| Concern | API-key integrations | ChatGPT Subscription |
| :--- | :--- | :--- |
| Secret at rest | API keys in per-agent config and/or environment files | Tokens in `~/.myaaw/credentials/chatgpt`; agent config contains a reference |
| API display | Configured/masked key state | Metadata-only account/connection state; sign-in URL returned only when starting a flow |
| Backup behavior | Agent archives can contain API keys; protect them | Default agent/global backups exclude the host OAuth store; references may still be archived |
| Moving to another host | Securely provision the service key and endpoint | Sign in locally on the new host and update the saved reference |
| Local removal | Deleting a provider does not revoke the service key | Disconnect/delete does not erase a shared credential or revoke the remote grant |

Permissions and masking are not encryption at rest. Protect the OS account, archives, `.env`, logs, and observability content. Gemini's handwritten REST transport puts its key in the URL query, which needs particular care in proxy/access logs. Custom API-key endpoints receive both their Bearer key and the conversation content; configure only trusted destinations.

Provider credentials authenticate **Myaaw to the model service**. A control-plane bearer token authenticates **a client to Myaaw**. They are different secrets and are not interchangeable. Use authenticated/HTTPS management access and never publish real keys, tokens, sensitive sign-in URLs, or complete private transcripts in examples or bug reports.

Each upstream service determines its current pricing, quotas, eligibility, and retention policies. Myaaw's usage counters and cost estimates are not a billing invoice. There is no silent switch from subscription quota to paid API usage.

## Implementation map

| Area | Source |
| :--- | :--- |
| Shared message/event contracts, factories, default models/endpoints | [`internal/provider/provider.go`](../../internal/provider/provider.go), [`events.go`](../../internal/provider/events.go) |
| OpenAI-compatible adapter | [`openai.go`](../../internal/provider/openai.go), [`openai_stream.go`](../../internal/provider/openai_stream.go) |
| Gemini chat and audio | [`gemini.go`](../../internal/provider/gemini.go), [`gemini_live.go`](../../internal/provider/gemini_live.go), [`gemini_transcriber.go`](../../internal/provider/gemini_transcriber.go), [`gemini_synthesizer.go`](../../internal/provider/gemini_synthesizer.go) |
| Subscription inference and streaming | [`chatgpt.go`](../../internal/provider/chatgpt.go), [`chatgpt_stream.go`](../../internal/provider/chatgpt_stream.go) |
| OAuth and credential lifecycle | [`internal/auth/chatgpt/`](../../internal/auth/chatgpt/) |
| Configuration | [`internal/config/config.go`](../../internal/config/config.go) |
| Onboarding presets/auth | [`capabilities.go`](../../internal/onboard/capabilities.go), [`chatgpt.go`](../../internal/onboard/chatgpt.go) |
| Provider management API | [`config.go`](../../internal/services/controlplane/service/config.go), [`models.go`](../../internal/services/controlplane/service/models.go), [`chatgpt.go`](../../internal/services/controlplane/service/chatgpt.go) |

Return to the [documentation index](../README.md).
