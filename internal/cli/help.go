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
  crusade -m do
  crusade -m align
  crusade -m box
  crusade open
    resume the last mode, or do when none is saved

  In the TUI, tab cycles do, align, and box.

  explain the x/y/z and p/q/r fields
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

  align rows store x, y, z, and p/q/r. Read lists upcoming rows.
  # crusade -m align -c [x <datetime> y <outcome> z <impact> p <score> q <score> r <score>]
  crusade -m align -c
  crusade -m align -c x "2026-01-26 00:00" y "ship the patch" z "reduce drift" p 7 q 8 r 6
  # crusade -m align -r [limit <count>]
  crusade -m align -r limit 10
  # crusade -m align -u <id> [x <value>] [y <value>] [z <value>] [p <score>] [q <score>] [r <score>]
  crusade -m align -u 3
  crusade -m align -u 3 y "updated outcome" p 8
  # crusade -m align -d <id>
  crusade -m align -d 3

  boxes are the daily checklist in the same database
  # crusade -m box -r
  crusade -m box -r
  # crusade -m box -c
  # crusade -m box -u
  crusade -m box -c
  crusade -m box -u

  when is YYYY-MM-DD or YYYY-MM-DD HH:MM[:SS]. A date alone is 23:59:59.
  Omit when on do create to use today.
`

const Usage = "Run: crusade help"
