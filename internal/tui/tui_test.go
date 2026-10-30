package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/ryangerardwilson/crusade/internal/model"
	"github.com/ryangerardwilson/crusade/internal/store"
)

func TestAgendaRendersCheckboxList(t *testing.T) {
	m := appModel{
		view:         viewAgenda,
		width:        86,
		height:       24,
		records:      sampleRecords(t),
		agendaCursor: 0,
		agendaCol:    1,
		monthDate:    sampleDate(2026, time.June, 13),
		monthFocus:   monthFocusGrid,
	}

	out := m.View()

	for _, want := range []string{
		"crusade",
		"ALIGN",
		"selected 1/3",
		"score 8/10",
		"[ ]",
		"2026-06-13 23:59",
		"2026-06-13  Have a body fat",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("agenda output missing %q:\n%s", want, out)
		}
	}
	for _, hidden := range []string{"nsm", " outcomes ", "Increase my chances", "0.8"} {
		if strings.Contains(out, hidden) {
			t.Fatalf("agenda still shows %q:\n%s", hidden, out)
		}
	}
	if strings.Contains(out, "a month  tab bucket") {
		t.Fatalf("agenda should not render the shortcut strip:\n%s", out)
	}
}

func TestMonthViewRendersGridAndSelectedDayEvents(t *testing.T) {
	m := appModel{
		view:          viewMonth,
		width:         90,
		height:        28,
		records:       sampleRecords(t),
		monthDate:     sampleDate(2026, time.June, 13),
		monthFocus:    monthFocusGrid,
		agendaCursor:  0,
		agendaCol:     0,
		monthEventCol: 1,
	}

	out := m.View()

	for _, want := range []string{
		"crusade",
		"ALIGN",
		"Sat 2026-06-13",
		"top score 8/10",
		"June, 2026",
		"selected day",
		"Mon",
		"Sun",
		"13*1",
		"2026-06-13 outcomes (1)",
		"[ ]",
		"Have a body fat",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("month output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "a agenda  tab bucket") {
		t.Fatalf("month should not render the shortcut strip:\n%s", out)
	}
	if strings.Contains(out, "June, 2026 -") {
		t.Fatalf("month should use spacing instead of separator bars:\n%s", out)
	}
	if strings.Contains(out, "Increase my chances") {
		t.Fatalf("month list still shows impact:\n%s", out)
	}
}

func TestViewsStartAtTheTop(t *testing.T) {
	for name, view := range map[string]string{
		"agenda": viewAgenda,
		"month":  viewMonth,
	} {
		t.Run(name, func(t *testing.T) {
			m := appModel{
				view:         view,
				width:        120,
				height:       40,
				records:      sampleRecords(t),
				monthDate:    sampleDate(2026, time.June, 13),
				monthFocus:   monthFocusGrid,
				agendaCursor: 0,
				agendaCol:    1,
			}

			out := m.View()
			lines := strings.Split(out, "\n")
			firstContentLine := -1
			for idx, line := range lines {
				if strings.TrimSpace(line) != "" {
					firstContentLine = idx
					break
				}
			}
			if firstContentLine != 0 {
				t.Fatalf("content does not start at the top; first content line = %d:\n%s", firstContentLine, out)
			}
			if !strings.HasPrefix(stripANSI(lines[0]), "crusade") {
				t.Fatalf("banner is not left-aligned: %q", stripANSI(lines[0]))
			}
		})
	}
}

func TestHelpViewStartsAtTheTopLeft(t *testing.T) {
	m := appModel{
		view:       viewMonth,
		width:      120,
		height:     40,
		showHelp:   true,
		monthDate:  sampleDate(2026, time.June, 13),
		monthFocus: monthFocusGrid,
	}

	out := m.View()
	for _, want := range []string{
		"crusade",
		"QUICK GUIDE",
		"GENERAL",
		"AGENDA",
		"MONTH",
		"EXIT",
		"VIM TASK FILE",
		"Close this guide",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("structured help output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "ctrl+c / esc quit") {
		t.Fatalf("help still uses the old undifferentiated shortcut list:\n%s", out)
	}

	firstBorder := -1
	lastBorder := -1
	for _, line := range strings.Split(out, "\n") {
		if !strings.ContainsAny(line, "╭│╰") {
			continue
		}
		border := strings.IndexAny(line, "╭│╰")
		if firstBorder == -1 {
			firstBorder = border
		}
		lastBorder = border
		if border != firstBorder {
			t.Fatalf("help panel border shifted from column %d to %d:\n%s", firstBorder, border, out)
		}
	}
	if firstBorder != 0 || lastBorder != firstBorder {
		t.Fatalf("help panel does not start at the left edge:\n%s", out)
	}
}

func TestHelpPanelDoesNotPaintABackgroundFill(t *testing.T) {
	forceStyledOutput(t)
	m := appModel{view: viewMonth}

	out := m.helpView(92)
	if strings.Contains(out, "\x1b[48;") {
		t.Fatalf("help panel emitted a terminal background color:\n%q", out)
	}
}

func TestMonthNavigationAndFocus(t *testing.T) {
	m := appModel{
		view:       viewMonth,
		width:      90,
		height:     28,
		records:    sampleRecords(t),
		monthDate:  sampleDate(2026, time.June, 13),
		monthFocus: monthFocusGrid,
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = updated.(appModel)
	if !sameDate(m.monthDate, sampleDate(2026, time.June, 14)) {
		t.Fatalf("expected next day, got %s", m.monthDate.Format("2006-01-02"))
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlH})
	m = updated.(appModel)
	if !sameDate(m.monthDate, sampleDate(2026, time.May, 14)) {
		t.Fatalf("expected previous month, got %s", m.monthDate.Format("2006-01-02"))
	}

	m.monthDate = sampleDate(2026, time.June, 13)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	if m.monthFocus != monthFocusEvents {
		t.Fatalf("expected enter to focus selected day events, got %s", m.monthFocus)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	if m.monthFocus != monthFocusGrid {
		t.Fatalf("expected enter to return to grid, got %s", m.monthFocus)
	}
}

func TestMonthTodayStyleUnderlinesOnlyDayNumber(t *testing.T) {
	forceStyledOutput(t)
	if !todayStyle.GetUnderline() {
		t.Fatal("today marker should underline the day number")
	}

	cell := renderTodayCell(sampleDate(2026, time.June, 10), 0, 7)
	want := todayStyle.Render("10") + strings.Repeat(" ", 5)
	if cell != want {
		t.Fatalf("today underline should apply only to the day number:\nwant %q\ngot  %q", want, cell)
	}

	cell = renderTodayCell(sampleDate(2026, time.June, 13), 3, 7)
	want = todayStyle.Render("13") + "*3" + strings.Repeat(" ", 3)
	if cell != want {
		t.Fatalf("today underline should not include event markers or padding:\nwant %q\ngot  %q", want, cell)
	}
}

func TestMonthSelectedTodayUsesSingleHighlight(t *testing.T) {
	forceStyledOutput(t)
	day := sampleDate(2026, time.June, 10)
	cell := renderMonthDayCell(day, day, day, 0, 7)
	want := activeCellStyle.Render(pad(monthDayLabel(day, 0, 7), 7, "left"))
	if cell != want {
		t.Fatalf("selected today should use the selected-cell highlight without nested underline:\nwant %q\ngot  %q", want, cell)
	}
	if strings.Contains(cell, todayStyle.Render("10")) {
		t.Fatalf("selected today should not include nested today underline styling:\n%s", cell)
	}
}

func forceStyledOutput(t *testing.T) {
	t.Helper()
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previous)
	})
}

func TestTitleDoesNotUsePulseAnimation(t *testing.T) {
	m := appModel{
		view:       viewMonth,
		width:      90,
		height:     28,
		records:    sampleRecords(t),
		monthDate:  sampleDate(2026, time.June, 13),
		monthFocus: monthFocusGrid,
	}

	out := m.View()
	if !strings.Contains(out, "crusade") || !strings.Contains(out, "ALIGN") {
		t.Fatalf("expected stable title, got:\n%s", out)
	}
	for _, marker := range []string{"[.]", "[o]", "[O]"} {
		if strings.Contains(out, marker) {
			t.Fatalf("title should not use pulse marker %q:\n%s", marker, out)
		}
	}
}

func sampleRecords(t *testing.T) []store.Record {
	t.Helper()
	rows := []struct {
		id int
		x  string
		y  string
		z  string
		p  string
		q  string
		r  string
	}{
		{
			id: 1,
			x:  "2026-06-13 23:59:59",
			y:  "Have a body fat % of 23.5, 33 kg of muscle, and a visceral fat level of 16",
			z:  "Increase my chances of living longer to serve the Lord, and also to look great in swimwear",
			p:  "9",
			q:  "9",
			r:  "6",
		},
		{
			id: 2,
			x:  "2026-12-31 23:59:59",
			y:  "Learned open water swimming",
			z:  "So I can flex during beach vacations doing crazy stuff",
			p:  "6.5",
			q:  "6.5",
			r:  "9",
		},
		{
			id: 3,
			x:  "2027-03-01 23:59:59",
			y:  "Ship a useful thing",
			z:  "Make the operating system stronger",
			p:  "7",
			q:  "8",
			r:  "9",
		},
	}
	records := make([]store.Record, 0, len(rows))
	for _, row := range rows {
		event, err := model.NewEvent(row.x, row.y, row.z, row.p, row.q, row.r)
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, store.Record{ID: row.id, Event: event})
	}
	return records
}

func sampleDate(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.Local)
}

func sampleDoRecords(t *testing.T) []store.Record {
	t.Helper()
	event, err := model.NewDoEvent(
		"2026-06-13 23:59:59",
		"Have a body fat % of 23.5, 33 kg of muscle, and a visceral fat level of 16",
	)
	if err != nil {
		t.Fatal(err)
	}
	return []store.Record{{ID: 1, Event: event}}
}

func TestQQuitsWhenNotTyping(t *testing.T) {
	for name, m := range map[string]appModel{
		"agenda": {view: viewAgenda, mode: modeDo},
		"help":   {view: viewAgenda, mode: modeDo, showHelp: true},
		"month":  {view: viewMonth, mode: modeDo, monthDate: sampleDate(2026, time.June, 13), monthFocus: monthFocusEvents},
	} {
		t.Run(name, func(t *testing.T) {
			_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
			if !isQuitCommand(cmd) {
				t.Fatal("q did not quit")
			}
		})
	}
}

func TestQTypesWhileComposing(t *testing.T) {
	m := appModel{view: viewAgenda, mode: modeDo, composing: true, composeText: "mail", composeCursor: 4}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	got, ok := next.(appModel)
	if !ok || cmd != nil || !got.composing || got.composeText != "mailq" {
		t.Fatalf("text=%q composing=%v cmd=%v", got.composeText, got.composing, cmd != nil)
	}
}

func TestCtrlCQuits(t *testing.T) {
	m := appModel{view: viewAgenda}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !isQuitCommand(cmd) {
		t.Fatal("ctrl+c did not return tea.Quit")
	}
}

func TestEscapeQuitsFromNestedTUIState(t *testing.T) {
	for name, m := range map[string]appModel{
		"month events": {
			view:       viewMonth,
			monthDate:  sampleDate(2026, time.June, 13),
			monthFocus: monthFocusEvents,
		},
		"help": {
			view:     viewAgenda,
			showHelp: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
			if !isQuitCommand(cmd) {
				t.Fatal("esc did not return tea.Quit")
			}
		})
	}
}

func TestMonthCalendarStaysPutWhenSelectedDayHasNoTasks(t *testing.T) {
	base := appModel{
		mode:       modeDo,
		view:       viewMonth,
		width:      90,
		height:     36,
		records:    sampleDoRecords(t),
		monthFocus: monthFocusGrid,
	}
	withTasks := base
	withTasks.monthDate = sampleDate(2026, time.June, 13)
	without := base
	without.monthDate = sampleDate(2026, time.June, 14)
	taskLine := calendarHeaderLine(withTasks.View())
	emptyLine := calendarHeaderLine(without.View())
	if taskLine < 0 || taskLine != emptyLine {
		t.Fatalf("calendar header moved from line %d to %d", taskLine, emptyLine)
	}
}

func calendarHeaderLine(out string) int {
	for idx, line := range strings.Split(out, "\n") {
		plain := stripANSI(line)
		if strings.Contains(plain, "Mon") && strings.Contains(plain, "Sun") {
			return idx
		}
	}
	return -1
}

func TestEscapeLeavesDoMonthTaskList(t *testing.T) {
	m := appModel{
		mode:       modeDo,
		view:       viewMonth,
		monthDate:  sampleDate(2026, time.June, 13),
		monthFocus: monthFocusEvents,
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := next.(appModel)
	if isQuitCommand(cmd) || got.monthFocus != monthFocusGrid {
		t.Fatalf("focus=%q quit=%v", got.monthFocus, isQuitCommand(cmd))
	}
	_, cmd = got.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !isQuitCommand(cmd) {
		t.Fatal("esc on the calendar did not quit")
	}
}

func TestNewTaskOpensSharedYAMLAtSelectedDateAndReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.yaml")
	m := appModel{
		dataPath:   path,
		view:       viewMonth,
		monthDate:  sampleDate(2026, time.August, 25),
		monthFocus: monthFocusGrid,
	}

	previous := openTaskEditor
	defer func() { openTaskEditor = previous }()
	var openedPath string
	var openedID int
	var openedDraft bool
	openTaskEditor = func(path string, id int, draft bool) tea.Cmd {
		openedPath, openedID, openedDraft = path, id, draft
		return func() tea.Msg { return editorFinishedMsg{id: id, draft: draft} }
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if cmd == nil || openedPath != path || openedID != 1 || !openedDraft {
		t.Fatalf("editor request = path %q id %d draft %v", openedPath, openedID, openedDraft)
	}
	records, err := store.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Event.X.Format("2006-01-02") != "2026-08-25" {
		t.Fatalf("seeded task = %#v", records)
	}
	edited, err := records[0].Event.WithUpdates(map[string]string{"y": "edited in Vim", "z": "keeps one YAML"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(path, openedID, edited); err != nil {
		t.Fatal(err)
	}
	finished, _ := next.(appModel).Update(cmd())
	updated := finished.(appModel)
	if len(updated.records) != 1 || updated.records[0].Event.Y != "edited in Vim" || updated.status != "Saved." {
		t.Fatalf("reloaded model = %#v status=%q", updated.records, updated.status)
	}
}

func TestUnchangedNewTaskDraftIsRemovedAfterVim(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.yaml")
	m := appModel{dataPath: path, view: viewMonth, monthDate: sampleDate(2026, time.August, 25), monthFocus: monthFocusGrid}

	previous := openTaskEditor
	defer func() { openTaskEditor = previous }()
	openTaskEditor = func(_ string, id int, draft bool) tea.Cmd {
		return func() tea.Msg { return editorFinishedMsg{id: id, draft: draft} }
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	finished, _ := next.(appModel).Update(cmd())
	updated := finished.(appModel)
	records, err := store.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 || len(updated.records) != 0 || updated.status != "New task cancelled." {
		t.Fatalf("unchanged draft remained: records=%#v status=%q", records, updated.status)
	}
}

func TestEditOpensSelectedStableIDInSharedYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.yaml")
	records := sampleRecords(t)
	if err := store.Save(path, records); err != nil {
		t.Fatal(err)
	}
	m := appModel{
		dataPath:         path,
		records:          records,
		view:             viewMonth,
		monthDate:        sampleDate(2026, time.December, 31),
		monthFocus:       monthFocusEvents,
		monthEventCursor: 0,
	}

	previous := openTaskEditor
	defer func() { openTaskEditor = previous }()
	var openedID int
	var openedDraft bool
	openTaskEditor = func(_ string, id int, draft bool) tea.Cmd {
		openedID, openedDraft = id, draft
		return func() tea.Msg { return editorFinishedMsg{id: id, draft: draft} }
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if cmd == nil || openedID != 2 || openedDraft {
		t.Fatalf("edit request = id %d draft %v", openedID, openedDraft)
	}
}

func TestAppOpensInDoAgenda(t *testing.T) {
	m := newAppModel(filepath.Join(t.TempDir(), "tasks.yaml"), nil)
	if m.mode != modeDo || m.view != viewAgenda {
		t.Fatalf("mode=%q view=%q", m.mode, m.view)
	}
}

func TestDoModeAddsTaskForTodayOrSelectedMonthDay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.yaml")
	m := newAppModel(path, nil)
	m.view = viewAgenda
	m.composeText = "ship the note"
	m.composing = true
	m.commitCompose()

	records, err := store.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d", len(records))
	}
	if records[0].Event.Y != "ship the note" || records[0].Event.Mode != model.ModeDo || records[0].Event.Z != "" {
		t.Fatalf("saved %+v", records[0].Event)
	}
	if !sameDate(records[0].Event.X, dateOnly(time.Now())) {
		t.Fatalf("agenda task date = %s", records[0].Event.X)
	}

	m.view = viewMonth
	m.monthDate = sampleDate(2026, time.March, 2)
	m.composeText = "march review"
	m.composing = true
	m.commitCompose()
	records, err = store.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	var found store.Record
	for _, record := range records {
		if record.Event.Y == "march review" {
			found = record
		}
	}
	if found.ID == 0 || found.Event.Mode != model.ModeDo || found.Event.Z != "" || !sameDate(found.Event.X, sampleDate(2026, time.March, 2)) {
		t.Fatalf("month task = %+v", found.Event)
	}
}

func TestDoModeDoesNotOpenVim(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.yaml")
	m := newAppModel(path, nil)
	previous := openTaskEditor
	defer func() { openTaskEditor = previous }()
	opened := false
	openTaskEditor = func(string, int, bool) tea.Cmd {
		opened = true
		return nil
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	got := next.(appModel)
	if opened || cmd != nil || !got.composing {
		t.Fatalf("opened=%v cmd=%v composing=%v", opened, cmd != nil, got.composing)
	}
}

func TestDoAgendaHidesScores(t *testing.T) {
	m := appModel{
		mode:         modeDo,
		view:         viewAgenda,
		width:        86,
		height:       24,
		records:      sampleDoRecords(t),
		agendaCursor: 0,
		monthDate:    sampleDate(2026, time.June, 13),
		monthFocus:   monthFocusGrid,
	}
	out := m.View()
	for _, want := range []string{
		"crusade",
		"DO",
		"2026-06-13  Have a body fat",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("do agenda missing %q:\n%s", want, out)
		}
	}
	for _, hidden := range []string{"nsm", "score ", " outcomes "} {
		if strings.Contains(out, hidden) {
			t.Fatalf("do agenda still shows %q:\n%s", hidden, out)
		}
	}
}

func TestDoMonthListUsesCheckboxRows(t *testing.T) {
	m := appModel{
		mode:       modeDo,
		view:       viewMonth,
		width:      90,
		height:     28,
		records:    sampleDoRecords(t),
		monthDate:  sampleDate(2026, time.June, 13),
		monthFocus: monthFocusGrid,
	}
	out := m.View()
	for _, line := range strings.Split(out, "\n") {
		plain := stripANSI(line)
		if strings.TrimSpace(plain) == "task" || strings.TrimSpace(plain) == "when" {
			t.Fatalf("do month still has a column header:\n%s", out)
		}
	}
	plain := stripANSI(out)
	if !strings.Contains(plain, "[ ]") || !strings.Contains(plain, "Have a body fat") {
		t.Fatalf("do month list is not a checkbox row:\n%s", plain)
	}
}

func TestComposeEmacsMotion(t *testing.T) {
	m := appModel{mode: modeDo, composing: true, composeText: "go snorkelling", composeCursor: 0}
	m.handleComposeKey("alt+f")
	if m.composeCursor != 2 {
		t.Fatalf("alt+f cursor = %d", m.composeCursor)
	}
	m.handleComposeKey("alt+f")
	if m.composeCursor != len([]rune(m.composeText)) {
		t.Fatalf("second alt+f cursor = %d", m.composeCursor)
	}
	m.handleComposeKey("alt+b")
	if got := string([]rune(m.composeText)[m.composeCursor:]); got != "snorkelling" {
		t.Fatalf("alt+b landed on %q", got)
	}
	m.handleComposeKey("alt+d")
	if m.composeText != "go " || m.composeCursor != 3 {
		t.Fatalf("alt+d text=%q cursor=%d", m.composeText, m.composeCursor)
	}
	m.handleComposeKey("ctrl+a")
	m.handleComposeKey("ctrl+d")
	if m.composeText != "o " {
		t.Fatalf("ctrl+d text=%q", m.composeText)
	}
	m.composeText = "go snorkelling"
	m.composeCursor = len([]rune(m.composeText))
	m.handleComposeKey("ctrl+w")
	if m.composeText != "go " {
		t.Fatalf("ctrl+w text=%q", m.composeText)
	}
	m.handleComposeKey("ctrl+a")
	m.handleComposeKey("ctrl+k")
	if m.composeText != "" || m.composeCursor != 0 {
		t.Fatalf("ctrl+k text=%q cursor=%d", m.composeText, m.composeCursor)
	}
	m.insertCompose("ab")
	m.handleComposeKey("left")
	m.insertCompose("!")
	if m.composeText != "a!b" || m.composeCursor != 2 {
		t.Fatalf("insert text=%q cursor=%d", m.composeText, m.composeCursor)
	}
}

func TestComposeCtrlDEditsInsteadOfSwitchingMode(t *testing.T) {
	m := appModel{mode: modeDo, composing: true, composeText: "ab", composeCursor: 0}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	got := next.(appModel)
	if got.mode != modeDo || !got.composing || got.composeText != "b" {
		t.Fatalf("mode=%q composing=%v text=%q", got.mode, got.composing, got.composeText)
	}
}

func TestModesKeepTheirOwnRecords(t *testing.T) {
	doEvent, err := model.NewDoEvent("2026-06-13 23:59:59", "mail the letter")
	if err != nil {
		t.Fatal(err)
	}
	records := append(sampleRecords(t), store.Record{ID: 9, Event: doEvent})
	doView := appModel{
		mode:    modeDo,
		view:    viewAgenda,
		width:   86,
		height:  24,
		records: records,
	}
	out := doView.View()
	if !strings.Contains(out, "crusade") || !strings.Contains(out, "mail the letter") || strings.Contains(out, "Have a body fat") {
		t.Fatalf("do agenda mixed records:\n%s", out)
	}
	thinkView := doView
	thinkView.mode = modeAlign
	out = thinkView.View()
	if !strings.Contains(out, "ALIGN") || !strings.Contains(out, "Have a body fat") || strings.Contains(out, "mail the letter") {
		t.Fatalf("think agenda mixed records:\n%s", out)
	}
}

func TestDoMonthNewTaskKeepsCalendarPut(t *testing.T) {
	m := appModel{
		mode:       modeDo,
		view:       viewMonth,
		width:      90,
		height:     36,
		records:    sampleDoRecords(t),
		monthDate:  sampleDate(2026, time.June, 13),
		monthFocus: monthFocusGrid,
	}
	before := m.View()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	got := next.(appModel)
	if cmd != nil || !got.composing || got.composeKind != composeTask {
		t.Fatalf("cmd=%v composing=%v kind=%q", cmd != nil, got.composing, got.composeKind)
	}
	after := got.View()
	if calendarHeaderLine(before) < 0 || calendarThroughHeader(before) != calendarThroughHeader(after) {
		t.Fatalf("calendar moved from line %d to %d", calendarHeaderLine(before), calendarHeaderLine(after))
	}
	plain := stripANSI(after)
	if !strings.Contains(plain, "task:") || !strings.Contains(plain, "Task for") {
		t.Fatalf("prompt missing:\n%s", plain)
	}
	typed, _ := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if calendarThroughHeader(after) != calendarThroughHeader(typed.(appModel).View()) {
		t.Fatal("calendar moved while typing")
	}
}

func TestDoRescheduleChangesDate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.yaml")
	records := sampleDoRecords(t)
	if err := store.Save(path, records); err != nil {
		t.Fatal(err)
	}
	m := appModel{
		dataPath:     path,
		mode:         modeDo,
		view:         viewAgenda,
		records:      records,
		agendaCursor: 0,
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	got := next.(appModel)
	if !got.composing || got.composeKind != composeWhen || got.composeText != "2026-06-13" {
		t.Fatalf("compose kind=%q text=%q composing=%v", got.composeKind, got.composeText, got.composing)
	}
	got.composeText = "2026-07-01"
	got.commitCompose()
	loaded, err := store.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].Event.Y != records[0].Event.Y || loaded[0].Event.X.Format(model.DateTimeFormat) != "2026-07-01 23:59:59" || loaded[0].Event.Mode != model.ModeDo {
		t.Fatalf("rescheduled = %+v", loaded)
	}
	if got.composing || got.status != "Rescheduled task." {
		t.Fatalf("status=%q composing=%v", got.status, got.composing)
	}

	again, _ := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	second := again.(appModel)
	if second.composeText != "2026-07-01" {
		t.Fatalf("date prefill = %q", second.composeText)
	}
	second.composeText = "2026-07-02 08:30"
	second.commitCompose()
	loaded, err = store.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded[0].Event.X.Format(model.DateTimeFormat) != "2026-07-02 08:30:00" || loaded[0].Event.Y != records[0].Event.Y {
		t.Fatalf("clock time = %+v", loaded[0].Event)
	}
}

func TestDoRescheduleKeepsCalendarPut(t *testing.T) {
	m := appModel{
		mode:       modeDo,
		view:       viewMonth,
		width:      90,
		height:     36,
		records:    sampleDoRecords(t),
		monthDate:  sampleDate(2026, time.June, 13),
		monthFocus: monthFocusEvents,
	}
	before := m.View()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	got := next.(appModel)
	after := got.View()
	if got.composeKind != composeWhen || calendarThroughHeader(before) != calendarThroughHeader(after) {
		t.Fatalf("kind=%q before %d after %d", got.composeKind, calendarHeaderLine(before), calendarHeaderLine(after))
	}
	if !strings.Contains(stripANSI(after), "when: 2026-06-13") {
		t.Fatalf("when prompt missing:\n%s", stripANSI(after))
	}
}

func TestDoRescheduleNeedsATask(t *testing.T) {
	m := appModel{mode: modeDo, view: viewAgenda}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	got := next.(appModel)
	if got.composing || got.status != "No task selected to reschedule." {
		t.Fatalf("composing=%v status=%q", got.composing, got.status)
	}
}

func TestDoRescheduleRejectsBadDate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.yaml")
	records := sampleDoRecords(t)
	if err := store.Save(path, records); err != nil {
		t.Fatal(err)
	}
	m := appModel{dataPath: path, mode: modeDo, view: viewAgenda, records: records}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	got := next.(appModel)
	got.composeText = "tomorrow"
	got.commitCompose()
	if !got.composing || !strings.Contains(got.status, "Invalid datetime") {
		t.Fatalf("status=%q composing=%v", got.status, got.composing)
	}
	loaded, err := store.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded[0].Event.X.Format("2006-01-02") != "2026-06-13" {
		t.Fatalf("date changed to %s", loaded[0].Event.X.Format(model.DateTimeFormat))
	}
}

func TestAlignRDoesNotReschedule(t *testing.T) {
	m := appModel{view: viewAgenda, records: sampleRecords(t)}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	got := next.(appModel)
	if got.composing {
		t.Fatal("align r started compose")
	}
}

func calendarThroughHeader(out string) string {
	idx := calendarHeaderLine(out)
	if idx < 0 {
		return ""
	}
	return strings.Join(strings.Split(out, "\n")[:idx+1], "\n")
}

func stripANSI(value string) string {
	var b strings.Builder
	for idx := 0; idx < len(value); {
		if value[idx] == 0x1b && idx+1 < len(value) && value[idx+1] == '[' {
			idx += 2
			for idx < len(value) && value[idx] != 'm' {
				idx++
			}
			if idx < len(value) {
				idx++
			}
			continue
		}
		b.WriteByte(value[idx])
		idx++
	}
	return b.String()
}

func isQuitCommand(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestSpaceMarksTheSelectedTaskDone(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SQLX_ROOT", root)
	t.Setenv("BOXES_CONFIG", filepath.Join(t.TempDir(), "missing.txt"))
	db := store.DatabasePath()
	event, err := model.NewDoEvent("2026-06-13 23:59:59", "mail the letter")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(db, []store.Record{{ID: 1, Event: event}}); err != nil {
		t.Fatal(err)
	}
	records, err := store.Load(db)
	if err != nil {
		t.Fatal(err)
	}
	m := newAppModel(db, records)
	m.width = 80
	m.height = 24
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	got := next.(appModel)
	if !strings.Contains(got.View(), "[x]") || got.status != "Marked done." {
		t.Fatalf("status=%q view missing mark:\n%s", got.status, got.View())
	}
	loaded, err := store.Load(db)
	if err != nil || len(loaded) != 1 || !loaded[0].Event.Done {
		t.Fatalf("stored done=%v err=%v", len(loaded) == 1 && loaded[0].Event.Done, err)
	}
	next, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	got = next.(appModel)
	loaded, err = store.Load(db)
	if err != nil || len(loaded) != 1 || loaded[0].Event.Done || got.status != "Cleared." {
		t.Fatalf("status=%q done=%v err=%v", got.status, len(loaded) == 1 && loaded[0].Event.Done, err)
	}
	if !strings.Contains(got.View(), "[ ]") {
		t.Fatalf("view missing open checkbox:\n%s", got.View())
	}
}

func TestTabCyclesDoAlignAndBox(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crusade.sqlite")
	t.Setenv("BOXES_CONFIG", "")
	doEvent, err := model.NewDoEvent("2026-06-13 23:59:59", "mail the letter")
	if err != nil {
		t.Fatal(err)
	}
	alignEvent, err := model.NewEvent("2026-06-14 23:59:59", "ship the patch", "reduce drift", "7", "8", "6")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(path, []store.Record{{ID: 1, Event: doEvent}, {ID: 2, Event: alignEvent}}); err != nil {
		t.Fatal(err)
	}
	records, err := store.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	m := newAppModel(path, records)
	m.width = 60
	m.height = 20
	tab := tea.KeyMsg{Type: tea.KeyTab}

	next, cmd := m.Update(tab)
	align, ok := next.(appModel)
	if !ok || cmd != nil || align.mode != modeAlign || align.width != 60 {
		t.Fatalf("align mode=%q width=%d type=%T cmd=%v", align.mode, align.width, next, cmd != nil)
	}
	alignView := align.View()
	if !strings.Contains(alignView, "ALIGN") || !strings.Contains(alignView, "ship the patch") || strings.Contains(alignView, "mail the letter") {
		t.Fatalf("align view:\n%s", alignView)
	}

	if mode, err := store.LastMode(path); err != nil || mode != model.ModeAlign {
		t.Fatalf("last mode after align = %q err=%v", mode, err)
	}

	next, cmd = align.Update(tab)
	box, ok := next.(boxModel)
	if !ok || cmd != nil || box.width != 60 || !strings.Contains(box.View(), "BOX") {
		t.Fatalf("box width=%d type=%T cmd=%v", box.width, next, cmd != nil)
	}
	if mode, err := store.LastMode(path); err != nil || mode != model.ModeBox {
		t.Fatalf("last mode after box = %q err=%v", mode, err)
	}
	box.showHelp = true
	next, cmd = box.Update(tab)
	do, ok := next.(appModel)
	if !ok || cmd != nil || do.mode != modeDo || do.width != 60 {
		t.Fatalf("do mode=%q width=%d type=%T cmd=%v", do.mode, do.width, next, cmd != nil)
	}
	doView := do.View()
	if !strings.Contains(doView, "DO") || !strings.Contains(doView, "mail the letter") || strings.Contains(doView, "ship the patch") {
		t.Fatalf("do view:\n%s", doView)
	}
	if mode, err := store.LastMode(path); err != nil || mode != model.ModeDo {
		t.Fatalf("last mode after do = %q err=%v", mode, err)
	}
}

func TestRememberModeWritesTheLaunchMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crusade.sqlite")
	t.Setenv("BOXES_CONFIG", "")
	rememberMode(path, model.ModeBox)
	mode, err := store.LastMode(path)
	if err != nil || mode != model.ModeBox {
		t.Fatalf("mode=%q err=%v", mode, err)
	}
	rememberMode(path, "nope")
	mode, err = store.LastMode(path)
	if err != nil || mode != model.ModeBox {
		t.Fatalf("invalid remember changed mode to %q err=%v", mode, err)
	}
}

func TestTabToBoxStaysInAlignWhenTheDatabaseCannotOpen(t *testing.T) {
	m := newAppModel(t.TempDir(), nil)
	m.mode = modeAlign
	next, cmd := m.cycleMode()
	got, ok := next.(appModel)
	if !ok || cmd != nil || got.mode != modeAlign || !strings.Contains(got.status, "error:") {
		t.Fatalf("mode=%q status=%q ok=%v cmd=%v", got.mode, got.status, ok, cmd != nil)
	}
}

func TestTabRestoresEachMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crusade.sqlite")
	t.Setenv("BOXES_CONFIG", "")
	doOne, err := model.NewDoEvent("2026-06-13 23:59:59", "mail the letter")
	if err != nil {
		t.Fatal(err)
	}
	doTwo, err := model.NewDoEvent("2026-06-15 23:59:59", "file the form")
	if err != nil {
		t.Fatal(err)
	}
	alignOne, err := model.NewEvent("2026-06-14 23:59:59", "ship the patch", "reduce drift", "7", "8", "6")
	if err != nil {
		t.Fatal(err)
	}
	alignTwo, err := model.NewEvent("2026-07-01 23:59:59", "write the note", "keep the trail", "5", "5", "5")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(path, []store.Record{{ID: 1, Event: doOne}, {ID: 2, Event: doTwo}, {ID: 3, Event: alignOne}, {ID: 4, Event: alignTwo}}); err != nil {
		t.Fatal(err)
	}
	records, err := store.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	m := newAppModel(path, records)
	m.width = 60
	m.height = 24
	m.view = viewMonth
	m.monthDate = dateOnly(time.Date(2026, 4, 2, 0, 0, 0, 0, time.Local))
	m.agendaCursor = 1
	m.status = "do place"
	m.showHelp = true
	tab := tea.KeyMsg{Type: tea.KeyTab}

	next, cmd := m.Update(tab)
	align, ok := next.(appModel)
	if !ok || cmd != nil || align.mode != modeAlign || align.view != viewAgenda || align.agendaCursor != 0 || align.status != "" || align.showHelp {
		t.Fatalf("first align mode=%s view=%s cursor=%d status=%q help=%v", align.mode, align.view, align.agendaCursor, align.status, align.showHelp)
	}
	align.view = viewMonth
	align.monthDate = dateOnly(time.Date(2026, 8, 9, 0, 0, 0, 0, time.Local))
	align.agendaCursor = 1
	align.status = "align place"
	align.showHelp = true

	next, cmd = align.Update(tab)
	box, ok := next.(boxModel)
	if !ok || cmd != nil || box.selected != 0 {
		t.Fatalf("first box type=%T selected=%d", next, box.selected)
	}
	box.day = "2026-09-03"
	box.selected = 2
	box.status = "box place"
	box.showHelp = true

	next, cmd = box.Update(tab)
	do, ok := next.(appModel)
	if !ok || cmd != nil || do.mode != modeDo || do.view != viewMonth || do.monthDate.Format(time.DateOnly) != "2026-04-02" || do.agendaCursor != 1 || do.status != "do place" || !do.showHelp {
		t.Fatalf("restored do mode=%s view=%s date=%s cursor=%d status=%q help=%v", do.mode, do.view, do.monthDate.Format(time.DateOnly), do.agendaCursor, do.status, do.showHelp)
	}

	next, cmd = do.Update(tab)
	align, ok = next.(appModel)
	if !ok || cmd != nil || align.mode != modeAlign || align.view != viewMonth || align.monthDate.Format(time.DateOnly) != "2026-08-09" || align.agendaCursor != 1 || align.status != "align place" || !align.showHelp {
		t.Fatalf("restored align mode=%s view=%s date=%s cursor=%d status=%q help=%v", align.mode, align.view, align.monthDate.Format(time.DateOnly), align.agendaCursor, align.status, align.showHelp)
	}

	next, cmd = align.Update(tab)
	box, ok = next.(boxModel)
	if !ok || cmd != nil || box.day != "2026-09-03" || box.selected != 2 || box.status != "box place" || !box.showHelp {
		t.Fatalf("restored box day=%s selected=%d status=%q help=%v", box.day, box.selected, box.status, box.showHelp)
	}
}

func TestTabStaysInCompose(t *testing.T) {
	m := newAppModel(filepath.Join(t.TempDir(), "tasks.sqlite"), nil)
	m.composing = true
	m.composeText = "hello"
	m.composeCursor = 5
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	got, ok := next.(appModel)
	if !ok || cmd != nil || !got.composing || got.mode != modeDo || got.composeText != "hello" {
		t.Fatalf("composing=%v mode=%q text=%q type=%T", got.composing, got.mode, got.composeText, next)
	}
}

func TestDoneThisWeekStartsOnMonday(t *testing.T) {
	monday := sampleDate(2026, time.June, 15)
	wednesday := sampleDate(2026, time.June, 17)
	sunday := time.Date(2026, time.June, 21, 23, 59, 59, 0, time.Local)
	nextMonday := sampleDate(2026, time.June, 22)
	previousSunday := time.Date(2026, time.June, 14, 23, 59, 59, 0, time.Local)

	records := []store.Record{
		doRecordAt(t, 1, monday, true, "monday"),
		doRecordAt(t, 2, sunday, true, "sunday"),
		doRecordAt(t, 3, wednesday, false, "open"),
		doRecordAt(t, 4, previousSunday, true, "last week"),
		doRecordAt(t, 5, nextMonday, true, "next week"),
	}
	align, err := model.NewEvent("2026-06-17 23:59:59", "align outcome", "because", "1", "1", "1")
	if err != nil {
		t.Fatal(err)
	}
	align.Done = true
	records = append(records, store.Record{ID: 6, Event: align})

	for _, now := range []time.Time{monday, wednesday, sunday} {
		if got := doneThisWeek(records, now); got != 2 {
			t.Fatalf("now %s done this week = %d", now.Format(time.RFC3339), got)
		}
	}
}

func TestDoModeShowsDoneThisWeek(t *testing.T) {
	now := time.Now()
	monday := mondayOf(now)
	records := []store.Record{
		doRecordAt(t, 1, monday, true, "monday task"),
		doRecordAt(t, 2, monday.AddDate(0, 0, 3), true, "midweek task"),
		doRecordAt(t, 3, monday.AddDate(0, 0, 6), false, "still open"),
		doRecordAt(t, 4, monday.AddDate(0, 0, -1), true, "last sunday"),
		doRecordAt(t, 5, monday.AddDate(0, 0, 7), true, "next monday"),
	}
	agenda := appModel{
		mode:    modeDo,
		view:    viewAgenda,
		width:   90,
		height:  24,
		records: records,
	}
	out := stripANSI(agenda.View())
	if !strings.Contains(out, "2 done this week") {
		t.Fatalf("agenda missing week count:\n%s", out)
	}
	month := agenda
	month.view = viewMonth
	month.monthDate = monday
	month.monthFocus = monthFocusGrid
	out = stripANSI(month.View())
	if !strings.Contains(out, "2 done this week") {
		t.Fatalf("month missing week count:\n%s", out)
	}
	empty := appModel{mode: modeDo, view: viewAgenda, width: 90, height: 24}
	if !strings.Contains(stripANSI(empty.View()), "0 done this week") {
		t.Fatalf("empty agenda missing week count:\n%s", empty.View())
	}
	alignView := agenda
	alignView.mode = modeAlign
	if strings.Contains(stripANSI(alignView.View()), "done this week") {
		t.Fatalf("align shows the do week count:\n%s", alignView.View())
	}
}

func doRecordAt(t *testing.T, id int, when time.Time, done bool, outcome string) store.Record {
	t.Helper()
	event, err := model.NewDoEvent(when.Format(model.DateTimeFormat), outcome)
	if err != nil {
		t.Fatal(err)
	}
	event.Done = done
	return store.Record{ID: id, Event: event}
}
