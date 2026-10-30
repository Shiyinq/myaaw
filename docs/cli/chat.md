# Chat

`myaaw chat` starts an interactive terminal UI or sends a single message.

## Usage

```bash
myaaw chat
myaaw chat -m "Summarize this repository"
myaaw --agent agent-b chat
```

## Flags

| Flag | Description |
| :--- | :--- |
| `-m`, `--message <text>` | Send one message and exit (non-interactive). |
| `--agent <name>` | Global flag. Skip the agent picker and use this agent. |

## Behavior

- **Interactive mode** (no `-m`) opens a TUI with streaming responses, tool call status, and a session picker.
- If no `--agent` is given, no message is sent, and more than one agent exists, chat shows an arrow-key picker before starting.
- **One-shot mode** (`-m`) prints the answer and exits. It uses streaming when `STREAM_RESPONSE` is enabled.
- The directory you launch `chat` from is registered as the active project for that session. The agent can read and edit files there directly, in addition to `~/.myaaw/<agent>/home`.
- Conversations are stored in the agent's SQLite database and can be reopened with `/sessions`.

## Slash commands

| Command | Action |
| :--- | :--- |
| `/sessions` | Open the session picker. |
| `/new` | Start a new chat session. |
| `/models` | Switch provider and model for this session. |
| `/exit` | Leave the chat. |

Type `/` to see suggestions. Tab completes a command.

## File attachments

You can reference files directly in the message:

```text
Explain @~/.myaaw/agent-b/home/MEMORY.md
What is wrong with /Users/me/project/main.go?
```

- `@path` and absolute or `~/` paths are recognized.
- Images (`.jpg`, `.jpeg`, `.png`, `.webp`, `.gif`) are read and attached as image input.
- Other files are converted into workspace links in the message so the agent can open them with its filesystem tools.
- Paths with spaces: use `@"path with spaces"`.

## Examples

```bash
# Interactive chat with a specific agent
myaaw --agent agent-b chat

# One-shot question
myaaw chat -m "What changed in the last commit?"

# One-shot with an image
myaaw chat -m "Describe this screenshot @~/Desktop/bug.png"

# Pipe-friendly answer
myaaw chat -m "Give me a one-line summary of AGENTS.md"
```

## Notes

- Interactive mode needs a terminal; scripts should use `-m`.
- `--agent` must point to an initialized agent. See [Multi-Agent](../agents/multi-agent.md).
- Provider and model selection is stored per agent. `provider` governs the default; `/models` changes the session.
- For an agent that is not running, `chat` still works: it loads the agent loop directly and does not require the daemon.

## Related

- [Multi-Agent Guide](../agents/multi-agent.md)
- [Providers](provider.md)
- [Agent Loop](../agents/agent-loop.md)
