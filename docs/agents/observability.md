# Observability

Observability answers one question: **what actually happened during that run?**
It records a trace per inbound message — the prompt that was sent, the model
requests, the tool calls, their latency, token usage, and what they cost.

Two ideas are easy to confuse, so start here:

- **Observability is a camera.** It records what the agent did, after the fact.
  It never changes a decision, a prompt, or a result.
- **Eval is a referee.** It scores a run against expectations and can block a
  release. Eval is not implemented yet; see the [roadmap](#roadmap).

Traces, metrics, and cost all come from the same append-only record, so a slow
answer and an expensive answer can be traced back to the same span.

---

## 1. Vocabulary

The trace uses the same words as the DeepSeek Harness trajectory view:

- A **turn** is one user message and everything the agent did for it. In the
  database that unit is a run (one `RunEvents` call), so
  `myaaw observability run <id>` shows exactly one turn.
- A **step** is one loop iteration: one provider request plus the tool calls it
  requested. The engine emits `EventStepStart` / `EventStepEnd` around it.

| Myaaw | Trace span | Meaning |
| :--- | :--- | :--- |
| one `RunEvents` call | `run` | One user turn, handled end to end. |
| one loop iteration | `step` | One model request plus the tools it called. |
| one provider request | `llm` | Tokens, TTFT, generation time, cost. |
| one tool call | `tool` | Arguments, result, duration, error state. |

In other words: `Turn 1` in the Harness trajectory view is a whole Myaaw run,
and the rows inside it are `step` spans.

Every run also records what started it: `user` (a chat message), `cron` (a
scheduled job), `subagent` (a delegated child run), or `a2a` (a task from another
agent). Heartbeats are not recorded.

A trace looks like this:

```text
run                     telegram · deepseek-flash · completed · 3.2s · $0.0004
├── system              the assembled system prompt (identity, skills, workspace)
├── user                the inbound message
├── step 1              loop iteration 1
│   ├── llm             deepseek-flash · ttft 780ms · 291 tok/s
│   ├── tool            bash · 42ms
│   └── tool            filesystem · 8ms
├── step 2              loop iteration 2
│   ├── compaction      transcript trimmed before the request
│   └── llm             deepseek-flash
└── step 3              loop iteration 3
    ├── llm             deepseek-flash · request 2
    └── tool            bash · 12ms
```

The system message contains every injected piece — identity files, skills, and
workspace paths are assembled into it before the request. There is no separate
`context` span: nothing is sent as a second system message, and a trace records
what the provider actually received. A tool blocked by policy appears as a
`tool` span with `status: error`, not as a separate `gate` span.

`request_index` counts model requests across the run. A retried request stays in
the same step but takes the next request index, so a retry is visible instead of
being folded into one number. The failed attempt is marked `retried`, which does
not fail the step: the retry can still complete the turn normally.

### Sessions, turns, and runs

One user message is one turn, and one turn is one recorded run. A **session** is
the set of runs that share a `conversation_id`, so chatting twice in the same
conversation produces two runs — Turn 1 and Turn 2 of the same session.

```bash
myaaw obs sessions                                  # all sessions, pick one
myaaw obs session 4f1c8a2e-8f3b-4c1d-9a77-2b6e5d0c1a44
```

```text
Session 4e7371da-… · 1 turn(s)
  TURN TYPE  TRIGGER  STARTED   RUN                    MODEL              STEPS DUR     TTFT   TOKENS      COST       STATUS
  1    cli   user     23:34:25  run_2240477b6447ff48   deepseek-v4-flash  5     14.35s  809ms  63.1k/732   $0.003927  completed

1 turn(s) · $0.003927 models
Open a turn:  myaaw obs run <run-id>  or  myaaw obs run 4e7371da.cli.1
All runs:     myaaw obs runs
```

Every turn is one row: type, trigger, start time, run id, model, steps, duration,
TTFT, tokens, cost, and status. A turn also has a short reference,
`<session prefix>.<type>.<turn>` — `4e7371da.cli.1` above — which `myaaw obs run`
accepts in place of the run id, so a turn can be addressed from the table without
copying a 20-character identifier. The prefix must be unambiguous; if two
sessions share it, the command says so and asks for more characters.

The same filter is available as `myaaw observability runs --conversation <id>`
and as `?conversation=<id>` on the runs endpoint. Each session member stays a
separate row because the transcript, the model, and the context window differ
per turn — merging them would lose exactly the per-turn comparison an eval
needs.

---

## 2. Storage

Traces live in a separate SQLite database per agent:

```text
~/.myaaw/<agent>/database/observability.db
```

It is independent of `myaaw.db`, so deleting traces never touches users or
conversations. Tables:

| Table | Contents |
| :--- | :--- |
| `runs` | One row per agent run: channel, user, conversation, model, trigger, status, totals, and the parent run for a subagent. |
| `spans` | The trace tree: kind, parent, timing, tokens, cost, and the captured content. |
| `checks` | Reserved for eval checks. |

Writes happen on a background writer, batched, and flushed without blocking the
agent loop. If the database cannot be written the run still completes; the
failure is logged.

Recording covers every message the bot service handles: Telegram, Discord, the
HTTP chat API, the interactive CLI (`myaaw chat`), and cron jobs. Heartbeat runs
are skipped because they are liveness checks, not user work. `myaaw voice` and
`myaaw voice-classic` drive the engine directly and are not traced yet.

### Nested runs (subagents)

A subagent is a real, separate run with its own cost, but it belongs to the
trace that spawned it. The `subagent` tool starts a child run whose
`parent_run_id` points at the caller and whose root span hangs under the
`tool · subagent` span:

```text
run                     telegram · completed
├── step 1
│   ├── llm
│   └── tool subagent            started 2 background tasks
│       ├── run researcher       subagent · completed
│       │   ├── step 1            → llm, tool, tool
│       │   └── step 2            → llm
│       └── run fact-checker     subagent · completed
└── step 2
    └── llm
```

Reading the parent trace includes one level of child runs, so the tree renders
in one call. Subagents run in the background with cancellation detached —
`context.WithoutCancel` keeps the trace (and the tool-call id) available — so a
child can outlive the request that started it; its run row keeps updating until
it finishes. A child run is also listed on its own in `myaaw observability runs`,
which is why the summary can attribute spend to the agent that actually spent it.

Retention runs when the database opens and deletes runs older than
`retention_days` (30 by default). Spans are deleted with their run. A daemon that
stays up for weeks never re-opens, so it never prunes; run
`myaaw obs prune` (optionally with `--vacuum`) on a schedule if the database must
stay small. Periodic pruning inside the daemon is not implemented yet.

The schema carries a `PRAGMA user_version`. A newer build upgrades an older
database in place on open; version 2 added `parent_run_id` / `parent_span_id` for
nested subagent runs, version 3 renamed `turn_index` to `step_index`, version 5
added `trigger_type`, and version 6 added `tool_cost_usd`. Runs recorded before
version 5 keep an empty trigger and are shown as `-`; the upgrade labels nested
runs as `subagent` but does not guess between `user` and `cron` for the rest.

Opening also reconciles the actual shape rather than trusting the version alone.
If a database was opened by a build whose vocabulary differed — for example one
that called the column `iteration_index` — the column is renamed back and any
`turn` / `iteration` span kinds become `step`, so an older daemon and a newer CLI
can share one file. Write failures are logged once per run, not once per batch.

A run row is written when the run starts and updated as each model request
finishes, so `myaaw observability run` works on a live run: tokens, cost, and
"duration so far" are real values, and only the status stays `running` until the
final event arrives. If the process dies mid-run, the next start marks runs that
have been `running` for more than two hours as `interrupted`, including their open
spans.

---

## 3. Reading a trace

Every number below comes from a span. [Reading the tree](#reading-the-tree)
explains the printed layout, and the [glossary](#glossary) defines each term.

Rule of thumb when a turn is slow: if the `llm` span is fast but the step is
slow, the agent is waiting on a tool. If `input_tokens` grows while
`cache_hit_tokens` stays low, the agent is rebuilding context it could have
cached.

### Reading the tree

`myaaw obs run <id>` prints one turn:

```text
Run run_2240477b6447ff48
  cli · user · deepseek-v4-flash · completed
  duration 14.35s · TTFT 809ms · tokens 63.1k in / 732 out · cost $0.003927 models
  conversation 4e7371da-… · user 1c043f52-…
  5 step(s) · 5 model request(s) · 4 tool call(s)

  system
  user
  Step 1                                        5.30s
    llm · deepseek-v4-flash                     1.28s  ttft 809ms  9.6k/104 tok  $0.001387
      thinking: I need to open the product page first…  (+4 more)
      text:     Opening the product page.
    tool · playwright_browser_navigate          4.01s
  Step 2                                        1.21s
    llm · deepseek-v4-flash                     1.11s  ttft 909ms  9.8k/49 tok   $0.000092
      thinking: The page is open; I will search for the price.
    tool · playwright_browser_find              95ms
```

Header lines:

| Field | Meaning |
| :--- | :--- |
| `Run run_…` | Identifier of this turn, also accepted by `obs run` |
| `cli · user · deepseek-v4-flash · completed` | Channel that delivered the message, trigger, model that served the last request, run status |
| `duration` | Wall-clock time for the whole turn, including tool execution |
| `TTFT` | First-token time of the first model request in the turn |
| `tokens … in / … out` | Sum of prompt and completion tokens across every request in the turn |
| `cost` | Sum of the cost of every `llm` span, plus any priced tool calls; see [Cost](#4-cost) |
| `conversation`, `user` | Session id and the user the session belongs to |
| `N step(s) · N request(s) · N tool call(s)` | Counts for this turn only; a step is one request plus the tools it called |

Tree lines:

| Column | Meaning |
| :--- | :--- |
| `system` / `user` | The exact system prompt and inbound message. Markers, not timed work. |
| `compaction` | The transcript was trimmed before this request. The stored numbers say by how much. |
| `Step N` | One loop iteration. Its duration covers the request plus its tools. |
| `llm · <model>` | One model request. A retry adds another line with `request N`. |
| `tool · <name>` | One tool call. A blocked tool is shown with `[error]`. It has no token columns; the request above it owns the usage. |
| duration | Time spent in that span. Empty for markers. |
| `ttft …` | Time to first streamed token for that request. Absent when the provider returned the answer without streaming deltas, for example a response that only contains tool calls. |
| `9.6k/104 tok` | Input / output tokens of that request. Input grows every step because the transcript is resent. Cached input is recorded but not printed on this line. |
| `$0.001387` | Cost of that request, priced with the model and rate in force when it ran. |
| `thinking:` / `text:` | Content stored on the `llm` response above: a one-line preview by default, the full text with `--content`. Absent when that request produced no thinking or text (a pure tool call) or when content capture is off. |
| `[running]` | The span was still open when the trace was read; elapsed time is shown "so far". |
| `[error]` | The request or tool failed. The run keeps the message in `error`. |

### Token accounting around tool calls

A step always starts with one `llm` span, and that span owns the usage of the
request that decided what to do next. The `tool` spans below it are executions:
they carry no tokens at all.

A response that asks for a tool is still a normal model response. The provider
bills it like any text answer and returns the same usage block — prompt tokens,
completion tokens, cached input tokens, and the total. The only differences are
`stop_reason: tool_calls` and the absence of a text delta. Myaaw stores that
usage on the `llm` span; the `tool` spans only record the executions that follow.

| What the provider returns | Stored on | Printed |
| :--- | :--- | :--- |
| `prompt_tokens` (includes cached) | `llm.input_tokens` | `9.6k` on the `llm` row, `Tokens input` in `summary` |
| `cached_input_tokens` (part of the prompt) | `llm.cache_hit_tokens` | `cached` in `summary`, `--json`, `spans` table; not printed per row |
| `completion_tokens` (text, thinking, tool-call JSON) | `llm.output_tokens` | `/209 tok` on the `llm` row |
| `total_tokens` | not stored separately (`input + output`) | not printed |
| — | `tool` span has no token fields | no token columns |

Read the table as: **the request that decided to call a tool is what paid for
it.** There is no separate "tool tokens" bucket anywhere, and nothing is
missing from the turn total — `obs run` adds up every `llm` span in its header.

```text
Step 2                                             11.84s
  llm · deepseek-v4-flash      1.77s  10.2k/82 tok  $0.000152
  tool · bash                  7.20s
```

`10.2k/82 tok` belongs to the request that asked for the `bash` call. The 82
completion tokens are the tool-call JSON the model generated; the prompt
includes the transcript and the tool schema. The `tool` row has no token columns
because executing a tool does not call a model. When a tool itself calls a paid
API, that is priced separately through `tool_prices` (see
[Tool pricing](#tool-pricing)) and lands on the `tool` span, not on the `llm`
span.

#### Why the tree makes tool calls look free

The tree splits one step into a request row and execution rows. Only the request
row has token columns; the execution rows are indented because they belong to
that step, not because they have their own usage. That is a presentation
boundary, not a billing boundary. In the worked example below, Step 1's 209
output tokens were generated by the very request that chose `filesystem` and
`bash`; the two `tool` rows that follow add no tokens of their own.

Cached tokens are not a separate bucket either. They are the part of
`prompt_tokens` that the provider served from its prompt cache, so they are
already inside `input_tokens` and are not added on top. Pricing splits the
prompt into `miss = prompt - cached` and `cached`, which is why two requests
with similar input can differ several-fold in cost. The tree prints only the
combined input, so read `cache_hit_tokens` from `--json` or the `spans` table
when the cost looks surprising.

One consequence of this layout: a **tool-call-only response has no `ttft`**.
TTFT starts at the first streamed text or thinking delta. A model that answers
with `tool_calls` and nothing else streams no delta, so the field is empty even
though the request is billed.

A retried request gets its own `llm` row per attempt, marked with `request N`, so
a single step can show more than one `llm` line.

#### Worked example: three steps, two tool calls

```text
Step 1                                             11.79s
  llm · deepseek-v4-flash      2.12s  ttft 874ms  9.6k/209 tok  $0.001447
    thinking: Let me inspect the repository first…  (+4 more)
  tool · filesystem            4.03s
  tool · bash                  2.64s
Step 2                                             11.84s
  llm · deepseek-v4-flash      1.77s             10.2k/82 tok   $0.000152
  tool · bash                  7.20s
Step 3                                              7.82s
  llm · deepseek-v4-flash      7.78s  ttft 1.01s 10.9k/291 tok  $0.000302
    thinking: I have the result now.
    text:     The test suite passes.
```

| Row | What was paid for |
| :--- | :--- |
| Step 1 `llm` | Request 1: the model read the system prompt and the user message, thought, and asked for `filesystem` + `bash`. The 209 output tokens cover the tool-call JSON (and any thinking); the 9.6k input is the prompt plus the tool schemas. |
| Step 1 `tool` ×2 | Executions only: no tokens, no model cost. |
| Step 2 `llm` | Request 2: it carries the two tool results, so input grows to 10.2k. The 82 output tokens are one `bash` tool call. There is no `ttft` because only tool calls were streamed, no text or thinking. It also has no `thinking:`/`text:` preview, which is how the tree shows that this request produced nothing but the call. |
| Step 2 `tool` | Execution only. |
| Step 3 `llm` | Request 3: it carries the last tool result and produces the final text/thinking — 291 output tokens, and `ttft` is present again. |

Read the rows as "the request above, the executions below". A tool call never
incurs tokens itself; the model request that decided it does, and that usage is
always on the `llm` row. If you look only at the `tool` rows, the turn appears
token-free; add the `llm` rows and the real bill is there. The preview lines
answer the other question the row alone cannot: Step 1 and Step 3 produced
thinking/text, Step 2 produced only a tool call. The cost difference between
Step 1 and Step 3 despite similar input comes from prompt-cache hits, which are
stored but not printed (see above).

### Glossary

| Term | Meaning |
| :--- | :--- |
| **turn** | One user message and everything the agent did for it. Stored as one run. |
| **step** | One loop iteration: a model request plus the tool calls it requested. |
| **request** | One call to the provider. A retried request stays in the same step and takes the next `request_index`. |
| **retried** | Status of an attempt that failed before any output and was retried. The step and the run can still complete. |
| **compaction** | Trimming of old turns to fit the context budget. Recorded as its own span with token counts before and after. |
| **TTFT** | Time to first token: from sending the request until the first streamed delta. Includes queueing and prompt processing. Absent when no delta was streamed, for example a tool-call-only response. |
| **generation** | Time from the first token to the end of the stream. Stored in `generation_ms`; not printed in the tree. |
| **throughput** | `output_tokens ÷ (generation_ms ÷ 1000)`, in tokens per second. Derived, not stored. |
| **input tokens** | Every token sent to the model: system prompt, history, tool results, and the new message. |
| **output tokens** | Tokens the model produced, including reasoning tokens when the provider reports them. |
| **cached tokens** | Part of the input served from the provider's prompt cache; billed at `cache_hit` instead of `input`. |
| **cost** | `(miss × input + cached × cache_hit + output × output) ÷ 1,000,000`, using the price in force at request time. |
| **p50 / p95** | Median and 95th percentile. Half the runs are faster than p50; 95% are faster than p95. Computed over finished runs only. |
| **error rate** | Finished runs whose status is `error` or `interrupted`, divided by finished runs. |
| **unpriced** | A call spent tokens but recorded no cost, because its model had no price entry at request time. Cost is frozen when the request runs, so a call recorded before an entry was added also stays unpriced. |
| **`request_index`** | Position of a provider request inside a turn, counting retries. |
| **turn reference** | `<session prefix>.<type>.<turn>`, for example `4e7371da.cli.1`. Accepted by `obs run`. |

Run statuses: `running` (still open), `completed`, `error`, `cancelled` (aborted
or timed out), `truncated` (hit the output limit), `max_iterations` (hit the loop
cap), `interrupted` (the process died before the turn ended; set on the next
start).

### Counting

The chat header and the trace count the same unit:

| Where | Counts |
| :--- | :--- |
| Chat header `Agent timeline · N steps` | Loop iterations, from the engine's step events |
| Trace `step` | Loop iterations: one model request plus the tools it called |
| Trace `llm` / `tool` | The spans inside one step |

The expanded timeline labels every one of them with `── step N ──`, including
the first. The final answer is itself the last iteration, so it gets its own label
only when it is a separate step; a single-step reply shows one label above its
reasoning and none again above the answer. A run with
`4 steps · 4 model requests · 3 tool calls` therefore shows seven lines under the
run — four requests plus three tools — because each step renders its request and
the tools it called. The last step made no tool call because it produced the
final answer. History restored from a transcript written before step events
existed falls back to counting visible activity items.

A missing `ttft` on a specific step means no streaming delta was observed for
that request — the provider returned the whole message at once — so there is no
first-token timestamp to report. It is not a failed measurement.

While a run is open the header shows `running (in progress)` and a duration
marked "so far". The `summary` command counts only finished runs for run counts,
error rate, and latency percentiles, but includes the spend of a run in flight —
those tokens are already paid for.

### Content capture

By default every span stores what it saw: the request transcript, the thinking
block, the answer, and each tool's arguments and result. The raw JSON is capped
at `max_content_chars` (16384 by default) per blob.

The cap is applied by role. The `system` and `user` messages are stored whole,
assistant text is capped at half the budget, and tool results at an eighth —
tool output is the first thing sacrificed. When the transcript still does not
fit, the oldest messages are dropped and `dropped_messages` records how many.
The leading `system` message is never dropped and is shortened last: cutting it
would make the stored transcript look like a different request, while the newest
messages are kept because the last message is what drove it.

For an assistant response, its `content` and `thought` share the same budget: a
long thought is shortened first and keeps a readable prefix; it is dropped only
when the answer alone already fills the cap. The answer itself is shortened
last, so the visible reply survives even when the reasoning does not.

The system prompt is also captured on its own `system` span, so it stays readable
even when the transcript inside `llm.input_json` is trimmed.

Set `store_content: false` to keep only metadata and token counts. That is the
privacy opt-out: prompts, answers, and tool payloads are not written at all.
Traces recorded this way have no `thinking:`/`text:` previews in `obs run`, and
`--content` has nothing to print.

### Redaction

Even with content capture on, secrets are masked before the blob is stored or
exported. The built-in patterns cover bearer tokens, `sk-` keys, GitHub tokens,
and Slack tokens; a match becomes `[redacted]`. Redaction runs before
truncation, so a cut cannot leave half a credential behind.

Override the list with `observability.redact_patterns` (JSON) or
`OBS_REDACT_PATTERNS` (semicolon-separated, because commas are common inside a
regular expression). An explicit empty list — or `OBS_REDACT_PATTERNS=none` —
disables redaction. An invalid pattern is skipped with a log line instead of
disabling recording.

The shipped `config.json` template lists the four built-in patterns explicitly,
so a fresh agent and a reset both show what is active. Deleting the key falls
back to the same built-in list.

---

## 4. Cost

Cost is computed per model request, not per run. Switching models mid-session
bills each request at the price of the model that served it, and a run's total is
the sum of its `llm` spans.

### Tool pricing

Tools do not report token usage, so they are priced per **successful call**. A
failed or blocked call is not charged.

```json
"observability": {
  "tool_prices": {
    "tavily_search": {"per_call": 0.008}
  }
}
```

or `OBS_TOOL_PRICES="tavily_search=0.008;image_gen=0.04"` (semicolon-separated;
config wins when both are set). Tool spend is stored separately from model
spend: `runs.tool_cost_usd` and the `cost_usd` of `tool` spans. The summary
reports both totals, `Per tool` gets a COST column, and `Per trigger` gets a
TOOLS column.

```text
cost = (miss_tokens × input + cached_tokens × cache_hit + output_tokens × output) / 1_000_000
```

Prices are USD per one million tokens. `input` is the cache-miss price.

Tool calls are not priced. Only tokens cost money here, so there is no
per-tool price table to maintain.

### Peak and off-peak

Only models that declare both `off_peak` and `peak_windows` get time-based
pricing. Everything else is flat. DeepSeek ships with its published schedule:

```json
"deepseek-chat": {
  "input": 0.30, "cache_hit": 0.006, "output": 1.20,
  "off_peak": { "input": 0.15, "cache_hit": 0.003, "output": 0.60 },
  "peak_windows": ["Mon-Fri 01:00-04:00 UTC", "Mon-Fri 06:00-10:00 UTC"]
}
```

The fields hold the **peak** price; `off_peak` is used outside every window.
Windows are parsed as `Day-Day HH:MM-HH:MM UTC`. A window that cannot be parsed
is treated as peak, so an estimate can only be too high. Chinese public holidays
are not modelled; a holiday is billed as peak.

The built-in table covers `deepseek-flash`, `deepseek-chat`,
`deepseek-v4-flash` (same rates as the flash tier), and `deepseek-v4-pro`, plus
a few `gpt-*` and `gemini-*-flash` models. Anything else — preview Gemini ids
and Groq models, for example — has no entry. A model without an entry is
reported as `unpriced` in the CLI and costs `0`; add its numbers under
`model_prices` to start collecting cost.

Because `0` would otherwise read as "free", the summary marks how much could
not be priced: `cost_complete`, `unpriced_runs`, `unpriced_input_tokens`,
`unpriced_output_tokens`, and `unpriced_models`. When `cost_complete` is false,
`cost_usd` is a lower bound, and both the CLI summary and the session view say
so instead of printing a total that looks complete. Cost is computed when the
request runs, so a run recorded before a price entry existed stays at `0` even
after the entry is added; the flags describe recorded cost, not today's table.

### Overriding prices

Add or replace entries under `observability.model_prices` in `config.json`. The
built-in table is only a default; unknown models cost `0` and are logged once so
a missing entry cannot break a run.

---

## 5. Configuration

`config.json`:

```json
"observability": {
  "enabled": true,
  "retention_days": 30,
  "max_content_chars": 16384,
  "store_content": true,
  "redact_patterns": ["(?i)bearer\\s+[a-z0-9._~+/=-]{16,}"],
  "model_prices": { },
  "tool_prices": { }
}
```

`.env` (takes effect when the key is not set in `config.json`):

| Variable | Default | Purpose |
| :--- | :--- | :--- |
| `OBS_ENABLED` | `true` | Record traces at all. |
| `OBS_RETENTION_DAYS` | `30` | Delete older traces on open. `0` keeps everything. |
| `OBS_MAX_CONTENT_CHARS` | `16384` | Cap for every stored content blob. |
| `OBS_STORE_CONTENT` | `true` | Store prompts, answers, and tool payloads. |
| `OBS_REDACT_PATTERNS` | built-in list | Semicolon-separated regular expressions masked as `[redacted]`. Empty uses the built-in patterns; `none` disables redaction. |
| `OBS_TOOL_PRICES` | empty | Semicolon-separated `tool=price` pairs, for tools billed per successful call. |

`enabled: false` means no database is opened and no span is built.

`retention_days: 0` keeps everything; omit the field to use the default 30. The
same values are writable through the control plane (`PATCH /agents/:name/config`).

The control plane exposes the same section through `GET/PATCH
/agents/:name/config`, and `PATCH /agents/:name/config/reset` with
`{"observability": true}` restores the shipped defaults.

---

## 6. CLI

`observability` has a short alias, `obs`.

```bash
myaaw obs summary                      # last 7 days
myaaw observability summary            # last 7 days
myaaw observability summary --days 30
myaaw observability summary --json

myaaw obs sessions                                  # pick a session
myaaw obs session <conversation-id>                 # every turn of one session
myaaw obs runs --limit 20 --status error            # pick a run
myaaw obs runs --conversation <conversation-id>
myaaw obs run <run-id>
myaaw obs run <run-id> --content                     # full thinking and text
myaaw obs run 4e7371da.cli.1                       # turn reference
myaaw observability run run_1a2b3c4d5e6f7a8b

myaaw observability prune --days 90
myaaw observability prune --before 2026-01-01
myaaw observability prune --days 90 --vacuum    # compact the file too
```

`--before` is interpreted in the local timezone, matching `runs --from/--to`.
`--vacuum` needs exclusive access, so stop the agent first.

### `summary`

```text
Observability · last 7 day(s)
Runs 24 · errors 0 (0.0%)
Tokens input 1.7M · cached 678.0k · output 13.2k
Cost $0.0418 models · $0.0080 tools · latency p50 11.66s · p95 64.78s · TTFT p50 1.64s
Database 12.4 MB

Per step
  STEP   COUNT      AVG      P95       COST
     1      23    6.28s   11.79s    $0.0241
     2      14    5.13s   28.18s  $0.001024

Per model
  MODEL                          CALLS     INPUT    OUTPUT        COST
  deepseek-v4-flash                 67    953.4k     11.1k     $0.0368
  models/gemini-3.1-pro-previ…      16    534.7k       976           -

Per trigger
  TRIGGER       RUNS  ERRORS     INPUT    OUTPUT        COST       TOOLS
  user            21       0    1.5M      12.9k     $0.0418     $0.0080
  cron             2       0   93.2k       321           -           -
  subagent         1       0   193.8k     1.5k           -           -

Per tool
  TOOL                              CALLS  ERRORS       AVG        COST
  playwright_browser_evaluate          18       0     1.37s       $0.0080

Highlights
  most expensive   run_12f3a4dcf4369022
  slowest          run_97bb323faead5b0e
```

| Block | Columns |
| :--- | :--- |
| Header | finished runs, runs that ended in `error`/`interrupted` and their share, summed tokens (input includes cached), summed model cost, tool cost, p50/p95 turn duration, p50 TTFT, and the on-disk database size |
| `Per step` | step number, how many steps reached that depth, average and p95 step duration, total cost of requests in steps of that depth |
| `Per model` | model id (`…` when the column is full), number of requests, prompt and completion tokens, cost |
| `Per trigger` | trigger (`user`, `cron`, `subagent`, or `a2a`), runs, failed runs, prompt and completion tokens, model cost, tool cost |
| `Per tool` | tool name, calls, failed calls, average duration, cost of successful calls |
| `Highlights` | the most expensive and the slowest turn in the window |

Long names are truncated with `…` so columns never drift; the full name is in
`--json` and in `obs run`.

### `runs`

```text
Runs (12 total)
  RUN                    STARTED             CHANNEL    TRIGGER    STATUS    DURATION  COST
  run_20d0cdbfc59102e7   2026-10-28 22:52:20 cli        user       completed 4.63s     $0.002067
```

Columns are the run id, start time, channel, trigger (`user`, `cron`,
`subagent`, or `a2a`), status, duration, and cost. `COST` is model spend; tool
spend is shown in `obs run` and the summary. `--model`, `--trigger`, `--from`, and
`--to` narrow the list; the time flags accept `YYYY-MM-DD`, RFC3339, or unix
milliseconds, and a date-only `--to` covers the whole day. On a
terminal this becomes a picker instead. Each picker row shows the same identity
plus model, tokens, cost, and duration; arrow keys move, Enter opens, `q` or `Esc`
leaves, and typing filters. When stdout is not a terminal (a pipe or a script),
the plain table is printed.

### `sessions` and `session`

`sessions` lists conversations; on a terminal it is a picker, newest activity
first. Picking one opens `session <conversation-id>`:

```text
Session 4e7371da-… · 1 turn(s)
  TURN TYPE  TRIGGER  STARTED   RUN                    MODEL              STEPS DUR     TTFT   TOKENS      COST       STATUS
  1    cli   user     23:34:25  run_2240477b6447ff48   deepseek-v4-flash  5     14.35s  809ms  63.1k/732   $0.003927  completed

1 turn(s) · $0.003927 models
Open a turn:  myaaw obs run <run-id>  or  myaaw obs run 4e7371da.cli.1
All runs:     myaaw obs runs
```

| Column | Meaning |
| :--- | :--- |
| `TURN` | Position inside the session, oldest first |
| `TYPE` | Channel of that turn (`cli`, `telegram`, `discord`, `api`, `subagent`, …) |
| `TRIGGER` | What started the turn (`user`, `cron`, `subagent`, `a2a`, or `-` for runs recorded before this field existed) |
| `STARTED` | Start time; the session header carries the date |
| `RUN` | Run id, usable with `obs run` |
| `MODEL` | Model that served the turn (`…` when truncated) |
| `STEPS` | Loop iterations in that turn |
| `DUR` | Turn duration |
| `TTFT` | First-token time of the turn's first request |
| `TOKENS` | Input / output tokens |
| `COST` | Cost of the turn, or `unpriced`. This is model cost; tool spend is added to the footer and shown by `obs run`. |
| `STATUS` | Run status; failed turns also print an `error:` line below |

The footer's `4e7371da.cli.1` is the short turn reference: `obs run` accepts it
instead of the 20-character run id. The prefix must be unambiguous; if it matches
two sessions the command says so and asks for more characters.

### `run`

Prints the header and the span tree described in
[Reading the tree](#reading-the-tree). Each `llm` row is followed by a one-line
preview of the stored thinking and text, so a step that only made a tool call is
visibly different from one that also wrote an answer; `--content` prints the
full stored text. `--json` returns `{run, spans}` with every stored field,
including `generation_ms` and `cache_hit_tokens`, which the tree does not print.

---

## 7. HTTP API

All endpoints require the control plane bearer token.

| Method | Path | Purpose |
| :--- | :--- | :--- |
| `GET` | `/agents/:name/observability/summary?days=7` | Totals, percentiles, breakdowns, badges, cost-completeness flags, and `database_bytes`. |
| `GET` | `/agents/:name/observability/sessions?limit` | Sessions with turn count, tokens, and cost. |
| `GET` | `/agents/:name/observability/sessions/:id` | One session with every turn, **oldest first**, its step count, and its full spans (prompts, thinking, answers, tool payloads). Add `?spans=false` for metadata only. |
| `GET` | `/agents/:name/observability/runs?limit&offset&status&channel&conversation&model&trigger&from&to` | List runs, newest first. `trigger` is `user`, `cron`, or `subagent`. |
| `GET` | `/agents/:name/observability/runs/:id` | One run with flat `spans` and a nested `tree`. |
| `DELETE` | `/agents/:name/observability/runs?before=<unix ms>` | Delete runs; without `before` deletes everything. |

The session endpoint is what a trajectory UI needs: one request returns Turn 1..N,
and every turn carries its spans (`system`, `user`, `step`, `compaction`, `llm`,
`tool`, and nested subagent runs). See the mapping table in the
[control plane guide](../api/control-plane.md#mapping-the-response-to-a-trajectory-ui).

An agent that has never recorded a run returns an empty summary rather than an
error, so a dashboard can render before the first message.

Every chat response points at its trace: non-streaming chats return `run_id`,
and streaming chats carry the same field on the closing `done` event. It is
absent when observability is disabled.

---

## 8. Exporting with OpenTelemetry

Traces can be forwarded to any OTLP collector (Langfuse, Phoenix, Grafana, …).
The local database stays the source of truth; the export is a copy.

```bash
# gRPC (default port 4317)
OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317

# HTTP/protobuf (port 4318)
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
```

| Variable | Default | Purpose |
| :--- | :--- | :--- |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | empty | Empty disables export entirely. |
| `OTEL_EXPORTER_OTLP_INSECURE` | `true` | Disable when the collector terminates TLS. |
| `OTEL_SERVICE_NAME` | `myaaw` | Resource service name. |

An `http://` or `https://` endpoint selects the HTTP exporter; anything else
uses gRPC. Export runs on the writer goroutine with a batch processor, and a
5-second flush happens on shutdown, so a slow collector never delays a run.

Attributes follow the OpenTelemetry GenAI conventions:

| Attribute | Span |
| :--- | :--- |
| `gen_ai.operation.name` | `invoke_agent`, `chat`, `execute_tool` |
| `gen_ai.system` | `deepseek`, `openai`, `gemini`, … |
| `gen_ai.request.model` | model of the request |
| `gen_ai.usage.input_tokens`, `gen_ai.usage.output_tokens` | token counts |
| `gen_ai.tool.name` | tool name |
| `myaaw.run.id`, `myaaw.step.index`, `myaaw.cost_usd`, `myaaw.ttft_ms` | Myaaw-specific detail |

---

## 9. Where the code lives

| Component | File |
| :--- | :--- |
| Types, span kinds, truncation | `internal/obs/obs.go` |
| Trace events | `internal/obs/event.go` |
| Built-in price table | `internal/obs/defaults.go` |
| Pricing and peak windows | `internal/obs/pricer.go` |
| SQLite storage | `internal/obs/store.go` |
| Event → span tree, async writer | `internal/obs/recorder.go` |
| Engine event bridge + context trace | `internal/obs/agenttrace/bridge.go` |
| Aggregation | `internal/obs/summary.go` |
| OTLP export | `internal/obs/exporter.go` |
| Engine events and run id | `internal/agent/events.go`, `internal/agent/engine.go` |
| Tool-call id in context | `internal/agent/tool_context.go` |
| Wiring for chat channels | `internal/services/bot/service/observability.go` |
| Subagent child traces | `internal/agent/tools/subagent/subagent.go` |

The recorder consumes `obs.Event`, not engine types, so the agent loop has no
dependency on this package. The bot service translates `AgentEvent` into
`obs.Event` in one switch statement.

---

## Roadmap

Both phases are designed for, but not implemented. The schema already reserves
`checks` and the `judge_*` columns on `runs`.

### Phase 2 — Feedback

- 👍 / 👎 reactions in Telegram and Discord, stored against the run that produced
  the message.
- Implicit feedback: a follow-up correction, a retried question, or an abandoned
  conversation counts as a negative signal.
- A feedback column in `myaaw observability runs` and a filter in the control
  plane, so "show me the thumbs-down runs" is one query.

### Phase 3 — Eval

- `myaaw eval` runs a dataset of prompts offline against the current agent and
  stores each result as a run with `run_id` linking to its checks.
- LLM-as-judge fills `checks` and `runs.judge_score` / `judge_model` /
  `judge_cost_usd`, so scoring cost is visible next to the run it judged.
- A release gate compares judge scores and error rate against the previous
  build before shipping a prompt or model change.
- Regression checks for the failures we already know: a "spend claim" that the
  agent cannot back with a tool result, and an error the agent never handled.
