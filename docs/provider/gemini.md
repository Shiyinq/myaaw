# Gemini

Myaaw's `gemini` provider uses the native Gemini API for text chat, streaming, function calls, and inline images. Gemini Live, speech transcription, and speech synthesis are separate integrations with different models and configuration paths.

This guide describes the current implementation, not every capability offered by Google's APIs. Model names below are repository defaults or examples, **not a guarantee that Google still offers them or that your project can use them**.

Related: [Provider index](README.md) · [OpenAI-compatible](openai-compatible.md) · [ChatGPT Subscription](chatgpt-subscription.md) · [Provider CLI](../cli/provider.md) · [Onboarding](../cli/onboard.md) · [Voice](../cli/voice.md) · [Control Plane API](../api/control-plane.md).

## Contents

- [Choose the integration](#choose-the-integration)
- [Authentication, endpoints, and model names](#authentication-endpoints-and-model-names)
- [Setup](#setup)
- [Model defaults and discovery limitations](#model-defaults-and-discovery-limitations)
- [Chat transport and capabilities](#chat-transport-and-capabilities)
- [Voice, transcription, and synthesis](#voice-transcription-and-synthesis)
- [Quotas, billing, privacy, and security](#quotas-billing-privacy-and-security)
- [Troubleshooting](#troubleshooting)
- [Implementation sources](#implementation-sources)

## Choose the integration

| Use case | Myaaw path | Important boundary |
| :--- | :--- | :--- |
| Native Gemini chat and agent tools | Provider type `gemini` | Gemini REST request/response format, not OpenAI Chat Completions. |
| An OpenAI-compatible service or gateway | Provider type `openai` | Use the [OpenAI-compatible guide](openai-compatible.md); setting a Gemini URL does not change its wire protocol. |
| ChatGPT plan-backed access | Provider type `openai_chatgpt` | Separate [ChatGPT Subscription](chatgpt-subscription.md) authentication and limits; not a Gemini credential. |
| Real-time microphone conversation | `myaaw voice` | Requires an active Gemini LLM provider; uses a separate Live session without the agent tool loop. |
| Turn-based speech with tools | `myaaw voice-classic` | Any configured LLM provider plus separately configured transcriber and synthesizer. |

A Gemini app subscription is not an API key or a promise of Gemini API quota. Obtain a key for the Gemini API through [Google AI Studio](https://aistudio.google.com/apikey), and check the project's API access, billing, region, and model eligibility. This integration selects the Gemini API backend, not Vertex AI; it does not configure service-account/ADC authentication or Google browser OAuth.

## Authentication, endpoints, and model names

### Native REST chat and discovery

The default base URL is:

```text
https://generativelanguage.googleapis.com
```

Use the origin, without `/v1beta`, `/models`, a method suffix, or a trailing slash. The adapter appends its own paths. A custom base URL must implement the native Gemini API at those paths; an OpenAI-compatible gateway is not sufficient.

The actual REST requests in `gemini.go` are:

```text
GET  <base_url>/v1beta/models?key=<api_key>
POST <base_url>/v1beta/<model>:generateContent?key=<api_key>
POST <base_url>/v1beta/<model>:streamGenerateContent?key=<api_key>
```

These requests set `Content-Type: application/json` and put the key in the **`key` query parameter**. This adapter does **not** send an `Authorization: Bearer` header or an `x-goog-api-key` header. Google's header-based examples are not an exact description of Myaaw's handwritten REST transport. Gateways that only accept header authentication will need compatible routing/authentication handling; there is no provider field for arbitrary headers.

Store the full resource name returned by discovery, for example:

```text
models/gemini-2.5-flash
```

**The REST request builder does not add `models/` to a bare model ID.** `DefaultModel()` returns the supplied model or the configured default unchanged. The adapter strips a leading `models/` only for capability checks and discovery filtering, then returns the original resource names. A bare `gemini-2.5-flash` therefore produces `/v1beta/gemini-2.5-flash:generateContent`, rather than the expected `/v1beta/models/gemini-2.5-flash:generateContent`.

### SDK-based audio paths

Live, transcription, and synthesis create their own `google.golang.org/genai` clients with `APIKey` and `BackendGeminiAPI`. They do not use the handwritten REST adapter or inherit the LLM provider's `base_url` or `default_model`.

With the currently pinned SDK (`v1.47.0`) and its default endpoint/version:

- Transcription and synthesis use Gemini `v1beta` `generateContent` over HTTPS, with `x-goog-api-key` header authentication. The SDK handles model resource names for these calls.
- Live uses a WebSocket connection to `wss://generativelanguage.googleapis.com/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent`, also with `x-goog-api-key` in the handshake.
- Myaaw does not pass explicit SDK HTTP endpoint/version options. A custom chat base URL does not redirect these audio clients; SDK-level environment behavior is separate from per-agent provider configuration.

Do not apply the REST query-key description to these SDK paths, or assume all Gemini traffic follows a configured chat gateway.

## Setup

### Interactive CLI

For a new agent:

```bash
myaaw onboard
```

Choose **Gemini**, use the default public base URL unless you have a native Gemini gateway, enter the API key, and select a discovered model. Review and save the configuration. See [Onboarding](../cli/onboard.md) for agent creation, channels, and installation behavior.

For an existing agent:

```bash
myaaw --agent personal provider create
# Type: Gemini
# Custom name: gemini-personal
# Base URL: https://generativelanguage.googleapis.com
# API key: enter at the hidden prompt

myaaw --agent personal provider default gemini-personal
myaaw --agent personal provider list
myaaw --agent personal provider set-model gemini-personal
```

The first provider becomes the default automatically; adding another provider does not automatically switch the default. The name `gemini-personal` is a per-agent integration ID, while `gemini` is its wire type.

On discovery failure or an empty result, the provider create/set-model commands offer manual model entry. Use the full `models/...` resource name. Interactive onboarding also offers a manual option alongside a successful discovery list. Manual entry is not validation of model access.

A small, billable smoke test after setup is:

```bash
myaaw --agent personal chat -m "Reply with a short greeting."
```

This command is an example for you to run; this guide does not claim a live API test was performed.

### Headless onboarding

Supply `MYAAW_API_KEY` through your CI secret store or a securely prepared environment, then run:

```bash
myaaw onboard --yes --agent personal \
  --provider gemini --provider-name gemini-personal \
  --base-url https://generativelanguage.googleapis.com \
  --model models/gemini-2.5-flash \
  --channel skip --json
```

Replace the example model with one confirmed for your project. The exact key flag is `--api-key`, but prefer the environment variable so the key is not a literal shell-history entry or a command-line argument. `MYAAW_API_KEY` is onboarding input, not a replacement for stored provider configuration during every chat request.

For Gemini, headless setup requires a key from `--api-key`, `MYAAW_API_KEY`, or an applicable saved provider. If no model is supplied, it can reuse the saved model or the built-in default. It does **not** perform live Gemini model discovery or an inference check before saving. `--json` implies non-interactive mode and returns the setup summary, not a model-health result.

Reconfiguring an initialized agent requires `--force`; do not add it casually to a fresh-install script. `--channel skip` leaves existing channel settings untouched rather than disabling them. See the [headless onboarding rules](../cli/onboard.md#headless-mode-ci-scripts).

### Dashboard

1. Select the target agent and open **Providers** (`/app/providers`), or choose Gemini in the create-agent wizard.
2. Set the type to `gemini`, choose an integration name, enter the public base URL and API key, and discover models.
3. Inspect any discovery warning and select the full `models/...` resource name. The Providers page also has a manual model field when no discovery list is displayed.
4. Save/create the provider, then use its default/star action if it should become the agent's active provider.

When editing a saved provider, leaving the API-key field blank preserves the stored key. Discovery with a blank key while editing uses the saved provider's credentials and endpoint; it does not test unsaved endpoint changes. To test a changed endpoint before saving, supply the intended key explicitly.

The dashboard talks to Myaaw's control plane, which makes the Google request server-side. Keys are write-only in provider API responses, represented by configured/masked state rather than the plaintext key. That masking does not encrypt the underlying configuration file.

### Per-agent JSON

Merge the following fields into `~/.myaaw/personal/config/config.json`; do not replace an existing file and lose its channels, tools, or other settings. Replace the key placeholder locally, and use a model available to your project:

```json
{
  "default_provider": "gemini-personal",
  "providers": {
    "gemini-personal": {
      "type": "gemini",
      "base_url": "https://generativelanguage.googleapis.com",
      "api_key": "REPLACE_WITH_YOUR_GEMINI_API_KEY",
      "default_model": "models/gemini-2.5-flash"
    }
  }
}
```

Keep the explicit origin and full model name in hand-edited configuration. Do not put a ChatGPT `credential_id` or OAuth token in this Gemini entry. Each agent has its own provider map and default; changing `personal` does not configure another agent.

## Model defaults and discovery limitations

The current source contains these independent defaults:

| Path | Repository default | Selection behavior |
| :--- | :--- | :--- |
| LLM chat | `models/gemini-3-flash-preview` | Used when the relevant provider setup/factory falls back to its built-in model. |
| Transcriber | `models/gemini-2.5-flash` | Chosen by the transcriber factory. |
| Synthesizer | `gemini-2.5-flash-preview-tts` | Chosen by the synthesizer factory; voice defaults to `Kore`. |
| Live | `gemini-2.5-flash-native-audio-preview-12-2025` | Hard-coded `LiveModel`; voice is `Aoede`. |

These are code defaults, not a current Google model catalog. Preview names can be retired, unavailable in a region, or inaccessible to a particular key. Changing the chat `default_model` does not update the other three paths. The transcriber/synthesizer config blocks have no model field, and `voice` has no model/voice override flag.

### What discovery actually returns

`GeminiProvider.Models()` performs one `GET /v1beta/models` with a 15-second timeout, then filters the returned models:

- The model must advertise `generateContent` in `supportedGenerationMethods`.
- After removing a leading `models/` for the check, it must contain `flash` or start with `gemini-2.`, `gemini-3.`, or `gemini-3-`.
- Names containing `-tts`, `-image`, `-generation`, `native-audio`, `robotics`, `computer-use`, `aqa`, `embedding`, `imagen`, or `veo` are excluded.
- Names starting with `deep-research` or `gemma-` are excluded.
- The original resource name, including `models/`, is returned to the picker.

**Pagination is not implemented.** The response type only reads `models`; it does not read/follow `nextPageToken`, and the request does not set `pageSize` or `pageToken`. A model missing from the list may be on another page or excluded by the local filter. Discovery does not independently verify streaming, image input, tool support, quotas, billing, or a successful generation request.

Tool/system-instruction/thinking support is determined separately by local name heuristics, not by the model listing's capability metadata. A newly released model can appear in discovery yet not receive tool declarations or thinking configuration from this adapter.

### Control-plane fallback is not successful discovery

The dashboard's model endpoints are:

| Method | Path | Credentials |
| :--- | :--- | :--- |
| `POST` | `/providers/models` | Request body with `type`, optional `base_url`, and `api_key`; usable before saving a provider. |
| `GET` | `/agents/:name/providers/:id/models` | Uses the saved provider's key without returning it to the caller. |

For Gemini, these return a view with `models`, `source`, and optional `warning`. A failed live request **or an empty filtered list** falls back to the built-in LLM default with `source: "default"`. A request error populates `warning`; an empty successful list can fall back **without a warning**.

Check `source`, not just the presence of a model or an HTTP success response. A fallback default proves neither authentication nor model availability. This is an API/UI convenience, not an automatic runtime failover to a working model. Direct CLI discovery instead reports an error/manual entry path. ChatGPT Subscription has different behavior and does not use this API-key fallback.

See [Control Plane API](../api/control-plane.md#10-providers) for authenticated requests. Prefer saved-provider discovery when possible so the Gemini key stays server-side; a control-plane bearer token authenticates to Myaaw, not to Google.

## Chat transport and capabilities

### Streaming and response handling

- Non-streaming chat uses `:generateContent`; streaming uses `:streamGenerateContent`.
- The upstream streaming parser expects a **JSON array**, incrementally decoded. It does not request `alt=sse` or parse Gemini SSE. A gateway returning SSE instead will fail with an expected-JSON-array error.
- Myaaw exposes text deltas, thinking deltas, usage, and a final assistant message. Function calls are collected into the final message rather than emitted as incremental tool-argument deltas.
- Only the first candidate is processed. Inline generated media is not converted into chat output; this is not a Gemini image-generation or generic audio/video-output adapter.
- `MAX_TOKENS` maps to a length stop. Safety/recitation/blocklist/prohibited-content/SPII finish reasons map to an error. A response with no candidates can also fail the non-streaming path.
- Reported prompt, completion, total, thought, cached-input, and modality token counts are mapped when present. These counters are not a billing invoice or an assurance of a cache discount.

Upstream JSON-array streaming is distinct from Myaaw's downstream HTTP/SSE chat API. Using the dashboard's SSE stream does not mean Myaaw requests Gemini SSE upstream.

The REST non-streaming request timeout is 120 seconds. Streaming uses the configured `LLM_STREAM_TIMEOUT_SECONDS` budget (default 180 seconds) and a 60-second idle watchdog. Shared retry handling covers transient transport failures and selected HTTP statuses, including 429 and common 5xx errors. Streaming is not retried after text/thinking has already reached the caller, to avoid duplicate output. These policies should not be assumed to apply identically to model discovery, Live, or the SDK speech clients.

### Tools and system instructions

Myaaw translates its function tool definitions into Gemini `function_declarations` with function-calling mode `AUTO`, then sends tool results as `functionResponse` parts matched by function name. Assistant messages become Gemini `model` content; tool messages become `user` content, and consecutive contents with the same role are bundled.

Tool declarations are sent only for recognized model-name prefixes: the Gemini Flash aliases and the configured 2.0 Flash/Pro, 2.5 Flash/Pro, 3 Flash/Pro, and 3.1 Flash/Pro families. Names containing `-tts`, `-image-generation`, `-native-audio`, or `robotics` are excluded from that check. This is a heuristic, not a guarantee that every matching model accepts tools.

The initial system message becomes `systemInstruction` only when the adapter considers the model supported. Otherwise, when a first content part exists, the system text is prefixed there as `System: ...`. Gemma and older Gemini names are among the unsupported cases. Later system-role messages are mapped into user content.

This path integrates Myaaw's agent tools; it does not configure Google Search grounding, URL context, Google's code execution, or every native Gemini tool type. Tool execution still depends on Myaaw's enabled tools and approval policy.

### Thoughts and thought signatures

For its explicit thinking-model allowlist, the adapter sends `thinkingConfig.includeThoughts: true`. The current list includes the Flash/Flash Lite latest aliases, Gemini 2.5 Flash/Pro/Flash Lite, the `gemini-2.5-flash-lite-preview-09-2025` variant, Gemini 3 Pro/Flash preview, and Gemini 3.1 Pro preview. It does not infer thinking support for every future family member, and it does not expose a per-provider thinking-level/budget control.

Response text marked with Gemini's `thought` flag is kept separate from answer text; formatting heuristics are not used to guess reasoning. This is provider-returned thought content, not a promise of access to hidden chain-of-thought.

For function calls, the adapter saves `thoughtSignature` in the tool call's `thought_signature` field and echoes it back with that call in subsequent Gemini requests. Preserve those opaque signatures when persisting or copying tool-call history; rebuilding a turn from visible text alone can break continuation. They are not displayable reasoning, and this handling does not imply support for every possible signed Gemini part.

### Images and attachments

Two image representations are supported by the chat converter:

- Base64 image bytes in `Message.Images`, mapped to `inlineData` for user-role content. The legacy MIME sniff recognizes PNG, WebP, and JPEG prefixes and otherwise defaults to JPEG; it is not comprehensive format validation.
- `image_url` content items containing `data:<mime>;base64,<payload>`, mapped to `inlineData`. This also supports image-bearing tool results such as screenshots.

**Remote HTTP(S) image URLs in content items are skipped**, with a log message. The adapter neither fetches them nor passes them as Gemini file references. A message can succeed while omitting such an image. Use a supported local/inline-image path rather than expecting an arbitrary remote URL to be analyzed.

Ordinary file attachments follow a different route: Myaaw stores the file locally, retains upload metadata, and adds its absolute path to an `<attached>` block with instructions to read it using filesystem/bash tools. The Gemini converter does not automatically upload these files to the Gemini Files API or turn a PDF/document path into native `fileData`. Separate inline images can still travel with the message.

Consequences:

- Reading an attached document depends on available, permitted tools and suitable local parsing; a model does not gain direct access to the host's filesystem from the path alone.
- Tool results can subsequently send extracted content to Google. “Stored locally” does not mean the file's contents can never leave the machine.
- Do not assume native PDF, arbitrary file, audio-file, video-file, or Gemini file-URI support in ordinary chat just because the upstream API offers it. Audio and live frames use the separate paths below.

## Voice, transcription, and synthesis

See [Voice](../cli/voice.md) for native audio dependencies, build requirements, and OS microphone/camera/screen permissions.

### Real-time Live mode

```bash
myaaw --agent personal provider default gemini-personal
myaaw --agent personal voice
myaaw --agent personal voice --video screen,camera --video-interval 5
```

`voice` checks that the active LLM type is Gemini and passes that provider's API key to `NewGeminiLiveSession`. It uses the hard-coded Live model and `Aoede` voice, not the chat model or provider endpoint. There is no automatic substitution with a discovered chat model if the Live model is unavailable.

The session sends 16 kHz, 16-bit mono PCM microphone audio and optional periodic JPEG frames. It receives 24 kHz, 16-bit mono PCM audio plus text, input/output transcriptions, turn-complete, and interruption events. Screen/camera frames are sampled captures, not a generic uploaded-video pipeline.

Live uses its own conversational system instruction and does not run the normal agent loop or register agent tools. Choosing a tool-capable chat model does not add tools to Live. This is a local CLI audio session, not a documented browser/control-plane voice transport.

### Classic mode: separate speech configuration

`voice-classic` runs speech-to-text → normal agent loop/tools → text-to-speech. Configuring or selecting a Gemini LLM provider does **not** populate its transcriber or synthesizer settings, even if you intend to reuse the same key.

Merge these separate blocks into the active agent's `config/config.json`:

```json
{
  "transcriber": {
    "provider": "gemini",
    "api_key": "REPLACE_WITH_YOUR_GEMINI_TRANSCRIBER_KEY"
  },
  "synthesizer": {
    "provider": "gemini",
    "api_key": "REPLACE_WITH_YOUR_GEMINI_SYNTHESIZER_KEY"
  }
}
```

The `provider` values here are factory types (`gemini`), not an integration ID such as `gemini-personal`. You may deliberately supply the same Google key in all blocks, but Myaaw does not copy it automatically. The JSON speech blocks override the corresponding `.env` settings when populated; a nonempty JSON provider selects the type, and a nonempty JSON key overrides the environment key.

The supported `.env` alternative is:

```dotenv
# ~/.myaaw/personal/.env — replace these placeholders locally
TRANSCRIBER_PROVIDER_NAME=gemini
TRANSCRIBER_API_KEY=REPLACE_WITH_YOUR_GEMINI_TRANSCRIBER_KEY
SYNTHESIZER_PROVIDER_NAME=gemini
SYNTHESIZER_API_KEY=REPLACE_WITH_YOUR_GEMINI_SYNTHESIZER_KEY
```

Then run:

```bash
myaaw --agent personal voice-classic
myaaw --agent personal voice-classic --video camera --video-interval 5
```

The factory chooses the transcription and TTS defaults listed earlier. Transcription sends inline audio with a request to transcribe it exactly and joins returned text parts. TTS requests `AUDIO` output with the `Kore` voice and returns the first nonempty inline audio part and its MIME type. Classic playback assumes 24 kHz audio rather than negotiating every returned format.

**Current format caveat:** the Gemini transcriber labels its bytes `audio/ogg` unconditionally, while `voice-classic` wraps microphone PCM in a WAV header before passing it to the transcriber. The code does not convert that WAV to Ogg or derive a matching MIME type. Do not assume robust format negotiation; if transcription fails, inspect this mismatch as well as key/model access. This guide documents the limitation rather than claiming that path was tested successfully.

## Quotas, billing, privacy, and security

- **Check Google's current terms.** Model access, free/paid tier behavior, project quotas, preview availability, regional restrictions, and prices can change. Consult [Gemini models](https://ai.google.dev/gemini-api/docs/models), [rate limits](https://ai.google.dev/gemini-api/docs/rate-limits), [pricing](https://ai.google.dev/gemini-api/docs/pricing), and [terms](https://ai.google.dev/gemini-api/terms). Do not assume a consumer subscription covers API requests.
- **A key does not create separate quota.** Requests using the same project can compete for limits across agents and services. Classic speech can incur transcription, LLM, tool-follow-up, and TTS usage; Live has its own model/usage behavior. Local retries do not increase Google's quota or repair exhausted billing.
- **Treat REST URLs as secrets.** The handwritten adapter puts the API key in the query string. HTTPS protects transport to the endpoint, but reverse proxies, access logs, tracing, and copied errors can expose the URL. Redact query keys before sharing diagnostics. Avoid logging full request URLs and avoid untrusted gateways.
- **Protect local credentials.** Provider and speech keys are stored in per-agent configuration or `.env`; masking in the dashboard/config output is not encryption at rest. Restrict file access, keep secrets out of Git and shared backups, rotate exposed keys, and apply appropriate Google API restrictions.
- **Understand what leaves the host.** Chat history, system/context text, inline images, and tool outputs can be sent to the chosen endpoint. Live sends microphone audio continuously while the session runs and sends screen/camera frames when enabled. Speech clients send audio or response text directly through their SDK clients, independently of the chat gateway.
- **Review local artifacts too.** Voice commands write transcripts and operational logs under `~/.myaaw/<agent>/logs/`. `--save-frames` adds captured frames; `voice-classic --save-audio` adds per-turn audio. These optional recordings are off by default, but transcripts are not controlled by those flags. Limit permissions and retention and obtain consent before capturing others.
- **Protect the management surface.** Use authenticated control-plane access, HTTPS for remote use, and trusted endpoint configuration. Do not confuse a masked API response with protection against an attacker who can read the host's files or change the provider URL.

## Troubleshooting

| Symptom | What to check |
| :--- | :--- |
| 400/401/403, invalid key, or permission denied | Verify the configured per-agent key, Gemini API access, key restrictions, billing/region eligibility, and endpoint. Native REST uses a query key; SDK speech/Live uses a header. A ChatGPT/Gemini app sign-in is not a replacement. |
| 404 or unsupported model/method | Use a full `models/...` name for chat and an origin-only base URL. Confirm model availability and the required generation method. Defaults and preview names can age out. |
| Dashboard shows a model despite a failed connection | Inspect discovery `source` and `warning`. `source: "default"` is a fallback, not proof that the key/model works; an empty list can trigger it without a warning. |
| Expected model missing from discovery | Check the first-page-only limitation and name/method filters. Use a confirmed resource name manually rather than interpreting omission as definitive lack of access. |
| Chat works but tools/thoughts do not | Capability checks are separate, hard-coded name heuristics. Confirm the model family is recognized and tools are enabled/approved. Not all discovered models receive those features. |
| Tool continuation fails after restoring history | Preserve function-call names, arguments, tool responses, and opaque thought signatures, not just visible answer text. |
| Image ignored | Remote image URLs are skipped. Check inline/base64 representation and MIME type; ordinary attachment paths are tool-readable files, not native uploads. |
| Streaming says it expected a JSON array | Ensure the gateway returns Gemini's JSON-array streaming format, not OpenAI SSE or Gemini `alt=sse`. |
| 429/resource exhausted, 5xx, timeout, or partial stream | Check Google project limits, current availability, network/proxy buffering, and the streaming timeout/idle budget. Bounded retries do not guarantee recovery and do not replay after text/thinking output. |
| `voice` rejects the provider or cannot connect | Select a Gemini default for the same agent; check its key, SDK/WebSocket reachability, and access to the hard-coded Live model. Changing the chat model/base URL does not change Live. |
| Classic voice says speech is not configured | Populate the separate `transcriber`/`synthesizer` blocks or environment settings; LLM provider setup does not do this automatically. |
| Classic STT/TTS fails while text chat works | Check the separate keys, speech-model availability, audio format/MIME mismatch, and native device permissions. A working chat model does not prove speech support. |

## Implementation sources

These project-relative links are the basis for this guide; defaults and behavior should be rechecked when the implementation or SDK changes.

- [Native Gemini requests, discovery, model checks, streaming, images, and signatures](../../internal/provider/gemini.go)
- [Provider factories, default endpoints/models, and speech factories](../../internal/provider/provider.go)
- [Gemini Live session](../../internal/provider/gemini_live.go)
- [Gemini transcriber](../../internal/provider/gemini_transcriber.go)
- [Gemini synthesizer](../../internal/provider/gemini_synthesizer.go)
- [Attachment/context block helpers](../../internal/provider/attachments.go)
- [Incoming-message attachments and image handling](../../internal/services/bot/service/conversation.go)
- [CLI provider creation, switching, and model selection](../../cmd/myaaw/provider.go)
- [CLI onboarding flags and model selection](../../cmd/myaaw/onboard.go)
- [Onboarding merge/persistence](../../internal/onboard/onboard.go) and [provider presets](../../internal/onboard/capabilities.go)
- [Live voice command](../../cmd/myaaw/voice.go) and [classic voice command/WAV conversion](../../cmd/myaaw/voice_classic.go)
- [Per-agent provider and speech configuration loading](../../internal/config/config.go)
- [Control-plane model discovery/fallback](../../internal/services/controlplane/service/models.go) and [provider persistence/secret views](../../internal/services/controlplane/service/config.go)
- [Dashboard provider management](../../webui/src/routes/app/providers/+page.svelte) and [agent setup](../../webui/src/routes/app/new/+page.svelte)
- [REST retry policy](../../internal/provider/retry.go) and [stream retry/idle policy](../../internal/provider/stream_retry.go)
- [Pinned SDK dependency](../../go.mod)
