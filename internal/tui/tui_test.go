package tui

import (
	"errors"
	"fmt"
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

// isolateSQLX points every database, config, and import path at a temporary
// directory.
func isolateSQLX(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("SQLX_ROOT", filepath.Join(root, "sqlx"))
	t.Setenv("BOXES_CONFIG", filepath.Join(root, "missing.txt"))
}

func TestAgendaRendersCheckboxList(t *testing.T) {
	m := appModel{
		width:        86,
		height:       24,
		records:      sampleRecords(t),
		agendaCursor: 0,
		agendaCol:    1,
	}

	out := m.View()

	for _, want := range []string{
		"crusade",
		"ALIGN",
		"selected 1/3",
		"June 13, 2026 (Saturday)",
		"2026-06-13",
		"[ ]",
		"2026-06-13  Have a body fat",
		"2026-12-31  Learned open water swimming",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("agenda output missing %q:\n%s", want, out)
		}
	}
	for _, hidden := range []string{"nsm", "score", "23:59", " outcomes ", "Increase my chances", "0.8", "June, 2026"} {
		if strings.Contains(out, hidden) {
			t.Fatalf("agenda still shows %q:\n%s", hidden, out)
		}
	}
	if strings.Contains(out, "a month  tab bucket") {
		t.Fatalf("agenda should not render the shortcut strip:\n%s", out)
	}
}

func TestEmptyAgendaPointsAtTheCLI(t *testing.T) {
	out := stripANSI(appModel{width: 86, height: 24}.View())
	if !strings.Contains(out, "empty agenda | crusade -m align -c") || !strings.Contains(out, "No tasks yet. Press n or use crusade -m align -c.") {
		t.Fatalf("empty agenda:\n%s", out)
	}
}

func TestViewsStartAtTheTop(t *testing.T) {
	views := map[string]tea.Model{
		"align": appModel{width: 120, height: 40, records: sampleRecords(t), agendaCol: 1},
		"do":    doModel{width: 120, height: 40, rows: model.ParseDoList(sampleDoList)},
	}
	for name, m := range views {
		t.Run(name, func(t *testing.T) {
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
	m := appModel{width: 120, height: 40, showHelp: true}

	out := m.View()
	for _, want := range []string{
		"crusade",
		"QUICK GUIDE",
		"GENERAL",
		"AGENDA",
		"EXIT",
		"VIM TASK FILE",
		"Close this guide",
		"Align is a list",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("structured help output missing %q:\n%s", want, out)
		}
	}
	for _, gone := range []string{"ctrl+c / esc quit", "MONTH", "Agenda / month"} {
		if strings.Contains(out, gone) {
			t.Fatalf("help still shows %q:\n%s", gone, out)
		}
	}
	checkModalInset(t, out)
}

// checkModalInset checks that the modal sits over the screen in one column.
func checkModalInset(t *testing.T, out string) {
	t.Helper()
	lines := strings.Split(stripANSI(out), "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "crusade") || strings.Contains(lines[0], "QUICK GUIDE") {
		t.Fatalf("help replaced the screen:\n%s", out)
	}
	firstBorder := -1
	for _, line := range lines {
		if !strings.ContainsAny(line, "╭│╰") {
			continue
		}
		border := strings.IndexAny(line, "╭│╰")
		if firstBorder == -1 {
			firstBorder = border
		}
		if border != firstBorder {
			t.Fatalf("help modal border shifted from column %d to %d:\n%s", firstBorder, border, out)
		}
	}
	if firstBorder <= 0 {
		t.Fatalf("help modal is not inset:\n%s", out)
	}
}

func TestQuestionTogglesShortcutModal(t *testing.T) {
	m := appModel{width: 100, height: 36, records: sampleRecords(t)}
	if strings.Contains(m.View(), "QUICK GUIDE") {
		t.Fatal("guide was already open")
	}
	next, cmd := m.Update(runeKey('?'))
	got, ok := next.(appModel)
	if cmd != nil || !ok || !got.showHelp {
		t.Fatalf("open cmd=%v ok=%v", cmd != nil, ok)
	}
	view := stripANSI(got.View())
	if !strings.Contains(view, "QUICK GUIDE") || !strings.Contains(view, "Close this guide") || !strings.HasPrefix(strings.Split(view, "\n")[0], "crusade") {
		t.Fatalf("modal:\n%s", view)
	}
	cursor := got.agendaCursor
	next, cmd = got.Update(runeKey('j'))
	got, ok = next.(appModel)
	if cmd != nil || !ok || !got.showHelp || got.agendaCursor != cursor {
		t.Fatalf("j moved the list help=%v cursor=%d", ok && got.showHelp, got.agendaCursor)
	}
	next, cmd = got.Update(runeKey('?'))
	got, ok = next.(appModel)
	if cmd != nil || !ok || got.showHelp || strings.Contains(got.View(), "QUICK GUIDE") {
		t.Fatalf("close help=%v\n%s", ok && got.showHelp, got.View())
	}
	next, cmd = got.Update(runeKey('?'))
	got, ok = next.(appModel)
	if cmd != nil || !ok || !got.showHelp {
		t.Fatalf("reopen cmd=%v ok=%v", cmd != nil, ok)
	}
	next, cmd = got.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got, ok = next.(appModel)
	if cmd != nil || !ok || got.showHelp {
		t.Fatalf("esc quit cmd=%v help=%v", cmd != nil, ok && got.showHelp)
	}
}

func TestHelpPanelDoesNotPaintABackgroundFill(t *testing.T) {
	forceStyledOutput(t)
	out := appModel{}.helpView(92)
	if strings.Contains(out, "\x1b[48;") {
		t.Fatalf("help panel emitted a terminal background color:\n%q", out)
	}
}

func TestTitleDoesNotUsePulseAnimation(t *testing.T) {
	m := appModel{width: 90, height: 28, records: sampleRecords(t)}

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

func TestQQuitsWhenNotTyping(t *testing.T) {
	for name, m := range map[string]appModel{
		"agenda": {},
		"help":   {showHelp: true},
	} {
		t.Run(name, func(t *testing.T) {
			_, cmd := m.Update(runeKey('q'))
			if !isQuitCommand(cmd) {
				t.Fatal("q did not quit")
			}
		})
	}
}

func TestCtrlCQuits(t *testing.T) {
	for name, m := range map[string]appModel{"agenda": {}, "help": {showHelp: true}} {
		t.Run(name, func(t *testing.T) {
			_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
			if !isQuitCommand(cmd) {
				t.Fatal("ctrl+c did not return tea.Quit")
			}
		})
	}
}

func TestEscapeQuitsTheAgendaAndClosesTheGuide(t *testing.T) {
	_, cmd := appModel{records: sampleRecords(t)}.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !isQuitCommand(cmd) {
		t.Fatal("esc on the agenda did not quit")
	}
	help := appModel{showHelp: true}
	next, cmd := help.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got, ok := next.(appModel)
	if cmd != nil || !ok || got.showHelp {
		t.Fatalf("esc quit the guide cmd=%v help=%v", cmd != nil, ok && got.showHelp)
	}
}

// stubTaskEditor replaces Vim with finish and records what n or e asked for.
func stubTaskEditor(t *testing.T, finish func(id int, seed model.Align) editorFinishedMsg) *[]editorRequest {
	t.Helper()
	previous := openTaskEditor
	t.Cleanup(func() { openTaskEditor = previous })
	var requests []editorRequest
	openTaskEditor = func(id int, seed model.Align) tea.Cmd {
		requests = append(requests, editorRequest{id: id, seed: seed})
		return func() tea.Msg { return finish(id, seed) }
	}
	return &requests
}

type editorRequest struct {
	id   int
	seed model.Align
}

func seededAlign(t *testing.T) (string, appModel) {
	t.Helper()
	isolateSQLX(t)
	path := filepath.Join(t.TempDir(), "crusade.sqlite")
	for _, record := range sampleRecords(t) {
		if err := store.Add(path, record.Align); err != nil {
			t.Fatal(err)
		}
	}
	records, err := store.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	m := newAppModel(path, records)
	m.width = 90
	m.height = 24
	return path, m
}

func loadAlign(t *testing.T, path string) []store.AlignRecord {
	t.Helper()
	records, err := store.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return records
}

func TestNewTaskStoresNothingUntilVimSaves(t *testing.T) {
	path, m := seededAlign(t)
	m.agendaCursor = 1
	requests := stubTaskEditor(t, func(id int, seed model.Align) editorFinishedMsg {
		seed.Y = "edited in Vim"
		seed.Z = "keeps one record"
		return editorFinishedMsg{id: id, event: seed, saved: true}
	})

	next, cmd := m.Update(runeKey('n'))
	if cmd == nil || len(*requests) != 1 {
		t.Fatalf("editor requests = %#v", *requests)
	}
	request := (*requests)[0]
	if request.id != 0 || request.seed.X.Format(time.DateOnly) != "2026-12-31" || request.seed.Y != "" || request.seed.Done() {
		t.Fatalf("editor request = %#v", request)
	}
	if got := next.(appModel); got.status != "Editing a new task in Vim..." {
		t.Fatalf("status %q", got.status)
	}
	if records := loadAlign(t, path); len(records) != 3 {
		t.Fatalf("n stored a record before Vim saved: %#v", records)
	}
	finished, _ := next.(appModel).Update(cmd())
	updated := finished.(appModel)
	records := loadAlign(t, path)
	if len(records) != 4 || len(updated.records) != 4 || updated.status != "Saved." {
		t.Fatalf("records=%d model=%d status=%q", len(records), len(updated.records), updated.status)
	}
	count := 0
	for _, record := range records {
		if record.Align.Y == "edited in Vim" && record.Align.X.Format(time.DateOnly) == "2026-12-31" && record.Align.Z == "keeps one record" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("saved records = %#v", records)
	}
}

func TestCancelledNewTaskStoresNothing(t *testing.T) {
	path, m := seededAlign(t)
	stubTaskEditor(t, func(id int, _ model.Align) editorFinishedMsg {
		return editorFinishedMsg{id: id}
	})

	next, cmd := m.Update(runeKey('n'))
	finished, _ := next.(appModel).Update(cmd())
	updated := finished.(appModel)
	if records := loadAlign(t, path); len(records) != 3 || len(updated.records) != 3 || updated.status != "New task cancelled." {
		t.Fatalf("blank draft stored: records=%d status=%q", len(records), updated.status)
	}
}

func TestNewTaskEditorErrorStoresNothing(t *testing.T) {
	path, m := seededAlign(t)
	stubTaskEditor(t, func(id int, _ model.Align) editorFinishedMsg {
		return editorFinishedMsg{id: id, err: errVim}
	})
	next, cmd := m.Update(runeKey('n'))
	finished, _ := next.(appModel).Update(cmd())
	updated := finished.(appModel)
	if records := loadAlign(t, path); len(records) != 3 || updated.status != "error: Vim exited: vim failed" {
		t.Fatalf("records=%d status=%q", len(records), updated.status)
	}
}

var errVim = errors.New("vim failed")

func TestEditOpensSelectedRecordAndUpdatesIt(t *testing.T) {
	path, m := seededAlign(t)
	m.agendaCursor = 1
	target := m.records[1]
	requests := stubTaskEditor(t, func(id int, seed model.Align) editorFinishedMsg {
		seed.Y = "swim a mile"
		return editorFinishedMsg{id: id, event: seed, saved: true}
	})

	next, cmd := m.Update(runeKey('e'))
	if cmd == nil || len(*requests) != 1 || (*requests)[0].id != target.ID || (*requests)[0].seed.Y != target.Align.Y {
		t.Fatalf("edit request = %#v want id %d", *requests, target.ID)
	}
	if got := next.(appModel); got.status != fmt.Sprintf("Editing task %d in Vim...", target.ID) {
		t.Fatalf("status %q", got.status)
	}
	finished, _ := next.(appModel).Update(cmd())
	updated := finished.(appModel)
	records := loadAlign(t, path)
	edited, found := recordByID(records, target.ID)
	if len(records) != 3 || !found || edited.Align.Y != "swim a mile" || edited.Align.Z != target.Align.Z || updated.status != "Saved." {
		t.Fatalf("records=%#v status=%q", records, updated.status)
	}
}

func TestEditNeedsARecord(t *testing.T) {
	requests := stubTaskEditor(t, func(id int, _ model.Align) editorFinishedMsg { return editorFinishedMsg{id: id} })
	next, cmd := appModel{}.Update(runeKey('e'))
	if got := next.(appModel); cmd != nil || len(*requests) != 0 || got.status != "No task selected to edit." {
		t.Fatalf("cmd=%v requests=%d status=%q", cmd != nil, len(*requests), got.status)
	}
}

func TestSpaceMarksTheSelectedTaskDone(t *testing.T) {
	isolateSQLX(t)
	db := store.DatabasePath()
	if err := store.Add(db, mustEvent(t, "2026-06-13", "mail the letter", "keep the promise")); err != nil {
		t.Fatal(err)
	}
	m := newAppModel(db, loadAlign(t, db))
	m.width = 80
	m.height = 24
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	got := next.(appModel)
	if !strings.Contains(got.View(), "[x]") || got.status != "Marked done." {
		t.Fatalf("status=%q view missing mark:\n%s", got.status, got.View())
	}
	loaded := loadAlign(t, db)
	if len(loaded) != 1 || !loaded[0].Align.Done() || !sameDate(loaded[0].Align.DoneAt, time.Now()) {
		t.Fatalf("stored %#v", loaded)
	}
	next, _ = got.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	got = next.(appModel)
	loaded = loadAlign(t, db)
	if len(loaded) != 1 || loaded[0].Align.Done() || got.status != "Cleared." {
		t.Fatalf("status=%q stored=%#v", got.status, loaded)
	}
	if !strings.Contains(got.View(), "[ ]") {
		t.Fatalf("view missing open checkbox:\n%s", got.View())
	}
}

func TestDDDeletesTheSelectedRecord(t *testing.T) {
	path, m := seededAlign(t)
	m.agendaCursor = 1
	target := m.records[1]
	next, _ := m.Update(runeKey('d'))
	got := next.(appModel)
	if !got.pendingDelete || got.status != "Press d again to delete the selected outcome." || len(loadAlign(t, path)) != 3 {
		t.Fatalf("first d pending=%v status=%q", got.pendingDelete, got.status)
	}
	next, _ = got.Update(runeKey('j'))
	got = next.(appModel)
	if got.pendingDelete {
		t.Fatal("j kept the pending delete")
	}
	next, _ = got.Update(runeKey('k'))
	got = next.(appModel)
	next, _ = got.Update(runeKey('d'))
	next, _ = next.(appModel).Update(runeKey('d'))
	got = next.(appModel)
	records := loadAlign(t, path)
	if _, found := recordByID(records, target.ID); found || len(records) != 2 || got.status != "Deleted outcome." || strings.Contains(got.View(), target.Align.Y) {
		t.Fatalf("status=%q records=%#v\n%s", got.status, records, got.View())
	}
	if count, err := store.DeletedThisWeek(path, time.Now()); err != nil || count != 0 {
		t.Fatalf("align delete counted as a do deletion: %d %v", count, err)
	}
}

func TestAgendaMovesByRowAndDate(t *testing.T) {
	m := appModel{records: sampleRecords(t)}
	steps := []struct {
		key    string
		cursor int
	}{{"j", 1}, {"j", 2}, {"j", 2}, {"k", 1}, {"g", 0}, {"G", 2}, {"H", 1}, {"H", 0}, {"L", 1}}
	for _, step := range steps {
		next, cmd := m.Update(runeKey([]rune(step.key)[0]))
		m = next.(appModel)
		if cmd != nil || m.agendaCursor != step.cursor {
			t.Fatalf("%s cursor %d want %d", step.key, m.agendaCursor, step.cursor)
		}
	}
}

func TestAlignModeIsAList(t *testing.T) {
	align := appModel{width: 90, height: 36, records: sampleRecords(t)}
	out := stripANSI(align.View())
	if strings.Contains(out, "June, 2026") || !strings.Contains(out, "Have a body fat") {
		t.Fatalf("align month leaked:\n%s", out)
	}
	for _, key := range []rune{'a', 'r', 'm'} {
		next, cmd := align.Update(runeKey(key))
		got, ok := next.(appModel)
		if cmd != nil || !ok || got.status != "" || stripANSI(got.View()) != out {
			t.Fatalf("align %c changed the list:\n%s", key, got.View())
		}
	}
}

func TestTabCyclesAlignBoxCalNLP(t *testing.T) {
	isolateSQLX(t)
	path := filepath.Join(t.TempDir(), "crusade.sqlite")
	if err := store.Add(path, mustEvent(t, "2026-06-14", "ship the patch", "reduce drift")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddDoItem(path, 0, "mail the letter"); err != nil {
		t.Fatal(err)
	}
	m := newAppModel(path, loadAlign(t, path))
	m.width = 60
	m.height = 20
	tab := tea.KeyMsg{Type: tea.KeyTab}
	if view := m.View(); !strings.Contains(view, "ALIGN") || !strings.Contains(view, "ship the patch") || strings.Contains(view, "mail the letter") {
		t.Fatalf("align view:\n%s", view)
	}

	next, cmd := m.Update(tab)
	box, ok := next.(boxModel)
	if !ok || cmd != nil || box.width != 60 || !strings.Contains(box.View(), "BOX") {
		t.Fatalf("box width=%d type=%T cmd=%v", box.width, next, cmd != nil)
	}
	checkLastMode(t, path, model.ModeBox)
	box.showHelp = true
	next, cmd = box.Update(tab)
	cal, ok := next.(calModel)
	if !ok || cmd != nil || cal.width != 60 || !strings.Contains(cal.View(), "CAL") {
		t.Fatalf("cal width=%d type=%T cmd=%v", cal.width, next, cmd != nil)
	}
	checkLastMode(t, path, model.ModeCal)
	next, cmd = cal.Update(tab)
	practice, ok := next.(nlpMode)
	if !ok || cmd != nil || practice.width != 60 || !strings.Contains(practice.View(), "NLP") {
		t.Fatalf("nlp width=%d type=%T cmd=%v", practice.width, next, cmd != nil)
	}
	checkLastMode(t, path, model.ModeNLP)
	next, cmd = practice.Update(tab)
	align, ok := next.(appModel)
	if !ok || cmd != nil || align.width != 60 || !strings.Contains(align.View(), "ALIGN") {
		t.Fatalf("return align width=%d type=%T cmd=%v", align.width, next, cmd != nil)
	}
	checkLastMode(t, path, model.ModeAlign)
}

func checkLastMode(t *testing.T, path, want string) {
	t.Helper()
	if mode, err := store.LastMode(path); err != nil || mode != want {
		t.Fatalf("last mode = %q want %q err=%v", mode, want, err)
	}
}

func TestTabToBoxStaysInAlignWhenTheDatabaseCannotOpen(t *testing.T) {
	isolateSQLX(t)
	t.Setenv("SQLX_SQLITE", "/bin/false")
	m := newAppModel(t.TempDir(), nil)
	next, cmd := m.cycleMode()
	got, ok := next.(appModel)
	if !ok || cmd != nil || !strings.Contains(got.status, "error:") {
		t.Fatalf("type=%T status=%q cmd=%v", next, got.status, cmd != nil)
	}
}

func TestTabRestoresEachMode(t *testing.T) {
	isolateSQLX(t)
	path := filepath.Join(t.TempDir(), "crusade.sqlite")
	for _, event := range []model.Align{
		mustEvent(t, "2026-06-14", "ship the patch", "reduce drift"),
		mustEvent(t, "2026-07-01", "write the note", "keep the trail"),
	} {
		if err := store.Add(path, event); err != nil {
			t.Fatal(err)
		}
	}
	after := 0
	for _, label := range []string{"Move", "Read", "Pray"} {
		id, err := store.AddBox(path, after, label)
		if err != nil {
			t.Fatal(err)
		}
		after = id
	}
	seedCalDay(t, path)
	align := newAppModel(path, loadAlign(t, path))
	align.width = 60
	align.height = 24
	align.agendaCursor = 1
	align.status = "align place"
	align.showHelp = true
	tab := tea.KeyMsg{Type: tea.KeyTab}

	next, cmd := align.Update(tab)
	box, ok := next.(boxModel)
	if !ok || cmd != nil || box.selected != 0 {
		t.Fatalf("first box type=%T selected=%d", next, box.selected)
	}
	box.day = "2026-09-03"
	box.selected = 2
	box.status = "box place"
	box.showHelp = true

	next, cmd = box.Update(tab)
	cal, ok := next.(calModel)
	if !ok || cmd != nil || !sameDate(cal.day, dateOnly(time.Now())) || cal.selected != 0 || cal.status != "" || cal.showHelp {
		t.Fatalf("first cal day=%s selected=%d status=%q help=%v", cal.day.Format(time.DateOnly), cal.selected, cal.status, cal.showHelp)
	}
	cal.day = dateOnly(time.Date(2026, 11, 6, 0, 0, 0, 0, time.Local))
	cal.selected = 1
	cal.status = "cal place"
	cal.showHelp = true

	next, cmd = cal.Update(tab)
	practice, ok := next.(nlpMode)
	if !ok || cmd != nil || !strings.Contains(practice.View(), "NLP") {
		t.Fatalf("nlp type=%T", next)
	}
	next, cmd = practice.Update(tab)
	align, ok = next.(appModel)
	if !ok || cmd != nil || align.agendaCursor != 1 || align.status != "align place" || !align.showHelp {
		t.Fatalf("restored align cursor=%d status=%q help=%v", align.agendaCursor, align.status, align.showHelp)
	}
	checkRestoredBoxAndCal(t, align)
}

// seedCalDay blocks two hours on 2026-11-06 so a cal selection there is kept.
func seedCalDay(t *testing.T, path string) {
	t.Helper()
	for _, span := range [][2]string{{"09:00", "10:00"}, {"11:00", "12:00"}} {
		block, err := model.NewBlock("block "+span[0], "2026-11-06 "+span[0], "2026-11-06 "+span[1])
		if err == nil {
			_, err = store.AddBlock(path, block)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

// checkRestoredBoxAndCal tabs on from a restored align and checks that box
// and cal come back where they were left.
func checkRestoredBoxAndCal(t *testing.T, align appModel) {
	t.Helper()
	tab := tea.KeyMsg{Type: tea.KeyTab}
	next, cmd := align.Update(tab)
	box, ok := next.(boxModel)
	if !ok || cmd != nil || box.day != "2026-09-03" || box.selected != 2 || box.status != "box place" || !box.showHelp {
		t.Fatalf("restored box day=%s selected=%d status=%q help=%v", box.day, box.selected, box.status, box.showHelp)
	}

	next, cmd = box.Update(tab)
	cal, ok := next.(calModel)
	if !ok || cmd != nil || cal.day.Format(time.DateOnly) != "2026-11-06" || cal.selected != 1 || cal.status != "cal place" || !cal.showHelp {
		t.Fatalf("restored cal day=%s selected=%d status=%q help=%v", cal.day.Format(time.DateOnly), cal.selected, cal.status, cal.showHelp)
	}
	next, cmd = cal.Update(tab)
	practice, ok := next.(nlpMode)
	if !ok || cmd != nil || !strings.Contains(practice.View(), "NLP") {
		t.Fatalf("restored nlp type=%T", next)
	}
}

func TestDoTabRestoresAlign(t *testing.T) {
	isolateSQLX(t)
	path := filepath.Join(t.TempDir(), "crusade.sqlite")
	for _, event := range []model.Align{
		mustEvent(t, "2026-06-14", "ship the patch", "reduce drift"),
		mustEvent(t, "2026-07-01", "write the note", "keep the trail"),
	} {
		if err := store.Add(path, event); err != nil {
			t.Fatal(err)
		}
	}
	do, err := openDoModel(path)
	if err != nil {
		t.Fatal(err)
	}
	do.memory = modeMemory{align: sessionState{agendaCursor: 1, status: "align place"}, hasAlign: true}
	next, cmd := do.Update(tea.KeyMsg{Type: tea.KeyTab})
	align, ok := next.(appModel)
	if !ok || cmd != nil || align.agendaCursor != 1 || align.status != "align place" {
		t.Fatalf("type=%T cursor=%d status=%q", next, align.agendaCursor, align.status)
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

func sampleRecords(t *testing.T) []store.AlignRecord {
	t.Helper()
	rows := []struct {
		id int
		x  string
		y  string
		z  string
	}{
		{
			id: 1,
			x:  "2026-06-13",
			y:  "Have a body fat % of 23.5, 33 kg of muscle, and a visceral fat level of 16",
			z:  "Increase my chances of living longer to serve the Lord, and also to look great in swimwear",
		},
		{
			id: 2,
			x:  "2026-12-31",
			y:  "Learned open water swimming",
			z:  "So I can flex during beach vacations doing crazy stuff",
		},
		{
			id: 3,
			x:  "2027-03-01",
			y:  "Ship a useful thing",
			z:  "Make the operating system stronger",
		},
	}
	records := make([]store.AlignRecord, 0, len(rows))
	for _, row := range rows {
		event, err := model.NewAlign(row.x, row.y, row.z)
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, store.AlignRecord{ID: row.id, Align: event})
	}
	return records
}

func sampleDate(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.Local)
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

func TestRememberModeWritesTheLaunchMode(t *testing.T) {
	isolateSQLX(t)
	path := filepath.Join(t.TempDir(), "crusade.sqlite")
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
