package store

import (
	"github.com/ryangerardwilson/crusade/internal/sqlite"

	"os"
	"path/filepath"
	"testing"

	"github.com/ryangerardwilson/crusade/internal/box"
	"github.com/ryangerardwilson/crusade/internal/model"
)

// boxIDAt is the id of the box with the label path, as box.Outline.Paths names it.
func boxIDAt(outline box.Outline, path string) (int, bool) {
	paths := outline.Paths()
	for index, item := range outline.Flatten() {
		if paths[index] == path {
			return item.ID, true
		}
	}
	return 0, false
}

func TestImportBoxesOnceThenIgnoresSourceEdits(t *testing.T) {
	db := testDB(t)
	outlinePath := filepath.Join(t.TempDir(), "rituals.txt")
	if err := os.WriteFile(outlinePath, []byte("Move\nWork\n  Email\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BOXES_CONFIG", outlinePath)
	dayHome := t.TempDir()
	t.Setenv("BOXES_DATA_HOME", dayHome)
	if err := os.MkdirAll(filepath.Join(dayHome, "days"), 0o755); err != nil {
		t.Fatal(err)
	}
	day := []byte(`{"date":"2026-10-20","checked_ids":["work/email"]}`)
	if err := os.WriteFile(filepath.Join(dayHome, "days", "2026-10-20.json"), day, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Open(db); err != nil {
		t.Fatal(err)
	}
	outline, err := LoadOutline(db)
	if err != nil {
		t.Fatal(err)
	}
	emailID, ok := boxIDAt(outline, "work/email")
	if !ok || emailID == 0 {
		t.Fatalf("missing imported box: %#v", outline)
	}
	checked, err := LoadDayChecks(db, "2026-10-20")
	if err != nil || !checked[emailID] || len(checked) != 1 {
		t.Fatalf("checks = %#v err=%v", checked, err)
	}
	if err := os.WriteFile(outlinePath, []byte("Other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(db); err != nil {
		t.Fatal(err)
	}
	outline, err = LoadOutline(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := boxIDAt(outline, "move"); !ok {
		t.Fatal("outline was imported again")
	}
	if _, ok := boxIDAt(outline, "other"); ok {
		t.Fatal("later source edit became the outline")
	}
}

func TestOpenAdoptsXecuteDatabase(t *testing.T) {
	db := testDB(t)
	legacy := filepath.Join(filepath.Dir(db), legacyDatabaseName+".sqlite")
	seedSQLiteFile(t, legacy, `CREATE TABLE task (
  "id" INTEGER PRIMARY KEY,
  "mode" TEXT NOT NULL,
  "x" TEXT NOT NULL,
  "y" TEXT NOT NULL,
  "z" TEXT,
  "done" INTEGER NOT NULL,
  "done_at" TEXT
);
INSERT INTO task(id, mode, x, y, z, done) VALUES (7, 'align', '2026-04-01 00:00:00', 'kept task', 'impact', 0);
INSERT INTO task(id, mode, x, y, done) VALUES (8, 'do', '2026-04-01 23:59:59', 'kept do', 0);
`)
	if _, err := Open(db); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy database stat = %v", err)
	}
	records, err := Load(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ID != 7 || records[0].Align.Y != "kept task" {
		t.Fatalf("records = %#v", records)
	}
	list, err := LoadDoList(db)
	if err != nil || len(list) != 1 || list[0].Body != "kept do" {
		t.Fatalf("do list = %#v err=%v", list, err)
	}
}

func TestLoadActiveDaysKeepsDaysWithACheck(t *testing.T) {
	path := testDB(t)
	move, err := AddBox(path, 0, "Move")
	if err != nil {
		t.Fatal(err)
	}
	work, err := AddBox(path, move, "Work")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ToggleBox(path, "2026-10-18", move); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{move, work} {
		if _, err := ToggleBox(path, "2026-10-19", id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ToggleBox(path, "2026-10-20", move); err != nil {
		t.Fatal(err)
	}
	if err := ClearDayChecks(path, "2026-10-20"); err != nil {
		t.Fatal(err)
	}
	days, err := LoadActiveDays(path)
	if err != nil {
		t.Fatal(err)
	}
	if !days["2026-10-18"] || !days["2026-10-19"] || days["2026-10-20"] || len(days) != 2 {
		t.Fatalf("days = %#v", days)
	}
	counts, err := LoadCheckCounts(path)
	if err != nil {
		t.Fatal(err)
	}
	if counts["2026-10-18"] != 1 || counts["2026-10-19"] != 2 || counts["2026-10-20"] != 0 || len(counts) != 2 {
		t.Fatalf("counts = %#v", counts)
	}
}

func TestRenameBoxKeepsItsIDAndChecks(t *testing.T) {
	db := testDB(t)
	id, err := AddBox(db, 0, "Email")
	if err != nil {
		t.Fatal(err)
	}
	if on, err := ToggleBox(db, "2026-10-20", id); err != nil || !on {
		t.Fatalf("toggle on=%v err=%v", on, err)
	}
	if err := RenameBox(db, id, "Inbox"); err != nil {
		t.Fatal(err)
	}
	outline, err := LoadOutline(db)
	if err != nil {
		t.Fatal(err)
	}
	item, ok := outline.ItemByID(id)
	if !ok || item.Label != "Inbox" || len(outline.Flatten()) != 1 {
		t.Fatalf("renamed outline = %#v", outline)
	}
	checked, err := LoadDayChecks(db, "2026-10-20")
	if err != nil || !checked[id] || len(checked) != 1 {
		t.Fatalf("checks after rename = %#v err=%v", checked, err)
	}
}

func TestDeleteBoxRemovesChildrenAndTheirChecks(t *testing.T) {
	db := testDB(t)
	parent, child, other := seedBoxTree(t, db)
	if on, err := ToggleBox(db, "2026-10-20", parent); err != nil || !on {
		t.Fatalf("toggle parent on=%v err=%v", on, err)
	}
	if _, err := ToggleBox(db, "2026-10-20", other); err != nil {
		t.Fatal(err)
	}
	checked, err := LoadDayChecks(db, "2026-10-20")
	if err != nil || !checked[child] || !checked[other] || checked[parent] {
		t.Fatalf("checks before delete = %#v err=%v", checked, err)
	}
	if err := DeleteBox(db, parent); err != nil {
		t.Fatal(err)
	}
	outline, err := LoadOutline(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := outline.ItemByID(child); ok || len(outline.Flatten()) != 1 {
		t.Fatalf("outline after delete = %#v", outline)
	}
	checked, err = LoadDayChecks(db, "2026-10-20")
	if err != nil || len(checked) != 1 || !checked[other] {
		t.Fatalf("checks after delete = %#v err=%v", checked, err)
	}
	var rows []struct {
		N int `json:"n"`
	}
	if err := sqlite.Query(db, "SELECT COUNT(*) AS n FROM box_check WHERE box_id NOT IN (SELECT id FROM box)", &rows); err != nil || len(rows) != 1 || rows[0].N != 0 {
		t.Fatalf("orphan checks = %#v err=%v", rows, err)
	}
}

// seedBoxTree stores Work with the child Email, and Move. It returns the ids
// of Work, Email, and Move.
func seedBoxTree(t *testing.T, db string) (int, int, int) {
	t.Helper()
	outline, err := box.ParseOutline("Work\n  Email\nMove\n")
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveOutline(db, outline); err != nil {
		t.Fatal(err)
	}
	stored, err := LoadOutline(db)
	if err != nil {
		t.Fatal(err)
	}
	work, okWork := boxIDAt(stored, "work")
	email, okEmail := boxIDAt(stored, "work/email")
	move, okMove := boxIDAt(stored, "move")
	if !okWork || !okEmail || !okMove {
		t.Fatalf("seeded outline = %#v", stored)
	}
	return work, email, move
}

func TestSaveOutlineKeepsIDsOfUnchangedPaths(t *testing.T) {
	db := testDB(t)
	work, email, move := seedBoxTree(t, db)
	if _, err := ToggleBox(db, "2026-10-20", email); err != nil {
		t.Fatal(err)
	}
	if _, err := ToggleBox(db, "2026-10-20", move); err != nil {
		t.Fatal(err)
	}
	next, err := box.ParseOutline("Move\nWork\n  Email\n  Chat\n")
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveOutline(db, next); err != nil {
		t.Fatal(err)
	}
	stored, err := LoadOutline(db)
	if err != nil {
		t.Fatal(err)
	}
	if id, _ := boxIDAt(stored, "work"); id != work {
		t.Fatalf("work id = %d, want %d", id, work)
	}
	if id, _ := boxIDAt(stored, "work/email"); id != email {
		t.Fatalf("email id = %d, want %d", id, email)
	}
	if id, _ := boxIDAt(stored, "move"); id != move || stored.Items[0].ID != move {
		t.Fatalf("move id = %d, want %d first: %#v", id, move, stored)
	}
	checked, err := LoadDayChecks(db, "2026-10-20")
	if err != nil || !checked[email] || !checked[move] || len(checked) != 2 {
		t.Fatalf("checks = %#v err=%v", checked, err)
	}
}

func TestLastModeDefaultsAndRoundTrips(t *testing.T) {
	db := testDB(t)
	if _, err := Open(db); err != nil {
		t.Fatal(err)
	}
	mode, err := LastMode(db)
	if err != nil || mode != model.ModeDo {
		t.Fatalf("default mode=%q err=%v", mode, err)
	}
	if err := SaveLastMode(db, model.ModeBox); err != nil {
		t.Fatal(err)
	}
	mode, err = LastMode(db)
	if err != nil || mode != model.ModeBox {
		t.Fatalf("saved mode=%q err=%v", mode, err)
	}
	if err := SaveLastMode(db, "think"); err == nil {
		t.Fatal("unknown mode was stored")
	}
	if err := execSQL(db, "UPDATE install SET last_mode = 'nope' WHERE id = 1;\n"); err == nil {
		t.Fatal("the install table took an unknown mode")
	}
	mode, err = LastMode(db)
	if err != nil || mode != model.ModeBox {
		t.Fatalf("rejected save changed mode to %q err=%v", mode, err)
	}
}
