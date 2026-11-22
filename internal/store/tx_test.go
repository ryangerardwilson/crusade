package store

import (
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

func TestApplyURLWithDayOverrideIsOneWrite(t *testing.T) {
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
	start := time.Date(2026, 11, 9, 9, 0, 0, 0, time.Local)
	if err := OverrideRepeatDayAndURL(db, id, nov9, "review", start, start.Add(time.Hour), "https://meet.example/all"); err != nil {
		t.Fatal(err)
	}
	repeats, err := LoadRepeats(db)
	if err != nil {
		t.Fatal(err)
	}
	if found, _ := findRepeat(repeats, id); found.Repeat.URL != "https://meet.example/all" {
		t.Fatalf("series url = %q", found.Repeat.URL)
	}
	changes, err := LoadRepeatChanges(db)
	if err != nil {
		t.Fatal(err)
	}
	if change, ok := changeFor(changes, id, nov9); !ok || change.Title != "review" || change.URL != "https://meet.example/all" {
		t.Fatalf("override = %#v", changes)
	}
}

func TestMetaSQLUpdatesOneRow(t *testing.T) {
	db := testDB(t)
	for _, mode := range []string{model.ModeBox, model.ModeCal} {
		if err := SaveLastMode(db, mode); err != nil {
			t.Fatal(err)
		}
	}
	var rows []struct {
		N int `json:"n"`
	}
	if err := queryJSON(db, "SELECT COUNT(*) AS n FROM meta WHERE key = "+sqlString(lastModeKey), &rows); err != nil || len(rows) != 1 || rows[0].N != 1 {
		t.Fatalf("meta rows = %#v %v", rows, err)
	}
	if mode, err := LastMode(db); err != nil || mode != model.ModeCal {
		t.Fatalf("last mode = %q %v", mode, err)
	}
}
