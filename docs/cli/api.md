# API Command

`myaaw api` runs the unified control plane: one HTTP endpoint and one token for web, desktop, and mobile clients. Every request targets an agent through the path (`/agents/<name>/...`), and chat is proxied to that agent's own server.

The endpoints, auth flow, and payloads are documented in the [Control Plane API guide](../api/control-plane.md). This page covers only the CLI lifecycle.

## Commands

```bash
myaaw api            # same as: myaaw api run
myaaw api run
myaaw api start
myaaw api stop
myaaw api restart
myaaw api status
```

All subcommands accept these persistent flags:

| Flag | Default | Meaning |
| :--- | :--- | :--- |
| `--port` | `9000` | Port the control plane listens on. |
| `--host` | `127.0.0.1` | Interface to bind. Loopback by default; use `0.0.0.0` only behind a tunnel or reverse proxy. |
| `--terminal` | `false` | Allow the dashboard to open a shell on this host (sets `MYAAW_TERMINAL=1`). |
| `--allowed-origins` | loopback only | Comma-separated browser origins allowed to call the API (sets `ALLOWED_ORIGINS`). Needed when the dashboard runs on another origin, such as a tunnel domain. |

```bash
myaaw api start --port 9100
myaaw api start --host 127.0.0.1 --port 9000
myaaw api start --allowed-origins https://panel.example.com
```

## Web terminal

Shells are off unless the control plane is started with `--terminal`. The flag exists because the daemon unit carries no environment, so an exported `MYAAW_TERMINAL` never reaches it:

```bash
myaaw api restart --terminal     # enable, and record the flag in the daemon unit
myaaw api restart                # restart without the flag turns it off again
```

`start` and `restart` pass the flag on to the child process (`myaaw api run --port <port> --terminal`), which is what makes it survive a reboot. For a foreground run, either form works:

```bash
myaaw api run --terminal
MYAAW_TERMINAL=1 myaaw api run
```

It grants a full shell as the user running the API, so read the [Web Terminal guide](../api/terminal.md) before enabling it.

## Remote dashboard (tunnel)

The control plane stays on loopback; the tunnel only forwards to that port. A
browser dashboard on another origin must be allowed explicitly, otherwise its
requests fail the CORS preflight:

```bash
myaaw api start --allowed-origins https://panel.example.com
myaaw api restart --allowed-origins https://panel.example.com
```

The flag is passed on to the daemon (`myaaw api run ... --allowed-origins ...`),
because daemon units carry no environment. An exported `ALLOWED_ORIGINS` is
forwarded for `api start` too, and `api run` reads it directly. Use
`ALLOWED_ORIGINS=*` only for local development.

## `api run` (and bare `api`)

Runs the control plane in the foreground, which is what you want while debugging:

```bash
myaaw api run --port 9000
```

Logs go to the terminal. Stop it with `Ctrl+C`. Running `myaaw api` without a subcommand does the same thing.

## `api start`

Starts the control plane as a background daemon named `myaaw-api`:

```bash
myaaw api start --port 9000
```

The daemon runs `myaaw api run --port <port>` and writes:

```text
PID:  ~/.myaaw/pids/myaaw-api.pid
Log:  ~/.myaaw/logs/myaaw-api.log
```

## `api stop`

Stops the `myaaw-api` daemon and removes its PID file. It is safe to run when the daemon is already stopped.

## `api restart`

Stops the daemon, waits one second, then starts it again with the same port. Use it after changing control plane credentials or rebuilding the binary. `--terminal` is not sticky: it is recorded only when you pass it, so `myaaw api restart` also revokes web terminal access.

## `api status`

Prints whether the daemon is running and, when it is, the Swagger UI URL:

```text
✅ API is running (PID: 48213)
   http://localhost:9000/docs/
```

Otherwise it prints `❌ API is stopped`.

## Examples

```bash
# Background control plane plus the agents it should reach
myaaw server --agent agent-b start
myaaw server --agent agent-c start
myaaw api start --port 9000
myaaw api status

# Foreground for debugging
myaaw api run --port 9000

# After a rebuild or credential change
myaaw api restart
```

## Notes

- **Global, not per agent.** `myaaw api` is the only daemon that is not tied to one agent; `--agent` has no effect on it. The target agent comes from the request path.
- **Loopback by default.** `--host` defaults to `127.0.0.1`, so the control plane is only reachable from the same machine. Expose it with a tunnel or SSH rather than binding `0.0.0.0` directly; see [Remote WebUI Connection](../webui/remote-connection.md).
- **Port must match.** `start`, `stop`, `restart`, and `status` each read `--port` independently. Use the same value you started with, otherwise `status` reports the wrong endpoint.
- **Agents must be running.** Chat is proxied, so the selected agent needs its own `myaaw server --agent <name> start`.
- **Authentication is not optional.** Credentials live in `~/.myaaw/controlplane.json` (bcrypt hash, never plaintext) and clients send `Authorization: Bearer <token>`.
- **`--terminal` is a host credential.** Whoever can log into the dashboard can run anything as the user that started the daemon. Leave it off unless the control plane stays on loopback.
- **Daemon ports are independent.** The control plane on `9000` and agent servers on `8080`, `8081`, ... are separate processes with separate logs.

## Related

- [Control Plane API guide](../api/control-plane.md)
- [Web Terminal](../api/terminal.md)
- [WebUI Command](webui.md)
- [Remote WebUI Connection](../webui/remote-connection.md)
- [Service Management](service.md)
- [Agent Command](agent.md)
- [Logs](logs.md)
