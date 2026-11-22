package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func pressCompose(t *testing.T, m doModel, keys ...tea.KeyMsg) doModel {
	t.Helper()
	for _, key := range keys {
		next, cmd := m.Update(key)
		if cmd != nil {
			t.Fatalf("command on %s while composing", key.String())
		}
		m = next.(doModel)
	}
	return m
}

func altRune(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: true}
}

func typeCompose(t *testing.T, m doModel, text string) doModel {
	t.Helper()
	for _, r := range text {
		key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
		if r == ' ' {
			key.Type = tea.KeySpace
		}
		m = pressCompose(t, m, key)
	}
	return m
}

func TestDoComposeBlockCursorAndEmacsKeys(t *testing.T) {
	isolateSQLX(t)
	forceStyledOutput(t)
	m := doModel{width: 80, height: 24}
	m.beginNewTask()
	m = typeCompose(t, m, "go snorkelling")
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

func TestComposeEmacsMotion(t *testing.T) {
	m := doModel{composing: true, composeText: "go snorkelling", composeCursor: 0}
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
	m := doModel{composing: true, composeText: "ab", composeCursor: 0}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	got, ok := next.(doModel)
	if !ok || cmd != nil || !got.composing || got.composeText != "b" {
		t.Fatalf("type=%T composing=%v text=%q", next, got.composing, got.composeText)
	}
}
