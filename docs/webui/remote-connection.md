# Remote WebUI Connection (Tunnel / SSH, Option A)

> Status: **draft** — see also [WebUI security](security.md).

This document describes the remote connection feature: the dashboard (frontend) on one machine, the binary (control plane + agent) on another server, connected over a **tunnel** or **SSH**.

---

## 1. Summary

- The **control plane stays on loopback** (`127.0.0.1:9000`) — safe from direct access.
- Remote access goes through a **tunnel** (Tailscale/Cloudflare/ngrok) or **SSH port-forward**.
- **Option A (direct)**: the browser fetches the control-plane URL **directly** (no server-side WebUI proxy).
- The user types the URL on the **login/settings page** — no manual config-file editing.

---

## 2. Concept: "Server URL" (control-plane base URL)

A single `api_url` value is the control-plane address the browser calls.

| Mode | `api_url` |
| :--- | :--- |
| Local (all-in-one) | `http://localhost:9000` |
| Remote via Tailscale | `http://100.x.y.z:9000` |
| Remote via Cloudflare | `https://panel.example.com` |
| Remote via ngrok | `https://abc.ngrok-free.app` |
| Remote via SSH forward | `http://localhost:9000` (forwarded) |

The value is stored in the browser's `localStorage`, seeded from `~/.myaaw/webui/config.json`.

---

## 3. Topology & flow

```
# Local (all-in-one)
browser → http://localhost:9000 (control plane, loopback)

# Remote
browser (laptop) → tunnel/SSH URL → control plane (server A, loopback)
```

The browser always fetches `api_url` directly. The control plane must allow the WebUI origin via CORS.

---

## 4. Connection options & examples

### 4.1 Tailscale (private VPN, recommended for personal use)

1. Install Tailscale on the laptop and the server, join the same tailnet.
2. Note the server's IP/name, e.g. `100.64.1.10` or `myserver.ts.net`.
3. In the WebUI login, set Server URL to `http://100.64.1.10:9000`.

Safe without TLS (WireGuard encrypts the connection).

### 4.2 Cloudflare Tunnel

1. Run `cloudflared` on the server, pointing at `http://localhost:9000`.
2. You get `https://panel.example.com`.
3. In the WebUI login, set Server URL to `https://panel.example.com`.

Can add Cloudflare Access for an extra login layer.

### 4.3 ngrok

1. Run `ngrok http 9000` on the server.
2. You get `https://abc.ngrok-free.app`.
3. In the WebUI login, set Server URL to `https://abc.ngrok-free.app`.

Free URLs may change; good for testing.

### 4.4 SSH port-forward (zero code)

```bash
ssh -L 9000:localhost:9000 user@server
```

Then set Server URL to `http://localhost:9000` in the WebUI login (the default).

---

## 5. UX (login & settings)

- **Implemented**: the login page has an optional **Server URL** field (pre-filled from the saved value).
- **Implemented**: the settings page has a **Connection** card that saves the URL and reconnects.
- **Implemented**: `/setup` reports whether the binary and control plane are available, and can install the binary on this host. For a remote control plane, sign in and set the Server URL instead of installing locally.

---

## 6. Frontend changes

- A new store/module (alongside `accessToken`) holds `api_url`.
- `webui/src/lib/apis/client.ts`: `API_BASE` becomes dynamic (`getApiBase()`) instead of the constant `/api`.
- Keep `/api` as the default (same-origin proxy) for local use; an absolute URL makes the browser call the control plane directly. The server-side proxy stays for the default path.
- The initial `api_url` is read from `~/.myaaw/webui/config.json` (via a small SSR endpoint, or client-side).
- `auth.login` posts to `${api_url}/auth/login`.

---

## 7. Backend changes

- `myaaw api` gains a `--host` flag, defaulting to `127.0.0.1` (`Run(host+":"+port)` instead of `Run(":"+port)`).
- CORS: allow the WebUI origin (dev default `*`, restricted in production).
- The control plane does **not** need built-in TLS for the main path — the tunnel/SSH provides encryption.

---

## 8. Config schema

`~/.myaaw/webui/config.json`:

```json
{
  "port": 5050,
  "api_url": "http://localhost:9000",
  "version": "v2026.11.9"
}
```

`api_url` is the initial/default value. User changes are stored in browser `localStorage` (simpler, avoids writing the file on every change).

---

## 9. End-to-end flow (remote example)

1. On server A: `myaaw api start` (loopback) + `myaaw server --agent <name> start`.
2. On server A: run a tunnel (`tailscale`/`cloudflared`/`ngrok`).
3. On the laptop: install the WebUI (`webui-install.sh` or `myaaw webui install`).
4. Open the dashboard → choose "Connect to a remote control plane".
5. Enter the Server URL (tunnel URL) + username + password.
6. Log in → (if no agents yet) onboard through the frontend.

---

## 10. References

- [WebUI security](security.md)
- [`api` CLI command](../cli/api.md)
- [Control Plane API](../api/control-plane.md)
