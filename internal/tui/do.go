package tui

import (
	"fmt"
	"io"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ryangerardwilson/crusade/internal/lineedit"
	"github.com/ryangerardwilson/crusade/internal/model"
	"github.com/ryangerardwilson/crusade/internal/store"
)

// doModel is do mode: the do list rows, headings and blank lines included.
// rows is a cache of the store's answer, reloaded after every write. The
// line being typed is local until enter.
type doModel struct {
	dataPath      string
	rows          []model.DoItem
	weekDeleted   int
	cursor        int
	composing     bool
	composeID     int
	composeText   string
	composeCursor int
	pendingDelete bool
	showHelp      bool
	status        string
	width         int
	height        int
	memory        modeMemory
}

func runDo(dataPath string, stdout, stderr io.Writer) Result {
	m, err := openDoModel(dataPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return Result{Code: 1}
	}
	return runProgram(m, stderr)
}

func openDoModel(dataPath string) (doModel, error) {
	m := doModel{dataPath: dataPath}
	return m, m.reload()
}

func (m *doModel) reload() error {
	rows, err := store.LoadDoList(m.dataPath)
	if err != nil {
		return err
	}
	count, err := store.DeletedThisWeek(m.dataPath, time.Now())
	if err != nil {
		return err
	}
	m.rows = rows
	m.weekDeleted = count
	m.cursor = clamp(m.cursor, 0, max(0, len(rows)-1))
	return nil
}

func (m doModel) Init() tea.Cmd { return nil }

func (m doModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch message := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = message.Width
		m.height = message.Height
	case tea.KeyMsg:
		return m.handleKey(message.String())
	}
	return m, nil
}

func (m doModel) handleKey(key string) (tea.Model, tea.Cmd) {
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	if m.composing {
		m.handleComposeKey(key)
		return m, nil
	}
	if m.showHelp {
		m.showHelp = key != "?" && key != "esc"
		return m, nil
	}
	if key != "d" {
		m.pendingDelete = false
	}
	switch key {
	case "q", "esc":
		return m, tea.Quit
	case "tab":
		return m.cycleMode()
	case "?":
		m.showHelp = true
	case "j", "down":
		m.cursor = clamp(m.cursor+1, 0, max(0, len(m.rows)-1))
	case "k", "up":
		m.cursor = clamp(m.cursor-1, 0, max(0, len(m.rows)-1))
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = max(0, len(m.rows)-1)
	case "n":
		m.beginNewTask()
	case "e", "i":
		m.beginEditTask()
	case "d":
		m.handleDelete()
	}
	return m, nil
}

func (m doModel) cycleMode() (tea.Model, tea.Cmd) {
	records, err := store.Load(m.dataPath)
	if err != nil {
		m.status = "error: " + err.Error()
		return m, nil
	}
	next := newAppModel(m.dataPath, records)
	next.width = m.width
	next.height = m.height
	next.memory = m.memory
	if m.memory.hasAlign {
		next.restoreSession(m.memory.align)
	}
	next.clampState()
	rememberMode(m.dataPath, model.ModeAlign)
	return next, nil
}

func (m doModel) selected() (model.DoItem, bool) {
	if len(m.rows) == 0 {
		return model.DoItem{}, false
	}
	return m.rows[clamp(m.cursor, 0, len(m.rows)-1)], true
}

func (m *doModel) beginNewTask() {
	m.composing = true
	m.composeID = 0
	m.composeText = ""
	m.composeCursor = 0
	m.status = "New item. Enter to save, esc to cancel."
}

func (m *doModel) beginEditTask() {
	row, ok := m.selected()
	if !ok || row.Kind == model.DoKindBlank {
		m.status = "No line selected to edit."
		return
	}
	m.composing = true
	m.composeID = row.ID
	m.composeText = row.Body
	m.composeCursor = len([]rune(row.Body))
	m.status = "Edit line. Enter to save, esc to cancel."
}

func (m *doModel) cancelCompose() {
	m.composing = false
	m.composeID = 0
	m.composeText = ""
	m.composeCursor = 0
	m.status = ""
}

func (m *doModel) handleComposeKey(key string) {
	switch key {
	case "esc":
		m.cancelCompose()
	case "enter":
		m.commitCompose()
	default:
		m.composeText, m.composeCursor, _ = lineedit.Key(m.composeText, m.composeCursor, key)
	}
}

func (m *doModel) insertCompose(text string) {
	m.composeText, m.composeCursor = lineedit.Insert(m.composeText, m.composeCursor, text)
}

func (m *doModel) commitCompose() {
	text := strings.TrimSpace(m.composeText)
	if text == "" {
		m.status = "Line cannot be empty."
		return
	}
	if m.composeID != 0 {
		m.apply(store.UpdateDoItem(m.dataPath, m.composeID, text), "Updated.", m.composeID)
		return
	}
	after := 0
	if row, ok := m.selected(); ok {
		after = row.ID
	}
	id, err := store.AddDoItem(m.dataPath, after, text)
	m.apply(err, "Added.", id)
}

func (m *doModel) handleDelete() {
	row, ok := m.selected()
	if !ok {
		m.status = "No line selected."
		return
	}
	if !m.pendingDelete {
		m.pendingDelete = true
		m.status = "Press d again to delete the selected line."
		return
	}
	m.pendingDelete = false
	m.apply(store.DeleteDoItem(m.dataPath, row.ID), "Deleted.", 0)
}

// apply shows the outcome of a store write, reloads the rows, and selects id.
func (m *doModel) apply(err error, status string, id int) {
	if err == nil {
		err = m.reload()
	}
	if err != nil {
		m.status = "error: " + err.Error()
		return
	}
	m.composing = false
	m.composeID = 0
	m.composeText = ""
	m.composeCursor = 0
	m.status = status
	for index, row := range m.rows {
		if id != 0 && row.ID == id {
			m.cursor = index
		}
	}
}

func (m doModel) View() string {
	width := m.width
	if width <= 0 {
		width = 100
	}
	height := m.height
	if height <= 0 {
		height = 30
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", renderModeBanner("DO", width))
	fmt.Fprintf(&b, "%s\n", infoStyle.Render(truncateText(fmt.Sprintf("%d deleted this week", m.weekDeleted), max(20, width))))
	b.WriteString(m.renderRows(max(1, height-5)))
	if m.composing {
		fmt.Fprintf(&b, "\n%s\n", infoStyle.Render("line: "+composeShown(m.composeText, m.composeCursor)))
	}
	if m.status != "" {
		style := mutedStyle
		if strings.HasPrefix(m.status, "error:") {
			style = errorStyle
		}
		fmt.Fprintf(&b, "\n%s\n", style.Render(m.status))
	}
	if m.showHelp {
		return placeModal(b.String(), m.helpView(width), width, height)
	}
	return b.String()
}

func (m doModel) renderRows(height int) string {
	if len(m.rows) == 0 {
		return "Empty list. Press n to add an item.\n"
	}
	start := 0
	if len(m.rows) > height {
		start = max(0, m.cursor-height+1)
	}
	var b strings.Builder
	for index := start; index < len(m.rows) && index < start+height; index++ {
		row := m.rows[index]
		line := row.Line()
		if row.Kind == model.DoKindHeading {
			line = sectionStyle.Render(line)
		}
		if index == m.cursor {
			line = titleStyle.Render("> ") + line
		} else {
			line = "  " + line
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func (m doModel) helpView(width int) string {
	innerWidth := min(72, max(32, width-8))
	items := []helpItem{
		{key: "j / k", description: "Move line"},
		{key: "n", description: "New item after the line"},
		{key: "e", description: "Edit the line"},
		{key: "dd", description: "Delete the line"},
		{key: "tab", description: "Align / box / cal / nlp"},
		{key: "q / esc", description: "Leave"},
	}
	body := renderHelpSection("DO", items, innerWidth) + "\n\n" +
		mutedStyle.Render("While typing: ctrl+a/e/b/f/h/d, alt+b/f")
	return helpPanelStyle.Width(innerWidth).Render(body)
}
