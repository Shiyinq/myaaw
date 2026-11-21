# Backup Guide

`myaaw backup` writes one portable `tar.gz` archive per agent, and `myaaw restore` verifies and restores it. An archive mirrors the on-disk layout (`<agent>/...`), so it can also be extracted by hand under `~/.myaaw`.

The dashboard exposes the same operations on the **Backups** page (`/app/backups`): list the archive directory, create an archive for one agent or the whole fleet, download it, and restore or delete an archive. The routes are documented in the [Control Plane API guide](../api/control-plane.md#22-backups).

## Quick start

```bash
# Back up every initialized agent plus the global registry files
myaaw backup

# Back up one agent
myaaw backup --agent agent-b

# Back up two agents, write elsewhere
myaaw backup --agent agent-b,agent-c -o ~/Backups

# Include the workspace and logs
myaaw backup --agent agent-b --include-workspace --include-logs
```

Output:

```text
agent-b: 36 files, 146.7 KB -> ~/.myaaw-backups/myaaw-backup-agent-b-20261027-002704.tar.gz
```

## Agent names

Agent names must match `[A-Za-z0-9][A-Za-z0-9_-]*` and be at most 64 characters. Hyphens and underscores are safe, for example `agent-b`, `agent_working`, or `agent-personal-working`. The name must start with a letter or digit, so `-agent` and `_agent` are invalid. `pids` is reserved.

Prefer lowercase names: macOS filesystems are usually case-insensitive, so `Agent-B` and `agent-b` would refer to the same directory. Avoid naming an agent `global`, because that name is used for the registry archive (`myaaw-backup-global-*.tar.gz`).

## Where archives go

Priority:

1. `--output <dir>` / `-o`
2. `MYAAW_BACKUP_DIR`
3. `~/.myaaw-backups`

Archives are written **outside** `~/.myaaw` on purpose: folders under `~/.myaaw/` are reserved for agents.

If `-o` points inside the agent being backed up, that directory is skipped and recorded in the manifest, so an archive never contains itself.

## What is included

Everything under `~/.myaaw/agents/<agent>/` by default, except the deny-list below:

```text
.env
contacts.json
config/*                # whole directory
database/*              # whole directory, SQLite via snapshot
home/*                  # markdown, memory/, scripts/, media/, skill data
skills/*                # includes skills.json when present
tools/*
cron/*                  # except runs/
jobs/*                  # except *.log
```

There is no hardcoded list of skill directories: anything a skill writes under `config/` or `database/` is included automatically.

## What is skipped

| Path | Why | Include with |
| :--- | :--- | :--- |
| `workspace/` | user working files, can be large | `--include-workspace` |
| `logs/`, `.logs/` | runtime logs | `--include-logs` |
| `cron/runs/` | cron run history | `--include-logs` |
| `jobs/*.log` | job logs | `--include-logs` |
| `home/.venv/` | Python virtualenv, platform-specific | `--include-venv` |
| `.playwright-mcp/` | browser profile/cache | `--include-caches` |
| `tmp/` | runtime scratch | `--include-caches` |
| `.DS_Store`, `*.tmp`, `*.part` | junk | never |
| `myaaw.db-wal`, `myaaw.db-shm` | replaced by the database snapshot | never |

Symlinks are skipped and listed in `manifest.json` under `skipped`.

## Database snapshot

`database/myaaw.db` is not copied as a raw file. SQLite writes recent changes to `myaaw.db-wal` first, so copying the main file while a daemon is running can produce an inconsistent backup.

Instead the backup runs:

```sql
VACUUM INTO '<temporary>/myaaw.db'
```

The result is a complete, consistent single-file snapshot, produced while the server keeps running. It is stored at the same archive path (`<agent>/database/myaaw.db`), so restore needs no special handling.

Other files in `database/` are copied normally.

## Python virtualenv

Each agent has one venv at `home/.venv`, created automatically by the Python tool and shared by all skills. It is excluded by default because it is large (19–273 MB) and tied to a specific OS/architecture.

The backup still records how to rebuild it:

```text
.myaaw-backup/python-version.txt
.myaaw-backup/venv-packages.txt   # output of pip freeze, when non-empty
```

Use `--include-venv` only when you need an exact copy and will restore on the same platform.

## Archive layout

```text
manifest.json
.myaaw-backup/
  registry-entry.json
  python-version.txt
  venv-packages.txt
default/
  .env
  config/config.json
  contacts.json
  database/myaaw.db
  home/...
  skills/...
  tools/...
  cron/...
  jobs/...
```

`--all` is not a flag: when no `--agent` is given, every initialized agent is backed up and an extra global archive is written.

```text
myaaw-backup-global-<timestamp>.tar.gz
  agents.json                     # restored to config/agents.json
  controlplane/controlplane.db    # SQLite snapshot restored to controlplane/controlplane.db
  config/model_capabilities.json
  config/model_prices.json
  config/tool_prices.json
```

`controlplane/controlplane.db` is captured as a live single-file SQLite snapshot via `VACUUM INTO` (no `-wal`/`-shm` side files) and holds control-plane credentials, UI settings, and multi-agent group chats. With `--no-secrets`, `auth_account` is deleted from the temporary snapshot (`DELETE FROM auth_account; VACUUM;`) so UI settings and group chats are still backed up without leaking `password_hash`; on restore to a fresh host, `myaaw api` automatically seeds default credentials (`myaaw` / `myaawadmin`, `role: "admin"`, `must_change_password: true`) while preserving the restored settings and groups.

## Manifest

`manifest.json` records format version, myaaw version, agent name, timestamp, hostname, include/exclude sets, the skipped list, and every file with size, mode, and SHA-256.

Archives are written to a temporary file, synced, then renamed. Permissions: archive `0600`, output directory `0700`.

## Flags

| Flag | Description |
| :--- | :--- |
| `--agent` | Comma-separated agent names (global flag). Omit to back up all initialized agents. |
| `-o`, `--output` | Output directory, or `-` to stream a single agent archive to stdout. |
| `--include-workspace` | Include `home/workspace`. |
| `--include-logs` | Include `logs/`, `.logs/`, `cron/runs/`, and `jobs/*.log`. |
| `--include-venv` | Include `home/.venv` (same platform only). |
| `--include-caches` | Include `.playwright-mcp/` and `tmp/`. |
| `--no-secrets` | Exclude `.env`, blank `api_key` in `config/config.json`, blank tokens in `contacts.json`, strip `auth_account` from the `controlplane/controlplane.db` snapshot (preserving settings and group chats). |
| `--json` | Print a machine-readable summary. |

## Restore

```bash
# Pick a backup interactively from the default directory
myaaw restore

# Pick from a custom directory
myaaw restore --dir ~/Backups

# Restore a specific archive
myaaw restore <archive> [--force] [--restore-packages] [--no-start]
```

When no archive path is given, restore lists the archives in `--dir`, `MYAAW_BACKUP_DIR`, or `~/.myaaw-backups` (newest first) and opens an arrow-key picker. If stdin is not a terminal, pass the archive path explicitly.

What it does:

1. Extracts into a staging directory under `~/.myaaw` and rejects absolute paths, `..`, backslashes, symlinks, and duplicate entries.
2. Verifies every file against `manifest.json` (size and SHA-256) and rejects unlisted files.
3. Refuses to replace an existing agent unless `--force` is given. A directory that only contains runtime logs (from an earlier CLI run) is replaced without `--force`.
4. With `--force`: stops the agent daemon, writes a pre-restore backup to `~/.myaaw-backups`, replaces the agent directory, and sets `.env`, `contacts.json`, and `config/config.json` to mode `0600`.
5. Registers the agent in `~/.myaaw/config/agents.json`, warns when its `PORT` is already used by another agent, and starts the daemon unless `--no-start` is given.

Examples:

```bash
# Restore into a fresh install
myaaw restore myaaw-backup-agent-b-20261027-002704.tar.gz --no-start

# Choose from the list interactively
myaaw restore

# Replace an existing agent (writes a pre-restore backup first)
myaaw restore myaaw-backup-agent-b-20261027-002704.tar.gz --force

# Restore under a different agent name
myaaw --agent agent-b-copy restore myaaw-backup-agent-b-20261027-002704.tar.gz

# Rebuild the Python venv from the archived pip freeze
myaaw restore archive.tar.gz --restore-packages

# Machine-readable summary
myaaw restore archive.tar.gz --json
```

Restore flags:

| Flag | Description |
| :--- | :--- |
| `--force` | Replace an existing agent (or global files). |
| `--restore-packages` | Recreate `home/.venv` and run `pip install -r` from the archived `pip freeze`. |
| `--no-start` | Do not start the agent daemon after restoring. |
| `--json` | Print a JSON summary. |
| `--dir` | Directory to search when no archive path is given. |
| `--agent` | Global flag: restore under a different agent name. |

### Global archives

```bash
myaaw restore myaaw-backup-global-20261027-002704.tar.gz --force
```

Restores `agents.json` and the model tables into `~/.myaaw/config/`, and `controlplane/controlplane.db` into `~/.myaaw/controlplane/controlplane.db` (mode `0600`). Existing files are copied to `<name>.pre-restore-<timestamp>` first, and stale `controlplane.db-wal` / `controlplane.db-shm` side files are removed so SQLite never replays old WAL frames onto the restored database. Restart the control plane API afterwards.

### Manual extraction

If you need to inspect or extract an archive by hand:

```bash
myaaw --agent default server stop
tar -xzf myaaw-backup-default-20261027-003000.tar.gz -C ~/.myaaw/agents default/
chmod 600 ~/.myaaw/agents/default/.env ~/.myaaw/agents/default/contacts.json
sqlite3 ~/.myaaw/agents/default/database/myaaw.db "PRAGMA integrity_check;"
myaaw --agent default server start
```

## Verification

```bash
# List contents
tar -tzf myaaw-backup-agent-b-20261027-002704.tar.gz

# Check one file against the manifest
tar -xOzf archive.tar.gz default/config/config.json | shasum -a 256
```

Every `files[]` entry in `manifest.json` has the expected `sha256`.

## Security

- Archives contain `.env`, provider API keys, A2A tokens, and `controlplane/controlplane.db` (which includes the control-plane password hash unless `--no-secrets` is used).
- Archives are created with mode `0600`; the default directory uses `0700`.
- Use `--no-secrets` for shareable backups.
- Nothing in the backup process prints secret values.

## Limitations

- No built-in encryption (`age`/`gpg` integration is planned). Store archives on encrypted disks.
- `--include-venv` is platform-specific and may not work across machines.
- `--restore-packages` needs network access and a local `python3`.
- Backups are a point-in-time snapshot of files; JSON files written concurrently (for example cron config) are copied as they are.
- A global restore updates `agents.json` and the model tables under `~/.myaaw/config/`, plus `controlplane/controlplane.db` under `~/.myaaw/controlplane/`; restart the control plane API afterwards.
- No automatic retention/pruning yet. Archive names are timestamped; delete old files manually.
