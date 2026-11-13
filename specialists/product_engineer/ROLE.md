# Product Engineer Role

## Purpose

Own crusade-specific facts that should not live in root generalists.

## Load Guidance

Load this file for `crusade` implementation, CLI/TUI, installer, release, storage,
configuration, or project-specific product work.

## Owns

- CLI/TUI contract, command grammar, config, storage, and installer constraints.
- Release, upgrade, and verification expectations specific to this app.
- Repo-local product facts around the JTBD `x/y/z` statement. Align x is a day.

## Project Context

`crusade` is a Go/Bubble Tea terminal tracker. It used to be a flat Python
`curses` project, but that implementation is retired. Do not add new Python
curses modules or PyInstaller release paths.

Current structure:

- `cmd/crusade/main.go`: binary entrypoint.
- `internal/cli/`: help/version/upgrade/config and deterministic command grammar.
- `internal/config/`: XDG config.
- `internal/model/`: datetime validation and the align YAML record.
- `internal/box/`: checklist outline, parent completion, and check toggles.
- `internal/store/`: the sqlx database `crusade`, plus a one-time boxes import.
- `internal/tui/`: Bubble Tea align, box, cal, and nlp surfaces.
- `internal/radial/`: the Quickshell radial menu.
- `internal/dofile/`: the freeform do list.
- `nlp/`: the practice-session mode. Sessions stay in the sqlx database `nlp`.
- `internal/version/`: release-time version variable.
- `install.sh`: release/local-source installer.
- `push_release_upgrade.sh`: push, tag, publish release asset, install from GitHub.

## Commands

```bash
go test ./...
go run ./cmd/crusade help
go run ./cmd/crusade open
go run ./cmd/crusade -m align -c x 2026-01-26 y "ship the patch" z "reduce drift"
go run ./cmd/crusade -m align -r limit 5
go run ./cmd/crusade -m do -c "mail the letter"
./install.sh from .
./push_release_upgrade.sh
```

## Storage Contract

Config lives at `$XDG_CONFIG_HOME/crusade/config.json`, falling back to
`~/.config/crusade/config.json`. The do list is `$XDG_CONFIG_HOME/crusade/do`.
Crusade removes `data_yaml_path` and `data_csv_path` when an older file still has them.
An existing config and do list under `~/.config/xyz` are moved once.

Tasks and boxes live in `$SQLX_ROOT/data/crusade.sqlite`, or `~/.sqlx/data/crusade.sqlite` when `SQLX_ROOT` is unset.

The `task` model stores stable IDs:

```text
model task
  id integer primary key
  mode text not null
  x text not null
  y text not null
  z text
  done integer not null
  done_at text

model deletion
  id integer primary key
  deleted_at text not null
```

Time blocks live in the same database:

```text
model block
  id integer primary key
  title text not null
  start_at text not null
  end_at text not null

model repeat
  id integer primary key
  title text not null
  start_at text not null
  end_at text not null
  freq text not null
  until_on text

model repeat_change
  id integer primary key
  repeat_id integer not null
  day text not null
  skipped integer not null
  title text
  start_at text
  end_at text
```

`-r` prints each stable `id`. `-u <id>` and `-d <id>` target that id and stay on the selected mode. In align mode, TUI `n` and `e` suspend Bubble Tea, open Vim on that one record as YAML, and save the row after validation. x is a day, stored as YYYY-MM-DD. Do mode opens the list file in vim. The first open writes each current do outcome on its own line. Later opens leave the file as the user arranged it. `crusade -m box -c` and `crusade -m box -u` open Vim on the whole outline as YAML and save every box. In box mode, n adds a box after the selected one, e edits its text, and dd deletes it and any box under it. While a box is being typed, q is a letter. Align lists are checkbox rows drawn from the top. Space marks the selected task done and leaves it in the list. `dd` still deletes it. `?` toggles a shortcuts modal over the current screen. Do, align, box, cal, and nlp use the same blue badge in the top right. `q` quits the TUI. Tab cycles align, box, cal, and nlp, and returns each mode to the screen you left. Align mode is a list. Cal mode blocks time with a title, a start, an end on one day, and an optional meeting url. Do tasks do not block time. The week is an hour sheet sized to the screen. A cell shows part of a block, and the focus box shows the whole block under the cursor. o opens its meeting link in the browser. From the Super+C window, that also closes crusade. j and k move one hour at a time. A block starts and ends on the hour. v selects a span. n and e open a form for the title, the start, the end, an optional meeting url, and the repeat. days repeats on chosen weekdays. On a repeating block, that form applies to this event, this and following, or all events. d removes or deletes a repeating block. o is this event, f is this and following, and a is all events. A one-off block uses the same form to edit and dd to delete. While a block is being typed, q is a letter. `crusade open` shows a radial menu of do, cal, box, nlp, and align, with the last mode selected. Leaving a mode returns to that menu. Esc on the menu quits. Below the box list, a momentum graph draws checked days as a contribution calendar: weeks are columns, weekdays are rows, bluer squares mean more boxes checked, and the caption is the current streak.

## Release Constraints

- Keep `internal/version.Version` at `0.0.0` in source.
- Inject the release version with Go ldflags.
- Release asset name: `crusade-linux-x64.tar.gz`.
- Installer must accept both old `# Managed by crusade local-bin launcher` and new
  `# Managed by crusade installer local-bin launcher` markers.
