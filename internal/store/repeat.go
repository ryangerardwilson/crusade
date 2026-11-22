package store

import (
	"github.com/ryangerardwilson/crusade/internal/sqlite"

	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ryangerardwilson/crusade/internal/model"
)

const repeatHorizonDays = 366

// RepeatRecord is one stored series with its skipped and overridden days.
type RepeatRecord struct {
	ID int
	model.Series
}

// FormTarget is what the calendar form saves: a new entry on Day, the block
// BlockID, or the day Day of the series RepeatID.
type FormTarget struct {
	BlockID  int
	RepeatID int
	Day      time.Time
}

// Saved is what a calendar form save wrote.
type Saved struct {
	BlockID  int
	RepeatID int
	Title    string
	Freq     string
	Change   model.CalChange
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

type overrideRow struct {
	RepeatID int     `json:"repeat_id"`
	Day      string  `json:"day"`
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

// LoadRepeats reads every series with its skipped and overridden days.
func LoadRepeats(dbPath string) ([]RepeatRecord, error) {
	if err := ensure(dbPath); err != nil {
		return nil, err
	}
	var rows []repeatRow
	if err := sqlite.Query(dbPath, "SELECT id, title, start_at, end_at, freq, until_on, url FROM repeat", &rows); err != nil {
		return nil, fmt.Errorf("failed to read repeats from %s: %w", dbPath, err)
	}
	repeats := make([]RepeatRecord, 0, len(rows))
	index := map[int]int{}
	for _, row := range rows {
		repeat, err := repeatFromRow(row)
		if err != nil {
			return nil, fmt.Errorf("failed to read repeats from %s: repeat id %d: %w", dbPath, row.ID, err)
		}
		index[row.ID] = len(repeats)
		repeats = append(repeats, RepeatRecord{ID: row.ID, Series: model.Series{Repeat: repeat}})
	}
	if err := loadSeriesDays(dbPath, repeats, index); err != nil {
		return nil, fmt.Errorf("failed to read repeats from %s: %w", dbPath, err)
	}
	sort.SliceStable(repeats, func(i, j int) bool {
		if repeats[i].Repeat.Start.Equal(repeats[j].Repeat.Start) {
			return repeats[i].ID < repeats[j].ID
		}
		return repeats[i].Repeat.Start.Before(repeats[j].Repeat.Start)
	})
	return repeats, nil
}

func repeatFromRow(row repeatRow) (model.Repeat, error) {
	repeat, err := model.NewRepeat(row.Title, row.StartAt, row.EndAt, row.Freq)
	if err != nil {
		return model.Repeat{}, err
	}
	if row.UntilOn != nil {
		until, err := parseDay(*row.UntilOn)
		if err != nil {
			return model.Repeat{}, err
		}
		repeat.Until = until
	}
	repeat.URL = plainURL(row.URL)
	return repeat, nil
}

func loadSeriesDays(dbPath string, repeats []RepeatRecord, index map[int]int) error {
	var skips []struct {
		RepeatID int    `json:"repeat_id"`
		Day      string `json:"day"`
	}
	if err := sqlite.Query(dbPath, "SELECT repeat_id, day FROM repeat_skip", &skips); err != nil {
		return err
	}
	for _, skip := range skips {
		day, err := parseDay(skip.Day)
		if err != nil {
			return err
		}
		at := index[skip.RepeatID]
		repeats[at].Skips = append(repeats[at].Skips, day)
	}
	var overrides []overrideRow
	if err := sqlite.Query(dbPath, "SELECT repeat_id, day, title, start_at, end_at, url FROM repeat_override", &overrides); err != nil {
		return err
	}
	for _, row := range overrides {
		override, err := overrideFromRow(row)
		if err != nil {
			return err
		}
		at := index[row.RepeatID]
		repeats[at].Overrides = append(repeats[at].Overrides, override)
	}
	return nil
}

func overrideFromRow(row overrideRow) (model.RepeatOverride, error) {
	day, err := parseDay(row.Day)
	if err != nil {
		return model.RepeatOverride{}, err
	}
	override := model.RepeatOverride{Day: day, URL: plainURL(row.URL)}
	if row.Title != nil {
		override.Title = *row.Title
	}
	if row.StartAt != nil && row.EndAt != nil {
		block, err := model.NewBlock("day", *row.StartAt, *row.EndAt)
		if err != nil {
			return model.RepeatOverride{}, err
		}
		override.Start, override.End = block.Start, block.End
	}
	return override, nil
}

// AddRepeat stores a new series on the hour.
func AddRepeat(dbPath string, repeat model.Repeat) (int, error) {
	repeat, err := normalizeRepeat(repeat)
	if err != nil {
		return 0, err
	}
	if err := repeat.OnTheHour(); err != nil {
		return 0, err
	}
	if err := spanConflicts(dbPath, probeRepeat(repeat), nil); err != nil {
		return 0, err
	}
	var rows []struct {
		ID int `json:"id"`
	}
	if err := sqlite.Query(dbPath, "INSERT INTO repeat(title, start_at, end_at, freq, until_on, url) VALUES ("+repeatValuesSQL(repeat)+") RETURNING id;\n", &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("%s: sqlite returned no id", dbPath)
	}
	published(dbPath)
	return rows[0].ID, nil
}

// MakeRepeatFromBlock turns the block into a series in one transaction.
func MakeRepeatFromBlock(dbPath string, blockID int, repeat model.Repeat) (int, error) {
	repeat, err := normalizeRepeat(repeat)
	if err != nil {
		return 0, err
	}
	if err := repeat.OnTheHour(); err != nil {
		return 0, err
	}
	if err := spanConflicts(dbPath, probeRepeat(repeat), func(existing span) bool {
		return existing.blockID == blockID
	}); err != nil {
		return 0, err
	}
	var rows []struct {
		ID int `json:"id"`
	}
	sql := inTx(
		"INSERT INTO repeat(title, start_at, end_at, freq, until_on, url) SELECT "+repeatValuesSQL(repeat)+fmt.Sprintf(" WHERE EXISTS (SELECT 1 FROM block WHERE id = %d) RETURNING id;\n", blockID),
		fmt.Sprintf("DELETE FROM block WHERE id = %d;\n", blockID),
	)
	if err := sqlite.Query(dbPath, sql, &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("ID %d not found", blockID)
	}
	published(dbPath)
	return rows[0].ID, nil
}

// DeleteRepeat removes the series; its skipped and overridden days go with it.
func DeleteRepeat(dbPath string, id int) error {
	if err := ensure(dbPath); err != nil {
		return err
	}
	return changeOne(dbPath, fmt.Sprintf("DELETE FROM repeat WHERE id = %d;\n", id), id)
}

// DeleteOccurrence removes day of the series id: that day, that day and the
// days after it, or every day, as scope says.
func DeleteOccurrence(dbPath string, id int, day time.Time, scope model.Scope) error {
	current, err := requireRepeat(dbPath, id)
	if err != nil {
		return err
	}
	plan := current.DeleteDay(day, scope)
	switch {
	case plan.Whole:
		return DeleteRepeat(dbPath, id)
	case plan.Skip:
		return execSQL(dbPath, inTx(skipSQL(id, plan.Day)))
	default:
		series := current.Repeat
		series.Until = plan.Until
		if series, err = normalizeRepeat(series); err != nil {
			return err
		}
		return execSQL(dbPath, inTx(repeatUpdateSQL(id, series), daysFromSQL(id, plan.Day)))
	}
}

// SaveForm stores one calendar form save. It decides from what changed
// whether a new entry is a block or a series, whether a block stays a block,
// and for a series day which days the save reaches. Each case is one
// transaction.
func SaveForm(dbPath string, target FormTarget, form model.CalForm) (Saved, error) {
	switch {
	case target.RepeatID != 0:
		return saveRepeatForm(dbPath, target, form)
	case target.BlockID != 0:
		return saveBlockForm(dbPath, target.BlockID, form)
	default:
		return addFormEntry(dbPath, target.Day, form)
	}
}

func addFormEntry(dbPath string, day time.Time, form model.CalForm) (Saved, error) {
	block, repeat, isRepeat, err := model.NewCalEntry(day, form)
	if err != nil {
		return Saved{}, err
	}
	if isRepeat {
		id, err := AddRepeat(dbPath, repeat)
		return Saved{RepeatID: id, Title: repeat.Title, Freq: repeat.Freq, Change: model.ChangeRepeats}, err
	}
	id, err := AddBlock(dbPath, block)
	return Saved{BlockID: id, Title: block.Title, Change: model.ChangeBlocked}, err
}

func saveBlockForm(dbPath string, id int, form model.CalForm) (Saved, error) {
	current, err := requireBlock(dbPath, id)
	if err != nil {
		return Saved{}, err
	}
	edit, err := model.EditBlock(current, form)
	if err != nil {
		return Saved{}, err
	}
	if edit.MakeRepeat {
		repeatID, err := MakeRepeatFromBlock(dbPath, id, edit.Repeat)
		return Saved{RepeatID: repeatID, Title: edit.Repeat.Title, Freq: edit.Repeat.Freq, Change: edit.Change}, err
	}
	err = updateBlock(dbPath, id, edit.Block)
	return Saved{BlockID: id, Title: edit.Block.Title, Change: edit.Change}, err
}

func saveRepeatForm(dbPath string, target FormTarget, form model.CalForm) (Saved, error) {
	current, err := requireRepeat(dbPath, target.RepeatID)
	if err != nil {
		return Saved{}, err
	}
	edit, err := current.EditRepeat(target.Day, form)
	if err != nil {
		return Saved{}, err
	}
	saved := Saved{RepeatID: current.ID, Title: edit.Next.Title, Freq: edit.Next.Freq, Change: edit.Change}
	next, err := normalizeRepeat(edit.Next)
	if err != nil {
		return Saved{}, err
	}
	edit.Next = next
	switch edit.Action {
	case model.ActSeriesURL, model.ActUpdateAll:
		err = execSQL(dbPath, inTx(seriesSQL(current.ID, edit, time.Time{})...))
	case model.ActOverride:
		err = saveOverride(dbPath, current, edit)
	case model.ActReplaceAll:
		err = replaceSeries(dbPath, current, edit)
	default:
		saved.RepeatID, err = continueSeries(dbPath, current, edit)
	}
	return saved, err
}

// seriesSQL writes the series and, from day on (every day when day is zero),
// lets the overridden days take its new title or url.
func seriesSQL(id int, edit model.RepeatEdit, from time.Time) []string {
	statements := []string{repeatUpdateSQL(id, edit.Next)}
	if edit.RenameDays {
		statements = append(statements, clearOverrideSQL(id, "title", from))
	}
	if edit.ApplyURL {
		statements = append(statements, clearOverrideSQL(id, "url", time.Time{}))
	}
	return statements
}

func saveOverride(dbPath string, current RepeatRecord, edit model.RepeatEdit) error {
	day := edit.Day
	if edit.Override.HasClock() {
		probe := span{start: edit.Override.Start, end: edit.Override.End}
		if err := spanConflicts(dbPath, []span{probe}, func(existing span) bool {
			return existing.repeatID == current.ID && sameDateLocal(existing.start, day)
		}); err != nil {
			return err
		}
	}
	statements := []string{}
	if edit.ApplyURL {
		statements = append(statements, repeatUpdateSQL(current.ID, edit.Next), clearOverrideSQL(current.ID, "url", time.Time{}))
	}
	statements = append(statements, fmt.Sprintf("DELETE FROM repeat_override WHERE repeat_id = %d AND day = %s;\n", current.ID, daySQL(day)))
	if edit.KeepOverride {
		statements = append(statements, overrideInsertSQL(current.ID, edit.Override))
	}
	return execSQL(dbPath, inTx(statements...))
}

// replaceSeries moves every day to the new clock or frequency and drops the
// overridden days. Skipped days stay skipped.
func replaceSeries(dbPath string, current RepeatRecord, edit model.RepeatEdit) error {
	probes := occurrenceProbes(model.Series{Repeat: edit.Next, Skips: current.Skips})
	if err := spanConflicts(dbPath, probes, func(existing span) bool {
		return existing.repeatID == current.ID
	}); err != nil {
		return err
	}
	return execSQL(dbPath, inTx(
		repeatUpdateSQL(current.ID, edit.Next),
		fmt.Sprintf("DELETE FROM repeat_override WHERE repeat_id = %d;\n", current.ID),
	))
}

// continueSeries keeps the days before edit.Day and starts edit.Next on it.
// From the first day it is the whole series.
func continueSeries(dbPath string, current RepeatRecord, edit model.RepeatEdit) (int, error) {
	day := edit.Day
	next := edit.Next
	if sameDateLocal(current.Repeat.Start, day) {
		next.Until = current.Repeat.Until
		edit.Next = next
		if edit.ClearDays {
			return current.ID, replaceSeries(dbPath, current, edit)
		}
		return current.ID, execSQL(dbPath, inTx(seriesSQL(current.ID, edit, time.Time{})...))
	}
	if !current.Repeat.Until.IsZero() && !dateOnlyLocal(current.Repeat.Until).Before(day) {
		next.Until = current.Repeat.Until
	}
	if err := spanConflicts(dbPath, probeRepeat(next), func(existing span) bool {
		return existing.repeatID == current.ID && !existing.start.Before(day)
	}); err != nil {
		return 0, err
	}
	earlier := current.Repeat
	earlier.Until = day.AddDate(0, 0, -1)
	var statements []string
	if edit.ApplyURL {
		earlier.URL = next.URL
		statements = append(statements, clearOverrideSQL(current.ID, "url", time.Time{}))
	}
	earlier, err := normalizeRepeat(earlier)
	if err != nil {
		return 0, err
	}
	statements = append(statements, repeatUpdateSQL(current.ID, earlier), daysFromSQL(current.ID, day),
		"INSERT INTO repeat(title, start_at, end_at, freq, until_on, url) VALUES ("+repeatValuesSQL(next)+") RETURNING id;\n")
	var rows []struct {
		ID int `json:"id"`
	}
	if err := sqlite.Query(dbPath, inTx(statements...), &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("%s: sqlite returned no id", dbPath)
	}
	published(dbPath)
	return rows[0].ID, nil
}

// inTx wraps statements in one transaction. sqlite bails at the first failed
// statement, so nothing before it commits.
func inTx(statements ...string) string {
	return "BEGIN IMMEDIATE;\n" + strings.Join(statements, "") + "COMMIT;\n"
}

func skipSQL(id int, day time.Time) string {
	return fmt.Sprintf("DELETE FROM repeat_override WHERE repeat_id = %[1]d AND day = %[2]s;\n"+
		"INSERT OR IGNORE INTO repeat_skip(repeat_id, day) VALUES (%[1]d, %[2]s);\n", id, daySQL(day))
}

// clearOverrideSQL lets the overridden days of id from day on (every day when
// from is zero) take the series value of column, title or url. A day left
// with nothing of its own is no longer overridden.
func clearOverrideSQL(id int, column string, from time.Time) string {
	others := "start_at IS NULL AND url IS NULL"
	if column == "url" {
		others = "start_at IS NULL AND title IS NULL"
	}
	where := fmt.Sprintf("repeat_id = %d", id)
	if !from.IsZero() {
		where += " AND day >= " + daySQL(from)
	}
	return fmt.Sprintf("DELETE FROM repeat_override WHERE %s AND %s;\nUPDATE repeat_override SET %s = NULL WHERE %s;\n", where, others, column, where)
}

func daysFromSQL(id int, day time.Time) string {
	return fmt.Sprintf("DELETE FROM repeat_skip WHERE repeat_id = %[1]d AND day >= %[2]s;\n"+
		"DELETE FROM repeat_override WHERE repeat_id = %[1]d AND day >= %[2]s;\n", id, daySQL(day))
}

func overrideInsertSQL(id int, override model.RepeatOverride) string {
	start, end := "NULL", "NULL"
	if override.HasClock() {
		start = sqlite.Quote(override.Start.Format(model.DateTimeFormat))
		end = sqlite.Quote(override.End.Format(model.DateTimeFormat))
	}
	return fmt.Sprintf("INSERT INTO repeat_override(repeat_id, day, title, start_at, end_at, url) VALUES (%d, %s, %s, %s, %s, %s);\n",
		id, daySQL(override.Day), nullableSQL(override.Title), start, end, storedURL(override.URL))
}

func nullableSQL(value string) string {
	if value == "" {
		return "NULL"
	}
	return sqlite.Quote(value)
}

func daySQL(day time.Time) string {
	return sqlite.Quote(dateOnlyLocal(day).Format(time.DateOnly))
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
	return next, nil
}

func requireRepeat(dbPath string, id int) (RepeatRecord, error) {
	repeats, err := LoadRepeats(dbPath)
	if err != nil {
		return RepeatRecord{}, err
	}
	for _, repeat := range repeats {
		if repeat.ID == id {
			return repeat, nil
		}
	}
	return RepeatRecord{}, fmt.Errorf("ID %d not found", id)
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
		for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
			if occurrence, ok := repeat.On(day); ok {
				spans = append(spans, span{start: occurrence.Start, end: occurrence.End, title: occurrence.Title, repeatID: repeat.ID})
			}
		}
	}
	return spans, nil
}

// occurrenceProbes is every day of the series within the horizon.
func occurrenceProbes(series model.Series) []span {
	from, to := repeatWindow(series.Repeat)
	out := make([]span, 0)
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		if occurrence, ok := series.On(day); ok {
			out = append(out, span{start: occurrence.Start, end: occurrence.End})
		}
	}
	return out
}

func probeRepeat(repeat model.Repeat) []span {
	return occurrenceProbes(model.Series{Repeat: repeat})
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

func repeatValuesSQL(repeat model.Repeat) string {
	return fmt.Sprintf("%s, %s, %s, %s, %s, %s",
		sqlite.Quote(repeat.Title),
		sqlite.Quote(repeat.Start.Format(model.DateTimeFormat)),
		sqlite.Quote(repeat.End.Format(model.DateTimeFormat)),
		sqlite.Quote(repeat.Freq),
		untilSQL(repeat.Until),
		storedURL(repeat.URL),
	)
}

func repeatUpdateSQL(id int, repeat model.Repeat) string {
	return fmt.Sprintf("UPDATE repeat SET title = %s, start_at = %s, end_at = %s, freq = %s, until_on = %s, url = %s WHERE id = %d;\n",
		sqlite.Quote(repeat.Title),
		sqlite.Quote(repeat.Start.Format(model.DateTimeFormat)),
		sqlite.Quote(repeat.End.Format(model.DateTimeFormat)),
		sqlite.Quote(repeat.Freq),
		untilSQL(repeat.Until),
		storedURL(repeat.URL),
		id,
	)
}

func untilSQL(until time.Time) string {
	if until.IsZero() {
		return "NULL"
	}
	return sqlite.Quote(dateOnlyLocal(until).Format(time.DateOnly))
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
