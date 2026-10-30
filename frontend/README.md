# Myaaw Dashboard

SvelteKit frontend for the Myaaw control plane (`myaaw api`). It uses the same
Svelte 5 + Tailwind v4 + shadcn-svelte structure as the `fasmo` frontend
template.

See [`docs/frontend-plan.md`](../docs/frontend-plan.md) for the API mapping and
the implementation plan.

## Requirements

- Node.js 20+
- A running control plane: `myaaw api start --port 9000`

## Configuration

```bash
cp .env.example .env
# MYAAW_API_BASE_URL=http://localhost:9000
```

The browser talks to `/api` on this app; `src/hooks.server.ts` proxies those
requests to `MYAAW_API_BASE_URL`, so the token never takes part in a CORS
preflight.

## Developing

```bash
npm install
npm run dev
```

## Checks

```bash
npm run check   # svelte-check
npm run build   # production build (adapter-node)
```

## Production

```bash
npm run build
node build      # PORT=3000 by default
```

Log in with the credentials from `~/.myaaw/controlplane.json` (seeded on the
first launch).
