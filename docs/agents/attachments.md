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
| Telegram / Discord | Directive is replaced by the plain absolute path | Bytes read into memory and uploaded natively (Telegram: photo for JPEG/PNG, animation for GIF, video for MP4, audio for MP3/M4A, document otherwise; photos/videos grouped into albums); the virtual path is also persisted for dashboard history |
| CLI / TUI | Directive is replaced by the plain absolute path | None |
| A2A / other | Directive is replaced by the plain absolute path | None |

Why the WebUI keeps the directive in the text: the transcript stores exactly
what the model wrote, and the frontend decides how to present it. This also
means the model sees its own directive when the transcript is replayed.

`@/path` and `@~/path` are recognized on every surface, for every extension
(including `.md`). Bare paths without `@` follow the promotion rules in §5:
media anywhere in prose, other files only on a line of their own.

---

## 4. Validation and security

Attachments cross a trust boundary: the model chooses the path, the browser or
chat platform fetches the bytes, and the agent host owns the file. The rules
below keep that boundary narrow.

### 4.1 Where a reference may come from

- Only the **final assistant answer** is scanned for `@attach(...)`. Directives
  in tool output, intermediate text, user messages, and fenced code blocks are
  documentation, not delivery.
- Bare paths follow the promotion rules in §5 and pass the same validation as
  directives.
- Users cannot attach arbitrary agent files from the composer; their uploads are
  stored separately under `<agent>/user/uploads/…` (§4.6).

### 4.2 Path validation (delivery time)

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

### 4.3 Serving the bytes (`/fs/raw`)

- The virtual path is **re-validated on every request**: allowed roots, symlink
  containment, and a regular file. A transcript that points outside the roots
  stops working even if it was stored earlier.
- `X-Content-Type-Options: nosniff` keeps the browser from guessing a type from
  the bytes.
- `Content-Security-Policy: sandbox; default-src 'none'; …` is added for `svg`,
  `html`, and `xml`, so opening one directly cannot inherit the control-plane
  origin's privileges.
- `download=1` sends `Content-Disposition: attachment`; `Range` is supported so
  video can seek.
- Requests without a valid session token get `401`.

### 4.4 Tokens in media URLs

`<img>`, `<video>`, `<iframe>`, and `<a download>` cannot send an
`Authorization` header, so `/fs/raw` also accepts the session token as a query
parameter (`internal/services/controlplane/middleware/auth.go`). It is a
24-hour local session. Consequences:

- the token can appear in browser history, proxy access logs, and any monitoring
  that records URLs;
- use **HTTPS** for anything beyond loopback; over plain HTTP the token is
  readable on the network;
- a remote dashboard must either share the API origin (reverse proxy) or be
  listed in `ALLOWED_ORIGINS`; the default allows loopback origins only
  (`internal/services/controlplane/router.go`);
- browsers ignore the `download` attribute for cross-origin URLs, so remote
  dashboards may open a file instead of saving it. Same-origin serving avoids
  that.

### 4.5 Messaging surfaces

- Telegram and Discord receive the **bytes**, so the file leaves the agent host
  and is stored by that platform; the platform's retention rules apply. Attach
  only what the user asked for.
- Files above 20 MB are skipped with a log line and never uploaded.
- Telegram errors are redacted before they reach the chat
  (`redactTelegramToken`), because the Bot API URL embeds the bot token.
- The virtual path is still stored in the transcript for dashboard history, so
  the file must remain inside the agent roots to render later.

### 4.6 User uploads (the other direction)

- The API accepts at most 8 files per message, 10 MB each and 20 MB total
  (`internal/channel/api/adapter.go`).
- Names are reduced to a basename and unknown extensions are rejected.
- Files are written with mode `0600` under
  `<agent>/user/uploads/<conversation>/<message>/`, one directory per message,
  with unique names so an upload never overwrites an earlier file.
- The model never receives the bytes: only absolute paths inside `<attached>`,
  which it reads with its own tools.

### 4.7 Auto-promotion risk

- Media paths in prose are promoted aggressively; other files only when a line
  contains just the path. Both shapes still pass §4.2, so an outside path or a
  directory is ignored.
- `.log`, `.tmp`, `.temp`, `.cache`, `.bak`, `.swp`, and `.swo` are never
  promoted, so scratch output does not become a delivery by accident.
- Fenced code blocks are never scanned, so documentation examples stay inert.

### 4.8 Out of scope

- This validation is about **where** a file comes from, not **what** it
  contains. An agent-produced file can still be malicious; treat downloaded
  artifacts as untrusted.
- The agent host's filesystem permissions are the final boundary. Run the agent
  as a dedicated user when the machine is shared.

---

## 5. Auto-promotion of bare paths

If the model forgets the directive, the engine still attaches files it names
with a bare path, in two shapes.

**Previewable media anywhere in prose** (image, video, audio, PDF):

- The path must look absolute (`/...` or `~/...`) and end in a known media
  extension.
- It must be preceded by a boundary (start of line, whitespace, quote,
  parenthesis, backtick), so URL paths and word-glued paths are not promoted.

**Any other file on a line of its own** (archives, documents, source files):

- The line must contain only the path, optionally wrapped in inline-code
  backticks or prefixed by a list bullet (`- /path/file.zip`). This is how a
  model lists a packaged deliverable.
- The path must exist inside the agent roots, like any other attachment.
- Scratch extensions (`.log`, `.tmp`, `.temp`, `.cache`, `.bak`, `.swp`, `.swo`)
  are skipped, so debug output is not delivered by accident.

A non-media path **inside a sentence is not promoted**. For example,
`The zip is at /home/workspace/a.zip, ready to download` stays text only; the
model must use `@attach(/home/workspace/a.zip)` (or put the path on its own
line) to deliver it. This keeps an explanation that mentions a config, log, or
source file from attaching every path it names.

Fenced code blocks are ignored in both shapes. The bare path stays in the answer
text; only the directive is hidden by the WebUI.

---

## 6. End-to-end flow

```
final assistant text
        │
        ▼
internal/attachment.Collect ──► []Item (AbsPath, Virtual, Name, Kind, Mime, Size)
        │                         ├─ @attach(...) / @/path / @~/path
        │                         └─ Promote: media in prose, other files on their own line
        ▼
internal/services/bot/service/attach.go
        ├─ api    → channel.Attachment{Path, Kind, Size}   (text unchanged)
        ├─ telegram/discord → read bytes → channel.Attachment{Data, Path}  (text substituted)
        └─ cli/tui→ no attachment                          (text substituted)
        ▼
channel.OutgoingMessage.Attachments
        ├─ SSE `done` event  → WebUI renders live
        ├─ API response      → non-stream clients
        └─ agent transcript  → Message.Attachments (path metadata only)
```

The API surface only carries **fetchable paths**; no file bytes enter the event
stream or the API response. On messaging surfaces, files above **20 MB** are
skipped (the platform uploads bytes, so the file is read into memory).

---

## 7. Attachment shape

Internal (`internal/channel/message.go`):

```go
type Attachment struct {
    FileName string
    MimeType string
    Data     []byte // messaging uploads (bytes); never serialized to the API
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

The non-streaming API response (`internal/services/bot/handler/api_handler.go`)
uses the same fields on `AttachmentResponse`. Tool attachments that live on disk
also set `Path` (for example the `computer` tool's
`/home/workspace/screenshots/shot.png`), so screenshots are streamed instead of
embedded. A byte-only tool attachment has no path and is omitted from the API
payload; those bytes are for messaging uploads only.

MCP servers run with `/home/workspace/mcp` as their working directory, so files
they write with a relative name (Playwright screenshots, downloads) land there
and can be attached as `/home/workspace/mcp/<file>`.

> Screenshot bytes used by the **model** are a separate path:
> `StructuredResult.Images` becomes a provider `image_url` content item. Removing
> delivery bytes does not affect the agent's ability to see tool output.

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

- Attachments that resolve to a file are persisted with their virtual path on
  every surface. Messaging adapters upload bytes for the platform but still keep
  the path, so the dashboard renders the same file in chat history.
- Attachments without a path (for example a tool attachment that only carries
  bytes) are not written to the transcript.
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
| Messaging upload | 20 MB | per file; larger files are skipped and logged |
| Attachment count | unbounded | metadata only; no per-response cap |

`@attach(...)` lines are not counted against the message text; they are hidden
by the WebUI and replaced by the path on messaging/terminal surfaces.

---

## 11. Troubleshooting

| Symptom | Likely cause |
| :--- | :--- |
| File text shows but no card | The model wrote a plain path without `@attach`. Media is promoted anywhere in prose; other files are promoted only when a line contains just the path. |
| A literal `@` remains in front of the path | The model wrote `@/path`; older builds did not recognize the shorthand. Current builds do, and the WebUI hides the `@`. |
| Nothing appears for an old turn | Attachments are stored per turn. A turn processed before the feature (or with an unresolved path) is not retroactively enriched; re-ask in a new turn. |
| A file arrived on Telegram/Discord but not in dashboard history | The turn predates path persistence, or the file has since left the agent roots. New turns store the virtual path next to the uploaded bytes. |
| `@attach(...)` is visible for a moment while streaming | The WebUI hides a complete directive; a partially streamed one is hidden as soon as the closing `)` arrives. |
| A file is skipped | Path outside the roots, hidden/credential name, symlink escape, or above the surface size limit. The messaging path logs `Skipping attachment …`. |
| Cards appear twice | Only one of the live run or the stored transcript should render a turn; reload the chat to settle the run. |

---

## 12. Code map

| Concern | File |
| :--- | :--- |
| Directive parsing, path resolution, kinds, auto-promotion | `internal/attachment/attachment.go` |
| Per-surface delivery and transcript persistence | `internal/services/bot/service/attach.go` |
| Tool-emitted attachments (`StructuredResult.Attachments`) | `internal/agent/tool.go`, `internal/agent/tools/computer/computer.go` |
| Run integration (final text, transcript cleanup) | `internal/services/bot/service/conversation.go` |
| Static prompt instruction | `internal/agent/context.go` |
| Attachment struct | `internal/channel/message.go` |
| SSE serialization | `internal/channel/api/timeline.go` |
| Raw file serving and re-validation | `internal/services/controlplane/service/filesystem.go`, `internal/services/controlplane/handler/files.go` |
| Media token and CORS | `internal/services/controlplane/middleware/auth.go`, `internal/services/controlplane/router.go` |
| Upload validation and storage | `internal/channel/api/adapter.go`, `internal/services/bot/service/uploads.go` |
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
