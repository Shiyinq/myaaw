---
name: myaaw-webui-check
description: 'Verify any Myaaw dashboard (webui/) change in a real browser, not just one feature. Use when a UI change or fix needs proving: start/check the dev server and control plane, log in headlessly, drive Chrome (desktop + true phone emulation) and Firefox with Selenium, assert computed geometry (overflow, scroll, rects), take screenshots, and run the svelte-check / prettier / eslint / build gates before reporting or committing.'
---

# Myaaw dashboard verification

The dashboard lives in `webui/` (SvelteKit 2, Svelte 5 runes, Tailwind 4).
It talks to the Go control plane through the same-origin `/api` proxy in
`webui/src/hooks.server.ts`, so the browser only ever sees `:5173`.

This skill is the whole loop for any UI work — a layout tweak, a new page, a
mobile bug, a form, a modal. It is not tied to one page or one feature.

## 1. Check the services before touching a browser

```bash
curl -s -o /dev/null -w 'app: %{http_code}\n'  http://localhost:5173/
curl -s -o /dev/null -w 'api: %{http_code}\n'  http://localhost:9000/health
```

- Both must be `200`.
- Dashboard: `cd webui && npm run dev` (Vite on `:5173`).
- Control plane: it normally runs from the installed CLI (`myaaw api start`);
  do not start a second copy if `:9000` already answers.
- Never leave a stuck Vite HMR build: if a page renders unstyled, reload it
  once and look at the terminal before blaming the CSS.

## 2. Read the code first, then measure

1. Open the page component **and** `webui/src/routes/app/+layout.svelte`.
   The shell decides height (`isFullHeight`), the header, the sidebar/drawer and
   the mobile breakpoints, so half of a layout bug is usually in the layout.
2. Find the classes that own the geometry (`h-[45vh]`, `flex-1`, `min-h-0`,
   `overflow-y-auto`, `max-w-[80%]`, `md:`, `lg:`, `max-sm:`).
3. Only then drive the browser, and assert **computed** facts: rects,
   `scrollHeight` vs `clientHeight`, elements whose `left < 0` or
   `right > innerWidth`, `document.documentElement.scrollWidth - innerWidth`.
   "Looks fine in the screenshot" is not a result; a number is.

## 3. Log in once, seed the client state

The app keeps everything in `localStorage`, so a token is enough:

```bash
curl -s -X POST http://localhost:5173/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"myaaw","password":"myaawadmin"}'
# -> {"expires_at": "...", "token": "...", "user_id": 0}
```

Then, on any page of the origin, seed:

| Key | Value | Why |
| --- | --- | --- |
| `myaaw_auth` | `'true'` | routes past the login guard |
| `myaaw_token` | the token | API calls |
| `myaaw_token_expires` | `expires_at` | expiry check |
| `myaaw_agent` | an agent name, e.g. `'default'` | most pages are agent-scoped |
| `app_locale` | `'en'` or `'id'` | plus cookie `app_locale=<same>` |
| `myaaw_sidebar_collapsed` | `'false'` | otherwise the sidebar is icons-only |
| `myaaw_chat_history_open` | `'false'` | closed drawer on every width |

Copy `templates/check.py`, adjust `PAGES`, and run it — it already logs in,
seeds the state, opens Chrome at a desktop and a phone size, and prints geometry
plus a horizontal-overflow report.

## 4. Two engines, and what each is for

- **Chrome, `mobileEmulation`** — the only way to get a real phone viewport.
  `width: 375` (or `390`), `pixelRatio: 2`, an iPhone UA. It also reports
  `(hover: none)`, so touch-only code paths (`matchMedia('(hover: none)')`)
  behave as they do on a phone. Headless Firefox **cannot** go below ~450 px.
- **Firefox, headless** — a second engine for CSS/HMR quirks and for
  keyboard-shortcut checks. Use `set_window_size(1440, 900)` for desktop.
- Both need `--headless` / `--headless=new`; `chromedriver` and `geckodriver`
  are already cached in `~/.cache/selenium`.

## 5. Interactions worth asserting

- **Scroll**: for a stacking layout expect the page root
  (`overflow-y-auto`) to have `scrollHeight > clientHeight` and the inner list
  to have none; for a split layout the opposite.
- **Selection**: click a row, then check the panel moved into view
  (`scrollTop` changed, panel `top` near 0) — `revealPane()` in
  `webui/src/lib/utils/scroll.ts` does this below `lg`.
- **Drawers/overlays**: assert `inert`/`translate-x` and that a scrim tap, `Esc`
  and a navigation all close it.
- **Keyboard**: one `ActionChains` per press (`key_down` … `perform()`);
  separate chains drop the modifier. Synthetic `KeyboardEvent` rarely reaches
  Svelte handlers — click the real element instead.
- **Both locales** when copy is new: switch `app_locale` and re-read the text.

## 6. The gates, in this order

```bash
cd webui
npx prettier --write <touched files>   # keeps the diff formatting-clean
npx eslint <touched files>
npm run check                          # svelte-check: 0 errors, 0 warnings
npm run lint                           # prettier --check . && eslint .
npm run build                          # adapter-node build must finish
```

`svelte-check` is the one that catches Svelte 5 mistakes (missing `state`,
snippet/`bind:` errors) — never skip it, and quote its actual line in the report.
`npm run build` also catches Tailwind classes that never got generated.

## 7. Traps that have already bitten

- **The shell sidebar is an `<aside>` inside `<main>`.** Queries like
  `document.querySelectorAll('main aside button')` hit the navigation, not the
  page's own aside. Scope from the page root, e.g. the element whose class
  starts with `flex h-full min-h-0 flex-col overflow-y-auto`, then query inside.
- **A persisted `myaaw_sidebar_collapsed=true`** (or an old token) silently
  changes every screenshot. Seed the state every run.
- **HMR**: after editing a file, reload the page before measuring; a hot update
  can leave the old DOM in place for a moment.
- **Smooth scrolling** needs ~600 ms before `scrollTop` reads the final value.
- **`overflow-wrap: break-word` does not shrink `min-content`.** A flex column
  around a long unbroken token still sizes to that token; cap the wrapper
  (`max-w-full`) or use `overflow-wrap: anywhere`.
- **Flex children default to `min-width: auto`**, so a wide child can push a
  `max-w-*` parent wider than the screen; `min-w-0` on the item is the usual fix.
- **Viewport units in tests**: `window.innerWidth` is the truth, `screen.width`
  is not.
- **Do not leave scratch files**: name them `.tmp-*.py` / `.tmp-*.png` at the
  repo root (or write under `$TMPDIR`), `read_file` the screenshot to look at it,
  then delete both before finishing.

## 8. Before reporting

- [ ] `npm run check`, `npm run lint`, `npm run build` all green (quote them).
- [ ] The changed routes checked at 1440×900 **and** 375×812 (Chrome emulation),
      plus 768–1024 if the change touches a `md:`/`lg:` boundary.
- [ ] Overflow and scroll numbers collected, not just screenshots.
- [ ] New copy added to **both** `webui/src/lib/i18n/locales/en.json` and
      `id.json`.
- [ ] Agent daemons, tool/skill toggles, theme and locale restored to what they
      were before the test.
- [ ] `git status` shows only intended changes; no temp scripts or screenshots.

## Boundaries

- This skill proves behaviour; it does not decide design. If the layout works
  but reads badly, say so and ask before reworking it.
- It never commits, pushes or tags. That stays with the user, or with the
  `myaaw-pull-requests` / `myaaw-release` skills.
- Do not add dependencies to the dashboard just to make a check pass; the
  Selenium harness lives outside `webui/` for that reason.
