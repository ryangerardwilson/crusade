package store

import (
	"github.com/ryangerardwilson/crusade/internal/sqlite"

	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ryangerardwilson/crusade/internal/model"
)

func findRepeat(repeats []RepeatRecord, id int) (RepeatRecord, bool) {
	for _, repeat := range repeats {
		if repeat.ID == id {
			return repeat, true
		}
	}
	return RepeatRecord{}, false
}

func localDay(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.Local)
}

func TestRepeatFrequencies(t *testing.T) {
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
	if _, err := model.NewRepeat("off", "2026-11-07 09:00:00", "2026-11-07 10:00:00", "weekdays"); anyOf(err == nil, err != nil && !strings.Contains(err.Error(), "does not fall")) {
		t.Fatalf("saturday weekdays = %v", err)
	}
	daily, err := model.NewRepeat("mail", "2026-11-02 12:00:00", "2026-11-02 12:30:00", "daily")
	if err != nil {
		t.Fatal(err)
	}
	daily.Until = localDay(2026, 11, 4)
	if anyOf(!daily.OccursOn(localDay(2026, 11, 4)), daily.OccursOn(localDay(2026, 11, 5))) {
		t.Fatal("until is inclusive")
	}
	weekdays, err := model.NewRepeat("desk", "2026-11-02 15:00:00", "2026-11-02 15:30:00", "weekdays")
	if anyOf(err != nil, !weekdays.OccursOn(monday), weekdays.OccursOn(localDay(2026, 11, 7))) {
		t.Fatalf("weekdays = %v", err)
	}
}

func TestRepeatChosenWeekdays(t *testing.T) {
	monday := localDay(2026, 11, 2)
	chosen, err := model.NewRepeat("studio", "2026-11-02 09:00:00", "2026-11-02 10:00:00", "wed,mon")
	if anyOf(err != nil, chosen.Freq != "mon,wed", model.FreqLabel(chosen.Freq) != "Mon Wed") {
		t.Fatalf("chosen days = %#v %v", chosen, err)
	}
	if anyOf(!chosen.OccursOn(monday), chosen.OccursOn(localDay(2026, 11, 3)), !chosen.OccursOn(localDay(2026, 11, 4))) {
		t.Fatal("chosen occurrence")
	}
	bad := map[string]string{"2026-11-03 09:00:00|mon,wed": "does not fall", "2026-11-02 09:00:00|mon,mon": "chosen weekdays", "2026-11-02 09:00:00|days": "chosen weekdays"}
	for input, want := range bad {
		start, freq, _ := strings.Cut(input, "|")
		end := strings.Replace(start, "09:00", "10:00", 1)
		if _, err := model.NewRepeat("studio", start, end, freq); anyOf(err == nil, err != nil && !strings.Contains(err.Error(), want)) {
			t.Fatalf("%s = %v", input, err)
		}
	}
}

func TestRepeatSeriesSkipOverrideSplitAndConflict(t *testing.T) {
	db := testDB(t)
	repeat, err := model.NewRepeat("standup", "2026-11-02 09:00:00", "2026-11-02 10:00:00", "weekly")
	if err != nil {
		t.Fatal(err)
	}
	id := assertRepeatStored(t, db, repeat)
	assertRepeatDayEdits(t, db, id)
	old := assertRepeatSplit(t, db, id)
	assertRepeatRenamed(t, db, id, old)
}

func assertRepeatStored(t *testing.T, db string, repeat model.Repeat) int {
	t.Helper()
	repeat.URL = "https://meet.example/stand"
	id, err := AddRepeat(db, repeat)
	if anyOf(err != nil, id == 0) {
		t.Fatalf("add = %d %v", id, err)
	}
	stored, err := LoadRepeats(db)
	if anyOf(err != nil, len(stored) != 1, stored[0].Repeat.URL != "https://meet.example/stand") {
		t.Fatalf("stored url %#v %v", stored, err)
	}
	for _, start := range []string{"2026-11-02 09:00:00", "2026-11-09 09:00:00"} {
		end := strings.Replace(start, "09:00", "10:00", 1)
		blocked, err := model.NewBlock("deep work", start, end)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := AddBlock(db, blocked); anyOf(err == nil, err != nil && !strings.Contains(err.Error(), "already blocked")) {
			t.Fatalf("series did not block %s: %v", start, err)
		}
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
	if _, err := AddRepeat(db, again); anyOf(err == nil, err != nil && !strings.Contains(err.Error(), "already blocked")) {
		t.Fatalf("second series = %v", err)
	}
	return id
}

// assertRepeatDayEdits renames 9 Nov with a new link and skips 16 Nov. The
// link becomes the series link; the title stays on the day.
func assertRepeatDayEdits(t *testing.T, db string, id int) {
	t.Helper()
	nov9, nov16 := localDay(2026, 11, 9), localDay(2026, 11, 16)
	form := model.CalForm{Title: "interview", StartHour: 9, EndHour: 10, Freq: "weekly", URL: "https://meet.example/once", Scope: model.ScopeThis}
	if _, err := SaveForm(db, FormTarget{RepeatID: id, Day: nov9}, form); err != nil {
		t.Fatal(err)
	}
	if err := DeleteOccurrence(db, id, nov16, model.ScopeThis); err != nil {
		t.Fatal(err)
	}
	freed, err := model.NewBlock("freed", "2026-11-16 09:00:00", "2026-11-16 10:00:00")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddBlock(db, freed); err != nil {
		t.Fatal(err)
	}
	repeats, err := LoadRepeats(db)
	if err != nil {
		t.Fatal(err)
	}
	series, _ := findRepeat(repeats, id)
	override, ok := series.OverrideOn(nov9)
	if anyOf(!ok, override.Title != "interview", override.URL != "", len(series.Overrides) != 1, !series.Skipped(nov16), len(series.Skips) != 1) {
		t.Fatalf("days = %#v", series)
	}
	if series.Repeat.URL != "https://meet.example/once" {
		t.Fatalf("series url = %q", series.Repeat.URL)
	}
}

func assertRepeatSplit(t *testing.T, db string, id int) RepeatRecord {
	t.Helper()
	nov9, nov16, nov23 := localDay(2026, 11, 9), localDay(2026, 11, 16), localDay(2026, 11, 23)
	form := model.CalForm{Title: "standup", StartHour: 10, EndHour: 11, Freq: "weekly", URL: "https://meet.example/once", Scope: model.ScopeFollowing}
	saved, err := SaveForm(db, FormTarget{RepeatID: id, Day: nov23}, form)
	newID := saved.RepeatID
	if anyOf(err != nil, newID == id, newID == 0) {
		t.Fatalf("split = %#v %v", saved, err)
	}
	repeats, err := LoadRepeats(db)
	if err != nil {
		t.Fatal(err)
	}
	old, ok := findRepeat(repeats, id)
	if anyOf(!ok, !sameDateLocal(old.Repeat.Until, localDay(2026, 11, 22)), old.Repeat.Title != "standup", old.Repeat.URL != "https://meet.example/once") {
		t.Fatalf("old series = %#v", old)
	}
	if anyOf(!old.Repeat.OccursOn(nov9), !old.Repeat.OccursOn(nov16), old.Repeat.OccursOn(nov23)) {
		t.Fatal("old series window")
	}
	created, ok := findRepeat(repeats, newID)
	if anyOf(!ok, !created.Repeat.OccursOn(nov23), created.Repeat.Start.Hour() != 10, created.Repeat.URL != "https://meet.example/once") {
		t.Fatalf("new series = %#v", created)
	}
	if override, ok := old.OverrideOn(nov9); anyOf(!ok, override.Title != "interview") {
		t.Fatalf("interview was dropped: %#v", old.Overrides)
	}
	for _, override := range old.Overrides {
		if !override.Day.Before(nov23) {
			t.Fatalf("override survived the split: %#v", override)
		}
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
	t.Helper()
	form := model.CalForm{Title: "forum", StartHour: 9, EndHour: 10, Freq: old.Repeat.Freq, URL: old.Repeat.URL, Scope: model.ScopeAll}
	saved, err := SaveForm(db, FormTarget{RepeatID: id, Day: localDay(2026, 11, 2)}, form)
	if anyOf(err != nil, saved.Change != model.ChangeRenamed) {
		t.Fatalf("rename = %#v %v", saved, err)
	}
	repeats, err := LoadRepeats(db)
	if err != nil {
		t.Fatal(err)
	}
	renamed, _ := findRepeat(repeats, id)
	if anyOf(renamed.Repeat.Title != "forum", !sameDateLocal(renamed.Repeat.Until, old.Repeat.Until), len(renamed.Overrides) != 0) {
		t.Fatalf("renamed series = %#v", renamed)
	}
	if occurrence, ok := renamed.On(localDay(2026, 11, 9)); anyOf(!ok, occurrence.Title != "forum") {
		t.Fatalf("renamed day = %#v", occurrence)
	}
	assertMakeRepeatFromBlock(t, db)
	if err := DeleteRepeat(db, id); err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		N int `json:"n"`
	}
	sql := fmt.Sprintf("SELECT (SELECT COUNT(*) FROM repeat_skip WHERE repeat_id = %[1]d) + (SELECT COUNT(*) FROM repeat_override WHERE repeat_id = %[1]d) AS n", id)
	if err := sqlite.Query(db, sql, &rows); anyOf(err != nil, len(rows) != 1, len(rows) == 1 && rows[0].N != 0) {
		t.Fatalf("days of the deleted series remain: %#v %v", rows, err)
	}
}

func assertMakeRepeatFromBlock(t *testing.T, db string) {
	t.Helper()
	block, err := model.NewBlock("once", "2026-11-03 11:00:00", "2026-11-03 12:00:00")
	if err != nil {
		t.Fatal(err)
	}
	blockID, err := AddBlock(db, block)
	if err != nil {
		t.Fatal(err)
	}
	form := model.CalForm{Title: "once", StartHour: 11, EndHour: 12, Freq: "weekly"}
	saved, err := SaveForm(db, FormTarget{BlockID: blockID}, form)
	if anyOf(err != nil, saved.RepeatID == 0, saved.BlockID != 0) {
		t.Fatalf("make = %#v %v", saved, err)
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
}

func TestMakeRepeatFromBlockNamesTheLaterConflict(t *testing.T) {
	db := testDB(t)
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
	if _, err := MakeRepeatFromBlock(db, blockID, made); anyOf(err == nil, err != nil && !strings.Contains(err.Error(), "already blocked by dev meeting on 10 Nov 2026")) {
		t.Fatalf("conflict = %v", err)
	}
}

func TestDeleteOccurrenceScopes(t *testing.T) {
	cases := []struct {
		scope model.Scope
		day   time.Time
		check func(RepeatRecord, bool) bool
	}{
		{model.ScopeThis, localDay(2026, 11, 9), func(r RepeatRecord, ok bool) bool { return ok && r.Skipped(localDay(2026, 11, 9)) }},
		{model.ScopeFollowing, localDay(2026, 11, 9), func(r RepeatRecord, ok bool) bool { return ok && sameDateLocal(r.Repeat.Until, localDay(2026, 11, 8)) }},
		{model.ScopeFollowing, localDay(2026, 11, 2), func(_ RepeatRecord, ok bool) bool { return !ok }},
		{model.ScopeAll, localDay(2026, 11, 9), func(_ RepeatRecord, ok bool) bool { return !ok }},
	}
	for index, tc := range cases {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			db := testDB(t)
			repeat, err := model.NewRepeat("standup", "2026-11-02 09:00:00", "2026-11-02 10:00:00", "weekly")
			if err != nil {
				t.Fatal(err)
			}
			id, err := AddRepeat(db, repeat)
			if err != nil {
				t.Fatal(err)
			}
			if err := DeleteOccurrence(db, id, tc.day, tc.scope); err != nil {
				t.Fatal(err)
			}
			repeats, err := LoadRepeats(db)
			if err != nil {
				t.Fatal(err)
			}
			if found, ok := findRepeat(repeats, id); !tc.check(found, ok) {
				t.Fatalf("after delete: %#v", repeats)
			}
		})
	}
}

// seedEveningSeries stores a 19:30-20:30 weekly series from Mon 2 Nov 2026
// with a link, and a 9 Nov day of its own title and link. AddRepeat takes only
// on-the-hour clocks, so the rows are written directly.
func seedEveningSeries(t *testing.T) string {
	t.Helper()
	db := testDB(t)
	if _, err := Open(db); err != nil {
		t.Fatal(err)
	}
	sql := "INSERT INTO repeat(id, title, start_at, end_at, freq, until_on, url) VALUES (1, 'dev meeting', '2026-11-02 19:30:00', '2026-11-02 20:30:00', 'weekly', NULL, 'https://meet.example/old');\n" +
		"INSERT INTO repeat_override(repeat_id, day, title, start_at, end_at, url) VALUES (1, '2026-11-09', 'retro', NULL, NULL, 'https://meet.example/day');\n"
	if err := execSQL(db, sql); err != nil {
		t.Fatal(err)
	}
	return db
}

func eveningForm(url string, scope model.Scope) model.CalForm {
	return model.CalForm{Title: "dev meeting", StartHour: 19, StartMinute: 30, EndHour: 20, EndMinute: 30, Freq: "weekly", URL: url, Scope: scope}
}

func TestSeriesURLSaveKeepsAnOffHourClock(t *testing.T) {
	const next = "https://meet.example/new"
	scopes := map[string]model.Scope{"this event": model.ScopeThis, "this and following": model.ScopeFollowing, "all events": model.ScopeAll}
	for name, scope := range scopes {
		t.Run(name, func(t *testing.T) {
			db := seedEveningSeries(t)
			nov16 := localDay(2026, 11, 16)
			if _, err := SaveForm(db, FormTarget{RepeatID: 1, Day: nov16}, eveningForm(next, scope)); err != nil {
				t.Fatal(err)
			}
			repeats, err := LoadRepeats(db)
			if err != nil {
				t.Fatal(err)
			}
			first, ok := findRepeat(repeats, 1)
			if !ok || first.Repeat.Start.Format("15:04") != "19:30" || first.Repeat.End.Format("15:04") != "20:30" {
				t.Fatalf("series = %#v", repeats)
			}
			assertEveryOccurrence(t, repeats, next)
		})
	}
}

// assertEveryOccurrence checks that each day of November the series falls on
// shows url at 19:30, and that 9 Nov keeps its own title.
func assertEveryOccurrence(t *testing.T, repeats []RepeatRecord, url string) {
	t.Helper()
	for _, day := range []int{2, 9, 16, 23, 30} {
		found := false
		for _, record := range repeats {
			occurrence, ok := record.On(localDay(2026, 11, day))
			if !ok {
				continue
			}
			found = true
			if occurrence.URL != url || occurrence.Start.Format("15:04") != "19:30" {
				t.Fatalf("%d Nov = %#v", day, occurrence)
			}
			if day == 9 && occurrence.Title != "retro" {
				t.Fatalf("9 Nov lost its title: %#v", occurrence)
			}
		}
		if !found {
			t.Fatalf("no occurrence on %d Nov: %#v", day, repeats)
		}
	}
}

func TestSeriesDayClockMoves(t *testing.T) {
	db := seedEveningSeries(t)
	nov16 := localDay(2026, 11, 16)
	target := FormTarget{RepeatID: 1, Day: nov16}
	offHour := eveningForm("https://meet.example/old", model.ScopeThis)
	offHour.StartMinute, offHour.EndMinute = 45, 45
	if _, err := SaveForm(db, target, offHour); err == nil || !strings.Contains(err.Error(), "on the hour") {
		t.Fatalf("19:45 = %v", err)
	}
	early := eveningForm("https://meet.example/old", model.ScopeThis)
	early.StartHour, early.StartMinute, early.EndHour, early.EndMinute = 18, 0, 19, 0
	saved, err := SaveForm(db, target, early)
	if err != nil || saved.Change != model.ChangeRescheduled {
		t.Fatalf("18:00 = %#v %v", saved, err)
	}
	var rows []struct {
		Title   *string `json:"title"`
		StartAt *string `json:"start_at"`
		EndAt   *string `json:"end_at"`
		URL     *string `json:"url"`
	}
	if err := sqlite.Query(db, "SELECT title, start_at, end_at, url FROM repeat_override WHERE repeat_id = 1 AND day = '2026-11-16'", &rows); err != nil || len(rows) != 1 {
		t.Fatalf("override rows = %#v %v", rows, err)
	}
	row := rows[0]
	if row.Title != nil || row.URL != nil || row.StartAt == nil || *row.StartAt != "2026-11-16 18:00:00" || row.EndAt == nil || *row.EndAt != "2026-11-16 19:00:00" {
		t.Fatalf("override = title %v start %v end %v url %v", row.Title, row.StartAt, row.EndAt, row.URL)
	}
	repeats, err := LoadRepeats(db)
	if err != nil {
		t.Fatal(err)
	}
	if first, _ := findRepeat(repeats, 1); first.Repeat.Start.Format("15:04") != "19:30" {
		t.Fatalf("series clock moved: %#v", first.Repeat)
	}
}
