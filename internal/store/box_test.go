package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ryangerardwilson/crusade/internal/model"
)

func TestImportBoxesOnceThenIgnoresSourceEdits(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SQLX_ROOT", root)
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

	db := DatabasePath()
	if _, err := Open(db); err != nil {
		t.Fatal(err)
	}
	outline, err := LoadOutline(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := outline.ItemByID("work/email"); !ok {
		t.Fatal("missing imported box")
	}
	checked, err := LoadDayChecks(db, "2026-10-20")
	if err != nil || !checked["work/email"] {
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
	if _, ok := outline.ItemByID("move"); !ok {
		t.Fatal("outline was imported again")
	}
	if _, ok := outline.ItemByID("other"); ok {
		t.Fatal("later source edit became the outline")
	}
}

func TestOpenAdoptsXecuteDatabase(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SQLX_ROOT", root)
	t.Setenv("BOXES_CONFIG", filepath.Join(root, "missing-outline.txt"))
	legacy := filepath.Join(root, "data", legacyDatabaseName+".sqlite")
	event, err := model.NewDoEvent("2026-04-01 23:59:59", "kept task")
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(legacy, []Record{{ID: 7, Event: event}}); err != nil {
		t.Fatal(err)
	}
	db := DatabasePath()
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
	if len(records) != 1 || records[0].ID != 7 || records[0].Event.Y != "kept task" {
		t.Fatalf("records = %#v", records)
	}
}

func TestLoadActiveDaysKeepsDaysWithACheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crusade.sqlite")
	t.Setenv("BOXES_CONFIG", "")
	if err := SaveDayChecks(path, "2026-10-18", map[string]bool{"move": true}); err != nil {
		t.Fatal(err)
	}
	if err := SaveDayChecks(path, "2026-10-19", map[string]bool{"move": true, "work": true}); err != nil {
		t.Fatal(err)
	}
	if err := SaveDayChecks(path, "2026-10-20", map[string]bool{}); err != nil {
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

func TestLastModeDefaultsAndRoundTrips(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SQLX_ROOT", root)
	t.Setenv("BOXES_CONFIG", filepath.Join(root, "missing-outline.txt"))
	db := DatabasePath()
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
	mode, err = LastMode(db)
	if err != nil || mode != model.ModeBox {
		t.Fatalf("rejected save changed mode to %q err=%v", mode, err)
	}
	if err := setMeta(db, lastModeKey, "nope"); err != nil {
		t.Fatal(err)
	}
	mode, err = LastMode(db)
	if err != nil || mode != model.ModeDo {
		t.Fatalf("invalid mode=%q err=%v", mode, err)
	}
}
