package store

import (
	"fmt"
	"strings"

	"github.com/ryangerardwilson/crusade/internal/model"
	"github.com/ryangerardwilson/crusade/internal/sqlite"
)

// schemaVersion is the user_version of a crusade file with these tables.
const schemaVersion = 2

// crusadeTables is the schema. Every nullable column states what null means.
const crusadeTables = `
-- This machine's crusade: the mode the next launch opens and the one-time
-- imports that already ran. There is one row.
CREATE TABLE install (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  last_mode TEXT NOT NULL DEFAULT 'do' CHECK (last_mode IN ('do', 'align', 'box', 'cal', 'nlp')),
  boxes_imported INTEGER NOT NULL DEFAULT 0 CHECK (boxes_imported IN (0, 1)),
  do_imported INTEGER NOT NULL DEFAULT 0 CHECK (do_imported IN (0, 1))
);
INSERT INTO install(id) VALUES (1);

-- One line of the do list, in list order: a heading, a blank line, or an item.
CREATE TABLE do_item (
  id INTEGER PRIMARY KEY,
  position INTEGER NOT NULL CHECK (position >= 0),
  kind TEXT NOT NULL CHECK (kind IN ('heading', 'blank', 'item')),
  body TEXT NOT NULL DEFAULT '' CHECK (instr(body, char(10)) = 0 AND instr(body, char(13)) = 0),
  CHECK ((kind = 'blank' AND body = '') OR (kind <> 'blank' AND trim(body) <> '')),
  CHECK (kind <> 'heading' OR substr(body, 1, 1) = '#')
);
CREATE INDEX do_item_position ON do_item(position);

-- One do item removed from the do list. deleted_at is UTC.
CREATE TABLE deletion (
  id INTEGER PRIMARY KEY,
  deleted_at TEXT NOT NULL CHECK (datetime(deleted_at) IS deleted_at)
);

-- One align record: on day x the outcome y matters because of z.
CREATE TABLE align (
  id INTEGER PRIMARY KEY,
  x TEXT NOT NULL CHECK (date(x) IS x),
  y TEXT NOT NULL CHECK (trim(y) <> ''),
  z TEXT NOT NULL CHECK (trim(z) <> ''),
  -- null: not done. UTC.
  done_at TEXT CHECK (done_at IS NULL OR datetime(done_at) IS done_at)
);
CREATE INDEX align_x ON align(x);

-- One box of the outline. A box with children is checked through its leaves.
CREATE TABLE box (
  id INTEGER PRIMARY KEY,
  -- null: a root box
  parent_id INTEGER REFERENCES box(id) ON DELETE CASCADE,
  position INTEGER NOT NULL CHECK (position >= 0),
  label TEXT NOT NULL CHECK (trim(label) <> '')
);
CREATE INDEX box_parent ON box(parent_id, position);

-- One box checked on one day.
CREATE TABLE box_check (
  id INTEGER PRIMARY KEY,
  day TEXT NOT NULL CHECK (date(day) IS day),
  box_id INTEGER NOT NULL REFERENCES box(id) ON DELETE CASCADE,
  UNIQUE (day, box_id)
);
CREATE INDEX box_check_box ON box_check(box_id);

-- One span of clock time on one day. Clock times are local wall time, as a
-- meeting is set.
CREATE TABLE block (
  id INTEGER PRIMARY KEY,
  title TEXT NOT NULL CHECK (trim(title) <> ''),
  start_at TEXT NOT NULL CHECK (datetime(start_at) IS start_at),
  end_at TEXT NOT NULL CHECK (datetime(end_at) IS end_at AND end_at > start_at),
  -- null: no meeting link
  url TEXT CHECK (url IS NULL OR trim(url) <> '')
);
CREATE INDEX block_start ON block(start_at);

-- One series of blocks at the same clock time.
CREATE TABLE repeat (
  id INTEGER PRIMARY KEY,
  title TEXT NOT NULL CHECK (trim(title) <> ''),
  start_at TEXT NOT NULL CHECK (datetime(start_at) IS start_at),
  end_at TEXT NOT NULL CHECK (datetime(end_at) IS end_at AND end_at > start_at),
  freq TEXT NOT NULL CHECK (freq IN ('daily', 'weekdays', 'weekly') OR (freq <> '' AND trim(freq, 'montuewdhfrisa,') = '')),
  -- null: the series has no last day
  until_on TEXT CHECK (until_on IS NULL OR (date(until_on) IS until_on AND until_on >= date(start_at))),
  -- null: no meeting link
  url TEXT CHECK (url IS NULL OR trim(url) <> '')
);

-- One day a series does not happen.
CREATE TABLE repeat_skip (
  repeat_id INTEGER NOT NULL REFERENCES repeat(id) ON DELETE CASCADE,
  day TEXT NOT NULL CHECK (date(day) IS day),
  PRIMARY KEY (repeat_id, day)
);

-- One day of a series that differs from the series.
CREATE TABLE repeat_override (
  repeat_id INTEGER NOT NULL REFERENCES repeat(id) ON DELETE CASCADE,
  day TEXT NOT NULL CHECK (date(day) IS day),
  -- null: the day has the series title
  title TEXT CHECK (title IS NULL OR trim(title) <> ''),
  -- null with end_at: the day keeps the series clock
  start_at TEXT CHECK (start_at IS NULL OR (datetime(start_at) IS start_at AND date(start_at) = day)),
  -- null with start_at: the day keeps the series clock
  end_at TEXT CHECK (end_at IS NULL OR (datetime(end_at) IS end_at AND end_at > start_at)),
  -- null: the day has the series url
  url TEXT CHECK (url IS NULL OR trim(url) <> ''),
  PRIMARY KEY (repeat_id, day),
  CHECK ((start_at IS NULL) = (end_at IS NULL)),
  CHECK (title IS NOT NULL OR start_at IS NOT NULL OR url IS NOT NULL)
);
`

func crusadeSchema() sqlite.Schema {
	return sqlite.Schema{Name: DatabaseName, Version: schemaVersion, Tables: crusadeTables, Migrate: migrateLegacy}
}

// legacyTables are the tables of a crusade file before schema version 2.
var legacyTables = []string{"task", "box", "box_check", "meta", "block", "repeat", "repeat_change", "deletion"}

const doTaskWhere = "(lower(trim(COALESCE(mode, ''))) = 'do' OR (trim(COALESCE(mode, '')) = '' AND z = 'done'))"

// migrateLegacy moves a file of the old schema (task, path-keyed box and
// box_check, meta, block, repeat, repeat_change, deletion) to version 2. Every
// statement runs in the one transaction sqlite.Prepare opens, so a failed
// statement or failed guard rolls everything back and the old tables stay.
// Old tables are renamed, copied, checked, and only then dropped.
//
// Backfill:
//   - task rows of mode align (or the older think and empty modes) become
//     align rows with x cut to its day. done_at stays when done is set, moved
//     from local time to UTC; a done row with no done_at gets midnight of its
//     day. done = 0 is null. deletion.deleted_at moves to UTC the same way.
//   - task rows of mode do become do items in x order. The do file, when the
//     program finds one, replaces them on first open (importDoOnce).
//   - box keeps each id; parent_path becomes the parent's id, and a box whose
//     parent path has no row becomes a root. box_check rows point at the box
//     of their path; a check of a path with no box is dropped.
//   - meta last_mode and boxes_imported become install columns.
//   - repeat_change: a skipped day becomes a repeat_skip row. A kept day
//     becomes a repeat_override that stores the title, clock, and url only
//     where they differ from the series, so an off-hour clock stays. A day
//     equal to its series in every value, and a change of a deleted series,
//     is dropped.
//
// Rollback: the program never runs version 1 again. To go back, restore the
// file from its backup.
func migrateLegacy(path string) (string, error) {
	have := map[string]map[string]bool{}
	for _, table := range legacyTables {
		cols, err := sqlite.Columns(path, table)
		if err != nil {
			return "", err
		}
		if len(cols) > 0 {
			have[table] = cols
		}
	}
	doItems, err := legacyDoItems(path, have["task"] != nil)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, table := range legacyTables {
		if have[table] != nil {
			fmt.Fprintf(&b, "ALTER TABLE %s RENAME TO %s;\n", table, table+"_v1")
		}
	}
	b.WriteString(crusadeTables)
	b.WriteString("CREATE TEMP TABLE migration_guard (ok INTEGER NOT NULL CHECK (ok = 1));\n")
	if cols := have["task"]; cols != nil {
		b.WriteString(migrateTaskSQL(cols, doItems))
	}
	if have["box"] != nil {
		b.WriteString(migrateBoxSQL(have["box_check"] != nil))
	}
	if have["meta"] != nil {
		b.WriteString(migrateMetaSQL)
	}
	if cols := have["block"]; cols != nil {
		b.WriteString(fmt.Sprintf(migrateBlockSQL, legacyColumn(cols, "url")))
	}
	if cols := have["repeat"]; cols != nil {
		b.WriteString(fmt.Sprintf(migrateRepeatSQL, legacyColumn(cols, "until_on"), legacyColumn(cols, "url")))
		if changeCols := have["repeat_change"]; changeCols != nil {
			b.WriteString(strings.ReplaceAll(migrateRepeatChangeSQL, "c.url", legacyColumn(changeCols, "c.url")))
		}
	}
	if have["deletion"] != nil {
		b.WriteString("INSERT INTO deletion(id, deleted_at) SELECT id, datetime(deleted_at, 'utc') FROM deletion_v1;\n")
	}
	b.WriteString("DROP TABLE migration_guard;\n")
	for _, table := range legacyTables {
		if have[table] != nil {
			fmt.Fprintf(&b, "DROP TABLE %s;\n", table+"_v1")
		}
	}
	b.WriteString("DROP TABLE IF EXISTS app_once;\n")
	return b.String(), nil
}

// legacyDoItems reads the do outcomes of the old task table in list order,
// each on one line.
func legacyDoItems(path string, hasTask bool) ([]string, error) {
	if !hasTask {
		return nil, nil
	}
	var rows []struct {
		Y string `json:"y"`
	}
	if err := sqlite.Query(path, "SELECT y FROM task WHERE "+doTaskWhere+" ORDER BY x, id", &rows); err != nil {
		return nil, err
	}
	items := make([]string, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.Y)
	}
	return items, nil
}

func migrateTaskSQL(cols map[string]bool, doItems []string) string {
	done, doneAt := "0", "NULL"
	if cols["done"] {
		done = "COALESCE(done, 0)"
	}
	if cols["done_at"] {
		doneAt = "datetime(NULLIF(trim(done_at), ''), 'utc')"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "INSERT INTO align(id, x, y, z, done_at) SELECT id, substr(trim(x), 1, 10), y, z, "+
		"CASE WHEN %s <> 0 THEN COALESCE(%s, datetime(substr(trim(x), 1, 10) || ' 00:00:00', 'utc')) END "+
		"FROM task_v1 WHERE lower(trim(COALESCE(mode, ''))) IN ('', 'align', 'think') AND NOT %s;\n", done, doneAt, doTaskWhere)
	written := 0
	for _, text := range doItems {
		item, err := model.NewDoItem(text)
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "INSERT INTO do_item(position, kind, body) VALUES (%d, 'item', %s);\n", written, sqlite.Quote(item.Body))
		written++
	}
	// Every task row is copied: the do rows read above are still all of them,
	// and no row has a mode outside align and do.
	fmt.Fprintf(&b, "INSERT INTO migration_guard(ok) SELECT (SELECT COUNT(*) FROM task_v1 WHERE %s) = %d;\n", doTaskWhere, len(doItems))
	fmt.Fprintf(&b, "INSERT INTO migration_guard(ok) SELECT (SELECT COUNT(*) FROM task_v1) = (SELECT COUNT(*) FROM align) + %d;\n", len(doItems))
	return b.String()
}

func migrateBoxSQL(hasChecks bool) string {
	sql := "INSERT INTO box(id, parent_id, position, label) SELECT b.id, " +
		"(SELECT p.id FROM box_v1 p WHERE p.path = b.parent_path AND b.parent_path <> '' ORDER BY p.id LIMIT 1), " +
		"b.position, b.label FROM box_v1 b;\n" +
		"INSERT INTO migration_guard(ok) SELECT (SELECT COUNT(*) FROM box) = (SELECT COUNT(*) FROM box_v1);\n"
	if hasChecks {
		sql += "INSERT INTO box_check(id, day, box_id) SELECT MIN(c.id), substr(c.day, 1, 10), " +
			"(SELECT b.id FROM box_v1 b WHERE b.path = c.path ORDER BY b.id LIMIT 1) AS box_id " +
			"FROM box_check_v1 c WHERE EXISTS (SELECT 1 FROM box_v1 b WHERE b.path = c.path) " +
			"GROUP BY substr(c.day, 1, 10), box_id;\n"
	}
	return sql
}

const migrateMetaSQL = "UPDATE install SET " +
	"last_mode = COALESCE((SELECT value FROM meta_v1 WHERE key = 'last_mode' AND value IN ('do', 'align', 'box', 'cal', 'nlp') ORDER BY id DESC LIMIT 1), 'do'), " +
	"boxes_imported = EXISTS (SELECT 1 FROM meta_v1 WHERE key = 'boxes_imported' AND value = '1') WHERE id = 1;\n"

// legacyColumn is column when the old table has it, or NULL for a table from
// before that column existed. The name may carry a table alias.
func legacyColumn(cols map[string]bool, column string) string {
	name := column[strings.Index(column, ".")+1:]
	if cols[name] {
		return column
	}
	return "NULL"
}

const migrateBlockSQL = "INSERT INTO block(id, title, start_at, end_at, url) " +
	"SELECT id, title, datetime(start_at), datetime(end_at), NULLIF(trim(COALESCE(%s, '')), '') FROM block_v1;\n"

const migrateRepeatSQL = "INSERT INTO repeat(id, title, start_at, end_at, freq, until_on, url) " +
	"SELECT id, title, datetime(start_at), datetime(end_at), freq, substr(NULLIF(trim(COALESCE(%s, '')), ''), 1, 10), " +
	"NULLIF(trim(COALESCE(%s, '')), '') FROM repeat_v1;\n"

const migrateRepeatChangeSQL = `CREATE TEMP TABLE change_v1 AS
SELECT c.repeat_id AS repeat_id, substr(c.day, 1, 10) AS day, c.skipped AS skipped,
  CASE WHEN c.title IS NULL OR trim(c.title) = '' OR c.title = r.title THEN NULL ELSE c.title END AS title,
  CASE WHEN c.start_at IS NULL OR c.end_at IS NULL OR (time(c.start_at) = time(r.start_at) AND time(c.end_at) = time(r.end_at)) THEN NULL ELSE datetime(c.start_at) END AS start_at,
  CASE WHEN c.start_at IS NULL OR c.end_at IS NULL OR (time(c.start_at) = time(r.start_at) AND time(c.end_at) = time(r.end_at)) THEN NULL ELSE datetime(c.end_at) END AS end_at,
  CASE WHEN c.url IS NULL OR trim(c.url) = '' OR trim(c.url) IS r.url THEN NULL ELSE trim(c.url) END AS url
FROM repeat_change_v1 c JOIN repeat r ON r.id = c.repeat_id
WHERE c.id = (SELECT MAX(c2.id) FROM repeat_change_v1 c2 WHERE c2.repeat_id = c.repeat_id AND substr(c2.day, 1, 10) = substr(c.day, 1, 10));
INSERT INTO repeat_skip(repeat_id, day) SELECT repeat_id, day FROM change_v1 WHERE skipped <> 0;
INSERT INTO repeat_override(repeat_id, day, title, start_at, end_at, url)
SELECT repeat_id, day, title, start_at, end_at, url FROM change_v1
WHERE skipped = 0 AND (title IS NOT NULL OR start_at IS NOT NULL OR url IS NOT NULL);
DROP TABLE change_v1;
`
