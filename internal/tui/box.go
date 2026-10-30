package tui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ryangerardwilson/crusade/internal/box"
	"github.com/ryangerardwilson/crusade/internal/store"
)

type boxEditorFinishedMsg struct {
	err error
}

var openBoxEditor = func(path, text string) tea.Cmd {
	fail := func(err error) tea.Cmd {
		return func() tea.Msg { return boxEditorFinishedMsg{err: err} }
	}
	tmp, err := os.CreateTemp("", "crusade-boxes-*.yaml")
	if err != nil {
		return fail(err)
	}
	if _, err := tmp.WriteString(text); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return fail(err)
	}
	tmpPath := tmp.Name()
	command := exec.Command("vim", tmpPath)
	return tea.ExecProcess(command, func(err error) tea.Msg {
		defer os.Remove(tmpPath)
		edited, readErr := os.ReadFile(tmpPath)
		if err != nil {
			return boxEditorFinishedMsg{err: err}
		}
		if readErr != nil {
			return boxEditorFinishedMsg{err: readErr}
		}
		outline, parseErr := box.ParseYAML(edited)
		if parseErr != nil {
			return boxEditorFinishedMsg{err: parseErr}
		}
		if saveErr := store.SaveOutline(path, outline); saveErr != nil {
			return boxEditorFinishedMsg{err: saveErr}
		}
		return boxEditorFinishedMsg{}
	})
}

type boxModel struct {
	dataPath string
	outline  box.Outline
	checked  map[string]bool
	day      string
	selected int
	showHelp bool
	status   string
	errText  string
	width      int
	height     int
	activeDays map[string]bool
	dayCounts  map[string]int
	memory     modeMemory
}

func runBox(dataPath string, stdout, stderr io.Writer) int {
	outline, err := store.LoadOutline(dataPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	day := time.Now().Local().Format(time.DateOnly)
	checked, err := store.LoadDayChecks(dataPath, day)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	active, err := store.LoadActiveDays(dataPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	counts, err := store.LoadCheckCounts(dataPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	model := newBoxModel(dataPath, outline, checked, day)
	model.activeDays = active
	model.dayCounts = counts
	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func newBoxModel(dataPath string, outline box.Outline, checked map[string]bool, day string) boxModel {
	if checked == nil {
		checked = map[string]bool{}
	}
	return boxModel{dataPath: dataPath, outline: outline, checked: checked, day: day}
}

func (m boxModel) Init() tea.Cmd { return nil }

func (m boxModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch message := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = message.Width
		m.height = message.Height
	case boxEditorFinishedMsg:
		if message.err != nil {
			m.errText = message.err.Error()
			m.status = ""
			return m, nil
		}
		outline, err := store.LoadOutline(m.dataPath)
		if err != nil {
			m.errText = err.Error()
			return m, nil
		}
		m.outline = outline
		m.errText = ""
		m.status = "Saved."
		m.clamp()
	case tea.KeyMsg:
		key := message.String()
		if key == "ctrl+c" || key == "esc" || key == "q" {
			return m, tea.Quit
		}
		if key == "tab" {
			return m.cycleMode()
		}
		if m.showHelp {
			if key == "?" {
				m.showHelp = false
			}
			return m, nil
		}
		switch key {
		case "?":
			m.showHelp = true
		case "j", "down":
			m.move(1)
		case "k", "up":
			m.move(-1)
		case "h", "left":
			m.moveDate(-1)
		case "l", "right":
			m.moveDate(1)
		case "ctrl+h":
			m.moveDate(-7)
		case "ctrl+l":
			m.moveDate(7)
		case " ", "enter":
			m.toggleSelected()
		case "e":
			payload, err := box.MarshalYAML(m.outline)
			if err != nil {
				m.errText = err.Error()
				return m, nil
			}
			return m, openBoxEditor(m.dataPath, string(payload))
		case "r":
			m.checked = map[string]bool{}
			m.save("Reset " + m.day + ".")
		}
	}
	return m, nil
}

func (m boxModel) View() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", renderModeBanner("BOX", m.width))
	fmt.Fprintf(&b, "%s\n\n", infoStyle.Render(fmt.Sprintf("%s  %d/%d", m.day, m.outline.CompletedCount(m.checked), m.outline.LeafCount())))
	items := m.outline.Flatten()
	if len(items) == 0 {
		b.WriteString(mutedStyle.Render("No boxes yet. Press e to edit the outline.") + "\n")
	}
	for index, item := range items {
		mark := "[ ]"
		switch m.outline.StatusFor(item.ID, m.checked) {
		case box.Checked:
			mark = "[x]"
		case box.Partial:
			mark = "[~]"
		}
		line := fmt.Sprintf("%s%s %s", strings.Repeat("  ", item.Depth), mark, item.Label)
		if index == m.selected {
			line = titleStyle.Render("> " + line)
		} else {
			line = "  " + line
		}
		fmt.Fprintf(&b, "%s\n", line)
	}
	fmt.Fprintf(&b, "\n%s\n", m.renderMomentum())
	if m.showHelp {
		fmt.Fprintf(&b, "\n%s\n", mutedStyle.Render("h/l day   ctrl+h/l week   j/k move   space check   e edit   r reset   tab mode   ? help   q quit"))
		fmt.Fprintf(&b, "%s\n", mutedStyle.Render("A parent is done when every box under it is done. The graph under the list is a contribution calendar: columns are weeks, rows are days, and bluer squares mean more boxes checked. The caption is the current streak. e opens Vim on the outline YAML, then crusade saves it."))
	}
	if m.status != "" {
		fmt.Fprintf(&b, "%s\n", mutedStyle.Render(m.status))
	}
	if m.errText != "" {
		fmt.Fprintf(&b, "%s\n", lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(m.errText))
	}
	return b.String()
}

func (m *boxModel) move(delta int) {
	items := m.outline.Flatten()
	if len(items) == 0 {
		return
	}
	next := m.selected + delta
	if next < 0 {
		next = len(items) - 1
	}
	if next >= len(items) {
		next = 0
	}
	m.selected = next
	m.status = ""
}

func (m *boxModel) moveDate(days int) {
	current, err := time.ParseInLocation(time.DateOnly, m.day, time.Local)
	if err != nil {
		m.errText = err.Error()
		return
	}
	nextDay := current.AddDate(0, 0, days).Format(time.DateOnly)
	checked, err := store.LoadDayChecks(m.dataPath, nextDay)
	if err != nil {
		m.errText = err.Error()
		return
	}
	m.day = nextDay
	m.checked = checked
	m.errText = ""
	m.status = "Viewing " + nextDay + "."
	m.clamp()
}

func (m *boxModel) toggleSelected() {
	items := m.outline.Flatten()
	if len(items) == 0 {
		return
	}
	m.clamp()
	item := items[m.selected]
	wasChecked := m.outline.StatusFor(item.ID, m.checked) == box.Checked
	next, err := box.Toggle(m.outline, m.checked, item.ID)
	if err != nil {
		m.errText = err.Error()
		return
	}
	m.checked = next
	verb := "Checked"
	if wasChecked {
		verb = "Unchecked"
	}
	m.save(verb + " " + item.Label + ".")
}

func (m *boxModel) save(status string) {
	if err := store.SaveDayChecks(m.dataPath, m.day, m.checked); err != nil {
		m.errText = err.Error()
		m.status = ""
		return
	}
	m.noteActiveDay()
	m.errText = ""
	m.status = status
}

func (m *boxModel) noteActiveDay() {
	if m.activeDays == nil {
		m.activeDays = map[string]bool{}
	}
	if m.dayCounts == nil {
		m.dayCounts = map[string]int{}
		for day, on := range m.activeDays {
			if on {
				m.dayCounts[day] = 1
			}
		}
	}
	count := 0
	for _, on := range m.checked {
		if on {
			count++
		}
	}
	if count > 0 {
		m.activeDays[m.day] = true
		m.dayCounts[m.day] = count
		return
	}
	delete(m.activeDays, m.day)
	delete(m.dayCounts, m.day)
}

// contributionLevelStyle is the crusade blue used by the badge and titles.
// Shade tracks boxes checked that day: none, one, two, three or four, then five or more.
var contributionLevelStyle = [5]lipgloss.Style{
	lipgloss.NewStyle().Foreground(lipgloss.Color("238")),
	lipgloss.NewStyle().Foreground(lipgloss.Color("23")),
	lipgloss.NewStyle().Foreground(lipgloss.Color("30")),
	lipgloss.NewStyle().Foreground(lipgloss.Color("37")),
	lipgloss.NewStyle().Foreground(lipgloss.Color("51")),
}

const (
	contributionGutter    = 4
	contributionCellWidth = 2
)

var contributionDayLabels = []string{"    ", "Mon ", "    ", "Wed ", "    ", "Fri ", "    "}

type contributionCell struct {
	day    time.Time
	count  int
	level  int
	future bool
}

func contributionLevel(count int) int {
	switch {
	case count <= 0:
		return 0
	case count == 1:
		return 1
	case count == 2:
		return 2
	case count <= 4:
		return 3
	default:
		return 4
	}
}

func contributionWeekCount(width int) int {
	const maxWeeks = 53
	if width <= 0 {
		return 26
	}
	weeks := (width - contributionGutter) / contributionCellWidth
	if weeks < 1 {
		return 1
	}
	if weeks > maxWeeks {
		return maxWeeks
	}
	return weeks
}

func contributionGrid(counts map[string]int, today time.Time, weeks int) [][]contributionCell {
	today = dateOnly(today)
	if weeks < 1 {
		weeks = 1
	}
	if counts == nil {
		counts = map[string]int{}
	}
	sunday := today.AddDate(0, 0, -int(today.Weekday()))
	start := sunday.AddDate(0, 0, -7*(weeks-1))
	grid := make([][]contributionCell, weeks)
	for week := 0; week < weeks; week++ {
		grid[week] = make([]contributionCell, 7)
		for row := 0; row < 7; row++ {
			day := start.AddDate(0, 0, week*7+row)
			cell := contributionCell{day: day, future: day.After(today)}
			if !cell.future {
				cell.count = counts[day.Format(time.DateOnly)]
				cell.level = contributionLevel(cell.count)
			}
			grid[week][row] = cell
		}
	}
	return grid
}

func momentumStreak(active map[string]bool, today time.Time) int {
	today = dateOnly(today)
	if active == nil {
		active = map[string]bool{}
	}
	end := today
	if !active[end.Format(time.DateOnly)] {
		end = end.AddDate(0, 0, -1)
	}
	if !active[end.Format(time.DateOnly)] {
		return 0
	}
	streak := 0
	for day := end; active[day.Format(time.DateOnly)]; day = day.AddDate(0, 0, -1) {
		streak++
	}
	return streak
}

func streakLabel(streak int) string {
	if streak == 1 {
		return "momentum  1 day streak"
	}
	return fmt.Sprintf("momentum  %d day streak", streak)
}

func contributionMonthHeader(grid [][]contributionCell) string {
	weeks := len(grid)
	if weeks == 0 {
		return ""
	}
	label := []rune(strings.Repeat(" ", weeks*contributionCellWidth))
	occupiedUntil := 0
	for week, days := range grid {
		month, ok := contributionMonthAt(days, week == 0)
		if !ok {
			continue
		}
		start := week * contributionCellWidth
		if start < occupiedUntil {
			continue
		}
		name := month.String()[:3]
		for i, r := range name {
			pos := start + i
			if pos < len(label) {
				label[pos] = r
			}
		}
		occupiedUntil = start + len(name) + 1
	}
	return string(label)
}

func contributionMonthAt(week []contributionCell, first bool) (time.Month, bool) {
	for _, cell := range week {
		if cell.day.Day() == 1 {
			return cell.day.Month(), true
		}
	}
	if first && len(week) > 0 {
		return week[0].day.Month(), true
	}
	return 0, false
}

func renderContributionCell(cell contributionCell, focused string) string {
	focusedDay := cell.day.Format(time.DateOnly) == focused
	if cell.future && !focusedDay {
		return strings.Repeat(" ", contributionCellWidth)
	}
	rendered := contributionLevelStyle[cell.level].Render("■")
	gap := contributionCellWidth - lipgloss.Width(rendered)
	if gap > 0 {
		return rendered + strings.Repeat(" ", gap)
	}
	return rendered
}

func contributionLegend(weeks int) string {
	squares := make([]string, 5)
	for level := 0; level < 5; level++ {
		squares[level] = contributionLevelStyle[level].Render("■")
	}
	legend := mutedStyle.Render("Less ") + strings.Join(squares, " ") + mutedStyle.Render(" More")
	gap := contributionGutter + weeks*contributionCellWidth - lipgloss.Width(legend)
	if gap < 0 {
		gap = 0
	}
	return strings.Repeat(" ", gap) + legend
}

func contributionTooltip(day string, count int) string {
	label := day
	if parsed, err := time.ParseInLocation(time.DateOnly, day, time.Local); err == nil {
		label = parsed.Format("Monday, Jan 2, 2006")
	}
	switch count {
	case 0:
		return "No boxes on " + label
	case 1:
		return "1 box on " + label
	default:
		return fmt.Sprintf("%d boxes on %s", count, label)
	}
}

func (m boxModel) contributionCounts() map[string]int {
	if m.dayCounts != nil {
		return m.dayCounts
	}
	counts := make(map[string]int, len(m.activeDays))
	for day, on := range m.activeDays {
		if on {
			counts[day] = 1
		}
	}
	return counts
}

func (m boxModel) renderMomentum() string {
	return m.renderContribution(time.Now())
}

func (m boxModel) renderContribution(today time.Time) string {
	counts := m.contributionCounts()
	grid := contributionGrid(counts, today, contributionWeekCount(m.width))
	var b strings.Builder
	b.WriteString(mutedStyle.Render(streakLabel(momentumStreak(m.activeDays, today))))
	b.WriteByte('\n')
	b.WriteString(mutedStyle.Render(strings.Repeat(" ", contributionGutter) + contributionMonthHeader(grid)))
	b.WriteByte('\n')
	for row := 0; row < 7; row++ {
		b.WriteString(mutedStyle.Render(contributionDayLabels[row]))
		for week := range grid {
			b.WriteString(renderContributionCell(grid[week][row], m.day))
		}
		b.WriteByte('\n')
	}
	b.WriteString(contributionLegend(len(grid)))
	b.WriteByte('\n')
	b.WriteString(mutedStyle.Render(contributionTooltip(m.day, counts[m.day])))
	return b.String()
}

func (m *boxModel) clamp() {
	items := m.outline.Flatten()
	if len(items) == 0 || m.selected < 0 {
		m.selected = 0
		return
	}
	if m.selected >= len(items) {
		m.selected = len(items) - 1
	}
}
