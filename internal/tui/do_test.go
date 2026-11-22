package tui

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ryangerardwilson/crusade/internal/model"
	"github.com/ryangerardwilson/crusade/internal/store"
)

const sampleDoList = "# Today\n- mail the letter\n\n# Later\n- file the form\n"

// openSampleDo stores sampleDoList in a fresh database and opens do mode on it.
func openSampleDo(t *testing.T) (string, doModel) {
	t.Helper()
	isolateSQLX(t)
	db := filepath.Join(t.TempDir(), "crusade.sqlite")
	if err := store.ReplaceDoList(db, model.ParseDoList(sampleDoList)); err != nil {
		t.Fatal(err)
	}
	m, err := openDoModel(db)
	if err != nil {
		t.Fatal(err)
	}
	m.width = 80
	m.height = 24
	return db, m
}

func keyDo(t *testing.T, m doModel, msg tea.KeyMsg) doModel {
	t.Helper()
	next, cmd := m.Update(msg)
	if cmd != nil {
		t.Fatalf("command on %s", msg.String())
	}
	got, ok := next.(doModel)
	if !ok {
		t.Fatalf("left do on %s: %T", msg.String(), next)
	}
	return got
}

// doListLines is the list part of the do view without styling.
func doListLines(m doModel) []string {
	lines := strings.Split(stripANSI(m.View()), "\n")
	var list []string
	for _, line := range lines[2:] {
		if !strings.HasPrefix(line, "> ") && !strings.HasPrefix(line, "  ") {
			break
		}
		list = append(list, line)
	}
	return list
}

func storedDoText(t *testing.T, db string) string {
	t.Helper()
	rows, err := store.LoadDoList(db)
	if err != nil {
		t.Fatal(err)
	}
	return model.RenderDoList(rows)
}

func rowIDs(rows []model.DoItem) []int {
	ids := make([]int, len(rows))
	for index, row := range rows {
		ids[index] = row.ID
	}
	return ids
}

func TestDoModeRendersHeadingsBlanksAndItemsInOrder(t *testing.T) {
	_, m := openSampleDo(t)
	view := stripANSI(m.View())
	first := strings.Split(view, "\n")
	if !strings.HasPrefix(first[0], "crusade") || !strings.HasSuffix(strings.TrimRight(first[0], " "), "DO") || !strings.Contains(first[1], "0 deleted this week") {
		t.Fatalf("header:\n%s", view)
	}
	want := []string{"> # Today", "  - mail the letter", "  ", "  # Later", "  - file the form"}
	if got := doListLines(m); !slices.Equal(got, want) {
		t.Fatalf("rows:\n%q\nwant\n%q", got, want)
	}
	kinds := []string{model.DoKindHeading, model.DoKindItem, model.DoKindBlank, model.DoKindHeading, model.DoKindItem}
	for index, row := range m.rows {
		if row.Kind != kinds[index] {
			t.Fatalf("row %d kind %q want %q", index, row.Kind, kinds[index])
		}
	}
	m = keyDo(t, m, runeKey('j'))
	if got := doListLines(m); got[0] != "  # Today" || got[1] != "> - mail the letter" {
		t.Fatalf("j did not move the cursor:\n%q", got)
	}
	m = keyDo(t, m, runeKey('G'))
	if m.cursor != 4 {
		t.Fatalf("G cursor %d", m.cursor)
	}
	m = keyDo(t, m, runeKey('j'))
	if m.cursor != 4 {
		t.Fatalf("j past the end cursor %d", m.cursor)
	}
	m = keyDo(t, m, runeKey('k'))
	m = keyDo(t, m, runeKey('g'))
	if m.cursor != 0 {
		t.Fatalf("g cursor %d", m.cursor)
	}
}

func TestDoModeEmptyList(t *testing.T) {
	isolateSQLX(t)
	db := filepath.Join(t.TempDir(), "crusade.sqlite")
	m, err := openDoModel(db)
	if err != nil {
		t.Fatal(err)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "Empty list. Press n to add an item.") || !strings.Contains(view, "0 deleted this week") {
		t.Fatalf("empty:\n%s", view)
	}
	m = keyDo(t, m, runeKey('e'))
	if m.composing || m.status != "No line selected to edit." {
		t.Fatalf("e on empty composing=%v status=%q", m.composing, m.status)
	}
	m = keyDo(t, m, runeKey('n'))
	m = typeCompose(t, m, "first thing")
	m = keyDo(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.status != "Added." || len(m.rows) != 1 || storedDoText(t, db) != "- first thing\n" {
		t.Fatalf("status=%q rows=%#v", m.status, m.rows)
	}
}

func TestDoNewItemGoesAfterTheCursorRow(t *testing.T) {
	db, m := openSampleDo(t)
	before := rowIDs(m.rows)
	m = keyDo(t, m, runeKey('j'))
	m = keyDo(t, m, runeKey('n'))
	if !m.composing || m.composeText != "" || m.status != "New item. Enter to save, esc to cancel." {
		t.Fatalf("compose=%v text=%q status=%q", m.composing, m.composeText, m.status)
	}
	m = typeCompose(t, m, "call mom")
	if storedDoText(t, db) != sampleDoList {
		t.Fatal("typing stored the line before enter")
	}
	if !strings.Contains(stripANSI(m.View()), "line: call mom█") {
		t.Fatalf("compose line:\n%s", stripANSI(m.View()))
	}
	m = keyDo(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	want := "# Today\n- mail the letter\n- call mom\n\n# Later\n- file the form\n"
	if got := storedDoText(t, db); got != want {
		t.Fatalf("stored:\n%s\nwant\n%s", got, want)
	}
	if m.composing || m.status != "Added." || m.cursor != 2 || m.rows[2].Body != "call mom" {
		t.Fatalf("composing=%v status=%q cursor=%d rows=%#v", m.composing, m.status, m.cursor, m.rows)
	}
	after := rowIDs(m.rows)
	if kept := slices.Delete(slices.Clone(after), 2, 3); !slices.Equal(kept, before) || slices.Contains(before, after[2]) {
		t.Fatalf("ids moved: before %v after %v", before, after)
	}
	if got := doListLines(m); got[2] != "> - call mom" || got[3] != "  " || got[4] != "  # Later" {
		t.Fatalf("view rows:\n%q", got)
	}
}

func TestDoComposeEscAndEmptyLineStoreNothing(t *testing.T) {
	db, m := openSampleDo(t)
	m = keyDo(t, m, runeKey('n'))
	m = keyDo(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.composing || m.status != "Line cannot be empty." {
		t.Fatalf("empty enter composing=%v status=%q", m.composing, m.status)
	}
	m = typeCompose(t, m, "never")
	m = keyDo(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.composing || m.composeText != "" || m.status != "" {
		t.Fatalf("esc composing=%v text=%q status=%q", m.composing, m.composeText, m.status)
	}
	if got := storedDoText(t, db); got != sampleDoList {
		t.Fatalf("stored:\n%s", got)
	}
}

func TestQTypesWhileComposing(t *testing.T) {
	m := doModel{composing: true, composeText: "mail", composeCursor: 4}
	next, cmd := m.Update(runeKey('q'))
	got, ok := next.(doModel)
	if !ok || cmd != nil || !got.composing || got.composeText != "mailq" {
		t.Fatalf("text=%q composing=%v cmd=%v", got.composeText, got.composing, cmd != nil)
	}
	next, cmd = got.Update(tea.KeyMsg{Type: tea.KeyTab})
	got, ok = next.(doModel)
	if !ok || cmd != nil || !got.composing || got.composeText != "mailq" {
		t.Fatalf("tab left compose type=%T text=%q", next, got.composeText)
	}
}

func TestDoEditKeepsTheID(t *testing.T) {
	db, m := openSampleDo(t)
	m = keyDo(t, m, runeKey('j'))
	id := m.rows[1].ID
	m = keyDo(t, m, runeKey('e'))
	if !m.composing || m.composeID != id || m.composeText != "mail the letter" || m.composeCursor != len("mail the letter") {
		t.Fatalf("edit prefill id=%d text=%q cursor=%d", m.composeID, m.composeText, m.composeCursor)
	}
	m = keyDo(t, m, tea.KeyMsg{Type: tea.KeyCtrlU})
	m = typeCompose(t, m, "post the parcel")
	m = keyDo(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.composing || m.status != "Updated." || m.cursor != 1 || m.rows[1].ID != id || m.rows[1].Body != "post the parcel" {
		t.Fatalf("status=%q cursor=%d rows=%#v", m.status, m.cursor, m.rows)
	}
	want := "# Today\n- post the parcel\n\n# Later\n- file the form\n"
	if got := storedDoText(t, db); got != want {
		t.Fatalf("stored:\n%s", got)
	}

	m = keyDo(t, m, runeKey('k'))
	headingID := m.rows[0].ID
	m = keyDo(t, m, runeKey('i'))
	if m.composeText != "# Today" {
		t.Fatalf("heading prefill %q", m.composeText)
	}
	m = keyDo(t, m, tea.KeyMsg{Type: tea.KeyCtrlU})
	m = typeCompose(t, m, "# Now")
	m = keyDo(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.status != "Updated." || m.rows[0].ID != headingID || m.rows[0].Kind != model.DoKindHeading || m.rows[0].Body != "# Now" {
		t.Fatalf("heading status=%q row=%#v", m.status, m.rows[0])
	}

	m = keyDo(t, m, runeKey('j'))
	m = keyDo(t, m, runeKey('j'))
	m = keyDo(t, m, runeKey('e'))
	if m.composing || m.status != "No line selected to edit." {
		t.Fatalf("blank edit composing=%v status=%q", m.composing, m.status)
	}
	if got := storedDoText(t, db); got != "# Now\n- post the parcel\n\n# Later\n- file the form\n" {
		t.Fatalf("stored:\n%s", got)
	}
}

func TestDoEditHeadingNeedsAHash(t *testing.T) {
	db, m := openSampleDo(t)
	m = keyDo(t, m, runeKey('e'))
	m = keyDo(t, m, tea.KeyMsg{Type: tea.KeyCtrlU})
	m = typeCompose(t, m, "Now")
	m = keyDo(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.composing || !strings.HasPrefix(m.status, "error:") || !strings.Contains(m.status, "#") {
		t.Fatalf("composing=%v status=%q", m.composing, m.status)
	}
	if got := storedDoText(t, db); got != sampleDoList {
		t.Fatalf("stored:\n%s", got)
	}
}

func TestDoDDDeletesAndCountsTheWeek(t *testing.T) {
	db, m := openSampleDo(t)
	m = keyDo(t, m, runeKey('j'))
	m = keyDo(t, m, runeKey('d'))
	if !m.pendingDelete || m.status != "Press d again to delete the selected line." || len(m.rows) != 5 {
		t.Fatalf("first d pending=%v status=%q", m.pendingDelete, m.status)
	}
	m = keyDo(t, m, runeKey('j'))
	if m.pendingDelete {
		t.Fatal("j kept the pending delete")
	}
	m = keyDo(t, m, runeKey('k'))
	m = keyDo(t, m, runeKey('d'))
	m = keyDo(t, m, runeKey('d'))
	view := stripANSI(m.View())
	if m.status != "Deleted." || !strings.Contains(view, "1 deleted this week") || strings.Contains(view, "mail the letter") {
		t.Fatalf("status=%q\n%s", m.status, view)
	}
	if got := storedDoText(t, db); got != "# Today\n\n# Later\n- file the form\n" {
		t.Fatalf("stored:\n%s", got)
	}

	m = keyDo(t, m, runeKey('g'))
	m = keyDo(t, m, runeKey('d'))
	m = keyDo(t, m, runeKey('d'))
	if m.status != "Deleted." || !strings.Contains(stripANSI(m.View()), "1 deleted this week") {
		t.Fatalf("heading delete counted:\n%s", stripANSI(m.View()))
	}
	if got := storedDoText(t, db); got != "\n# Later\n- file the form\n" {
		t.Fatalf("stored:\n%q", got)
	}
	checkAlignDeleteKeepsDoCount(t, db, m)
}

// checkAlignDeleteKeepsDoCount deletes an align record and checks the do week line.
func checkAlignDeleteKeepsDoCount(t *testing.T, db string, m doModel) {
	t.Helper()
	align, err := model.NewAlign("2026-06-01", "align outcome", "because")
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.AddRecord(db, align)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(db, id); err != nil {
		t.Fatal(err)
	}
	if err := m.reload(); err != nil {
		t.Fatal(err)
	}
	if count, err := store.DeletedThisWeek(db, time.Now()); err != nil || count != 1 || !strings.Contains(stripANSI(m.View()), "1 deleted this week") {
		t.Fatalf("align delete changed the do count to %d err=%v:\n%s", count, err, stripANSI(m.View()))
	}
}

func TestDoQuitAndHelpKeys(t *testing.T) {
	for _, key := range []tea.KeyMsg{runeKey('q'), {Type: tea.KeyEsc}, {Type: tea.KeyCtrlC}} {
		if _, cmd := (doModel{}).Update(key); !isQuitCommand(cmd) {
			t.Fatalf("%s did not quit", key.String())
		}
	}
	m := doModel{width: 100, height: 36}
	m = keyDo(t, m, runeKey('?'))
	view := stripANSI(m.View())
	if !m.showHelp || !strings.Contains(view, "New item after the line") || !strings.HasPrefix(view, "crusade") {
		t.Fatalf("help:\n%s", view)
	}
	m = keyDo(t, m, runeKey('j'))
	if !m.showHelp {
		t.Fatal("j closed the guide")
	}
	m = keyDo(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.showHelp || strings.Contains(m.View(), "New item after the line") {
		t.Fatal("esc did not close the guide")
	}
}

func TestDoTabOpensAlign(t *testing.T) {
	db, m := openSampleDo(t)
	if err := store.Add(db, mustEvent(t, "2026-06-14", "ship the patch", "reduce drift")); err != nil {
		t.Fatal(err)
	}
	m.width = 60
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	align, ok := next.(appModel)
	if !ok || cmd != nil || align.width != 60 {
		t.Fatalf("type=%T cmd=%v", next, cmd != nil)
	}
	view := stripANSI(align.View())
	if !strings.Contains(view, "ALIGN") || !strings.Contains(view, "ship the patch") || strings.Contains(view, "mail the letter") {
		t.Fatalf("align view:\n%s", view)
	}
	if mode, err := store.LastMode(db); err != nil || mode != model.ModeAlign {
		t.Fatalf("last mode %q err=%v", mode, err)
	}
	if strings.Contains(stripANSI(m.View()), "ship the patch") {
		t.Fatalf("do view shows align records:\n%s", stripANSI(m.View()))
	}
}

func mustEvent(t *testing.T, x, y, z string) model.Align {
	t.Helper()
	event, err := model.NewAlign(x, y, z)
	if err != nil {
		t.Fatal(err)
	}
	return event
}
