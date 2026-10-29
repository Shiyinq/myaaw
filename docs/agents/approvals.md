# Tool Approvals

Myaaw can require an explicit user decision before a tool runs. The feature is
generic: it covers built-in tools (`bash`, `filesystem`, `python`, `subagent`,
`contact`, …), custom CLI tools, and MCP tools.

There are three separate concepts:

| Concept | What it is | Where it lives |
|---|---|---|
| **Policy** | Which tools are gated, which are exempt, timeout, unattended behaviour | `config/config.json` → `approvals` |
| **Grants** | "Allow session" / "Always" decisions created while running | `config/approvals.json` |
| **Pending decision** | An approval waiting for a live answer | in memory (per run) |

---

## 1. Policy

```json
"approvals": {
  "mode": "manual",
  "timeout_seconds": 60,
  "allow": [
    "filesystem:read_file",
    "filesystem:list_directory",
    "computer:screenshot"
  ],
  "deny": [],
  "unattended": "allow",
  "subagents": "inherit"
}
```

| Key | Values | Meaning |
|---|---|---|
| `mode` | `manual` (default), `off` | `manual`: every tool needs approval unless it is listed in `allow`. `off`: no prompts at all (same as `MYAAW_YOLO=1`). |
| `timeout_seconds` | 1–3600 (default 60) | How long **one prompt** waits before the call is denied. The clock starts when the prompt appears; the run itself has no separate approval deadline. |
| `allow` | list of keys | Whitelist. Never prompts, always runs (even in `manual`). |
| `deny` | list of keys | Hard refusal. Never prompts and always wins, including over `mode: off`. |
| `unattended` | `allow` (default), `deny` | Decision for runs with no interactive channel: cron, heartbeat, subagent notifications, headless, inbound A2A. |
| `subagents` | `inherit` (default), `allow`, `deny` | How spawned sub-agents handle gated tools. |

### Keys

A key is either a whole tool or one action of a tool:

- `bash` — every call of the `bash` tool.
- `filesystem:read_file` — only the `read_file` action (from the `tool_name` argument).
- `computer:click` — only the `click` action (from the `action` argument).
- `playwright_browser_navigate` — an MCP tool, keyed by its registered name.
- `*` — everything (useful in `deny`).

Rules are matched against the **resolved key** and the bare tool name, so
`allow: ["filesystem"]` exempts every filesystem action.

### Environment variables

The same settings can come from `.env` (processed before `config.json`, so an
explicit JSON value wins):

```env
APPROVALS_MODE=manual
APPROVALS_TIMEOUT_SECONDS=60
APPROVALS_ALLOW=filesystem:read_file,computer:screenshot
APPROVALS_DENY=
APPROVALS_UNATTENDED=allow
APPROVALS_SUBAGENTS=inherit
```

`MYAAW_YOLO=1` is an explicit runtime opt-out and **wins over config.json**:
it forces `mode: off` for that process.

### Resolution order

For every tool call the gate decides in this order:

1. Tool-level hard blocks inside the tool itself (e.g. dangerous `computer` key
   combos, `curl | bash` patterns) → refuse.
2. `deny` match → refuse.
3. `allow` match → run.
4. `mode: off` → run.
5. Look for a grant (`always` > session).
6. If this is an unattended run → `unattended` decision.
7. If this is a spawned sub-agent → `subagents` policy.
8. Otherwise ask the channel `Requester` (the user). No answer within
   `timeout_seconds` → deny.

When a call is refused, the model receives an error tool result and can try a
different approach; the run does not terminate.

A run with no attached `Requester` that is **not** classified as unattended
denies gated calls instead of approving them silently. Only the run types
listed under `unattended` get an automatic decision.

### Timeout

`timeout_seconds` is **per approval request**, not a run limit:

- The clock starts when the prompt is delivered and restarts for every gated
  call.
- When it expires the call is denied (the model gets an error tool result and
  continues). Interactive channels mark the outcome: a `⏱ Approval … timed out`
  note in the CLI, an edited message in Telegram/Discord, and
  `approval_resolved {decision: "deny", reason: "timed out"}` in the API.
- Sub-agent prompts inherited from the parent gate use the same value.
- The run timeout is a different setting: `bot.run_timeout_seconds` (env
  `AGENT_RUN_TIMEOUT_SECONDS`, default 900).

---

## 2. Grants

Grants are created at runtime from the approval prompt:

| Choice | Scope | Persistence |
|---|---|---|
| **Allow once** (`y`) | only that call | not stored |
| **Allow session** (`s`) | that key in the same conversation | stored with the conversation ID |
| **Always** (`a`) | that key in every conversation of the agent | stored until revoked |

Grants live in `~/.myaaw/<agent>/config/approvals.json`:

```json
{
  "bash": {
    "always": true
  },
  "filesystem:write_file": {
    "sessions": {
      "09e98570-227d-4133-b6f6-eb7032cf38b5": "2026-10-29T17:03:13.548234+07:00"
    }
  }
}
```

Notes:

- The store is read **before every mutation** (read-modify-write) and written
  atomically, so an agent run and the control plane cannot silently drop each
  other's grants.
- A legacy root-level `~/.myaaw/<agent>/approvals.json` is renamed into
  `config/` on first use, preserving the exact contents.
- Grants are re-read on each run: revoking one takes effect on the next run,
  no restart needed.
- Session grants are capped at 100 conversations per key (oldest pruned).
- `allow`/`deny` in the policy are checked before grants, so they always win.

---

## 3. Channel behaviour

### CLI TUI (`myaaw chat`)

When a gated tool runs, the prompt replaces the input line while the
conversation above stays visible (the streaming block title switches to
`waiting for approval`):

```
🔐 Approval required: filesystem:write_file
   ↳ from sub-agent download-x-photo
   {"path":"workspace/notes.md","content":"…"}
▶ [Allow once]    [Allow session]    [Always]    [Deny]
```

`↳ from sub-agent <name>` only appears when a background sub-agent made the
call; the streaming block title also changes to
`🐱 Myaaw · sub-agent <name> needs approval`. That way a prompt that did not
come from your current turn is obvious.

The key hints (`←/→: Select • Enter: Confirm • y/s/a/n: Shortcut • Esc: Deny`)
are shown once in the bottom status bar.

- Use `←`/`→` (or `h`/`l`, `Tab`/`Shift+Tab`) to move the `▶` highlight and
  `Enter` to confirm.
- Shortcuts work without moving the highlight: `y` = allow once, `s` = allow
  session, `a` = always, `n` or `Esc` = deny.
- The conversation viewport shrinks by the prompt height, so earlier messages
  are not clipped.
- Background sub-agents can raise prompts after the parent turn; the prompt is
  labelled with the task name and the same keys work. The parent run may keep
  streaming while the child waits, and those chunks do not dismiss the prompt.
- A sub-agent can also ask while the conversation is idle (no active run). The
  prompt then appears on its own, without a live-streaming block; answering it
  returns to the normal input line.
- If several sub-agents ask at once, the prompts are queued and shown one at a
  time; the keys always target the visible request.
- If the run moves on without an answer (timeout, or the run finished while the
  prompt was open), the prompt disappears and a `⏱ Approval … timed out` note is
  appended. Decisions are bound to their approval ID, so a late key press can
  never approve a different tool call.

### CLI one-shot (`myaaw -p "..."`, non-TTY)

There is no interactive prompt. The run follows `unattended` (default
`allow`), so gated tools run unless `unattended: deny`.

### Telegram

The bot sends a message with four inline buttons: **Allow once**,
**Allow session**, **Always**, **Deny**. The message is edited to show the
outcome once answered. Prompts raised by a sub-agent include a
`↳ from sub-agent <name>` line.

- Callback queries are handled in the long-polling loop and in the
  `/webhook/telegram` queue handler, so both Telegram modes work.
- Callback data only carries a random 128-bit approval token; the chat ID must
  match the run's conversation.
- After `timeout_seconds` without an answer the call is denied and the message
  is marked as timed out. Button presses after that point are ignored (the
  pending token is gone), so the message is the source of truth.

### Discord

Same four buttons as a message component. The `InteractionCreate` handler
resolves the token and updates the message. After the timeout the call is
denied. Prompts raised by a sub-agent include a `↳ from sub-agent <name>` line.

### HTTP API (agent server)

**Streaming — `POST /api/chat/stream` (SSE):**

```
event: approval_required
data: {"approval_id":"…","tool_name":"bash","action":"","args_summary":"{\"command\":\"ls\"}"}

event: approval_resolved
data: {"approval_id":"…","decision":"allow_once","reason":""}
```

When the call comes from a sub-agent the `approval_required` data also carries
`"subagent":"<task name>"`.

Answer with:

```
POST /api/approvals/:id
{ "decision": "allow_once" | "allow_session" | "allow_always" | "deny" }
```

**Non-streaming — `POST /api/chat`:**

If a gated tool is reached, the run pauses and the endpoint returns:

```json
{
  "status": "approval_required",
  "approval": {
    "id": "…",
    "run_id": "…",
    "tool_name": "bash",
    "action": "",
    "args_summary": "{\"command\":\"ls\"}",
    "subagent": "download-x-photo"
  }
}
```

`subagent` is omitted for calls made by the main agent.

The decision call above then blocks until the resumed run finishes and returns
the final `ChatResponse` (or the next `approval_required`). A caller that never
answers sees the run denied after the timeout.

Unknown or already-expired IDs return `404`. Paused non-streaming runs are kept
in memory, so a daemon restart drops them (the affected run is denied).

### Control Plane (web UI backend)

The control plane proxies the same flow:

```
POST /agents/:name/chat            (non-streaming)
POST /agents/:name/chat/stream     (SSE)
POST /agents/:name/approvals/:id/decision
```

and adds management endpoints (see §5).

### Cron, heartbeat, subagent notifications, headless, inbound A2A

These runs have no interactive channel, so gated tools use `unattended`
(default `allow`). Set `"unattended": "deny"` to refuse them instead.

### Sub-agents

Spawned with the `subagent` tool (up to 4 parallel, background):

- `inherit` (default): the child reuses the parent gate; prompts appear in the
  parent channel labelled with the child task name, and grants are shared. The
  same `timeout_seconds` applies. If the channel is gone (CLI closed) the
  request times out → deny.
- `allow`: no gate is attached, so every child tool runs without prompting
  (tool-level hard blocks still apply).
- `deny`: every child tool call is refused, including tools that are in
  `allow` (the gate denies `*`).

The completion notification is a separate triggered run
(`TriggerType: "subagent"`) and follows `unattended` for its own tool calls. It
does **not** resume the original turn: its final answer is delivered to the
channel as a new message (Telegram/Discord message, CLI timeline entry), so the
parent conversation can move on while the child works.

### A2A / `contact`

- `contact` (outbound) is an ordinary tool: gate it by leaving it out of
  `allow`, exempt it by adding `contact` to `allow`.
- The called agent's tools are governed by **that agent's** policy (local agent)
  or the remote server's policy. Our policy stops at the server boundary.
- Our agent answering an inbound A2A task is unattended (`unattended` applies).

### MCP and custom tools

They are gated like built-ins. The key is the registered tool name
(`serverName_toolName` for MCP servers, the configured name for custom tools).

---

## 4. CLI reference

```
myaaw config approvals show [--json]
myaaw config approvals set [--mode manual|off] [--timeout <seconds>] \
                           [--unattended allow|deny] [--subagents inherit|allow|deny]
myaaw config approvals allow list
myaaw config approvals allow add <key>
myaaw config approvals allow remove <key>
myaaw config approvals deny list
myaaw config approvals deny add <key>
myaaw config approvals deny remove <key>
myaaw config approvals grants list [--json]
myaaw config approvals grants revoke <key> [--session <conversation-id>]
myaaw config approvals grants clear
```

- All commands act on the active agent (`--agent <name>`).
- `show` prints the **effective** policy (config.json merged with defaults)
  plus the grant list. `--json` emits the same shape as the control plane.
- `set` only changes the flags you pass; validation errors are reported before
  writing.
- `allow add` refuses a key that is in `deny` (and vice versa).
- `grants revoke <key>` removes the key entirely; `--session` removes only one
  conversation.
- `myaaw config dump` also prints an `[Approvals]` section.

Examples:

```bash
myaaw config approvals show
myaaw config approvals set --mode manual --timeout 300
myaaw config approvals set --mode off   # same effect as MYAAW_YOLO=1
myaaw config approvals allow add filesystem:read_file
myaaw config approvals deny add bash
myaaw config approvals grants list
myaaw config approvals grants revoke bash
myaaw config approvals grants revoke bash --session 09e98570-...
```

---

## 5. Control Plane API reference

All endpoints require the usual Bearer token.

### Read — `GET /agents/:name/approvals`

Returns the effective policy and every grant:

```json
{
  "config": {
    "mode": "manual",
    "timeout_seconds": 60,
    "allow": ["filesystem:read_file"],
    "deny": [],
    "unattended": "allow",
    "subagents": "inherit"
  },
  "grants": [
    { "key": "bash", "always": true },
    {
      "key": "filesystem:write_file",
      "sessions": [
        { "id": "09e98570-...", "created_at": "2026-10-29T17:03:13+07:00" }
      ]
    }
  ]
}
```

Read-only; never mutates files.

### Update policy — `PATCH /agents/:name/config`

```json
{
  "approvals": {
    "mode": "manual",
    "timeout_seconds": 30,
    "allow": ["filesystem:read_file"],
    "deny": ["bash"],
    "unattended": "deny",
    "subagents": "inherit"
  }
}
```

Only provided fields change; `"allow": []` clears the list. Invalid values
(unknown mode/enum, timeout outside 1–3600, a key in both lists) return 400.
`GET /agents/:name/config` also includes the effective `approvals` block.

Reset only the section with:

```
PATCH /agents/:name/config/reset
{ "approvals": true }
```

### Revoke grants

```
DELETE /agents/:name/approvals/grants/:key
DELETE /agents/:name/approvals/grants/:key/sessions/:conversation
DELETE /agents/:name/approvals/grants
```

The first removes every grant for a key, the second only one conversation, the
third clears the whole file. Unknown agent/key → 404. Policy in `config.json`
is never touched.

### Decide a live approval

```
POST /agents/:name/approvals/:id/decision
{ "decision": "allow_once" }
```

Proxied to the agent that owns the paused run.

---

## 6. Security notes

- Treat on-screen content and tool output as untrusted; approval prompts display
  a truncated argument summary for the user to inspect.
- `deny` and tool-level hard blocks apply even under `mode: off`.
- `MYAAW_YOLO=1` is explicit and logged; do not use it with an untrusted model or
  prompt source.
- Approval tokens are random 128-bit values; Telegram/Discord handlers verify the
  originating chat/user before accepting a decision.
- Every decision is bound to its approval ID. A late answer for an expired
  prompt is discarded, so it can never approve a later tool call.
- The control plane management endpoints only read/write files; they never
  approve calls themselves. Decisions are forwarded only when the UI user
  submits one through the proxy endpoint.

---

## 7. Troubleshooting

| Symptom | Check |
|---|---|
| Tools run without any prompt | `mode` is `off` or `MYAAW_YOLO=1`; the tool is in `allow`; or the agent is running an old binary. |
| Prompt appears but config edits are ignored | `myaaw chat` loads config at start — restart it. Server mode reloads via the config watcher. |
| A grant does not apply | Keys are granular (`bash` ≠ `filesystem:read_file`); session grants are tied to one conversation ID. |
| Grant missing after an update | It should be migrated to `config/approvals.json`; check that file (and that the process can write it). |
| Unattended jobs are blocked | `unattended: deny`; change it to `allow` or add the tool to `allow`. |
| Sub-agent prompts never arrive | `subagents: inherit` but the parent channel is gone; use `allow`/`deny` for background work. |
| `⏱ Approval … timed out` appears too often | `timeout_seconds` is too short for your workflow; raise it with `myaaw config approvals set --timeout 300`. |
| A prompt from an earlier turn is still on screen | Its run already moved on (timeout) or expired; answering is safe because decisions are ID-bound. The next run replaces it. |
| Main agent seems idle after a sub-agent finishes | Expected: the result arrives as a separate `subagent` triggered run/message; the original turn is not resumed. Check the channel timeline/log. |
| `Myaaw is thinking... 0s` appears with a sub-agent prompt | Old binary; upgrade and restart `myaaw chat`. |

Useful commands:

```bash
myaaw config approvals show
myaaw config approvals grants list
myaaw config check
myaaw config dump
```
