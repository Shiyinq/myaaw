# A2A Input Tool

The A2A Input tool lets an agent that is **serving** an A2A task ask the caller for missing information. The task pauses in `input-required` state until the caller answers.

This is the mirror image of the [Contact tool](../contact/README.md):

| | Contact tool | A2A Input tool |
| :--- | :--- | :--- |
| Direction | Caller → remote agent | Serving agent → caller |
| Purpose | Send a task | Ask a question |
| Effect | HTTP request to `/a2a` | Writes a marker file for the A2A service |
| Availability | Any time | Only while handling an A2A task |

## Function Name

`a2a_input`

## Parameters

| Parameter | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `question` | `string` | Yes | The question for the caller. |

## Usage Example

```json
{
  "question": "Tanggal berapa laporan yang harus dibuat?"
}
```

The tool returns:

```text
Input request recorded. Stop working and wait for the caller's answer.
```

The agent should stop after this call. The task will be marked `input-required`, and the caller resumes it with the same task ID.

## How It Works

1. The A2A service sets `MYAAW_A2A_TASK_DIR` for the duration of the task run.
2. `a2a_input` writes `<taskDir>/input-required.json` containing the question.
3. When the run stops, the service reads the file, removes it, and sets the task status:

```json
{
  "status": {
    "state": "TASK_STATE_INPUT_REQUIRED",
    "message": {
      "role": "ROLE_AGENT",
      "parts": [{"text": "Tanggal berapa laporan yang harus dibuat?"}]
    }
  }
}
```

4. The caller resumes with the **same task ID**:
   - direct A2A: `SendMessage` with `message.taskId` set to the task id
   - Myaaw caller: `contact` with `"taskId": "<task-id>"`

## Availability

The tool only works while an A2A task is running. Outside that context it returns:

```text
Error: a2a_input is only available while handling an A2A task
```

This is why it is a separate built-in tool instead of an action inside `contact`:

- it is only meaningful on the receiving side of a task;
- it has no network side effects;
- agents can enable or disable it independently of outbound contact calls.

## Limits

- One pending question per run. If the tool is called more than once, the last question wins.
- The caller must know the task ID to resume. Myaaw surfaces it as `[input-required taskId=...]`.
- Only the caller that created the task may resume it.
- If the caller never answers, the task stays `input-required` indefinitely. It is not expired automatically.

## Related

- [A2A Guide](../../../../docs/a2a.md)
- [A2A Protocol](../../../../docs/a2a-protocol.md)
- [A2A Interop Guide](../../../../docs/a2a-interop.md)
- [Contact Tool](../contact/README.md)
