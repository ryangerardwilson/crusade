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
- `crusade -m do` opens the do list in vim. `crusade -m align` opens align mode. `crusade -m box` opens the daily checklist. `q` quits the TUI. Tab cycles align, box, cal, and nlp, and returns each mode to the screen you left. `crusade open` shows a radial menu of do, cal, box, nlp, and align, with the last mode selected. Leaving a mode returns to that menu. Esc on the menu quits. Enter or ctrl+m opens the selected mode. Each mode puts the same gray badge, `ALIGN`, `BOX`, `CAL`, or `NLP`, in the top right. Cal mode blocks time. Each block has a title, a start, an end on one day, and an optional meeting url. Do tasks do not block time. h and l change the day. The week is an hour sheet sized to the screen. A cell shows part of a block, and the focus box shows the whole block under the cursor. o opens its meeting link in the browser. From the Super+C window, that also closes crusade. j and k move one hour at a time. A block starts and ends on the hour. A meeting already off the hour keeps that time when its clock is left unchanged. v selects a span. n and e open a form for the title, the start, the end, an optional meeting url, and the repeat. days repeats on chosen weekdays. On a repeating block, that form applies to this event, this and following, or all events. A meeting url applies to every occurrence. d removes or deletes a repeating block. o is this event, f is this and following, and a is all events. A one-off block uses the same form to edit and dd to delete.
- Align lists are checkbox rows drawn from the top of the terminal downward. Space marks the selected task done and leaves it in the list. `dd` still deletes it. `?` toggles a shortcuts modal over the current screen. In the box list, n adds a box after the selected one, e edits its text, and dd deletes it and any box under it. Each edit saves that one box in the database; a rename keeps the box id and its checks. While a box is being typed, q is a letter. Below the box list, a momentum graph draws checked days as a contribution calendar: weeks are columns, weekdays are rows, lighter squares mean more boxes checked, and the caption is the current streak.
- The do list is the `do_item` rows in the database: headings, blank lines, and items, in order.
  Do mode writes them to `$XDG_CONFIG_HOME/crusade/do`, or `~/.config/crusade/do`, opens that file in vim,
  and replaces the rows with what vim saved in one transaction. The file is derived and rewritten on each open.
  `-c`, `-r`, `-u`, and `-d` read and write the same rows.
- Align mode is the outcome tracker. From the TUI, open Vim on that one record
  as YAML to add at the selected date or edit the selected task, then save the row.
- Preserve the deterministic command grammar. CRUD names the mode and one operation:
  - `crusade open` shows the radial menu. `crusade -m do` opens the do list. `crusade -m align` opens align mode. `crusade -m box` opens box mode.
  - `crusade -m do|align -c|-r|-u|-d [args]`
  - `crusade -m box -r` shows today. `crusade -m box -c` and `crusade -m box -u` open the outline YAML in Vim and save every box.
  - `crusade -m do -c <outcome>`
  - `crusade -m do -r [limit <count>]`
  - `crusade -m do -u <id> <text>`
  - `crusade -m do -d <id>`
  - `crusade -m align -c [x <YYYY-MM-DD> y <outcome> z <impact>]`
  - `crusade -m align -r [limit <count>]`
  - `crusade -m align -u <id> [field <value> ...]`
  - `crusade -m align -d <id>`
  - `crusade open`, `crusade explain fields`, `crusade config`, `crusade help`, `crusade version`, `crusade upgrade`

## Architecture

- `cmd/crusade`: binary entrypoint.
- `internal/cli`: command grammar, editor flow, help, config, upgrade.
- `internal/model`: modes, datetimes, the align record and its YAML, the do list rows, and the calendar series rules.
- `internal/box`: the box outline and its check rules.
- `internal/boxyaml`: the outline YAML that `crusade -m box -c` edits.
- `internal/sqlite`: runs SQL through sqlite3 and migrates a file to its schema version. The crusade store and the nlp state both use it.
- `internal/store`: the crusade database: schema, migration, and one write path per fact.
- `internal/tui`: Bubble Tea align list, box checklist, cal time blocks, and the nlp practice list, plus Vim suspension and
  reload.
- `internal/radial`: the Quickshell radial menu.
- `internal/dofile`: the vim buffer of the do list.
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

That file is the vim buffer of the `do_item` rows. A do file from before the rows existed is imported once on open.

Database:

```text
$SQLX_ROOT/data/crusade.sqlite
~/.sqlx/data/crusade.sqlite
```

The schema is `internal/store/schema.go` at `PRAGMA user_version` 2. Every connection turns on foreign keys.

```text
install(id = 1, last_mode, boxes_imported, do_imported)
do_item(id, position, kind heading|blank|item, body)
deletion(id, deleted_at UTC)
align(id, x YYYY-MM-DD, y, z, done_at UTC or null when not done)
box(id, parent_id -> box or null for a root, position, label)
box_check(id, day, box_id -> box, unique day and box)
block(id, title, start_at, end_at, url or null)
repeat(id, title, start_at, end_at, freq, until_on or null, url or null)
repeat_skip(repeat_id -> repeat, day)
repeat_override(repeat_id -> repeat, day, title or null, start_at and end_at or null, url or null)
```

Align rows store x, y, and z. x is a day, `YYYY-MM-DD`. Done is derived from `done_at`. Block rows store a title and the start and end of one span on a day, in local wall time. A repeat row stores that span plus daily, weekdays, weekly, or chosen weekdays, and an optional last day. A skip removes one day. An override stores only what differs from the series on one day: null is the series value. A series url change clears day urls, so the link is the same on every day. Deleting a do item stores that moment in `deletion.deleted_at`.
`-r` prints that mode's rows and each stable `id`. Align `-r` lists upcoming align records. Do `-r` lists every do row.

A file of the older schema (task, path-keyed boxes, meta, repeat_change) is migrated once in one transaction; a failed step leaves it as it was. An existing xecute database file is renamed to crusade on open. Boxes are imported once from the boxes outline and day files, which stay on disk. In that migration a `mode: think` row is an align record, and a row with no mode and impact exactly `done` is a do item. Config is `$XDG_CONFIG_HOME/crusade/config.json`. Crusade removes `data_yaml_path` and `data_csv_path` when an older file still has them.

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
