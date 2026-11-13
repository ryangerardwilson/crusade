package store

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ryangerardwilson/crusade/internal/model"
)

func TestBlocksRoundTripApartFromDoTasks(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SQLX_ROOT", root)
	t.Setenv("BOXES_CONFIG", filepath.Join(t.TempDir(), "missing.txt"))
	db := DatabasePath()

	do, err := model.NewDoEvent("2026-11-02 23:59:59", "mail the letter")
	if err != nil {
		t.Fatal(err)
	}
	if err := Add(db, do); err != nil {
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
	tasks, err := Load(db)
	if anyOf(err != nil, len(tasks) != 1, tasks[0].Event.Y != "mail the letter", !tasks[0].Event.IsDo()) {

		t.Fatalf("tasks=%+v err=%v", tasks, err)
	}

	overlap, err := model.NewBlock("other", "2026-11-02 09:00:00", "2026-11-02 10:00:00")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddBlock(db, overlap); anyOf(err == nil, !strings.Contains(err.Error(), "already blocked")) {

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
	renamed, err := model.NewBlock("focus", "2026-11-02 09:00:00", "2026-11-02 10:00:00")
	if err != nil {
		t.Fatal(err)
	}
	if err := UpdateBlock(db, id, renamed); err != nil {
		t.Fatal(err)
	}
	if err := DeleteBlock(db, quoteID); err != nil {
		t.Fatal(err)
	}
	loaded, err = LoadBlocks(db)
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
	if _, err := AddBlock(db, half); anyOf(err == nil, !strings.Contains(err.Error(), "on the hour")) {

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

func TestMeetingURLMigratesAndRoundTrips(t *testing.T) {
	old := `CREATE TABLE block (
  id INTEGER PRIMARY KEY,
  title TEXT NOT NULL,
  start_at TEXT NOT NULL,
  end_at TEXT NOT NULL
);
INSERT INTO block(id, title, start_at, end_at) VALUES (1, 'deep work', '2026-11-02 09:00:00', '2026-11-02 10:00:00');
CREATE TABLE repeat (
  id INTEGER PRIMARY KEY,
  title TEXT NOT NULL,
  start_at TEXT NOT NULL,
  end_at TEXT NOT NULL,
  freq TEXT NOT NULL,
  until_on TEXT
);
INSERT INTO repeat(id, title, start_at, end_at, freq) VALUES (1, 'standup', '2026-11-02 11:00:00', '2026-11-02 11:15:00', 'weekly');
CREATE TABLE repeat_change (
  id INTEGER PRIMARY KEY,
  repeat_id INTEGER NOT NULL,
  day TEXT NOT NULL,
  skipped INTEGER NOT NULL,
  title TEXT,
  start_at TEXT,
  end_at TEXT
);
INSERT INTO repeat_change(id, repeat_id, day, skipped, title, start_at, end_at) VALUES (1, 1, '2026-11-09', 0, 'interview', '2026-11-09 11:00:00', '2026-11-09 11:15:00');
`
	path := seedSQLite(t, old)
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
	changes, err := LoadRepeatChanges(path)
	if anyOf(err != nil, len(changes) != 1, changes[0].HasURL, changes[0].Title != "interview") {

		t.Fatalf("change %#v %v", changes, err)
	}
	blocks[0].Block.URL = " https://meet.example/o'clock "
	if err := UpdateBlock(path, blocks[0].ID, blocks[0].Block); err != nil {
		t.Fatal(err)
	}
	blocks, err = LoadBlocks(path)
	if anyOf(err != nil, len(blocks) != 1, blocks[0].Block.URL != "https://meet.example/o'clock") {

		t.Fatalf("block url %#v %v", blocks, err)
	}
	repeats[0].Repeat.URL = "https://meet.example/stand"
	if err := UpdateRepeat(path, repeats[0].ID, repeats[0].Repeat); anyOf(err == nil, !strings.Contains(err.Error(), "on the hour")) {

		t.Fatalf("legacy minute repeat = %v", err)
	}
	repeats[0].Repeat.End = repeats[0].Repeat.Start.Add(time.Hour)
	if err := UpdateRepeat(path, repeats[0].ID, repeats[0].Repeat); err != nil {
		t.Fatal(err)
	}
	repeats, err = LoadRepeats(path)
	if anyOf(err != nil, len(repeats) != 1, repeats[0].Repeat.URL != "https://meet.example/stand") {

		t.Fatalf("repeat url %#v %v", repeats, err)
	}
	changes, err = LoadRepeatChanges(path)
	if anyOf(err != nil, len(changes) != 1, changes[0].HasURL) {

		t.Fatalf("inherited change %#v %v", changes, err)
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
