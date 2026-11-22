package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ryangerardwilson/crusade/internal/lineedit"
	"github.com/ryangerardwilson/crusade/nlp/internal/course"
	"github.com/ryangerardwilson/crusade/nlp/internal/state"
)

var (
	persistCourse    = state.SaveCourse
	persistNewCourse = state.CreateCourse
	removeCourse     = state.DeleteStoredCourse
)

func (m model) menuScreen() bool {
	return m.screen == screenCourses
}

func (m model) updateInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.inputKind = ""
		m.input = ""
		m.inputCursor = 0
		m.status = "cancelled"
		return m, nil
	case "enter":
		return m, m.commitInput()
	}
	if text, cursor, ok := lineedit.Key(m.input, m.inputCursor, msg.String()); ok {
		m.input, m.inputCursor = text, cursor
	} else if pastedRunes(msg) {
		m.input, m.inputCursor = lineedit.Insert(m.input, m.inputCursor, string(msg.Runes))
	}
	m.paintInput()
	return m, nil
}

func pastedRunes(msg tea.KeyMsg) bool {
	return !msg.Alt && len(msg.Runes) > 0 && !strings.ContainsAny(string(msg.Runes), "\r\n")
}

func (m model) updateLeader(key string) (tea.Model, tea.Cmd) {
	m.leaderSeq += key
	switch {
	case m.leaderSeq == "rn":
		m.leader = false
		m.leaderSeq = ""
		m.beginRename()
	case strings.HasPrefix("rn", m.leaderSeq):
		m.status = ",rn rename"
	default:
		m.leader = false
		m.leaderSeq = ""
		m.status = "unknown leader action"
	}
	return m, nil
}

func (m *model) beginRename() {
	if m.selected < 0 || m.selected >= m.itemCount() {
		m.status = "nothing to rename"
		return
	}
	name := m.itemName(m.selected)
	if name == "" {
		m.status = "nothing to rename"
		return
	}
	m.leader = false
	m.leaderSeq = ""
	m.deleteArmed = false
	m.inputKind = "rename"
	m.input = name
	m.inputCursor = len([]rune(name))
	m.paintInput()
}

func (m *model) beginNew() {
	if m.screen != screenCourses {
		return
	}
	m.newLabel = "new session"
	m.leader = false
	m.leaderSeq = ""
	m.deleteArmed = false
	m.inputKind = "new"
	m.input = ""
	m.inputCursor = 0
	m.paintInput()
}

func (m *model) paintInput() {
	label := "rename"
	if m.inputKind == "new" {
		label = m.newLabel
	}
	m.status = label + " " + lineedit.Show(m.input, m.inputCursor, func(cell string) string { return cursorStyle.Render(cell) })
}

func (m *model) commitInput() tea.Cmd {
	value := strings.TrimSpace(m.input)
	kind := m.inputKind
	m.inputKind = ""
	m.input = ""
	m.inputCursor = 0
	if value == "" {
		m.status = "missing name"
		return nil
	}
	if kind == "rename" {
		m.commitRename(value)
		return nil
	}
	return m.commitNew(value)
}

func (m *model) commitRename(name string) {
	if m.screen == screenCourses {
		m.renameCourse(m.selected, name)
	}
}

func (m *model) commitNew(name string) tea.Cmd {
	if m.screen == screenCourses {
		return m.createCourse(name)
	}
	return nil
}

func (m *model) renameCourse(index int, name string) {
	if index < 0 || index >= len(m.pages) {
		m.status = "nothing to rename"
		return
	}
	p := m.pages[index]
	item, ok := m.courseAt(p.courseIndex)
	if !ok {
		return
	}
	if name == p.name {
		m.status = "unchanged"
		return
	}
	body, err := course.RenameLesson(item.Body, p.partIndex, p.sectionIndex, p.lessonIndex, name)
	if err != nil {
		m.status = err.Error()
		return
	}
	if lessonCount(item) < 2 {
		body, err = course.RenameCourse(body, name)
		if err != nil {
			m.status = err.Error()
			return
		}
	}
	if err := m.storeCourse(p.courseIndex, item.SourceFile, item.Name, body); err != nil {
		m.status = err.Error()
		return
	}
	m.sortCourses()
	m.rebuildPages()
	m.selectStoredPage(item.SourceFile, p.partIndex, p.sectionIndex, p.lessonIndex)
	m.status = "renamed"
}

func (m *model) deleteSelected() {
	if m.screen == screenCourses {
		m.deletePage(m.selected)
	}
}

func (m *model) deletePage(index int) {
	if index < 0 || index >= len(m.pages) {
		m.status = "nothing selected"
		return
	}
	p := m.pages[index]
	item, ok := m.courseAt(p.courseIndex)
	if !ok {
		return
	}
	if lessonCount(item) < 2 {
		m.deleteCourse(p.courseIndex)
		return
	}
	body, err := course.DeleteLesson(item.Body, p.partIndex, p.sectionIndex, p.lessonIndex)
	if err != nil {
		m.status = err.Error()
		return
	}
	if err := m.storeCourse(p.courseIndex, item.SourceFile, item.Name, body); err != nil {
		m.status = err.Error()
		return
	}
	m.rebuildPages()
	if m.selected >= len(m.pages) {
		m.selected = len(m.pages) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
	m.status = "deleted"
}

func (m *model) deleteCourse(index int) {
	item, ok := m.courseAt(index)
	if !ok {
		return
	}
	if item.SourceFile == "" {
		m.status = "course has no stored text"
		return
	}
	if err := removeCourse(item.SourceFile); err != nil {
		m.status = err.Error()
		return
	}
	m.cfg.Courses = append(m.cfg.Courses[:index], m.cfg.Courses[index+1:]...)
	m.rebuildPages()
	if m.selected >= len(m.pages) {
		m.selected = len(m.pages) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
	m.status = "deleted"
}

func (m *model) storeCourse(index int, slug, name, body string) error {
	parsed, ok := course.ParseMarkdown(slug, body)
	if !ok {
		return fmt.Errorf("course would be empty")
	}
	if err := persistCourse(slug, parsed.Name, body); err != nil {
		return err
	}
	parsed.SourceFile = slug
	parsed.Body = body
	if name != "" && parsed.Name == "" {
		parsed.Name = name
	}
	m.cfg.Courses[index] = parsed
	m.rebuildPages()
	return nil
}

func (m *model) courseAt(index int) (course.Course, bool) {
	if index < 0 || index >= len(m.cfg.Courses) {
		m.status = "nothing selected"
		return course.Course{}, false
	}
	item := m.cfg.Courses[index]
	if strings.TrimSpace(item.Body) == "" {
		m.status = "course has no stored text"
		return course.Course{}, false
	}
	return item, true
}

func (m *model) sortCourses() {
	courses := m.cfg.Courses
	for i := 1; i < len(courses); i++ {
		item := courses[i]
		j := i
		for j > 0 && strings.ToLower(courses[j-1].Name) > strings.ToLower(item.Name) {
			courses[j] = courses[j-1]
			j--
		}
		courses[j] = item
	}
	m.cfg.Courses = courses
}
