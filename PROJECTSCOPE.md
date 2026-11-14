# Project Scope

## Overview

`crusade` is a Go/Bubble Tea terminal task tracker for Jobs-to-Be-Done outcome
statements. It replaced the former Python
`curses` implementation so CLI parsing, storage, TUI rendering, and release
packaging are separated cleanly.

## Product Contract

- Align records are statements: `When X happens, I want Y outcome so I can Z`.
  Empty x, y, or z is invalid. x is the day this happens, stored as `YYYY-MM-DD`.
- Do records store the outcome and the day the task belongs to.
  They do not store an x/y/z statement.
- Keep do records, align records, boxes, and time blocks in the sql database `crusade`.
  Align mode lists align records and has no month calendar. Do mode opens the list file. Box mode lists the checklist. Cal mode draws the week as an hour sheet sized to the screen. A cell shows part of a block, and the focus box shows the whole block under the cursor. o opens its meeting link in the browser. From the Super+C window, that also closes crusade.
- `crusade -m do` opens the do list in vim. `crusade -m align` opens align mode. `crusade -m box` opens the daily checklist. `q` quits the TUI. Tab cycles align, box, cal, and nlp, and returns each mode to the screen you left. `crusade open` shows a radial menu of do, cal, box, nlp, and align, with the last mode selected. Leaving a mode returns to that menu. Esc on the menu quits. Enter or ctrl+m opens the selected mode. Each mode puts the same blue badge, `ALIGN`, `BOX`, `CAL`, or `NLP`, in the top right. Cal mode blocks time. Each block has a title, a start, an end on one day, and an optional meeting url. Do tasks do not block time. h and l change the day. The week is an hour sheet sized to the screen. A cell shows part of a block, and the focus box shows the whole block under the cursor. o opens its meeting link in the browser. From the Super+C window, that also closes crusade. j and k move one hour at a time. A block starts and ends on the hour. v selects a span. n and e open a form for the title, the start, the end, an optional meeting url, and the repeat. days repeats on chosen weekdays. On a repeating block, that form applies to this event, this and following, or all events. d removes or deletes a repeating block. o is this event, f is this and following, and a is all events. A one-off block uses the same form to edit and dd to delete.
- Align lists are checkbox rows drawn from the top of the terminal downward. Space marks the selected task done and leaves it in the list. `dd` still deletes it. `?` toggles a shortcuts modal over the current screen. In the box list, n adds a box after the selected one, e edits its text, and dd deletes it and any box under it. Those edits stay in the TUI. While a box is being typed, q is a letter. Below the box list, a momentum graph draws checked days as a contribution calendar: weeks are columns, weekdays are rows, bluer squares mean more boxes checked, and the caption is the current streak.
- Do mode opens `$XDG_CONFIG_HOME/crusade/do`, or `~/.config/crusade/do`, in vim.
  The first open writes each current do outcome on its own line.
  Later opens leave the file as you arranged it.
  `-c`, `-r`, `-u`, and `-d` still read and write the dated do rows in the database.
- Align mode is the outcome tracker. From the TUI, open Vim on that one record
  as YAML to add at the selected date or edit the selected task, then save the row.
- Preserve the deterministic command grammar. CRUD names the mode and one operation:
  - `crusade open` shows the radial menu. `crusade -m do` opens the do list. `crusade -m align` opens align mode. `crusade -m box` opens box mode.
  - `crusade -m do|align -c|-r|-u|-d [args]`
  - `crusade -m box -r` shows today. `crusade -m box -c` and `crusade -m box -u` open the outline YAML in Vim and save every box.
  - `crusade -m do -c <outcome> [when]`
  - `crusade -m do -r [limit <count>]`
  - `crusade -m do -u <id> <outcome> [when]`
  - `crusade -m do -d <id>`
  - `crusade -m align -c [x <YYYY-MM-DD> y <outcome> z <impact>]`
  - `crusade -m align -r [limit <count>]`
  - `crusade -m align -u <id> [field <value> ...]`
  - `crusade -m align -d <id>`
  - `crusade open`, `crusade explain fields`, `crusade config`, `crusade help`, `crusade version`, `crusade upgrade`

## Architecture

- `cmd/crusade`: binary entrypoint.
- `internal/cli`: command grammar, editor flow, help, config, upgrade.
- `internal/model`: mode, datetime, and the align YAML record.
- `internal/store`: sql database load/save/add/update/delete, plus a one-time boxes import.
- `internal/tui`: Bubble Tea align list, box checklist, cal time blocks, and the nlp practice list, plus Vim suspension and
  reload.
- `internal/radial`: the Quickshell radial menu.
- `internal/dofile`: the freeform do list.
- `internal/config`: XDG config.
- `internal/version`: release-time version injection.

The TUI and CLI share the sql database `crusade`. Align-mode add and edit suspend
Bubble Tea and open Vim on that one record as YAML. Do mode opens the list file in vim. Box mode adds, edits, and deletes each box in the terminal. The database lives under the sqlx root, which defaults to `~/.sqlx`.

## Config And Storage

Config path:

```text
$XDG_CONFIG_HOME/crusade/config.json
~/.config/crusade/config.json
```

Do list:

```text
$XDG_CONFIG_HOME/crusade/do
~/.config/crusade/do
```

An existing config and do list under `~/.config/xyz` are moved once.

The first open writes each current do outcome on its own line. Later opens leave the file as you arranged it.

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
  done integer not null
  done_at text

model deletion
  id integer primary key
  deleted_at text not null

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

Align rows store x, y, and z. x is a day, `YYYY-MM-DD`. Do rows store the outcome in y and the day in x. Block rows store a title and the start and end of one span on a day. A do row is not a block. A repeat row stores that span plus daily, weekdays, weekly, or chosen weekdays, and an optional last day. A repeat change skips one day or replaces that day's title and times.
`mode` is `align` or `do`. A do row leaves z empty. `done` is 0 until the task is marked done. Marking done keeps the row and stores `done_at` as that moment. Clearing the mark clears `done_at`. Deleting a do task removes the row and stores that moment in `deletion.deleted_at`. An align delete does not.
`-r` prints that mode's records and each stable `id`. Align `-r` lists upcoming align records. Do `-r` lists every do record.
`-u` and `-d` refuse an id from the other mode. A do date of `YYYY-MM-DD` is stored at 23:59:59, and omitting it on create uses today.

An existing xecute database file is renamed to crusade on open. Boxes are imported once from the boxes outline and day files, which stay on disk. A saved `mode: think` row is an align record. A row with no mode and impact exactly `done` is read as a do record. Config is `$XDG_CONFIG_HOME/crusade/config.json`. Crusade removes `data_yaml_path` and `data_csv_path` when an older file still has them.

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
