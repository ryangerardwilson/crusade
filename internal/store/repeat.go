package store

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ryangerardwilson/crusade/internal/model"
)

const repeatModel = "model repeat\n" +
	"  id integer primary key\n" +
	"  title text not null\n" +
	"  start_at text not null\n" +
	"  end_at text not null\n" +
	"  freq text not null\n" +
	"  until_on text\n" +
	"  url text\n"

const repeatChangeModel = "model repeat_change\n" +
	"  id integer primary key\n" +
	"  repeat_id integer not null\n" +
	"  day text not null\n" +
	"  skipped integer not null\n" +
	"  title text\n" +
	"  start_at text\n" +
	"  end_at text\n" +
	"  url text\n"

const createRepeat = `CREATE TABLE IF NOT EXISTS repeat (
  "id" INTEGER PRIMARY KEY,
  "title" TEXT NOT NULL,
  "start_at" TEXT NOT NULL,
  "end_at" TEXT NOT NULL,
  "freq" TEXT NOT NULL,
  "until_on" TEXT,
  "url" TEXT
)`

const createRepeatChange = `CREATE TABLE IF NOT EXISTS repeat_change (
  "id" INTEGER PRIMARY KEY,
  "repeat_id" INTEGER NOT NULL,
  "day" TEXT NOT NULL,
  "skipped" INTEGER NOT NULL,
  "title" TEXT,
  "start_at" TEXT,
  "end_at" TEXT,
  "url" TEXT
)`

const repeatHorizonDays = 366

// RepeatRecord is one recurring block. It is not a do task.
type RepeatRecord struct {
	ID     int
	Repeat model.Repeat
}

// RepeatChange skips one day or replaces that day's title, times, and meeting url.
// HasURL is false when that day still uses the series url.
type RepeatChange struct {
	ID       int
	RepeatID int
	Day      time.Time
	Skipped  bool
	Title    string
	Start    time.Time
	End      time.Time
	URL      string
	HasURL   bool
}

type repeatRow struct {
	ID      int     `json:"id"`
	Title   string  `json:"title"`
	StartAt string  `json:"start_at"`
	EndAt   string  `json:"end_at"`
	Freq    string  `json:"freq"`
	UntilOn *string `json:"until_on"`
	URL     *string `json:"url"`
}

type repeatChangeRow struct {
	ID       int     `json:"id"`
	RepeatID int     `json:"repeat_id"`
	Day      string  `json:"day"`
	Skipped  int     `json:"skipped"`
	Title    *string `json:"title"`
	StartAt  *string `json:"start_at"`
	EndAt    *string `json:"end_at"`
	URL      *string `json:"url"`
}

type span struct {
	start    time.Time
	end      time.Time
	title    string
	blockID  int
	repeatID int
}

func LoadRepeats(dbPath string) ([]RepeatRecord, error) {
	if err := ensure(dbPath); err != nil {
		return nil, err
	}
	var rows []repeatRow
	if err := queryJSON(dbPath, "SELECT id, title, start_at, end_at, freq, until_on, url FROM repeat", &rows); err != nil {
		return nil, fmt.Errorf("failed to read repeats from %s: %w", dbPath, err)
	}
	repeats := make([]RepeatRecord, 0, len(rows))
	seen := make(map[int]bool, len(rows))
	for _, row := range rows {
		if row.ID <= 0 {
			return nil, fmt.Errorf("failed to read repeats from %s: repeat has invalid id", dbPath)
		}
		if seen[row.ID] {
			return nil, fmt.Errorf("failed to read repeats from %s: duplicate repeat id %d", dbPath, row.ID)
		}
		seen[row.ID] = true
		repeat, err := model.NewRepeat(row.Title, row.StartAt, row.EndAt, row.Freq)
		if err != nil {
			return nil, fmt.Errorf("failed to read repeats from %s: repeat id %d: %w", dbPath, row.ID, err)
		}
		if row.UntilOn != nil && strings.TrimSpace(*row.UntilOn) != "" {
			until, err := parseDay(*row.UntilOn)
			if err != nil {
				return nil, fmt.Errorf("failed to read repeats from %s: repeat id %d: %w", dbPath, row.ID, err)
			}
			repeat.Until = until
		}
		repeat.URL = plainURL(row.URL)
		repeats = append(repeats, RepeatRecord{ID: row.ID, Repeat: repeat})
	}
	sort.SliceStable(repeats, func(i, j int) bool {
		if repeats[i].Repeat.Start.Equal(repeats[j].Repeat.Start) {
			return repeats[i].ID < repeats[j].ID
		}
		return repeats[i].Repeat.Start.Before(repeats[j].Repeat.Start)
	})
	return repeats, nil
}

func LoadRepeatChanges(dbPath string) ([]RepeatChange, error) {
	if err := ensure(dbPath); err != nil {
		return nil, err
	}
	var rows []repeatChangeRow
	if err := queryJSON(dbPath, "SELECT id, repeat_id, day, skipped, title, start_at, end_at, url FROM repeat_change", &rows); err != nil {
		return nil, fmt.Errorf("failed to read repeat changes from %s: %w", dbPath, err)
	}
	changes := make([]RepeatChange, 0, len(rows))
	for _, row := range rows {
		day, err := parseDay(row.Day)
		if err != nil {
			return nil, fmt.Errorf("failed to read repeat changes from %s: change id %d: %w", dbPath, row.ID, err)
		}
		change := RepeatChange{ID: row.ID, RepeatID: row.RepeatID, Day: day, Skipped: row.Skipped != 0}
		if row.URL != nil {
			change.URL = strings.TrimSpace(*row.URL)
			change.HasURL = true
		}
		if change.Skipped {
			changes = append(changes, change)
			continue
		}
		if row.Title == nil || row.StartAt == nil || row.EndAt == nil {
			return nil, fmt.Errorf("failed to read repeat changes from %s: change id %d is incomplete", dbPath, row.ID)
		}
		block, err := model.NewBlock(*row.Title, *row.StartAt, *row.EndAt)
		if err != nil {
			return nil, fmt.Errorf("failed to read repeat changes from %s: change id %d: %w", dbPath, row.ID, err)
		}
		change.Title = block.Title
		change.Start = block.Start
		change.End = block.End
		changes = append(changes, change)
	}
	return changes, nil
}

func AddRepeat(dbPath string, repeat model.Repeat) (int, error) {
	repeat, err := normalizeRepeat(repeat)
	if err != nil {
		return 0, err
	}
	if err := spanConflicts(dbPath, probeRepeat(repeat), nil); err != nil {
		return 0, err
	}
	return insertRepeat(dbPath, repeat)
}

func UpdateRepeat(dbPath string, id int, repeat model.Repeat) error {
	return saveRepeat(dbPath, id, repeat, false)
}

// ReplaceRepeat saves the series clock and drops renamed or rescheduled days.
// Skipped days stay skipped.
func ReplaceRepeat(dbPath string, id int, repeat model.Repeat) error {
	if err := saveRepeat(dbPath, id, repeat, true); err != nil {
		return err
	}
	return DeleteRepeatOverrides(dbPath, id)
}

func saveRepeat(dbPath string, id int, repeat model.Repeat, dropOverrides bool) error {
	repeat, err := normalizeRepeat(repeat)
	if err != nil {
		return err
	}
	changes, err := LoadRepeatChanges(dbPath)
	if err != nil {
		return err
	}
	if err := spanConflicts(dbPath, occurrenceProbes(repeat, changes, id, dropOverrides), func(existing span) bool {
		return existing.repeatID == id
	}); err != nil {
		return err
	}
	if err := ensure(dbPath); err != nil {
		return err
	}
	ok, err := repeatExists(dbPath, id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("ID %d not found", id)
	}
	return execSQL(dbPath, repeatUpdateSQL(id, repeat))
}

func DeleteRepeat(dbPath string, id int) error {
	if err := ensure(dbPath); err != nil {
		return err
	}
	if err := execSQL(dbPath, fmt.Sprintf("DELETE FROM repeat_change WHERE repeat_id = %d;\n", id)); err != nil {
		return err
	}
	changed, err := rowsChanged(dbPath, fmt.Sprintf("DELETE FROM repeat WHERE id = %d;\n", id))
	if err != nil {
		return err
	}
	if !changed {
		return fmt.Errorf("ID %d not found", id)
	}
	return nil
}

func SetRepeatUntil(dbPath string, id int, until time.Time) error {
	repeats, err := LoadRepeats(dbPath)
	if err != nil {
		return err
	}
	current, ok := findRepeat(repeats, id)
	if !ok {
		return fmt.Errorf("ID %d not found", id)
	}
	current.Repeat.Until = dateOnlyLocal(until)
	if err := UpdateRepeat(dbPath, id, current.Repeat); err != nil {
		return err
	}
	return DeleteRepeatChangesFrom(dbPath, id, dateOnlyLocal(until).AddDate(0, 0, 1))
}

// ContinueRepeat keeps the days before day and starts next on day.
// When day is the first day, next replaces the series.
// clearOverrides drops renamed or rescheduled days when the whole series is replaced.
func ContinueRepeat(dbPath string, id int, day time.Time, next model.Repeat, clearOverrides bool) (int, error) {
	repeats, err := LoadRepeats(dbPath)
	if err != nil {
		return 0, err
	}
	current, ok := findRepeat(repeats, id)
	if !ok {
		return 0, fmt.Errorf("ID %d not found", id)
	}
	day = dateOnlyLocal(day)
	next, err = normalizeRepeat(next)
	if err != nil {
		return 0, err
	}
	if sameDateLocal(current.Repeat.Start, day) {
		next.Until = current.Repeat.Until
		var err error
		if clearOverrides {
			err = ReplaceRepeat(dbPath, id, next)
		} else {
			err = UpdateRepeat(dbPath, id, next)
		}
		if err != nil {
			return 0, err
		}
		return id, nil
	}
	if !current.Repeat.Until.IsZero() && !dateOnlyLocal(current.Repeat.Until).Before(day) {
		next.Until = current.Repeat.Until
	}
	if err := spanConflicts(dbPath, probeRepeat(next), func(existing span) bool {
		return existing.repeatID == id && !existing.start.Before(day)
	}); err != nil {
		return 0, err
	}
	if err := SetRepeatUntil(dbPath, id, day.AddDate(0, 0, -1)); err != nil {
		return 0, err
	}
	return AddRepeat(dbPath, next)
}

func MakeRepeatFromBlock(dbPath string, blockID int, repeat model.Repeat) (int, error) {
	repeat, err := normalizeRepeat(repeat)
	if err != nil {
		return 0, err
	}
	if err := spanConflicts(dbPath, probeRepeat(repeat), func(existing span) bool {
		return existing.blockID == blockID
	}); err != nil {
		return 0, err
	}
	if err := DeleteBlock(dbPath, blockID); err != nil {
		return 0, err
	}
	return insertRepeat(dbPath, repeat)
}

func SkipRepeatDay(dbPath string, id int, day time.Time) error {
	if _, ok, err := requireRepeat(dbPath, id); err != nil || !ok {
		if err != nil {
			return err
		}
		return fmt.Errorf("ID %d not found", id)
	}
	return saveChange(dbPath, RepeatChange{RepeatID: id, Day: dateOnlyLocal(day), Skipped: true})
}

func OverrideRepeatDay(dbPath string, id int, day time.Time, title string, start, end time.Time, url string) error {
	if _, ok, err := requireRepeat(dbPath, id); err != nil || !ok {
		if err != nil {
			return err
		}
		return fmt.Errorf("ID %d not found", id)
	}
	block, err := model.NewBlock(title, start.Format(model.DateTimeFormat), end.Format(model.DateTimeFormat))
	if err != nil {
		return err
	}
	if err := block.OnTheHour(); err != nil {
		return err
	}
	if !sameDateLocal(block.Start, day) {
		return fmt.Errorf("a block stays on one day")
	}
	probe := span{start: block.Start, end: block.End}
	if err := spanConflicts(dbPath, []span{probe}, func(existing span) bool {
		return existing.repeatID == id && sameDateLocal(existing.start, day)
	}); err != nil {
		return err
	}
	return saveChange(dbPath, RepeatChange{
		RepeatID: id,
		Day:      dateOnlyLocal(day),
		Title:    block.Title,
		Start:    block.Start,
		End:      block.End,
		URL:      strings.TrimSpace(url),
		HasURL:   true,
	})
}

func DeleteRepeatOverrides(dbPath string, id int) error {
	if err := ensure(dbPath); err != nil {
		return err
	}
	return execSQL(dbPath, fmt.Sprintf("DELETE FROM repeat_change WHERE repeat_id = %d AND skipped = 0;\n", id))
}

func RenameRepeatOverrides(dbPath string, id int, title string) error {
	if err := ensure(dbPath); err != nil {
		return err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("a block needs a title")
	}
	return execSQL(dbPath, fmt.Sprintf("UPDATE repeat_change SET title = %s WHERE repeat_id = %d AND skipped = 0;\n", sqlString(title), id))
}

func SetRepeatOverrideURLs(dbPath string, id int, url string) error {
	if err := ensure(dbPath); err != nil {
		return err
	}
	return execSQL(dbPath, fmt.Sprintf("UPDATE repeat_change SET url = %s WHERE repeat_id = %d AND skipped = 0;\n", sqlString(strings.TrimSpace(url)), id))
}

func DeleteRepeatChangesFrom(dbPath string, id int, day time.Time) error {
	if err := ensure(dbPath); err != nil {
		return err
	}
	return execSQL(dbPath, fmt.Sprintf("DELETE FROM repeat_change WHERE repeat_id = %d AND day >= %s;\n", id, sqlString(dateOnlyLocal(day).Format(time.DateOnly))))
}

func normalizeRepeat(repeat model.Repeat) (model.Repeat, error) {
	next, err := model.NewRepeat(repeat.Title, repeat.Start.Format(model.DateTimeFormat), repeat.End.Format(model.DateTimeFormat), repeat.Freq)
	if err != nil {
		return model.Repeat{}, err
	}
	next.Until = repeat.Until
	next.URL = strings.TrimSpace(repeat.URL)
	if !next.Until.IsZero() && dateOnlyLocal(next.Until).Before(dateOnlyLocal(next.Start)) {
		return model.Repeat{}, fmt.Errorf("a repeat ends on or after it starts")
	}
	if err := next.OnTheHour(); err != nil {
		return model.Repeat{}, err
	}
	return next, nil
}

func insertRepeat(dbPath string, repeat model.Repeat) (int, error) {
	if err := ensure(dbPath); err != nil {
		return 0, err
	}
	id, err := nextRepeatID(dbPath)
	if err != nil {
		return 0, err
	}
	if err := execSQL(dbPath, repeatInsertSQL(id, repeat)); err != nil {
		return 0, err
	}
	return id, nil
}

func saveChange(dbPath string, change RepeatChange) error {
	if err := ensure(dbPath); err != nil {
		return err
	}
	changes, err := LoadRepeatChanges(dbPath)
	if err != nil {
		return err
	}
	change.Day = dateOnlyLocal(change.Day)
	if found, ok := changeFor(changes, change.RepeatID, change.Day); ok {
		return execSQL(dbPath, changeUpdateSQL(found.ID, change))
	}
	id, err := nextChangeID(dbPath)
	if err != nil {
		return err
	}
	return execSQL(dbPath, changeInsertSQL(id, change))
}

func requireRepeat(dbPath string, id int) (RepeatRecord, bool, error) {
	repeats, err := LoadRepeats(dbPath)
	if err != nil {
		return RepeatRecord{}, false, err
	}
	found, ok := findRepeat(repeats, id)
	return found, ok, nil
}

func findRepeat(repeats []RepeatRecord, id int) (RepeatRecord, bool) {
	for _, repeat := range repeats {
		if repeat.ID == id {
			return repeat, true
		}
	}
	return RepeatRecord{}, false
}

func changeFor(changes []RepeatChange, id int, day time.Time) (RepeatChange, bool) {
	for _, change := range changes {
		if change.RepeatID == id && sameDateLocal(change.Day, day) {
			return change, true
		}
	}
	return RepeatChange{}, false
}

func repeatExists(dbPath string, id int) (bool, error) {
	_, ok, err := requireRepeat(dbPath, id)
	return ok, err
}

func blockConflicts(dbPath string, id int, block model.Block) error {
	probe := span{start: block.Start, end: block.End}
	return spanConflicts(dbPath, []span{probe}, func(existing span) bool {
		return id != 0 && existing.blockID == id
	})
}

func spanConflicts(dbPath string, probes []span, skip func(span) bool) error {
	if len(probes) == 0 {
		return nil
	}
	from := dateOnlyLocal(probes[0].start)
	to := from
	for _, probe := range probes[1:] {
		day := dateOnlyLocal(probe.start)
		if day.Before(from) {
			from = day
		}
		if day.After(to) {
			to = day
		}
	}
	existing, err := collectSpans(dbPath, from, to)
	if err != nil {
		return err
	}
	for _, probe := range probes {
		for _, other := range existing {
			if skip != nil && skip(other) {
				continue
			}
			if probe.start.Before(other.end) && other.start.Before(probe.end) {
				return blockedBy(other)
			}
		}
	}
	return nil
}

func collectSpans(dbPath string, from, to time.Time) ([]span, error) {
	blocks, err := LoadBlocks(dbPath)
	if err != nil {
		return nil, err
	}
	repeats, err := LoadRepeats(dbPath)
	if err != nil {
		return nil, err
	}
	changes, err := LoadRepeatChanges(dbPath)
	if err != nil {
		return nil, err
	}
	from = dateOnlyLocal(from)
	to = dateOnlyLocal(to)
	spans := make([]span, 0)
	for _, block := range blocks {
		day := dateOnlyLocal(block.Block.Start)
		if day.Before(from) || day.After(to) {
			continue
		}
		spans = append(spans, span{start: block.Block.Start, end: block.Block.End, title: block.Block.Title, blockID: block.ID})
	}
	for _, repeat := range repeats {
		spans = append(spans, expandRepeat(repeat, changes, from, to)...)
	}
	return spans, nil
}

func expandRepeat(repeat RepeatRecord, changes []RepeatChange, from, to time.Time) []span {
	from = dateOnlyLocal(from)
	to = dateOnlyLocal(to)
	start := dateOnlyLocal(repeat.Repeat.Start)
	if from.Before(start) {
		from = start
	}
	out := make([]span, 0)
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		if !repeat.Repeat.OccursOn(day) {
			continue
		}
		change, ok := changeFor(changes, repeat.ID, day)
		if ok && change.Skipped {
			continue
		}
		spanStart, spanEnd := repeat.Repeat.SpanOn(day)
		title := repeat.Repeat.Title
		if ok {
			spanStart, spanEnd = change.Start, change.End
			if strings.TrimSpace(change.Title) != "" {
				title = change.Title
			}
		}
		out = append(out, span{start: spanStart, end: spanEnd, title: title, repeatID: repeat.ID})
	}
	return out
}

func occurrenceProbes(repeat model.Repeat, changes []RepeatChange, id int, dropOverrides bool) []span {
	from, to := repeatWindow(repeat)
	out := make([]span, 0)
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		if !repeat.OccursOn(day) {
			continue
		}
		change, ok := changeFor(changes, id, day)
		if ok && change.Skipped {
			continue
		}
		start, end := repeat.SpanOn(day)
		if ok && !dropOverrides {
			start, end = change.Start, change.End
		}
		out = append(out, span{start: start, end: end})
	}
	return out
}

func probeRepeat(repeat model.Repeat) []span {
	from, to := repeatWindow(repeat)
	out := make([]span, 0)
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		if !repeat.OccursOn(day) {
			continue
		}
		start, end := repeat.SpanOn(day)
		out = append(out, span{start: start, end: end})
	}
	return out
}

func blockedBy(other span) error {
	when := other.start.Format("2 Jan 2006")
	name := strings.TrimSpace(other.title)
	if name == "" {
		return fmt.Errorf("that time is already blocked on %s", when)
	}
	return fmt.Errorf("that time is already blocked by %s on %s", name, when)
}

func repeatWindow(repeat model.Repeat) (time.Time, time.Time) {
	from := dateOnlyLocal(repeat.Start)
	to := from.AddDate(0, 0, repeatHorizonDays)
	if !repeat.Until.IsZero() {
		until := dateOnlyLocal(repeat.Until)
		if until.Before(to) {
			to = until
		}
	}
	return from, to
}

func nextRepeatID(dbPath string) (int, error) {
	return nextTableID(dbPath, "repeat")
}

func nextChangeID(dbPath string) (int, error) {
	return nextTableID(dbPath, "repeat_change")
}

func repeatInsertSQL(id int, repeat model.Repeat) string {
	return fmt.Sprintf("INSERT INTO repeat(id, title, start_at, end_at, freq, until_on, url) VALUES (%d, %s, %s, %s, %s, %s, %s);\n",
		id,
		sqlString(repeat.Title),
		sqlString(repeat.Start.Format(model.DateTimeFormat)),
		sqlString(repeat.End.Format(model.DateTimeFormat)),
		sqlString(repeat.Freq),
		untilSQL(repeat.Until),
		storedURL(repeat.URL),
	)
}

func repeatUpdateSQL(id int, repeat model.Repeat) string {
	return fmt.Sprintf("UPDATE repeat SET title = %s, start_at = %s, end_at = %s, freq = %s, until_on = %s, url = %s WHERE id = %d;\n",
		sqlString(repeat.Title),
		sqlString(repeat.Start.Format(model.DateTimeFormat)),
		sqlString(repeat.End.Format(model.DateTimeFormat)),
		sqlString(repeat.Freq),
		untilSQL(repeat.Until),
		storedURL(repeat.URL),
		id,
	)
}

func changeInsertSQL(id int, change RepeatChange) string {
	if change.Skipped {
		return fmt.Sprintf("INSERT INTO repeat_change(id, repeat_id, day, skipped, title, start_at, end_at, url) VALUES (%d, %d, %s, 1, NULL, NULL, NULL, NULL);\n",
			id, change.RepeatID, sqlString(change.Day.Format(time.DateOnly)))
	}
	return fmt.Sprintf("INSERT INTO repeat_change(id, repeat_id, day, skipped, title, start_at, end_at, url) VALUES (%d, %d, %s, 0, %s, %s, %s, %s);\n",
		id,
		change.RepeatID,
		sqlString(change.Day.Format(time.DateOnly)),
		sqlString(change.Title),
		sqlString(change.Start.Format(model.DateTimeFormat)),
		sqlString(change.End.Format(model.DateTimeFormat)),
		storedChangeURL(change),
	)
}

func changeUpdateSQL(id int, change RepeatChange) string {
	if change.Skipped {
		return fmt.Sprintf("UPDATE repeat_change SET skipped = 1, title = NULL, start_at = NULL, end_at = NULL, url = NULL WHERE id = %d;\n", id)
	}
	return fmt.Sprintf("UPDATE repeat_change SET skipped = 0, title = %s, start_at = %s, end_at = %s, url = %s WHERE id = %d;\n",
		sqlString(change.Title),
		sqlString(change.Start.Format(model.DateTimeFormat)),
		sqlString(change.End.Format(model.DateTimeFormat)),
		storedChangeURL(change),
		id,
	)
}

func untilSQL(until time.Time) string {
	if until.IsZero() {
		return "NULL"
	}
	return sqlString(dateOnlyLocal(until).Format(time.DateOnly))
}

func parseDay(raw string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if len(value) >= 10 {
		if day, err := time.ParseInLocation(time.DateOnly, value[:10], time.Local); err == nil {
			return day, nil
		}
	}
	parsed, err := model.ParseDateTime(value)
	if err != nil {
		return time.Time{}, err
	}
	return dateOnlyLocal(parsed), nil
}

func dateOnlyLocal(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.Local)
}

func sameDateLocal(a, b time.Time) bool {
	return dateOnlyLocal(a).Equal(dateOnlyLocal(b))
}
