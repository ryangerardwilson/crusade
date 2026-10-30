# Project Scope

## Overview

`crusade` is a Go/Bubble Tea terminal task tracker for Jobs-to-Be-Done outcome
statements and `p/q/r` North Star scoring. It replaced the former Python
`curses` implementation so CLI parsing, storage, TUI rendering, and release
packaging are separated cleanly.

## Product Contract

- Align records are statements: `When X happens, I want Y outcome so I can Z`.
  Empty x, y, or z is invalid. Each align record also stores numeric `p`, `q`, and `r`.
- Do records store the outcome and the day the task belongs to.
  They do not store an x/y/z statement or scores.
- Keep do records, align records, and boxes in the sqlx database `crusade`.
  Align mode lists align records. Do mode lists do records. Box mode lists the checklist.
- `crusade -m do` opens the do agenda. `crusade -m align` opens align mode. `crusade -m box` opens the daily checklist. `q` quits the TUI. While a do task is being typed, `q` is a letter. Tab cycles do, align, and box, and returns each mode to the screen you left. crusade open resumes the last mode, or do when none is saved. Each mode puts the same blue badge, `DO`, `ALIGN`, or `BOX`, in the top right.
- Do and align lists are checkbox rows drawn from the top of the terminal downward. Space marks the selected task done and leaves it in the list. `dd` still deletes it. Box shortcuts appear only while `?` is open. Below the box list, a momentum graph draws checked days as a contribution calendar: weeks are columns, weekdays are rows, bluer squares mean more boxes checked, and the caption is the current streak.
- Do mode asks for the task text only. A new task is dated today in the agenda,
  or on the selected day of the month calendar. `e` edits that text. `r` changes
  the selected task's date. A date alone is stored at 23:59:59.
  The do agenda and the do month show how many do tasks are marked done in the
  current week. That week starts on Monday and ends on Sunday.
- Align mode is the outcome tracker. From the TUI, open Vim on that one record
  to add at the selected date or edit the selected task, then save the row.
- Preserve the deterministic command grammar. CRUD names the mode and one operation:
  - `crusade -m do` opens do mode. `crusade -m align` opens align mode. `crusade -m box` opens box mode.
  - `crusade -m do|align -c|-r|-u|-d [args]`
  - `crusade -m box -r` shows today. `crusade -m box -c` and `crusade -m box -u` open the outline YAML in Vim and save every box.
  - `crusade -m do -c <outcome> [when]`
  - `crusade -m do -r [limit <count>]`
  - `crusade -m do -u <id> <outcome> [when]`
  - `crusade -m do -d <id>`
  - `crusade -m align -c [x <datetime> y <outcome> z <impact> p <score> q <score> r <score>]`
  - `crusade -m align -r [limit <count>]`
  - `crusade -m align -u <id> [field <value> ...]`
  - `crusade -m align -d <id>`
  - `crusade open`, `crusade explain fields`, `crusade config`, `crusade help`, `crusade version`, `crusade upgrade`

## Architecture

- `cmd/crusade`: binary entrypoint.
- `internal/cli`: command grammar, editor flow, help, config, upgrade.
- `internal/model`: mode, datetime, metric, and statement validation.
- `internal/store`: sqlx database load/save/add/update/delete, plus a one-time boxes import.
- `internal/tui`: Bubble Tea agenda and month surfaces plus Vim suspension and
  reload.
- `internal/config`: XDG config.
- `internal/version`: release-time version injection.

The TUI and CLI share the sqlx database `crusade`. Align-mode add and edit suspend
Bubble Tea and open Vim on that one record. Do mode edits the outcome in
the terminal. The database lives under the sqlx root, which defaults to `~/.sqlx`.

## Config And Storage

Config path:

```text
$XDG_CONFIG_HOME/xyz/config.json
~/.config/xyz/config.json
```

Database:

```text
$SQLX_ROOT/data/crusade.sqlite
~/.sqlx/data/crusade.sqlite
```

The sqlx model is `task`:

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

model box
  id integer primary key
  path text not null
  label text not null
  parent_path text not null
  position integer not null
  depth integer not null

model box_check
  id integer primary key
  day text not null
  path text not null
```

Align rows store x, y, z, and p/q/r. Do rows store the outcome in y and the day in x.
`mode` is `align` or `do`. A do row leaves z and the scores empty. `done` is 0 until the task is marked done. Marking done keeps the row.
`-r` prints that mode's records and each stable `id`. Align `-r` lists upcoming align records. Do `-r` lists every do record.
`-u` and `-d` refuse an id from the other mode. A do date of `YYYY-MM-DD` is stored at 23:59:59, and omitting it on create uses today.

An existing xecute database file is renamed to crusade on open. Boxes are imported once from the boxes outline and day files, which stay on disk. A saved `mode: think` row is an align record. A row with no mode, impact exactly `done`, and scores of zero is read as a do record. Config is `$XDG_CONFIG_HOME/xyz/config.json`. Crusade removes `data_yaml_path` and `data_csv_path` when an older file still has them.

## Non-Goals

- Background reminders, daemons, or notifications.
- Remote sync or OAuth providers.
- Multi-user sharing.
- GUI frontends.
- Reintroducing Python curses.

## Verification

Required before release:

```bash
go test ./...
go run ./cmd/crusade help
go run ./cmd/crusade version
./install.sh from .
~/.crusade/bin/crusade version
```

Release and local upgrade:

```bash
./push_release_upgrade.sh
```
