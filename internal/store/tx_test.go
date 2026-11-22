package store

import (
	"github.com/ryangerardwilson/crusade/internal/sqlite"

	"testing"
	"time"

	"github.com/ryangerardwilson/crusade/internal/model"
)

func TestFailedStatementRollsBackTransaction(t *testing.T) {
	db := testDB(t)
	repeat, err := model.NewRepeat("standup", "2026-11-02 09:00:00", "2026-11-02 10:00:00", "weekly")
	if err != nil {
		t.Fatal(err)
	}
	id, err := AddRepeat(db, repeat)
	if err != nil {
		t.Fatal(err)
	}
	if err := execSQL(db, inTx(
		"UPDATE repeat SET title = 'changed';\n",
		"INSERT INTO missing_table VALUES (1);\n",
	)); err == nil {
		t.Fatal("a failed statement returned no error")
	}
	repeats, err := LoadRepeats(db)
	if err != nil {
		t.Fatal(err)
	}
	if found, ok := findRepeat(repeats, id); !ok || found.Repeat.Title != "standup" {
		t.Fatalf("statement before the failure committed: %#v", repeats)
	}
}

func TestMakeRepeatFromMissingBlockWritesNothing(t *testing.T) {
	db := testDB(t)
	repeat, err := model.NewRepeat("standup", "2026-11-02 09:00:00", "2026-11-02 10:00:00", "weekly")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := MakeRepeatFromBlock(db, 42, repeat); err == nil {
		t.Fatal("missing block made a repeat")
	}
	repeats, err := LoadRepeats(db)
	if err != nil || len(repeats) != 0 {
		t.Fatalf("repeats = %#v %v", repeats, err)
	}
}

func TestDayRenameWithURLMakesTheURLTheSeriesLink(t *testing.T) {
	db := testDB(t)
	repeat, err := model.NewRepeat("standup", "2026-11-02 09:00:00", "2026-11-02 10:00:00", "weekly")
	if err != nil {
		t.Fatal(err)
	}
	id, err := AddRepeat(db, repeat)
	if err != nil {
		t.Fatal(err)
	}
	nov9 := time.Date(2026, 11, 9, 0, 0, 0, 0, time.Local)
	form := model.CalForm{Title: "review", StartHour: 9, EndHour: 10, Freq: "weekly", URL: "https://meet.example/all", Scope: model.ScopeThis}
	if _, err := SaveForm(db, FormTarget{RepeatID: id, Day: nov9}, form); err != nil {
		t.Fatal(err)
	}
	repeats, err := LoadRepeats(db)
	if err != nil {
		t.Fatal(err)
	}
	found, _ := findRepeat(repeats, id)
	if found.Repeat.URL != "https://meet.example/all" || found.Repeat.Title != "standup" {
		t.Fatalf("series = %#v", found.Repeat)
	}
	override, ok := found.OverrideOn(nov9)
	if !ok || override.Title != "review" || override.URL != "" || override.HasClock() {
		t.Fatalf("override = %#v", found.Overrides)
	}
	occurrence, ok := found.On(nov9)
	if !ok || occurrence.Title != "review" || occurrence.URL != "https://meet.example/all" {
		t.Fatalf("occurrence = %#v", occurrence)
	}
}

func TestInstallHasOneRow(t *testing.T) {
	db := testDB(t)
	for _, mode := range []string{model.ModeBox, model.ModeCal} {
		if err := SaveLastMode(db, mode); err != nil {
			t.Fatal(err)
		}
	}
	var rows []struct {
		N int `json:"n"`
	}
	if err := sqlite.Query(db, "SELECT COUNT(*) AS n FROM install", &rows); err != nil || len(rows) != 1 || rows[0].N != 1 {
		t.Fatalf("install rows = %#v %v", rows, err)
	}
	if err := execSQL(db, "INSERT INTO install(id) VALUES (2);\n"); err == nil {
		t.Fatal("a second install row was stored")
	}
	if mode, err := LastMode(db); err != nil || mode != model.ModeCal {
		t.Fatalf("last mode = %q %v", mode, err)
	}
}
