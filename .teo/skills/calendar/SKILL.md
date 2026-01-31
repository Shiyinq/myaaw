---
name: Calendar Tool
description: Manage user schedules (add, update, delete, search).
---

This skill allows you to manage calendar schedules.

## Usage
Run the `calendar.py` script using the `bash` tool.

**Command:**
```bash
.venv/bin/python scripts/calendar.py '<json_arguments>'
```

**Arguments:**
The script accepts a single JSON string argument.

**Parameters (JSON structure):**
- `action`: (Required) `add_schedule`, `update_schedule`, `delete_schedule`, `search_by_date`, `search_by_title`, `search_by_tags`.
- `user_id`: (Required) User ID.
- `schedule`: (Required for add/update) Object containing `title`, `description`, `start_time` (RFC3339), `end_time` (RFC3339), `tags`.
- `schedule_id`: (Required for delete).
- `date_range`: (Required for search_by_date) Object with `start` and `end`.
- `title`: (Required for search_by_title).
- `tags`: (Required for search_by_tags) List of strings.

**Examples:**

1. **Add Schedule:**
   **Bash Command**:
   `command`: `.venv/bin/python .teo/skills/calendar/scripts/calendar.py '{"action": "add_schedule", "user_id": "u1", "schedule": {"title": "Team Meeting", "description": "Weekly sync", "start_time": "2023-10-27T10:00:00Z", "end_time": "2023-10-27T11:00:00Z", "tags": ["work"]}}'`

2. **Search by Date:**
   **Bash Command**:
   `command`: `.venv/bin/python .teo/skills/calendar/scripts/calendar.py '{"action": "search_by_date", "user_id": "u1", "date_range": {"start": "2023-10-27T00:00:00Z", "end": "2023-10-27T23:59:59Z"}}'`
