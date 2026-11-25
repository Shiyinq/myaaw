# Runtime Command

`myaaw runtime` inspects the built-in Myaaw runtime and every supported external CLI agent runtime detected on the host. It reuses the same discovery, global defaults, agent usage, and CLI authentication probing code (`RuntimeManageService` and `internal/agent/runtime`) that backs **Settings → External Runtimes** in the WebUI dashboard.

For full architecture, protocol details, MCP bridge behavior, and security guardrails, see the [External Agent Runtimes Guide](../agents/external-agents.md).

## Commands

```bash
myaaw runtime list                   # detect installed runtimes, versions, and binary paths
myaaw runtime ls                     # alias for list
myaaw runtime list --check-auth      # also probe CLI login / auth readiness
myaaw runtime list --json            # machine-readable JSON output
```

`runtime` is a host-global command, so it does not require an initialized agent or the `--agent` flag.

## `runtime list`

Probes the host `PATH` and common user/package-manager bin directories (`~/.local/bin`, `~/.opencode/bin`, `~/.nvm`, Homebrew, DeepSeek Harness Desktop bundle, etc.) concurrently for all supported runtime presets:

| Preset ID | Runtime | Protocol | Default Command |
| :--- | :--- | :--- | :--- |
| `myaaw` | Myaaw (Built-in) | Native Myaaw Agent | built-in |
| `claude_code` | Claude Code | Stream-JSON | `claude` |
| `codex` | OpenAI Codex CLI | App-Server JSON-RPC | `codex app-server` |
| `antigravity` | Google Antigravity | ACP JSON-RPC | `agy acp` |
| `opencode` | OpenCode | ACP JSON-RPC | `opencode acp` |
| `deepseek_harness` | DeepSeek Harness | ACP JSON-RPC | `dsh --profile acp` |
| `pi` | Pi Coding Agent | JSONL RPC | `pi --mode rpc` |
| `omp` | Oh My Pi | JSONL RPC | `omp --mode rpc` |
| `custom_acp` | Custom ACP Agent | ACP JSON-RPC | User-configured binary |

### Flags

| Flag | Description |
| :--- | :--- |
| `--check-auth` | Also run non-interactive official CLI auth readiness probes for installed runtimes (e.g. `codex login status`, `agy models`, `opencode auth list`, `dsh --profile acp`, `pi --mode rpc`). Off by default so `myaaw runtime list` stays fast. |
| `--json` | Print machine-readable JSON containing `native`, `management_enabled`, and `runtimes` (each entry includes `installed`, `version`, `binary_path`, `install_methods`, `update_method`, `auth`, `defaults`, and `used_by_agents`). |

### Example output

```text
Available Agent Runtimes

  myaaw              Myaaw (Built-in)           built-in
  claude_code        Claude Code (claude)       not installed
                     install: curl -fsSL https://claude.ai/install.sh | bash
  codex              OpenAI Codex CLI (codex)   installed · codex-cli 0.172.0  1 agent(s): myaadex
                     ~/.nvm/versions/node/v24.14.1/bin/codex
  antigravity        Google Antigravity (agy)   installed · 1.5.17  1 agent(s): myaagy
                     ~/.local/bin/agy
  opencode           OpenCode (opencode)        installed · 1.19.6  1 agent(s): myaaopencode
                     ~/.opencode/bin/opencode
  deepseek_harness   DeepSeek Harness (dsh)     installed · 0.2.1-rc.3  1 agent(s): myaadh
                     /Applications/DeepSeek Harness.app/Contents/Resources/runtime/cli/bin/dsh
  pi                 Pi Coding Agent (pi)       installed · 1.6.1  1 agent(s): myaapi
                     ~/.local/bin/pi
  omp                Oh My Pi (omp)             not installed
                     install: curl -fsSL https://omp.sh/install | sh
  custom_acp         Custom ACP Agent           custom binary

Detected 5/7 external runtimes installed on this host.
Use with: myaaw onboard --yes --agent <name> --runtime <preset>
Tip: pass --check-auth to also probe CLI login readiness, or --json for machine-readable output.
```

## Creating an agent with a detected runtime

Once a preset is installed and authenticated on the host, pass its preset ID to `myaaw onboard` or `myaaw agent create`:

```bash
# Create a new agent using OpenAI Codex CLI
myaaw onboard --yes --agent coder --runtime codex --bot-type private

# Or with agent create and a custom repo working directory
myaaw agent create dsh-dev --yes \
  --runtime deepseek_harness \
  --working-dir ~/Development/my-app \
  --runtime-permission bridge

# Verify the active runtime for that agent
myaaw --agent coder doctor
```

For scripting and CI, you can combine `--json` with `jq`:

```bash
myaaw runtime list --json | jq '.runtimes[] | select(.installed) | {id, version, binary_path}'
```

## Notes

- `myaaw runtime list` reads saved global defaults (if present) from `~/.myaaw/controlplane/controlplane.db` with API keys masked (`********`), and works even before the first agent is initialized.
- To install, update, or run an interactive CLI login/logout PTY from the browser, open **Settings → External Runtimes** in the WebUI dashboard (`myaaw api start && myaaw webui open`).

## Related

- [External Agent Runtimes Guide](../agents/external-agents.md) — protocol drivers, MCP bridge, permissions, and security model.
- [Onboarding](onboard.md) — creating agents with `--runtime` flags.
- [Agent Command](agent.md) — `agent list/create/use/delete`.
- [Utilities (`doctor`)](utilities.md#myaaw-doctor) — checking active runtime readiness for one agent.
