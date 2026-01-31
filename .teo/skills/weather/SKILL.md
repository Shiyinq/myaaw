---
name: Weather
description: Get the current weather in a given location.
---

# Weather Skill

Get the current weather in a given location.

## Usage

The skill is executed via a Python script: `.teo/skills/weather/scripts/weather.py`.
It accepts a JSON string as the first argument calling the tool.

### Arguments

The input JSON should contain the following fields:

| Field | Type | Description | Required |
| :--- | :--- | :--- | :--- |
| `location` | string | The city and state, e.g. `San Francisco, CA`. | Yes |
| `unit` | string | `celsius` or `fahrenheit`. | No |

### Example

```bash
.venv/bin/python .teo/skills/weather/scripts/weather.py '{"location": "London", "unit": "celsius"}'
```
