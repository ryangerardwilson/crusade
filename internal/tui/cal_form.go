package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/ryangerardwilson/crusade/internal/lineedit"
	"github.com/ryangerardwilson/crusade/internal/model"
	"github.com/ryangerardwilson/crusade/internal/store"
)

const (
	formTitle = iota
	formStart
	formEnd
	formURL
	formRepeat
	formDays
	formApply
)

func (m *calModel) beginNew() {
	m.pendingDelete = false
	m.clearScope()
	m.spanSet = false
	var start, end time.Time
	var err error
	if m.visual {
		start, end = m.selectionSpan()
		m.spanSet = true
	} else {
		start, end, err = blockWindowFrom(m.gridCursor(), m.itemsOn(m.day), 0)
		if err != nil {
			m.fail(err)
			return
		}
	}
	m.composeText = ""
	m.composeCursor = 0
	m.formStart = start.Format("15:04")
	m.formEnd = end.Format("15:04")
	m.formURL = ""
	m.formURLCursor = 0
	m.formRepeat = 0
	m.formApply = 0
	m.formDayMask = 0
	m.formDayCursor = 0
	m.formHasApply = false
	m.formEditing = false
	m.formField = formTitle
	m.startForm()
}

func (m *calModel) beginRename() {
	m.beginEdit(formTitle)
}

func (m *calModel) beginReschedule() {
	m.beginEdit(formStart)
}

func (m *calModel) beginEdit(focus int) {
	item, ok := m.selectedItem()
	if !ok {
		m.pendingDelete = false
		m.status = "No block selected to edit."
		return
	}
	m.pendingDelete = false
	m.capture(item)
	m.composeText = item.title
	m.formStart = item.start.Format("15:04")
	m.formEnd = item.end.Format("15:04")
	m.formURL = item.url
	m.formURLCursor = len([]rune(item.url))
	m.formHasApply = item.repeatID != 0
	m.formApply = 0
	m.formEditing = true
	m.formRepeat = 0
	m.formDayMask = 0
	m.formDayCursor = 0
	freq := item.freq
	if model.IsDayFreq(freq) {
		freq = model.FreqDays
		m.formDayMask = model.DayMask(item.freq)
		for index := 0; index < model.WeekdayCount(); index++ {
			if m.formDayMask&(1<<index) != 0 {
				m.formDayCursor = index
				break
			}
		}
	}
	if freq == "" {
		freq = "none"
	}
	for index, choice := range m.repeatChoices() {
		if choice == freq {
			m.formRepeat = index
			break
		}
	}
	m.formField = focus
	m.startForm()
}

func (m *calModel) startForm() {
	m.composing = true
	m.composeKind = "form"
	m.errText = ""
	m.status = ""
	m.placeFormCursor()
}

func (m *calModel) handleFormKey(key string) {
	if key == "enter" {
		m.commitForm()
		return
	}
	if key == "tab" || key == "down" {
		m.moveForm(1)
		return
	}
	if key == "shift+tab" || key == "up" {
		m.moveForm(-1)
		return
	}
	if m.formField == formDays {
		switch key {
		case "left", "h":
			m.cycleForm(-1)
		case "right", "l":
			m.cycleForm(1)
		case " ":
			m.formDayMask ^= 1 << m.formDayCursor
		}
		return
	}
	if m.formField == formRepeat || m.formField == formApply {
		switch key {
		case "left", "h":
			m.cycleForm(-1)
		case "right", "l", " ":
			m.cycleForm(1)
		}
		return
	}
	text, cursor := m.formText()
	text, cursor = editFormLine(text, cursor, key)
	m.setFormText(text, cursor)
}

func (m calModel) formFields() []int {
	fields := []int{formTitle, formStart, formEnd, formURL, formRepeat}
	if m.repeatLabel() == model.FreqDays {
		fields = append(fields, formDays)
	}
	if m.formHasApply {
		fields = append(fields, formApply)
	}
	return fields
}

func (m *calModel) moveForm(delta int) {
	fields := m.formFields()
	index := 0
	found := false
	for i, field := range fields {
		if field == m.formField {
			index = i
			found = true
			break
		}
	}
	if !found {
		m.formField = formRepeat
		m.placeFormCursor()
		return
	}
	count := len(fields)
	index = (index + delta%count + count) % count
	m.formField = fields[index]
	m.placeFormCursor()
}

func (m *calModel) placeFormCursor() {
	switch m.formField {
	case formStart:
		m.formStartCursor = len([]rune(m.formStart))
	case formEnd:
		m.formEndCursor = len([]rune(m.formEnd))
	case formURL:
		m.formURLCursor = len([]rune(m.formURL))
	case formTitle:
		m.composeCursor = len([]rune(m.composeText))
	}
}

func (m calModel) formText() (string, int) {
	switch m.formField {
	case formStart:
		return m.formStart, m.formStartCursor
	case formEnd:
		return m.formEnd, m.formEndCursor
	case formURL:
		return m.formURL, m.formURLCursor
	default:
		return m.composeText, m.composeCursor
	}
}

func (m *calModel) setFormText(text string, cursor int) {
	switch m.formField {
	case formStart:
		m.formStart = text
		m.formStartCursor = cursor
	case formEnd:
		m.formEnd = text
		m.formEndCursor = cursor
	case formURL:
		m.formURL = text
		m.formURLCursor = cursor
	default:
		m.composeText = text
		m.composeCursor = cursor
	}
}

func editFormLine(text string, cursor int, key string) (string, int) {
	text, cursor, _ = lineedit.Key(text, cursor, key)
	return text, cursor
}

func (m *calModel) pasteIntoForm(runes []rune) {
	if m.formField == formRepeat || m.formField == formDays || m.formField == formApply {
		return
	}
	text := pastedLine(runes)
	if text == "" {
		return
	}
	current, cursor := m.formText()
	m.setFormText(lineedit.Insert(current, cursor, text))
}

func pastedLine(runes []rune) string {
	raw := strings.TrimRight(string(runes), "\r\n")
	return strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t':
			return ' '
		default:
			if r < 32 {
				return -1
			}
			return r
		}
	}, raw)
}

func (m *calModel) cycleForm(delta int) {
	if m.formField == formRepeat {
		count := len(m.repeatChoices())
		m.formRepeat = (m.formRepeat + delta%count + count) % count
		if m.repeatLabel() == model.FreqDays {
			m.seedDays()
		}
		return
	}
	if m.formField == formDays {
		count := model.WeekdayCount()
		m.formDayCursor = (m.formDayCursor + delta%count + count) % count
		return
	}
	if m.formField == formApply {
		m.formApply = (m.formApply + delta%3 + 3) % 3
	}
}

func (m *calModel) seedDays() {
	if m.formDayMask != 0 {
		return
	}
	index := model.WeekdayIndex(m.day.Weekday())
	m.formDayMask = 1 << index
	m.formDayCursor = index
}

func (m calModel) repeatChoices() []string {
	if m.formEditing && m.targetRepeatID != 0 {
		return []string{model.FreqDaily, model.FreqWeekdays, model.FreqWeekly, model.FreqDays}
	}
	return []string{"none", model.FreqDaily, model.FreqWeekdays, model.FreqWeekly, model.FreqDays}
}

func (m calModel) repeatLabel() string {
	choices := m.repeatChoices()
	if m.formRepeat < 0 || m.formRepeat >= len(choices) {
		return choices[0]
	}
	return choices[m.formRepeat]
}

func (m calModel) chosenFreq() (string, error) {
	label := m.repeatLabel()
	if label == "none" {
		return "", nil
	}
	if label != model.FreqDays {
		return label, nil
	}
	freq := model.FreqFromMask(m.formDayMask)
	if freq == "" {
		return "", fmt.Errorf("a repeat needs a day")
	}
	return freq, nil
}

func (m calModel) applyLabel() string {
	labels := []string{"this event", "this and following", "all events"}
	if m.formApply < 0 || m.formApply >= len(labels) {
		return labels[0]
	}
	return labels[m.formApply]
}

func (m calModel) renderForm() string {
	lines := []string{
		m.formLine("Title", m.formValue(formTitle)),
		m.formLine("Start", m.formValue(formStart)),
		m.formLine("End", m.formValue(formEnd)),
		m.formLine("URL", m.formValue(formURL)),
		m.formLine("Repeat", m.formValue(formRepeat)),
	}
	if m.repeatLabel() == model.FreqDays {
		lines = append(lines, m.formLine("Days", m.daysValue()))
	}
	if m.formHasApply {
		lines = append(lines, m.formLine("Applies", m.formValue(formApply)))
	}
	lines = append(lines, "")
	hint := "tab moves    h/l changes    enter saves    esc cancels"
	if m.formField == formDays {
		hint = "tab moves    h/l moves    space toggles    enter saves    esc cancels"
	}
	lines = append(lines, mutedStyle.Render(hint))
	heading := "Block"
	if m.formEditing {
		heading = "Edit block"
	}
	body := sectionStyle.Render(heading) + "\n" + strings.Join(lines, "\n")
	return helpPanelStyle.Render(body)
}

func (m calModel) formLine(label, value string) string {
	return fmt.Sprintf("%-8s %s", label, value)
}

func (m calModel) formValue(field int) string {
	switch field {
	case formTitle:
		if m.formField == formTitle {
			return composeShown(m.composeText, m.composeCursor)
		}
		return m.composeText
	case formStart:
		if m.formField == formStart {
			return composeShown(m.formStart, m.formStartCursor)
		}
		return m.formStart
	case formEnd:
		if m.formField == formEnd {
			return composeShown(m.formEnd, m.formEndCursor)
		}
		return m.formEnd
	case formURL:
		if m.formField == formURL {
			return composeShown(m.formURL, m.formURLCursor)
		}
		return m.formURL
	case formRepeat:
		return m.choiceShown(m.repeatLabel(), m.formField == formRepeat)
	case formDays:
		return m.daysValue()
	default:
		return m.choiceShown(m.applyLabel(), m.formField == formApply)
	}
}

func (m calModel) daysValue() string {
	var b strings.Builder
	for index := 0; index < model.WeekdayCount(); index++ {
		if index > 0 {
			b.WriteByte(' ')
		}
		name := model.WeekdayName(index)
		if m.formDayMask&(1<<index) == 0 {
			name = strings.ToLower(name)
		}
		if m.formField == formDays && index == m.formDayCursor {
			name = activeCellStyle.Render(name)
		}
		b.WriteString(name)
	}
	return b.String()
}

func (m calModel) choiceShown(label string, active bool) string {
	if active {
		return activeCellStyle.Render(label)
	}
	return label
}

func (m *calModel) commitForm() {
	form, err := m.collectForm()
	if err != nil {
		m.fail(err)
		return
	}
	target := store.FormTarget{Day: m.day}
	if m.formEditing {
		target = store.FormTarget{BlockID: m.targetBlockID, RepeatID: m.targetRepeatID, Day: m.targetDay}
	}
	saved, err := store.SaveForm(m.dataPath, target, form)
	m.finish(savedStatus(saved, target, form.Scope), err, calPick{blockID: saved.BlockID, repeatID: saved.RepeatID})
}

// collectForm reads what the form holds. The store decides what it changes.
func (m calModel) collectForm() (model.CalForm, error) {
	title := strings.TrimSpace(m.composeText)
	if title == "" {
		return model.CalForm{}, fmt.Errorf("a block needs a title")
	}
	startHour, startMinute, startOK := parseClock(strings.TrimSpace(m.formStart))
	endHour, endMinute, endOK := parseClock(strings.TrimSpace(m.formEnd))
	if !startOK || !endOK {
		return model.CalForm{}, fmt.Errorf("a block needs a start and an end")
	}
	freq, err := m.chosenFreq()
	if err != nil {
		return model.CalForm{}, err
	}
	return model.CalForm{
		Title:       title,
		StartHour:   startHour,
		StartMinute: startMinute,
		EndHour:     endHour,
		EndMinute:   endMinute,
		Freq:        freq,
		URL:         strings.TrimSpace(m.formURL),
		Scope:       model.Scope(m.formApply),
	}, nil
}

// savedStatus is the status line of a form save.
func savedStatus(saved store.Saved, target store.FormTarget, scope model.Scope) string {
	switch saved.Change {
	case model.ChangeBlocked:
		return "Blocked " + saved.Title + "."
	case model.ChangeRepeats:
		return fmt.Sprintf("Repeats %s %s.", saved.Title, model.FreqLabel(saved.Freq))
	}
	if target.RepeatID == 0 {
		return map[model.CalChange]string{model.ChangeSaved: "Saved ", model.ChangeRenamed: "Renamed ", model.ChangeRescheduled: "Rescheduled "}[saved.Change] + saved.Title + "."
	}
	if saved.Change == model.ChangeLink {
		return "Saved the meeting link."
	}
	verb := map[model.CalChange]string{model.ChangeSaved: "Saved", model.ChangeRenamed: "Renamed", model.ChangeRescheduled: "Rescheduled"}[saved.Change]
	return verb + " " + []string{"this event", "this and following", "all events"}[scope] + "."
}
