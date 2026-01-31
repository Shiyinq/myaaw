---
name: Notes Tool
description: Manage user notes (create, read, update, delete, search).
---

This skill allows you to manage notes for a user.

## Usage
Run the `notes.py` script using the `bash` tool.

**Command:**
```bash
.venv/bin/python scripts/notes.py '<json_arguments>'
```

**Arguments:**
The script implementation accepts a single JSON string argument containing the parameters.

**Parameters (JSON structure):**
- `action`: (Required) The action to perform. Options: `GET`, `GET_DETAIL`, `POST`, `PUT`, `DELETE`, `SEARCH`, `GET_BY_DATE`.
- `user_id`: (Required) The ID of the user owning the note.
- `title`: (Required for POST, PUT, DELETE, GET_DETAIL) The title of the note.
- `content`: (Required for POST, PUT) The content of the note.
- `search`: (Required for SEARCH) The keyword to search for in titles and content.
- `start_date`: (Required for GET_BY_DATE) Start date (YYYY-MM-DD or RFC3339).
- `end_date`: (Required for GET_BY_DATE) End date (YYYY-MM-DD or RFC3339).

**Examples:**

1. **Create a Note (POST):**
   ```python
   # JSON Argument
   {"action": "POST", "user_id": "user123", "title": "Meeting Notes", "content": "Discuss project timeline..."}
   ```
   **Bash Command**:
   `command`: `.venv/bin/python .teo/skills/notes/scripts/notes.py '{"action": "POST", "user_id": "user123", "title": "Meeting Notes", "content": "Discuss project timeline..."}'`

2. **Get All Notes (GET):**
   **Bash Command**:
   `command`: `.venv/bin/python .teo/skills/notes/scripts/notes.py '{"action": "GET", "user_id": "user123"}'`

3. **Search Notes (SEARCH):**
   **Bash Command**:
   `command`: `.venv/bin/python .teo/skills/notes/scripts/notes.py '{"action": "SEARCH", "user_id": "user123", "search": "timeline"}'`
