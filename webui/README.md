# Myaaw Dashboard

Web UI for the Myaaw control plane (`myaaw api`). This is not a static
bundle: `@sveltejs/adapter-node` builds a small Node server that renders pages,
serves the PWA, and proxies every `/api` request to the control plane.

Stack: SvelteKit 2, Svelte 5 (runes), TypeScript, Tailwind CSS v4,
shadcn-svelte + bits-ui, xterm.js, and `@vite-pwa/sveltekit` for offline support.

## Requirements

- Node.js 20+
- A running control plane: `myaaw api start --port 9000`

Agents are not served by this app. Chat is proxied to the control plane, so the
selected agent needs its own server running (`myaaw server --agent <name> start`).

## Quick start

```bash
# terminal 1 — control plane
myaaw api start --port 9000

# terminal 2 — dashboard
cd webui
npm install
cp .env.example .env
npm run dev          # http://localhost:5173
```

Sign in with the credentials seeded on the control plane's first launch:
`myaaw` / `myaawadmin`. They are stored as a bcrypt hash in
`~/.myaaw/config/controlplane.json` — change them after logging in.

## Configuration

`.env.example` contains the server-side variables:

```bash
MYAAW_API_BASE_URL=http://localhost:9000
# Public hosting: /setup returns 404, and the login host hints are hidden.
# MYAAW_WEBUI_PUBLIC=1
```

`src/hooks.server.ts` forwards `/api/*` to that origin through the same-origin
proxy. Two consequences:

- Tokens never take part in a CORS preflight, because the browser only ever
  talks to the dashboard origin.
- SSE and streaming responses work with a plain `fetch`.

The variable is private (`$env/dynamic/private`), so it is read at runtime and
never shipped to the browser. The same server also answers `GET /health` with
`200 OK`, which is handy for container health checks.

## What's inside

| Group      | Routes                                                                                                                     |
| :--------- | :------------------------------------------------------------------------------------------------------------------------- |
| Fleet      | `/app` (agent list), `/app/new` (add agent)                                                                                |
| Workspace  | `/app/dashboard`, `/app/chat`, `/app/context`, `/app/skills`, `/app/tools`, `/app/contacts`, `/app/files`, `/app/terminal` |
| Configure  | `/app/providers`, `/app/channels`, `/app/config`, `/app/cron`                                                              |
| Operations | `/app/server`, `/app/logs`, `/app/backups`, `/app/approvals`, `/app/observability`                                         |
| Account    | `/app/plugins`, `/app/profile`, `/app/settings`                                                                            |
| Public     | `/login`, `/offline`                                                                                                       |

The web terminal (`/app/terminal`) only works when the control plane was started
with `myaaw api start --terminal`; otherwise its routes answer `404`. See
[Web Terminal](../docs/api/terminal.md).

## Development

```bash
npm run dev        # Vite dev server on :5173, proxying /api to the control plane
npm run check      # svelte-kit sync + svelte-check
npm run check:watch
npm run lint       # prettier --check . && eslint .
npm run format     # prettier --write .
```

- UI copy lives in `src/lib/i18n/locales/en.json` and `id.json`. Add new keys to
  **both** files; `detectLocale` picks the locale on the server.
- Generated output (`.svelte-kit/`, `build/`, `dev-dist/`) is gitignored. Delete
  `.svelte-kit/` and `node_modules/.vite/` if you move the checkout and the dev
  server starts with stale absolute paths.

## Related docs

- [Control Plane API](../docs/api/control-plane.md)
- [Web Terminal](../docs/api/terminal.md)
- [CLI `api` command](../docs/cli/api.md)
- [Agent context blocks](../docs/agents/context.md)
