# WebUI Security (Distribution, Loopback, & Remote Access)

> Status: **draft** — see also [remote connection](remote-connection.md).

This document describes the security model for the features being built: WebUI distribution, loopback binding, and remote access (tunnel/SSH, "Option A").

---

## 1. Threat model

What we protect:

1. **Control-plane credentials** (username/password) and the **bearer token** — must not be sniffed or stolen.
2. **The binary host** (server A) — no unauthorized person should get a shell or file access.
3. **Agent data** (`~/.myaaw/<agent>`: chats, files, config, backups) — must not leak to outsiders.

Primary threats:

- Network-path sniffing (public Wi-Fi, LAN, ISP).
- Default-credential attacks (internet bot scanners).
- CSRF / phishing via malicious web pages.
- SSRF through any feature that accepts arbitrary URLs.

---

## 2. Core principles

| Principle | Implementation |
| :--- | :--- |
| **Loopback by default** | `myaaw api` listens on `127.0.0.1:9000`; the WebUI listens on `127.0.0.1:5050`. No `0.0.0.0` unless explicitly requested. |
| **Remote through a safe door** | Only the tunnel/SSH entry point is exposed; the control plane stays on loopback. |
| **Strong, unique password** | Change the default before connecting remotely. |
| **Encrypted transport** | HTTPS (ngrok/Cloudflare) or a VPN (Tailscale WireGuard). |
| **Least privilege** | `--terminal` is off by default. |

---

## 3. Credentials & login

- The control plane seeds default credentials `myaaw` / `myaawadmin` in `~/.myaaw/controlplane.json` (bcrypt) — see `internal/services/controlplane/service/auth.go`.
- **Required**: force a password change on first login (especially before connecting remotely).
- **Implemented**: `/auth/login` and `/auth/me` return `must_change_password`; the dashboard keeps the reader on the profile page (route guard) until the seeded password is changed.
- The password is sent via `POST /auth/login`; ensure the path is encrypted (see §4).

---

## 4. Transport (HTTP vs HTTPS vs VPN)

| Path | Status | Notes |
| :--- | :--- | :--- |
| Loopback (`http://localhost:9000`) | Safe | Never leaves the machine; plain `http` is acceptable here |
| Tailscale (`http://100.x.y.z:9000`) | Safe | Encrypted by WireGuard; `http` on top is acceptable |
| Cloudflare/ngrok (`https://...`) | Safe | Must be `https` |
| `http://` to a public/LAN host | **Unsafe** | Token and password travel in plaintext and can be sniffed |

**Rule**: never expose a plain-HTTP control plane to an untrusted network.

---

## 5. Token & session

- Login returns a random 32-byte (hex) token with a 24-hour TTL, stored in the control plane's memory (`auth.go`).
- The token is sent in the `Authorization: Bearer <token>` header.
- In the frontend, the token lives in `localStorage` (see `webui/src/lib/constants/auth.ts`).
- **Option A consequence**: the token goes directly from the browser to the control plane (not through the proxy). This is safe **as long as** the path is encrypted (tunnel/HTTPS/Tailscale). Never type a tunnel URL you do not own — that enables credential phishing.

---

## 6. CORS (Option A only)

- Because the browser fetches the control plane directly (cross-origin), the control plane must allow the WebUI origin via CORS.
- Default: loopback origins (`localhost`, `127.0.0.1`, `::1`) are allowed, so a local dashboard works out of the box. Any other origin (a remote dashboard or a tunnel domain) must be listed in `ALLOWED_ORIGINS`; `ALLOWED_ORIGINS=*` restores the permissive behavior for development.
- Native desktop/mobile clients do not send an `Origin` and are unaffected by this default.

---

## 7. Terminal (`--terminal`)

- `--terminal` opens a real PTY shell on the control-plane host (`myaaw api`).
- In remote mode this is a **remote shell**. A leaked token means code execution on the server.
- Policy: **off by default**; only enable it for trusted hosts, behind a tunnel/VPN, with a strong password.

---

## 8. Setup / binary-install endpoints (frontend-first)

- Endpoints such as `POST /setup/install` (used to download and install the binary) carry risk:
  - **CSRF**: a malicious web page could POST to `http://localhost:5050/setup/install`.
  - **SSRF / phishing**: if the target URL can be set freely.
- Mitigations:
  - Bind the WebUI to `127.0.0.1`.
  - Require a one-time token/confirmation (rendered by the setup page and sent back).
  - Disable the endpoint once the binary is installed.
  - Validate the download URL: `https://` to allowed hosts (GitHub releases) only, and verify the checksum.
- **Implemented**: the install endpoint rejects cross-origin requests (`Origin`), requires a one-time token issued by the setup page (10-minute TTL, consumed on success), refuses to run once the binary is installed, and verifies the release `SHA256SUMS` when present.

---

## 9. Public (FE-only) hosting

When the dashboard is the only thing you publish — the frontend on a small VPS, the binary on another machine — run it with:

```bash
MYAAW_WEBUI_PUBLIC=1 node build/index.js
# or: webui-install.sh --public
# or: myaaw webui start --public   (persisted in ~/.myaaw/webui/config.json)
```

An explicit `MYAAW_WEBUI_PUBLIC` wins; when it is unset, the dashboard falls back to `"public": true` in `~/.myaaw/webui/config.json`, so a later manual `node build/index.js` stays in public mode. `MYAAW_WEBUI_PUBLIC=0` turns the mode off.

This mode removes every host-management surface, so a visitor cannot install or inspect anything on the machine that serves the dashboard:

| Surface | Default | With `MYAAW_WEBUI_PUBLIC=1` |
| :--- | :--- | :--- |
| `/setup` (install the binary on this host) | available | **404**, and the same for `/setup/status` and `/setup/install` |
| "Default credentials come from `~/.myaaw/controlplane.json`." | shown | hidden |
| "First run? Set up Myaaw" link | shown | hidden |

The block runs in `webui/src/hooks.server.ts` before any route is resolved, so nothing under `/setup` can be reached and the 404 does not reveal that the page ever existed.

What it does **not** do:

- It does not change the bind address: `HOST` stays `127.0.0.1`. Exposure is still the tunnel's job (§11).
- It does not protect the control plane. Visitors still reach it through the WebUI's `/api` proxy (or directly, Option A), so change the default password (§3) and keep `ALLOWED_ORIGINS` tight (§6).

Typical layout: the dashboard and the control plane each sit behind their own tunnel. Visitors open the dashboard URL, paste the control-plane tunnel URL into the **Server URL** field on the login page, and sign in.

---

## 10. Downloads & verification

- Binaries and WebUI artifacts are downloaded from GitHub Releases (HTTPS).
- Verify the checksum (`*.sha256`) when available.
- Write files via temp + rename (atomic); keep the previous version for rollback.
- Do not trust redirects to other hosts.

---

## 11. Tunnel vs SSH

| Aspect | SSH port-forward | Tailscale | Cloudflare/ngrok |
| :--- | :--- | :--- | :--- |
| Encryption | SSH | WireGuard | TLS |
| Who can reach it | Only SSH owners | Only tailnet devices | Public (restrictable) |
| Setup | Zero code, manual | App on both sides | Account + config |
| Good for | Yourself, ad hoc | Private, permanent | Sharing / many users |

---

## 12. Security checklist before shipping the feature

- [ ] `myaaw api` defaults to loopback (`--host 127.0.0.1`).
- [ ] The WebUI binds `127.0.0.1`.
- [ ] Forced password change on first login.
- [ ] Production CORS is restricted (not `*`).
- [ ] Setup endpoints are CSRF-protected and URL-validated.
- [ ] Public hosting disables `/setup` and the login host hints (`MYAAW_WEBUI_PUBLIC=1`).
- [ ] Downloads use HTTPS + checksums.
- [ ] `--terminal` defaults off and its risk is documented.
- [ ] This document is linked from the README.
