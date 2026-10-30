---
name: my-skill
description: What this skill does and when to use it. Include the words a user would say when they need it.
compatibility: Remove this line unless the skill needs extra packages, binaries, or network access.
---

# My Skill

One short paragraph: what problem this skill solves and what the agent should do
with it.

## Usage

Run the script with the `bash` tool. Replace `<skill-dir>` with the absolute
skill path shown in the `# Agent Skills` section of your prompt.

```bash
python3 <skill-dir>/scripts/main.py '{"action":"example"}'
```

## Arguments

| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `action` | string | Yes | Operation to perform |
| `payload` | object | No | Extra data for the action |

## Examples

```bash
python3 <skill-dir>/scripts/main.py '{"action":"example","payload":{"id":"42"}}'
```

Expected output:

```json
{"ok": true, "result": "..."}
```

## Errors

| Output | Meaning | What to do |
| :--- | :--- | :--- |
| `missing argument` | Required field not supplied | Ask the user for the value |
| `not found` | Target does not exist | Confirm the identifier with the user |

## Safety

Describe any action that changes data, spends money, or contacts an external
service, and when to ask for confirmation first.
