package model

import (
	"strings"
	"testing"
	"time"
)

func seriesDay(day int) time.Time {
	return time.Date(2026, 11, day, 0, 0, 0, 0, time.Local)
}

func evening(t *testing.T) Series {
	t.Helper()
	repeat, err := NewRepeat("standup", "2026-11-02 19:30:00", "2026-11-02 20:30:00", FreqDaily)
	if err != nil {
		t.Fatal(err)
	}
	repeat.URL = "https://meet/a"
	return Series{Repeat: repeat}
}

func TestSeriesOnSkipAndOverride(t *testing.T) {
	s := evening(t)
	moved := time.Date(2026, 11, 5, 9, 0, 0, 0, time.Local)
	s.Skips = []time.Time{seriesDay(4)}
	s.Overrides = []RepeatOverride{
		{Day: seriesDay(5), Start: moved, End: moved.Add(time.Hour)},
		{Day: seriesDay(6), Title: "retro", URL: "https://meet/b"},
	}
	if _, ok := s.On(seriesDay(1)); ok {
		t.Fatal("on before the first day")
	}
	if _, ok := s.On(seriesDay(4)); ok {
		t.Fatal("skipped day shows")
	}
	plain, ok := s.On(seriesDay(3))
	if !ok || plain.Overridden || plain.Title != "standup" || plain.Start.Format("15:04") != "19:30" || plain.URL != "https://meet/a" {
		t.Fatalf("plain = %#v", plain)
	}
	clock, _ := s.On(seriesDay(5))
	if !clock.Overridden || clock.Title != "standup" || !clock.Start.Equal(moved) || clock.URL != "https://meet/a" {
		t.Fatalf("clock = %#v", clock)
	}
	renamed, _ := s.On(seriesDay(6))
	if !renamed.Overridden || renamed.Title != "retro" || renamed.URL != "https://meet/b" || renamed.Start.Format("15:04") != "19:30" {
		t.Fatalf("renamed = %#v", renamed)
	}
}

func TestSeriesOverrideFor(t *testing.T) {
	s := evening(t)
	day := seriesDay(5)
	start, end := s.Repeat.SpanOn(day)
	if _, keep := s.OverrideFor(day, "standup", start, end, "https://meet/a"); keep {
		t.Fatal("unchanged day kept an override")
	}
	override, keep := s.OverrideFor(day, "retro", start, end, "https://meet/a")
	if !keep || override.Title != "retro" || override.HasClock() || override.URL != "" || !override.Day.Equal(day) {
		t.Fatalf("title override = %#v", override)
	}
	override, keep = s.OverrideFor(day, "standup", start.Add(time.Hour), end.Add(time.Hour), "https://meet/b")
	if !keep || override.Title != "" || !override.Start.Equal(start.Add(time.Hour)) || override.URL != "https://meet/b" {
		t.Fatalf("clock override = %#v", override)
	}
}

func eveningForm(scope Scope) CalForm {
	return CalForm{Title: "standup", StartHour: 19, StartMinute: 30, EndHour: 20, EndMinute: 30, Freq: FreqDaily, URL: "https://meet/new", Scope: scope}
}

func TestEditRepeatURLKeepsOffHourClock(t *testing.T) {
	s := evening(t)
	for _, scope := range []Scope{ScopeThis, ScopeFollowing, ScopeAll} {
		edit, err := s.EditRepeat(seriesDay(5), eveningForm(scope))
		if err != nil {
			t.Fatalf("scope %d: %v", scope, err)
		}
		if !edit.ApplyURL || edit.Next.URL != "https://meet/new" || edit.Next.Title != "standup" {
			t.Fatalf("scope %d: edit = %#v", scope, edit)
		}
		if edit.Next.Start.Format("15:04") != "19:30" || edit.Next.End.Format("15:04") != "20:30" || edit.ClearDays {
			t.Fatalf("scope %d: clock = %#v", scope, edit)
		}
	}
	this, _ := s.EditRepeat(seriesDay(5), eveningForm(ScopeThis))
	if this.Action != ActSeriesURL || this.Change != ChangeLink {
		t.Fatalf("this = %#v", this)
	}
	all, _ := s.EditRepeat(seriesDay(5), eveningForm(ScopeAll))
	if all.Action != ActUpdateAll || !all.Next.Start.Equal(s.Repeat.Start) {
		t.Fatalf("all = %#v", all)
	}
}

func TestEditRepeatMovedClockOnTheHour(t *testing.T) {
	s := evening(t)
	form := eveningForm(ScopeAll)
	form.StartMinute, form.EndMinute = 45, 45
	if _, err := s.EditRepeat(seriesDay(5), form); err == nil || !strings.Contains(err.Error(), "on the hour") {
		t.Fatalf("moved err = %v", err)
	}
	form = eveningForm(ScopeThis)
	form.StartHour, form.StartMinute, form.EndHour, form.EndMinute = 9, 0, 10, 0
	edit, err := s.EditRepeat(seriesDay(5), form)
	if err != nil || edit.Action != ActOverride || !edit.KeepOverride || !edit.Override.HasClock() || edit.Override.Title != "" {
		t.Fatalf("this move = %#v err=%v", edit, err)
	}
}

func TestEditRepeatThisFreqChangeErrors(t *testing.T) {
	s := evening(t)
	form := eveningForm(ScopeThis)
	form.Freq = FreqWeekdays
	if _, err := s.EditRepeat(seriesDay(5), form); err == nil {
		t.Fatal("freq change on this event accepted")
	}
	s.Skips = []time.Time{seriesDay(5)}
	if _, err := s.EditRepeat(seriesDay(5), eveningForm(ScopeAll)); err == nil {
		t.Fatal("edit of a skipped day accepted")
	}
}

func TestSeriesDeleteDay(t *testing.T) {
	s := evening(t)
	if got := s.DeleteDay(seriesDay(5), ScopeThis); !got.Skip || got.Whole || !got.Day.Equal(seriesDay(5)) {
		t.Fatalf("this = %#v", got)
	}
	if got := s.DeleteDay(seriesDay(5), ScopeFollowing); got.Skip || got.Whole || !got.Until.Equal(seriesDay(4)) {
		t.Fatalf("following = %#v", got)
	}
	if got := s.DeleteDay(seriesDay(2), ScopeFollowing); !got.Whole || !got.Until.IsZero() {
		t.Fatalf("following from first = %#v", got)
	}
	if got := s.DeleteDay(seriesDay(9), ScopeAll); !got.Whole || got.Skip {
		t.Fatalf("all = %#v", got)
	}
}
