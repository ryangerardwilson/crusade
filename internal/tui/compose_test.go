package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func pressCompose(t *testing.T, m appModel, keys ...tea.KeyMsg) appModel {
	t.Helper()
	for _, key := range keys {
		next, _ := m.Update(key)
		m = next.(appModel)
	}
	return m
}

func altRune(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: true}
}

func TestDoComposeBlockCursorAndEmacsKeys(t *testing.T) {
	isolateSQLX(t)
	forceStyledOutput(t)
	m := appModel{view: viewAgenda, mode: modeDo, width: 80, height: 24}
	m.beginNewTask()
	for _, r := range "go snorkelling" {
		key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
		if r == ' ' {
			key.Type = tea.KeySpace
		}
		m = pressCompose(t, m, key)
	}
	if m.composeText != "go snorkelling" {
		t.Fatalf("typed %q", m.composeText)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "go snorkelling█") || strings.Contains(view, "▌") {
		t.Fatalf("end cursor is not a block:\n%s", view)
	}

	m = pressCompose(t, m, tea.KeyMsg{Type: tea.KeyCtrlA})
	if m.composeCursor != 0 || !strings.Contains(m.View(), cursorStyle.Render("g")+"o snorkelling") {
		t.Fatalf("ctrl+a cursor %d is not a reversed cell", m.composeCursor)
	}
	if strings.Contains(stripANSI(m.View()), "█") {
		t.Fatal("cursor on a character drew a trailing block")
	}

	m = pressCompose(t, m, altRune('f'))
	if m.composeCursor != 2 {
		t.Fatalf("alt+f cursor %d", m.composeCursor)
	}
	m = pressCompose(t, m, tea.KeyMsg{Type: tea.KeyCtrlE}, altRune('b'))
	if m.composeCursor != 3 || !strings.Contains(m.View(), "go "+cursorStyle.Render("s")+"norkelling") {
		t.Fatalf("alt+b cursor %d", m.composeCursor)
	}
	m = pressCompose(t, m, tea.KeyMsg{Type: tea.KeyCtrlE}, tea.KeyMsg{Type: tea.KeyCtrlH})
	if m.composeText != "go snorkellin" || m.composeCursor != 13 {
		t.Fatalf("ctrl+h text %q cursor %d", m.composeText, m.composeCursor)
	}
}
