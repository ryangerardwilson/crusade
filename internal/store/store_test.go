package store

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryangerardwilson/crusade/internal/model"
)

func TestSaveLoadPreservesStableIDsAndSorts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crusade.sqlite")
	later := mustEvent(t, "2026-02-01 00:00", "later")
	earlier := mustEvent(t, "2026-01-01 00:00", "earlier")
	if err := Save(path, []Record{{ID: 42, Event: later}, {ID: 7, Event: earlier}}); err != nil {
		t.Fatal(err)
	}
	records, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("record count = %d", len(records))
	}
	if records[0].ID != 7 || records[0].Event.Y != "earlier" || records[1].ID != 42 {
		t.Fatalf("stable sorted records = %#v", records)
	}
	if records[0].Event.Mode != model.ModeAlign || records[1].Event.Mode != model.ModeAlign {
		t.Fatalf("modes = %#v", records)
	}
}

func TestCRUDUsesStableIDsAcrossChronologicalReordering(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crusade.sqlite")
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
	if records[1].ID != id || records[1].Event.Y != "updated by stable id" {
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
}

func TestOpenCreatesEmptyDatabaseAndKeepsRows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crusade.sqlite")
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
	if err := Save(dbPath, []Record{{ID: 2, Event: mustEvent(t, "2026-01-01 00:00", "kept")}}); err != nil {
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
	if len(records) != 1 || records[0].ID != 2 || records[0].Event.Y != "kept" {
		t.Fatalf("records = %#v", records)
	}
}

func TestSaveRejectsEmptyImpact(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "crusade.sqlite")
	event := model.Event{X: mustEvent(t, "2026-01-01 00:00", "kept").X, Y: "kept"}
	if err := Save(dbPath, []Record{{ID: 1, Event: event}}); err == nil || !strings.Contains(err.Error(), "impact") {
		t.Fatalf("save without impact = %v", err)
	}
}

func TestDatabaseMatchesSQLX(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SQLX_ROOT", root)
	db := DatabasePath()
	event := mustEvent(t, "2026-04-01 00:00", "stored in sqlx")
	if err := Save(db, []Record{{ID: 4, Event: event}}); err != nil {
		t.Fatal(err)
	}
	bin, err := exec.LookPath("sqlx")
	if err != nil {
		t.Skip("sqlx is not installed")
	}
	inspect := exec.Command(bin, "crusade", "inspect")
	inspectOut, err := inspect.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect: %v\n%s", err, inspectOut)
	}
	if !strings.Contains(string(inspectOut), "orm match") || !strings.Contains(string(inspectOut), "task") {
		t.Fatalf("inspect:\n%s", inspectOut)
	}
	query := exec.Command(bin, "crusade", "query", "select id, y from task")
	queryOut, err := query.CombinedOutput()
	if err != nil {
		t.Fatalf("query: %v\n%s", err, queryOut)
	}
	if !strings.Contains(string(queryOut), "stored in sqlx") || !strings.Contains(string(queryOut), "4") {
		t.Fatalf("query:\n%s", queryOut)
	}
}

func mustEvent(t *testing.T, date, outcome string) model.Event {
	t.Helper()
	event, err := model.NewEvent(date, outcome, "impact", "1", "2", "3")
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func TestDoneColumnMigratesAndRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "crusade.sqlite")
	t.Setenv("BOXES_CONFIG", "")
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
`
	if err := execSQL(path, old); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err != nil {
		t.Fatal(err)
	}
	records, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Event.Done || records[0].Event.Y != "kept task" || !records[0].Event.IsDo() {
		t.Fatalf("migrated count=%d done=%v do=%v", len(records), len(records) == 1 && records[0].Event.Done, len(records) == 1 && records[0].Event.IsDo())
	}
	event := records[0].Event
	event.Done = true
	if err := Update(path, records[0].ID, event); err != nil {
		t.Fatal(err)
	}
	records, err = Load(path)
	if err != nil || len(records) != 1 || !records[0].Event.Done {
		t.Fatalf("marked err=%v", err)
	}
	event = records[0].Event
	event.Done = false
	if err := Update(path, records[0].ID, event); err != nil {
		t.Fatal(err)
	}
	records, err = Load(path)
	if err != nil || len(records) != 1 || records[0].Event.Done {
		t.Fatalf("cleared err=%v", err)
	}
	align := mustEvent(t, "2026-07-01 00:00", "align mark")
	align.Done = true
	if err := Add(path, align); err != nil {
		t.Fatal(err)
	}
	records, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, record := range records {
		if record.Event.Y == "align mark" {
			found = record.Event.Done && record.Event.Mode == model.ModeAlign
		}
	}
	if !found {
		t.Fatal("align done mark was not stored")
	}
}
