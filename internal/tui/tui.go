package tui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ryangerardwilson/crusade/internal/lineedit"
	"github.com/ryangerardwilson/crusade/internal/model"
	"github.com/ryangerardwilson/crusade/internal/store"
)

const headerDateFormat = "January 2, 2006 (Monday)"

// appModel is align mode: a list of align records. records is a cache of
// the store's answer, reloaded after every write.
type appModel struct {
	dataPath string
	records  []store.AlignRecord

	width  int
	height int

	agendaCursor int
	agendaCol    int

	showHelp      bool
	pendingDelete bool
	status        string

	memory modeMemory
}

// sessionState is the screen an align visit returns to.
type sessionState struct {
	agendaCursor int
	agendaCol    int
	showHelp     bool
	status       string
}

// modeMemory keeps the align, box, cal, and nlp screens while tab moves between them.
type modeMemory struct {
	align    sessionState
	hasAlign bool
	box      *boxModel
	hasBox   bool
	cal      *calModel
	hasCal   bool
	nlp      *nlpMode
	hasNlp   bool
}

type helpItem struct {
	key         string
	description string
}

// editorFinishedMsg is what Vim left. id is zero for a new record. saved is
// false when Vim failed or a new record was left blank.
type editorFinishedMsg struct {
	id    int
	event model.Align
	saved bool
	err   error
}

// openTaskEditor edits seed as YAML in a temporary file. Nothing is stored
// until Vim saves; the caller writes the record from the message.
var openTaskEditor = func(id int, seed model.Align) tea.Cmd {
	fail := func(err error) tea.Cmd {
		return func() tea.Msg { return editorFinishedMsg{id: id, err: err} }
	}
	payload, err := model.ToYAML(seed)
	if err != nil {
		return fail(err)
	}
	tmp, err := os.CreateTemp("", "crusade-*.yaml")
	if err != nil {
		return fail(err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(payload); err != nil {
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
		return readEditedTask(id, seed, tmpPath, err)
	})
}

func readEditedTask(id int, seed model.Align, path string, vimErr error) editorFinishedMsg {
	if vimErr != nil {
		return editorFinishedMsg{id: id, err: vimErr}
	}
	edited, err := os.ReadFile(path)
	if err != nil {
		return editorFinishedMsg{id: id, err: err}
	}
	event, err := model.FromYAML(edited)
	if err != nil {
		if errors.Is(err, model.ErrBlankAlign) {
			return editorFinishedMsg{id: id}
		}
		return editorFinishedMsg{id: id, err: err}
	}
	event.DoneAt = seed.DoneAt
	return editorFinishedMsg{id: id, event: event, saved: true}
}

var (
	titleStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	badgeStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("255"))
	mutedStyle      = lipgloss.NewStyle().Faint(true)
	infoStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	sectionStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	activeCellStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("16")).Background(lipgloss.Color("255"))
	todayStyle      = lipgloss.NewStyle().Bold(true).Underline(true).Foreground(lipgloss.Color("255"))
	eventDayStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	errorStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("204"))
	cursorStyle     = lipgloss.NewStyle().Reverse(true)
	helpPanelStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("250")).Padding(1, 2)
	helpKeyStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
)

// Result is how a mode ended. Closed reports that the mode closed crusade,
// as opening a meeting link from the Super+C window does.
type Result struct {
	Code   int
	Closed bool
}

// closer is a screen that can close crusade as it quits.
type closer interface {
	closedCrusade() bool
}

// runProgram runs the screen until it quits and reports how it ended.
func runProgram(start tea.Model, stderr io.Writer) Result {
	final, err := tea.NewProgram(start, tea.WithAltScreen()).Run()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return Result{Code: 1}
	}
	c, ok := final.(closer)
	return Result{Closed: ok && c.closedCrusade()}
}

func Run(dataPath, mode string, stdout, stderr io.Writer) Result {
	rememberMode(dataPath, mode)
	switch mode {
	case model.ModeBox:
		return runBox(dataPath, stdout, stderr)
	case model.ModeCal:
		return runCal(dataPath, stdout, stderr)
	case model.ModeNLP:
		return runNLP(dataPath, stdout, stderr)
	case model.ModeDo:
		return runDo(dataPath, stdout, stderr)
	}
	records, err := store.Load(dataPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return Result{Code: 1}
	}
	return runProgram(newAppModel(dataPath, records), stderr)
}

func rememberMode(dataPath, mode string) {
	_ = store.SaveLastMode(dataPath, mode)
}

func newAppModel(dataPath string, records []store.AlignRecord) appModel {
	return appModel{dataPath: dataPath, records: records}
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
		if m.showHelp && key != "tab" {
			switch key {
			case "?", "esc":
				m.showHelp = false
			case "q":
				return m, tea.Quit
			}
			return m, nil
		}
		switch key {
		case "q", "esc":
			return m, tea.Quit
		case "tab":
			return m.cycleMode()
		case "?":
			m.showHelp = true
		case "t":
			m.jumpToday()
		case "d":
			m.handleDelete()
		case "n":
			return m, m.addTaskInVim()
		case "e", "i":
			return m, m.editTaskInVim()
		default:
			m.pendingDelete = false
			m.handleAgendaKey(key)
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
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", renderModeBanner("ALIGN", width))
	fmt.Fprintf(&b, "%s\n", infoStyle.Render(m.agendaSummary(width)))
	b.WriteString(m.renderAgenda(max(1, height-5)))
	if m.status != "" {
		fmt.Fprintf(&b, "\n%s\n", m.statusStyle().Render(m.status))
	}
	if m.showHelp {
		return placeModal(b.String(), m.helpView(width), width, height)
	}
	return b.String()
}

func (m appModel) statusStyle() lipgloss.Style {
	lowered := strings.ToLower(m.status)
	if strings.Contains(lowered, "error") || strings.Contains(lowered, "not found") || strings.Contains(lowered, "failed") {
		return errorStyle
	}
	return mutedStyle
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

func (m appModel) agendaSummary(width int) string {
	if len(m.records) == 0 {
		return "empty agenda | crusade -m align -c"
	}
	idx := clamp(m.agendaCursor, 0, len(m.records)-1)
	parts := []string{
		fmt.Sprintf("selected %d/%d", idx+1, len(m.records)),
		headerDate(m.records[idx].Align.X),
	}
	return truncateText(strings.Join(parts, " | "), max(20, width))
}

func headerDate(day time.Time) string {
	return day.Format(headerDateFormat)
}

func headerDateText(day string) string {
	parsed, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(day), time.Local)
	if err != nil {
		return day
	}
	return headerDate(parsed)
}

func placeModal(base, modal string, width, height int) string {
	modal = strings.TrimRight(modal, "\n")
	modalLines := strings.Split(modal, "\n")
	modalWidth := 0
	for _, line := range modalLines {
		if lineWidth := ansi.StringWidth(line); lineWidth > modalWidth {
			modalWidth = lineWidth
		}
	}
	if width <= 0 {
		width = ansi.StringWidth(base)
	}
	if width < modalWidth {
		width = modalWidth
	}
	if width <= 0 {
		width = 80
	}
	base = strings.TrimRight(base, "\n")
	if height <= 0 {
		height = max(len(strings.Split(base, "\n")), len(modalLines))
	}
	canvas := lipgloss.Place(width, height, lipgloss.Left, lipgloss.Top, base)
	lines := strings.Split(canvas, "\n")
	for i, line := range lines {
		switch gap := width - ansi.StringWidth(line); {
		case gap > 0:
			lines[i] = line + strings.Repeat(" ", gap)
		case gap < 0:
			lines[i] = ansi.Cut(line, 0, width)
		}
	}
	x := 0
	if width > modalWidth {
		x = (width - modalWidth) / 2
	}
	y := 0
	if len(lines) > len(modalLines) {
		y = (len(lines) - len(modalLines)) / 2
	}
	for i, modalLine := range modalLines {
		row := y + i
		if row < 0 || row >= len(lines) {
			continue
		}
		if gap := modalWidth - ansi.StringWidth(modalLine); gap > 0 {
			modalLine += strings.Repeat(" ", gap)
		}
		bg := lines[row]
		left := ansi.Cut(bg, 0, x)
		right := ansi.Cut(bg, x+modalWidth, ansi.StringWidth(bg))
		lines[row] = left + modalLine + right
	}
	return strings.Join(lines, "\n")
}

func shortcutModal(width int, section string, items []helpItem, detail string) string {
	if width <= 0 {
		width = 80
	}
	inner := min(72, max(32, width-8))
	parts := []string{
		titleStyle.Render("crusade") + mutedStyle.Render("  QUICK GUIDE"),
		"",
		renderHelpSection(section, items, inner),
	}
	if detail != "" {
		parts = append(parts, "", mutedStyle.Width(inner).Render(detail))
	}
	body := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return helpPanelStyle.Width(inner).Render(body)
}

func (m appModel) helpView(width int) string {
	innerWidth := min(72, max(32, width-8))
	general := m.helpGeneral()
	agenda := []helpItem{
		{key: "j / k", description: "Move row"},
		{key: "H / L", description: "Previous / next date"},
	}
	exit := []helpItem{
		{key: "q", description: "Leave"},
		{key: "esc", description: "Leave"},
		{key: "ctrl+c", description: "Leave"},
	}
	var shortcuts string
	if innerWidth >= 60 {
		gap := 4
		columnWidth := (innerWidth - gap) / 2
		left := lipgloss.JoinVertical(lipgloss.Left,
			renderHelpSection("GENERAL", general, columnWidth),
			"",
			renderHelpSection("AGENDA", agenda, columnWidth),
		)
		right := renderHelpSection("EXIT", exit, columnWidth)
		shortcuts = lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", gap), right)
	} else {
		shortcuts = lipgloss.JoinVertical(lipgloss.Left,
			renderHelpSection("GENERAL", general, innerWidth),
			"",
			renderHelpSection("AGENDA", agenda, innerWidth),
			"",
			renderHelpSection("EXIT", exit, innerWidth),
		)
	}
	header := titleStyle.Render("crusade") + mutedStyle.Render("  QUICK GUIDE")
	body := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		mutedStyle.Render("Navigation and actions"),
		"",
		shortcuts,
		"",
		infoStyle.Render(strings.Repeat("─", innerWidth)),
		m.helpFooter(),
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

func (m *appModel) jumpToday() {
	today := dateOnly(time.Now())
	m.agendaCursor = max(0, len(m.records)-1)
	for idx, record := range m.records {
		if !record.Align.X.Before(today) {
			m.agendaCursor = idx
			return
		}
	}
}

func (m *appModel) handleDelete() {
	if !m.pendingDelete {
		m.pendingDelete = true
		m.status = "Press d again to delete the selected outcome."
		return
	}
	m.pendingDelete = false
	target, found := m.selectedRecord()
	if !found {
		m.status = "No item selected."
		return
	}
	m.apply(store.Delete(m.dataPath, target.ID), "Deleted outcome.")
}

// apply shows the outcome of a store write and reloads the records.
func (m *appModel) apply(err error, status string) {
	if err == nil {
		var records []store.AlignRecord
		if records, err = store.Load(m.dataPath); err == nil {
			m.records = records
		}
	}
	if err != nil {
		m.status = "error: " + err.Error()
		return
	}
	m.status = status
}

func (m appModel) captureSession() sessionState {
	return sessionState{agendaCursor: m.agendaCursor, agendaCol: m.agendaCol, showHelp: m.showHelp, status: m.status}
}

func (m *appModel) restoreSession(s sessionState) {
	m.agendaCursor = s.agendaCursor
	m.agendaCol = s.agendaCol
	m.showHelp = s.showHelp
	m.status = s.status
	m.pendingDelete = false
}

func (m appModel) cycleMode() (tea.Model, tea.Cmd) {
	m.memory.align = m.captureSession()
	m.memory.hasAlign = true
	m.pendingDelete = false
	if m.memory.hasBox {
		box := *m.memory.box
		box.width = m.width
		box.height = m.height
		box.memory = m.memory
		// The kept screen is a cache: reload it from the store.
		if err := box.reload(); err != nil {
			box.errText = err.Error()
		}
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
	return openCal(m.dataPath, m.width, m.height, mem)
}

func (m appModel) helpGeneral() []helpItem {
	return []helpItem{
		{key: "tab", description: "Align / box / cal / nlp"},
		{key: "t", description: "Jump to today"},
		{key: "n", description: "New task in Vim"},
		{key: "e", description: "Edit selected in Vim"},
		{key: "space", description: "Mark selected done"},
		{key: "dd", description: "Delete selected"},
		{key: "? / esc", description: "Close this guide"},
	}
}

func (m appModel) helpFooter() string {
	return lipgloss.JoinVertical(
		lipgloss.Left,
		sectionStyle.Render("VIM TASK FILE"),
		mutedStyle.Render("n adds on the selected date · e edits the selected task"),
		mutedStyle.Render("Vim edits that one job as YAML, then crusade saves it"),
		mutedStyle.Render("Align is a list"),
		mutedStyle.Render("tab cycles align, box, cal, and nlp"),
	)
}

func renderTaskLines(records []store.AlignRecord, includeDate bool, height int, selected int) string {
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
		event := records[index].Align
		mark := "[ ]"
		if event.Done() {
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

func composeShown(text string, cursor int) string {
	return lineedit.Show(text, cursor, func(cell string) string { return cursorStyle.Render(cell) })
}

// addTaskInVim opens Vim on a new record for the selected date. The record
// is stored only when Vim saves it with an outcome and an impact.
func (m *appModel) addTaskInVim() tea.Cmd {
	m.pendingDelete = false
	m.status = "Editing a new task in Vim..."
	return openTaskEditor(0, model.DefaultAlign(m.selectedDate()))
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
	return openTaskEditor(target.ID, target.Align)
}

func (m *appModel) finishEditor(message editorFinishedMsg) {
	switch {
	case message.err != nil:
		m.status = fmt.Sprintf("error: Vim exited: %v", message.err)
	case !message.saved && message.id == 0:
		m.status = "New task cancelled."
	case !message.saved:
		m.status = "Saved."
	case message.id == 0:
		m.apply(store.Add(m.dataPath, message.event), "Saved.")
	default:
		m.apply(store.Update(m.dataPath, message.id, message.event), "Saved.")
	}
}

func (m appModel) selectedDate() time.Time {
	if target, found := m.selectedRecord(); found {
		return dateOnly(target.Align.X)
	}
	return dateOnly(time.Now())
}

func (m appModel) selectedRecord() (store.AlignRecord, bool) {
	if len(m.records) == 0 {
		return store.AlignRecord{}, false
	}
	return m.records[clamp(m.agendaCursor, 0, len(m.records)-1)], true
}

func recordByID(records []store.AlignRecord, id int) (store.AlignRecord, bool) {
	for _, record := range records {
		if record.ID == id {
			return record, true
		}
	}
	return store.AlignRecord{}, false
}

func (m *appModel) toggleDone() {
	m.pendingDelete = false
	record, found := m.selectedRecord()
	if !found {
		m.status = "No task selected."
		return
	}
	status := "Cleared."
	if record.Align.Done() {
		record.Align.DoneAt = time.Time{}
	} else {
		record.Align.DoneAt = time.Now()
		status = "Marked done."
	}
	m.apply(store.Update(m.dataPath, record.ID, record.Align), status)
}

func (m *appModel) handleAgendaKey(key string) {
	visible := m.records
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
	visible := m.records
	if len(visible) == 0 {
		return
	}
	cur := clamp(m.agendaCursor, 0, len(visible)-1)
	curDay := dateOnly(visible[cur].Align.X)
	if direction < 0 {
		for idx := cur - 1; idx >= 0; idx-- {
			day := dateOnly(visible[idx].Align.X)
			if day.Before(curDay) {
				for first := 0; first <= idx; first++ {
					if sameDate(dateOnly(visible[first].Align.X), day) {
						m.agendaCursor = first
						return
					}
				}
			}
		}
		return
	}
	for idx := cur + 1; idx < len(visible); idx++ {
		day := dateOnly(visible[idx].Align.X)
		if day.After(curDay) {
			m.agendaCursor = idx
			return
		}
	}
}

func (m *appModel) clampState() {
	m.agendaCursor = clamp(m.agendaCursor, 0, max(0, len(m.records)-1))
	m.agendaCol = clamp(m.agendaCol, 0, 3)
}

func (m appModel) renderAgenda(height int) string {
	if len(m.records) == 0 {
		return "No tasks yet. Press n or use crusade -m align -c.\n"
	}
	return renderTaskLines(m.records, true, height, m.agendaCursor)
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
