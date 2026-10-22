# A2A Auth Request Tool

The A2A Auth Request tool lets an agent that is **serving** an A2A task ask the caller to authenticate or authorize before the task can continue. The task pauses in `auth-required` state until the caller resumes it with credentials.

`a2a_auth_request` is the authentication counterpart of the [A2A Input tool](../a2ainput/README.md):

| | A2A Input tool | A2A Auth Request tool |
| :--- | :--- | :--- |
| Purpose | Ask for missing information | Ask for authentication |
| Task state | `TASK_STATE_INPUT_REQUIRED` | `TASK_STATE_AUTH_REQUIRED` |
| Marker file | `<taskDir>/input-required.json` | `<taskDir>/auth-required.json` |
| Caller action | Reply with the answer | Authenticate, then resume the task |

## Function Name

`a2a_auth_request`

## Parameters

| Parameter | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `message` | `string` | Yes | What the caller must do, e.g. which credential or scope is required. |

## Usage Example

```json
{
  "message": "Provide a bearer token with the delegate scope to continue."
}
```

The tool returns:

```text
Authentication request recorded. Stop working and wait for the caller to authenticate.
```

The agent should stop after this call. The task is marked `auth-required` and the caller resumes it with the same task ID after authenticating.

## How It Works

1. The A2A service sets `MYAAW_A2A_TASK_DIR` for the duration of the task run.
2. `a2a_auth_request` writes `<taskDir>/auth-required.json` containing the message.
3. When the run stops, the service reads the file, removes it, and sets the task status:

```json
{
  "status": {
    "state": "TASK_STATE_AUTH_REQUIRED",
    "message": {
      "role": "ROLE_AGENT",
      "parts": [{"text": "Provide a bearer token with the delegate scope to continue."}]
    }
  }
}
```

4. The caller authenticates and resumes with the **same task ID**:
   - direct A2A: `SendMessage` with `message.taskId`
   - Myaaw caller: `contact` with `"taskId": "<task-id>"`

## Availability

The tool only works while an A2A task is running. Outside that context it returns:

```text
Error: a2a_auth_request is only available while handling an A2A task
```

## Limits

- One pending auth request per run. If the tool is called more than once, the last message wins.
- The caller must know the task ID to resume.
- Only the caller that created the task may resume it.
- A task can be interrupted multiple times; each interruption records the latest request.

## Related

- [A2A Guide](../../../../docs/a2a.md)
- [A2A Protocol](../../../../docs/a2a-protocol.md)
- [A2A Interop Guide](../../../../docs/a2a-interop.md)
- [A2A Input Tool](../a2ainput/README.md)
- [Contact Tool](../contact/README.md)
