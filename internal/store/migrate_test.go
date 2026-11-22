package store

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ryangerardwilson/crusade/internal/config"
	"github.com/ryangerardwilson/crusade/internal/model"
	"github.com/ryangerardwilson/crusade/internal/sqlite"
)

// legacySchema is a crusade file before schema version 2.
const legacySchema = `CREATE TABLE task (
  id INTEGER PRIMARY KEY,
  mode TEXT NOT NULL,
  x TEXT NOT NULL,
  y TEXT NOT NULL,
  z TEXT,
  done INTEGER NOT NULL DEFAULT 0,
  done_at TEXT
);
CREATE TABLE box (
  id INTEGER PRIMARY KEY,
  path TEXT NOT NULL UNIQUE,
  label TEXT NOT NULL,
  parent_path TEXT NOT NULL DEFAULT '',
  position INTEGER NOT NULL,
  depth INTEGER NOT NULL
);
CREATE TABLE box_check (
  id INTEGER PRIMARY KEY,
  day TEXT NOT NULL,
  path TEXT NOT NULL
);
CREATE TABLE meta (
  id INTEGER PRIMARY KEY,
  key TEXT NOT NULL UNIQUE,
  value TEXT NOT NULL
);
CREATE TABLE block (
  id INTEGER PRIMARY KEY,
  title TEXT NOT NULL,
  start_at TEXT NOT NULL,
  end_at TEXT NOT NULL,
  url TEXT
);
CREATE TABLE repeat (
  id INTEGER PRIMARY KEY,
  title TEXT NOT NULL,
  start_at TEXT NOT NULL,
  end_at TEXT NOT NULL,
  freq TEXT NOT NULL,
  until_on TEXT,
  url TEXT
);
CREATE TABLE repeat_change (
  id INTEGER PRIMARY KEY,
  repeat_id INTEGER NOT NULL,
  day TEXT NOT NULL,
  skipped INTEGER NOT NULL DEFAULT 0,
  title TEXT,
  start_at TEXT,
  end_at TEXT,
  url TEXT
);
CREATE TABLE deletion (
  id INTEGER PRIMARY KEY,
  deleted_at TEXT NOT NULL
);
`

// legacyRows fills legacySchema. Tasks 1-3 are align records (done with a
// time, done with none, not done); 4 and 5 are do outcomes, 5 first by x.
const legacyRows = `INSERT INTO task(id, mode, x, y, z, done, done_at) VALUES
  (1, 'align', '2026-01-26 18:30:00', 'ship the patch', 'reduce drift', 1, '2026-01-27 08:15:00'),
  (2, 'align', '2026-02-01 00:00:00', 'write the memo', 'clear plan', 1, NULL),
  (3, 'align', '2026-03-01 00:00:00', 'not yet', 'later', 0, NULL),
  (4, 'do', '2026-05-02 23:59:59', 'second do', NULL, 0, NULL),
  (5, 'do', '2026-05-01 23:59:59', 'first do', NULL, 0, NULL);
INSERT INTO box(id, path, label, parent_path, position, depth) VALUES
  (10, 'work', 'Work', '', 0, 0),
  (11, 'work/email', 'Email', 'work', 0, 1),
  (12, 'move', 'Move', '', 1, 0);
INSERT INTO box_check(id, day, path) VALUES
  (1, '2026-10-20', 'work/email'),
  (2, '2026-10-20', 'move'),
  (3, '2026-10-21', 'gone/path');
INSERT INTO meta(id, key, value) VALUES (1, 'last_mode', 'cal'), (2, 'boxes_imported', '1');
INSERT INTO block(id, title, start_at, end_at, url) VALUES (1, 'deep work', '2026-11-03 09:00:00', '2026-11-03 10:00:00', NULL);
INSERT INTO repeat(id, title, start_at, end_at, freq, until_on, url) VALUES
  (1, 'dev meeting', '2026-11-02 19:30:00', '2026-11-02 20:30:00', 'weekly', NULL, 'https://meet.example/dev');
INSERT INTO repeat_change(id, repeat_id, day, skipped, title, start_at, end_at, url) VALUES
  (1, 1, '2026-11-09', 1, NULL, NULL, NULL, NULL),
  (2, 1, '2026-11-16', 0, 'dev meeting', '2026-11-16 19:30:00', '2026-11-16 20:30:00', 'https://meet.example/dev'),
  (3, 1, '2026-11-23', 0, 'retro', '2026-11-23 19:45:00', '2026-11-23 20:45:00', 'https://meet.example/retro'),
  (4, 1, '2026-11-30', 0, 'demo', NULL, NULL, '');
INSERT INTO deletion(id, deleted_at) VALUES (1, '2026-11-01 10:00:00');
`

// seedLegacy writes the old file at DatabasePath and returns it.
func seedLegacy(t *testing.T, extra string) string {
	t.Helper()
	return seedSQLite(t, legacySchema+legacyRows+extra)
}

func sqliteDump(t *testing.T, db string) string {
	t.Helper()
	out, err := exec.Command("/usr/bin/sqlite3", db, ".dump").CombinedOutput()
	if err != nil {
		t.Fatalf("dump: %v\n%s", err, out)
	}
	return string(out)
}

func tableNames(t *testing.T, db string) map[string]bool {
	t.Helper()
	var rows []struct {
		Name string `json:"name"`
	}
	if err := sqlite.Query(db, "SELECT name FROM sqlite_master WHERE type = 'table'", &rows); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, row := range rows {
		names[row.Name] = true
	}
	return names
}

func countRows(t *testing.T, db, sql string) int {
	t.Helper()
	var rows []struct {
		N int `json:"n"`
	}
	if err := sqlite.Query(db, "SELECT ("+sql+") AS n", &rows); err != nil || len(rows) != 1 {
		t.Fatalf("%s: %#v %v", sql, rows, err)
	}
	return rows[0].N
}

func TestOpenMigratesTheLegacySchema(t *testing.T) {
	db := seedLegacy(t, "")
	if _, err := os.Stat(config.DoPath()); !os.IsNotExist(err) {
		t.Fatalf("do file stat = %v", err)
	}
	if _, err := Open(db); err != nil {
		t.Fatal(err)
	}
	assertMigratedSchema(t, db)
	assertMigratedAlign(t, db)
	assertMigratedInstall(t, db, 1)
	assertMigratedBoxes(t, db)
	assertMigratedRepeats(t, db)
	list, err := LoadDoList(db)
	if err != nil || len(list) != 2 || list[0].Body != "first do" || list[1].Body != "second do" || list[0].Kind != model.DoKindItem || list[1].Kind != model.DoKindItem {
		t.Fatalf("do list = %#v err=%v", list, err)
	}
	before := sqliteDump(t, db)
	if _, err := Open(db); err != nil {
		t.Fatal(err)
	}
	if after := sqliteDump(t, db); after != before {
		t.Fatalf("second open changed the file:\n%s\n---\n%s", before, after)
	}
	if _, err := os.Stat(config.DoPath()); !os.IsNotExist(err) {
		t.Fatalf("open wrote a do file: %v", err)
	}
}

func assertMigratedSchema(t *testing.T, db string) {
	t.Helper()
	if version, err := sqlite.UserVersion(db); err != nil || version != 2 {
		t.Fatalf("user_version = %d %v", version, err)
	}
	names := tableNames(t, db)
	for _, gone := range []string{"task", "meta", "repeat_change", "task_v1", "box_v1", "box_check_v1", "meta_v1", "block_v1", "repeat_v1", "repeat_change_v1", "deletion_v1"} {
		if names[gone] {
			t.Fatalf("table %s remains: %v", gone, names)
		}
	}
	for _, want := range []string{"install", "do_item", "deletion", "align", "box", "box_check", "block", "repeat", "repeat_skip", "repeat_override"} {
		if !names[want] {
			t.Fatalf("table %s missing: %v", want, names)
		}
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM deletion WHERE id = 1 AND deleted_at IS datetime('2026-11-01 10:00:00', 'utc')"); n != 1 {
		t.Fatal("deletion was not moved to UTC")
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM block WHERE id = 1 AND url IS NULL"); n != 1 {
		t.Fatal("block was not copied")
	}
}

func assertMigratedAlign(t *testing.T, db string) {
	t.Helper()
	records, err := Load(db)
	if err != nil || len(records) != 3 {
		t.Fatalf("align records = %#v err=%v", records, err)
	}
	want := map[int]time.Time{
		1: time.Date(2026, 1, 27, 8, 15, 0, 0, time.Local),
		2: time.Date(2026, 2, 1, 0, 0, 0, 0, time.Local),
		3: {},
	}
	for _, record := range records {
		at, ok := want[record.ID]
		if !ok || !record.Align.DoneAt.Equal(at) {
			t.Fatalf("record %d done_at = %s, want %s", record.ID, record.Align.DoneAt, at)
		}
	}
	if records[0].ID != 1 || records[0].Align.X.Format(time.DateOnly) != "2026-01-26" || records[0].Align.Z != "reduce drift" {
		t.Fatalf("first record = %#v", records[0])
	}
	sql := "SELECT COUNT(*) FROM align WHERE (id = 1 AND done_at IS datetime('2026-01-27 08:15:00', 'utc')) OR " +
		"(id = 2 AND done_at IS datetime('2026-02-01 00:00:00', 'utc')) OR (id = 3 AND done_at IS NULL)"
	if n := countRows(t, db, sql); n != 3 {
		t.Fatalf("stored done_at match %d of 3 rows", n)
	}
}

func assertMigratedInstall(t *testing.T, db string, doImported int) {
	t.Helper()
	var rows []struct {
		Mode  string `json:"last_mode"`
		Boxes int    `json:"boxes_imported"`
		Do    int    `json:"do_imported"`
	}
	if err := sqlite.Query(db, "SELECT last_mode, boxes_imported, do_imported FROM install", &rows); err != nil || len(rows) != 1 {
		t.Fatalf("install = %#v %v", rows, err)
	}
	if rows[0].Mode != model.ModeCal || rows[0].Boxes != 1 || rows[0].Do != doImported {
		t.Fatalf("install = %#v", rows[0])
	}
}

func assertMigratedBoxes(t *testing.T, db string) {
	t.Helper()
	var rows []boxRow
	if err := sqlite.Query(db, "SELECT id, parent_id, position, label FROM box ORDER BY id", &rows); err != nil || len(rows) != 3 {
		t.Fatalf("boxes = %#v %v", rows, err)
	}
	if rows[0].ID != 10 || rows[0].Parent != nil || rows[1].ID != 11 || rows[1].Parent == nil || *rows[1].Parent != 10 || rows[2].ID != 12 || rows[2].Parent != nil {
		t.Fatalf("boxes = %#v", rows)
	}
	checked, err := LoadDayChecks(db, "2026-10-20")
	if err != nil || len(checked) != 2 || !checked[11] || !checked[12] {
		t.Fatalf("checks = %#v %v", checked, err)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM box_check"); n != 2 {
		t.Fatalf("box_check rows = %d, the orphan path was kept", n)
	}
	outline, err := LoadOutline(db)
	if err != nil {
		t.Fatal(err)
	}
	if id, ok := boxIDAt(outline, "work/email"); !ok || id != 11 {
		t.Fatalf("outline = %#v", outline)
	}
}

func assertMigratedRepeats(t *testing.T, db string) {
	t.Helper()
	if n := countRows(t, db, "SELECT COUNT(*) FROM repeat_skip WHERE repeat_id = 1 AND day = '2026-11-09'"); n != 1 || countRows(t, db, "SELECT COUNT(*) FROM repeat_skip") != 1 {
		t.Fatal("skipped day was not moved")
	}
	var rows []overrideRow
	if err := sqlite.Query(db, "SELECT repeat_id, day, title, start_at, end_at, url FROM repeat_override ORDER BY day", &rows); err != nil || len(rows) != 2 {
		t.Fatalf("overrides = %#v %v", rows, err)
	}
	retro, demo := rows[0], rows[1]
	if retro.Day != "2026-11-23" || ptr(retro.Title) != "retro" || ptr(retro.StartAt) != "2026-11-23 19:45:00" || ptr(retro.EndAt) != "2026-11-23 20:45:00" || ptr(retro.URL) != "https://meet.example/retro" {
		t.Fatalf("retro = %s %s %s %s %s", retro.Day, ptr(retro.Title), ptr(retro.StartAt), ptr(retro.EndAt), ptr(retro.URL))
	}
	if demo.Day != "2026-11-30" || ptr(demo.Title) != "demo" || demo.StartAt != nil || demo.EndAt != nil || demo.URL != nil {
		t.Fatalf("demo = %s %s %s %s %s", demo.Day, ptr(demo.Title), ptr(demo.StartAt), ptr(demo.EndAt), ptr(demo.URL))
	}
	assertMigratedSeriesDays(t, db)
}

// assertMigratedSeriesDays checks the migrated days as the calendar shows them.
func assertMigratedSeriesDays(t *testing.T, db string) {
	t.Helper()
	repeats, err := LoadRepeats(db)
	if err != nil || len(repeats) != 1 {
		t.Fatalf("repeats = %#v %v", repeats, err)
	}
	series := repeats[0]
	if _, ok := series.On(localDay(2026, 11, 9)); ok {
		t.Fatal("skipped day shows")
	}
	same, ok := series.On(localDay(2026, 11, 16))
	if !ok || same.Overridden || same.Start.Format("15:04") != "19:30" || same.URL != "https://meet.example/dev" {
		t.Fatalf("16 Nov = %#v", same)
	}
	off, ok := series.On(localDay(2026, 11, 23))
	if !ok || off.Title != "retro" || off.Start.Format("15:04") != "19:45" || off.End.Format("15:04") != "20:45" || off.URL != "https://meet.example/retro" {
		t.Fatalf("23 Nov = %#v", off)
	}
	if demo, ok := series.On(localDay(2026, 11, 30)); !ok || demo.Title != "demo" || demo.URL != "https://meet.example/dev" {
		t.Fatalf("30 Nov = %#v", demo)
	}
}

func ptr(value *string) string {
	if value == nil {
		return "<null>"
	}
	return *value
}

func TestOpenImportsTheDoFileOnce(t *testing.T) {
	db := seedLegacy(t, "")
	doPath := config.DoPath()
	const text = "# Work\n\n- ship it\nplain line\n"
	if err := os.MkdirAll(filepath.Dir(doPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(doPath, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(db); err != nil {
		t.Fatal(err)
	}
	assertMigratedInstall(t, db, 1)
	list, err := LoadDoList(db)
	if err != nil {
		t.Fatal(err)
	}
	want := []model.DoItem{{Kind: model.DoKindHeading, Body: "# Work"}, {Kind: model.DoKindBlank}, {Kind: model.DoKindItem, Body: "ship it"}, {Kind: model.DoKindItem, Body: "plain line"}}
	if len(list) != len(want) {
		t.Fatalf("do list = %#v", list)
	}
	for index, row := range list {
		if row.Kind != want[index].Kind || row.Body != want[index].Body {
			t.Fatalf("row %d = %#v, want %#v", index, row, want[index])
		}
	}
	if got, err := os.ReadFile(doPath); err != nil || string(got) != text {
		t.Fatalf("do file = %q %v", got, err)
	}
	before := sqliteDump(t, db)
	if err := os.WriteFile(doPath, []byte("- changed later\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(db); err != nil {
		t.Fatal(err)
	}
	if after := sqliteDump(t, db); after != before {
		t.Fatalf("second open imported again:\n%s\n---\n%s", before, after)
	}
}

func TestFailedMigrationLeavesTheOldTables(t *testing.T) {
	db := seedLegacy(t, "INSERT INTO task(id, mode, x, y, z, done) VALUES (6, 'bogus', '2026-04-01 00:00:00', 'odd row', 'why', 0);\n")
	before := sqliteDump(t, db)
	if _, err := Open(db); err == nil {
		t.Fatal("a migration with an unknown task mode succeeded")
	}
	if _, err := os.Stat(db); err != nil {
		t.Fatalf("the old file is gone: %v", err)
	}
	if version, err := sqlite.UserVersion(db); err != nil || version != 0 {
		t.Fatalf("user_version = %d %v", version, err)
	}
	names := tableNames(t, db)
	if !names["task"] || !names["meta"] || !names["repeat_change"] || names["align"] || names["install"] || names["task_v1"] {
		t.Fatalf("tables after a failed migration = %v", names)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM task"); n != 6 {
		t.Fatalf("task rows = %d", n)
	}
	if after := sqliteDump(t, db); after != before {
		t.Fatalf("failed migration changed the file:\n%s\n---\n%s", before, after)
	}
}

func TestFailedStatementInAStoreWriteLeavesNothing(t *testing.T) {
	db := testDB(t)
	if _, err := Open(db); err != nil {
		t.Fatal(err)
	}
	err := sqlite.Exec(db, sqlite.Tx(
		"INSERT INTO align(x, y, z) VALUES ('2026-01-01', 'a', 'b');\n",
		"INSERT INTO align(x, y, z) VALUES ('not a date', 'a', 'b');\n",
	))
	if err == nil {
		t.Fatal("a failed statement returned no error")
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM align"); n != 0 {
		t.Fatalf("align rows = %d after a failed transaction", n)
	}
	err = sqlite.Exec(db, "INSERT INTO box_check(day, box_id) VALUES ('2026-01-01', 999);\n")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "foreign key") {
		t.Fatalf("check of a missing box = %v", err)
	}
	if n := countRows(t, db, "SELECT COUNT(*) FROM box_check"); n != 0 {
		t.Fatalf("box_check rows = %d", n)
	}
}

func TestOpenMigratesCalendarTablesFromBeforeMeetingLinks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("SQLX_ROOT", filepath.Join(home, "sqlx"))
	t.Setenv("BOXES_CONFIG", filepath.Join(home, "missing-outline.txt"))
	db := DatabasePath()
	if err := os.MkdirAll(filepath.Dir(db), 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := `CREATE TABLE task (id INTEGER PRIMARY KEY, mode TEXT NOT NULL, x TEXT NOT NULL, y TEXT NOT NULL, z TEXT, done INTEGER NOT NULL DEFAULT 0, done_at TEXT);
CREATE TABLE block (id INTEGER PRIMARY KEY, title TEXT NOT NULL, start_at TEXT NOT NULL, end_at TEXT NOT NULL);
CREATE TABLE repeat (id INTEGER PRIMARY KEY, title TEXT NOT NULL, start_at TEXT NOT NULL, end_at TEXT NOT NULL, freq TEXT NOT NULL, until_on TEXT);
CREATE TABLE repeat_change (id INTEGER PRIMARY KEY, repeat_id INTEGER NOT NULL, day TEXT NOT NULL, skipped INTEGER NOT NULL, title TEXT, start_at TEXT, end_at TEXT);
INSERT INTO block VALUES (1, 'focus', '2026-11-09 09:00:00', '2026-11-09 10:00:00');
INSERT INTO repeat VALUES (1, 'standup', '2026-11-09 19:30:00', '2026-11-09 20:30:00', 'daily', NULL);
INSERT INTO repeat_change VALUES (1, 1, '2026-11-10', 0, 'retro', '2026-11-10 19:30:00', '2026-11-10 20:30:00');
INSERT INTO repeat_change VALUES (2, 1, '2026-11-11', 1, NULL, NULL, NULL);
`
	if out, err := exec.Command("/usr/bin/sqlite3", db, legacy).CombinedOutput(); err != nil {
		t.Fatalf("seed: %v %s", err, out)
	}
	if _, err := Open(db); err != nil {
		t.Fatal(err)
	}
	blocks, err := LoadBlocks(db)
	if err != nil || len(blocks) != 1 || blocks[0].Block.URL != "" {
		t.Fatalf("blocks = %#v err=%v", blocks, err)
	}
	repeats, err := LoadRepeats(db)
	if err != nil || len(repeats) != 1 || len(repeats[0].Skips) != 1 || len(repeats[0].Overrides) != 1 {
		t.Fatalf("repeats = %#v err=%v", repeats, err)
	}
	day := time.Date(2026, 11, 10, 0, 0, 0, 0, time.Local)
	if occurrence, ok := repeats[0].On(day); !ok || occurrence.Title != "retro" || occurrence.Start.Hour() != 19 || occurrence.Start.Minute() != 30 {
		t.Fatalf("10 Nov = %#v ok=%v", occurrence, ok)
	}
}
