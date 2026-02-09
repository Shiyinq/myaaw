# Bash Tool

The Bash tool allows the agent to execute shell commands within a controlled environment. It is useful for file manipulation, system information retrieval, and running scripts.

## Function Name

`bash`

## Parameters

| Parameter | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `command` | `string` | Yes | The bash command to execute. |
| `timeout` | `integer` | No | Timeout in seconds (default: 60). |

## Usage Examples

### List Files

```json
{
  "command": "ls -la"
}
```

### Check Disk Usage

```json
{
  "command": "df -h"
}
```

### Run a Script with Timeout

```json
{
  "command": "./myscript.sh",
  "timeout": 120
}
```

## Security

This tool includes a safety mechanism that blocks potentially dangerous commands, including:
- Deletion commands (`rm`)
- Privilege escalation (`sudo`, `su`)
- System control (`shutdown`, `reboot`)
- Disk operations (`mkfs`, `dd`)
- Network downloaders (`wget`, `curl`)

Attempting to run these commands will result in a security error.
