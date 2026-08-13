# MCP (Model Context Protocol) & Custom Tools Guide

Myaaw provides an extensible, modular tooling system that empowers AI agents with custom capabilities. You can seamlessly extend Myaaw with **Model Context Protocol (MCP) servers**, **Custom CLI tools**, and configure **Built-in tools** through a single configuration file with **zero-downtime hot-reloading**.

> **Looking for on-demand agent skills?** Check out the [Agent Skills Guide](agent_skills.md) for dynamic `SKILL.md` workflows.

---

## Table of Contents
- [Overview & Architecture](#overview--architecture)
- [Configuration File (`tools.json`)](#configuration-file-toolsjson)
- [Managing Built-in Tools](#managing-built-in-tools)
- [Model Context Protocol (MCP) Servers](#model-context-protocol-mcp-servers)
  - [How MCP Works in Myaaw](#how-mcp-works-in-myaaw)
  - [Example: Playwright Browser Automation](#example-playwright-browser-automation)
  - [Example: Filesystem MCP Server](#example-filesystem-mcp-server)
- [Custom CLI Tools](#custom-cli-tools)
  - [CLI Tool Contract (`--schema` & `--execute`)](#cli-tool-contract---schema----execute)
  - [Method 1: Configured via `tools.json`](#method-1-configured-via-toolsjson)
  - [Method 2: Zero-Config Directory Tools](#method-2-zero-config-directory-tools)
  - [Custom Tool Examples](#custom-tool-examples)
    - [JavaScript (Node.js) Example](#javascript-nodejs-example)
    - [Python Example](#python-example)
    - [Bash Script Example](#bash-script-example)
- [Hot-Reloading & Live Monitoring](#hot-reloading--live-monitoring)
- [Priority & Safety Guidelines](#priority--safety-guidelines)

---

## Overview & Architecture

Myaaw combines three tiers of tools:
1. **Built-in Tools**: Core capabilities (`bash`, `filesystem`, `cron`, `execute_python`, `provider`, `subagent`).
2. **Custom CLI Tools**: Standalone scripts or executables adhering to the `--schema` and `--execute` protocol.
3. **MCP Servers**: Industry-standard [Model Context Protocol](https://modelcontextprotocol.io/) servers communicating via `stdio` JSON-RPC.

```
                  ┌───────────────────────────────┐
                  │    ~/.myaaw/tools/tools.json  │
                  └───────────────┬───────────────┘
                                  │ (Hot-Reloaded via fsnotify)
                                  ▼
 ┌─────────────────────────────────────────────────────────────────┐
 │                        Myaaw Agent Core                         │
 ├──────────────────┬───────────────────────┬──────────────────────┤
 │  Built-in Tools  │   Custom CLI Tools    │     MCP Servers      │
 │  (bash, python,  │ (Node.js, Python,     │ (Playwright, SQLite, │
 │  filesystem, etc)│  Bash, Go, Binaries)  │  Filesystem, etc.)   │
 └──────────────────┴───────────────────────┴──────────────────────┘
```

---

## Configuration File (`tools.json`)

All tools are configured in `~/.myaaw/tools/tools.json`.

```json
{
  "builtin_tools": {
    "bash": true,
    "cron": true,
    "filesystem": true,
    "provider": true,
    "python": true,
    "subagent": true
  },
  "custom_tools": [
    {
      "name": "hello_helper",
      "command": "node",
      "args": [
        "/path/to/script.js"
      ],
      "enabled": true
    }
  ],
  "mcp_servers": [
    {
      "name": "playwright",
      "command": "npx",
      "args": [
        "-y",
        "@playwright/mcp"
      ],
      "enabled": true
    }
  ]
}
```

---

## Managing Built-in Tools

You can enable or disable built-in tools by setting their boolean flag in `builtin_tools`:

| Tool Name | Key / Alias | Description |
| :--- | :--- | :--- |
| **Bash** | `"bash"` | Execute system shell commands |
| **Filesystem** | `"filesystem"` | Read, write, search, and manage local files |
| **Cron** | `"cron"` | Schedule one-time or recurring tasks and reminders |
| **Python** | `"python"` or `"execute_python"` | Dynamic Python code execution in isolated venv |
| **Provider** | `"provider"` | Change or switch LLM providers dynamically |
| **Subagent** | `"subagent"` | Spawn autonomous subagents for task delegation |

> **Note**: If a built-in tool is omitted from `builtin_tools`, it defaults to `true` (enabled).

---

## Model Context Protocol (MCP) Servers

Myaaw implements native MCP client capabilities over standard input/output (`stdio`).

### How MCP Works in Myaaw
- **Automatic PATH Resolution**: Works seamlessly in background daemon services (`myaaw start`), automatically resolving Node.js, NVM, and Homebrew paths from your login shell.
- **Prefix Isolation**: All tools exposed by an MCP server are prefixed as `<server_name>_<tool_name>` (e.g., `playwright_browser_navigate`) to avoid naming collisions.
- **Schema Sanitization**: Incompatible JSON Schema draft fields (e.g., `$schema`, `propertyNames`) are automatically sanitized to ensure compatibility with LLMs like Gemini and OpenAI.

### Example: Playwright Browser Automation
Enables full browser navigation, scraping, and web interaction for your agent:

```json
{
  "mcp_servers": [
    {
      "name": "playwright",
      "command": "npx",
      "args": [
        "-y",
        "@playwright/mcp"
      ],
      "enabled": true
    }
  ]
}
```

### Example: Filesystem MCP Server
Exposes specific directories via the official MCP filesystem server:

```json
{
  "mcp_servers": [
    {
      "name": "extfs",
      "command": "npx",
      "args": [
        "-y",
        "@modelcontextprotocol/server-filesystem",
        "/path/to/allowed/directory"
      ],
      "enabled": true
    }
  ]
}
```

---

## Custom CLI Tools

Custom CLI tools allow you to integrate any script or binary (JavaScript, Python, Bash, Go, Ruby, Rust, etc.) into the AI agent.

> [!IMPORTANT]
> **Schema is Mandatory**: An LLM cannot call a tool without knowing its parameters and description. Every custom tool **must have a valid OpenAI-format Function Schema**. If a tool does not provide a valid schema, Myaaw will reject and skip it for safety.

---

### Two Ways to Provide Tool Schema

You can provide the tool's schema in either of two ways:

#### Option A: Direct Inline Schema in `tools.json` (Recommended for existing scripts/binaries)
If your script does not support a `--schema` flag (e.g. standard curl, external CLI, or legacy script), you can write the full schema directly inside `tools.json`:

```json
{
  "custom_tools": [
    {
      "name": "fetch_quote",
      "command": "curl",
      "args": ["-s", "https://api.quotable.io/random"],
      "enabled": true,
      "schema": {
        "type": "function",
        "function": {
          "name": "fetch_quote",
          "description": "Fetches a random inspirational quote from the internet.",
          "parameters": {
            "type": "object",
            "properties": {}
          }
        }
      }
    }
  ]
}
```
*When `"schema"` is defined in `tools.json`, Myaaw uses it directly and skips executing `--schema`.*

---

#### Option B: Dynamic Schema via `--schema` Flag (Recommended for self-contained tools)
If `"schema"` is omitted from `tools.json` (or when using [Zero-Config Directory Tools](#method-2-zero-config-directory-tools)), your script must output its JSON schema when executed with the `--schema` argument:

```bash
my_script --schema
```

**Schema Output Format:**
```json
{
  "type": "function",
  "function": {
    "name": "tool_name",
    "description": "What this tool does",
    "parameters": {
      "type": "object",
      "properties": {
        "param1": {
          "type": "string",
          "description": "Parameter description"
        }
      },
      "required": ["param1"]
    }
  }
}
```

---

### CLI Execution Contract (`--execute '<json_arguments>'`)
When the AI agent decides to call your custom tool, Myaaw executes your command with `--execute` followed by the arguments formatted as a single JSON string:

```bash
<command> <args...> --execute '{"param1":"value"}'
```

The output written to `stdout` will be captured and returned to the LLM as the tool result.

---

### Method 1: Configured via `tools.json`
Define your tool under the `custom_tools` array in `~/.myaaw/tools/tools.json`:

```json
{
  "custom_tools": [
    {
      "name": "my_tool",
      "command": "node",
      "args": ["/path/to/my_script.js"],
      "enabled": true
    }
  ]
}
```

> [!TIP]
> **Organizing Tools in Subdirectories**:
> For more complex tools that have multiple helper files, `package.json`, or virtual environments, you can create dedicated subdirectories inside `~/.myaaw/tools/` (e.g. `~/.myaaw/tools/hello_helper/script.js`):
> 
> ```
> ~/.myaaw/tools/
> ├── tools.json
> ├── hello_helper/
> │   ├── package.json
> │   └── script.js
> └── weather_tool/
>     ├── requirements.txt
>     └── main.py
> ```
> 
> Then reference the script path in `tools.json`:
> ```json
> {
>   "custom_tools": [
>     {
>       "name": "hello_helper",
>       "command": "node",
>       "args": [
>         "~/.myaaw/tools/hello_helper/script.js"
>       ],
>       "enabled": true
>     }
>   ]
> }
> ```

---

### Method 2: Zero-Config Directory Tools
Place an executable script directly inside the `~/.myaaw/tools/` directory and ensure it has execute permissions (`chmod +x <script>`):

```bash
chmod +x ~/.myaaw/tools/weather.py
```

Myaaw will automatically discover the script, execute it with `--schema`, and register it into the agent's tool registry without needing any entry in `tools.json`.

---

### Custom Tool Examples

#### JavaScript (Node.js) Example
Save as `~/.myaaw/tools/hello.js`:

```javascript
#!/usr/bin/env node

const args = process.argv.slice(2);

// 1. Handle Schema Request
if (args.includes("--schema")) {
  const schema = {
    type: "function",
    function: {
      name: "hello_helper",
      description: "Generates a personalized greeting message and returns current timestamp.",
      parameters: {
        type: "object",
        properties: {
          name: {
            type: "string",
            description: "The name of the user to greet"
          },
          language: {
            type: "string",
            description: "Greeting language: id, en, jp (optional)"
          }
        },
        required: ["name"]
      }
    }
  };
  console.log(JSON.stringify(schema, null, 2));
  process.exit(0);
}

// 2. Handle Tool Execution
const execIdx = args.indexOf("--execute");
if (execIdx !== -1 && args[execIdx + 1]) {
  try {
    const input = JSON.parse(args[execIdx + 1]);
    const name = input.name || "Friend";
    const lang = (input.language || "en").toLowerCase();

    let greeting = `Hello ${name}! Welcome to MyAAW.`;
    if (lang === "id") greeting = `Halo ${name}! Selamat datang di MyAAW.`;
    if (lang === "jp") greeting = `Konnichiwa, ${name}-san! MyAAW e youkoso.`;

    console.log(JSON.stringify({
      message: greeting,
      timestamp: new Date().toISOString()
    }));
    process.exit(0);
  } catch (err) {
    console.error("Error parsing arguments:", err.message);
    process.exit(1);
  }
}
```

#### Python Example
Save as `~/.myaaw/tools/calc.py`:

```python
#!/usr/bin/env python3
import sys
import json

args = sys.argv[1:]

if "--schema" in args:
    schema = {
        "type": "function",
        "function": {
            "name": "custom_calc",
            "description": "Calculates arithmetic expression safely",
            "parameters": {
                "type": "object",
                "properties": {
                    "expression": {
                        "type": "string",
                        "description": "Math expression, e.g. '24 * 7'"
                    }
                },
                "required": ["expression"]
            }
        }
    }
    print(json.dumps(schema))
    sys.exit(0)

if "--execute" in args:
    idx = args.index("--execute")
    if idx + 1 < len(args):
        payload = json.loads(args[idx + 1])
        expr = payload.get("expression", "0")
        try:
            result = eval(expr, {"__builtins__": None}, {})
            print(json.dumps({"result": result}))
        except Exception as e:
            print(f"Error: {e}")
```

#### Bash Script Example
Save as `~/.myaaw/tools/uptime.sh`:

```bash
#!/bin/bash

if [[ "$1" == "--schema" ]]; then
  cat << 'EOF'
{
  "type": "function",
  "function": {
    "name": "system_uptime",
    "description": "Returns current server uptime and load averages.",
    "parameters": {
      "type": "object",
      "properties": {}
    }
  }
}
EOF
  exit 0
fi

if [[ "$1" == "--execute" ]]; then
  uptime
  exit 0
fi
```

---

## Hot-Reloading & Live Monitoring

Myaaw continuously watches `~/.myaaw/tools/` using file-system notifications (`fsnotify`). Whenever you modify `tools.json` or add/remove scripts in the directory:
1. Myaaw re-reads the configuration.
2. Applies built-in tool filters.
3. Reloads custom CLI tools and MCP servers.
4. Logs active tool counts and names.

You can inspect the live status in real time:

```bash
tail -f ~/.myaaw/logs/tools.log
```

**Example Log Output:**
```log
2026/08/14 02:29:08 Tools configuration changed, reloading...
2026/08/14 02:29:08 Built-in tool 'bash' disabled by config
2026/08/14 02:29:08 Custom tool 'hello_helper' disabled by config
2026/08/14 02:29:08 Total active tools: 24 | Tools: [cron execute_python filesystem playwright_browser_click playwright_browser_close ... provider subagent]
```

---

## Priority & Safety Guidelines

1. **Subagent Exclusion Priority**:
   When Myaaw spawns a subagent, subagent-specific exclusions (e.g. `subagent`, `provider`, `memory`) **always take precedence**, even if `"subagent": true` in `tools.json`. This guarantees that subagents cannot recursively spawn other subagents.

2. **Custom Tool Overrides**:
   If a custom tool is defined in `tools.json` with `"enabled": false`, Myaaw's zero-config loader will respect the setting and **will not** auto-load the executable from the directory.

3. **Background Daemon Consistency**:
   When modifying core application source code, compile and install using `make install`, then run `myaaw restart` to update the background daemon. Configuration edits in `tools.json` do not require a restart and will hot-reload automatically.
