# WebUI Command

`myaaw webui` installs, runs, and manages the optional Myaaw dashboard separately from the binary. The dashboard is a Node.js app (`@sveltejs/adapter-node`) that talks to the control plane (`myaaw api`).

## Requirements

- Node.js 22+ (the dashboard's only extra runtime dependency).
- A control plane: `myaaw api start` (default `http://localhost:9000`).

## Install

```bash
myaaw webui install                    # latest release
myaaw webui install --version v2026.11.9
myaaw webui install --public           # public hosting: no /setup, no login host hints
```

The command downloads `myaaw-webui-<tag>.tar.gz` from GitHub Releases, extracts it
to `~/.myaaw/webui/current`, and writes `~/.myaaw/webui/config.json`.

You can also install the dashboard **without the binary**:

```bash
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/Shiyinq/myaaw/main/webui-install.sh | bash
# remote control plane, public hosting
curl -fsSL https://raw.githubusercontent.com/Shiyinq/myaaw/main/webui-install.sh \
  | bash -s -- --connect https://control.example.com --public
```

```powershell
# Windows (PowerShell): download, then run
irm https://raw.githubusercontent.com/Shiyinq/myaaw/main/webui-install.ps1 -OutFile webui-install.ps1
powershell -ExecutionPolicy Bypass -File .\webui-install.ps1
# with a remote control plane / public hosting
powershell -ExecutionPolicy Bypass -File .\webui-install.ps1 -Connect https://control.example.com -Public
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
| `webui rollback` | List or restore a previous bundle (`--version <tag>`). |
| `webui uninstall` | Stop and remove the bundle. |
| `webui open` | Open `http://127.0.0.1:5050` (starts it if needed). |

## Flags

| Flag | Default | Meaning |
| :--- | :--- | :--- |
| `--port` | `5050` | WebUI port. |
| `--api-url` | `http://localhost:9000` | Control-plane URL. |
| `--public` | `false` | Public hosting: `/setup` returns 404 and the login host hints are hidden. Saved in `config.json`, and read back when `MYAAW_WEBUI_PUBLIC` is unset. |
| `--version` | latest | Release tag for `install` / `update` / `rollback`. |
| `--purge` | `false` | With `uninstall`, also remove control-plane credentials. |

## Data layout

```text
~/.myaaw/
  webui/
    current/             # extracted bundle (build/, node_modules/, package.json)
    versions/<tag>/      # previous bundles kept for rollback (last three)
    config.json          # {"port":"5050","api_url":"http://localhost:9000","version":"...","node_path":"/path/to/node"}
  pids/myaaw-webui.pid
  logs/myaaw-webui.log
```

## How it runs

`myaaw webui start` daemonizes `myaaw webui run` as `myaaw-webui`. `webui run`
executes `node build/index.js` with:

- `PORT=<port>`
- `HOST=127.0.0.1`
- `MYAAW_API_BASE_URL=<api-url>`
- `MYAAW_WEBUI_PUBLIC=1` when `public` is enabled

On Unix the process is replaced with Node (`exec`), so the daemon's PID is the
Node server and `webui stop` terminates it cleanly.

`webui start` also resolves the Node.js binary in your shell and saves its
absolute path as `node_path` in `config.json`. launchd and systemd start the
daemon with a minimal `PATH`, so a Node installed by nvm, asdf, or another
version manager would otherwise be invisible to `webui run`. Re-run
`webui start` after switching Node versions; the pinned path is validated on
every start and `webui run` falls back to `PATH` when it is gone.

## Rollback

`myaaw webui update` and `myaaw webui install` keep the previous bundle under
`~/.myaaw/webui/versions/<tag>` (the last three). `myaaw webui rollback` lists
them; `myaaw webui rollback --version <tag>` restores one and restarts the
daemon if it was running.

## First run (frontend-first)

When no binary is present, open `/setup`. It reports whether the binary and the
control plane are available, and can download/install the correct binary for the
host, then start `myaaw api`. The login page links to it ("First run? Set up
Myaaw"). If the binary is installed elsewhere, skip `/setup` and sign in with the
remote control plane's Server URL. The `--connect` / `--api-url` value is seeded
as a saved connection, so you can switch between it, other tunnels, and Local on
the login or Settings page.

For a dashboard served to other people (frontend only, reached through a
tunnel), use `--public` (or `MYAAW_WEBUI_PUBLIC=1 node build/index.js`). It makes
`/setup` and its endpoints answer `404`, and hides the default-credentials hint
and the setup link on the login page. See [WebUI security](../webui/security.md)
for what this mode does and does not protect.

The release currently publishes binaries for **linux/amd64** and **darwin/arm64**
only. On other platforms `/setup` cannot download a binary yet; install `myaaw`
manually and sign in.

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
