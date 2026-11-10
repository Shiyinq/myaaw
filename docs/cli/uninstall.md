# Uninstall

`myaaw uninstall` removes Myaaw from the machine and leaves nothing behind except the archives you choose to keep.

```bash
myaaw uninstall              # confirm, then remove everything
myaaw uninstall --dry-run    # show the plan and exit
myaaw uninstall --yes        # no confirmation prompt
myaaw uninstall --keep-binary        # keep the myaaw binary
myaaw uninstall --purge-backup       # delete ~/.myaaw-backups without asking
```

## What it does

1. Stops the WebUI, the control plane API, and every registered agent **one at a time**, printing one line per agent (`stopping agent "agent-b" (PID …)`, `agent "agent-c" was not running`).
2. Removes `~/.myaaw` (agents, config, databases, logs, WebUI bundle, PID files).
3. Removes leftover service definitions (`~/Library/LaunchAgents/myaaw-*.plist`, `~/.config/systemd/user/myaaw-*.service`), including definitions whose agent is no longer registered.
4. Removes the running binary (`myaaw`), plus `myaaw.old` and `myaaw.tmp` left by earlier updates.
5. Shows the archives in `~/.myaaw-backups` and asks whether to delete them.

**No backup is created by this command.** Run [`myaaw backup`](backup.md) first if you want an archive of your agents.

## Backup archives

The archives live outside `~/.myaaw` on purpose, so a repository or install cleanup cannot take them down. After the removal finishes, the command lists every entry and asks:

```text
Backup archives: /Users/<you>/.myaaw-backups
  2026-11-10T10-30-00
  2026-11-08T09-12-44
Delete every backup in /Users/<you>/.myaaw-backups? [y/N]:
```

- Answer `y` to delete the whole folder.
- Answer `n` (or press Enter) to keep it; the path is printed again.
- `--purge-backup` answers `y` automatically, which makes the run fully non-interactive together with `--yes`.
- `--yes` alone keeps the archives, so a scripted uninstall never destroys backups by accident.

The prompt is skipped when there is nothing to delete. The folder is the same one `myaaw backup` uses: `MYAAW_BACKUP_DIR` when set, otherwise `~/.myaaw-backups`.

## Flags

| Flag | Meaning |
| :--- | :--- |
| `--yes`, `-y` | Skip the confirmation prompt. |
| `--dry-run` | Print the plan (agents, paths, binary, archives) and exit. |
| `--keep-binary` | Leave the `myaaw` binary in place. |
| `--purge-backup` | Delete everything under `~/.myaaw-backups` without asking. |

## What is not removed

- Archives under `~/.myaaw-backups`, unless you confirm or pass `--purge-backup`.
- The macOS code-signing identity created by [`myaaw codesign`](utilities.md#myaaw-codesign); remove it with `myaaw codesign remove` before uninstalling if you want it gone.
- Shell completion snippets you added to your own shell profile.
- Files you copied out of `~/.myaaw` manually.

## Notes

- On Windows the running executable cannot be deleted. The command reports the path and asks you to close every Myaaw process and delete it manually.
- If `~/.myaaw` is a symlink, only the link is removed; the target directory stays.
- Uninstalling requires no running agent and no initialized configuration; it works even when the registry is empty.
- Uninstalling does not touch backups made with `myaaw backup` that you moved elsewhere.

## Reinstalling

Install the binary again with the release artifact or `install.sh`, then run `myaaw onboard`. If you kept an archive, restore it with [`myaaw restore`](backup.md#restore).
