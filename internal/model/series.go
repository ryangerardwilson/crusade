package model

import (
	"fmt"
	"strings"
	"time"
)

// RepeatOverride is one day of a series that differs from the series. An
// empty Title or URL, or a zero Start and End, means that day uses the series
// value.
type RepeatOverride struct {
	Day   time.Time
	Title string
	Start time.Time
	End   time.Time
	URL   string
}

// HasClock reports that the day moves off the series clock.
func (o RepeatOverride) HasClock() bool {
	return !o.Start.IsZero()
}

// Series is a repeat with its skipped and overridden days.
type Series struct {
	Repeat    Repeat
	Skips     []time.Time
	Overrides []RepeatOverride
}

// Occurrence is one day of a series as it shows.
type Occurrence struct {
	Day        time.Time
	Title      string
	Start      time.Time
	End        time.Time
	URL        string
	Overridden bool
}

// Skipped reports that the series does not happen on day.
func (s Series) Skipped(day time.Time) bool {
	for _, skip := range s.Skips {
		if calendarDay(skip).Equal(calendarDay(day)) {
			return true
		}
	}
	return false
}

// OverrideOn is the override of day.
func (s Series) OverrideOn(day time.Time) (RepeatOverride, bool) {
	for _, override := range s.Overrides {
		if calendarDay(override.Day).Equal(calendarDay(day)) {
			return override, true
		}
	}
	return RepeatOverride{}, false
}

// On is the series on day. It is false when the series does not fall on day
// or that day is skipped.
func (s Series) On(day time.Time) (Occurrence, bool) {
	day = calendarDay(day)
	if !s.Repeat.OccursOn(day) || s.Skipped(day) {
		return Occurrence{}, false
	}
	start, end := s.Repeat.SpanOn(day)
	occurrence := Occurrence{Day: day, Title: s.Repeat.Title, Start: start, End: end, URL: s.Repeat.URL}
	override, ok := s.OverrideOn(day)
	if !ok {
		return occurrence, true
	}
	occurrence.Overridden = true
	if override.Title != "" {
		occurrence.Title = override.Title
	}
	if override.HasClock() {
		occurrence.Start, occurrence.End = override.Start, override.End
	}
	if override.URL != "" {
		occurrence.URL = override.URL
	}
	return occurrence, true
}

// OverrideFor is the override that makes day show title, start, end, and url.
// It holds only the values that differ from the series, and is false when
// none do.
func (s Series) OverrideFor(day time.Time, title string, start, end time.Time, url string) (RepeatOverride, bool) {
	day = calendarDay(day)
	override := RepeatOverride{Day: day}
	if title != s.Repeat.Title {
		override.Title = title
	}
	seriesStart, seriesEnd := s.Repeat.SpanOn(day)
	if !start.Equal(seriesStart) || !end.Equal(seriesEnd) {
		override.Start, override.End = start, end
	}
	if url != s.Repeat.URL {
		override.URL = url
	}
	keep := override.Title != "" || override.HasClock() || override.URL != ""
	return override, keep
}

// Scope is how much of a series an edit of one day changes.
type Scope int

const (
	ScopeThis Scope = iota
	ScopeFollowing
	ScopeAll
)

// CalForm is what the calendar form holds. Freq is empty for a one-off block.
type CalForm struct {
	Title       string
	StartHour   int
	StartMinute int
	EndHour     int
	EndMinute   int
	Freq        string
	URL         string
	Scope       Scope
}

// Times places the form clock on day. An end of 00:00 is the next midnight.
func (f CalForm) Times(day time.Time) (time.Time, time.Time) {
	day = calendarDay(day)
	start := time.Date(day.Year(), day.Month(), day.Day(), f.StartHour, f.StartMinute, 0, 0, time.Local)
	end := time.Date(day.Year(), day.Month(), day.Day(), f.EndHour, f.EndMinute, 0, 0, time.Local)
	if !end.After(start) && f.EndHour == 0 {
		end = end.AddDate(0, 0, 1)
	}
	return start, end
}

func (f CalForm) title() (string, error) {
	title := strings.TrimSpace(f.Title)
	if title == "" {
		return "", fmt.Errorf("a block needs a title")
	}
	return title, nil
}

// CalChange is what a calendar save did, for the status line.
type CalChange int

const (
	ChangeSaved CalChange = iota
	ChangeRenamed
	ChangeRescheduled
	ChangeLink
	ChangeBlocked
	ChangeRepeats
)

// NewCalEntry is the block or series a new form on day makes. A new clock is
// on the hour.
func NewCalEntry(day time.Time, form CalForm) (Block, Repeat, bool, error) {
	title, err := form.title()
	if err != nil {
		return Block{}, Repeat{}, false, err
	}
	start, end := form.Times(day)
	if err := (Block{Start: start, End: end}).OnTheHour(); err != nil {
		return Block{}, Repeat{}, false, err
	}
	if form.Freq != "" {
		repeat, err := NewRepeat(title, start.Format(DateTimeFormat), end.Format(DateTimeFormat), form.Freq)
		repeat.URL = strings.TrimSpace(form.URL)
		return Block{}, repeat, true, err
	}
	block, err := NewBlock(title, start.Format(DateTimeFormat), end.Format(DateTimeFormat))
	block.URL = strings.TrimSpace(form.URL)
	return block, Repeat{}, false, err
}

// BlockEdit is a saved block form: the block, or the series it becomes.
type BlockEdit struct {
	Block      Block
	Repeat     Repeat
	MakeRepeat bool
	Change     CalChange
}

// EditBlock applies form to current. An unchanged clock may stay off the
// hour. A moved clock and a new series are on the hour.
func EditBlock(current Block, form CalForm) (BlockEdit, error) {
	title, err := form.title()
	if err != nil {
		return BlockEdit{}, err
	}
	start, end := form.Times(current.Start)
	moved := !start.Equal(current.Start) || !end.Equal(current.End)
	if moved || form.Freq != "" {
		if err := (Block{Start: start, End: end}).OnTheHour(); err != nil {
			return BlockEdit{}, err
		}
	}
	url := strings.TrimSpace(form.URL)
	if form.Freq != "" {
		repeat, err := NewRepeat(title, start.Format(DateTimeFormat), end.Format(DateTimeFormat), form.Freq)
		repeat.URL = url
		return BlockEdit{Repeat: repeat, MakeRepeat: true, Change: ChangeRepeats}, err
	}
	block, err := NewBlock(title, start.Format(DateTimeFormat), end.Format(DateTimeFormat))
	if err != nil {
		return BlockEdit{}, err
	}
	block.URL = url
	change := ChangeSaved
	if moved {
		change = ChangeRescheduled
	} else if title != current.Title {
		change = ChangeRenamed
	}
	return BlockEdit{Block: block, Change: change}, nil
}

// RepeatAction is the write a series edit needs.
type RepeatAction int

const (
	// ActSeriesURL makes the url the link of every day.
	ActSeriesURL RepeatAction = iota
	// ActOverride changes one day.
	ActOverride
	// ActContinue changes the day and the days after it.
	ActContinue
	// ActUpdateAll renames or relinks every day and keeps each day's clock.
	ActUpdateAll
	// ActReplaceAll moves every day to the new clock or frequency and drops
	// the overridden days. Skipped days stay skipped.
	ActReplaceAll
)

// RepeatEdit is how a series form save lands.
type RepeatEdit struct {
	Action RepeatAction
	Day    time.Time
	// Next is the series after the save.
	Next Repeat
	// Override is the day's override for ActOverride. KeepOverride is false
	// when the day no longer differs from the series.
	Override     RepeatOverride
	KeepOverride bool
	// ApplyURL: the url changed, so it is the link of every day.
	ApplyURL bool
	// RenameDays: the title changed for the scope, so overridden days in it
	// drop their own title.
	RenameDays bool
	// ClearDays: the clock or frequency changed, so overridden days in the
	// scope are dropped.
	ClearDays bool
	Change    CalChange
}

type repeatDiff struct {
	occurrence   Occurrence
	title        string
	start, end   time.Time
	url          string
	titleChanged bool
	timesChanged bool
	freqChanged  bool
	urlChanged   bool
}

// EditRepeat decides how a form saved on one day of s lands: this event, this
// and following, or all events. Title, time, and frequency stay in the scope.
// A url change is the link of every day. An unchanged clock may stay off the
// hour; a moved clock is on the hour.
func (s Series) EditRepeat(day time.Time, form CalForm) (RepeatEdit, error) {
	diff, err := s.diff(day, form)
	if err != nil {
		return RepeatEdit{}, err
	}
	switch form.Scope {
	case ScopeThis:
		return s.editThis(diff)
	case ScopeFollowing:
		return s.editFollowing(diff, form.Freq)
	default:
		return s.editAll(diff, form.Freq)
	}
}

func (s Series) diff(day time.Time, form CalForm) (repeatDiff, error) {
	occurrence, ok := s.On(day)
	if !ok {
		return repeatDiff{}, fmt.Errorf("that event is not on this day")
	}
	title, err := form.title()
	if err != nil {
		return repeatDiff{}, err
	}
	start, end := form.Times(occurrence.Day)
	url := strings.TrimSpace(form.URL)
	freq, _ := ParseFreq(form.Freq)
	diff := repeatDiff{
		occurrence:   occurrence,
		title:        title,
		start:        start,
		end:          end,
		url:          url,
		titleChanged: title != occurrence.Title,
		timesChanged: !start.Equal(occurrence.Start) || !end.Equal(occurrence.End),
		freqChanged:  freq != s.Repeat.Freq,
		urlChanged:   url != occurrence.URL,
	}
	if diff.timesChanged {
		if err := (Block{Start: start, End: end}).OnTheHour(); err != nil {
			return repeatDiff{}, err
		}
	}
	return diff, nil
}

func (s Series) seriesURL(diff repeatDiff) string {
	if diff.urlChanged {
		return diff.url
	}
	return s.Repeat.URL
}

func (s Series) seriesTitle(diff repeatDiff) string {
	if diff.titleChanged {
		return diff.title
	}
	return s.Repeat.Title
}

func (s Series) editThis(diff repeatDiff) (RepeatEdit, error) {
	if diff.freqChanged {
		return RepeatEdit{}, fmt.Errorf("a repeat change applies to this and following, or to all events")
	}
	next := s.Repeat
	next.URL = s.seriesURL(diff)
	day := diff.occurrence.Day
	if diff.urlChanged && !diff.titleChanged && !diff.timesChanged {
		return RepeatEdit{Action: ActSeriesURL, Day: day, Next: next, ApplyURL: true, Change: ChangeLink}, nil
	}
	block, err := NewBlock(diff.title, diff.start.Format(DateTimeFormat), diff.end.Format(DateTimeFormat))
	if err != nil {
		return RepeatEdit{}, err
	}
	if !calendarDay(block.Start).Equal(day) {
		return RepeatEdit{}, fmt.Errorf("a block stays on one day")
	}
	dayURL := diff.occurrence.URL
	if diff.urlChanged {
		dayURL = next.URL
	}
	relinked := Series{Repeat: next}
	override, keep := relinked.OverrideFor(day, block.Title, block.Start, block.End, dayURL)
	change := ChangeSaved
	switch {
	case diff.titleChanged && !diff.timesChanged:
		change = ChangeRenamed
	case diff.timesChanged && !diff.titleChanged:
		change = ChangeRescheduled
	case diff.urlChanged:
		change = ChangeLink
	}
	return RepeatEdit{Action: ActOverride, Day: day, Next: next, Override: override, KeepOverride: keep, ApplyURL: diff.urlChanged, Change: change}, nil
}

func (s Series) editFollowing(diff repeatDiff, freq string) (RepeatEdit, error) {
	day := diff.occurrence.Day
	clear := diff.timesChanged || diff.freqChanged
	start, end := diff.start, diff.end
	if !clear {
		start, end = s.Repeat.SpanOn(day)
		freq = s.Repeat.Freq
	}
	next, err := NewRepeat(s.seriesTitle(diff), start.Format(DateTimeFormat), end.Format(DateTimeFormat), freq)
	if err != nil {
		return RepeatEdit{}, err
	}
	next.URL = s.seriesURL(diff)
	change := ChangeSaved
	if diff.titleChanged && !clear {
		change = ChangeRenamed
	} else if !diff.titleChanged && clear {
		change = ChangeRescheduled
	}
	return RepeatEdit{Action: ActContinue, Day: day, Next: next, ApplyURL: diff.urlChanged, RenameDays: diff.titleChanged, ClearDays: clear, Change: change}, nil
}

func (s Series) editAll(diff repeatDiff, freq string) (RepeatEdit, error) {
	day := diff.occurrence.Day
	if !diff.timesChanged && !diff.freqChanged {
		next := s.Repeat
		next.Title = s.seriesTitle(diff)
		next.URL = s.seriesURL(diff)
		change := ChangeSaved
		if diff.titleChanged && !diff.urlChanged {
			change = ChangeRenamed
		}
		return RepeatEdit{Action: ActUpdateAll, Day: day, Next: next, ApplyURL: diff.urlChanged, RenameDays: diff.titleChanged, Change: change}, nil
	}
	first := calendarDay(s.Repeat.Start)
	start := time.Date(first.Year(), first.Month(), first.Day(), diff.start.Hour(), diff.start.Minute(), 0, 0, time.Local)
	end := start.Add(diff.end.Sub(diff.start))
	next, err := NewRepeat(s.seriesTitle(diff), start.Format(DateTimeFormat), end.Format(DateTimeFormat), freq)
	if err != nil {
		return RepeatEdit{}, err
	}
	next.Until = s.Repeat.Until
	next.URL = s.seriesURL(diff)
	change := ChangeRescheduled
	if diff.titleChanged {
		change = ChangeSaved
	}
	return RepeatEdit{Action: ActReplaceAll, Day: day, Next: next, ApplyURL: diff.urlChanged, ClearDays: true, Change: change}, nil
}

// RepeatDelete is how removing one day of a series lands.
type RepeatDelete struct {
	// Skip removes only Day.
	Skip bool
	// Until ends the series on that day. It is zero when Skip or Whole is set.
	Until time.Time
	// Whole deletes the series and all its days.
	Whole bool
	Day   time.Time
}

// DeleteDay decides what removing day of s with scope deletes. This and
// following from the first day is the whole series.
func (s Series) DeleteDay(day time.Time, scope Scope) RepeatDelete {
	day = calendarDay(day)
	switch {
	case scope == ScopeThis:
		return RepeatDelete{Skip: true, Day: day}
	case scope == ScopeFollowing && day.After(calendarDay(s.Repeat.Start)):
		return RepeatDelete{Until: day.AddDate(0, 0, -1), Day: day}
	default:
		return RepeatDelete{Whole: true, Day: day}
	}
}
