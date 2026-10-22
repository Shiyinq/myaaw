# Contact Tool

The Contact tool manages the agent's contact book and delegates work to other agents over the [Agent2Agent (A2A)](https://a2a-protocol.org/) protocol. Contacts are other agents, not sub-agents: they run in their own process, with their own tools, memory, and identity.

For setup recipes, see [A2A Interop Guide](../../../../docs/a2a-interop.md). For protocol internals, see [A2A Protocol](../../../../docs/a2a-protocol.md).

## Function Name

`contact`

## Actions

| Action | Purpose | Required parameters |
| :--- | :--- | :--- |
| `send` | Delegate a task and return the final answer | `contact`, `message` |
| `list` | Show configured contacts (tokens masked) | - |
| `get` | Show one contact plus its remote Agent Card skills | `contact` |
| `add` | Register a new contact | `contact`, `url` |

Optional parameters:

| Parameter | Used by | Description |
| :--- | :--- | :--- |
| `taskId` | `send` | Resume a task that returned `[input-required taskId=...]` |
| `token` | `add` | Bearer token for the remote agent |
| `description` | `add` | Human-readable label |

Aliases are accepted: `send_message`, `list_contacts`, `get_contact`, `add_contact`. A call without `action` that contains `contact` and `message` is treated as `send` for backwards compatibility.

## Usage Examples

### Delegate a task

```json
{
  "action": "send",
  "contact": "myfriend",
  "message": "Summarize the latest notes about the release"
}
```

### Discover contacts

```json
{"action": "list"}
```

```text
Contacts (1):
- myfriend: Agent myfriend di server lokal (port 8081) (http://localhost:8081/a2a) [token configured]
```

### Inspect one contact

```json
{"action": "get", "contact": "myfriend"}
```

```text
Contact: myfriend
URL: http://localhost:8081/a2a
Description: Agent myfriend di server lokal (port 8081)
Token: configured
Agent card: myfriend - Myaaw agent myfriend
Card signature: verified (short-key-id)
Skills: calendar (Calendar), converter (Converter), scraping (Scraping)
```

`Card signature` appears only when the remote card is signed. It is `verified` when the signing key is in this agent's `A2A_TRUSTED_KEYS`, otherwise `unverified`.

### Register a contact

Only use `add` when the user explicitly provides the URL.

```json
{
  "action": "add",
  "contact": "research",
  "url": "https://other-host/a2a",
  "token": "remote-token",
  "description": "Research agent"
}
```

### Resume an `input-required` task

When the remote agent asks for more information, `send` returns:

```text
[input-required taskId=7f2c...]
Tanggal berapa?
```

Answer with the same task ID:

```json
{
  "action": "send",
  "contact": "research",
  "taskId": "7f2c...",
  "message": "1 Oktober 2026"
}
```

## How `send` works

1. Looks up the contact in `~/.myaaw/<agent>/contacts.json`.
2. Sends `SendMessage` with the `A2A-Version: 1.0` header, a `ROLE_USER` message, and text parts.
3. Waits up to 10 minutes for the task to finish.
4. Returns, in order: artifact text, then `history` agent text.

State handling:

| State | Tool result |
| :--- | :--- |
| `TASK_STATE_COMPLETED` | Final text |
| `TASK_STATE_FAILED` | `Contact <name> failed: <reason>` |
| `TASK_STATE_CANCELED` | `Contact <name> canceled the task` |
| `TASK_STATE_INPUT_REQUIRED` / `TASK_STATE_AUTH_REQUIRED` | `[input-required taskId=<id>]` + question |

Resuming sends `message.taskId`, matching the A2A v1.0 `SendMessage` shape. A `taskId` that is no longer resumable (`-32004`) produces a readable error instead of creating a duplicate task.

## Contact file

```text
~/.myaaw/<agent>/contacts.json
```

```json
[
  {
    "name": "myfriend",
    "url": "http://localhost:8081/a2a",
    "token": "remote-agent-token",
    "description": "Agent myfriend"
  }
]
```

- `url` must be the JSON-RPC endpoint (`/a2a`), not the Agent Card URL.
- `token` is the remote agent's `A2A_TOKEN` or a named `A2A_CALLERS` token. It is never returned by `list`/`get` and is not echoed by `add`.
- The file is written atomically with mode `0600` by `add`.

The same registry can be managed through the control plane:

```text
GET    /agents/:name/contacts
POST   /agents/:name/contacts
DELETE /agents/:name/contacts/:contact
```

## Configuration on the remote agent

The remote agent decides how callers are identified:

```env
# ~/.myaaw/<remote>/.env
A2A_TOKEN=<shared-token>

# or named callers with scopes
A2A_CALLERS=alice:<token-alice>:delegate,bob:<token-bob>:read
```

If the remote agent's Agent Card is signed, this agent can verify it with:

```env
A2A_TRUSTED_KEYS=<remote-name>:<remote-public-key>
```

`get` then reports `Card signature: verified` (or `unverified`). Request authentication uses the bearer token from the contact entry; there is no per-request signing in A2A v1.0.

## Security notes

- Contacts are a trust boundary. Only add agents you control or trust.
- Signed Agent Cards are verified against `A2A_TRUSTED_KEYS`; pin the peer's public key to trust it.
- `list`/`get` mask tokens; `add` never echoes them.
- Use HTTPS for contacts across hosts.
- `add` lets the model register an endpoint. A prompt-injected model could point it at an attacker-controlled URL and send data there. If an agent must not manage contacts, block the tool with `AGENT_BLOCKED_TOOLS=contact` or make `contacts.json` read-only.

## Related

- [A2A Guide](../../../../docs/a2a.md)
- [A2A Protocol](../../../../docs/a2a-protocol.md)
- [A2A Interop Guide](../../../../docs/a2a-interop.md)
- [A2A Input Tool](../a2ainput/README.md)
