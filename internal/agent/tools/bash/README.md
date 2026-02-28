# Bash Tool

The Bash tool allows the agent to execute shell commands within a controlled environment. It is useful for file manipulation, system information retrieval, and running scripts.

## Function Name

`bash`

## Parameters

| `command` | `string` | Yes | The bash command to execute. |
| `timeout` | `integer` | No | Timeout in seconds (default: 60). |
| `env` | `object` | No | Optional map of environment variables. |

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

```json
{
  "command": "./myscript.sh",
  "timeout": 120
}
```

### Run with Environment Variables

```json
{
  "command": "go build main.go",
  "env": {
    "GOOS": "linux",
    "GOARCH": "amd64"
  }
}
```

## Output Truncation

Command output is limited to **32KB** per call to maintain performance. If an output is truncated, a message will indicate the location of the **full log file** in `~/.myaaw/home/.logs/`. You can read the full output incrementally using the `filesystem` tool (with `read_file` and line ranges).


## Security

This tool includes a safety mechanism that blocks potentially dangerous commands, including:
- Deletion commands (`rm`)
- Privilege escalation (`sudo`, `su`)
- System control (`shutdown`, `reboot`)
- Disk operations (`mkfs`, `dd`)
- Network downloaders (`wget`, `curl`)

Attempting to run these commands will result in a security error.
