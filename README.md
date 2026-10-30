# crusade

crusade is a terminal tracker with three modes:

- **do** is the agenda. A task is a line of text and the day it belongs to.
- **align** is the outcome tracker. A row is a Jobs-to-Be-Done statement plus three scores.
- **box** is the daily checklist, with a contribution calendar of days you checked.

One Go binary serves the Bubble Tea TUI and the scriptable CLI. Both read the same database.

## Install

The packaged release is Linux x64. It does not need Go.

```sh
curl -fsSL https://raw.githubusercontent.com/ryangerardwilson/crusade/main/install.sh | bash
```

The installer puts the binary at `~/.crusade/bin/crusade` and the launcher at `~/.local/bin/crusade`. It also writes three compatibility launchers:

| Launcher | What it runs |
| --- | --- |
| `xyz` | `crusade` |
| `xecute` | `crusade` |
| `boxes` | `crusade -m box` |

`~/.local/bin` needs to be on `PATH`.

Build from a checkout when you want the current source. That needs Go 1.26 and `sqlite3`.

```sh
git clone https://github.com/ryangerardwilson/crusade.git
cd crusade
./install.sh from .
```

Other installer commands:

```sh
./install.sh help
./install.sh version          # print the latest release
./install.sh version 0.1.0    # install that release
./install.sh upgrade
```

## Open a mode

```sh
crusade -m do       # agenda
crusade -m align    # outcomes
crusade -m box      # daily checklist
crusade open        # last mode, or do when none is saved
crusade help
```

In the TUI, Tab cycles do, then align, then box, and returns each mode to the screen you left. `q` quits. While a do task is being typed, `q` is a letter.

Each mode shows only its own records. The mode badge in the top right is `DO`, `ALIGN`, or `BOX`.

## Do

A do task is the text you type. In the agenda it is dated today. In the month calendar it is dated the selected day. A date with no clock time is stored at 23:59:59.

```sh
crusade -m do -c "mail the letter"
crusade -m do -c "mail the letter" 2026-06-13
crusade -m do -c "mail the letter" "2026-06-13 15:00"
crusade -m do -r
crusade -m do -r limit 10
crusade -m do -u 3 "mail the letter"
crusade -m do -d 3
```

`-r` prints an `id`. `-u` and `-d` use that id and refuse a row from another mode.

TUI keys:

| Key | Action |
| --- | --- |
| `n` | Type a new task |
| `e` | Edit the selected task |
| `r` | Reschedule the selected task |
| `space` | Mark the selected task done and leave it in the list |
| `dd` | Delete the selected task |
| `a` | Switch agenda and month |
| `t` | Jump to today |
| `j` / `k` | Move the row |
| `H` / `L` | Previous or next day that has a task |
| `?` | Help |

In the month view, `h` and `l` move a day, `j` and `k` move a week, Ctrl+h and Ctrl+l move a month, and Ctrl+k and Ctrl+j move a year. Enter moves between the calendar and that day's tasks. Escape leaves the day list. A second Escape quits.

The agenda and the month show how many do tasks are marked done in the current week, Monday through Sunday.

While typing, the line accepts the usual readline-style keys: Ctrl+A, Ctrl+E, Ctrl+B, Ctrl+F, Ctrl+H, Ctrl+D, Ctrl+K, Ctrl+U, Ctrl+W, Alt+B, Alt+F, and the arrows.

## Align

An align row is the statement "when X happens, I want Y so I can Z", plus three scores:

| Field | Meaning |
| --- | --- |
| `x` | When it happens |
| `y` | The outcome |
| `z` | Why it matters |
| `p` | Alignment |
| `q` | Outward impact |
| `r` | Embodied practice |

X, y, and z are required. Scores are numbers.

```sh
crusade -m align -c x "2026-01-26 00:00" y "ship the patch" z "reduce drift" p 7 q 8 r 6
crusade -m align -r limit 5
crusade -m align -u 12 y "updated outcome" p 8.5
crusade -m align -d 12
```

`-r` lists upcoming rows. Bare `-c`, or `-u` with only an id, opens `$VISUAL`, then `$EDITOR`, then `vim` on that one record. Saving the file writes the row.

In the TUI, `n` and `e` open that same editor. `a`, `t`, `space`, and `dd` behave as they do in do mode. Align ignores `r`.

## Box

Box mode is the daily checklist in the same database. A parent box is done when every box under it is done.

```sh
crusade -m box -r    # show today
crusade -m box -c    # edit the outline
crusade -m box -u    # edit the outline
```

`-c` and `-u` open the whole outline as YAML in your editor. Saving replaces every box. The file looks like this:

```yaml
version: 1
boxes:
  - label: Move
  - label: Work
    children:
      - label: Email
      - label: Deep work
```

Labels become ids such as `work/email`. Checking a box is a TUI action, not a CLI flag.

TUI keys:

| Key | Action |
| --- | --- |
| `h` / `l` | Previous or next day |
| Ctrl+h / Ctrl+l | Previous or next week |
| `j` / `k` | Move in the list |
| `space` | Check or uncheck |
| `e` | Edit the outline in your editor |
| `r` | Reset the viewed day |
| `?` | Show the shortcut line |
| `q` | Quit |

Under the list, a momentum graph draws checked days as a contribution calendar. Weeks are columns, weekdays are rows, and bluer squares mean more boxes checked that day. The caption is the current streak.

## Storage

Config is an empty JSON object:

```text
$XDG_CONFIG_HOME/xyz/config.json
~/.config/xyz/config.json
```

The directory name stays `xyz` so existing installs keep their config. `crusade config` opens that file. Older files may still contain `data_yaml_path` or `data_csv_path`. Crusade removes those keys. It does not read them.

Tasks, boxes, and day checks live in one sqlite database:

```text
$SQLX_ROOT/data/crusade.sqlite
~/.sqlx/data/crusade.sqlite
```

Crusade talks to `sqlite3` directly (`/usr/bin/sqlite3`, or `$SQLX_SQLITE`). The `sqlx` command is not required to run it.

The tables are `task`, `box`, and `box_check`. A do row stores the outcome in `y` and the day in `x`. An align row stores x, y, z, and p, q, r. `done` is 0 until Space marks the row done.

The first time box mode opens an empty outline, crusade imports one from `BOXES_CONFIG`, `~/.config/boxes/config.json`, or `~/Documents/notes/rituals.txt`, and imports day files under `~/.local/share/boxes/days`. Those files stay on disk and are not read again.

## Develop

From this repository, with the home `go.work` out of the way:

```sh
GOWORK=off go test ./...
./install.sh from .
```

`./push_release_upgrade.sh` tests, pushes `main`, publishes the next patch tag, and installs that release. Pass an explicit version when you do not want the next patch:

```sh
./push_release_upgrade.sh 0.2.0
```
