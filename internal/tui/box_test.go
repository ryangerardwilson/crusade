package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ryangerardwilson/crusade/internal/box"
	"github.com/ryangerardwilson/crusade/internal/store"
)

func TestBoxToggleSavesAndDayMoveDoesNotCreateRows(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SQLX_ROOT", root)
	outlinePath := filepath.Join(t.TempDir(), "rituals.txt")
	if err := os.WriteFile(outlinePath, []byte("Move\nWork\n  Email\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BOXES_CONFIG", outlinePath)
	t.Setenv("BOXES_DATA_HOME", t.TempDir())
	db := store.DatabasePath()
	if _, err := store.Open(db); err != nil {
		t.Fatal(err)
	}
	outline, err := store.LoadOutline(db)
	if err != nil {
		t.Fatal(err)
	}
	m := newBoxModel(db, outline, nil, "2026-10-20")
	m.selected = 2
	m.toggleSelected()
	checked, err := store.LoadDayChecks(db, "2026-10-20")
	if err != nil || !checked["work/email"] {
		t.Fatalf("checks = %#v err=%v", checked, err)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	got := next.(boxModel)
	if got.day != "2026-10-21" {
		t.Fatalf("day = %s", got.day)
	}
	empty, err := store.LoadDayChecks(db, "2026-10-21")
	if err != nil || len(empty) != 0 {
		t.Fatalf("next day checks = %#v err=%v", empty, err)
	}
}

func TestModeKeyDoesNotToggle(t *testing.T) {
	m := newAppModel(filepath.Join(t.TempDir(), "tasks.sqlite"), nil)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	got := next.(appModel)
	if got.mode != modeDo || cmd != nil {
		t.Fatalf("mode=%q cmd=%v", got.mode, cmd != nil)
	}
}

func TestBoxShortcutsStayHiddenUntilHelp(t *testing.T) {
	m := newBoxModel("", box.Outline{}, nil, "2026-10-20")
	out := m.View()
	if strings.Contains(out, "space check") || strings.Contains(out, "? help") {
		t.Fatalf("shortcuts visible before help:\n%s", out)
	}
	if !strings.Contains(out, "momentum") || !strings.Contains(out, "day streak") {
		t.Fatalf("momentum graph missing:\n%s", out)
	}
	m.showHelp = true
	out = m.View()
	for _, want := range []string{"space check", "e edit", "? help"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help missing %q:\n%s", want, out)
		}
	}
}

func TestModeBadgesShareTheBlueCorner(t *testing.T) {
	forceStyledOutput(t)
	do := appModel{mode: modeDo, view: viewAgenda, width: 48, height: 12}
	align := appModel{mode: modeAlign, view: viewAgenda, width: 48, height: 12}
	box := newBoxModel("", box.Outline{}, nil, "2026-10-20")
	box.width = 48
	views := []struct {
		label string
		line  string
	}{
		{"DO", strings.Split(do.View(), "\n")[0]},
		{"ALIGN", strings.Split(align.View(), "\n")[0]},
		{"BOX", strings.Split(box.View(), "\n")[0]},
	}
	color := ""
	for _, view := range views {
		plain := stripANSI(view.line)
		if !strings.HasPrefix(plain, "crusade") || !strings.HasSuffix(strings.TrimRight(plain, " "), view.label) {
			t.Fatalf("%s banner = %q", view.label, plain)
		}
		got := badgeEscape(view.line, view.label)
		if got == "" {
			t.Fatalf("%s badge has no color: %q", view.label, view.line)
		}
		if color == "" {
			color = got
			continue
		}
		if got != color {
			t.Fatalf("%s badge color %q != %q", view.label, got, color)
		}
	}
	if strings.Contains(color, "220") {
		t.Fatalf("mode badge still uses amber: %q", color)
	}
	if !strings.Contains(box.View(), "2026-10-20  0/0") {
		t.Fatalf("box summary missing day count:\n%s", box.View())
	}
}

func badgeEscape(line, label string) string {
	idx := strings.LastIndex(line, label)
	if idx < 0 {
		return ""
	}
	esc := strings.LastIndex(line[:idx], "\x1b[")
	if esc < 0 {
		return ""
	}
	end := strings.Index(line[esc:], "m")
	if end < 0 {
		return ""
	}
	return line[esc : esc+end+1]
}

func TestBoxMomentumGraph(t *testing.T) {
	today := dateOnly(time.Now())
	key := func(day time.Time) string { return day.Format(time.DateOnly) }
	active := map[string]bool{
		key(today):                   true,
		key(today.AddDate(0, 0, -1)): true,
		key(today.AddDate(0, 0, -4)): true,
	}
	if momentumStreak(active, today) != 2 {
		t.Fatalf("streak = %d", momentumStreak(active, today))
	}

	broken := map[string]bool{
		key(today.AddDate(0, 0, -1)): true,
		key(today.AddDate(0, 0, -2)): true,
		key(today.AddDate(0, 0, -4)): true,
	}
	if momentumStreak(broken, today) != 2 {
		t.Fatalf("open today streak = %d", momentumStreak(broken, today))
	}

	gap := map[string]bool{
		key(today):                   true,
		key(today.AddDate(0, 0, -2)): true,
	}
	if momentumStreak(gap, today) != 1 {
		t.Fatalf("gap streak = %d", momentumStreak(gap, today))
	}

	m := newBoxModel("", box.Outline{}, nil, key(today))
	m.width = 40
	m.activeDays = active
	out := stripANSI(m.View())
	if !strings.Contains(out, "momentum  2 day streak") {
		t.Fatalf("view:\n%s", out)
	}
	for _, want := range []string{"Mon ", "Wed ", "Fri ", "Less", "More", "■"} {
		if !strings.Contains(out, want) {
			t.Fatalf("graph missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "▣") || strings.Contains(out, "░") || strings.Contains(out, "▓") || strings.Contains(out, "█") {
		t.Fatalf("graph glyph drifted:\n%s", out)
	}
}

func TestContributionCalendar(t *testing.T) {
	today := time.Date(2026, 10, 22, 15, 0, 0, 0, time.Local)
	if today.Weekday() != time.Thursday {
		t.Fatalf("fixture weekday = %s", today.Weekday())
	}
	counts := map[string]int{
		"2026-09-13": 2,
		"2026-10-18": 2,
		"2026-10-21": 1,
		"2026-10-22": 5,
		"2026-10-15": 4,
	}
	grid := contributionGrid(counts, today, 6)
	if len(grid) != 6 || len(grid[0]) != 7 {
		t.Fatalf("shape %d x %d", len(grid), len(grid[0]))
	}
	if got := grid[0][0].day.Format(time.DateOnly); got != "2026-09-13" || grid[0][0].level != 2 {
		t.Fatalf("start = %+v", grid[0][0])
	}
	if grid[5][0].day.Format(time.DateOnly) != "2026-10-18" || grid[5][0].level != 2 {
		t.Fatalf("sunday = %+v", grid[5][0])
	}
	if grid[5][3].level != 1 || grid[5][3].day.Format(time.DateOnly) != "2026-10-21" {
		t.Fatalf("wednesday = %+v", grid[5][3])
	}
	if grid[5][4].level != 4 || grid[5][4].future || grid[5][4].day.Weekday() != time.Thursday {
		t.Fatalf("today cell = %+v", grid[5][4])
	}
	if !grid[5][5].future || grid[5][5].day.Format(time.DateOnly) != "2026-10-23" {
		t.Fatalf("friday = %+v", grid[5][5])
	}
	if got := contributionMonthHeader(grid); got != "Sep Oct"+strings.Repeat(" ", 5) {
		t.Fatalf("header %q", got)
	}
	for _, label := range contributionDayLabels {
		if len(label) != contributionGutter {
			t.Fatalf("day label %q width %d", label, len(label))
		}
	}
	for _, sample := range []struct {
		count int
		level int
	}{{0, 0}, {1, 1}, {2, 2}, {3, 3}, {4, 3}, {5, 4}} {
		if contributionLevel(sample.count) != sample.level {
			t.Fatalf("level(%d) = %d", sample.count, contributionLevel(sample.count))
		}
	}

	m := newBoxModel("", box.Outline{}, nil, "2026-10-22")
	m.width = 16
	m.dayCounts = counts
	m.activeDays = map[string]bool{"2026-10-22": true, "2026-10-21": true}
	raw := m.renderContribution(today)
	out := stripANSI(raw)
	lines := strings.Split(out, "\n")
	if len(lines) < 9 {
		t.Fatalf("calendar lines:\n%s", out)
	}
	rows := lines[2:9]
	width := lipgloss.Width(rows[0])
	for _, row := range rows[1:] {
		if lipgloss.Width(row) != width {
			t.Fatalf("row width %d != %d:\n%s", lipgloss.Width(row), width, out)
		}
	}
	square := stripANSI(renderContributionCell(contributionCell{level: 1}, ""))
	if lines[6] != "    "+strings.Repeat(square, 6) {
		t.Fatalf("thursday %q", lines[6])
	}
	if lines[7] != "Fri "+strings.Repeat(square, 5)+strings.Repeat(" ", contributionCellWidth) {
		t.Fatalf("friday %q", lines[7])
	}
	forceStyledOutput(t)
	if contributionLevelStyle[4].Render("■") != infoStyle.Render("■") {
		t.Fatal("brightest square is not the UI blue")
	}
	viewed := contributionCell{day: time.Date(2026, 10, 20, 0, 0, 0, 0, time.Local), level: 2}
	focused := renderContributionCell(viewed, "2026-10-20")
	plain := renderContributionCell(viewed, "2026-10-21")
	if focused != plain || lipgloss.Width(focused) != contributionCellWidth {
		t.Fatalf("viewed day drifted: %q width %d", stripANSI(focused), lipgloss.Width(focused))
	}
	if !strings.Contains(out, "momentum  2 day streak") {
		t.Fatalf("streak line missing:\n%s", out)
	}
	if !strings.Contains(out, "5 boxes on Thursday, Oct 22, 2026") {
		t.Fatalf("tooltip missing:\n%s", out)
	}
	if !strings.Contains(out, "Less") || !strings.Contains(out, "More") {
		t.Fatalf("legend missing:\n%s", out)
	}
}

func TestSaveUpdatesTheStreakDay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crusade.sqlite")
	t.Setenv("BOXES_CONFIG", "")
	day := time.Now().Local().Format(time.DateOnly)
	m := newBoxModel(path, box.Outline{}, map[string]bool{"move": true}, day)
	m.activeDays = map[string]bool{}
	m.save("checked")
	if !m.activeDays[day] {
		t.Fatal("checked day was not marked active")
	}
	if m.dayCounts[day] != 1 {
		t.Fatalf("count = %d", m.dayCounts[day])
	}
	m.checked = map[string]bool{}
	m.save("reset")
	if m.activeDays[day] {
		t.Fatal("empty day stayed active")
	}
	if _, ok := m.dayCounts[day]; ok {
		t.Fatal("empty day kept a count")
	}
	days, err := store.LoadActiveDays(path)
	if err != nil || days[day] {
		t.Fatalf("stored days = %#v err=%v", days, err)
	}
}
