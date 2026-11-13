package app

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/ryangerardwilson/crusade/nlp/internal/course"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestViewsFillTerminalHeight(t *testing.T) {
	m := testModel()
	m.width = 40
	m.height = 12

	for name, configure := range map[string]func(*model){
		"list": func(m *model) {
			m.screen = screenCourses
		},
		"doc": func(m *model) {
			m.screen = screenDoc
		},
		"type": func(m *model) {
			m.startTyping()
		},
		"help": func(m *model) {
			m.before = screenDoc
			m.screen = screenHelp
		},
	} {
		t.Run(name, func(t *testing.T) {
			current := m
			configure(&current)
			if got := renderedLineCount(current.View()); got != current.height {
				t.Fatalf("View() line count = %d, want %d\n%s", got, current.height, current.View())
			}
		})
	}
}

func TestTypeModeRendersTypedTextOverTargetLine(t *testing.T) {
	m := testModel()
	m.width = 40
	m.height = 12
	m.startTyping()
	m.typed = "abc"
	m.cursorShown = false

	view := stripANSI(m.View())
	if strings.Contains(view, "Typed:") {
		t.Fatalf("type mode should not render typed text below the target:\n%s", view)
	}
	if !strings.Contains(view, "abcdef") {
		t.Fatalf("typed overlay should preserve the target line with typed prefix over it:\n%s", view)
	}
}

func TestTypeModeRendersBlockCursorOnActiveLine(t *testing.T) {
	m := testModelWithContent("next\nsecond")
	m.width = 40
	m.height = 12
	m.startTyping()
	m.cursorShown = true

	view := stripANSI(m.View())
	if !strings.Contains(view, "█ext") {
		t.Fatalf("type mode should render a block cursor over the active character:\n%s", view)
	}
	if strings.Contains(view, "█econd") {
		t.Fatalf("type mode should render the cursor on the active line only:\n%s", view)
	}
}

func TestTypeModeHidesBlockCursorBetweenBlinks(t *testing.T) {
	m := testModelWithContent("next")
	m.width = 40
	m.height = 12
	m.startTyping()
	m.cursorShown = false

	view := stripANSI(m.View())
	if strings.Contains(view, "█") {
		t.Fatalf("hidden cursor frame should not render a block cursor:\n%s", view)
	}
	if !strings.Contains(view, "next") {
		t.Fatalf("hidden cursor frame should render the target text:\n%s", view)
	}
}

func TestTypeModeAllowsTypingNBeforeCompletion(t *testing.T) {
	m := testModelWithContent("next")
	m.startTyping()
	m.cursorShown = false

	updated, _ := m.updateTypeKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = updated.(model)

	if got := m.currentTypeInput(); got != "n" {
		t.Fatalf("typed input = %q, want n", got)
	}
	if m.typeDone {
		t.Fatalf("typing n should not complete or advance the lesson before the target is complete")
	}
	if !m.cursorShown {
		t.Fatalf("typing should make the block cursor visible immediately")
	}
}

func TestCtrlRDoesNotRestartTyping(t *testing.T) {
	m := testModelWithContent("alpha\nbeta")
	m.startTyping()
	m.setCurrentTypeInput("alp")

	updated, _ := m.updateTypeKey(tea.KeyMsg{Type: tea.KeyCtrlR})
	m = updated.(model)

	if m.typeLine != 0 || m.currentTypeInput() != "alp" || m.status == "typing reset" {
		t.Fatalf("ctrl+r restarted the lesson: line %d input %q status %q", m.typeLine, m.currentTypeInput(), m.status)
	}
}

func TestCtrlNSkipsTheLineAsDone(t *testing.T) {
	m := testModelWithContent("alpha\nbeta")
	m.width = 40
	m.height = 12
	m.startTyping()
	m.setCurrentTypeInput("al")

	updated, _ := m.updateTypeKey(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = updated.(model)

	if m.typeLine != 1 || m.typeInputs[0] != "alpha" || m.typeDone {
		t.Fatalf("skip line %d inputs %#v done %v", m.typeLine, m.typeInputs, m.typeDone)
	}
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
	want := statusStyle.Render("alpha")
	if got := m.renderTypeLine(0); got != want || !strings.Contains(got, "38;5;80") {
		t.Fatalf("skipped line %q", got)
	}
}

func TestCtrlNCompletesTheLastLine(t *testing.T) {
	m := testModelWithContent("alpha\nbeta")
	m.startTyping()
	updated, _ := m.updateTypeKey(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = updated.(model)
	updated, _ = m.updateTypeKey(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = updated.(model)

	if !m.typeDone || m.typeLine != 1 || m.typeInputs[1] != "beta" {
		t.Fatalf("last line line %d inputs %#v done %v", m.typeLine, m.typeInputs, m.typeDone)
	}
}

func TestCtrlPReturnsToThePreviousLine(t *testing.T) {
	m := testModelWithContent("alpha\nbeta\ngamma")
	m.startTyping()
	updated, _ := m.updateTypeKey(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = updated.(model)
	updated, _ = m.updateTypeKey(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = updated.(model)

	updated, _ = m.updateTypeKey(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = updated.(model)

	if m.typeLine != 1 || m.typeInputs[0] != "alpha" || m.typeInputs[1] != "" || m.typeInputs[2] != "" || m.typeDone {
		t.Fatalf("back line %d inputs %#v done %v", m.typeLine, m.typeInputs, m.typeDone)
	}
	if m.currentTypeInput() != "" {
		t.Fatalf("current input %q", m.currentTypeInput())
	}
	if stripANSI(m.renderTypeLine(1)) != "\u258ceta" && stripANSI(m.renderTypeLine(1)) != "\u2588eta" {
		t.Fatalf("cursor %q", stripANSI(m.renderTypeLine(1)))
	}
}

func TestCtrlPReopensAFinishedLesson(t *testing.T) {
	m := testModelWithContent("alpha")
	m.startTyping()
	updated, _ := m.updateTypeKey(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = updated.(model)
	if !m.typeDone {
		t.Fatal("lesson did not complete")
	}

	updated, _ = m.updateTypeKey(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = updated.(model)

	if m.typeDone || m.typeLine != 0 || m.typeInputs[0] != "" || m.currentTypeInput() != "" {
		t.Fatalf("reopen line %d inputs %#v done %v input %q", m.typeLine, m.typeInputs, m.typeDone, m.currentTypeInput())
	}
}

func TestCtrlPOnTheFirstLineStays(t *testing.T) {
	m := testModelWithContent("alpha\nbeta")
	m.startTyping()
	m.setCurrentTypeInput("al")

	updated, _ := m.updateTypeKey(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = updated.(model)

	if m.typeLine != 0 || m.currentTypeInput() != "" {
		t.Fatalf("first line line %d input %q", m.typeLine, m.currentTypeInput())
	}
}

func TestTypeModeKeepsLessonLinesVisible(t *testing.T) {
	m := testModel()
	m.width = 40
	m.height = 12
	m.startTyping()
	m.cursorShown = false

	view := stripANSI(m.View())
	for _, want := range []string{"abcdef", "second line", "third line"} {
		if !strings.Contains(view, want) {
			t.Fatalf("type mode should keep surrounding lesson lines visible; missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Line:") {
		t.Fatalf("type mode should render the lesson viewport, not a single line prompt:\n%s", view)
	}
}

func TestTypeModeScrollsAroundCurrentLine(t *testing.T) {
	m := testModelWithContent(strings.Join([]string{
		"line 01",
		"line 02",
		"line 03",
		"line 04",
		"line 05",
		"line 06",
		"line 07",
		"line 08",
		"line 09",
		"line 10",
		"line 11",
		"line 12",
	}, "\n"))
	m.width = 40
	m.height = 10
	m.startTyping()
	m.typeLine = 8
	m.cursorShown = false

	view := stripANSI(m.View())
	for _, want := range []string{"line 08", "line 09", "line 12"} {
		if !strings.Contains(view, want) {
			t.Fatalf("type mode should keep the current line and nearby lines visible; missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "line 01") {
		t.Fatalf("type mode should scroll long lessons instead of pinning the first line:\n%s", view)
	}
}

func TestStartTypingKeepsBlankAndSkipLines(t *testing.T) {
	m := testModelWithContent("first\n\n//! run this outside typing\nlast\n")
	m.startTyping()

	if got, want := len(m.typeLines), 4; got != want {
		t.Fatalf("typeLines length = %d, want %d: %#v", got, want, m.typeLines)
	}
	if !m.typeSkip[2] {
		t.Fatalf("skip line was not marked: %#v", m.typeSkip)
	}
	_, total := m.typeStats()
	if total != len("first")+len("last") {
		t.Fatalf("typeStats total = %d, want non-skip characters only", total)
	}
}

func testModel() model {
	return testModelWithContent("abcdef\nsecond line\nthird line")
}

func testModelWithContent(content string) model {
	return newModel(Config{
		Courses: []course.Course{{
			Name: "Course",
			Parts: []course.Part{{
				Name: "Main",
				Sections: []course.Section{{
					Name: "Main",
					Lessons: []course.Lesson{{
						Name:    "Lesson",
						Content: content,
					}},
				}},
			}},
		}},
		InitialSequenceName: "Course",
		InitialLessons: []course.Lesson{{
			Name:    "Lesson",
			Content: content,
		}},
	})
}

func renderedLineCount(value string) int {
	return strings.Count(value, "\n") + 1
}

func stripANSI(value string) string {
	return ansiRE.ReplaceAllString(value, "")
}

func TestTypingKeysDoNotRewriteTheSavedPlace(t *testing.T) {
	m := testModel()
	m.savePlace = true
	calls := 0
	previous := persistPlace
	persistPlace = func(string, string) error {
		calls++
		return nil
	}
	t.Cleanup(func() { persistPlace = previous })

	opened := m.remember()
	if calls != 1 {
		t.Fatalf("open saves = %d", calls)
	}
	next, _ := opened.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	typed := next.(model)
	next, _ = typed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if calls != 1 {
		t.Fatalf("typing saves = %d", calls)
	}
	left := next.(model)
	left.screen = screenCourses
	_ = left.remember()
	if calls != 2 {
		t.Fatalf("leave saves = %d", calls)
	}
}
