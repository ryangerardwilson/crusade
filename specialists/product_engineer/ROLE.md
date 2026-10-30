# Product Engineer Role

## Purpose

Own crusade-specific facts that should not live in root generalists.

## Load Guidance

Load this file for crusade implementation, CLI/TUI, installer, release, storage,
configuration, or project-specific product work.

## Owns

- CLI/TUI contract, command grammar, config, storage, and installer constraints.
- Release, upgrade, and verification expectations specific to this app.
- Repo-local product facts around the JTBD `x/y/z` plus `p/q/r` scoring model.

## Project Context

`crusade` is a Go/Bubble Tea terminal tracker. It used to be a flat Python
`curses` project, but that implementation is retired. Do not add new Python
curses modules or PyInstaller release paths.

Current structure:

- `cmd/crusade/main.go`: binary entrypoint.
- `internal/cli/`: help/version/upgrade/config and deterministic command grammar.
- `internal/config/`: XDG config.
- `internal/model/`: datetime, metric, and JSON validation.
- `internal/box/`: checklist outline, parent completion, and check toggles.
- `internal/store/`: the sqlx database `crusade`, plus a one-time boxes import.
- `internal/tui/`: Bubble Tea do, align, and box surfaces.
- `internal/version/`: release-time version variable.
- `install.sh`: release/local-source installer.
- `push_release_upgrade.sh`: push, tag, publish release asset, install from GitHub.

## Commands

```bash
go test ./...
go run ./cmd/crusade help
go run ./cmd/crusade open
go run ./cmd/crusade -m align -c x "2026-01-26 00:00" y "ship the patch" z "reduce drift" p 7 q 8 r 6
go run ./cmd/crusade -m align -r limit 5
go run ./cmd/crusade -m do -c "mail the letter"
./install.sh from .
./push_release_upgrade.sh
```

## Storage Contract

Config lives at `$XDG_CONFIG_HOME/xyz/config.json`, falling back to
`~/.config/xyz/config.json`. Crusade removes `data_yaml_path` and `data_csv_path` when an older file still has them.

Tasks and boxes live in `$SQLX_ROOT/data/crusade.sqlite`, or `~/.sqlx/data/crusade.sqlite` when `SQLX_ROOT` is unset.

The `task` model stores stable IDs:

```text
model task
  id integer primary key
  mode text not null
  x text not null
  y text not null
  z text
  p real
  q real
  r real
  done integer not null
```

`-r` prints each stable `id`. `-u <id>` and `-d <id>` target that id and stay on the selected mode. In align mode, TUI `n` and `e` suspend Bubble Tea, open Vim on that one record, and save the row after validation. In do mode, `e` edits the task text and `r` reschedules the selected task. The do agenda and the do month show how many do tasks are marked done in the current week. That week starts on Monday and ends on Sunday. `crusade -m box -c` and `crusade -m box -u` open Vim on the whole outline as YAML and save every box. In box mode, TUI `e` does the same. Do and align lists are checkbox rows drawn from the top. Space marks the selected task done and leaves it in the list. `dd` still deletes it. Box shortcuts appear only while `?` is open. Do, align, and box use the same blue badge in the top right. `q` quits the TUI. While a do task is being typed, `q` is a letter. Tab cycles do, align, and box, and returns each mode to the screen you left. crusade open resumes the last mode, or do when none is saved. Below the box list, a momentum graph draws checked days as a contribution calendar: weeks are columns, weekdays are rows, bluer squares mean more boxes checked, and the caption is the current streak.

## Release Constraints

- Keep `internal/version.Version` at `0.0.0` in source.
- Inject the release version with Go ldflags.
- Release asset name: `xyz-linux-x64.tar.gz`.
- Installer must accept both old `# Managed by xyz local-bin launcher` and new
  `# Managed by xyz installer local-bin launcher` markers.
