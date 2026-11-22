package tui

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ryangerardwilson/crusade/internal/model"
	"github.com/ryangerardwilson/crusade/internal/store"
)

// calModel blocks clock time. A row is one block or one day of a repeating series.
type calModel struct {
	dataPath       string
	blocks         []store.BlockRecord
	repeats        []store.RepeatRecord
	day            time.Time
	selected       int
	showHelp       bool
	composing      bool
	composeKind    string
	composeText    string
	composeCursor  int
	pendingDelete  bool
	scope          string
	scopeChoice    string
	targetBlockID  int
	targetRepeatID int
	targetDay      time.Time
	status         string
	errText        string
	width          int
	height         int
	memory         modeMemory
	// closed: opening a meeting link from the Super+C window closes crusade.
	closed          bool
	cursor          time.Time
	anchor          time.Time
	visual          bool
	spanSet         bool
	spanStart       time.Time
	spanEnd         time.Time
	formField       int
	formStart       string
	formStartCursor int
	formEnd         string
	formEndCursor   int
	formURL         string
	formURLCursor   int
	formRepeat      int
	formApply       int
	formDayMask     uint8
	formDayCursor   int
	formHasApply    bool
	formEditing     bool
}

type calItem struct {
	blockID    int
	repeatID   int
	day        time.Time
	title      string
	start      time.Time
	end        time.Time
	freq       string
	url        string
	overridden bool
}

type calPick struct {
	blockID  int
	repeatID int
}

func runCal(dataPath string, stdout, stderr io.Writer) Result {
	_ = stdout
	model := newCalModel(dataPath, nil)
	if err := model.reload(); err != nil {
		fmt.Fprintln(stderr, err)
		return Result{Code: 1}
	}
	return runProgram(model, stderr)
}

func newCalModel(dataPath string, blocks []store.BlockRecord) calModel {
	return calModel{
		dataPath: dataPath,
		blocks:   blocks,
		day:      dateOnly(time.Now()),
	}
}

func openCal(dataPath string, width, height int, mem modeMemory) (tea.Model, tea.Cmd) {
	if mem.hasCal {
		saved := *mem.cal
		saved.width = width
		saved.height = height
		saved.memory = mem
		// The kept screen is a cache: reload it from the store.
		if err := saved.reload(); err != nil {
			saved.errText = err.Error()
		}
		rememberMode(dataPath, model.ModeCal)
		return saved, nil
	}
	next := newCalModel(dataPath, nil)
	next.width = width
	next.height = height
	next.memory = mem
	if err := next.reload(); err != nil {
		next.errText = err.Error()
	}
	rememberMode(dataPath, model.ModeCal)
	return next, nil
}

func (m calModel) Init() tea.Cmd { return nil }

func (m calModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch message := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = message.Width
		m.height = message.Height
	case tea.KeyMsg:
		key := message.String()
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		if m.composing {
			if message.Paste || (message.Type == tea.KeyRunes && len(message.Runes) > 1) {
				m.pasteIntoForm(message.Runes)
				return m, nil
			}
			if key == "esc" {
				m.cancelCompose()
				return m, nil
			}
			m.handleComposeKey(key)
			return m, nil
		}
		if m.scope != "" {
			m.handleScope(key)
			return m, nil
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
		if m.visual && key == "esc" {
			m.visual = false
			m.spanSet = false
			return m, nil
		}
		if key == "q" || key == "esc" {
			return m, tea.Quit
		}
		if key == "tab" {
			return m.cycleMode()
		}
		return m.handleCalGridKey(key)
	}
	return m, nil
}

func (m calModel) handleCalGridKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "?":
		m.pendingDelete = false
		m.showHelp = true
	case "j", "down":
		m.pendingDelete = false
		m.moveGrid(1)
	case "k", "up":
		m.pendingDelete = false
		m.moveGrid(-1)
	case "v":
		m.pendingDelete = false
		m.toggleSpan()
	case "h", "left":
		m.pendingDelete = false
		m.moveDay(-1)
	case "l", "right":
		m.pendingDelete = false
		m.moveDay(1)
	case "ctrl+h":
		m.pendingDelete = false
		m.moveDay(-7)
	case "ctrl+l":
		m.pendingDelete = false
		m.moveDay(7)
	case "t":
		clock := m.gridCursor()
		m.pendingDelete = false
		m.visual = false
		m.spanSet = false
		m.day = dateOnly(time.Now())
		m.cursor = clockOn(m.day, clock.Hour(), clock.Minute())
		m.selected = 0
		m.status = ""
		m.errText = ""
		m.clamp()
	case "n":
		m.beginNew()
	case "e":
		m.beginRename()
	case "r":
		m.beginReschedule()
	case "d":
		m.deleteSelected()
	case "o":
		if m.openSelected() {
			m.closed = true
			return m, tea.Quit
		}
	default:
		m.pendingDelete = false
	}
	return m, nil
}

func (m calModel) closedCrusade() bool {
	return m.closed
}

func (m calModel) cycleMode() (tea.Model, tea.Cmd) {
	mem := m.memory
	saved := m
	saved.memory = modeMemory{}
	mem.cal = &saved
	mem.hasCal = true
	next, err := openNLPMode(m.dataPath, m.width, m.height, mem)
	if err != nil {
		m.errText = err.Error()
		return m, nil
	}
	rememberMode(m.dataPath, model.ModeNLP)
	return next, nil
}

func (m calModel) View() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", renderModeBanner("CAL", m.width))
	fmt.Fprintf(&b, "%s\n\n", infoStyle.Render(headerDate(m.day)))
	if !m.composing {
		fmt.Fprintf(&b, "%s\n", m.renderFocus())
	}
	fmt.Fprintf(&b, "%s\n\n", m.renderWeek())
	if m.composing {
		fmt.Fprintf(&b, "%s\n", m.renderForm())
	} else {
		if m.weekEmpty() {
			b.WriteString(mutedStyle.Render("No time blocked.") + "\n")
		}
		if m.visual {
			start, end := m.selectionSpan()
			fmt.Fprintf(&b, "%s\n", infoStyle.Render(start.Format("15:04")+"-"+end.Format("15:04")))
		}
		b.WriteString(m.renderGrid())
		if m.scope != "" {
			fmt.Fprintf(&b, "\n%s\n", infoStyle.Render(m.scopePrompt()))
		}
	}
	if m.status != "" {
		fmt.Fprintf(&b, "%s\n", mutedStyle.Render(m.status))
	}
	if m.errText != "" {
		fmt.Fprintf(&b, "%s\n", errorStyle.Render(m.errText))
	}
	if m.showHelp {
		return placeModal(b.String(), m.helpModal(), m.width, m.height)
	}
	return b.String()
}

func (m calModel) helpModal() string {
	items := []helpItem{
		{key: "h / l", description: "Previous / next day"},
		{key: "ctrl+h/l", description: "Previous / next week"},
		{key: "j / k", description: "One hour"},
		{key: "v", description: "Select a span"},
		{key: "n", description: "New block"},
		{key: "e / r", description: "Edit the block"},
		{key: "d", description: "Delete"},
		{key: "o", description: "Open the meeting link"},
		{key: "t", description: "Today"},
		{key: "tab", description: "Align / box / cal / nlp"},
		{key: "? / esc", description: "Close this guide"},
		{key: "q", description: "Leave"},
	}
	detail := "The focus box shows the block under the cursor. o opens its meeting link in the browser. From the Super+C window, that also closes crusade. n and e open a form for the title, start, end, an optional meeting url, and the repeat. days repeats on chosen weekdays. On a repeat, the form applies to this event, this and following, or all events. A meeting url applies to every occurrence. d removes a repeat the same way: o, f, or a."
	return shortcutModal(m.width, "CAL", items, detail)
}

func (m calModel) scopePrompt() string {
	switch m.scope {
	case "rename":
		return "Rename. o this event, f this and following, a all events. esc cancels."
	case "reschedule":
		return "Reschedule. o this event, f this and following, a all events. esc cancels."
	default:
		return "Remove this event (o), delete this and following (f), or delete all events (a). esc cancels."
	}
}

func (m calModel) renderFocus() string {
	text := m.day.Format("Mon 2 Jan") + "  " + m.gridCursor().Format("15:04")
	if item, _, ok := m.itemAt(m.gridCursor()); ok {
		text = itemLabel(item)
	}
	width := m.width
	if width < 4 {
		width = 4
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("250")).Width(width - 2).Render(text)
}

func (m calModel) weekEmpty() bool {
	for _, day := range m.weekDays() {
		if len(m.itemsOn(day)) > 0 {
			return false
		}
	}
	return true
}

func (m calModel) renderWeek() string {
	sheet := m.sheet()
	names := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	var b strings.Builder
	b.WriteString(cellPad("", sheet.gutter))
	for _, day := range sheet.days {
		name := names[(int(day.Weekday())+6)%7]
		text := cellPad(fmt.Sprintf("%s %d", name, day.Day()), sheet.colWidth)
		if sameDate(day, m.day) {
			text = activeCellStyle.Render(text)
		}
		b.WriteString(text)
	}
	return b.String()
}

func (m calModel) weekDays() []time.Time {
	start := mondayOf(m.day)
	days := make([]time.Time, 7)
	for index := range days {
		days[index] = start.AddDate(0, 0, index)
	}
	return days
}

func (m calModel) itemsOn(day time.Time) []calItem {
	day = dateOnly(day)
	items := make([]calItem, 0)
	for _, block := range m.blocks {
		if !sameDate(block.Block.Start, day) {
			continue
		}
		items = append(items, calItem{
			blockID: block.ID,
			day:     day,
			title:   block.Block.Title,
			start:   block.Block.Start,
			end:     block.Block.End,
			url:     block.Block.URL,
		})
	}
	for _, repeat := range m.repeats {
		occurrence, ok := repeat.On(day)
		if !ok {
			continue
		}
		items = append(items, calItem{
			repeatID:   repeat.ID,
			day:        day,
			title:      occurrence.Title,
			start:      occurrence.Start,
			end:        occurrence.End,
			freq:       repeat.Repeat.Freq,
			url:        occurrence.URL,
			overridden: occurrence.Overridden,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].start.Equal(items[j].start) {
			if items[i].blockID != items[j].blockID {
				return items[i].blockID < items[j].blockID
			}
			return items[i].repeatID < items[j].repeatID
		}
		return items[i].start.Before(items[j].start)
	})
	return items
}

func (m *calModel) moveDay(delta int) {
	clock := m.gridCursor()
	m.day = dateOnly(m.day.AddDate(0, 0, delta))
	m.cursor = clockOn(m.day, clock.Hour(), clock.Minute())
	m.visual = false
	m.spanSet = false
	m.selected = 0
	m.status = ""
	m.errText = ""
	m.pendingDelete = false
	m.scope = ""
	m.scopeChoice = ""
	m.clamp()
}

func (m *calModel) clamp() {
	items := m.itemsOn(m.day)
	if len(items) == 0 || m.selected < 0 {
		m.selected = 0
		return
	}
	if m.selected >= len(items) {
		m.selected = len(items) - 1
	}
}

func (m *calModel) handleScope(key string) {
	switch key {
	case "esc":
		m.clearScope()
		m.status = ""
		m.errText = ""
	case "o", "f", "a":
		if m.scope == "delete" {
			m.applyDelete(key)
		}
	}
}

func (m *calModel) selectedItem() (calItem, bool) {
	item, index, ok := m.itemAt(m.gridCursor())
	if !ok {
		return calItem{}, false
	}
	m.selected = index
	return item, true
}

func (m *calModel) capture(item calItem) {
	m.targetBlockID = item.blockID
	m.targetRepeatID = item.repeatID
	m.targetDay = item.day
}

func (m *calModel) clearScope() {
	m.scope = ""
	m.scopeChoice = ""
}

func (m *calModel) cancelCompose() {
	m.composing = false
	m.composeKind = ""
	m.composeText = ""
	m.composeCursor = 0
	m.clearScope()
	m.status = ""
	m.spanSet = false
	m.formEditing = false
	m.formHasApply = false
	m.formField = 0
	m.formDayMask = 0
	m.formDayCursor = 0
	m.formURL = ""
	m.formURLCursor = 0
}

func (m *calModel) handleComposeKey(key string) {
	m.handleFormKey(key)
}

func (m *calModel) applyDelete(choice string) {
	scope := scopeOf(choice)
	status := map[model.Scope]string{
		model.ScopeThis:      "Removed this event.",
		model.ScopeFollowing: "Deleted this and following.",
		model.ScopeAll:       "Deleted all events.",
	}[scope]
	err := store.DeleteOccurrence(m.dataPath, m.targetRepeatID, m.targetDay, scope)
	m.finish(status, err, calPick{})
}

// scopeOf reads the o, f, and a choices.
func scopeOf(choice string) model.Scope {
	switch choice {
	case "o":
		return model.ScopeThis
	case "f":
		return model.ScopeFollowing
	default:
		return model.ScopeAll
	}
}

func (m *calModel) deleteSelected() {
	item, ok := m.selectedItem()
	if !ok {
		m.pendingDelete = false
		m.status = "No block selected to delete."
		return
	}
	if item.repeatID != 0 {
		m.pendingDelete = false
		m.capture(item)
		m.scope = "delete"
		m.scopeChoice = ""
		m.errText = ""
		m.status = ""
		return
	}
	if !m.pendingDelete {
		m.pendingDelete = true
		m.status = "Press d again to delete the selected block."
		return
	}
	m.pendingDelete = false
	if err := store.DeleteBlock(m.dataPath, item.blockID); err != nil {
		m.fail(err)
		return
	}
	m.finish("Deleted "+item.title+".", nil, calPick{})
}

func (m *calModel) fail(err error) {
	m.errText = err.Error()
	m.status = ""
}

func (m *calModel) finish(status string, err error, pick calPick) {
	if err != nil {
		m.fail(err)
		return
	}
	if err := m.reload(); err != nil {
		m.fail(err)
		return
	}
	m.selectPick(pick)
	m.errText = ""
	m.visual = false
	m.spanSet = false
	m.cancelCompose()
	m.status = status
}

func (m *calModel) reload() error {
	blocks, err := store.LoadBlocks(m.dataPath)
	if err != nil {
		return err
	}
	repeats, err := store.LoadRepeats(m.dataPath)
	if err != nil {
		return err
	}
	m.blocks = blocks
	m.repeats = repeats
	m.clamp()
	return nil
}

func (m *calModel) selectPick(pick calPick) {
	for index, item := range m.itemsOn(m.day) {
		if pick.repeatID != 0 && item.repeatID == pick.repeatID {
			m.selected = index
			m.cursor = snapDown(item.start)
			return
		}
		if pick.blockID != 0 && item.blockID == pick.blockID {
			m.selected = index
			m.cursor = snapDown(item.start)
			return
		}
	}
	m.selected = 0
	m.clamp()
}

func clockOn(day time.Time, hour, minute int) time.Time {
	day = dateOnly(day)
	return time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, time.Local)
}

func clockFrom(day, clock time.Time) time.Time {
	day = dateOnly(day)
	return time.Date(day.Year(), day.Month(), day.Day(), clock.Hour(), clock.Minute(), clock.Second(), 0, time.Local)
}

func clockEndFrom(day, clock time.Time) time.Time {
	placed := clockFrom(day, clock)
	if !placed.After(dateOnly(day)) {
		return placed.AddDate(0, 0, 1)
	}
	return placed
}

func parseClock(token string) (int, int, bool) {
	parts := strings.Split(token, ":")
	if len(parts) != 2 || parts[0] == "" || len(parts[1]) != 2 {
		return 0, 0, false
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, false
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, false
	}
	return hour, minute, true
}
