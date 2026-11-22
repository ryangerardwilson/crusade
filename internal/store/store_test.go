package store

import (
	"github.com/ryangerardwilson/crusade/internal/sqlite"

	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ryangerardwilson/crusade/internal/model"
)

func TestLoadSortsByDateAndKeepsStableIDs(t *testing.T) {
	path := testDB(t)
	laterID, err := AddRecord(path, mustEvent(t, "2026-02-01 00:00", "later"))
	if err != nil {
		t.Fatal(err)
	}
	earlierID, err := AddRecord(path, mustEvent(t, "2026-01-01 00:00", "earlier"))
	if err != nil {
		t.Fatal(err)
	}
	records, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("record count = %d", len(records))
	}
	if records[0].ID != earlierID || records[0].Align.Y != "earlier" || records[1].ID != laterID {
		t.Fatalf("stable sorted records = %#v", records)
	}
}

func TestCRUDUsesStableIDsAcrossChronologicalReordering(t *testing.T) {
	path := testDB(t)
	id, err := AddRecord(path, mustEvent(t, "2026-06-01 00:00", "first"))
	if err != nil {
		t.Fatal(err)
	}
	if id != 1 {
		t.Fatalf("first id = %d", id)
	}
	secondID, err := AddRecord(path, mustEvent(t, "2026-01-01 00:00", "chronologically earlier"))
	if err != nil {
		t.Fatal(err)
	}
	if secondID != 2 {
		t.Fatalf("second id = %d", secondID)
	}
	if err := Update(path, id, mustEvent(t, "2027-01-01 00:00", "updated by stable id")); err != nil {
		t.Fatal(err)
	}
	records, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if records[1].ID != id || records[1].Align.Y != "updated by stable id" {
		t.Fatalf("update targeted wrong record: %#v", records)
	}
	if err := Delete(path, secondID); err != nil {
		t.Fatal(err)
	}
	records, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ID != id {
		t.Fatalf("delete targeted wrong record: %#v", records)
	}
	if err := Delete(path, secondID); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing delete err=%v", err)
	}
}

func TestOpenCreatesEmptyDatabaseAndKeepsRows(t *testing.T) {
	dbPath := testDB(t)
	created, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("database was not created")
	}
	records, err := Load(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("initialized records = %#v", records)
	}
	id, err := AddRecord(dbPath, mustEvent(t, "2026-01-01 00:00", "kept"))
	if err != nil {
		t.Fatal(err)
	}
	created, err = Open(dbPath)
	if err != nil || created {
		t.Fatalf("second open = %v, %v", created, err)
	}
	records, err = Load(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ID != id || records[0].Align.Y != "kept" {
		t.Fatalf("records = %#v", records)
	}
}

func TestAddRejectsEmptyImpact(t *testing.T) {
	dbPath := testDB(t)
	event := model.Align{X: mustEvent(t, "2026-01-01 00:00", "kept").X, Y: "kept"}
	if err := Add(dbPath, event); err == nil || !strings.Contains(err.Error(), "impact") {
		t.Fatalf("add without impact = %v", err)
	}
}

func TestAddWritesTheAppSQLite(t *testing.T) {
	db := testDB(t)
	event := mustEvent(t, "2026-04-01 00:00", "stored in sqlx")
	if err := Add(db, event); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("/usr/bin/sqlite3", db, "select id, y from align").CombinedOutput()
	if err != nil {
		t.Fatalf("query: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "1|stored in sqlx") {
		t.Fatalf("query:\n%s", out)
	}
}

// testDB points every path the store reads at a temp dir and returns the
// crusade file, which is DatabasePath, so the one-time imports run on it.
func testDB(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("SQLX_ROOT", filepath.Join(root, "sqlx"))
	t.Setenv("BOXES_CONFIG", filepath.Join(root, "missing-outline.txt"))
	t.Setenv("BOXES_DATA_HOME", filepath.Join(root, "boxes"))
	return DatabasePath()
}

func seedSQLite(t *testing.T, sql string) string {
	t.Helper()
	path := testDB(t)
	return seedSQLiteFile(t, path, sql)
}

func seedSQLiteFile(t *testing.T, path, sql string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/usr/bin/sqlite3", path, sql)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("seed sqlite: %v\n%s", err, out)
	}
	return path
}

func mustEvent(t *testing.T, date, outcome string) model.Align {
	t.Helper()
	event, err := model.NewAlign(date, outcome, "impact")
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func TestTaskTableWithoutDoneColumnsMigratesAndRoundTrips(t *testing.T) {
	old := `CREATE TABLE task (
  id INTEGER PRIMARY KEY,
  mode TEXT NOT NULL,
  x TEXT NOT NULL,
  y TEXT NOT NULL,
  z TEXT,
  p REAL,
  q REAL,
  r REAL
);
INSERT INTO task(id, mode, x, y) VALUES (1, 'do', '2026-06-13 23:59:59', 'kept task');
INSERT INTO task(id, mode, x, y, z) VALUES (2, 'align', '2026-07-01 00:00:00', 'align mark', 'impact');
`
	path := seedSQLite(t, old)
	if _, err := Open(path); err != nil {
		t.Fatal(err)
	}
	list, err := LoadDoList(path)
	if err != nil || len(list) != 1 || list[0].Kind != model.DoKindItem || list[0].Body != "kept task" {
		t.Fatalf("do list = %#v err=%v", list, err)
	}
	records, err := Load(path)
	if err != nil || len(records) != 1 || records[0].ID != 2 || records[0].Align.Done() {
		t.Fatalf("records = %#v err=%v", records, err)
	}
	event := records[0].Align
	event.DoneAt = time.Date(2026, 7, 2, 6, 10, 0, 0, time.Local)
	if err := Update(path, 2, event); err != nil {
		t.Fatal(err)
	}
	records, err = Load(path)
	if err != nil || len(records) != 1 || !records[0].Align.Done() || !records[0].Align.DoneAt.Equal(event.DoneAt) {
		t.Fatalf("marked = %#v err=%v", records, err)
	}
	event.DoneAt = time.Time{}
	if err := Update(path, 2, event); err != nil {
		t.Fatal(err)
	}
	records, err = Load(path)
	if err != nil || len(records) != 1 || records[0].Align.Done() {
		t.Fatalf("cleared = %#v err=%v", records, err)
	}
}

func TestDoneAtRoundTripsInUTCAndClears(t *testing.T) {
	path := testDB(t)
	event := mustEvent(t, "2026-10-22 00:00", "fold clothes")
	event.DoneAt = time.Date(2026, 10, 30, 6, 10, 0, 0, time.Local)
	id, err := AddRecord(path, event)
	if err != nil {
		t.Fatal(err)
	}
	var stored []struct {
		DoneAt string `json:"done_at"`
	}
	if err := sqlite.Query(path, "SELECT done_at FROM align", &stored); err != nil || len(stored) != 1 {
		t.Fatalf("stored = %#v err=%v", stored, err)
	}
	if want := event.DoneAt.UTC().Format(model.DateTimeFormat); stored[0].DoneAt != want {
		t.Fatalf("stored done_at = %q, want UTC %q", stored[0].DoneAt, want)
	}
	records, err := Load(path)
	if err != nil || len(records) != 1 || !records[0].Align.Done() || !records[0].Align.DoneAt.Equal(event.DoneAt) {
		t.Fatalf("loaded = %#v err=%v", records, err)
	}
	cleared := records[0].Align
	cleared.DoneAt = time.Time{}
	if err := Update(path, id, cleared); err != nil {
		t.Fatal(err)
	}
	records, err = Load(path)
	if err != nil || len(records) != 1 || records[0].Align.Done() || !records[0].Align.DoneAt.IsZero() {
		t.Fatalf("cleared = %#v err=%v", records, err)
	}
}

func TestDeleteCountsDoItemsThisWeek(t *testing.T) {
	path := testDB(t)
	itemID, err := AddDoItem(path, 0, "fold clothes")
	if err != nil {
		t.Fatal(err)
	}
	alignID, err := AddRecord(path, mustEvent(t, "2026-06-01 00:00", "align outcome"))
	if err != nil {
		t.Fatal(err)
	}
	if err := Delete(path, alignID); err != nil {
		t.Fatal(err)
	}
	if count, err := DeletedThisWeek(path, time.Now()); err != nil || count != 0 {
		t.Fatalf("align delete count=%d err=%v", count, err)
	}
	if err := DeleteDoItem(path, itemID); err != nil {
		t.Fatal(err)
	}
	if count, err := DeletedThisWeek(path, time.Now()); err != nil || count != 1 {
		t.Fatalf("do delete count=%d err=%v", count, err)
	}
	if err := DeleteDoItem(path, itemID); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing delete err=%v", err)
	}
	if count, err := DeletedThisWeek(path, time.Now()); err != nil || count != 1 {
		t.Fatalf("missing delete changed count=%d err=%v", count, err)
	}
	older := time.Now().AddDate(0, 0, -8).UTC().Format(model.DateTimeFormat)
	if err := execSQL(path, "INSERT INTO deletion(id, deleted_at) VALUES (50, '"+older+"');"); err != nil {
		t.Fatal(err)
	}
	if count, err := DeletedThisWeek(path, time.Now()); err != nil || count != 1 {
		t.Fatalf("old deletion count=%d err=%v", count, err)
	}
	list, err := LoadDoList(path)
	if err != nil || len(list) != 0 {
		t.Fatalf("do rows left=%d err=%v", len(list), err)
	}
}

func TestDeletingAHeadingOrBlankIsNotCounted(t *testing.T) {
	path := testDB(t)
	if err := ReplaceDoList(path, model.ParseDoList("# Work\n\n- ship it\n")); err != nil {
		t.Fatal(err)
	}
	list, err := LoadDoList(path)
	if err != nil || len(list) != 3 {
		t.Fatalf("list = %#v err=%v", list, err)
	}
	for _, row := range list[:2] {
		if err := DeleteDoItem(path, row.ID); err != nil {
			t.Fatal(err)
		}
	}
	if count, err := DeletedThisWeek(path, time.Now()); err != nil || count != 0 {
		t.Fatalf("heading and blank deletions counted=%d err=%v", count, err)
	}
}

func TestDeletionsDuringWeekStartsOnMonday(t *testing.T) {
	monday := time.Date(2026, time.June, 15, 8, 0, 0, 0, time.Local)
	sunday := time.Date(2026, time.June, 21, 23, 59, 59, 0, time.Local)
	previous := time.Date(2026, time.June, 14, 23, 59, 59, 0, time.Local)
	next := time.Date(2026, time.June, 22, 0, 0, 1, 0, time.Local)
	stamps := []time.Time{monday, sunday, previous, next}
	for _, now := range []time.Time{monday, time.Date(2026, time.June, 17, 12, 0, 0, 0, time.Local), sunday} {
		if got := deletionsDuringWeek(stamps, now); got != 2 {
			t.Fatalf("now %s deleted this week = %d", now.Format(time.RFC3339), got)
		}
	}
}

func TestAlignDropsScoresAndStoresTheDate(t *testing.T) {
	old := `CREATE TABLE task (
  id INTEGER PRIMARY KEY,
  mode TEXT NOT NULL,
  x TEXT NOT NULL,
  y TEXT NOT NULL,
  z TEXT,
  p REAL,
  q REAL,
  r REAL
);
INSERT INTO task(id, mode, x, y, z, p, q, r) VALUES
  (1, 'align', '2026-01-26 18:30:00', 'ship the patch', 'reduce drift', 7, 8, 6),
  (2, '', '2026-06-13 23:59:59', 'kept task', 'done', 0, 0, 0),
  (3, 'do', '2026-06-14 23:59:59', 'mail the letter', NULL, NULL, NULL, NULL),
  (4, 'think', '2026-03-01 00:00:00', 'real outcome', 'clear head', 1, 0, 0);
`
	path := seedSQLite(t, old)
	if _, err := Open(path); err != nil {
		t.Fatal(err)
	}
	var cols []struct {
		Name string `json:"name"`
	}
	if err := sqlite.Query(path, "SELECT name FROM pragma_table_info('align') WHERE name IN ('p', 'mode')", &cols); err != nil || len(cols) != 0 {
		t.Fatalf("old columns remain: %#v %v", cols, err)
	}
	var rows []struct {
		ID int    `json:"id"`
		X  string `json:"x"`
	}
	if err := sqlite.Query(path, "SELECT id, x FROM align ORDER BY id", &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ID != 1 || rows[0].X != "2026-01-26" || rows[1].ID != 4 || rows[1].X != "2026-03-01" {
		t.Fatalf("align rows = %#v", rows)
	}
	list, err := LoadDoList(path)
	if err != nil || len(list) != 2 || list[0].Body != "kept task" || list[1].Body != "mail the letter" {
		t.Fatalf("do list = %#v err=%v", list, err)
	}
	records, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].Align.Y != "ship the patch" || records[1].Align.Y != "real outcome" || !records[1].Align.X.Equal(time.Date(2026, 3, 1, 0, 0, 0, 0, time.Local)) {
		t.Fatalf("loaded = %#v", records)
	}
}
