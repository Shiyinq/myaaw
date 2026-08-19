# Agent Skills Guide

Myaaw provides a dynamic, lightweight, and extensible **Agent Skills System**. Skills allow AI agents to learn new domain-specific capabilities, automate complex workflows, and execute scripts on demand without polluting the LLM's active function-calling schema space.

Each skill is self-contained within its own directory and defined by a `SKILL.md` specification file. When needed, Myaaw agents dynamically read skill documentation using filesystem tools and execute corresponding scripts in an isolated environment.

---

## Table of Contents
- [Overview & Architecture](#overview--architecture)
- [Comparison: Skills vs. Tools vs. MCP](#comparison-skills-vs-tools-vs-mcp)
- [Skill Directory Structure](#skill-directory-structure)
- [The `SKILL.md` Specification](#the-skillmd-specification)
  - [YAML Frontmatter Contract](#yaml-frontmatter-contract)
  - [Markdown Body Structure](#markdown-body-structure)
- [How Myaaw Discovers & Executes Skills](#how-myaaw-discovers--executes-skills)
  - [1. Discovery & Prompt Injection](#1-discovery--prompt-injection)
  - [2. On-Demand Inspection (Filesystem)](#2-on-demand-inspection-filesystem)
  - [3. Execution (Bash / Python)](#3-execution-bash--python)
- [Managing Skills (Enable / Disable)](#managing-skills-enable--disable)
- [Step-by-Step: Creating a New Skill](#step-by-step-creating-a-new-skill)
  - [Step 1: Create the Skill Directory](#step-1-create-the-skill-directory)
  - [Step 2: Implement the Execution Script](#step-2-implement-the-execution-script)
  - [Step 3: Write the `SKILL.md` Documentation](#step-3-write-the-skillmd-documentation)
  - [Step 4: Verify and Test with Myaaw](#step-4-verify-and-test-with-myaaw)
- [Practical Skill Examples](#practical-skill-examples)
  - [Example 1: External API Integration (Weather / Search)](#example-1-external-api-integration-weather--search)
  - [Example 2: User Storage & Database Operations (Notes / Cashflow)](#example-2-user-storage--database-operations-notes--cashflow)
  - [Example 3: Unit Transformation & Utilities (Converter)](#example-3-unit-transformation--utilities-converter)
  - [Example 4: Node.js / JavaScript Skill](#example-4-nodejs--javascript-skill)
  - [Example 5: Shell / Bash Script Skill](#example-5-shell--bash-script-skill)
- [Subagent Skill Delegation](#subagent-skill-delegation)
  - [Targeting Specific Skills for Subagents](#targeting-specific-skills-for-subagents)
  - [Skill Validation Rules](#skill-validation-rules)
- [Best Practices & Safety Guidelines](#best-practices--safety-guidelines)
- [Troubleshooting & FAQs](#troubleshooting--faqs)

---

## Overview & Architecture

Unlike traditional tool definitions that inject large JSON schemas for every tool into every single LLM prompt, Myaaw uses a **two-tier cognitive workflow**:

1. **Compact Index in System Prompt**: At startup/request time, Myaaw scans `~/.myaaw/skills/` and injects only a lightweight summary (name and description) into the agent's system prompt.
2. **On-Demand Loading & Execution**: When a user's task requires a skill, the agent reads the full `SKILL.md` via the `filesystem` tool, understands the parameters, and runs the script using `bash` or `execute_python`.

```
                        ┌──────────────────────────────┐
                        │      ~/.myaaw/skills/        │
                        │   ├── weather/SKILL.md       │
                        │   ├── notes/SKILL.md         │
                        │   └── tavily/SKILL.md        │
                        └──────────────┬───────────────┘
                                       │
                         Scans Frontmatter (name, desc)
                                       │
                                       ▼
 ┌───────────────────────────────────────────────────────────────────────────┐
 │                            Agent System Prompt                            │
 │  # Agent Skills                                                           │
 │  - Weather (~/.myaaw/skills/weather): Get current weather in a location   │
 │  - Notes (~/.myaaw/skills/notes): Manage user notes (create, search, etc) │
 └─────────────────────────────────────┬─────────────────────────────────────┘
                                       │
                               User asks for task
                                       │
                                       ▼
 ┌───────────────────────────────────────────────────────────────────────────┐
 │                             ReAct Loop Action                             │
 │                                                                           │
 │ 1. Agent reads ~/.myaaw/skills/<skill>/SKILL.md using `filesystem` tool   │
 │ 2. Agent parses argument format & JSON requirements                       │
 │ 3. Agent executes script via `bash`:                                      │
 │    .venv/bin/python ~/.myaaw/skills/<skill>/scripts/<name>.py '<json>'    │
 │ 4. Script prints JSON / output to stdout -> Agent returns final answer    │
 └───────────────────────────────────────────────────────────────────────────┘
```

---

## Comparison: Skills vs. Tools vs. MCP

| Feature | Built-in Tools | Custom CLI Tools | MCP Servers | Agent Skills |
| :--- | :--- | :--- | :--- | :--- |
| **Location** | Compiled in Go Core | `~/.myaaw/tools/` or `tools.json` | `tools.json` stdio | `~/.myaaw/skills/<name>/` |
| **LLM Exposure** | Native JSON schema | OpenAI Function Schema | Sanitized MCP Schema | Brief summary in System Prompt |
| **Token Footprint** | Constant per tool | Moderate (all active schemas) | High (registers all tools) | **Minimal** (1 line summary per skill) |
| **Invocation** | Direct Tool Call | Direct Tool Call | Direct Tool Call | 2-step: Read `SKILL.md` + Execute via `bash` |
| **Hot-Reloadable** | Recompile required | Yes (`fsnotify`) | Yes (`fsnotify`) | **Instant** (file-read per execution) |
| **Best For** | Core OS operations (`bash`, `fs`) | Standard single-purpose tools | Ecosystem integrations (Playwright, SQLite) | Complex workflows, multi-step scripts, multi-tenant databases |

---

## Skill Directory Structure

All skills live in `~/.myaaw/skills/` (or `.myaaw/skills/` within the workspace during development).

A standard skill directory contains:

```
~/.myaaw/skills/<skill_name>/
├── SKILL.md                 # REQUIRED: Metadata header + full agent documentation
├── scripts/                 # RECOMMENDED: Directory for execution scripts
│   └── <skill_name>.py      # Main Python/Node/Bash executable script
├── requirements.txt         # OPTIONAL: Python package dependencies
└── templates/ or data/      # OPTIONAL: Additional assets, templates, or local schemas
```

### Directory Naming Conventions
- Directory names must be lowercase, alphanumeric, with dashes or underscores (e.g., `weather`, `tavily`, `cashflow`, `github_issues`).
- The directory name corresponds to the path Myaaw uses for discovery: `~/.myaaw/skills/<dir_name>`.

---

## The `SKILL.md` Specification

`SKILL.md` is the brain of your skill. It consists of two essential parts:
1. **YAML Frontmatter** (delimited by `---`)
2. **Markdown Documentation Body**

### YAML Frontmatter Contract

The frontmatter must be at the very top of `SKILL.md` and contains the metadata indexed by Myaaw:

```yaml
---
name: Skill Name
description: A concise 1-2 sentence description explaining what the skill does and when the agent should use it.
---
```

| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `name` | string | **Yes** | Human-readable title of the skill (e.g., `Weather`, `Cashflow`, `Notes`). |
| `description` | string | **Yes** | Clear summary of capabilities. The LLM uses this to determine if the skill is relevant to the user query. |
| `enabled` | boolean | No | Set to `false` to disable the skill without deleting it. Defaults to `true`. Note: `~/.myaaw/skills/skills.json` takes priority when it explicitly lists the skill. |

> **Important**: Keep the `description` clear and keyword-rich so the LLM recognizes when to use it (e.g., mention `"manage personal finances"`, `"search the web"`, `"convert units"`).

---

### Markdown Body Structure

The body of `SKILL.md` instructs the agent on how to call the skill script. Follow this standard template:

````markdown
---
name: ExampleSkill
description: Summary of what ExampleSkill does.
---

# Example Skill

Detailed explanation of what this skill does and its capabilities.

## Usage

Run the script using the `bash` tool.

**Command:**
```bash
.venv/bin/python ~/.myaaw/skills/example_skill/scripts/example.py '<json_arguments>'
```

### Arguments

The script accepts a single JSON string argument:

| Field | Type | Description | Required | Default |
| :--- | :--- | :--- | :--- | :--- |
| `action` | string | Operation to perform (`create`, `get`, `delete`) | Yes | - |
| `user_id` | string | User ID from system `# User Info` | Yes | - |
| `payload` | object | Data payload for the action | No | `{}` |

### Examples

#### 1. Perform an action
**Bash Command:**
```bash
.venv/bin/python ~/.myaaw/skills/example_skill/scripts/example.py '{"action": "create", "user_id": "12345", "payload": {"title": "Task 1"}}'
```

### Interaction Guidelines
Instructions for the agent on how to format or present the output to the user.
````

---

## How Myaaw Discovers & Executes Skills

### 1. Discovery & Prompt Injection

During conversation setup, Myaaw calls `skills.GetSkillsInstruction()`, which reads all directories inside `~/.myaaw/skills/` (skipping any disabled via `skills.json`), parses the `SKILL.md` frontmatter, and formats them:

```markdown
# Agent Skills

You have access to the following skills. You can use the 'filesystem' to read file skill and 'bash' tool to execute script if available.
To use a skill, you must first read its documentation in the `~/.myaaw/skills/<skill-name>/SKILL.md` file. 
If skills need user_id, userId, or userID, you must use User ID from the User Info. 

- **Weather** (`~/.myaaw/skills/weather`): Get the current weather in a given location.
- **Notes** (`~/.myaaw/skills/notes`): Manage user notes (create, read, update, delete, search).
- **Cashflow** (`~/.myaaw/skills/cashflow`): Manage personal finances (income, expense, analytics).
```

### 2. On-Demand Inspection (Filesystem)

When the user asks something relevant (e.g., *"What is the weather in Tokyo?"*), the agent identifies `Weather` in its skills list, calls `filesystem` to read `~/.myaaw/skills/weather/SKILL.md`, and learns the exact parameters.

### 3. Execution (Bash / Python)

The agent invokes the skill script with JSON payload via the `bash` tool:

```bash
.venv/bin/python ~/.myaaw/skills/weather/scripts/weather.py '{"location": "Tokyo", "unit": "celsius"}'
```

The script processes the input and prints results to `stdout`. The agent formats the output into a natural conversational response for the user.

---

## Managing Skills (Enable / Disable)

You can enable or disable individual skills **without deleting them** by creating `~/.myaaw/skills/skills.json`. Each key is a skill **directory name** (`~/.myaaw/skills/<dir_name>`) mapped to a boolean:

```json
{
  "weather": true,
  "tavily": false,
  "notes": true
}
```

### Rules

| Situation | Behavior |
| :--- | :--- |
| `"<skill>": true` | **Enabled** — skill appears in the agent's system prompt |
| `"<skill>": false` | **Disabled** — skill is hidden from the agent's system prompt |
| Omitted (not listed) | Falls back to the skill's `SKILL.md` frontmatter `enabled` field (default: **enabled**) |

> **Note**: Keys are the skill **directory names** (e.g. `tavily`), not the display `name` from the `SKILL.md` frontmatter (e.g. `Tavily`).

### Alternative: Frontmatter `enabled` Field

You can also disable a skill directly in its `SKILL.md` frontmatter — useful when you don't want a separate config file or when shipping skills:

```yaml
---
name: Tavily
description: Search the web using Tavily.
enabled: false
---
```

**Priority**: when `skills.json` explicitly lists a skill, it wins over the frontmatter. The frontmatter only applies to skills not listed in `skills.json`.

### Hot-Reload & Logs

Changes to `skills.json` take effect immediately for new conversations — **no restart required** — because the config is read fresh every time `skills.GetSkillsInstruction()` runs (at the start of each conversation or sub-agent).

Myaaw also watches the skills directory (like it does for tools) and logs the resulting skill state to its own log file, **separate from the tools log**:

```bash
tail -f ~/.myaaw/logs/skills.log
```

**Example Log Output:**
```log
2026/08/18 10:15:03 Total active skills: 8 | Skills: [calendar cashflow converter notes scraping tavily time weather]
2026/08/18 10:15:44 Skills configuration changed, reloading...
2026/08/18 10:15:44 Skill 'tavily' disabled by config
2026/08/18 10:15:44 Total active skills: 7 | Skills: [calendar cashflow converter notes scraping time weather]
```

At startup, the active skill set is logged once; after every change to `skills.json` (or the skills directory), the skills disabled by config and the new active set are logged.

### Disabled Skill Validation

If a sub-agent task references a disabled skill via its `skills` field, the `subagent` tool rejects the task with a clear error:

```
Error in task 'research': Skill 'tavily' is disabled. Enable it in ~/.myaaw/skills/skills.json
```

> [!NOTE]
> Disabling a skill hides it from the agent's prompt, but skills are executed through the `bash`/`filesystem` tools, so a user or agent that already knows the script path can still run it. For full removal, delete the skill directory.

---

## Step-by-Step: Creating a New Skill

Let's build a complete skill: **`currency_converter`** that fetches exchange rates and converts currencies.

### Step 1: Create the Skill Directory

```bash
mkdir -p ~/.myaaw/skills/currency_converter/scripts
```

---

### Step 2: Implement the Execution Script

Create `~/.myaaw/skills/currency_converter/scripts/converter.py`:

```python
#!/usr/bin/env python3
import sys
import json
import urllib.request
import urllib.error

def convert_currency(amount, from_curr, to_curr):
    from_curr = from_curr.upper()
    to_curr = to_curr.upper()
    
    if from_curr == to_curr:
        return {"amount": amount, "from": from_curr, "to": to_curr, "result": amount, "rate": 1.0}
    
    url = f"https://api.exchangerate-api.com/v4/latest/{from_curr}"
    
    try:
        req = urllib.request.Request(url, headers={"User-Agent": "Myaaw-Agent"})
        with urllib.request.urlopen(req, timeout=10) as response:
            if response.status != 200:
                return {"error": f"API returned status {response.status}"}
            
            data = json.loads(response.read().decode('utf-8'))
            rates = data.get("rates", {})
            
            if to_curr not in rates:
                return {"error": f"Unsupported target currency: {to_curr}"}
            
            rate = rates[to_curr]
            result = amount * rate
            return {
                "amount": amount,
                "from": from_curr,
                "to": to_curr,
                "rate": rate,
                "result": round(result, 4),
                "date": data.get("date")
            }
    except urllib.error.URLError as e:
        return {"error": f"Failed to connect to currency API: {str(e)}"}
    except Exception as e:
        return {"error": str(e)}

def main():
    if len(sys.argv) < 2:
        print(json.dumps({"error": "No arguments provided. Expected JSON string as argument 1."}))
        sys.exit(1)

    try:
        params = json.loads(sys.argv[1])
    except json.JSONDecodeError:
        print(json.dumps({"error": "Invalid JSON argument."}))
        sys.exit(1)

    amount = float(params.get("amount", 1.0))
    from_curr = params.get("from_currency", "USD")
    to_curr = params.get("to_currency", "IDR")

    output = convert_currency(amount, from_curr, to_curr)
    print(json.dumps(output, indent=2))

if __name__ == "__main__":
    main()
```

Make sure the script has execute permissions:
```bash
chmod +x ~/.myaaw/skills/currency_converter/scripts/converter.py
```

---

### Step 3: Write the `SKILL.md` Documentation

Create `~/.myaaw/skills/currency_converter/SKILL.md`:

````markdown
---
name: Currency Converter
description: Real-time currency exchange rate conversion between global currencies (USD, IDR, EUR, JPY, GBP, etc.).
---

# Currency Converter Skill

Convert amounts between different fiat currencies using real-time exchange rates.

## Usage

Execute the script via Python using the `bash` tool.

**Command:**
```bash
.venv/bin/python ~/.myaaw/skills/currency_converter/scripts/converter.py '<json_arguments>'
```

### Arguments

The input JSON string must contain:

| Field | Type | Description | Required | Default |
| :--- | :--- | :--- | :--- | :--- |
| `amount` | number | Amount of money to convert. | Yes | `1.0` |
| `from_currency` | string | 3-letter currency code to convert from (e.g. `USD`, `IDR`, `EUR`, `JPY`). | Yes | `USD` |
| `to_currency` | string | 3-letter currency code to convert to (e.g. `IDR`, `USD`, `SGD`). | Yes | `IDR` |

### Examples

**Convert 50 USD to IDR:**
```bash
.venv/bin/python ~/.myaaw/skills/currency_converter/scripts/converter.py '{"amount": 50, "from_currency": "USD", "to_currency": "IDR"}'
```

**Convert 1,000,000 IDR to JPY:**
```bash
.venv/bin/python ~/.myaaw/skills/currency_converter/scripts/converter.py '{"amount": 1000000, "from_currency": "IDR", "to_currency": "JPY"}'
```

## Guidelines
- Always format currencies cleanly for the user with thousand separators.
- Mention the exchange rate and date returned by the tool.
````

---

### Step 4: Verify and Test with Myaaw

1. **Test the script manually in terminal**:
   ```bash
   .venv/bin/python ~/.myaaw/skills/currency_converter/scripts/converter.py '{"amount": 100, "from_currency": "USD", "to_currency": "IDR"}'
   ```

2. **Test via Myaaw Chat / Bot**:
   Ask Myaaw in Telegram, Discord, or Terminal:
   > *"Convert 150 USD to IDR"*

   Myaaw will:
   - Identify `Currency Converter` in its skill list.
   - Read `~/.myaaw/skills/currency_converter/SKILL.md`.
   - Run the command via `bash`.
   - Deliver the final converted amount!

---

## Practical Skill Examples

### Example 1: External API Integration (Weather / Search)

Skills that interact with external web APIs (like Tavily, Jina Scraping, wttr.in) should handle network timeouts and return clean JSON.

**File:** `~/.myaaw/skills/weather/SKILL.md`
```markdown
---
name: Weather
description: Get the current weather in a given location.
---

# Weather Skill

Get the current weather and forecasts for any city or region.

## Usage

```bash
.venv/bin/python ~/.myaaw/skills/weather/scripts/weather.py '{"location": "Jakarta", "unit": "celsius"}'
```

### Arguments
- `location` (string, Required): City name or coordinates.
- `unit` (string, Optional): `celsius` (default) or `fahrenheit`.
```

---

### Example 2: User Storage & Database Operations (Notes / Cashflow)

Skills can persist multi-tenant user data using the `User ID` provided dynamically in `# User Info`.

**File:** `~/.myaaw/skills/notes/scripts/notes.py` (Pattern)
```python
import sys
import json
import os

DATA_DIR = os.path.join(os.path.expanduser("~"), ".myaaw", "database", "notes")

def ensure_user_dir(user_id):
    path = os.path.join(DATA_DIR, str(user_id))
    os.makedirs(path, exist_ok=True)
    return path

def save_note(user_id, title, content):
    user_dir = ensure_user_dir(user_id)
    file_path = os.path.join(user_dir, f"{title}.json")
    with open(file_path, "w") as f:
        json.dump({"title": title, "content": content}, f)
    return {"status": "success", "message": f"Note '{title}' saved."}

def main():
    params = json.loads(sys.argv[1])
    action = params.get("action")
    user_id = params.get("user_id") # Passed by agent from # User Info
    
    if action == "POST":
        print(json.dumps(save_note(user_id, params["title"], params["content"])))
```

**Key Principle**:
Always isolate user storage by `~/.myaaw/database/<skill_name>/<user_id>/` so different users on Telegram/Discord/CLI do not cross data.

---

### Example 3: Unit Transformation & Utilities (Converter)

Stateless skills that perform unit conversions, math calculations, data parsing, or regex formatting.

**File:** `~/.myaaw/skills/converter/SKILL.md`
```markdown
---
name: Converter
description: Convert values between various units (distance, mass, volume, temperature, speed).
---

## Usage
```bash
.venv/bin/python ~/.myaaw/skills/converter/scripts/converter.py '{"value": 100, "from_unit": "celsius", "to_unit": "fahrenheit"}'
```
```

---

### Example 4: Node.js / JavaScript Skill

Skills are not restricted to Python. You can write them in JavaScript/TypeScript run with `node` or `bun`:

**File:** `~/.myaaw/skills/uuid_generator/scripts/generate.js`
```javascript
#!/usr/bin/env node
const crypto = require('crypto');

const args = process.argv.slice(2);
let count = 1;

if (args[0]) {
  try {
    const params = JSON.parse(args[0]);
    count = params.count || 1;
  } catch (e) {}
}

const uuids = Array.from({ length: count }, () => crypto.randomUUID());
console.log(JSON.stringify({ uuids }));
```

**File:** `~/.myaaw/skills/uuid_generator/SKILL.md`
```markdown
---
name: UUID Generator
description: Generate cryptographically secure UUID v4 strings.
---

## Usage
```bash
node ~/.myaaw/skills/uuid_generator/scripts/generate.js '{"count": 5}'
```
```

---

### Example 5: Shell / Bash Script Skill

For native system diagnostics:

**File:** `~/.myaaw/skills/disk_usage/scripts/disk.sh`
```bash
#!/bin/bash
df -h | awk 'NR==1 || /^\/dev/' | jq -R -s -c 'split("\n")[:-1] | map(split(" ") | select(length > 0))'
```

---

## Subagent Skill Delegation

Myaaw includes autonomous background subagents via the `subagent` tool. When launching a subagent, you can instruct it to focus on specific skills.

### Targeting Specific Skills for Subagents

When delegating a task to a subagent:

```json
{
  "tasks": [
    {
      "description": "Fetch weather in Paris and Tokyo and compare their temperatures.",
      "skills": "weather,converter"
    }
  ]
}
```

### Skill Validation Rules
1. When `skills` is specified in a `subagent` call, Myaaw validates each skill name against `~/.myaaw/skills/<skill_name>`.
2. If any skill directory does not exist, the subagent call fails immediately with:
   `Skill '<name>' not found. Please ensure it is installed in ~/.myaaw/skills/`
3. If valid, Myaaw automatically prioritizes those skills in the subagent's ReAct system prompt.

---

## Best Practices & Safety Guidelines

1. **Always Use the Virtual Environment (`.venv`)**:
   - In Myaaw, Python dependencies must run inside `.venv`.
   - In `SKILL.md`, always specify `.venv/bin/python` or standard python paths:
     ```bash
     .venv/bin/python ~/.myaaw/skills/<skill>/scripts/<script>.py '<json>'
     ```
   - Before running pip installs for new skill libraries:
     ```bash
     source .venv/bin/activate
     pip install -r ~/.myaaw/skills/<skill>/requirements.txt
     ```

2. **JSON Argument Escaping**:
   - Always accept parameters as a single JSON string in `sys.argv[1]`.
   - Wrap the JSON string in single quotes (`'...'`) in examples to prevent shell quoting issues.

3. **Output Structured JSON to `stdout`**:
   - Output clean JSON (or well-formatted text) on `stdout`.
   - Log debug info to `stderr` so it doesn't pollute the LLM tool result.

4. **Multi-Tenant User Isolation**:
   - When a skill accesses user files or databases, require `user_id`.
   - Note in `SKILL.md`: `user_id: (Required) User ID from User Info`.
   - Store user data in `~/.myaaw/database/<skill>/<user_id>/`.

5. **Graceful Error Handling**:
   - Don't crash with raw tracebacks if input is bad.
   - Return JSON with `{"error": "descriptive message"}` so the LLM can understand what went wrong and self-correct.

6. **Descriptive `SKILL.md` Frontmatter**:
   - Keep `name` concise (1-3 words).
   - Keep `description` under 200 characters but rich in functional keywords.

---

## Troubleshooting & FAQs

### Q: Why didn't the agent use my new skill?
1. Check that `~/.myaaw/skills/<skill_name>/SKILL.md` exists and contains valid YAML frontmatter (`---`).
2. Make sure the `description` in frontmatter contains relevant keywords that match the user's intent.
3. Check Myaaw console logs to verify that `skills.GetSkillsInstruction()` picked up the skill without syntax errors.

### Q: Why is my skill missing from the agent's skills list?
1. Check `~/.myaaw/skills/skills.json` — if the skill's directory name is set to `false`, it is disabled and will not be injected into the prompt.
2. Confirm the key uses the skill **directory name** (e.g. `tavily`), not the display `name` from the frontmatter.
3. Verify the skill directory still exists at `~/.myaaw/skills/<skill_name>/SKILL.md`.

### Q: The agent read `SKILL.md` but got `command not found` or `ModuleNotFoundError`
1. Ensure required Python packages are installed in the active virtual environment:
   ```bash
   .venv/bin/pip install -r ~/.myaaw/skills/<skill>/requirements.txt
   ```
2. Verify absolute vs. relative paths in the command. Using `~/.myaaw/skills/<skill>/scripts/...` is recommended.

### Q: How do I test my skill outside Myaaw?
Run the command directly in your terminal with test JSON:
```bash
.venv/bin/python ~/.myaaw/skills/<skill>/scripts/<script>.py '{"test_key": "test_value"}'
```
Ensure it returns exit code `0` and prints valid JSON.
