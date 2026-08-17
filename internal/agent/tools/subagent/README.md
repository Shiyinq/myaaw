# Sub-Agent Tool

The Sub-Agent tool allows the main agent to delegate long-running or parallel tasks to background worker agents. It is designed to orchestrate multiple independent tasks concurrently without blocking the main agent's execution loop.

## Function Name

`subagent`

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `tasks` | `array` | Yes | An array of task objects to be executed by sub-agents. |

### Task Object Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `name` | `string` | Yes | A short, descriptive name for the task (e.g., "Refactoring API", "Run tests"). |
| `instruction` | `string` | Yes | The detailed prompt/instruction for the sub-agent. Must be comprehensive. |
| `skills` | `array of strings` | No | List of skill names required for this task (must exist in `.myaaw/skills`). |

## Usage Examples

### Spawning a Single Task

```json
{
  "tasks": [
    {
      "name": "Audit Security",
      "instruction": "Run a security audit on the internal/auth package. Report any vulnerabilities.",
      "skills": []
    }
  ]
}
```

### Spawning Multiple Parallel Tasks (Batch Orchestration)

```json
{
  "tasks": [
    {
      "name": "Update Documentation",
      "instruction": "Read main.go and update README.md to reflect the new CLI flags.",
      "skills": ["docs_writer"]
    },
    {
      "name": "Run E2E Tests",
      "instruction": "Execute the end-to-end test suite and summarize the failures.",
      "skills": ["tester"]
    }
  ]
}
```

## How It Works

1. **Batch ID & Concurrency:** When called, the tool assigns a unique `Batch ID` to the group of tasks and spawns a separate, non-blocking *goroutine* for each task. The main agent receives an immediate response confirming the tasks have started.
2. **Execution Environment:** Each sub-agent runs an isolated instance of the `AgentProvider`. Sub-agents are intentionally restricted from spawning further nested sub-agents. 
3. **Execution Logs:** Sub-agents stream their step-by-step logs into the file system (usually `~/.myaaw/jobs/`).
4. **Heartbeat & IPC:** When a sub-agent completes its task (either successfully or with an error), it saves a final report and sends an HTTP POST request to the local `/heartbeat` endpoint. 
5. **Main Agent Notification:** The `/heartbeat` endpoint securely locks the state per-user (to prevent race conditions) and injects a `[SYSTEM TRIGGER: SUB-AGENT RESULT]` message into the main agent's conversation. This "wakes up" the main agent, allowing it to read the sub-agent's report, track batch progress, and summarize the findings back to the user.
