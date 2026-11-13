package cli

const HelpText = `crusade
terminal tracker for do tasks, align outcomes, and daily boxes

global actions:
  crusade help
    show this help
  crusade version
    print the installed version
  crusade upgrade
    upgrade to the latest release
  crusade config
    open config in $VISUAL/$EDITOR

open a mode:
  crusade open
    show the radial menu. The last mode starts selected.
  crusade -m do
    open the do list in $VISUAL, $EDITOR, or vim
  crusade -m align
  crusade -m box
  crusade -m cal
  crusade -m nlp

  The do list is $XDG_CONFIG_HOME/xyz/do, or ~/.config/xyz/do.
  The first open lists each do outcome on its own line.
  Later opens keep the file as you left it.
  In the TUI, tab cycles align, box, cal, and nlp. ? toggles a shortcuts modal.
  Align is a list. Cal blocks time on an hour grid and has no -c, -r, -u, or -d. The week is an hour sheet sized to the screen. The focus box shows the block under the cursor. o opens its meeting link in the browser. From the Super+C window, that also closes crusade. j and k move one hour at a time. A block starts and ends on the hour. v selects a span. n and e open a form for the title, the start, the end, an optional meeting url, and the repeat. days repeats on chosen weekdays. On a repeat, the form applies to this event, this and following, or all events. d uses o, f, and a the same way.

  explain the x/y/z fields
  # crusade explain fields
  crusade explain fields

records, one mode and one operation:
  -m do|align|box
  -c create   -r read   -u update   -d delete

  do tasks store an outcome and a day
  # crusade -m do -c <outcome> [when]
  crusade -m do -c "mail the letter"
  crusade -m do -c "mail the letter" 2026-06-13
  # crusade -m do -r [limit <count>]
  crusade -m do -r limit 10
  # crusade -m do -u <id> <outcome> [when]
  crusade -m do -u 3 "mail the letter"
  # crusade -m do -d <id>
  crusade -m do -d 3

  align rows store x, y, and z. x is a day, YYYY-MM-DD. Bare -c and -u open that record as YAML. Read lists upcoming rows.
  # crusade -m align -c [x <YYYY-MM-DD> y <outcome> z <impact>]
  crusade -m align -c
  crusade -m align -c x 2026-01-26 y "ship the patch" z "reduce drift"
  # crusade -m align -r [limit <count>]
  crusade -m align -r limit 10
  # crusade -m align -u <id> [x <YYYY-MM-DD>] [y <value>] [z <value>]
  crusade -m align -u 3
  crusade -m align -u 3 y "updated outcome"
  # crusade -m align -d <id>
  crusade -m align -d 3

  boxes are the daily checklist in the same database
  # crusade -m box -r
  crusade -m box -r
  # crusade -m box -c
  # crusade -m box -u
  crusade -m box -c
  crusade -m box -u

  nlp is the practice list. Sessions stay in the sqlx database nlp.
  # crusade -m nlp
  crusade -m nlp

  when is YYYY-MM-DD or YYYY-MM-DD HH:MM[:SS]. A date alone is 23:59:59.
  Omit when on do create to use today.
`

const Usage = "Run: crusade help"
