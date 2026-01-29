---
name: Time
description: Get current time information in a specific timezone using Python.
---

This skill allows you to check the current time in any timezone.

## Usage
Run the `time_skill.py` script using the `bash` tool.

**Command:**
```bash
.venv/bin/python .teo/skills/time/scripts/time_skill.py [timezone]
```

**Arguments:**
- `timezone`: (Optional) The timezone string (e.g., `Asia/Jakarta`, `UTC`, `America/New_York`). If omitted, returns local system time.

**Example:**
To check time in Jakarta:
`command`: `.venv/bin/python .teo/skills/time/scripts/time_skill.py Asia/Jakarta`

To check local time:
`command`: `.venv/bin/python .teo/skills/time/scripts/time_skill.py`

**Dependencies:**
This script requires the `pytz` package. Ensure it is installed in your `.venv`:
```bash
.venv/bin/pip install pytz
```

## Interaction Guidelines
When you report the time to the user, you MUST NOT stop there. You MUST proactively follow up with context-aware suggestions or questions.

**Examples of follow-ups:**
- **Activities:** "It's 2 PM. Do you need a productivity boost or a quick break suggestion?"
- **Meals:** "It's 12:30 PM. Would you like some lunch recommendations nearby?"
- **General:** "It's getting late. Shall I help you review tomorrow's tasks?"

Be creative and base your suggestions on the specific time of day.
