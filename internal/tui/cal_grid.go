package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/ryangerardwilson/crusade/internal/model"
)

const gridSlot = time.Hour

func (m calModel) gridCursor() time.Time {
	if m.cursor.IsZero() {
		return clockOn(m.day, 9, 0)
	}
	return snapDown(clockOn(m.day, m.cursor.Hour(), m.cursor.Minute()))
}

func (m *calModel) moveGrid(delta int) {
	next := m.gridCursor().Add(time.Duration(delta) * gridSlot)
	start, end := m.gridBounds()
	if next.Before(start) || !next.Before(end) {
		return
	}
	m.cursor = next
	if _, index, ok := m.itemAt(next); ok {
		m.selected = index
		return
	}
	m.selected = 0
}

func (m *calModel) toggleSpan() {
	if m.visual {
		m.visual = false
		return
	}
	m.anchor = m.gridCursor()
	m.cursor = m.anchor
	m.visual = true
}

func (m calModel) selectionSpan() (time.Time, time.Time) {
	start := m.anchor
	end := m.gridCursor()
	if start.IsZero() {
		start = end
	}
	if end.Before(start) {
		start, end = end, start
	}
	return start, end.Add(gridSlot)
}

func (m calModel) gridBounds() (time.Time, time.Time) {
	start := clockOn(m.day, 0, 0)
	return start, start.AddDate(0, 0, 1)
}

func (m calModel) allSlots() []time.Time {
	start, end := m.gridBounds()
	slots := make([]time.Time, 0, 32)
	for slot := start; slot.Before(end); slot = slot.Add(gridSlot) {
		slots = append(slots, slot)
		if len(slots) > 64 {
			break
		}
	}
	return slots
}

type sheetLayout struct {
	gutter   int
	days     []time.Time
	colWidth int
}

func (m calModel) sheet() sheetLayout {
	const gutter = 6
	const minCol = 8
	all := m.weekDays()
	width := m.width
	if width <= 0 {
		width = 80
	}
	fit := 1
	if width > gutter {
		fit = (width - gutter) / minCol
		if fit < 1 {
			fit = 1
		}
	}
	if fit > len(all) {
		fit = len(all)
	}
	focus := 0
	for i, day := range all {
		if sameDate(day, m.day) {
			focus = i
			break
		}
	}
	start := focus - fit/2
	if start < 0 {
		start = 0
	}
	if start+fit > len(all) {
		start = len(all) - fit
	}
	col := 1
	if width > gutter {
		col = (width - gutter) / fit
	}
	if col < 1 {
		col = 1
	}
	return sheetLayout{gutter: gutter, days: all[start : start+fit], colWidth: col}
}

func cellPad(value string, width int) string {
	runes := []rune(value)
	if width <= 0 {
		return ""
	}
	if len(runes) > width {
		runes = runes[:width]
	}
	if len(runes) < width {
		runes = append(runes, []rune(strings.Repeat(" ", width-len(runes)))...)
	}
	return string(runes)
}

func (m calModel) gridRows() int {
	overhead := 11
	if m.weekEmpty() {
		overhead++
	}
	if m.visual && !m.composing {
		overhead++
	}
	if m.composing || (m.scope != "" && !m.composing) {
		overhead += 2
	}
	if m.status != "" || m.errText != "" {
		overhead++
	}
	rows := m.height - overhead
	if m.height == 0 || rows < 8 {
		rows = 16
	}
	return rows
}

func (m calModel) visibleSlots() []time.Time {
	slots := m.allSlots()
	rows := m.gridRows()
	if len(slots) <= rows {
		return slots
	}
	cursor := m.gridCursor()
	index := 0
	for i, slot := range slots {
		if !slot.Before(cursor) {
			index = i
			break
		}
	}
	start := index - 2
	if start < 0 {
		start = 0
	}
	if start+rows > len(slots) {
		start = len(slots) - rows
	}
	return slots[start : start+rows]
}

func (m calModel) itemAt(slot time.Time) (calItem, int, bool) {
	slotEnd := slot.Add(gridSlot)
	for index, item := range m.itemsOn(m.day) {
		if slot.Before(item.end) && item.start.Before(slotEnd) {
			return item, index, true
		}
	}
	return calItem{}, 0, false
}

func (m calModel) renderGrid() string {
	slots := m.visibleSlots()
	var b strings.Builder
	for _, slot := range slots {
		fmt.Fprintf(&b, "%s\n", m.renderSlot(slot, slots))
	}
	return b.String()
}

func (m calModel) renderSlot(slot time.Time, slots []time.Time) string {
	sheet := m.sheet()
	var b strings.Builder
	b.WriteString(cellPad(slot.Format("15:04"), sheet.gutter))
	for _, day := range sheet.days {
		cellSlot := clockOn(day, slot.Hour(), slot.Minute())
		text := cellPad(m.cellText(day, cellSlot, slots), sheet.colWidth)
		if m.slotSelected(cellSlot) {
			text = activeCellStyle.Render(text)
		}
		b.WriteString(text)
	}
	return b.String()
}

func (m calModel) cellText(day, slot time.Time, slots []time.Time) string {
	item, ok := m.itemOn(day, slot)
	if !ok {
		return ""
	}
	mapped := make([]time.Time, len(slots))
	for i, row := range slots {
		mapped[i] = clockOn(day, row.Hour(), row.Minute())
	}
	if firstSlotOf(item, slot, mapped) {
		return strings.TrimSpace(item.title)
	}
	return "▌"
}

func (m calModel) itemOn(day, slot time.Time) (calItem, bool) {
	slotEnd := slot.Add(gridSlot)
	for _, item := range m.itemsOn(day) {
		if slot.Before(item.end) && item.start.Before(slotEnd) {
			return item, true
		}
	}
	return calItem{}, false
}

func (m calModel) slotSelected(slot time.Time) bool {
	if m.visual {
		start, end := m.selectionSpan()
		return !slot.Before(start) && slot.Before(end)
	}
	return slot.Equal(m.gridCursor())
}

func firstSlotOf(item calItem, slot time.Time, slots []time.Time) bool {
	for _, earlier := range slots {
		if !earlier.Before(slot) {
			break
		}
		slotEnd := earlier.Add(gridSlot)
		if earlier.Before(item.end) && item.start.Before(slotEnd) {
			return false
		}
	}
	return true
}

func itemLabel(item calItem) string {
	label := fmt.Sprintf("%s-%s  %s", item.start.Format("15:04"), item.end.Format("15:04"), strings.TrimSpace(item.title))
	if item.freq != "" {
		label += "  " + model.FreqLabel(item.freq)
	}
	if strings.TrimSpace(item.url) != "" {
		label += "  " + strings.TrimSpace(item.url)
	}
	return label
}

func snapDown(value time.Time) time.Time {
	return clockOn(value, value.Hour(), 0)
}

func snapUp(value time.Time) time.Time {
	down := snapDown(value)
	if down.Before(value) {
		return down.Add(gridSlot)
	}
	return down
}

func blockWindowFrom(start time.Time, items []calItem, ignoreBlockID int) (time.Time, time.Time, error) {
	limit := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location()).AddDate(0, 0, 1)
	for step := 0; step < 48; step++ {
		if !start.Before(limit) {
			return time.Time{}, time.Time{}, fmt.Errorf("no hour left on this day")
		}
		end := start.Add(time.Hour)
		if end.After(limit) {
			return time.Time{}, time.Time{}, fmt.Errorf("no hour left on this day")
		}
		latest := time.Time{}
		hit := false
		for _, item := range items {
			if ignoreBlockID != 0 && item.blockID == ignoreBlockID {
				continue
			}
			if start.Before(item.end) && item.start.Before(end) {
				hit = true
				if !item.end.Before(latest) {
					latest = item.end
				}
			}
		}
		if !hit {
			return start, end, nil
		}
		if !latest.After(start) {
			return time.Time{}, time.Time{}, fmt.Errorf("no hour left on this day")
		}
		start = latest
	}
	return time.Time{}, time.Time{}, fmt.Errorf("no hour left on this day")
}
