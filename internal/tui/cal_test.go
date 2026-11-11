package tui

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ryangerardwilson/crusade/internal/model"
	"github.com/ryangerardwilson/crusade/internal/store"
)

func TestCalBlocksTimeApartFromDoTasks(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SQLX_ROOT", root)
	t.Setenv("BOXES_CONFIG", filepath.Join(t.TempDir(), "missing.txt"))
	db := store.DatabasePath()
	do, err := model.NewDoEvent("2026-11-02 23:59:59", "mail the letter")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Add(db, do); err != nil {
		t.Fatal(err)
	}
	align, err := model.NewEvent("2026-11-02 23:59:59", "ship the patch", "reduce drift")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Add(db, align); err != nil {
		t.Fatal(err)
	}
	monday := dateOnly(time.Date(2026, 11, 2, 12, 0, 0, 0, time.Local))
	blocks, err := store.LoadBlocks(db)
	if err != nil {
		t.Fatal(err)
	}
	m := newCalModel(db, blocks)
	m.day = monday
	m.width = 80
	m.height = 24
	if anyOf(strings.Contains(m.View(), "mail the letter"), strings.Contains(m.View(), "ship the patch"), !strings.Contains(m.View(), "No time blocked.")) {

		t.Fatalf("empty calendar showed tasks:\n%s", m.View())
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	got, ok := next.(calModel)
	if anyOf(cmd != nil, !ok, !got.composing, !strings.Contains(got.View(), "Title"), !strings.Contains(got.View(), "Repeat")) {

		t.Fatalf("new block cmd=%v ok=%v", cmd != nil, ok)
	}
	got = typeCal(t, got, "q")
	if anyOf(!got.composing, got.composeText != "q") {

		t.Fatalf("q quit the composer text=%q composing=%v", got.composeText, got.composing)
	}
	next, cmd = got.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got, ok = next.(calModel)
	if anyOf(cmd != nil, !ok, got.composing) {

		t.Fatalf("esc cancel cmd=%v ok=%v composing=%v", cmd != nil, ok, got.composing)
	}
	next, cmd = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	got, ok = next.(calModel)
	if anyOf(cmd != nil, !ok, !got.composing, got.composeText != "") {

		t.Fatalf("second new cmd=%v text=%q", cmd != nil, got.composeText)
	}
	got = setForm(t, got, "deep work", "09:00", "10:00", "", "")
	got = keyCal(t, got, tea.KeyMsg{Type: tea.KeyEnter})
	if got.composing {
		t.Fatalf("save composing err=%s", got.errText)
	}
	view := stripANSI(got.View())
	if anyOf(!strings.Contains(view, "CAL"), !strings.Contains(view, "09:00-10:00"), !strings.Contains(view, "deep work"), strings.Contains(view, "mail the letter"), strings.Contains(view, "ship the patch")) {

		t.Fatalf("monday block:\n%s", view)
	}
	if anyOf(!strings.Contains(view, "Tue"), !strings.Contains(view, "Wed")) {

		t.Fatalf("week sheet:\n%s", view)
	}

	next, cmd = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	got, ok = next.(calModel)
	if anyOf(cmd != nil, !ok, !strings.Contains(got.View(), "Tuesday"), !strings.Contains(got.View(), "deep work"), strings.Contains(got.View(), "No time blocked.")) {

		t.Fatalf("tuesday cmd=%v\n%s", cmd != nil, got.View())
	}

	next, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	got = next.(calModel)
	next, cmd = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	got = next.(calModel)
	got = setForm(t, got, "other", "09:00", "10:00", "", "")
	got = keyCal(t, got, tea.KeyMsg{Type: tea.KeyEnter})
	if anyOf(!strings.Contains(got.errText, "already blocked"), !got.composing) {

		t.Fatalf("overlap err=%q composing=%v", got.errText, got.composing)
	}
	next, _ = got.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got = next.(calModel)
	if anyOf(got.composing, strings.Contains(got.View(), "other")) {

		t.Fatalf("cancelled overlap stayed:\n%s", got.View())
	}

	next, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	got = next.(calModel)
	got = typeCal(t, got, "reading")
	next, _ = got.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got = next.(calModel)
	view = stripANSI(got.View())
	if anyOf(!strings.Contains(view, "10:00-11:00"), !strings.Contains(view, "reading"), strings.Contains(view, "mail the letter")) {

		t.Fatalf("default hour:\n%s\nerr=%s", view, got.errText)
	}

	loaded, err := store.LoadBlocks(db)
	if anyOf(err != nil, len(loaded) != 2) {

		t.Fatalf("stored=%d err=%v", len(loaded), err)
	}
	tasks, err := store.Load(db)
	if anyOf(err != nil, len(tasks) != 2) {

		t.Fatalf("tasks changed: %d %v", len(tasks), err)
	}
}

func typeCal(t *testing.T, m calModel, text string) calModel {
	t.Helper()
	for _, r := range text {
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		if cmd != nil {
			t.Fatalf("cmd while typing %q", text)
		}
		var ok bool
		m, ok = next.(calModel)
		if !ok {
			t.Fatalf("typing left cal: %T", next)
		}
	}
	return m
}
func TestCalRepeatCanBeRenamedRescheduledAndDeleted(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SQLX_ROOT", root)
	t.Setenv("BOXES_CONFIG", filepath.Join(t.TempDir(), "missing.txt"))
	db := store.DatabasePath()
	do, err := model.NewDoEvent("2026-11-02 23:59:59", "mail the letter")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Add(db, do); err != nil {
		t.Fatal(err)
	}
	monday := dateOnly(time.Date(2026, 11, 2, 12, 0, 0, 0, time.Local))
	if monday.Weekday() != time.Monday {
		t.Fatalf("2026-11-02 is %s", monday.Weekday())
	}
	m := newCalModel(db, nil)
	m.day = monday
	m.width = 96
	m.height = 24

	m = keyCal(t, m, runeKey('n'))
	m = setForm(t, m, "standup", "09:00", "10:00", "weekly", "")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view := stripANSI(m.View())
	if anyOf(m.errText != "", !strings.Contains(view, "09:00-10:00"), !strings.Contains(view, "standup"), !strings.Contains(view, "weekly"), strings.Contains(view, "mail the letter")) {

		t.Fatalf("created series:\n%s\nerr=%s", view, m.errText)
	}

	m = keyCal(t, m, runeKey('n'))
	m = setForm(t, m, "other", "09:00", "10:00", "weekly", "")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if anyOf(!m.composing, !strings.Contains(m.errText, "already blocked")) {

		t.Fatalf("overlap err=%q composing=%v", m.errText, m.composing)
	}
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	m = keyCal(t, m, runeKey('e'))
	if anyOf(!m.composing, !strings.Contains(m.View(), "Edit block"), !strings.Contains(m.View(), "this event")) {

		t.Fatalf("edit form composing=%v\n%s", m.composing, m.View())
	}
	m = keyCal(t, m, runeKey('q'))
	if anyOf(!m.composing, !strings.Contains(m.composeText, "q")) {

		t.Fatalf("q left the form text=%q composing=%v", m.composeText, m.composing)
	}
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.composing {
		t.Fatal("esc left the form open")
	}
	m = keyCal(t, m, runeKey('d'))
	m = keyCal(t, m, runeKey('q'))
	if !strings.Contains(m.View(), "Remove this event") {
		t.Fatalf("q cleared delete:\n%s", m.View())
	}
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if strings.Contains(m.View(), "Remove this event") {
		t.Fatal("esc left the delete prompt")
	}

	m = keyCal(t, m, runeKey('l'))
	if anyOf(!strings.Contains(stripANSI(m.View()), "Tuesday"), !strings.Contains(stripANSI(m.View()), "standup"), strings.Contains(m.View(), "No time blocked.")) {

		t.Fatalf("tuesday:\n%s", m.View())
	}
	m = keyCal(t, m, runeKey('n'))
	m = setForm(t, m, "planning", "", "", "weekly", "")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view = stripANSI(m.View())
	if anyOf(!strings.Contains(view, "09:00-10:00"), !strings.Contains(view, "planning"), !strings.Contains(view, "weekly")) {

		t.Fatalf("title-only repeat:\n%s\nerr=%s", view, m.errText)
	}
	m = keyCal(t, m, runeKey('d'))
	m = keyCal(t, m, runeKey('a'))
	m = keyCal(t, m, runeKey('h'))
	view = stripANSI(m.View())
	if anyOf(!strings.Contains(view, "standup"), strings.Contains(view, "planning")) {

		t.Fatalf("monday after deleting tuesday series:\n%s", view)
	}

	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlL})
	view = stripANSI(m.View())
	if anyOf(m.day.Day() != 9, !strings.Contains(view, "standup"), !strings.Contains(view, "09:00-10:00")) {

		t.Fatalf("nov 9 before rename:\n%s", view)
	}
	m = keyCal(t, m, runeKey('e'))
	if anyOf(!m.composing, !strings.Contains(m.View(), "Title"), !strings.Contains(stripANSI(m.View()), "this event")) {

		t.Fatalf("rename this event:\n%s", m.View())
	}
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlU})
	m = typeCal(t, m, "interview")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view = stripANSI(m.View())
	if anyOf(m.day.Day() != 9, !strings.Contains(view, "interview"), strings.Contains(view, "standup"), !strings.Contains(view, "09:00-10:00"), !strings.Contains(view, "weekly")) {

		t.Fatalf("renamed one day:\n%s\nerr=%s", view, m.errText)
	}

	finishCalRepeatEdits(t, m)
}

func finishCalRepeatEdits(t *testing.T, m calModel) {
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlH})
	view := stripANSI(m.View())
	if anyOf(m.day.Day() != 2, !strings.Contains(view, "standup"), strings.Contains(view, "interview")) {

		t.Fatalf("nov 2 stayed standup:\n%s", view)
	}

	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlL})
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlL})
	if m.day.Day() != 16 {
		t.Fatalf("expected 16 Nov, got %s", m.day.Format("2006-01-02"))
	}
	m = keyCal(t, m, runeKey('r'))
	if anyOf(!m.composing, m.formField != formStart, !strings.Contains(m.View(), "Edit block"), !strings.Contains(stripANSI(m.View()), "weekly")) {

		t.Fatalf("edit form field=%d\n%s", m.formField, m.View())
	}
	m = setForm(t, m, "", "10:00", "11:00", "", "this and following")
	if anyOf(!strings.Contains(stripANSI(m.View()), "this and following"), m.formStart != "10:00", m.formEnd != "11:00") {

		t.Fatalf("following form start=%q end=%q\n%s", m.formStart, m.formEnd, m.View())
	}
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view = stripANSI(m.View())
	if anyOf(m.errText != "", m.day.Day() != 16, !strings.Contains(view, "10:00-11:00"), !strings.Contains(view, "standup"), strings.Contains(view, "09:00-10:00")) {

		t.Fatalf("split:\n%s\nerr=%s", view, m.errText)
	}

	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlH})
	view = stripANSI(m.View())
	interviewOnNine := false
	for _, line := range strings.Split(view, "\n") {
		if allOf(strings.HasPrefix(line, "09:00"), strings.Contains(line, "interview")) {

			interviewOnNine = true
		}
	}
	if anyOf(m.day.Day() != 9, !interviewOnNine, strings.Contains(view, "09:00-10:00"), strings.Contains(view, "10:00-11:00")) {

		t.Fatalf("nov 9 after split:\n%s", view)
	}
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlH})
	view = stripANSI(m.View())
	standupOnNine := false
	for _, line := range strings.Split(view, "\n") {
		if allOf(strings.HasPrefix(line, "09:00"), strings.Contains(line, "standup")) {

			standupOnNine = true
		}
	}
	if anyOf(m.day.Day() != 2, !standupOnNine, strings.Contains(view, "09:00-10:00"), strings.Contains(view, "interview")) {

		t.Fatalf("nov 2 after split:\n%s", view)
	}

	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlL})
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlL})
	m = keyCal(t, m, runeKey('d'))
	if !strings.Contains(m.View(), "Remove this event") {
		t.Fatalf("delete prompt:\n%s", m.View())
	}
	m = keyCal(t, m, runeKey('o'))
	view = stripANSI(m.View())
	if anyOf(m.day.Day() != 16, strings.Contains(view, "standup"), strings.Contains(view, "10:00-11:00"), !strings.Contains(view, "No time blocked.")) {

		t.Fatalf("removed nov 16:\n%s\nerr=%s", view, m.errText)
	}
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlL})
	view = stripANSI(m.View())
	if anyOf(m.day.Day() != 23, !strings.Contains(view, "10:00-11:00"), !strings.Contains(view, "standup")) {

		t.Fatalf("nov 23 remains:\n%s", view)
	}
	m = keyCal(t, m, runeKey('d'))
	m = keyCal(t, m, runeKey('a'))
	view = stripANSI(m.View())
	if anyOf(strings.Contains(view, "standup"), !strings.Contains(view, "No time blocked.")) {

		t.Fatalf("deleted later series:\n%s\nerr=%s", view, m.errText)
	}
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlH})
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlH})
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlH})
	view = stripANSI(m.View())
	if anyOf(m.day.Day() != 2, !strings.Contains(view, "standup")) {

		t.Fatalf("nov 2 survived deleting the later series:\n%s", view)
	}
	if got := m.gridCursor().Format("15:04"); got != "10:00" {
		t.Fatalf("cursor %s", got)
	}
	m = keyCal(t, m, runeKey('k'))
	m = keyCal(t, m, runeKey('d'))
	m = keyCal(t, m, runeKey('a'))
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlL})
	view = stripANSI(m.View())
	if anyOf(m.day.Day() != 9, strings.Contains(view, "interview"), strings.Contains(view, "standup"), !strings.Contains(view, "No time blocked.")) {

		t.Fatalf("deleted the original series:\n%s\nerr=%s", view, m.errText)
	}
}
func TestCalOneOffRenameRescheduleAndDelete(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SQLX_ROOT", root)
	t.Setenv("BOXES_CONFIG", filepath.Join(t.TempDir(), "missing.txt"))
	db := store.DatabasePath()
	tuesday := dateOnly(time.Date(2026, 11, 3, 12, 0, 0, 0, time.Local))
	m := newCalModel(db, nil)
	m.day = tuesday
	m.width = 96
	m.height = 24

	m = keyCal(t, m, runeKey('n'))
	m = setForm(t, m, "weekly review", "09:00", "10:00", "", "")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view := stripANSI(m.View())
	if anyOf(strings.Contains(view, "review  weekly"), !strings.Contains(view, "weekly review"), !strings.Contains(view, "09:00-10:00")) {

		t.Fatalf("title is not a repeat:\n%s\nerr=%s", view, m.errText)
	}
	m = keyCal(t, m, runeKey('d'))
	m = keyCal(t, m, runeKey('d'))
	m = keyCal(t, m, runeKey('l'))
	m = keyCal(t, m, runeKey('h'))
	if strings.Contains(stripANSI(m.View()), "weekly review") {
		t.Fatalf("deleted review stayed:\n%s", m.View())
	}

	m = keyCal(t, m, runeKey('n'))
	m = setForm(t, m, "deep work", "09:00", "10:00", "", "")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = keyCal(t, m, runeKey('e'))
	if anyOf(!m.composing, m.composeText != "deep work", !strings.Contains(m.View(), "Title")) {

		t.Fatalf("one-off edit prefill %q\n%s", m.composeText, m.View())
	}
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlU})
	m = typeCal(t, m, "focus")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view = stripANSI(m.View())
	if anyOf(!strings.Contains(view, "focus"), strings.Contains(view, "deep work"), !strings.Contains(view, "09:00-10:00")) {

		t.Fatalf("renamed one-off:\n%s\nerr=%s", view, m.errText)
	}
	m = keyCal(t, m, runeKey('r'))
	if anyOf(!m.composing, m.formField != formStart, m.formStart != "09:00", m.formEnd != "10:00") {

		t.Fatalf("one-off times start=%q end=%q\n%s", m.formStart, m.formEnd, m.View())
	}
	m = setForm(t, m, "", "11:00", "12:00", "", "")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view = stripANSI(m.View())
	if anyOf(!strings.Contains(view, "11:00-12:00"), !strings.Contains(view, "focus"), strings.Contains(view, "09:00-10:00")) {

		t.Fatalf("rescheduled one-off:\n%s\nerr=%s", view, m.errText)
	}
	m = keyCal(t, m, runeKey('d'))
	if anyOf(m.scope != "", !strings.Contains(m.status, "Press d again")) {

		t.Fatalf("one-off delete is two steps: scope=%q status=%q", m.scope, m.status)
	}
	m = keyCal(t, m, runeKey('d'))
	m = keyCal(t, m, runeKey('l'))
	m = keyCal(t, m, runeKey('h'))
	if anyOf(strings.Contains(stripANSI(m.View()), "focus"), !strings.Contains(m.View(), "No time blocked.")) {

		t.Fatalf("deleted one-off:\n%s", m.View())
	}

	m = keyCal(t, m, runeKey('n'))
	m = setForm(t, m, "deep work", "14:00", "15:00", "", "")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = keyCal(t, m, runeKey('r'))
	m = setForm(t, m, "", "", "", "weekly", "")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view = stripANSI(m.View())
	if anyOf(!strings.Contains(view, "weekly"), !strings.Contains(view, "deep work"), !strings.Contains(view, "14:00-15:00")) {

		t.Fatalf("one-off became a series:\n%s\nerr=%s", view, m.errText)
	}
	blocks, err := store.LoadBlocks(db)
	if anyOf(err != nil, len(blocks) != 0) {

		t.Fatalf("block remained: %#v %v", blocks, err)
	}
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlL})
	view = stripANSI(m.View())
	if anyOf(m.day.Day() != 10, !strings.Contains(view, "deep work"), !strings.Contains(view, "weekly")) {

		t.Fatalf("following week:\n%s", view)
	}
}

func TestCalGridBlocksTheSelectedSpan(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SQLX_ROOT", root)
	t.Setenv("BOXES_CONFIG", filepath.Join(t.TempDir(), "missing.txt"))
	db := store.DatabasePath()
	monday := dateOnly(time.Date(2026, 11, 2, 12, 0, 0, 0, time.Local))
	m := newCalModel(db, nil)
	m.day = monday
	m.width = 96
	m.height = 24

	if !m.gridCursor().Equal(clockOn(monday, 9, 0)) {
		t.Fatalf("cursor %s", m.gridCursor().Format("15:04"))
	}
	m.cursor = clockOn(monday, 19, 30)
	m = keyCal(t, m, runeKey('j'))
	evening := stripANSI(m.View())
	if anyOf(!m.gridCursor().Equal(clockOn(monday, 20, 0)), !strings.Contains(evening, "20:00")) {

		t.Fatalf("evening %s\n%s", m.gridCursor().Format("15:04"), evening)
	}
	m.cursor = clockOn(monday, 23, 0)
	m = keyCal(t, m, runeKey('j'))
	endOfDay := stripANSI(m.View())
	if anyOf(!m.gridCursor().Equal(clockOn(monday, 23, 0)), !strings.Contains(endOfDay, "23:00"), strings.Contains(endOfDay, "\n00:00")) {

		t.Fatalf("end of day %s\n%s", m.gridCursor().Format("15:04"), endOfDay)
	}
	m.cursor = clockOn(monday, 0, 0)
	m = keyCal(t, m, runeKey('k'))
	if !m.gridCursor().Equal(clockOn(monday, 0, 0)) {
		t.Fatalf("grid wrapped %s", m.gridCursor().Format("15:04"))
	}
	m.cursor = time.Time{}
	if !m.gridCursor().Equal(clockOn(monday, 9, 0)) {
		t.Fatalf("back to nine %s", m.gridCursor().Format("15:04"))
	}
	m = keyCal(t, m, runeKey('j'))
	if !m.gridCursor().Equal(clockOn(monday, 10, 0)) {
		t.Fatalf("j cursor %s", m.gridCursor().Format("15:04"))
	}
	m = keyCal(t, m, runeKey('k'))
	m = keyCal(t, m, runeKey('v'))
	m = keyCal(t, m, runeKey('j'))
	m = keyCal(t, m, runeKey('j'))
	view := stripANSI(m.View())
	if anyOf(!m.visual, !strings.Contains(view, "09:00-12:00")) {

		t.Fatalf("span:\n%s", view)
	}
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.visual {
		t.Fatal("esc left the span")
	}
	m = keyCal(t, m, runeKey('k'))
	m = keyCal(t, m, runeKey('k'))
	m = keyCal(t, m, runeKey('v'))
	m = keyCal(t, m, runeKey('j'))
	m = keyCal(t, m, runeKey('j'))
	m = keyCal(t, m, runeKey('n'))
	if anyOf(m.composeText != "", m.formStart != "09:00", m.formEnd != "12:00", !strings.Contains(m.View(), "Title")) {

		t.Fatalf("block form start=%q end=%q text=%q\n%s", m.formStart, m.formEnd, m.composeText, m.View())
	}
	m = typeCal(t, m, "focus")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view = stripANSI(m.View())
	if anyOf(m.errText != "", !strings.Contains(view, "09:00-12:00"), !strings.Contains(view, "focus")) {

		t.Fatalf("saved span:\n%s\nerr=%s", view, m.errText)
	}
	labeled := false
	continued := false
	for _, line := range strings.Split(view, "\n") {
		if allOf(strings.Contains(line, "09:00"), strings.Contains(line, "focus")) {

			labeled = true
		}
		if allOf(strings.Contains(line, "10:00"), strings.Contains(line, "▌"), !strings.Contains(line, "focus")) {

			continued = true
		}
	}
	if anyOf(!labeled, !continued) {

		t.Fatalf("span was not drawn across the grid:\n%s", view)
	}
	blocks, err := store.LoadBlocks(db)
	if anyOf(err != nil, len(blocks) != 1, blocks[0].Block.Title != "focus", blocks[0].Block.Start.Format("15:04") != "09:00", blocks[0].Block.End.Format("15:04") != "12:00") {

		t.Fatalf("stored %#v %v", blocks, err)
	}

	m = keyCal(t, m, runeKey('v'))
	m = keyCal(t, m, runeKey('j'))
	m = keyCal(t, m, runeKey('j'))
	m = keyCal(t, m, runeKey('n'))
	m = typeCal(t, m, "again")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if anyOf(!m.composing, !strings.Contains(m.errText, "already blocked")) {

		t.Fatalf("overlap err=%q composing=%v", m.errText, m.composing)
	}
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	blocks, err = store.LoadBlocks(db)
	if anyOf(err != nil, len(blocks) != 1, strings.Contains(stripANSI(m.View()), "again")) {

		t.Fatalf("overlap stayed %#v %v\n%s", blocks, err, m.View())
	}

	m = keyCal(t, m, runeKey('v'))
	m = keyCal(t, m, runeKey('j'))
	m = keyCal(t, m, runeKey('n'))
	m = setForm(t, m, "deep work", "13:00", "14:00", "", "")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view = stripANSI(m.View())
	if anyOf(m.errText != "", !strings.Contains(view, "13:00-14:00"), !strings.Contains(view, "deep work")) {

		t.Fatalf("clocks override:\n%s\nerr=%s", view, m.errText)
	}

	m = keyCal(t, m, runeKey('l'))
	m = keyCal(t, m, runeKey('n'))
	m = typeCal(t, m, "later")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view = stripANSI(m.View())
	if anyOf(m.errText != "", !strings.Contains(view, "13:00-14:00"), !strings.Contains(view, "later"), strings.Contains(view, "09:00-10:00")) {

		t.Fatalf("cursor hour:\n%s\nerr=%s", view, m.errText)
	}
}

func TestCalSheetClipsACellAndFocusesTheBlock(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SQLX_ROOT", root)
	t.Setenv("BOXES_CONFIG", filepath.Join(t.TempDir(), "missing.txt"))
	db := store.DatabasePath()
	monday := dateOnly(time.Date(2026, 11, 2, 12, 0, 0, 0, time.Local))
	m := newCalModel(db, nil)
	m.day = monday
	m.width = 80
	m.height = 24

	const title = "quarterly planning review"
	m = keyCal(t, m, runeKey('n'))
	m = setForm(t, m, title, "09:00", "11:00", "", "")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view := stripANSI(m.View())
	if anyOf(m.errText != "", !strings.Contains(view, title), !strings.Contains(view, "09:00-11:00")) {

		t.Fatalf("focus:\n%s\nerr=%s", view, m.errText)
	}
	clipped, continued := false, false
	for _, line := range strings.Split(view, "\n") {
		if allOf(strings.HasPrefix(line, "09:00"), strings.Contains(line, "quarterl"), !strings.Contains(line, "planning")) {

			clipped = true
		}
		if allOf(strings.HasPrefix(line, "10:00"), strings.Contains(line, "▌"), !strings.Contains(line, "planning")) {

			continued = true
		}
	}
	if anyOf(!clipped, !continued) {

		t.Fatalf("cell clip:\n%s", view)
	}

	m = keyCal(t, m, runeKey('j'))
	view = stripANSI(m.View())
	if anyOf(!m.gridCursor().Equal(clockOn(monday, 10, 0)), !strings.Contains(view, title)) {

		t.Fatalf("still on the block:\n%s", view)
	}
	m = keyCal(t, m, runeKey('j'))
	view = stripANSI(m.View())
	if anyOf(!m.gridCursor().Equal(clockOn(monday, 11, 0)), strings.Contains(view, "09:00-11:00")) {

		t.Fatalf("left the block:\n%s", view)
	}
	stillClipped := false
	for _, line := range strings.Split(view, "\n") {
		if allOf(strings.HasPrefix(line, "09:00"), strings.Contains(line, "quarterl"), !strings.Contains(line, "planning")) {

			stillClipped = true
		}
	}
	if !stillClipped {
		t.Fatalf("scrolled cell:\n%s", view)
	}

	m = keyCal(t, m, runeKey('k'))
	m = keyCal(t, m, runeKey('l'))
	view = stripANSI(m.View())
	if anyOf(!strings.Contains(view, "Tuesday"), strings.Contains(view, "planning"), strings.Contains(view, "No time blocked.")) {

		t.Fatalf("tuesday hides the focus:\n%s", view)
	}
	mondayCell := false
	for _, line := range strings.Split(view, "\n") {
		if allOf(strings.HasPrefix(line, "09:00"), strings.Contains(line, "quarterl")) {

			mondayCell = true
		}
	}
	if !mondayCell {
		t.Fatalf("monday cell:\n%s", view)
	}
	m = keyCal(t, m, runeKey('h'))
	view = stripANSI(m.View())
	if anyOf(!strings.Contains(view, "Monday"), !strings.Contains(view, title)) {

		t.Fatalf("focus returned:\n%s", view)
	}

	m.cursor = clockOn(monday, 23, 0)
	m = keyCal(t, m, runeKey('j'))
	view = stripANSI(m.View())
	morning := false
	for _, line := range strings.Split(view, "\n") {
		if strings.HasPrefix(line, "09:00") {
			morning = true
		}
	}
	if anyOf(morning, strings.Contains(view, "planning"), !strings.Contains(view, "23:00")) {

		t.Fatalf("hour window:\n%s", view)
	}

	sunday := dateOnly(time.Date(2026, 11, 8, 12, 0, 0, 0, time.Local))
	m.day = sunday
	m.width = 40
	view = stripANSI(m.View())
	if anyOf(strings.Contains(view, "Mon 2"), strings.Contains(view, "planning"), !strings.Contains(view, "Thu 5"), !strings.Contains(view, "Sun 8"), strings.Contains(view, "No time blocked.")) {

		t.Fatalf("day window:\n%s", view)
	}
}

func TestCalMeetingURL(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SQLX_ROOT", root)
	t.Setenv("BOXES_CONFIG", filepath.Join(t.TempDir(), "missing.txt"))
	db := store.DatabasePath()
	monday := dateOnly(time.Date(2026, 11, 2, 12, 0, 0, 0, time.Local))
	m := newCalModel(db, nil)
	m.day = monday
	m.width = 96
	m.height = 24
	if err := m.reload(); err != nil {
		t.Fatal(err)
	}

	m = keyCal(t, m, runeKey('n'))
	if !strings.Contains(m.View(), "URL") {
		t.Fatalf("form has no url field:\n%s", m.View())
	}
	m = setForm(t, m, "deep work", "09:00", "10:00", "", "")
	m = setFormURL(t, m, "https://meet.example/deep")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view := stripANSI(m.View())
	if anyOf(m.errText != "", !strings.Contains(view, "deep work"), !strings.Contains(view, "https://meet.example/deep")) {

		t.Fatalf("saved url:\n%s\nerr=%s", view, m.errText)
	}
	blocks, err := store.LoadBlocks(db)
	if anyOf(err != nil, len(blocks) != 1, blocks[0].Block.URL != "https://meet.example/deep") {

		t.Fatalf("stored %#v %v", blocks, err)
	}

	m = keyCal(t, m, runeKey('e'))
	if anyOf(m.formURL != "https://meet.example/deep", !strings.Contains(m.View(), "Edit block")) {

		t.Fatalf("prefill %q\n%s", m.formURL, m.View())
	}
	m = setFormURL(t, m, "https://meet.example/focus")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view = stripANSI(m.View())
	if anyOf(strings.Contains(view, "https://meet.example/deep"), !strings.Contains(view, "https://meet.example/focus")) {

		t.Fatalf("updated url:\n%s", view)
	}

	m = keyCal(t, m, runeKey('e'))
	m = setFormURL(t, m, "")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view = stripANSI(m.View())
	if strings.Contains(view, "meet.example") {
		t.Fatalf("cleared url:\n%s", view)
	}
	blocks, err = store.LoadBlocks(db)
	if anyOf(err != nil, len(blocks) != 1, blocks[0].Block.URL != "") {

		t.Fatalf("cleared store %#v %v", blocks, err)
	}

	m = keyCal(t, m, runeKey('n'))
	m = setForm(t, m, "standup", "11:00", "12:00", "weekly", "")
	m = setFormURL(t, m, "https://meet.example/stand")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view = stripANSI(m.View())
	if anyOf(m.errText != "", !strings.Contains(view, "https://meet.example/stand"), !strings.Contains(view, "weekly")) {

		t.Fatalf("repeat url:\n%s\nerr=%s", view, m.errText)
	}
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlL})
	view = stripANSI(m.View())
	if !strings.Contains(view, "https://meet.example/stand") {
		t.Fatalf("next week url:\n%s", view)
	}
	m = keyCal(t, m, runeKey('e'))
	m = setFormURL(t, m, "https://meet.example/once")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view = stripANSI(m.View())
	if anyOf(!strings.Contains(view, "https://meet.example/once"), strings.Contains(view, "https://meet.example/stand")) {

		t.Fatalf("this event url:\n%s", view)
	}
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlH})
	view = stripANSI(m.View())
	if anyOf(!strings.Contains(view, "https://meet.example/stand"), strings.Contains(view, "https://meet.example/once")) {

		t.Fatalf("series url stayed:\n%s", view)
	}
	m = keyCal(t, m, runeKey('e'))
	m = setFormURL(t, m, "https://meet.example/next")
	m = setForm(t, m, "", "", "", "", "this and following")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view = stripANSI(m.View())
	if anyOf(m.errText != "", !strings.Contains(view, "https://meet.example/next"), strings.Contains(view, "https://meet.example/stand")) {

		t.Fatalf("following url:\n%s\nerr=%s", view, m.errText)
	}
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlL})
	view = stripANSI(m.View())
	if anyOf(!strings.Contains(view, "https://meet.example/next"), strings.Contains(view, "https://meet.example/once")) {

		t.Fatalf("following reached the override:\n%s", view)
	}
}

func TestCalOpenLinkUsesTheBrowser(t *testing.T) {
	var opened []string
	previous := launchMeetingLink
	launchMeetingLink = func(link string) error {
		opened = append(opened, link)
		return nil
	}
	t.Cleanup(func() { launchMeetingLink = previous })
	t.Setenv("CRUSADE_POPUP", "")

	root := t.TempDir()
	t.Setenv("SQLX_ROOT", root)
	t.Setenv("BOXES_CONFIG", filepath.Join(t.TempDir(), "missing.txt"))
	db := store.DatabasePath()
	monday := dateOnly(time.Date(2026, 11, 2, 12, 0, 0, 0, time.Local))
	m := newCalModel(db, nil)
	m.day = monday
	m.width = 96
	m.height = 24

	m = keyCal(t, m, runeKey('n'))
	m = keyCal(t, m, runeKey('o'))
	if anyOf(!m.composing, m.composeText != "o", len(opened) != 0) {

		t.Fatalf("o typed into the form %q opened=%v", m.composeText, opened)
	}
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	m = keyCal(t, m, runeKey('n'))
	m = setForm(t, m, "deep work", "09:00", "10:00", "", "")
	m = setFormURL(t, m, "https://meet.example/deep")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = keyCal(t, m, runeKey('o'))
	if anyOf(m.errText != "", m.status != "Opened the link.", len(opened) != 1, opened[0] != "https://meet.example/deep") {

		t.Fatalf("open status=%q err=%q opened=%v", m.status, m.errText, opened)
	}
	m = keyCal(t, m, runeKey('j'))
	m = keyCal(t, m, runeKey('o'))
	if anyOf(m.status != "No block selected to open.", len(opened) != 1) {

		t.Fatalf("empty hour status=%q opened=%v", m.status, opened)
	}
	m = keyCal(t, m, runeKey('k'))
	m = keyCal(t, m, runeKey('e'))
	m = setFormURL(t, m, "")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = keyCal(t, m, runeKey('o'))
	if anyOf(m.status != "No link on this block.", len(opened) != 1) {

		t.Fatalf("cleared link status=%q opened=%v", m.status, opened)
	}

	m = keyCal(t, m, runeKey('n'))
	m = setForm(t, m, "standup", "11:00", "12:00", "weekly", "")
	m = setFormURL(t, m, "https://meet.example/stand")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = keyCal(t, m, runeKey('o'))
	if anyOf(len(opened) != 2, opened[1] != "https://meet.example/stand") {

		t.Fatalf("series opened=%v err=%s", opened, m.errText)
	}
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlL})
	m = keyCal(t, m, runeKey('o'))
	if anyOf(m.day.Day() != 9, len(opened) != 3, opened[2] != "https://meet.example/stand") {

		t.Fatalf("next week opened=%v day=%d", opened, m.day.Day())
	}
	m = keyCal(t, m, runeKey('e'))
	m = setFormURL(t, m, "https://meet.example/once")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = keyCal(t, m, runeKey('o'))
	if anyOf(len(opened) != 4, opened[3] != "https://meet.example/once") {

		t.Fatalf("override opened=%v err=%s", opened, m.errText)
	}
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlH})
	m = keyCal(t, m, runeKey('o'))
	if anyOf(len(opened) != 5, opened[4] != "https://meet.example/stand") {

		t.Fatalf("series link stayed opened=%v", opened)
	}

	m = keyCal(t, m, runeKey('n'))
	m = setForm(t, m, "bad", "13:00", "14:00", "", "")
	m = setFormURL(t, m, "javascript:alert(1)")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = keyCal(t, m, runeKey('o'))
	if anyOf(!strings.Contains(m.errText, "cannot be opened"), len(opened) != 5) {

		t.Fatalf("bad link err=%q opened=%v", m.errText, opened)
	}

	launchMeetingLink = func(string) error { return errors.New("could not open the link") }
	m = keyCal(t, m, runeKey('k'))
	m = keyCal(t, m, runeKey('k'))
	m = keyCal(t, m, runeKey('o'))
	if anyOf(m.status != "", !strings.Contains(m.errText, "could not open the link"), len(opened) != 5) {

		t.Fatalf("launch error status=%q err=%q opened=%v", m.status, m.errText, opened)
	}

	m = keyCal(t, m, runeKey('?'))
	view := stripANSI(m.View())
	if anyOf(!strings.Contains(view, "Open the meeting link"), !strings.Contains(view, "o opens its meeting"), !strings.Contains(view, "╭")) {

		t.Fatalf("help:\n%s", view)
	}
	if !strings.HasPrefix(strings.Split(view, "\n")[0], "crusade") {
		t.Fatalf("modal replaced the calendar:\n%s", view)
	}
}

func TestCalOpenLinkClosesTheSuperCWindow(t *testing.T) {
	previous := launchMeetingLink
	launchMeetingLink = func(string) error { return nil }
	t.Cleanup(func() { launchMeetingLink = previous })
	t.Setenv("CRUSADE_POPUP", "1")

	root := t.TempDir()
	t.Setenv("SQLX_ROOT", root)
	t.Setenv("BOXES_CONFIG", filepath.Join(t.TempDir(), "missing.txt"))
	db := store.DatabasePath()
	monday := dateOnly(time.Date(2026, 11, 2, 12, 0, 0, 0, time.Local))
	m := newCalModel(db, nil)
	m.day = monday
	m.width = 96
	m.height = 24

	next, cmd := m.Update(runeKey('o'))
	if cmd != nil {
		t.Fatal("empty hour closed the window")
	}
	m = next.(calModel)
	if m.status != "No block selected to open." {
		t.Fatalf("empty hour status=%q", m.status)
	}

	m = keyCal(t, m, runeKey('n'))
	m = setForm(t, m, "deep work", "09:00", "10:00", "", "")
	m = setFormURL(t, m, "https://meet.example/deep")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	next, cmd = m.Update(runeKey('o'))
	if cmd == nil {
		t.Fatal("Super+C window stayed open")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("close command was %T", cmd())
	}
	got := next.(calModel)
	if anyOf(got.status != "Opened the link.", got.errText != "") {

		t.Fatalf("status=%q err=%q", got.status, got.errText)
	}

	launchMeetingLink = func(string) error { return errors.New("could not open the link") }
	next, cmd = got.Update(runeKey('o'))
	if cmd != nil {
		t.Fatal("failed open closed the window")
	}
	got = next.(calModel)
	if !strings.Contains(got.errText, "could not open the link") {
		t.Fatalf("err=%q", got.errText)
	}
}

func TestCalFormAcceptsClipboardPaste(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SQLX_ROOT", root)
	t.Setenv("BOXES_CONFIG", filepath.Join(t.TempDir(), "missing.txt"))
	db := store.DatabasePath()
	monday := dateOnly(time.Date(2026, 11, 2, 12, 0, 0, 0, time.Local))
	m := newCalModel(db, nil)
	m.day = monday
	m.width = 96
	m.height = 24

	m = keyCal(t, m, runeKey('n'))
	m = pasteCal(t, m, "q standup\n")
	if anyOf(!m.composing, m.composeText != "q standup", strings.Contains(m.composeText, "[")) {

		t.Fatalf("pasted title %q composing=%v", m.composeText, m.composing)
	}
	m = focusForm(t, m, formRepeat)
	before := m.composeText
	m = pasteCal(t, m, "daily")
	if anyOf(m.composeText != before, m.repeatLabel() != "none") {

		t.Fatalf("paste on repeat title=%q repeat=%q", m.composeText, m.repeatLabel())
	}
	m = focusForm(t, m, formURL)
	m = pasteCal(t, m, "https://meet.example/paste\r\n")
	if anyOf(m.formURL != "https://meet.example/paste", !m.composing) {

		t.Fatalf("pasted url %q composing=%v", m.formURL, m.composing)
	}
	m = focusForm(t, m, formStart)
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlU})
	m = pasteCal(t, m, "13:00")
	m = focusForm(t, m, formEnd)
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlU})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("14:00")})
	if cmd != nil {
		t.Fatal("burst quit")
	}
	var ok bool
	m, ok = next.(calModel)
	if anyOf(!ok, m.formEnd != "14:00") {

		t.Fatalf("burst end ok=%v end=%q", ok, m.formEnd)
	}
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view := stripANSI(m.View())
	if anyOf(m.errText != "", m.composing, !strings.Contains(view, "q standup"), !strings.Contains(view, "https://meet.example/paste"), !strings.Contains(view, "13:00-14:00")) {

		t.Fatalf("saved paste:\n%s\nerr=%s", view, m.errText)
	}
}

func TestCalDaysRepeat(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SQLX_ROOT", root)
	t.Setenv("BOXES_CONFIG", filepath.Join(t.TempDir(), "missing.txt"))
	db := store.DatabasePath()
	monday := dateOnly(time.Date(2026, 11, 2, 12, 0, 0, 0, time.Local))
	if monday.Weekday() != time.Monday {
		t.Fatalf("2026-11-02 is %s", monday.Weekday())
	}
	m := newCalModel(db, nil)
	m.day = monday
	m.width = 96
	m.height = 24

	m = keyCal(t, m, runeKey('n'))
	m = setForm(t, m, "studio", "09:00", "10:00", "days", "")
	m = focusForm(t, m, formDays)
	if anyOf(m.formDayMask != 1, m.formDayCursor != 0) {

		t.Fatalf("seed mask=%d cursor=%d", m.formDayMask, m.formDayCursor)
	}
	m = keyCal(t, m, runeKey(' '))
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if anyOf(!m.composing, !strings.Contains(m.errText, "a repeat needs a day")) {

		t.Fatalf("empty days err=%q composing=%v", m.errText, m.composing)
	}
	m = keyCal(t, m, runeKey(' '))
	m = keyCal(t, m, runeKey('l'))
	m = keyCal(t, m, runeKey(' '))
	m = keyCal(t, m, runeKey('l'))
	m = keyCal(t, m, runeKey(' '))
	m = keyCal(t, m, runeKey('h'))
	m = keyCal(t, m, runeKey(' '))
	if anyOf(m.formDayMask != 5, m.formDayCursor != 1) {

		t.Fatalf("mask=%d cursor=%d", m.formDayMask, m.formDayCursor)
	}
	m = pasteCal(t, m, "sat")
	if anyOf(m.formDayMask != 5, m.composeText != "studio") {

		t.Fatalf("paste changed days mask=%d title=%q", m.formDayMask, m.composeText)
	}
	days := formRow(t, stripANSI(m.View()), "Days")
	if anyOf(!strings.Contains(days, "Mon"), !strings.Contains(days, "Wed"), !strings.Contains(days, "tue"), strings.Contains(days, "Tue")) {

		t.Fatalf("days row %q", days)
	}
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view := stripANSI(m.View())
	if anyOf(m.errText != "", m.composing, !strings.Contains(view, "09:00-10:00  studio  Mon Wed")) {

		t.Fatalf("saved days:\n%s\nerr=%s", view, m.errText)
	}
	cells := sheetCells(t, view, "09:00", 12)
	if anyOf(len(cells) != 7, !strings.Contains(cells[0], "studio"), strings.TrimSpace(cells[1]) != "", !strings.Contains(cells[2], "studio")) {

		t.Fatalf("monday row %#v", cells)
	}

	m = keyCal(t, m, runeKey('e'))
	if anyOf(!m.composing, !strings.Contains(m.View(), "Edit block")) {

		t.Fatalf("edit composing=%v\n%s", m.composing, m.View())
	}
	days = formRow(t, stripANSI(m.View()), "Days")
	if anyOf(!strings.Contains(days, "Mon"), !strings.Contains(days, "Wed"), strings.Contains(days, "Tue")) {

		t.Fatalf("edit days %q", days)
	}
	m = focusForm(t, m, formDays)
	m = keyCal(t, m, runeKey(' '))
	m = focusForm(t, m, formApply)
	m = chooseForm(t, m, "all events")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if anyOf(!m.composing, !strings.Contains(m.errText, "does not fall")) {

		t.Fatalf("anchor day err=%q composing=%v", m.errText, m.composing)
	}
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	m = keyCal(t, m, runeKey('l'))
	view = stripANSI(m.View())
	if anyOf(strings.Contains(view, "09:00-10:00"), strings.Contains(view, "Mon Wed")) {

		t.Fatalf("tuesday focus:\n%s", view)
	}
	cells = sheetCells(t, view, "09:00", 12)
	if anyOf(!strings.Contains(cells[0], "studio"), strings.TrimSpace(cells[1]) != "", !strings.Contains(cells[2], "studio")) {

		t.Fatalf("tuesday row %#v", cells)
	}
	m = keyCal(t, m, runeKey('l'))
	view = stripANSI(m.View())
	if anyOf(!strings.Contains(view, "studio"), !strings.Contains(view, "Mon Wed")) {

		t.Fatalf("wednesday focus:\n%s", view)
	}

	m = keyCal(t, m, runeKey('n'))
	m = setForm(t, m, "clash", "09:00", "10:00", "", "")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if anyOf(!m.composing, !strings.Contains(m.errText, "already blocked")) {

		t.Fatalf("wednesday overlap err=%q composing=%v", m.errText, m.composing)
	}
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	m = keyCal(t, m, runeKey('h'))
	m = keyCal(t, m, runeKey('n'))
	m = setForm(t, m, "open", "09:00", "10:00", "", "")
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if anyOf(m.errText != "", m.composing, !strings.Contains(stripANSI(m.View()), "open")) {

		t.Fatalf("tuesday one-off:\n%s\nerr=%s", m.View(), m.errText)
	}

	stored, err := store.LoadRepeats(db)
	if anyOf(err != nil, len(stored) != 1, stored[0].Repeat.Title != "studio", stored[0].Repeat.Freq != "mon,wed") {

		t.Fatalf("stored %#v %v", stored, err)
	}
}

func formRow(t *testing.T, view, label string) string {
	t.Helper()
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, label+" ") {
			return line
		}
	}
	t.Fatalf("no %s row:\n%s", label, view)
	return ""
}

func sheetCells(t *testing.T, view, clock string, width int) []string {
	t.Helper()
	var row string
	for _, line := range strings.Split(view, "\n") {
		if strings.HasPrefix(line, clock) {
			row = line
			break
		}
	}
	if row == "" {
		t.Fatalf("no %s row:\n%s", clock, view)
	}
	rest := []rune(row)
	if len(rest) < 6 {
		t.Fatalf("%s row %q", clock, row)
	}
	rest = rest[6:]
	if len(rest)%width != 0 {
		t.Fatalf("%s width %d %q", clock, len(rest), row)
	}
	cells := make([]string, 0, len(rest)/width)
	for i := 0; i < len(rest); i += width {
		cells = append(cells, string(rest[i:i+width]))
	}
	return cells
}

func pasteCal(t *testing.T, m calModel, text string) calModel {
	t.Helper()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text), Paste: true})
	if cmd != nil {
		t.Fatal("paste quit")
	}
	got, ok := next.(calModel)
	if !ok {
		t.Fatalf("paste left cal: %T", next)
	}
	return got
}

func focusForm(t *testing.T, m calModel, field int) calModel {
	t.Helper()
	for i := 0; i < 8 && m.formField != field; i++ {
		m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyTab})
	}
	if m.formField != field {
		t.Fatalf("form field %d", m.formField)
	}
	return m
}

func chooseForm(t *testing.T, m calModel, want string) calModel {
	t.Helper()
	for i := 0; i < 6; i++ {
		if allOf(m.formField == formRepeat, m.repeatLabel() == want) {

			return m
		}
		if allOf(m.formField == formApply, m.applyLabel() == want) {

			return m
		}
		m = keyCal(t, m, runeKey('l'))
	}
	t.Fatalf("form choice %q", want)
	return m
}

func setForm(t *testing.T, m calModel, title, start, end, repeat, apply string) calModel {
	t.Helper()
	if title != "" {
		m = focusForm(t, m, formTitle)
		m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlU})
		m = typeCal(t, m, title)
	}
	if start != "" {
		m = focusForm(t, m, formStart)
		m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlU})
		m = typeCal(t, m, start)
	}
	if end != "" {
		m = focusForm(t, m, formEnd)
		m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlU})
		m = typeCal(t, m, end)
	}
	if repeat != "" {
		m = focusForm(t, m, formRepeat)
		m = chooseForm(t, m, repeat)
	}
	if apply != "" {
		m = focusForm(t, m, formApply)
		m = chooseForm(t, m, apply)
	}
	return m
}

func setFormURL(t *testing.T, m calModel, url string) calModel {
	t.Helper()
	m = focusForm(t, m, formURL)
	m = keyCal(t, m, tea.KeyMsg{Type: tea.KeyCtrlU})
	if url != "" {
		m = typeCal(t, m, url)
	}
	return m
}

func keyCal(t *testing.T, m calModel, msg tea.KeyMsg) calModel {
	t.Helper()
	next, cmd := m.Update(msg)
	if cmd != nil {
		t.Fatalf("quit on %s", msg.String())
	}
	got, ok := next.(calModel)
	if !ok {
		t.Fatalf("left cal on %s: %T", msg.String(), next)
	}
	return got
}

func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}
