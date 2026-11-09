# WebUI Command

`myaaw webui` installs, runs, and manages the optional Myaaw dashboard separately from the binary. The dashboard is a Node.js app (`@sveltejs/adapter-node`) that talks to the control plane (`myaaw api`).

## Requirements

- Node.js 20+ (the dashboard's only extra runtime dependency).
- A control plane: `myaaw api start` (default `http://localhost:9000`).

## Install

```bash
myaaw webui install                    # latest release
myaaw webui install --version v2026.11.9
```

The command downloads `myaaw-webui-<tag>.tar.gz` from GitHub Releases, extracts it
to `~/.myaaw/webui/current`, and writes `~/.myaaw/webui/config.json`.

You can also install the dashboard **without the binary**:

```bash
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/Shiyinq/myaaw/main/webui-install.sh | bash
```

```powershell
# Windows (PowerShell)
powershell -ExecutionPolicy Bypass -File .\webui-install.ps1
```

Or use the unified installer:

```bash
curl -fsSL .../install.sh | bash -s -- --webui-only  # dashboard only
curl -fsSL .../install.sh | bash -s -- --webui       # binary + dashboard
```

## Commands

| Command | Description |
| :--- | :--- |
| `webui install` | Download and install the WebUI bundle. |
| `webui run` | Run the WebUI in the foreground. |
| `webui start` | Start the WebUI as a background daemon. |
| `webui stop` | Stop the WebUI daemon. |
| `webui restart` | Restart the WebUI daemon. |
| `webui status` | Check daemon status and print the URL. |
| `webui update` | Update to the latest bundle (restarts if running). |
| `webui uninstall` | Stop and remove the bundle. |
| `webui open` | Open `http://127.0.0.1:5050` (starts it if needed). |

## Flags

| Flag | Default | Meaning |
| :--- | :--- | :--- |
| `--port` | `5050` | WebUI port. |
| `--api-url` | `http://localhost:9000` | Control-plane URL. |
| `--version` | latest | Release tag for `install` / `update`. |
| `--purge` | `false` | With `uninstall`, also remove control-plane credentials. |

## Data layout

```text
~/.myaaw/
  webui/
    current/             # extracted bundle (build/, node_modules/, package.json)
    config.json          # {"port":"5050","api_url":"http://localhost:9000","version":"..."}
  pids/myaaw-webui.pid
  logs/myaaw-webui.log
```

## How it runs

`myaaw webui start` daemonizes `myaaw webui run` as `myaaw-webui`. `webui run`
executes `node build/index.js` with:

- `PORT=<port>`
- `HOST=127.0.0.1`
- `MYAAW_API_BASE_URL=<api-url>`

On Unix the process is replaced with Node (`exec`), so the daemon's PID is the
Node server and `webui stop` terminates it cleanly.

## First run (frontend-first)

When no binary is present, open `/setup`. It reports whether the binary and the
control plane are available, and can download/install the correct binary for the
host, then start `myaaw api`. The login page links to it ("First run? Set up
Myaaw"). If the binary is installed elsewhere, skip `/setup` and sign in with the
remote control plane's Server URL.

## Examples

```bash
# Binary-first: control plane, then dashboard
myaaw api start
myaaw webui install
myaaw webui start
myaaw webui open

# Lifecycle
myaaw webui status
myaaw webui update
myaaw webui uninstall
```

## Remote access

Keep the control plane on loopback and expose it through a tunnel (Tailscale,
Cloudflare, ngrok) or SSH port-forward. The login page has a **Server URL**
field for this; leave it empty to use the local control plane, or enter the
tunnel URL to connect directly. See [Remote WebUI Connection](../webui/remote-connection.md)
and [WebUI Security](../webui/security.md).

## Notes

- The dashboard binds `127.0.0.1` only.
- The bundle is platform-independent (pure JS), so one artifact works on every OS.
- `webui install` needs a release that contains `myaaw-webui-*.tar.gz`. Until such
  a release exists, `install` reports that no WebUI bundle was found.
- `myaaw api` defaults to `--host 127.0.0.1`; use `--host 0.0.0.0` only behind a
  tunnel or reverse proxy.
