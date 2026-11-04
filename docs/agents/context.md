# Agent Context

What the agent knows about a turn, how that context is stored, and how it
interacts with provider prompt caching.

The short version: the **system prompt is static per agent**, and all per-turn
context (`<userinfo>`) and uploaded-file paths (`<attached>`) live in the user
message itself. That keeps the cacheable prefix — system prompt plus the whole
conversation history — byte-stable.

---

## 1. What the agent knows about a turn

| Input | Where it comes from | Reaches the model |
| :--- | :--- | :--- |
| Agent identity, rules, tools, workspace | `BOOTSTRAP.md`, `IDENTITY.md`, `AGENTS.md`, `SOUL.md`, `TOOLS.md`, `USER.md` | yes, system prompt |
| Skills instruction | `skills.GetSkillsInstruction()` | yes, system prompt |
| Platform, room, ids, speaker, time | channel adapter → `<userinfo>` | yes, user message |
| Uploaded file paths | `<attached>` | yes, user message |
| Sender metadata fields | `provider.Message.Sender*` | no, dashboard only |
| Session scope (`tg:…`, `dc:…`) | conversation store | no, routing only |

---

## 2. The system prompt

Built by `agent.SystemPromptBuilder.Build()` (`internal/agent/context.go`) and
rebuilt on every turn. It contains only content that is stable for an agent:

```
BOOTSTRAP → IDENTITY → AGENTS → Agent Home → Runtime → Workspace
→ Current Directory (local channels only) → SOUL → TOOLS → USER
```

then `skills.GetSkillsInstruction()` is appended at the call site.

It deliberately contains **no** user id, **no** chat context, and **no** clock.
Those change per turn or per speaker, and anything that changes before the
conversation history invalidates provider prefix caching for everything after it
(see §7).

`Agent Home`, `Workspace root`, and the skills text depend on the agent, not on
the message, so two turns of the same agent produce a byte-identical system
prompt.

---

## 3. The `<userinfo>` block

Written once, at the top of every user message, by
`agent.RenderUserInfo(msg, now)` (`internal/agent/userinfo.go`) and prepended in
`buildUserMessage` (`internal/services/bot/service/conversation.go`).

```
<userinfo>
Platform: Discord · Chat: Example Server › general (server channel, shared) · ChatID: 222333444555666777 · Speaker: Alice (@alice) · UserID: 111222333444555666 · Session: 1a2b3c4d-… · Current time: 2026-11-04 08:43:12 UTC
</userinfo>

buka foto terbaru di workspace
```

### Fields

| Field | Meaning | Notes |
| :--- | :--- | :--- |
| `Platform` | `Telegram`, `Discord`, `CLI`, `API`, `A2A`, `Voice`, `Sub-agent` | always present |
| `Chat` | room kind, name, audience | always present; CLI/API show `local session` / `API session` |
| `ChatID` | generic label for the platform room id | Telegram `chat.id`; Discord `channel.id` (also a thread id); omitted for a Telegram DM because it equals the sender id |
| `TopicID` | Telegram forum `message_thread_id` | only when `>= 2`; Discord has no separate topic id because a thread **is** a channel |
| `Speaker` | `Name (@handle)` | human identity; omitted for CLI, API, and sub-agent turns |
| `UserID` | `UserID: 111222333444555666` | the id that used to live in the system prompt; always present when known (sender id for Telegram/Discord, authenticated id for API) |
| `Session` | `Session: 0f4e8b3a-…` | conversation id; present even on the first turn of a new chat |
| time | `Current time: 2026-11-04 08:43:12 UTC` | every turn, to the second; server clock, always UTC |

### Examples

Telegram group:

```
Platform: Telegram · Chat: Example Group (group, shared) · ChatID: -1001234567890 · Speaker: Andi (@andi) · UserID: 123456789 · Session: 2b3c4d5e-… · Current time: 2026-11-04 08:45:31 UTC
```

Telegram forum topic:

```
Platform: Telegram · Chat: Example Group › Daily Report (forum topic, shared) · ChatID: -1001234567890 · TopicID: 77 · Speaker: Andi (@andi) · UserID: 123456789 · Session: 2b3c4d5e-… · Current time: 2026-11-04 08:45:31 UTC
```

Telegram DM:

```
Platform: Telegram · Chat: direct message (only this user) · Speaker: Alice (@alice) · UserID: 123456789 · Session: 0a1b2c3d-… · Current time: 2026-11-04 08:43:12 UTC
```

Discord thread:

```
Platform: Discord · Chat: Example Server › bug-report (thread, shared) · ChatID: 222333444555666888 · Speaker: Budi (@budi) · UserID: 111222333444555666 · Session: 1a2b3c4d-… · Current time: 2026-11-04 08:46:07 UTC
```

CLI / API:

```
Platform: CLI · Chat: local session · Session: 0f4e8b3a-1c2d-4e5f-8a9b-0c1d2e3f4a5b · Current time: 2026-11-04 08:43:12 UTC
```

```
Platform: API · Chat: API session · UserID: 111222333444555666 · Session: 0f4e8b3a-1c2d-4e5f-8a9b-0c1d2e3f4a5b · Current time: 2026-11-04 08:43:12 UTC
```

On the API path the user id is not trusted from the request body: the control
plane overwrites it with the authenticated token's user id before proxying the
request to the agent (`controlplane/service/proxy.go`, `injectUserID`).

The `Session` id is resolved or created **before** the user message is built
(`buildConversationMessages`), so the very first turn of a new chat already
carries it. `Session` is skipped when the value is the placeholder `NEW`.

The clock comes from the **server** and is rendered in UTC with an explicit
`UTC` suffix, so the value is unambiguous no matter what timezone the host runs
in. Myaaw has no per-user timezone setting; cron jobs keep their own `tz` field,
and the model can convert UTC to a local time when a person asks for it.

### Rules

- One inner line, with **no truncation** of ids, the session, or the clock.
  Chat/topic/speaker names are only guarded at the platforms' own 128-character
  limit, so a real name is never shortened.
- Names are **untrusted input**: control characters and newlines collapse to
  spaces, and `<`/`>` are removed so a channel name cannot close the tag early
  or inject a second block.
- A missing name never becomes `unknown`: the line falls back to
  `Telegram group`, `Discord channel`, or `topic <id>`.
- `scope`, `is_forum`, message ids, member counts, and permission state are not
  included.
- Ids are reference data. They are not secrets (no tokens, no credentials), and
  routing still uses `RawMeta`, not the prompt.

### Why not the system prompt?

Because the block changes per turn (time, and speaker in a group). In the system
prompt it would shift the first differing byte **before the conversation
history**, and the provider would have to reprocess the whole transcript on every
turn. In the user message it sits after the history, so only the new block and
the new message are uncached.

---

## 4. The `<attached>` block

When a turn carries uploads, `buildUserMessage` prepends a block of absolute
paths so the model can read the files with its tools:

```
<attached>
[read them with the filesystem or bash tools]
/user/uploads/conv/msg/a.pdf
</attached>
```

Order in a message with both blocks:

```
<userinfo>...</userinfo>

<attached>...</attached>

the text the person typed
```

---

## 5. Where the tags are hidden

The tags are model-only. Each surface strips them before rendering:

| Surface | Implementation |
| :--- | :--- |
| Chat bubbles and the edit box | `withoutContext()` in `frontend/src/routes/app/chat/[[conversation]]/+page.svelte` |
| Auto-generated title | `provider.StripContextBlocks` in `conversation.go` |
| Turn rail labels (control plane) | `provider.StripContextBlocks` in `controlplane/service/conversations.go` |
| CLI resumed transcript | `provider.StripContextBlocks` in `cmd/myaaw/chat.go` |
| Observability trace | **not** stripped: the trace shows the real prompt, including the block |

`provider.StripContextBlocks` removes `<userinfo>` and `<attached>` together;
`SplitContextBlocks`/`JoinContextBlocks` extract and restore the leading blocks.

---

## 6. Editing a message

The dashboard edit box contains only the text the person typed. On the server,
`rewindToUserMessage` replaces **only** that text and preserves everything else:

- the leading `<userinfo>` and `<attached>` blocks, in their original order;
- image parts for providers that store content as `[]ContentItem`
  (OpenAI/Groq), and `GeminiPart.InlineData` for Gemini.

Regeneration keeps the content untouched (`replacement == ""`), so it reuses the
same prompt prefix.

Messages written before this feature have no blocks; editing them does not
invent one.

---

## 7. Prompt caching

All supported providers cache by **prefix matching**: the first differing byte
invalidates everything after it. Myaaw uses OpenAI-compatible providers
(OpenAI, DeepSeek, Groq), Gemini, and Ollama; there is no Anthropic provider, so
explicit `cache_control` breakpoints do not apply.

| Provider | Mechanism | Minimum | Reported as |
| :--- | :--- | :--- | :--- |
| OpenAI | automatic prefix caching; stable first, dynamic last | ~1,024 visible tokens (model-dependent) | `cached_tokens`, rounded to 128-token blocks |
| DeepSeek | automatic; a later request must fully match a persisted prefix unit, and common prefixes are persisted too | no fixed minimum documented | `prompt_cache_hit_tokens` / `prompt_cache_miss_tokens` |
| Gemini | implicit caching | 2,048 tokens (2.5) / 4,096 (newer) | `total_cached_tokens` |

Myaaw normalizes all three into `provider.Usage.CachedInputTokens`; the
observability UI shows the remainder as `uncached_input_tokens`.

### Why the layout is cache-safe

```
[system prompt: agent files]          stable per agent
[history: user1, assistant1, …]       append-only
[user: <userinfo> + text]             only this part is new
```

- The system prompt is identical between turns and between members of the same
  agent.
- `<userinfo>` is written once and stored with the message; it is never rewritten
  on later turns.
- A different group member changes only the new block (`Speaker`, time), not the
  history before it.
- Tool schemas are sorted by name (`tools.GetFactories`), so tool changes are
  deterministic.

### What invalidates the cache

- Editing a message, regenerating, or compaction (the transcript is rewritten).
- `/new` (a different conversation row).
- Changing the model, tool list, or any agent file (system prompt changes).
- Prompts shorter than the provider's minimum, which simply do not cache.

### Verifying

Send two messages in the same chat and compare `cached_input_tokens` on the
second run in the observability view. The second turn should report cached tokens
roughly equal to the system prompt plus the first turn.

### Sources

- **OpenAI — Prompt caching**
  <https://platform.openai.com/docs/guides/prompt-caching> — prefix matching,
  keep stable instructions first and move timestamps/user-specific content to
  the end or into later messages, minimum ~1,024 visible tokens, `cached_tokens`
  rounded to 128-token blocks, and the warning that extending an existing
  message can prevent reuse of its prefix.
- **DeepSeek — Context caching**
  <https://api-docs.deepseek.com/guides/kv_cache> — overlapping prefixes are
  served from cache, a later request must fully match a persisted prefix unit,
  common prefixes across requests are persisted too, and the hit/miss counts are
  reported as `prompt_cache_hit_tokens` / `prompt_cache_miss_tokens`.
- **Gemini — Context caching**
  <https://ai.google.dev/gemini-api/docs/caching> — implicit caching, minimum
  2,048 tokens (2.5) / 4,096 tokens (newer models), put large common content at
  the beginning, and report hits in `total_cached_tokens`.

---

## 8. Is Myaaw cache-safe?

Yes, with these caveats:

- **The system prompt no longer contains per-turn data.** This was the main
  blocker historically: a per-second `Current Time` and a per-user `User ID`
  meant the history could never be reused.
- **The `<userinfo>` block costs ~35 tokens per user turn**, stored once. That is
  the price of the context; it does not grow with the conversation.
- **Per-user caches do not share.** Providers key caches per account/project, so
  different users never share hits; within a group, different speakers still hit
  the history because only the new block differs.
- **Short chats may not cache at all** below the provider minimum.
- **Edits, regeneration, compaction, and `/new` invalidate by design.**
- **Ollama has no prompt cache** to reuse.

---

## 9. Privacy and security

- Channel and user names are untrusted: sanitized to one line, capped, and
  stripped of `<`/`>` so they cannot break the tag or inject instructions.
- Ids are reference data, never credentials; no token or secret is placed in
  `<userinfo>`.
- Sender metadata in `provider.Message` stays dashboard-only; the prompt gets
  the speaker through `<userinfo>`.
- Observability records the prompt as sent, including the block. Treat trace
  exports accordingly.

---

## 10. Where this lives in the code

| Concern | Location |
| :--- | :--- |
| System prompt | `internal/agent/context.go` |
| `<userinfo>` renderer | `internal/agent/userinfo.go` |
| Extract/strip blocks | `internal/provider/attachments.go` |
| Prepend block, edit preservation | `internal/services/bot/service/conversation.go` |
| Adapters fill chat type/ids | `internal/channel/telegram/adapter.go`, `internal/channel/discord/adapter.go` |
| Dashboard stripping | `frontend/src/routes/app/chat/[[conversation]]/+page.svelte` |
| CLI stripping | `cmd/myaaw/chat.go` |
| Cache measurement | `provider.Usage.CachedInputTokens`, observability |
