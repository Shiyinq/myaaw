# Agent Attachments

How an agent hands files back to the user: the delivery directive, how each
surface presents a file, what is validated, and where the metadata is stored.

This document is about **agent → user** files. User → agent uploads are a
different path and are covered in [Agent Context](context.md).

---

## 1. Two directions

| Direction | Input | How it travels | Documented here |
| :--- | :--- | :--- | :--- |
| User → agent | Attach a file in the composer or messaging platform | Stored on disk, paths reach the model inside `<attached>` | no |
| Agent → user | A file the run created, generated, downloaded, or exported | `@attach(...)` in the final answer | yes |

The rest of this page describes the second one.

---

## 2. Delivery contract

The model references a file in its **final answer** with one directive per file:

```
@attach(/absolute/path/to/file)
@attach("/path with spaces/report.pdf")   # quoted
@attach('single-quoted/path.png')         # quoted
```

Two shorthand forms are accepted as well:

```
@/absolute/path/to/file.md
@~/home/path/clip.mp4
```

Rules:

- Paths must be **absolute** (or start with `~`). Relative paths are ignored.
- A directive is only read from the **final answer**, not from intermediate
  text or tool output.
- The same file is attached once even if it is referenced repeatedly.
- `@attach(...)` inside a fenced code block is documentation, not delivery, and
  is left untouched.
- The model is told never to base64-encode a file or paste its contents.

The system prompt instruction is a single static constant
(`deliveryInstruction` in `internal/agent/context.go`), identical for every
channel, so switching surfaces does not change the cached prompt prefix.

### What to deliver

- Files the user asked for: created, generated, downloaded, exported, or
  modified during the turn.
- Documents, images, audio/video, archives, reports, and project outputs.
- For a project: the relevant source files or the packaged output, **not**
  dependencies or build caches.

### What not to deliver

- Temporary helpers, debug scripts, probes, scratch files.
- Logs, caches, dependency trees, and build intermediates.
- Anything outside the agent's roots (see §4).

These exclusions are stated in the prompt instruction, because a model that
creates a debug script should not hand it to the user as the deliverable.

---

## 3. Per-surface behavior

The same directive produces a different presentation on each surface.

| Surface | Text | Attachment |
| :--- | :--- | :--- |
| API / WebUI (`api`) | Directive is kept; the WebUI hides `@attach(`/`)` (and the `@` of `@/path`) and shows only the path | Virtual path (`/home/...`, `/user/...`); streamed by the browser through `/fs/raw` |
| Telegram / Discord | Directive is replaced by the plain absolute path | Bytes read into memory and uploaded as a native attachment |
| CLI / TUI | Directive is replaced by the plain absolute path | None |
| A2A / other | Directive is replaced by the plain absolute path | None |

Why the WebUI keeps the directive in the text: the transcript stores exactly
what the model wrote, and the frontend decides how to present it. This also
means the model sees its own directive when the transcript is replayed.

`@/path` and `@~/path` are recognized on every surface, for every extension
(including `.md`). Bare paths without `@` are only auto-promoted for media
(§5).

---

## 4. Validation and security

A reference is accepted only if it resolves to a readable **file** inside one of
the agent's roots:

| Virtual root | Absolute directory |
| :--- | :--- |
| `/home` | `<agent>/home` |
| `/user` | `<agent>/user` |
| `/skills` | `<agent>/skills` |
| `/jobs` | `<agent>/jobs` |

Checks applied per reference:

- **Root containment** — the path (and its symlink-resolved target) must stay
  inside one of the roots. Symlink escapes are rejected.
- **Regular file** — directories are not attachments (a project preview is a
  separate, planned feature).
- **Hidden segments** — any path segment starting with `.` is rejected
  (`.env`, `.git`, `.ssh`, …). The install root itself (`~/.myaaw`) is not
  affected because only the part inside the root is checked.
- **Credential names** — `auth.json`, `credentials.json`, `id_rsa`,
  `id_ed25519`, `id_ecdsa` are rejected.
- **Code fences** — a directive inside a fenced block is ignored.
- **Trailing punctuation** — a path followed by `.`, `,`, `;`, `:`, `!`, or `?`
  is retried without the punctuation.

The same rules are re-applied when the browser fetches the bytes through
`/fs/raw`, so a stale transcript cannot read a file that has since moved outside
the roots.

---

## 5. Auto-promotion of bare media paths

If the model forgets the directive and simply writes a media path in prose,
the engine still attaches it:

- Matched only for previewable kinds: **image, video, audio, PDF**.
- The path must look absolute (`/...` or `~/...`) and end in a known media
  extension.
- The path must be preceded by a boundary (start of line, whitespace, quote,
  parenthesis, backtick), so URL paths and word-glued paths are not promoted.
- Fenced code blocks are ignored.
- Non-media files (`.md`, `.zip`, source code, …) are **not** auto-promoted;
  they require `@attach(...)` or the `@/path` shorthand.

The bare path stays in the answer text; only the directive is hidden by the
WebUI.

---

## 6. End-to-end flow

```
final assistant text
        │
        ▼
internal/attachment.Collect ──► []Item (AbsPath, Virtual, Name, Kind, Mime, Size)
        │                         ├─ @attach(...) / @/path / @~/path
        │                         └─ Promote: bare media paths
        ▼
internal/services/bot/service/attach.go
        ├─ api    → channel.Attachment{Path, Kind, Size}   (text unchanged)
        ├─ telegram/discord → read bytes → channel.Attachment{Data}  (text substituted)
        └─ cli/tui→ no attachment                          (text substituted)
        ▼
channel.OutgoingMessage.Attachments
        ├─ SSE `done` event  → WebUI renders live
        ├─ API response      → non-stream clients
        └─ agent transcript  → Message.Attachments (path metadata only)
```

On the API surface, files up to **5 MB** also carry `data_base64` for older
clients; larger files are path-only and never enter the event stream. On
messaging surfaces, files above **20 MB** are skipped (the platform uploads
bytes, so the file is read into memory).

---

## 7. Attachment shape

Internal (`internal/channel/message.go`):

```go
type Attachment struct {
    FileName string
    MimeType string
    Data     []byte // messaging / small legacy attachments
    Path     string // virtual path for /fs/raw (API/WebUI)
    Kind     string // image | video | audio | pdf | file
    Size     int64
}
```

SSE `done` event (`internal/channel/api/timeline.go`):

```json
{
  "file_name": "clip.mp4",
  "mime_type": "video/mp4",
  "path": "/home/workspace/clip.mp4",
  "kind": "video",
  "size": 1048576
}
```

Small tool attachments keep the legacy shape:

```json
{
  "file_name": "shot.png",
  "mime_type": "image/png",
  "data_base64": "iVBORw0KGgo..."
}
```

The non-streaming API response (`internal/services/bot/handler/api_handler.go`)
uses the same fields on `AttachmentResponse`.

---

## 8. Persistence and history

Attachment metadata is stored with the assistant turn in
`provider.Message.Attachments` (`[]provider.MessageAttachment`):

```json
{
  "role": "assistant",
  "content": "...",
  "attachments": [
    {"file_name": "clip.mp4", "mime_type": "video/mp4", "path": "/home/workspace/clip.mp4", "kind": "video", "size": 1048576}
  ]
}
```

- Only path-based attachments are persisted. Byte-only attachments (messaging
  uploads) are not written to the transcript.
- `CloneMessages` copies the slice, so checkpoints, compaction, and title
  generation keep it.
- Provider adapters build their own wire types and never send this field to a
  model.
- Opening a chat later re-renders the same previews from the stored metadata;
  the bytes are fetched again from `/fs/raw`.

Turns stored before this feature have no `attachments` field and are not
retroactively enriched.

---

## 9. Rendering (WebUI)

`webui/src/lib/components/app/RunAttachments.svelte` renders the attachments of
one answer, both live and from history:

| Kind | Rendering |
| :--- | :--- |
| `image` | inline `<img>`, capped at `max-h-72`, natural width |
| `video` | `<video controls preload="metadata">`, aspect-correct, capped at `70vh` |
| `audio` | `<audio controls>` |
| `pdf` | inline `<iframe>` |
| anything else | download card with name, size, and a `download=1` link |

When an answer has **more than one** photo/video, they are grouped into a
horizontal carousel (`snap-x`, hidden scrollbar, dot indicators). On mobile the
carousel supports swipe and shows previous/next arrow buttons; the arrows are
disabled at the ends. Non-media files stay in their own rows.

Media URLs are built with `files.rawUrl(agent, path)`, which produces an
absolute URL (control-plane base) with the session token as a query parameter.
That is what makes previews work when the WebUI and the backend are on
different servers. `/fs/raw` supports `Range`, so video can seek.

The chat page also adds a `ResizeObserver` on the transcript: when media
finishes loading and the content grows, the view stays pinned to the bottom if
the reader was already there.

---

## 10. Limits

| Limit | Value | Scope |
| :--- | :--- | :--- |
| Inline base64 for API clients | 5 MB | per file |
| Messaging upload | 20 MB | per file; larger files are skipped and logged |
| Attachment count | unbounded | metadata only; no per-response cap |

`@attach(...)` lines are not counted against the message text; they are hidden
by the WebUI and replaced by the path on messaging/terminal surfaces.

---

## 11. Troubleshooting

| Symptom | Likely cause |
| :--- | :--- |
| File text shows but no card | The model wrote a plain non-media path without `@attach`. Only media paths are auto-promoted. |
| A literal `@` remains in front of the path | The model wrote `@/path`; older builds did not recognize the shorthand. Current builds do, and the WebUI hides the `@`. |
| Nothing appears for an old turn | Attachments are stored per turn. A turn processed before the feature (or with an unresolved path) is not retroactively enriched; re-ask in a new turn. |
| `@attach(...)` is visible for a moment while streaming | The WebUI hides a complete directive; a partially streamed one is hidden as soon as the closing `)` arrives. |
| A file is skipped | Path outside the roots, hidden/credential name, symlink escape, or above the surface size limit. The messaging path logs `Skipping attachment …`. |
| Cards appear twice | Only one of the live run or the stored transcript should render a turn; reload the chat to settle the run. |

---

## 12. Code map

| Concern | File |
| :--- | :--- |
| Directive parsing, path resolution, kinds, auto-promotion | `internal/attachment/attachment.go` |
| Per-surface delivery and transcript persistence | `internal/services/bot/service/attach.go` |
| Run integration (final text, transcript cleanup) | `internal/services/bot/service/conversation.go` |
| Static prompt instruction | `internal/agent/context.go` |
| Attachment struct | `internal/channel/message.go` |
| SSE serialization | `internal/channel/api/timeline.go` |
| Non-stream API response | `internal/services/bot/handler/api_handler.go` |
| Stored metadata type | `internal/provider/provider.go` |
| Transcript cloning | `internal/agent/messages.go` |
| WebUI renderer | `webui/src/lib/components/app/RunAttachments.svelte` |
| Directive display and per-message rendering | `webui/src/routes/app/chat/[[conversation]]/+page.svelte` |
| Live `done` handling | `webui/src/lib/stores/chatRuns.svelte.ts` |

---

## 13. Not implemented yet

- A dedicated **project card** that opens a static web project in a sandboxed
  preview server (a directory with `index.html`).
- An artifacts gallery that collects files across sessions.
- A configuration switch to disable bare-path auto-promotion.
