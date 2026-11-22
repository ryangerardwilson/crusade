package store

import (
	"strings"
	"testing"
	"time"

	"github.com/ryangerardwilson/crusade/internal/model"
)

func TestBlocksRoundTripApartFromDoItems(t *testing.T) {
	db := testDB(t)
	if _, err := AddDoItem(db, 0, "mail the letter"); err != nil {
		t.Fatal(err)
	}
	block, err := model.NewBlock("deep work", "2026-11-02 09:00:00", "2026-11-02 10:00:00")
	if err != nil {
		t.Fatal(err)
	}
	id, err := AddBlock(db, block)
	if anyOf(err != nil, id <= 0) {
		t.Fatalf("id=%d err=%v", id, err)
	}
	loaded, err := LoadBlocks(db)
	if anyOf(err != nil, len(loaded) != 1, loaded[0].ID != id, loaded[0].Block.Title != "deep work", loaded[0].Block.Start.Format("15:04") != "09:00", loaded[0].Block.End.Format("15:04") != "10:00") {
		t.Fatalf("loaded=%+v err=%v", loaded, err)
	}
	list, err := LoadDoList(db)
	if anyOf(err != nil, len(list) != 1, list[0].Body != "mail the letter") {
		t.Fatalf("do list=%+v err=%v", list, err)
	}
	if records, err := Load(db); anyOf(err != nil, len(records) != 0) {
		t.Fatalf("align records=%+v err=%v", records, err)
	}
	assertBlockConflictsAndRename(t, db, id)
	assertBlockClockRules(t, db)
}

func assertBlockConflictsAndRename(t *testing.T, db string, id int) {
	t.Helper()
	overlap, err := model.NewBlock("other", "2026-11-02 09:00:00", "2026-11-02 10:00:00")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddBlock(db, overlap); anyOf(err == nil, err != nil && !strings.Contains(err.Error(), "already blocked")) {
		t.Fatalf("overlap err=%v", err)
	}
	touch, err := model.NewBlock("next", "2026-11-02 10:00:00", "2026-11-02 11:00:00")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddBlock(db, touch); err != nil {
		t.Fatal(err)
	}
	quoted, err := model.NewBlock("o'clock", "2026-11-03 09:00:00", "2026-11-03 10:00:00")
	if err != nil {
		t.Fatal(err)
	}
	quoteID, err := AddBlock(db, quoted)
	if err != nil {
		t.Fatal(err)
	}
	form := model.CalForm{Title: "focus", StartHour: 9, EndHour: 10}
	saved, err := SaveForm(db, FormTarget{BlockID: id}, form)
	if anyOf(err != nil, saved.BlockID != id, saved.Change != model.ChangeRenamed) {
		t.Fatalf("rename = %#v %v", saved, err)
	}
	if err := DeleteBlock(db, quoteID); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadBlocks(db)
	if err != nil {
		t.Fatal(err)
	}
	titles := make([]string, 0, len(loaded))
	for _, item := range loaded {
		titles = append(titles, item.Block.Title)
	}
	if strings.Join(titles, ",") != "focus,next" {
		t.Fatalf("titles=%v", titles)
	}
}

func assertBlockClockRules(t *testing.T, db string) {
	t.Helper()
	if _, err := model.NewBlock("night", "2026-11-02 23:00:00", "2026-11-03 00:30:00"); err == nil {
		t.Fatal("overnight block was accepted")
	}
	if _, err := AddBlock(db, model.Block{}); err == nil {
		t.Fatal("empty block was stored")
	}
	half, err := model.NewBlock("half", "2026-11-04 12:00:00", "2026-11-04 12:30:00")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddBlock(db, half); anyOf(err == nil, err != nil && !strings.Contains(err.Error(), "on the hour")) {
		t.Fatalf("half hour = %v", err)
	}
	last, err := model.NewBlock("late", "2026-11-04 23:00:00", "2026-11-05 00:00:00")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddBlock(db, last); err != nil {
		t.Fatal(err)
	}
}

const meetingURLLegacy = `CREATE TABLE block (
  id INTEGER PRIMARY KEY,
  title TEXT NOT NULL,
  start_at TEXT NOT NULL,
  end_at TEXT NOT NULL,
  url TEXT
);
INSERT INTO block(id, title, start_at, end_at) VALUES (1, 'deep work', '2026-11-02 09:00:00', '2026-11-02 10:00:00');
CREATE TABLE repeat (
  id INTEGER PRIMARY KEY,
  title TEXT NOT NULL,
  start_at TEXT NOT NULL,
  end_at TEXT NOT NULL,
  freq TEXT NOT NULL,
  until_on TEXT,
  url TEXT
);
INSERT INTO repeat(id, title, start_at, end_at, freq) VALUES (1, 'standup', '2026-11-02 11:00:00', '2026-11-02 11:15:00', 'weekly');
CREATE TABLE repeat_change (
  id INTEGER PRIMARY KEY,
  repeat_id INTEGER NOT NULL,
  day TEXT NOT NULL,
  skipped INTEGER NOT NULL,
  title TEXT,
  start_at TEXT,
  end_at TEXT,
  url TEXT
);
INSERT INTO repeat_change(id, repeat_id, day, skipped, title, start_at, end_at) VALUES (1, 1, '2026-11-09', 0, 'interview', '2026-11-09 11:00:00', '2026-11-09 11:15:00');
`

func TestMeetingURLMigratesAndRoundTrips(t *testing.T) {
	path := seedSQLite(t, meetingURLLegacy)
	if _, err := Open(path); err != nil {
		t.Fatal(err)
	}
	blocks, err := LoadBlocks(path)
	if anyOf(err != nil, len(blocks) != 1, blocks[0].Block.URL != "", blocks[0].Block.Title != "deep work") {
		t.Fatalf("block %#v %v", blocks, err)
	}
	repeats, err := LoadRepeats(path)
	if anyOf(err != nil, len(repeats) != 1, repeats[0].Repeat.URL != "", repeats[0].Repeat.Title != "standup") {
		t.Fatalf("repeat %#v %v", repeats, err)
	}
	nov9 := time.Date(2026, 11, 9, 0, 0, 0, 0, time.Local)
	override, ok := repeats[0].OverrideOn(nov9)
	if anyOf(!ok, override.URL != "", override.Title != "interview", override.HasClock()) {
		t.Fatalf("override %#v", repeats[0].Overrides)
	}
	blockForm := model.CalForm{Title: "deep work", StartHour: 9, EndHour: 10, URL: " https://meet.example/o'clock "}
	if _, err := SaveForm(path, FormTarget{BlockID: blocks[0].ID}, blockForm); err != nil {
		t.Fatal(err)
	}
	blocks, err = LoadBlocks(path)
	if anyOf(err != nil, len(blocks) != 1, blocks[0].Block.URL != "https://meet.example/o'clock") {
		t.Fatalf("block url %#v %v", blocks, err)
	}
	assertMinuteSeriesURL(t, path, repeats[0].ID, nov9)
}

func assertMinuteSeriesURL(t *testing.T, path string, id int, nov9 time.Time) {
	t.Helper()
	nov2 := time.Date(2026, 11, 2, 0, 0, 0, 0, time.Local)
	target := FormTarget{RepeatID: id, Day: nov2}
	form := model.CalForm{Title: "standup", StartHour: 11, EndHour: 11, EndMinute: 15, Freq: "weekly", URL: "https://meet.example/stand", Scope: model.ScopeAll}
	if _, err := SaveForm(path, target, form); err != nil {
		t.Fatalf("url on a minute series = %v", err)
	}
	repeats, err := LoadRepeats(path)
	if anyOf(err != nil, len(repeats) != 1, repeats[0].Repeat.URL != "https://meet.example/stand", repeats[0].Repeat.End.Minute() != 15) {
		t.Fatalf("kept minute clock %#v %v", repeats, err)
	}
	if occurrence, ok := repeats[0].On(nov9); anyOf(!ok, occurrence.Title != "interview", occurrence.URL != "https://meet.example/stand") {
		t.Fatalf("overridden day %#v", occurrence)
	}
	moved := form
	moved.EndMinute = 20
	if _, err := SaveForm(path, target, moved); anyOf(err == nil, err != nil && !strings.Contains(err.Error(), "on the hour")) {
		t.Fatalf("moved minute repeat = %v", err)
	}
	hour := form
	hour.EndHour, hour.EndMinute = 12, 0
	if _, err := SaveForm(path, target, hour); err != nil {
		t.Fatal(err)
	}
	repeats, err = LoadRepeats(path)
	if anyOf(err != nil, len(repeats) != 1, repeats[0].Repeat.URL != "https://meet.example/stand", repeats[0].Repeat.End.Hour() != 12, len(repeats[0].Overrides) != 0) {
		t.Fatalf("rescheduled series %#v %v", repeats, err)
	}
}

func anyOf(flags ...bool) bool {
	for _, flag := range flags {
		if flag {
			return true
		}
	}
	return false
}

func allOf(flags ...bool) bool {
	for _, flag := range flags {
		if !flag {
			return false
		}
	}
	return true
}
