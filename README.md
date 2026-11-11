# crusade

`crusade` is a terminal-native Jobs-to-Be-Done tracker for outcome statements:

> When X happens, I want Y outcome so I can Z.

An align row stores that statement. x is the day it happens, as `YYYY-MM-DD`.
Vim edits the row as YAML.

The app is now a single Go binary with a Bubble Tea terminal surface and a
deterministic CLI for scripting outcome capture.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/ryangerardwilson/crusade/main/install.sh | bash
```

The installer places the binary at `~/.crusade/bin/crusade` and writes the public
launcher at `~/.local/bin/crusade`.

Installer commands:

```bash
install.sh version
install.sh version 0.1.29
install.sh upgrade
install.sh from /path/to/crusade
```

Runtime commands:

```bash
crusade help
crusade version
crusade upgrade
crusade config
crusade open
```

## Storage

Config lives at `$XDG_CONFIG_HOME/xyz/config.json`, with fallback
`~/.config/xyz/config.json`. The file is an empty object. Crusade removes
`data_yaml_path` and `data_csv_path` when an older file still has them.

Tasks live in the sqlx database `crusade`:

```text
$SQLX_ROOT/data/crusade.sqlite
~/.sqlx/data/crusade.sqlite
```

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

Align rows require x, y, and z. Do rows store the outcome in y and the day in x. Block rows store a title and the start and end of one span on a day. A repeat row stores that span plus daily, weekdays, weekly, or chosen weekdays, and an optional last day. A repeat change skips one day or replaces that day's title and times.
`-m align` and `-m do` choose which rows a command reads or changes. `-m` with no operation opens that mode. A saved `mode: think` row
is an align row. A row with no mode and impact exactly `done` is read as a do record.
Boxes live in the same database. An existing boxes outline is imported once from `BOXES_CONFIG`, `~/.config/boxes/config.json`, or `~/Documents/notes/rituals.txt`. Day files under `~/.local/share/boxes/days` are imported in that same step. Those files stay on disk.

## CLI

Name the mode and one operation. `-c` creates, `-r` reads, `-u` updates, and `-d` deletes.

```bash
crusade -m do
crusade -m align
crusade -m box
crusade -m box -r
crusade -m box -c
crusade -m box -u

crusade -m do -c "mail the letter"
crusade -m do -c "mail the letter" 2026-06-13
crusade -m do -r limit 10
crusade -m do -u 3 "mail the letter"
crusade -m do -d 3

crusade -m align -r limit 5
crusade -m align -c x 2026-01-26 y "ship the patch" z "reduce drift"
crusade -m align -c
crusade -m align -u 12
crusade -m align -u 12 y "updated outcome"
crusade -m align -d 12
```

Align `-c` and `-u` with no field values open that record as YAML in `$VISUAL`, `$EDITOR`, or `vim`. Box `-c` and `-u` open that same editor on the whole outline as YAML, and saving the file replaces every box. Align `-r` lists upcoming rows. Box `-r` shows today. Do `-r` lists every do row. A do date without a clock time is stored at 23:59:59, and omitting it on create uses today. Do and align `-u` and `-d` use the `id` printed by `-r` and refuse a row from the other mode.

## TUI

```bash
crusade -m do
crusade -m align
crusade -m box
crusade -m cal
crusade -m nlp
```

The agenda, month calendar, and selected-day outcomes start
at the top of the terminal and run downward. Do, align, box, cal, and nlp each put the same blue mode badge in the top right. `?` toggles a bordered
shortcuts modal over that screen. The guide stays sectioned so shortcut columns keep their alignment.

Keys:

```text
q/Ctrl+C/Esc quit (`q` types while a task is being entered)
?          help
tab        cycle do, align, box, cal, and nlp
a          do: toggle agenda/month
t          jump to today
n          do: type a task; align: add a task on the selected date in Vim; cal: open the block form
e / i      do: edit the task text; align: edit the selected task in Vim; cal: edit the selected block
r          do: reschedule the selected task; cal: edit the selected block
hjkl       move row or date
v          cal: select a span on the time grid
H/L        agenda: previous/next day with outcomes
Ctrl+h/l   month: previous/next month
Ctrl+k/j   month: previous/next year
Enter      month: move focus between calendar grid and selected day outcomes
space      mark the selected task done
dd         delete selected outcome; cal: delete a one-off block
```

`crusade -m do` opens the do agenda. `crusade -m align` opens align mode. `crusade -m box` opens the daily checklist. A new do task is the text you type, stored as an outcome on today in the agenda, or on the selected day in the month calendar. `e` edits that text. `r` changes the selected task's date; a date alone is stored at 23:59:59. The do agenda and the do month show how many do tasks were marked done during the current week and how many do tasks were deleted during that same week. The line reads N items done and M deleted this week. That week starts on Monday and ends on Sunday. The done count uses the moment the task is marked done, not the day the task is scheduled for. The deleted count uses the moment the task is deleted. Deleting a task that was marked done this week counts it as deleted and no longer as done. Align mode stores x, y, and z and refuses a row that omits any of them. x is a day, stored as YYYY-MM-DD. In align mode, `n` and `e` open Vim on that one record as YAML. The TUI validates the YAML and saves the row. `q` quits. While a do task is being typed, `q` is a letter. Tab cycles do, align, box, cal, and nlp, and returns each mode to the screen you left. Align mode is a list. Cal mode blocks time with a title, a start, an end on one day, and an optional meeting url. Do tasks do not block time. The week is an hour sheet sized to the screen. A cell shows part of a block, and the focus box shows the whole block under the cursor. o opens its meeting link in the browser. From the Super+C window, that also closes crusade. j and k move one hour at a time. A block starts and ends on the hour. v selects a span. n and e open a form for the title, the start, the end, an optional meeting url, and the repeat. days repeats on chosen weekdays. On a repeating block, that form applies to this event, this and following, or all events. d removes or deletes a repeating block. o is this event, f is this and following, and a is all events. A one-off block uses the same form to edit and dd to delete. While a block is being typed, q is a letter. crusade open resumes the last mode, or do when none is saved. Each mode shows only its own records. An unchanged new-task draft is removed automatically. Box mode uses h/l to change day, space to check, n to add a box after the selected one, e to edit its text, dd to delete it and any box under it, and r to reset the day. While a box is being typed, q is a letter. A parent box is done when every box under it is done. Below the list, a momentum graph draws checked days as a contribution calendar: weeks are columns, weekdays are rows, bluer squares mean more boxes checked, and the caption is the current streak. Today stays open until it is checked, and the streak still counts through yesterday. Box shortcuts stay hidden until `?`. Do and align lists are checkbox rows. Space marks the selected task done and leaves it in the list. `dd` still deletes it.

## Development

```bash
go test ./...
go run ./cmd/crusade help
go run ./cmd/crusade -m align -r limit 5
./install.sh from .
./push_release_upgrade.sh
```
