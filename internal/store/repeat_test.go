package store

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ryangerardwilson/crusade/internal/model"
)

func TestRepeatSeriesSkipOverrideSplitAndConflict(t *testing.T) {
	db := filepath.Join(t.TempDir(), "crusade.sqlite")
	repeat, err := model.NewRepeat("standup", "2026-11-02 09:00:00", "2026-11-02 10:00:00", "Weekly")
	if err != nil {
		t.Fatal(err)
	}
	if repeat.Freq != model.FreqWeekly {
		t.Fatalf("freq = %s", repeat.Freq)
	}
	monday := time.Date(2026, 11, 2, 12, 0, 0, 0, time.Local)
	if monday.Weekday() != time.Monday {
		t.Fatalf("2026-11-02 is %s", monday.Weekday())
	}
	if anyOf(!repeat.OccursOn(monday), repeat.OccursOn(monday.AddDate(0, 0, 1)), repeat.OccursOn(monday.AddDate(0, 0, -7))) {

		t.Fatal("weekly occurrence")
	}
	if _, err := model.NewRepeat("off", "2026-11-07 09:00:00", "2026-11-07 10:00:00", "weekdays"); anyOf(err == nil, !strings.Contains(err.Error(), "does not fall")) {

		t.Fatalf("saturday weekdays = %v", err)
	}
	daily, err := model.NewRepeat("mail", "2026-11-02 12:00:00", "2026-11-02 12:30:00", "daily")
	if err != nil {
		t.Fatal(err)
	}
	daily.Until = time.Date(2026, 11, 4, 0, 0, 0, 0, time.Local)
	if anyOf(!daily.OccursOn(time.Date(2026, 11, 4, 0, 0, 0, 0, time.Local)), daily.OccursOn(time.Date(2026, 11, 5, 0, 0, 0, 0, time.Local))) {

		t.Fatal("until is inclusive")
	}
	weekdays, err := model.NewRepeat("desk", "2026-11-02 15:00:00", "2026-11-02 15:30:00", "weekdays")
	if anyOf(err != nil, !weekdays.OccursOn(monday), weekdays.OccursOn(time.Date(2026, 11, 7, 0, 0, 0, 0, time.Local))) {

		t.Fatalf("weekdays = %v", err)
	}
	chosen, err := model.NewRepeat("studio", "2026-11-02 09:00:00", "2026-11-02 10:00:00", "wed,mon")
	if anyOf(err != nil, chosen.Freq != "mon,wed", model.FreqLabel(chosen.Freq) != "Mon Wed") {

		t.Fatalf("chosen days = %#v %v", chosen, err)
	}
	tuesday := time.Date(2026, 11, 3, 0, 0, 0, 0, time.Local)
	wednesday := time.Date(2026, 11, 4, 0, 0, 0, 0, time.Local)
	if anyOf(!chosen.OccursOn(monday), chosen.OccursOn(tuesday), !chosen.OccursOn(wednesday)) {

		t.Fatal("chosen occurrence")
	}
	if _, err := model.NewRepeat("studio", "2026-11-03 09:00:00", "2026-11-03 10:00:00", "mon,wed"); anyOf(err == nil, !strings.Contains(err.Error(), "does not fall")) {

		t.Fatalf("tuesday days = %v", err)
	}
	if _, err := model.NewRepeat("studio", "2026-11-02 09:00:00", "2026-11-02 10:00:00", "mon,mon"); anyOf(err == nil, !strings.Contains(err.Error(), "chosen weekdays")) {

		t.Fatalf("duplicate days = %v", err)
	}
	if _, err := model.NewRepeat("studio", "2026-11-02 09:00:00", "2026-11-02 10:00:00", "days"); anyOf(err == nil, !strings.Contains(err.Error(), "chosen weekdays")) {

		t.Fatalf("days word = %v", err)
	}

	id, nov9, nov16 := assertRepeatStored(t, db, repeat)
	old := assertRepeatSplit(t, db, id, nov9, nov16)
	assertRepeatRenamed(t, db, id, old)
}

func assertRepeatStored(t *testing.T, db string, repeat model.Repeat) (int, time.Time, time.Time) {
	repeat.URL = "https://meet.example/stand"
	id, err := AddRepeat(db, repeat)
	if anyOf(err != nil, id == 0) {

		t.Fatalf("add = %d %v", id, err)
	}
	stored, err := LoadRepeats(db)
	if anyOf(err != nil, len(stored) != 1, stored[0].Repeat.URL != "https://meet.example/stand") {

		t.Fatalf("stored url %#v %v", stored, err)
	}
	blocked, err := model.NewBlock("deep work", "2026-11-02 09:00:00", "2026-11-02 10:00:00")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddBlock(db, blocked); anyOf(err == nil, !strings.Contains(err.Error(), "already blocked")) {

		t.Fatalf("series did not block the one-off: %v", err)
	}
	later, err := model.NewBlock("later", "2026-11-09 09:00:00", "2026-11-09 10:00:00")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddBlock(db, later); anyOf(err == nil, !strings.Contains(err.Error(), "already blocked")) {

		t.Fatalf("later monday = %v", err)
	}
	touch, err := model.NewBlock("after", "2026-11-02 10:00:00", "2026-11-02 11:00:00")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddBlock(db, touch); err != nil {
		t.Fatal(err)
	}
	again, err := model.NewRepeat("standup", "2026-11-02 09:00:00", "2026-11-02 10:00:00", "weekly")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddRepeat(db, again); anyOf(err == nil, !strings.Contains(err.Error(), "already blocked")) {

		t.Fatalf("second series = %v", err)
	}

	nov9 := time.Date(2026, 11, 9, 9, 0, 0, 0, time.Local)
	if err := OverrideRepeatDay(db, id, nov9, "interview", nov9, nov9.Add(time.Hour), "https://meet.example/once"); err != nil {
		t.Fatal(err)
	}
	nov16 := time.Date(2026, 11, 16, 0, 0, 0, 0, time.Local)
	if err := SkipRepeatDay(db, id, nov16); err != nil {
		t.Fatal(err)
	}
	freed, err := model.NewBlock("freed", "2026-11-16 09:00:00", "2026-11-16 10:00:00")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddBlock(db, freed); err != nil {
		t.Fatal(err)
	}
	changes, err := LoadRepeatChanges(db)
	if anyOf(err != nil, len(changes) != 2) {

		t.Fatalf("changes = %#v %v", changes, err)
	}
	for _, change := range changes {
		if allOf(change.Title == "interview", (!change.HasURL || change.URL != "https://meet.example/once")) {

			t.Fatalf("override url %#v", change)
		}
	}
	return id, nov9, nov16
}

func assertRepeatSplit(t *testing.T, db string, id int, nov9, nov16 time.Time) RepeatRecord {
	next, err := model.NewRepeat("standup", "2026-11-23 10:00:00", "2026-11-23 11:00:00", "weekly")
	if err != nil {
		t.Fatal(err)
	}
	nov23 := time.Date(2026, 11, 23, 0, 0, 0, 0, time.Local)
	newID, err := ContinueRepeat(db, id, nov23, next, true)
	if anyOf(err != nil, newID == id) {

		t.Fatalf("split = %d %v", newID, err)
	}
	repeats, err := LoadRepeats(db)
	if err != nil {
		t.Fatal(err)
	}
	old, ok := findRepeat(repeats, id)
	if anyOf(!ok, !sameDateLocal(old.Repeat.Until, time.Date(2026, 11, 22, 0, 0, 0, 0, time.Local)), old.Repeat.Title != "standup", old.Repeat.URL != "https://meet.example/stand") {

		t.Fatalf("old series = %#v", old)
	}
	if anyOf(!old.Repeat.OccursOn(nov9), !old.Repeat.OccursOn(nov16), old.Repeat.OccursOn(nov23)) {

		t.Fatal("old series window")
	}
	created, ok := findRepeat(repeats, newID)
	if anyOf(!ok, !created.Repeat.OccursOn(nov23), created.Repeat.Start.Hour() != 10) {

		t.Fatalf("new series = %#v", created)
	}
	changes, err := LoadRepeatChanges(db)
	if err != nil {
		t.Fatal(err)
	}
	keptInterview := false
	for _, change := range changes {
		if allOf(change.RepeatID == id, sameDateLocal(change.Day, nov9), change.Title == "interview", !change.Skipped) {

			keptInterview = true
		}
		if allOf(change.RepeatID == id, !change.Day.Before(nov23)) {

			t.Fatalf("change survived the split: %#v", change)
		}
	}
	if !keptInterview {
		t.Fatalf("interview was dropped: %#v", changes)
	}
	if err := DeleteRepeat(db, newID); err != nil {
		t.Fatal(err)
	}
	repeats, err = LoadRepeats(db)
	if anyOf(err != nil, len(repeats) != 1, repeats[0].ID != id) {

		t.Fatalf("after deleting the later series: %#v %v", repeats, err)
	}
	return old
}

func assertRepeatRenamed(t *testing.T, db string, id int, old RepeatRecord) {
	renamed, err := model.NewRepeat("forum", old.Repeat.Start.Format(model.DateTimeFormat), old.Repeat.End.Format(model.DateTimeFormat), old.Repeat.Freq)
	if err != nil {
		t.Fatal(err)
	}
	renamed.Until = old.Repeat.Until
	if err := UpdateRepeat(db, id, renamed); err != nil {
		t.Fatal(err)
	}
	if err := UpdateRepeat(db, id, renamed); err != nil {
		t.Fatal(err)
	}
	if err := RenameRepeatOverrides(db, id, "forum"); err != nil {
		t.Fatal(err)
	}
	changes, err := LoadRepeatChanges(db)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range changes {
		if allOf(change.RepeatID == id, !change.Skipped, change.Title != "forum") {

			t.Fatalf("override title = %#v", change)
		}
	}

	block, err := model.NewBlock("once", "2026-11-03 11:00:00", "2026-11-03 12:00:00")
	if err != nil {
		t.Fatal(err)
	}
	blockID, err := AddBlock(db, block)
	if err != nil {
		t.Fatal(err)
	}
	made, err := model.NewRepeat("once", "2026-11-03 11:00:00", "2026-11-03 12:00:00", "weekly")
	if err != nil {
		t.Fatal(err)
	}
	madeID, err := MakeRepeatFromBlock(db, blockID, made)
	if anyOf(err != nil, madeID == 0) {

		t.Fatalf("make = %d %v", madeID, err)
	}
	blocks, err := LoadBlocks(db)
	if err != nil {
		t.Fatal(err)
	}
	for _, loaded := range blocks {
		if loaded.ID == blockID {
			t.Fatal("one-off remained after it became a series")
		}
	}
	if err := DeleteRepeat(db, id); err != nil {
		t.Fatal(err)
	}
	changes, err = LoadRepeatChanges(db)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range changes {
		if change.RepeatID == id {
			t.Fatalf("change remained: %#v", change)
		}
	}
}

func TestMakeRepeatFromBlockNamesTheLaterConflict(t *testing.T) {
	db := filepath.Join(t.TempDir(), "crusade.sqlite")
	block, err := model.NewBlock("meetup", "2026-11-03 19:00:00", "2026-11-03 21:00:00")
	if err != nil {
		t.Fatal(err)
	}
	blockID, err := AddBlock(db, block)
	if err != nil {
		t.Fatal(err)
	}
	later, err := model.NewRepeat("dev meeting", "2026-11-10 19:00:00", "2026-11-10 20:00:00", "weekly")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddRepeat(db, later); err != nil {
		t.Fatal(err)
	}
	made, err := model.NewRepeat("meetup", "2026-11-03 19:00:00", "2026-11-03 21:00:00", "weekly")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := MakeRepeatFromBlock(db, blockID, made); anyOf(err == nil, !strings.Contains(err.Error(), "already blocked by dev meeting on 10 Nov 2026")) {

		t.Fatalf("conflict = %v", err)
	}
}
