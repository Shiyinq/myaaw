---
name: Cashflow Tool
description: Manage personal finances (income, expense, analytics).
---

This skill allows you to manage cashflow transactions and view analytics.

## Usage
Run the `cashflow.py` script using the `bash` tool.

**Command:**
```bash
.venv/bin/python scripts/cashflow.py '<json_arguments>'
```

**Arguments:**
The script accepts a single JSON string argument.

**Parameters (JSON structure):**
- `action`: (Required) `add_transaction`, `get_transactions`, `update_transaction`, `delete_transaction`, `get_analytics`, `add_category`, `get_categories`.
- `user_id`: (Required for transactions/analytics).
- `transaction`: (Required for add/update) Object with `type` (income/expense), `amount`, `category` ({name}), `description`, `date`.
- `transaction_id`: (Required for update/delete).
- `date_range`: (Required for get_transactions/analytics) Object with `start` and `end`.
- `category`: (Required for add_category) Object with `name`.

**Examples:**

1. **Add Transaction:**
   **Bash Command**:
   `command`: `.venv/bin/python .teo/skills/cashflow/scripts/cashflow.py '{"action": "add_transaction", "user_id": "u1", "transaction": {"type": "expense", "amount": 50000, "category": {"name": "food"}, "description": "Lunch", "date": "2023-10-27T12:00:00Z"}}'`

2. **Get Analytics:**
   **Bash Command**:
   `command`: `.venv/bin/python .teo/skills/cashflow/scripts/cashflow.py '{"action": "get_analytics", "user_id": "u1", "date_range": {"start": "2023-10-01", "end": "2023-10-31"}}'`
