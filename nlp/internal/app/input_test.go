package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/ryangerardwilson/crusade/nlp/internal/course"
	"github.com/ryangerardwilson/crusade/nlp/internal/state"
)

func altKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: true}
}

func TestRenameFieldBlockCursorAndEmacsKeys(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	persistCourse = func(slug, name, body string) error { return nil }
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previous)
		persistCourse = state.SaveCourse
	})
	parsed, ok := course.ParseMarkdown("alpha", menuBody)
	if !ok {
		t.Fatal("fixture did not parse")
	}
	m := newModel(Config{Courses: []course.Course{parsed}})
	m.width = 80
	m.height = 24
	m.screen = screenCourses
	m.selected = 0

	m = press(m, runeKey(','), runeKey('r'), runeKey('n'))
	if m.input != "Next" || m.inputCursor != 4 {
		t.Fatalf("rename input %q cursor %d", m.input, m.inputCursor)
	}
	if view := stripANSI(m.View()); !strings.Contains(view, "rename Next█") || strings.Contains(view, "▌") {
		t.Fatalf("end cursor is not a block:\n%s", view)
	}

	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlA}, runeKey('A'))
	if m.input != "ANext" || m.inputCursor != 1 {
		t.Fatalf("ctrl+a input %q cursor %d", m.input, m.inputCursor)
	}
	if !strings.Contains(m.status, "A"+cursorStyle.Render("N")+"ext") || strings.Contains(stripANSI(m.View()), "█") {
		t.Fatalf("cursor on a character is not reversed: %q", m.status)
	}

	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlE}, tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}, runeKey('g'), runeKey('o'))
	if m.input != "ANext go" || m.inputCursor != 8 {
		t.Fatalf("ctrl+e input %q cursor %d", m.input, m.inputCursor)
	}

	m = press(m, altKey('b'))
	if m.inputCursor != 6 || !strings.Contains(m.status, "ANext "+cursorStyle.Render("g")+"o") {
		t.Fatalf("alt+b cursor %d status %q", m.inputCursor, m.status)
	}
	m = press(m, altKey('b'), altKey('f'))
	if m.inputCursor != 5 {
		t.Fatalf("alt+f cursor %d", m.inputCursor)
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlH})
	if m.input != "ANex go" || m.inputCursor != 4 {
		t.Fatalf("ctrl+h input %q cursor %d", m.input, m.inputCursor)
	}

	m = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.status != "renamed" || m.pages[m.selected].name != "ANex go" {
		t.Fatalf("rename status %q name %q", m.status, m.pages[m.selected].name)
	}
}

func TestNewSessionFieldShowsBlockCursor(t *testing.T) {
	m := newModel(Config{})
	m.width = 80
	m.height = 24
	m.screen = screenCourses
	m = press(m, runeKey('n'), runeKey('a'), runeKey('b'), tea.KeyMsg{Type: tea.KeyCtrlA}, tea.KeyMsg{Type: tea.KeyCtrlD})
	if m.input != "b" || m.inputCursor != 0 {
		t.Fatalf("new input %q cursor %d", m.input, m.inputCursor)
	}
	m = press(m, tea.KeyMsg{Type: tea.KeyCtrlE})
	if view := stripANSI(m.View()); !strings.Contains(view, "new session b█") {
		t.Fatalf("new session cursor:\n%s", view)
	}
}
