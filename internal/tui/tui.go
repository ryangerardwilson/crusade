package tui

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ryangerardwilson/crusade/internal/model"
	"github.com/ryangerardwilson/crusade/internal/store"
)

const (
	viewMonth  = "month"
	viewAgenda = "agenda"

	monthFocusGrid   = "grid"
	monthFocusEvents = "events"

	displayDateTime = "2006-01-02 15:04"

	draftOutcome = "EDIT ME: desired outcome"
	draftImpact  = "EDIT ME: why it matters"

	modeDo    = "do"
	modeAlign = "align"

	composeTask = "task"
	composeWhen = "when"
)

type appModel struct {
	dataPath string
	records  []store.Record

	view   string
	width  int
	height int

	agendaCursor int
	agendaCol    int

	monthDate        time.Time
	monthFocus       string
	monthEventCursor int
	monthEventCol    int

	showHelp      bool
	pendingDelete bool
	status        string

	mode          string
	composing     bool
	composeID     int
	composeText   string
	composeCursor int
	composeKind   string

	memory modeMemory
}

// sessionState is the screen a do or align visit returns to.
type sessionState struct {
	view             string
	agendaCursor     int
	agendaCol        int
	monthDate        time.Time
	monthFocus       string
	monthEventCursor int
	monthEventCol    int
	showHelp         bool
	status           string
}

// modeMemory keeps each mode's screen while tab moves between them.
type modeMemory struct {
	do       sessionState
	align    sessionState
	hasDo    bool
	hasAlign bool
	box      *boxModel
	hasBox   bool
	nlp      *nlpMode
	hasNlp   bool
}

type helpItem struct {
	key         string
	description string
}

type editorFinishedMsg struct {
	id    int
	draft bool
	err   error
}

var openTaskEditor = func(path string, id int, draft bool) tea.Cmd {
	fail := func(err error) tea.Cmd {
		return func() tea.Msg { return editorFinishedMsg{id: id, draft: draft, err: err} }
	}
	records, err := store.Load(path)
	if err != nil {
		return fail(err)
	}
	record, found := recordByID(records, id)
	if !found {
		return fail(fmt.Errorf("ID %d not found", id))
	}
	payload, err := json.MarshalIndent(model.ToJSONEvent(record.Event), "", "  ")
	if err != nil {
		return fail(err)
	}
	tmp, err := os.CreateTemp("", "crusade-*.json")
	if err != nil {
		return fail(err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(append(payload, '\n')); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fail(err)
	}
	command := exec.Command("vim", tmpPath)
	return tea.ExecProcess(command, func(err error) tea.Msg {
		defer os.Remove(tmpPath)
		if err != nil {
			return editorFinishedMsg{id: id, draft: draft, err: err}
		}
		edited, readErr := os.ReadFile(tmpPath)
		if readErr != nil {
			return editorFinishedMsg{id: id, draft: draft, err: readErr}
		}
		event, parseErr := model.FromJSON(edited)
		if parseErr != nil {
			return editorFinishedMsg{id: id, draft: draft, err: parseErr}
		}
		event.Done = record.Event.Done
		if upErr := store.Update(path, id, event); upErr != nil {
			return editorFinishedMsg{id: id, draft: draft, err: upErr}
		}
		return editorFinishedMsg{id: id, draft: draft}
	})
}

var (
	titleStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	badgeStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("51"))
	mutedStyle      = lipgloss.NewStyle().Faint(true)
	infoStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("51"))
	sectionStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	activeCellStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("51"))
	todayStyle      = lipgloss.NewStyle().Bold(true).Underline(true).Foreground(lipgloss.Color("51"))
	eventDayStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
	errorStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("204"))
	helpPanelStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("51")).Padding(1, 2)
	helpKeyStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
)

func Run(dataPath, mode string, stdout, stderr io.Writer) int {
	rememberMode(dataPath, mode)
	if mode == model.ModeNLP {
		return runNLP(dataPath, stdout, stderr)
	}
	if mode == model.ModeBox {
		return runBox(dataPath, stdout, stderr)
	}
	records, err := store.Load(dataPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	app := newAppModel(dataPath, records)
	if mode == model.ModeAlign {
		app.mode = modeAlign
	}
	program := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func rememberMode(dataPath, mode string) {
	_ = store.SaveLastMode(dataPath, mode)
}

func newAppModel(dataPath string, records []store.Record) appModel {
	return appModel{
		dataPath:   dataPath,
		records:    records,
		view:       viewAgenda,
		monthDate:  dateOnly(time.Now()),
		monthFocus: monthFocusGrid,
		mode:       modeDo,
	}
}

func (m appModel) Init() tea.Cmd {
	return nil
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch message := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = message.Width
		m.height = message.Height
	case editorFinishedMsg:
		m.finishEditor(message)
	case tea.KeyMsg:
		key := message.String()
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		if m.composing {
			switch key {
			case "esc":
				m.cancelCompose()
			default:
				m.handleComposeKey(key)
			}
			m.clampState()
			return m, nil
		}
		if key == "q" {
			return m, tea.Quit
		}
		if key == "esc" {
			if m.leaveDoTaskList() {
				m.clampState()
				return m, nil
			}
			return m, tea.Quit
		}
		if key == "tab" {
			return m.cycleMode()
		}
		if m.showHelp {
			switch key {
			case "?":
				m.showHelp = false
			}
			return m, nil
		}
		switch key {
		case "?":
			m.showHelp = true
		case "a":
			m.toggleView()
		case "t":
			m.jumpToday()
		case "d":
			m.handleDelete()
		case "n":
			if !m.aligning() {
				m.beginNewTask()
				return m, nil
			}
			cmd := m.addTaskInVim()
			return m, cmd
		case "e", "i":
			if !m.aligning() {
				m.beginEditTask()
				return m, nil
			}
			cmd := m.editTaskInVim()
			return m, cmd
		case "r":
			if !m.aligning() {
				m.beginReschedule()
				return m, nil
			}
			m.pendingDelete = false
			if m.view == viewAgenda {
				m.handleAgendaKey(key)
			} else {
				m.handleMonthKey(key)
			}
		default:
			m.pendingDelete = false
			if m.view == viewAgenda {
				m.handleAgendaKey(key)
			} else {
				m.handleMonthKey(key)
			}
		}
	}
	m.clampState()
	return m, nil
}

func (m appModel) View() string {
	width := m.width
	if width <= 0 {
		width = 100
	}
	height := m.height
	if height <= 0 {
		height = 30
	}
	if m.showHelp {
		return m.helpView(width)
	}

	var b strings.Builder
	weekLine := m.renderWeekDone(width)
	if m.view == viewAgenda {
		visible := m.visibleRecords()
		fmt.Fprintf(&b, "%s\n", m.renderBanner(width))
		reserved := 5
		if weekLine != "" {
			fmt.Fprintf(&b, "%s\n", weekLine)
			reserved++
		}
		fmt.Fprintf(&b, "%s\n", infoStyle.Render(m.agendaSummary(visible, width)))
		b.WriteString(m.renderAgenda(max(1, height-reserved)))
		if m.composing {
			fmt.Fprintf(&b, "\n%s\n", infoStyle.Render(m.composeLabel()+m.composeShown()))
		}
		if m.status != "" {
			fmt.Fprintf(&b, "\n%s\n", m.statusStyle().Render(m.status))
		}
	} else {
		visible := m.visibleRecords()
		fmt.Fprintf(&b, "%s\n", m.renderBanner(width))
		extra := 0
		if weekLine != "" {
			fmt.Fprintf(&b, "%s\n", weekLine)
			extra = 1
		}
		fmt.Fprintf(&b, "%s\n\n", infoStyle.Render(m.monthSummary(visible, width)))
		// Reserved rows keep the calendar in place while a task is typed or rescheduled.
		const monthFooterLines = 2
		b.WriteString(m.renderMonth(width, max(1, height-6-monthFooterLines-extra)))
		b.WriteByte('\n')
		b.WriteString(m.monthComposeLine(width))
		b.WriteByte('\n')
		b.WriteString(m.monthStatusLine(width))
	}
	return b.String()
}

func (m appModel) renderWeekDone(width int) string {
	if m.aligning() {
		return ""
	}
	label := fmt.Sprintf("%d done this week", doneThisWeek(m.records, time.Now()))
	return infoStyle.Render(truncateText(label, max(20, width)))
}

func (m appModel) statusStyle() lipgloss.Style {
	lowered := strings.ToLower(m.status)
	if strings.Contains(lowered, "error") || strings.Contains(lowered, "not found") || strings.Contains(lowered, "failed") {
		return errorStyle
	}
	return mutedStyle
}

func (m appModel) composeLabel() string {
	if m.composeKind == composeWhen {
		return "when: "
	}
	return "task: "
}

func (m appModel) monthComposeLine(width int) string {
	if !m.composing {
		return " "
	}
	return infoStyle.Render(truncateText(m.composeLabel()+m.composeShown(), max(1, width)))
}

func (m appModel) monthStatusLine(width int) string {
	if m.status == "" {
		return " "
	}
	return m.statusStyle().Render(truncateText(m.status, max(1, width)))
}

func renderModeBanner(label string, width int) string {
	if width <= 0 {
		width = 100
	}
	name := titleStyle.Render("crusade")
	badge := badgeStyle.Render(" " + label + " ")
	gap := width - lipgloss.Width(name) - lipgloss.Width(badge)
	if gap < 1 {
		gap = 1
	}
	return name + strings.Repeat(" ", gap) + badge
}

func (m appModel) renderBanner(width int) string {
	label := "DO"
	if m.aligning() {
		label = "ALIGN"
	}
	return renderModeBanner(label, width)
}

func (m appModel) agendaSummary(records []store.Record, width int) string {
	if len(records) == 0 {
		if m.aligning() {
			return "empty agenda | crusade -m align -c"
		}
		return "empty list | press n to add a task"
	}
	idx := clamp(m.agendaCursor, 0, len(records)-1)
	event := records[idx].Event
	parts := []string{
		fmt.Sprintf("selected %d/%d", idx+1, len(records)),
		event.X.Format(displayDateTime),
	}
	if m.aligning() {
		parts = append(parts, fmt.Sprintf("score %s", formatScore(event)))
	}
	return truncateText(strings.Join(parts, " | "), max(20, width))
}

func (m appModel) monthSummary(records []store.Record, width int) string {
	selected := m.monthDate.Format("Mon 2006-01-02")
	dayRecords := m.recordsForDate(m.monthDate)
	parts := []string{selected}
	if len(dayRecords) == 0 {
		if m.aligning() {
			parts = append(parts, "no outcomes")
		} else {
			parts = append(parts, "no tasks")
		}
		if next, ok := nextRecord(records, m.monthDate); ok {
			parts = append(parts, fmt.Sprintf("next %s %s", next.Event.X.Format("Jan 2"), next.Event.Y))
		}
		return truncateText(strings.Join(parts, " | "), max(20, width))
	}
	if m.aligning() {
		parts = append(parts, plural(len(dayRecords), "outcome", "outcomes"))
		parts = append(parts, fmt.Sprintf("top score %s", formatScore(bestRecord(dayRecords).Event)))
	} else {
		parts = append(parts, plural(len(dayRecords), "task", "tasks"))
	}
	return truncateText(strings.Join(parts, " | "), max(20, width))
}

func (m appModel) helpView(width int) string {
	innerWidth := min(72, max(32, width-8))
	general := m.helpGeneral()
	agenda := []helpItem{
		{key: "j / k", description: "Move row"},
		{key: "H / L", description: "Previous / next date"},
	}
	month := []helpItem{
		{key: "h / l", description: "Previous / next day"},
		{key: "j / k", description: "Previous / next week"},
		{key: "ctrl+h/l", description: "Previous / next month"},
		{key: "ctrl+k/j", description: "Previous / next year"},
		{key: "enter", description: "Calendar / day list"},
	}
	if !m.aligning() {
		month = append(month, helpItem{key: "esc", description: "Leave the day list"})
	}
	exit := []helpItem{
		{key: "q", description: "Quit"},
		{key: "esc", description: "Quit"},
		{key: "ctrl+c", description: "Quit"},
	}

	var shortcuts string
	if innerWidth >= 60 {
		gap := 4
		columnWidth := (innerWidth - gap) / 2
		left := lipgloss.JoinVertical(
			lipgloss.Left,
			renderHelpSection("GENERAL", general, columnWidth),
			"",
			renderHelpSection("AGENDA", agenda, columnWidth),
		)
		right := lipgloss.JoinVertical(
			lipgloss.Left,
			renderHelpSection("MONTH", month, columnWidth),
			"",
			renderHelpSection("EXIT", exit, columnWidth),
		)
		shortcuts = lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", gap), right)
	} else {
		shortcuts = lipgloss.JoinVertical(
			lipgloss.Left,
			renderHelpSection("GENERAL", general, innerWidth),
			"",
			renderHelpSection("AGENDA", agenda, innerWidth),
			"",
			renderHelpSection("MONTH", month, innerWidth),
			"",
			renderHelpSection("EXIT", exit, innerWidth),
		)
	}

	header := titleStyle.Render("crusade") + mutedStyle.Render("  QUICK GUIDE")
	footer := m.helpFooter()
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		mutedStyle.Render("Navigation and actions"),
		"",
		shortcuts,
		"",
		infoStyle.Render(strings.Repeat("─", innerWidth)),
		footer,
	)
	return helpPanelStyle.Width(innerWidth).Render(body)
}

func renderHelpSection(title string, items []helpItem, width int) string {
	lines := []string{sectionStyle.Render(title)}
	keyWidth := min(10, max(7, width/3))
	for _, item := range items {
		descriptionWidth := max(1, width-keyWidth-1)
		line := helpKeyStyle.Width(keyWidth).Render(item.key) + " " + truncateText(item.description, descriptionWidth)
		lines = append(lines, line)
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
}

func (m *appModel) toggleView() {
	m.pendingDelete = false
	m.status = ""
	if m.view == viewAgenda {
		m.view = viewMonth
		m.monthFocus = monthFocusGrid
		m.monthEventCursor = 0
		return
	}
	m.view = viewAgenda
}

func (m *appModel) jumpToday() {
	today := dateOnly(time.Now())
	if m.view == viewAgenda {
		visible := m.visibleRecords()
		m.agendaCursor = 0
		for idx, record := range visible {
			if !record.Event.X.Before(today) {
				m.agendaCursor = idx
				break
			}
			if idx == len(visible)-1 {
				m.agendaCursor = idx
			}
		}
		return
	}
	m.monthDate = today
	m.monthEventCursor = 0
}

func (m *appModel) handleDelete() {
	if !m.pendingDelete {
		m.pendingDelete = true
		m.status = "Press d again to delete the selected outcome."
		return
	}
	var target store.Record
	found := false
	if m.view == viewAgenda {
		visible := m.visibleRecords()
		if len(visible) > 0 {
			target = visible[clamp(m.agendaCursor, 0, len(visible)-1)]
			found = true
		}
	} else if m.monthFocus == monthFocusEvents {
		events := m.recordsForDate(m.monthDate)
		if len(events) > 0 {
			target = events[clamp(m.monthEventCursor, 0, len(events)-1)]
			found = true
		}
	}
	if !found {
		m.status = "No item selected."
		m.pendingDelete = false
		return
	}
	if err := store.Delete(m.dataPath, target.ID); err != nil {
		m.status = err.Error()
	} else {
		if records, err := store.Load(m.dataPath); err == nil {
			m.records = records
		}
		m.status = "Deleted outcome."
	}
	m.pendingDelete = false
}

func (m appModel) aligning() bool {
	return m.mode != modeDo
}

func (m appModel) captureSession() sessionState {
	return sessionState{
		view:             m.view,
		agendaCursor:     m.agendaCursor,
		agendaCol:        m.agendaCol,
		monthDate:        m.monthDate,
		monthFocus:       m.monthFocus,
		monthEventCursor: m.monthEventCursor,
		monthEventCol:    m.monthEventCol,
		showHelp:         m.showHelp,
		status:           m.status,
	}
}

func (m *appModel) restoreSession(s sessionState) {
	m.view = s.view
	m.agendaCursor = s.agendaCursor
	m.agendaCol = s.agendaCol
	m.monthDate = s.monthDate
	m.monthFocus = s.monthFocus
	m.monthEventCursor = s.monthEventCursor
	m.monthEventCol = s.monthEventCol
	m.showHelp = s.showHelp
	m.status = s.status
	m.pendingDelete = false
}

func (m appModel) cycleMode() (tea.Model, tea.Cmd) {
	if !m.aligning() {
		m.memory.do = m.captureSession()
		m.memory.hasDo = true
		m.mode = modeAlign
		m.pendingDelete = false
		if m.memory.hasAlign {
			m.restoreSession(m.memory.align)
		} else {
			m.showHelp = false
			m.status = ""
			m.view = viewAgenda
			m.agendaCursor = 0
			m.agendaCol = 0
			m.monthDate = dateOnly(time.Now())
			m.monthFocus = monthFocusGrid
			m.monthEventCursor = 0
			m.monthEventCol = 0
		}
		m.clampState()
		rememberMode(m.dataPath, model.ModeAlign)
		return m, nil
	}
	m.memory.align = m.captureSession()
	m.memory.hasAlign = true
	m.pendingDelete = false
	if m.memory.hasBox {
		box := *m.memory.box
		box.width = m.width
		box.height = m.height
		box.memory = m.memory
		rememberMode(m.dataPath, model.ModeBox)
		return box, nil
	}
	outline, err := store.LoadOutline(m.dataPath)
	if err != nil {
		m.status = "error: " + err.Error()
		return m, nil
	}
	day := time.Now().Local().Format(time.DateOnly)
	checked, err := store.LoadDayChecks(m.dataPath, day)
	if err != nil {
		m.status = "error: " + err.Error()
		return m, nil
	}
	active, err := store.LoadActiveDays(m.dataPath)
	if err != nil {
		m.status = "error: " + err.Error()
		return m, nil
	}
	counts, err := store.LoadCheckCounts(m.dataPath)
	if err != nil {
		m.status = "error: " + err.Error()
		return m, nil
	}
	next := newBoxModel(m.dataPath, outline, checked, day)
	next.width = m.width
	next.height = m.height
	next.activeDays = active
	next.dayCounts = counts
	next.memory = m.memory
	rememberMode(m.dataPath, model.ModeBox)
	return next, nil
}

func (m boxModel) cycleMode() (tea.Model, tea.Cmd) {
	mem := m.memory
	saved := m
	saved.memory = modeMemory{}
	mem.box = &saved
	mem.hasBox = true
	next, err := openNLPMode(m.dataPath, m.width, m.height, mem)
	if err != nil {
		m.errText = err.Error()
		return m, nil
	}
	rememberMode(m.dataPath, model.ModeNLP)
	return next, nil
}

func (m *appModel) cancelCompose() {
	m.composing = false
	m.composeKind = ""
	m.composeID = 0
	m.composeText = ""
	m.composeCursor = 0
}

func (m *appModel) handleComposeKey(key string) {
	switch key {
	case "enter":
		m.commitCompose()
	case "backspace", "ctrl+h":
		m.deleteCompose(m.composeCursor-1, m.composeCursor)
	case "ctrl+d", "delete":
		m.deleteCompose(m.composeCursor, m.composeCursor+1)
	case "left", "ctrl+b":
		m.moveCompose(-1)
	case "right", "ctrl+f":
		m.moveCompose(1)
	case "home", "ctrl+a":
		m.composeCursor = 0
	case "end", "ctrl+e":
		m.composeCursor = len([]rune(m.composeText))
	case "ctrl+k":
		m.deleteCompose(m.composeCursor, len([]rune(m.composeText)))
	case "ctrl+u":
		m.deleteCompose(0, m.composeCursor)
	case "ctrl+w", "alt+backspace":
		m.deleteCompose(backwardWord([]rune(m.composeText), m.composeCursor), m.composeCursor)
	case "alt+d":
		runes := []rune(m.composeText)
		m.deleteCompose(m.composeCursor, forwardWord(runes, m.composeCursor))
	case "alt+f":
		m.composeCursor = forwardWord([]rune(m.composeText), m.composeCursor)
	case "alt+b":
		m.composeCursor = backwardWord([]rune(m.composeText), m.composeCursor)
	case "ctrl+t":
		m.transposeCompose()
	default:
		if len(key) == 1 && key[0] >= 32 {
			m.insertCompose(key)
		}
	}
}

func (m *appModel) beginNewTask() {
	m.pendingDelete = false
	m.composing = true
	m.composeKind = composeTask
	m.composeID = 0
	m.composeText = ""
	m.composeCursor = 0
	m.status = m.doPrompt()
}

func (m *appModel) beginEditTask() {
	target, found := m.selectedRecord()
	if !found {
		m.status = "No task selected to edit."
		m.pendingDelete = false
		return
	}
	m.pendingDelete = false
	m.composing = true
	m.composeKind = composeTask
	m.composeID = target.ID
	m.composeText = target.Event.Y
	m.composeCursor = len([]rune(target.Event.Y))
	m.status = "Edit task. Enter to save, esc to cancel."
}

func (m *appModel) beginReschedule() {
	target, found := m.selectedRecord()
	if !found {
		m.status = "No task selected to reschedule."
		m.pendingDelete = false
		return
	}
	m.pendingDelete = false
	m.composing = true
	m.composeKind = composeWhen
	m.composeID = target.ID
	m.composeText = doWhenText(target.Event.X)
	m.composeCursor = len([]rune(m.composeText))
	m.status = "Reschedule (YYYY-MM-DD). Enter to save, esc to cancel."
}

func doWhenText(when time.Time) string {
	if when.Hour() == 23 && when.Minute() == 59 && when.Second() == 59 {
		return when.Format("2006-01-02")
	}
	return when.Format(model.DateTimeFormat)
}

func (m *appModel) commitCompose() {
	text := strings.TrimSpace(m.composeText)
	if m.composeKind == composeWhen {
		if text == "" {
			m.status = "Date cannot be empty."
			return
		}
		m.rescheduleDoTask(text)
		return
	}
	if text == "" {
		m.status = "Task cannot be empty."
		return
	}
	if m.composeID == 0 {
		m.addDoTask(text)
		return
	}
	m.updateDoTask(text)
}

func (m *appModel) addDoTask(text string) {
	event, err := model.NewDoEvent(m.doStamp().Format(model.DateTimeFormat), text)
	if err != nil {
		m.status = "error: " + err.Error()
		return
	}
	if _, err := store.AddRecord(m.dataPath, event); err != nil {
		m.status = "error: " + err.Error()
		return
	}
	m.reloadRecords()
	m.cancelCompose()
	m.status = "Added task."
}

func (m *appModel) updateDoTask(text string) {
	record, found := recordByID(m.records, m.composeID)
	if !found {
		m.cancelCompose()
		m.status = "No task selected to edit."
		return
	}
	record.Event.Y = text
	if err := store.Update(m.dataPath, record.ID, record.Event); err != nil {
		m.status = "error: " + err.Error()
		return
	}
	m.reloadRecords()
	m.cancelCompose()
	m.status = "Updated task."
}

func (m *appModel) rescheduleDoTask(raw string) {
	record, found := recordByID(m.records, m.composeID)
	if !found {
		m.cancelCompose()
		m.status = "No task selected to reschedule."
		return
	}
	when, err := normalizeDoWhen(raw)
	if err != nil {
		m.status = "error: " + err.Error()
		return
	}
	event, err := model.NewDoEvent(when, record.Event.Y)
	if err != nil {
		m.status = "error: " + err.Error()
		return
	}
	event.Done = record.Event.Done
	if err := store.Update(m.dataPath, record.ID, event); err != nil {
		m.status = "error: " + err.Error()
		return
	}
	m.reloadRecords()
	m.cancelCompose()
	m.status = "Rescheduled task."
}

func normalizeDoWhen(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if len(value) == 10 {
		if day, err := time.ParseInLocation("2006-01-02", value, time.Local); err == nil {
			stamp := time.Date(day.Year(), day.Month(), day.Day(), 23, 59, 59, 0, time.Local)
			return stamp.Format(model.DateTimeFormat), nil
		}
	}
	parsed, err := model.ParseDateTime(value)
	if err != nil {
		return "", err
	}
	return parsed.Format(model.DateTimeFormat), nil
}

func (m *appModel) reloadRecords() {
	records, err := store.Load(m.dataPath)
	if err != nil {
		m.status = "error: " + err.Error()
		return
	}
	m.records = records
}

func (m appModel) doStamp() time.Time {
	day := dateOnly(time.Now())
	if m.view == viewMonth {
		day = dateOnly(m.monthDate)
	}
	return time.Date(day.Year(), day.Month(), day.Day(), 23, 59, 59, 0, time.Local)
}

func (m appModel) doPrompt() string {
	stamp := m.doStamp()
	when := "today"
	if m.view == viewMonth && !sameDate(dateOnly(stamp), dateOnly(time.Now())) {
		when = stamp.Format("2006-01-02")
	}
	return "Task for " + when + ". Enter to save, esc to cancel."
}

func (m appModel) helpGeneral() []helpItem {
	items := []helpItem{
		{key: "tab", description: "Do / align / box / nlp"},
		{key: "a", description: "Agenda / month"},
		{key: "t", description: "Jump to today"},
	}
	if m.aligning() {
		items = append(items,
			helpItem{key: "n", description: "New task in Vim"},
			helpItem{key: "e", description: "Edit selected in Vim"},
		)
	} else {
		items = append(items,
			helpItem{key: "n", description: "New task"},
			helpItem{key: "e", description: "Edit task text"},
			helpItem{key: "r", description: "Reschedule selected"},
		)
	}
	return append(items,
		helpItem{key: "space", description: "Mark selected done"},
		helpItem{key: "dd", description: "Delete selected"},
		helpItem{key: "?", description: "Close this guide"},
	)
}

func (m appModel) helpFooter() string {
	if !m.aligning() {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			sectionStyle.Render("DO MODE"),
			mutedStyle.Render("n adds a task for today, or for the selected day in the open month"),
			mutedStyle.Render("e edits the text · r reschedules the selected task"),
			mutedStyle.Render("While typing: ctrl+a/e/b/f/h/d, alt+b/f"),
			mutedStyle.Render("tab cycles do, align, box, and nlp"),
		)
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		sectionStyle.Render("VIM TASK FILE"),
		mutedStyle.Render("n adds on the selected date · e edits the selected task"),
		mutedStyle.Render("Vim edits that one outcome, then crusade saves it"),
		mutedStyle.Render("tab cycles do, align, box, and nlp"),
	)
}

func renderTaskLines(records []store.Record, includeDate bool, height int, selected int) string {
	if height <= 0 || len(records) == 0 {
		return ""
	}
	selected = clamp(selected, 0, len(records)-1)
	start := 0
	if len(records) > height {
		start = selected - height + 1
		if start < 0 {
			start = 0
		}
	}
	var b strings.Builder
	written := 0
	for index := start; index < len(records) && written < height; index++ {
		event := records[index].Event
		mark := "[ ]"
		if event.Done {
			mark = "[x]"
		}
		text := event.Y
		if includeDate {
			text = event.X.Format("2006-01-02") + "  " + event.Y
		}
		line := mark + " " + text
		if index == selected {
			line = titleStyle.Render("> " + line)
		} else {
			line = "  " + line
		}
		b.WriteString(line)
		b.WriteByte('\n')
		written++
	}
	return b.String()
}

func (m appModel) composeShown() string {
	runes := []rune(m.composeText)
	cursor := clamp(m.composeCursor, 0, len(runes))
	if cursor >= len(runes) {
		return string(runes) + "▌"
	}
	under := lipgloss.NewStyle().Reverse(true).Render(string(runes[cursor]))
	return string(runes[:cursor]) + under + string(runes[cursor+1:])
}

func (m *appModel) insertCompose(text string) {
	runes := []rune(m.composeText)
	cursor := clamp(m.composeCursor, 0, len(runes))
	extra := []rune(text)
	next := append(append(append([]rune{}, runes[:cursor]...), extra...), runes[cursor:]...)
	m.composeText = string(next)
	m.composeCursor = cursor + len(extra)
}

func (m *appModel) deleteCompose(from int, to int) {
	runes := []rune(m.composeText)
	from = clamp(from, 0, len(runes))
	to = clamp(to, 0, len(runes))
	if to < from {
		from, to = to, from
	}
	m.composeText = string(append(append([]rune{}, runes[:from]...), runes[to:]...))
	m.composeCursor = from
}

func (m *appModel) moveCompose(delta int) {
	m.composeCursor = clamp(m.composeCursor+delta, 0, len([]rune(m.composeText)))
}

func (m *appModel) transposeCompose() {
	runes := []rune(m.composeText)
	cursor := clamp(m.composeCursor, 0, len(runes))
	if len(runes) < 2 || cursor == 0 {
		return
	}
	if cursor == len(runes) {
		runes[cursor-2], runes[cursor-1] = runes[cursor-1], runes[cursor-2]
	} else {
		runes[cursor-1], runes[cursor] = runes[cursor], runes[cursor-1]
		cursor++
	}
	m.composeText = string(runes)
	m.composeCursor = cursor
}

func forwardWord(runes []rune, cursor int) int {
	cursor = clamp(cursor, 0, len(runes))
	for cursor < len(runes) && !isWordRune(runes[cursor]) {
		cursor++
	}
	for cursor < len(runes) && isWordRune(runes[cursor]) {
		cursor++
	}
	return cursor
}

func backwardWord(runes []rune, cursor int) int {
	cursor = clamp(cursor, 0, len(runes))
	for cursor > 0 && !isWordRune(runes[cursor-1]) {
		cursor--
	}
	for cursor > 0 && isWordRune(runes[cursor-1]) {
		cursor--
	}
	return cursor
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func (m *appModel) addTaskInVim() tea.Cmd {
	selected := m.selectedDate()
	event := model.Event{
		Mode: model.ModeAlign,
		X:    time.Date(selected.Year(), selected.Month(), selected.Day(), 23, 59, 59, 0, selected.Location()),
		Y:    draftOutcome,
		Z:    draftImpact,
	}
	id, err := store.AddRecord(m.dataPath, event)
	if err != nil {
		m.status = "error: " + err.Error()
		return nil
	}
	m.pendingDelete = false
	m.status = fmt.Sprintf("Editing new task %d in Vim...", id)
	return openTaskEditor(m.dataPath, id, true)
}

func (m *appModel) editTaskInVim() tea.Cmd {
	target, found := m.selectedRecord()
	if !found {
		m.status = "No task selected to edit."
		m.pendingDelete = false
		return nil
	}
	m.pendingDelete = false
	m.status = fmt.Sprintf("Editing task %d in Vim...", target.ID)
	return openTaskEditor(m.dataPath, target.ID, false)
}

func (m *appModel) finishEditor(message editorFinishedMsg) {
	if message.err != nil {
		if message.draft {
			_ = store.Delete(m.dataPath, message.id)
		}
		m.status = fmt.Sprintf("error: Vim exited: %v", message.err)
		return
	}
	records, err := store.Load(m.dataPath)
	if err != nil {
		m.status = "error: " + err.Error()
		return
	}
	if message.draft {
		record, found := recordByID(records, message.id)
		if !found {
			m.records = records
			m.status = "New task cancelled."
			return
		}
		if record.Event.Y == draftOutcome && record.Event.Z == draftImpact {
			if err := store.Delete(m.dataPath, message.id); err != nil {
				m.status = "error: " + err.Error()
				return
			}
			records, err = store.Load(m.dataPath)
			if err != nil {
				m.status = "error: " + err.Error()
				return
			}
			m.records = records
			m.status = "New task cancelled."
			return
		}
	}
	m.records = records
	m.status = "Saved."
}

func (m appModel) selectedDate() time.Time {
	if m.view == viewMonth {
		return dateOnly(m.monthDate)
	}
	if target, found := m.selectedRecord(); found {
		return dateOnly(target.Event.X)
	}
	return dateOnly(time.Now())
}

func (m appModel) selectedRecord() (store.Record, bool) {
	if m.view == viewAgenda {
		visible := m.visibleRecords()
		if len(visible) == 0 {
			return store.Record{}, false
		}
		return visible[clamp(m.agendaCursor, 0, len(visible)-1)], true
	}
	events := m.recordsForDate(m.monthDate)
	if len(events) == 0 {
		return store.Record{}, false
	}
	return events[clamp(m.monthEventCursor, 0, len(events)-1)], true
}

func recordByID(records []store.Record, id int) (store.Record, bool) {
	for _, record := range records {
		if record.ID == id {
			return record, true
		}
	}
	return store.Record{}, false
}

func (m *appModel) toggleDone() {
	record, found := m.selectedRecord()
	if !found {
		m.pendingDelete = false
		m.status = "No task selected."
		return
	}
	record.Event.Done = !record.Event.Done
	if err := store.Update(m.dataPath, record.ID, record.Event); err != nil {
		m.pendingDelete = false
		m.status = "error: " + err.Error()
		return
	}
	m.reloadRecords()
	m.pendingDelete = false
	if strings.HasPrefix(m.status, "error:") {
		return
	}
	if record.Event.Done {
		m.status = "Marked done."
	} else {
		m.status = "Cleared."
	}
}

func (m *appModel) handleAgendaKey(key string) {
	visible := m.visibleRecords()
	switch key {
	case " ":
		m.toggleDone()
	case "j", "down":
		m.agendaCursor++
	case "k", "up":
		m.agendaCursor--
	case "h", "left":
		m.agendaCol--
	case "l", "right":
		m.agendaCol++
	case "g", "home":
		m.agendaCursor = 0
	case "G", "end":
		m.agendaCursor = len(visible) - 1
	case "H":
		m.agendaJumpDay(-1)
	case "L":
		m.agendaJumpDay(1)
	}
}

func (m *appModel) agendaJumpDay(direction int) {
	visible := m.visibleRecords()
	if len(visible) == 0 {
		return
	}
	cur := clamp(m.agendaCursor, 0, len(visible)-1)
	curDay := dateOnly(visible[cur].Event.X)
	if direction < 0 {
		for idx := cur - 1; idx >= 0; idx-- {
			day := dateOnly(visible[idx].Event.X)
			if day.Before(curDay) {
				for first := 0; first <= idx; first++ {
					if sameDate(dateOnly(visible[first].Event.X), day) {
						m.agendaCursor = first
						return
					}
				}
			}
		}
		return
	}
	for idx := cur + 1; idx < len(visible); idx++ {
		day := dateOnly(visible[idx].Event.X)
		if day.After(curDay) {
			m.agendaCursor = idx
			return
		}
	}
}

func (m *appModel) leaveDoTaskList() bool {
	if m.aligning() || m.view != viewMonth || m.monthFocus != monthFocusEvents {
		return false
	}
	m.monthFocus = monthFocusGrid
	m.monthEventCursor = 0
	m.pendingDelete = false
	m.status = ""
	return true
}

func (m *appModel) handleMonthKey(key string) {
	if m.monthFocus == monthFocusEvents {
		switch key {
		case " ":
			m.toggleDone()
		case "enter":
			m.monthFocus = monthFocusGrid
		case "j", "down":
			m.monthEventCursor++
		case "k", "up":
			m.monthEventCursor--
		case "h", "left":
			m.monthEventCol--
		case "l", "right":
			m.monthEventCol++
		case "ctrl+h", "ctrl+l", "ctrl+j", "ctrl+k":
			m.monthFocus = monthFocusGrid
			m.monthEventCursor = 0
			m.handleMonthKey(key)
		}
		return
	}

	switch key {
	case "enter":
		if len(m.recordsForDate(m.monthDate)) > 0 {
			m.monthFocus = monthFocusEvents
			m.monthEventCursor = 0
		} else {
			m.status = "No outcomes on selected day."
		}
	case "h", "left":
		m.monthDate = m.monthDate.AddDate(0, 0, -1)
		m.monthEventCursor = 0
	case "l", "right":
		m.monthDate = m.monthDate.AddDate(0, 0, 1)
		m.monthEventCursor = 0
	case "j", "down":
		m.monthDate = m.monthDate.AddDate(0, 0, 7)
		m.monthEventCursor = 0
	case "k", "up":
		m.monthDate = m.monthDate.AddDate(0, 0, -7)
		m.monthEventCursor = 0
	case "ctrl+h":
		m.monthDate = moveMonth(m.monthDate, -1)
		m.monthEventCursor = 0
	case "ctrl+l":
		m.monthDate = moveMonth(m.monthDate, 1)
		m.monthEventCursor = 0
	case "ctrl+k":
		m.monthDate = moveMonth(m.monthDate, -12)
		m.monthEventCursor = 0
	case "ctrl+j":
		m.monthDate = moveMonth(m.monthDate, 12)
		m.monthEventCursor = 0
	}
}

func (m *appModel) clampState() {
	visible := m.visibleRecords()
	m.agendaCursor = clamp(m.agendaCursor, 0, max(0, len(visible)-1))
	agendaMax := 3
	monthMax := 3
	if !m.aligning() {
		agendaMax = 1
		monthMax = 0
	}
	m.agendaCol = clamp(m.agendaCol, 0, agendaMax)
	m.monthDate = dateOnly(m.monthDate)
	events := m.recordsForDate(m.monthDate)
	m.monthEventCursor = clamp(m.monthEventCursor, 0, max(0, len(events)-1))
	m.monthEventCol = clamp(m.monthEventCol, 0, monthMax)
	if len(events) == 0 && m.monthFocus == monthFocusEvents {
		m.monthFocus = monthFocusGrid
	}
	if m.view != viewAgenda && m.view != viewMonth {
		m.view = viewMonth
	}
	if m.monthFocus != monthFocusGrid && m.monthFocus != monthFocusEvents {
		m.monthFocus = monthFocusGrid
	}
}

func (m appModel) renderAgenda(height int) string {
	records := m.visibleRecords()
	if len(records) == 0 {
		if m.aligning() {
			return "No tasks yet. Press n or use crusade -m align -c.\n"
		}
		return "No tasks yet. Press n to add one.\n"
	}
	return renderTaskLines(records, true, height, m.agendaCursor)
}

func (m appModel) renderMonth(width int, height int) string {
	if height <= 0 {
		return ""
	}
	return padToHeight(m.renderMonthBody(width, height), height)
}

func (m appModel) renderMonthBody(width int, height int) string {
	if height <= 0 {
		return ""
	}
	weeks := monthWeeks(m.monthDate)
	gridLines := renderMonthGrid(weeks, m.monthDate, m.eventsByDate(), width)
	eventRows := m.recordsForDate(m.monthDate)

	var b strings.Builder
	monthTitle := m.monthDate.Format("January, 2006")
	b.WriteString(sectionHeading(monthTitle, width))
	b.WriteByte('\n')

	available := height - 1
	if available > len(gridLines)+6 {
		b.WriteByte('\n')
		available--
	}
	gridBudget := len(gridLines)
	if available < gridBudget {
		gridBudget = available
	}
	for idx := 0; idx < gridBudget; idx++ {
		b.WriteString(gridLines[idx])
		b.WriteByte('\n')
	}

	remaining := available - gridBudget
	if remaining <= 0 {
		return b.String()
	}
	writeGap(&b, &remaining, 1)
	if remaining <= 0 {
		return b.String()
	}

	b.WriteString(sectionHeading("selected day", width))
	b.WriteByte('\n')
	remaining--
	if remaining <= 0 {
		return b.String()
	}
	writeGap(&b, &remaining, 1)
	if remaining <= 0 {
		return b.String()
	}

	selectedLabel := m.monthDate.Format("2006-01-02")
	noun := "tasks"
	if m.aligning() {
		noun = "outcomes"
	}
	if len(eventRows) == 0 {
		fmt.Fprintf(&b, "%s\n", mutedStyle.Render(fmt.Sprintf("%s has no %s", selectedLabel, noun)))
		remaining--
		if remaining > 0 {
			if next, ok := nextRecord(m.visibleRecords(), m.monthDate); ok {
				fmt.Fprintf(&b, "%s\n", infoStyle.Render(truncateText(fmt.Sprintf("next: %s  %s", next.Event.X.Format("Jan 2"), next.Event.Y), max(20, width))))
			}
		}
		return b.String()
	}
	if m.aligning() {
		fmt.Fprintf(&b, "%s\n", sectionStyle.Render(fmt.Sprintf("%s %s (%d)", selectedLabel, noun, len(eventRows))))
		remaining--
		if remaining <= 0 {
			return b.String()
		}
	}
	b.WriteString(renderTaskLines(eventRows, false, remaining, m.monthEventCursor))
	return b.String()
}

func formatScore(event model.Event) string {
	return model.FormatMetric((event.P+event.Q+event.R)/3) + "/10"
}

func bestRecord(records []store.Record) store.Record {
	best := records[0]
	bestScore := best.Event.P + best.Event.Q + best.Event.R
	for _, record := range records[1:] {
		score := record.Event.P + record.Event.Q + record.Event.R
		if score > bestScore {
			best = record
			bestScore = score
		}
	}
	return best
}

func nextRecord(records []store.Record, day time.Time) (store.Record, bool) {
	day = dateOnly(day)
	for _, record := range records {
		if record.Event.X.After(day) {
			return record, true
		}
	}
	return store.Record{}, false
}

func plural(count int, singular string, plural string) string {
	if count == 1 {
		return fmt.Sprintf("1 %s", singular)
	}
	return fmt.Sprintf("%d %s", count, plural)
}

func truncateText(value string, width int) string {
	if width <= 4 || len(value) <= width {
		return value
	}
	return value[:width-3] + "..."
}

func sectionHeading(label string, width int) string {
	if width <= 0 {
		return sectionStyle.Render(label)
	}
	return sectionStyle.Render(truncateText(label, width))
}

func padToHeight(content string, height int) string {
	if height <= 0 {
		return content
	}
	text := strings.TrimRight(content, "\n")
	var lines []string
	if text != "" {
		lines = strings.Split(text, "\n")
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, " ")
	}
	return strings.Join(lines, "\n")
}

func writeGap(b *strings.Builder, remaining *int, lines int) {
	for idx := 0; idx < lines && *remaining > 0; idx++ {
		b.WriteByte('\n')
		*remaining--
	}
}

func monthGridWidth(width int) int {
	cellWidth := 7
	if cellWidth*7 > width {
		cellWidth = max(4, width/7)
	}
	if cellWidth < 1 {
		cellWidth = 1
	}
	return cellWidth * 7
}

func renderMonthGrid(weeks [][]time.Time, selected time.Time, events map[time.Time]int, width int) []string {
	cellWidth := 7
	if width > 0 {
		cellWidth = monthGridWidth(width) / 7
	}
	weekdays := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	lines := make([]string, 0, len(weeks)+1)
	header := make([]string, 0, len(weekdays))
	for _, day := range weekdays {
		header = append(header, mutedStyle.Render(pad(day, cellWidth, "left")))
	}
	lines = append(lines, strings.Join(header, ""))

	today := dateOnly(time.Now())
	selected = dateOnly(selected)
	for _, week := range weeks {
		cells := make([]string, 0, len(week))
		for _, day := range week {
			day = dateOnly(day)
			cells = append(cells, renderMonthDayCell(day, selected, today, events[day], cellWidth))
		}
		lines = append(lines, strings.Join(cells, ""))
	}
	return lines
}

func renderMonthDayCell(day time.Time, selected time.Time, today time.Time, eventCount int, cellWidth int) string {
	day = dateOnly(day)
	selected = dateOnly(selected)
	today = dateOnly(today)

	if sameDate(day, selected) {
		return activeCellStyle.Render(pad(monthDayLabel(day, eventCount, cellWidth), cellWidth, "left"))
	}
	if sameDate(day, today) {
		return renderTodayCell(day, eventCount, cellWidth)
	}

	cell := pad(monthDayLabel(day, eventCount, cellWidth), cellWidth, "left")
	switch {
	case eventCount > 0 && day.Month() == selected.Month():
		return eventDayStyle.Render(cell)
	case day.Month() != selected.Month():
		return mutedStyle.Render(cell)
	default:
		return cell
	}
}

func renderTodayCell(day time.Time, eventCount int, cellWidth int) string {
	dayNumber := strconv.Itoa(day.Day())
	prefix := ""
	if day.Day() < 10 && cellWidth >= 2 {
		prefix = " "
	}
	suffix := ""
	if eventCount > 0 && cellWidth >= 7 {
		suffix = fmt.Sprintf("*%d", min(eventCount, 99))
	}
	visibleWidth := len(prefix) + len(dayNumber) + len(suffix)
	if visibleWidth > cellWidth {
		suffix = ""
		visibleWidth = len(prefix) + len(dayNumber)
	}
	if visibleWidth > cellWidth {
		prefix = ""
		visibleWidth = len(dayNumber)
	}
	if visibleWidth > cellWidth {
		dayNumber = dayNumber[len(dayNumber)-cellWidth:]
		visibleWidth = len(dayNumber)
	}
	return prefix + todayStyle.Render(dayNumber) + suffix + strings.Repeat(" ", max(0, cellWidth-visibleWidth))
}

func monthDayLabel(day time.Time, eventCount int, cellWidth int) string {
	label := fmt.Sprintf("%2d", day.Day())
	if eventCount > 0 && cellWidth >= 7 {
		label = fmt.Sprintf("%2d*%d", day.Day(), min(eventCount, 99))
	}
	return label
}

func monthWeeks(selected time.Time) [][]time.Time {
	first := time.Date(selected.Year(), selected.Month(), 1, 0, 0, 0, 0, selected.Location())
	offset := (int(first.Weekday()) + 6) % 7
	start := first.AddDate(0, 0, -offset)
	last := first.AddDate(0, 1, -1)
	weeks := make([][]time.Time, 0, 6)
	for day := start; !day.After(last) || day.Weekday() != time.Monday; day = day.AddDate(0, 0, 7) {
		week := make([]time.Time, 0, 7)
		for idx := 0; idx < 7; idx++ {
			week = append(week, day.AddDate(0, 0, idx))
		}
		weeks = append(weeks, week)
		if week[6].After(last) {
			break
		}
	}
	return weeks
}

func (m appModel) visibleRecords() []store.Record {
	records := make([]store.Record, 0, len(m.records))
	for _, record := range m.records {
		if m.aligning() {
			if record.Event.IsDo() {
				continue
			}
		} else if !record.Event.IsDo() {
			continue
		}
		records = append(records, record)
	}
	return records
}

func (m appModel) recordsForDate(day time.Time) []store.Record {
	day = dateOnly(day)
	records := make([]store.Record, 0)
	for _, record := range m.visibleRecords() {
		if sameDate(dateOnly(record.Event.X), day) {
			records = append(records, record)
		}
	}
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].Event.X.Before(records[j].Event.X)
	})
	return records
}

func (m appModel) eventsByDate() map[time.Time]int {
	events := make(map[time.Time]int)
	for _, record := range m.visibleRecords() {
		events[dateOnly(record.Event.X)]++
	}
	return events
}

func pad(value string, width int, align string) string {
	if width <= 0 {
		return ""
	}
	if len(value) > width {
		if align == "right" {
			value = value[len(value)-width:]
		} else {
			value = value[:width]
		}
	}
	switch align {
	case "right":
		return strings.Repeat(" ", width-len(value)) + value
	case "center":
		left := (width - len(value)) / 2
		right := width - len(value) - left
		return strings.Repeat(" ", left) + value + strings.Repeat(" ", right)
	default:
		return value + strings.Repeat(" ", width-len(value))
	}
}

func moveMonth(day time.Time, delta int) time.Time {
	day = dateOnly(day)
	firstTarget := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, day.Location()).AddDate(0, delta, 0)
	maxDay := firstTarget.AddDate(0, 1, -1).Day()
	return time.Date(firstTarget.Year(), firstTarget.Month(), min(day.Day(), maxDay), 0, 0, 0, 0, day.Location())
}

// doneThisWeek counts do tasks marked done whose day falls in the Monday-Sunday week that contains now.
func doneThisWeek(records []store.Record, now time.Time) int {
	start := mondayOf(now)
	end := start.AddDate(0, 0, 7)
	count := 0
	for _, record := range records {
		if !record.Event.IsDo() || !record.Event.Done {
			continue
		}
		day := dateOnly(record.Event.X)
		if !day.Before(start) && day.Before(end) {
			count++
		}
	}
	return count
}

func mondayOf(day time.Time) time.Time {
	day = dateOnly(day)
	offset := (int(day.Weekday()) + 6) % 7
	return day.AddDate(0, 0, -offset)
}

func dateOnly(value time.Time) time.Time {
	if value.IsZero() {
		value = time.Now()
	}
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
}

func sameDate(a time.Time, b time.Time) bool {
	a = dateOnly(a)
	b = dateOnly(b)
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

func clamp(value int, minValue int, maxValue int) int {
	if maxValue < minValue {
		return minValue
	}
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
