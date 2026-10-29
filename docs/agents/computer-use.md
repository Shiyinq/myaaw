# Computer Use (Desktop Automation)

Myaaw ships a built-in `computer` tool that lets an agent see the screen
(screenshots) and control the mouse and keyboard. It is **macOS-only** and
**disabled by default**; every action runs through the generic
[approval gate](approvals.md), so nothing happens on the desktop without
policy saying so.

> Linux/Windows: the tool registers but every action fails with
> `COMPUTER_UNSUPPORTED_PLATFORM`. Use the [cua-driver MCP route](mcp-and-custom-tools.md)
> for desktop automation on other platforms.

**Why a native backend instead of cua-driver, and why macOS-only?** The
built-in tool is an in-process Go backend: one binary, no Node/Python runtime,
no IPC, and a single code-signing identity for macOS TCC. `cua-driver` is an
external driver (MCP/node); using it adds a separate process, install/update
path, and permission identity. The MCP route is still supported — register any
driver in `tools.json` and its screenshots flow through the image pipeline
(the reason Point 2 forwards MCP `ImageContent`). Native Windows/Linux
backends are a separate effort: Windows needs `SendInput` + UIAutomation +
capture APIs, Linux needs X11 (XTest/XGetImage) or Wayland (portal + libei),
each with different security models and headless CI runners. That is why
native Windows/Linux drivers are explicitly out of scope for the first
iteration (see `docs/computer-use-plan.md`).

---

## 1. Enabling

The tool only exists when computer use is on. Two ways to enable it:

| Way | Setting |
|---|---|
| Environment | `MYAAW_COMPUTER_USE=true` |
| Config file | `config.json` → `"bot": { "computer_use": { "enabled": true } }` |

The JSON value wins over the environment variable. Default is `false`, and
while disabled the tool is not registered at all — the model cannot see or
call it, and no schema is sent.

Control plane: PATCH `/agents/<name>/config` with
`{"bot":{"computer_use":{"enabled":true}}}`. `{"bot":["computer_use"]}` on
`/config/reset` restores the template default.

## 2. macOS permissions

Three **separate** permissions are involved. They live in different panes of
**System Settings → Privacy & Security**, and granting one does **not** grant
the others:

| Permission | Pane | Used by | Needed for | Failure signature |
|---|---|---|---|---|
| **Screen Recording** | Privacy & Security → Screen Recording | `screencapture` | `screenshot` | `screencapture: could not create image from display` |
| **Accessibility** | Privacy & Security → Accessibility | `cliclick` (input synthesis) | `move`, `click`, `double_click`, `right_click`, `drag`, `type`, `key` | `cliclick` errors / input silently ignored |
| **Automation** | Privacy & Security → Automation → *[app]* → **System Events** | AppleScript / `osascript` | `list_windows`, `focus_app`, `scroll`, desktop-size lookup (Finder) | `Not authorized to send Apple events to System Events (-1743)` |

**Who gets the grant.** macOS attributes the request to the *responsible app*,
not to `myaaw` or `cliclick`:

- `myaaw chat` run from a terminal → grant the **terminal app** (Terminal,
iTerm2, Ghostty, WezTerm, …). If the terminal is integrated in an editor
(VS Code, Zed, …), grant the **editor**.
- The daemon (`myaaw api run` / `server start`) → grant the **`myaaw` binary**
itself. **Every rebuild changes an unsigned binary's identity**, so all three
grants can silently become stale: the toggle still shows ON but macOS treats
the new build as a different app and re-prompts (symptom: `screenshot` fails
while actions granted after the rebuild keep working). Re-grant after a
rebuild, or sign the binary with a stable certificate (see below).
- Terminal and daemon are separate entries: granting one does not cover the other.

`myaaw doctor` prints **Launched from** so you can see which app is responsible
for the current shell, and probes Screen Recording and Automation
(non-destructively). Accessibility has no safe probe — granting it is verified
by actually using a mouse/keyboard action.

**Stale grants after a rebuild.** Rebuilt unsigned binaries get a new code
identity, so old grants stop applying even though the toggles look ON. Remove
the stale `myaaw` row with the `−` button in the pane, trigger the action
again, allow the fresh prompt, then restart the daemon. To avoid this loop,
sign the binary with a stable self-signed certificate so its identity survives
rebuilds:

```bash
make codesign-setup   # one time: creates the "Myaaw Agent" certificate
make install          # every build: signs automatically after building
make sign             # sign an existing bin/myaaw without rebuilding
```

The first signing run shows a keychain dialog (*"codesign wants to access key
'cert'"*): enter the macOS login password and choose **Always Allow** so
later builds sign without prompts.

The first signed install changes the identity from ad-hoc to the certificate,
so grant the three permissions once more; afterwards they survive rebuilds.

**Lifetime.** The certificate is valid for 10 years (`-days 3650`). TCC grants
are per machine and per identity: when the certificate expires, when you
replace it, or when you move to a new machine, create a new one and grant the
three permissions once again.

**Removing the identity.** `myaaw codesign remove` deletes the certificate and
its code-signing trust settings (confirmation prompt; `--yes` to skip).
Existing binaries stay signed, but new builds/updates stop being signed and
fall back to ad-hoc, so permissions reset on every update again. To go back to
ad-hoc immediately, replace the binary atomically:

```bash
cp "$(command -v myaaw)" /tmp/myaaw && codesign --force --sign - /tmp/myaaw && mv /tmp/myaaw "$(command -v myaaw)"
```

**Downloaded builds (`install.sh` / `myaaw update`).** Both paths re-sign the
new binary with the local "Myaaw Agent" identity when it exists, so the
grants survive updates:

- `install.sh` signs after installing; if the identity is missing it prints a
  hint instead of failing.
- `myaaw update` signs after replacing the binary and reports the result.
- **No source checkout needed:** `myaaw codesign setup` creates the local
  identity (this is what the installer points you to), and
  `myaaw codesign status` shows whether the identity exists and whether the
  running binary is signed. With a checkout, `make codesign-setup` does the
  same thing via `scripts/setup-codesign.sh`.

Without a local identity, the downloaded binary stays ad-hoc signed, so every
update changes the code identity and users must re-grant once after each
update. With the identity present, `myaaw update` prints
`Signed with "Myaaw Agent"` and **no re-grant is needed**; if it prints
`Not re-signed: …`, the next computer action will prompt again.

Signing a binary you downloaded by hand works the same way — the local
certificate and identifier decide the identity, not the file contents:

```bash
codesign --force --identifier com.myaaw.cli --sign "Myaaw Agent" ~/.local/bin/myaaw
xattr -d com.apple.quarantine ~/.local/bin/myaaw 2>/dev/null || true
```

Check whether a downloaded binary is already signed before doing anything:

```bash
codesign -dv --verbose=2 ~/.local/bin/myaaw 2>&1 | grep -E "Identifier|Authority|Signature"
```

- `Identifier=com.myaaw.cli` + `Authority=Myaaw Agent` → already signed; no
action needed (a grant for "Myaaw Agent" keeps working).
- `Signature=adhoc` (or no `Authority` line) → the binary is unsigned; sign it
with the command above.

`install.sh` and `myaaw update` run the signing step automatically when the
"Myaaw Agent" identity exists, and print `Signed with "Myaaw Agent"` or
`Not re-signed: …`. The manual commands are for binaries downloaded by other
means, or when the identity was missing at install time. After signing by
hand, restart the processes; if the existing grant is already for "Myaaw
Agent", no new prompt appears.

Replace the installed binary **atomically** (`mv`), not by overwriting it with
`cp`: overwriting a signed executable in place can leave a stale kernel code
signature cache, and macOS kills the next run with `Killed: 9` even though
`codesign -dv` looks valid. `make install`, `install.sh`, and `myaaw update`
already replace the file safely.

For release binaries consumed by many machines, sign in CI with an **Apple
Developer ID** instead of a per-machine self-signed certificate: the identity
is then stable for every user and survives updates without local setup. The
release workflow (`.github/workflows/release.yml`) has a guarded step that only
runs when these repository secrets are set:

| Secret | Value | Why it exists |
|---|---|---|
| `MACOS_CERT_P12` | base64 of the Developer ID `.p12` | the runner imports cert + private key into a temporary keychain |
| `MACOS_CERT_PASSWORD` | password of the `.p12` | unlocks the bundle during `security import` |
| `MACOS_CERT_IDENTITY` | `Developer ID Application: Name (TEAMID)` | exact common name passed to `codesign --sign` |

**1. Why CI needs a certificate at all.** The GitHub macOS runner is a fresh,
ephemeral machine; none of your local keychain entries exist there. To give
release binaries a stable code identity, CI must carry the certificate with it.

**2. What happens without the secrets.** The signing step prints
`No Developer ID certificate configured; skipping signing.` and exits 0, so
releases still succeed — but the binary is ad-hoc signed. On macOS every
update then looks like a new app, and users must re-grant Screen Recording,
Accessibility, and Automation after each update. This is convenience, not a
security boundary: an unsigned binary still runs, it just loses TCC grants.

**3. Why `make install` needs no secrets.** `make install` / `make sign` run on
your machine and use the local "Myaaw Agent" certificate from your login
keychain (created once with `make codesign-setup`). There are two independent
signing paths:

| Path | Certificate | Use |
|---|---|---|
| `make install` / `make sign` / `myaaw update` | "Myaaw Agent" (local, self-signed) | development and personal machines |
| GitHub Actions release | Developer ID (via the three secrets) | distribution to many macOS users |

If both exist, `install.sh` / `myaaw update` re-sign a downloaded binary with
the local certificate, which replaces the Developer ID signature (still a
stable identity, per machine). Pick one strategy per use case. Notarization is
still required for Finder/Gatekeeper launches; TCC persistence only needs the
stable signature.

**Filling the secrets (optional — Apple Developer Program required).**
This needs a paid Apple Developer Program membership (~US$99/year); without it
there is no legitimate Developer ID certificate. Skipping it is fine: releases
still build and run. Without a stable signature, macOS users re-grant the three
permissions after each update, or sign locally with `myaaw codesign setup`. The
CI step is `continue-on-error`, so a missing, expired, or invalid certificate
only logs a warning and never blocks a release.

1. **Create a "Developer ID Application" certificate** — on the same machine
   whose keychain will hold the private key (the private key cannot be moved
   casually):
   1. Keychain Access → menu **Keychain Access → Certificate Assistant →
      Request a Certificate From a Certificate Authority**.
   2. Enter your email and name, choose **Saved to disk**, and save
      `CertificateSigningRequest.certSigningRequest`.
   3. developer.apple.com → **Certificates, Identifiers & Profiles →
      Certificates → +** → **Developer ID Application** → upload the CSR →
      download the `.cer`.
   4. Double-click the `.cer` to import it into the login keychain (it pairs
      automatically with the private key created in step 1.1).

2. **Export the `.p12` (certificate + private key)**:
   1. Keychain Access → **login** → category **My Certificates**.
   2. Find **`Developer ID Application: Name (TEAMID)`** → right-click →
      **Export**.
   3. Format **Personal Information Exchange (.p12)** → save, then set an
      export password.
   4. ⚠️ That password becomes `MACOS_CERT_PASSWORD`; do not lose it.

3. **Collect the secret values**:
   ```bash
   # MACOS_CERT_P12 — single-line base64 (safe as a secret)
   base64 -i cert.p12 | tr -d '\n' | pbcopy

   # MACOS_CERT_IDENTITY — exact name including Team ID
   security find-identity -v -p codesigning
   # e.g. "Developer ID Application: Shiyinq (ABCDE12345)" — copy without quotes

   # MACOS_CERT_PASSWORD is the export password from step 2 (no command)
   ```

4. **Add them to GitHub** — repo → **Settings → Secrets and variables →
   Actions → New repository secret** (names must match exactly):

   | Name | Value |
   |---|---|
   | `MACOS_CERT_P12` | base64 output from step 3 |
   | `MACOS_CERT_PASSWORD` | the `.p12` export password |
   | `MACOS_CERT_IDENTITY` | `Developer ID Application: Name (TEAMID)` |

5. **Verify** — push a `v*` tag, open the macOS job log, and check the
   **Sign binary (macOS, optional)** step. Then verify the artifact:
   ```bash
   codesign -dv --verbose=2 myaaw-darwin-arm64 2>&1 | grep -E "Identifier|Authority"
   # Authority=Developer ID Application: ...
   ```

**Notes.**

- Developer ID certificates are valid for ~5 years. Renew with the **same Team
  ID**; the TCC identity stays stable, so users do not re-grant.
- Never commit the `.p12` or its password; delete the temp files after pasting
  the secrets.
- Only the macOS matrix is signed; the Linux artifact is untouched.
- `base64 --decode` in the workflow works on the macOS runner (BSD base64
  supports both `--decode` and `-D`).
- Notarization is separate and only matters for Finder/Gatekeeper launches;
  TCC persistence needs only the stable signature.

**After granting — restart the processes.** TCC is evaluated when a process
starts, so a process that was already running keeps the old decision. Grant
first, then restart:

| What | How | Notes |
|---|---|---|
| Control plane | `myaaw api restart` (or stop and `myaaw api run`) | If it spawns agent servers, this restarts them too |
| Agent server | `myaaw --agent <name> server restart` | This process actually executes the `computer` tool |
| Chat | Ctrl+C, then `myaaw chat` | Each chat session is its own process |

Grants are attached to the **binary** (`com.myaaw.cli`), not to an agent name,
so one grant covers every agent — but every already-running agent process must
be restarted to pick it up. Screen Recording in particular only applies on
relaunch; Accessibility is often cached too. Verify from the real channel
(Telegram/Discord), not only with `myaaw doctor` (which tests the terminal
identity).

Restarting by itself **never triggers a new prompt**: it only lets a process
pick up a grant that was made while it was running. Prompts appear when an
action is attempted by a binary whose identity has no matching grant — for
example after replacing the binary without re-signing it.

**Removing / resetting permissions.** The Screen Recording and Accessibility
panes have a **`−`** button to remove an app. The **Automation** pane has no
remove button — entries are per *app → target* and are cleared only with
`tccutil`:

```bash
tccutil reset AppleEvents                        # all apps, all Automation grants
tccutil reset AppleEvents com.mitchellh.ghostty  # one bundled app (e.g. Ghostty)
tccutil reset ScreenCapture                      # Screen Recording for ALL apps
tccutil reset Accessibility                      # Accessibility for ALL apps
tccutil reset All                                # every TCC service (heavy-handed)
```

`myaaw` is a bare executable, so it has no bundle ID and cannot be targeted
individually — reset the whole service. After a reset every affected app asks
again on its next request, so re-grant and restart the processes. Use the
per-service resets only when needed: `ScreenCapture`, `Accessibility`, and
`All` also reset other apps (Chrome, iTerm, …); `All` resets camera,
microphone, and the rest as well.

If the Automation toggle for System Events is missing (the consent prompt was
dismissed earlier), reset and re-trigger it:

```bash
tccutil reset AppleEvents   # or: tccutil reset AppleEvents com.mitchellh.ghostty
myaaw doctor
```

Then click **Allow** on the “wants to control System Events” dialog.

**Recommended:** `brew install cliclick`. Without it, `move`,
`right_click`, `double_click`, and `drag` fail; `click`, `type`, and `key`
fall back to AppleScript with reduced coverage (e.g. modifiers on named
keys like `cmd+return` require cliclick). cliclick itself runs under the
caller's Accessibility grant; if macOS asks specifically for
`/opt/homebrew/bin/cliclick`, add it to the Accessibility pane as well.

**Platforms & distribution.**

- **macOS, local build/install** — permissions are per machine. The binary is
  signed with "Myaaw Agent" by `make install`, `myaaw update`, and `install.sh`
  (when the identity exists), so grants survive rebuilds and updates.
- **macOS, GitHub release** — the release binary is signed only when the CI
  secrets `MACOS_CERT_P12` / `MACOS_CERT_PASSWORD` / `MACOS_CERT_IDENTITY` are
  configured. With them, users grant once and updates keep the grants. Without
  them the binary is ad-hoc signed, so macOS treats every update as a new app
  and users must re-grant. `install.sh` re-signs the download on machines that
  ran `make codesign-setup`, but users who only download binaries have no local
  identity.
- **Linux / servers** — desktop automation is unavailable: every action except
  `wait` returns `COMPUTER_UNSUPPORTED_PLATFORM`, `codesign` is a no-op, and
  `install.sh`/Makefile skip signing. Keep `computer_use.enabled` off (the
  default) on servers. A headless macOS server is equally unusable for
  `screenshot` because there is no GUI session.

## 3. Actions

One typed action per call. Coordinates (`x`/`y`) are **screenshot pixels**;
the tool remaps them to screen points automatically (see §5).

| Action | Arguments | What it does | Approval key |
|---|---|---|---|
| `screenshot` | `deliver` (bool) | Capture the screen, return as image | `computer:screenshot` |
| `move` | `x`, `y` | Move pointer | `computer:move` |
| `click` | `x`, `y`, `button` (`left`/`right`/`middle`, default `left`) | Single click | `computer:click` |
| `double_click` | `x`, `y` | Double click | `computer:double_click` |
| `right_click` | `x`, `y` | Right click | `computer:right_click` |
| `drag` | `x`, `y`, `x2`, `y2` | Press at start, move, release at end | `computer:drag` |
| `scroll` | `direction` (`up`/`down`, default `down`), `amount` (default 3) | Page-scroll the focused element | `computer:scroll` |
| `type` | `text` | Type text | `computer:type` |
| `key` | `keys` (e.g. `cmd+shift+t`) | Press a key combination | `computer:key` |
| `wait` | `duration` ms (default 500, max 60000) | Sleep | `computer:wait` |
| `list_windows` | — | Visible apps + window titles | `computer:list_windows` |
| `focus_app` | `window` (app name **or** window title) | Bring to front | `computer:focus_app` |

Limitations to keep in mind:

- `scroll` sends page-up / page-down key codes (`amount` times) to the
  focused element; pixel-precise wheel scrolling is not supported and
  `x`/`y` are ignored.
- `focus_app` tries an exact process name first, then falls back to a
  window-title `contains` match.
- Actions that change the screen are **not** followed by an automatic
  screenshot — ask for `screenshot` explicitly when the model needs to see
  the result.

## 4. Screenshots

- Captured with `screencapture -x` (no sound, no cursor UI).
- If either dimension exceeds **1568 px**, the PNG is downscaled with
  `sips` so the model's context stays small (see image budget below).
- **Dedup:** screenshots are hashed; if the screen is unchanged since the
  last capture, no image is returned — only a text note with the frame ID.
- The result text tells the model the frame size and, when known, the
  logical desktop size: `Screen is 1440x900 points.`
- `deliver: true` additionally saves the PNG to
  `~/.myaaw/<agent>/jobs/captures/screenshot-<frame>.png` (newest 20 kept)
  and attaches it to the user's chat — Telegram photo, Discord attachment,
  CLI/API attachment.

## 5. Coordinate mapping (Retina)

`cliclick` and AppleScript work in **logical points**, but the PNG the model
sees may be larger (Retina 2x) or downscaled (§4). Before every pointer
action (`move`, `click`, `double_click`, `right_click`, `drag`), the tool
maps the model's coordinates:

```
screen_x = round(x * display_width  / png_width)
screen_y = round(y * display_height / png_height)
```

`display_width`/`display_height` come from the Finder desktop bounds. If
the display size is unavailable, coordinates pass through unchanged. The
mapping is based on the **most recent screenshot**, so the model should
screenshot first, then act on what it sees.

## 6. Approvals & security

Every action maps to the approval key `computer:<action>` (last column in
§3). Read-only actions are whitelisted by default in the template
`approvals.allow`:

```json
"allow": [
  "computer:screenshot",
  "computer:wait",
  "computer:list_windows"
]
```

Everything that touches the desktop (`move`, `click`, `type`, `key`, …)
therefore prompts for approval under `mode: manual`. Considerations:

- Keep `mode: manual` (the default) — desktop control is powerful.
- `unattended: deny` (or explicit `deny` entries) prevents desktop actions
  in cron / heartbeat / headless / inbound A2A runs.
- `always` grants are scoped per **channel + user** (see
  [approvals](approvals.md)); a grant on Telegram does not extend to the
  control-plane chat.
- All decisions are audited to `logs/approvals.jsonl` including tool name
  and full arguments.

## 7. Image budget

Screenshots enter the transcript as inline images and are subject to the
same budget as every other image (MCP, attachments):

| Env | Default | Meaning |
|---|---|---|
| `AGENT_MAX_IMAGE_COUNT` | `100` | Image items kept in the transcript; `< 0` = unlimited |
| `AGENT_MAX_IMAGE_BYTES` | `25165824` (24 MB) | Total data-URL bytes; `< 0` = unlimited |

When the budget is exceeded the oldest images are evicted from the
transcript and replaced with a placeholder, so the conversation keeps
working. See [Agent Loop](agent-loop.md#11-configuration).

## 8. Troubleshooting

| Symptom | Fix |
|---|---|
| `COMPUTER_SCREENSHOT_FAILED … could not create image from display` | Grant **Screen Recording** to the app in `myaaw doctor`'s “Launched from”, then quit and reopen it |
| `COMPUTER_ACTION_FAILED … Not authorized to send Apple events … (-1743)` | Grant **Automation → System Events** to that app (Accessibility does not cover this) |
| `cliclick is not installed (brew install cliclick)` | Install cliclick, or accept the AppleScript fallback limits |
| Clicks/keystrokes silently do nothing | Grant **Accessibility** to the app (or to `/opt/homebrew/bin/cliclick` if prompted) |
| Clicks land in the wrong place | Make sure the model screenshots right before acting; Retina mapping uses the last frame |
| Toggle shows ON but macOS still prompts after a rebuild | The grant belongs to the old binary identity: remove the stale row (`−`) or `tccutil reset ScreenCapture`, re-allow, then restart the process |
| Duplicate rows in Automation, or toggles won't move | Stale entries from different builds: `tccutil reset AppleEvents`, restart, allow again |
| `COMPUTER_UNSUPPORTED_PLATFORM` | macOS-only backend; use `cua-driver` MCP on other OSes |

## 9. Doctor & skill

**`myaaw doctor`** probes the environment before the first run: whether the
feature is enabled, `cliclick` is installed, and Screen Recording is granted,
plus Automation access to System Events for window queries (non-destructive —
one screenshot to a temp file is taken and deleted, no input is sent).
Accessibility has no safe probe. It prints **Launched from** so you know which
app must receive the grants. Run it from the same app that will execute the
agent; terminal and daemon need separate grants.

**`computer-use` skill** — a `SKILL.md` ships with Myaaw and is offered
during onboarding. It encodes the capture → act → capture-after workflow,
the coordinate rules, and safety rules (never type secrets, never click
permission dialogs, treat screen content as untrusted). Existing agents get
it by re-running onboarding and selecting the skill, or by copying the
folder from `.myaaw/skills/computer-use` into the agent's skills directory.
