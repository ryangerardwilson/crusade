package tui

import (
	"fmt"
	"strings"
	"time"

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
	m.formStart = snapDown(item.start).Format("15:04")
	m.formEnd = snapUp(item.end).Format("15:04")
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
	scratch := appModel{composeText: text, composeCursor: cursor}
	scratch.handleComposeKey(key)
	return scratch.composeText, scratch.composeCursor
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
	scratch := appModel{composeText: current, composeCursor: cursor}
	scratch.insertCompose(text)
	m.setFormText(scratch.composeText, scratch.composeCursor)
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
	title := strings.TrimSpace(m.composeText)
	if title == "" {
		m.fail(fmt.Errorf("a block needs a title"))
		return
	}
	start, end, err := m.formTimes()
	if err != nil {
		m.fail(err)
		return
	}
	freq, freqErr := m.chosenFreq()
	if freqErr != nil {
		m.fail(freqErr)
		return
	}
	url := strings.TrimSpace(m.formURL)
	if !m.formEditing {
		m.commitNewForm(title, start, end, freq, url)
		return
	}
	if m.targetRepeatID == 0 {
		m.commitOneOffForm(title, start, end, freq, url)
		return
	}
	m.commitRepeatForm(title, start, end, freq, url)
}

func (m calModel) formTimes() (time.Time, time.Time, error) {
	day := m.day
	if m.formEditing {
		day = m.targetDay
	}
	startHour, startMinute, startOK := parseClock(strings.TrimSpace(m.formStart))
	endHour, endMinute, endOK := parseClock(strings.TrimSpace(m.formEnd))
	if !startOK || !endOK {
		return time.Time{}, time.Time{}, fmt.Errorf("a block needs a start and an end")
	}
	if startMinute != 0 || endMinute != 0 {
		return time.Time{}, time.Time{}, fmt.Errorf("a block starts and ends on the hour")
	}
	start := clockOn(day, startHour, startMinute)
	end := clockOn(day, endHour, endMinute)
	if !end.After(start) && endHour == 0 {
		end = end.AddDate(0, 0, 1)
	}
	return start, end, nil
}

func (m *calModel) commitNewForm(title string, start, end time.Time, freq, url string) {
	if freq == "" {
		block, err := model.NewBlock(title, start.Format(model.DateTimeFormat), end.Format(model.DateTimeFormat))
		if err != nil {
			m.fail(err)
			return
		}
		block.URL = url
		id, err := store.AddBlock(m.dataPath, block)
		m.finish("Blocked "+block.Title+".", err, calPick{blockID: id})
		return
	}
	repeat, err := model.NewRepeat(title, start.Format(model.DateTimeFormat), end.Format(model.DateTimeFormat), freq)
	if err != nil {
		m.fail(err)
		return
	}
	repeat.URL = url
	id, err := store.AddRepeat(m.dataPath, repeat)
	m.finish(fmt.Sprintf("Repeats %s %s.", repeat.Title, model.FreqLabel(repeat.Freq)), err, calPick{repeatID: id})
}

func (m *calModel) commitOneOffForm(title string, start, end time.Time, freq, url string) {
	if freq != "" {
		repeat, err := model.NewRepeat(title, start.Format(model.DateTimeFormat), end.Format(model.DateTimeFormat), freq)
		if err != nil {
			m.fail(err)
			return
		}
		repeat.URL = url
		id, err := store.MakeRepeatFromBlock(m.dataPath, m.targetBlockID, repeat)
		m.finish(fmt.Sprintf("Repeats %s %s.", repeat.Title, model.FreqLabel(repeat.Freq)), err, calPick{repeatID: id})
		return
	}
	block, err := model.NewBlock(title, start.Format(model.DateTimeFormat), end.Format(model.DateTimeFormat))
	if err != nil {
		m.fail(err)
		return
	}
	block.URL = url
	err = store.UpdateBlock(m.dataPath, m.targetBlockID, block)
	status := "Saved " + block.Title + "."
	if !start.Equal(m.targetStart) || !end.Equal(m.targetEnd) {
		status = "Rescheduled " + block.Title + "."
	} else if title != m.targetTitle {
		status = "Renamed " + block.Title + "."
	}
	m.finish(status, err, calPick{blockID: m.targetBlockID})
}

func (m *calModel) commitRepeatForm(title string, start, end time.Time, freq, url string) {
	choice := []string{"o", "f", "a"}[m.formApply]
	titleChanged := title != m.targetTitle
	timesChanged := !start.Equal(m.targetStart) || !end.Equal(m.targetEnd)
	freqChanged := freq != m.targetFreq
	urlChanged := url != m.targetURL
	if choice == "o" && freqChanged {
		m.fail(fmt.Errorf("a repeat change applies to this and following, or to all events"))
		return
	}
	switch choice {
	case "o":
		m.commitThisRepeat(title, start, end, url, titleChanged, timesChanged)
	case "f":
		m.commitFollowingRepeat(title, start, end, freq, url, titleChanged, timesChanged, freqChanged, urlChanged)
	default:
		m.commitAllRepeats(title, start, end, freq, url, titleChanged, timesChanged, freqChanged, urlChanged)
	}
}

func (m *calModel) commitThisRepeat(title string, start, end time.Time, url string, titleChanged, timesChanged bool) {
	err := store.OverrideRepeatDay(m.dataPath, m.targetRepeatID, m.targetDay, title, start, end, url)
	status := "Saved this event."
	if titleChanged && !timesChanged {
		status = "Renamed this event."
	} else if timesChanged && !titleChanged {
		status = "Rescheduled this event."
	}
	m.finish(status, err, calPick{repeatID: m.targetRepeatID})
}

func (m *calModel) commitFollowingRepeat(title string, start, end time.Time, freq, url string, titleChanged, timesChanged, freqChanged, urlChanged bool) {
	name := m.targetSeriesTitle
	if titleChanged {
		name = title
	}
	clockStart, clockEnd, useFreq := start, end, freq
	clear := timesChanged || freqChanged
	if !clear {
		clockStart = clockFrom(m.targetDay, m.targetSeriesStart)
		clockEnd = clockEndFrom(m.targetDay, m.targetSeriesEnd)
		useFreq = m.targetFreq
	}
	next, err := model.NewRepeat(name, clockStart.Format(model.DateTimeFormat), clockEnd.Format(model.DateTimeFormat), useFreq)
	if err != nil {
		m.fail(err)
		return
	}
	next.URL = m.seriesURL(urlChanged, url)
	id, err := store.ContinueRepeat(m.dataPath, m.targetRepeatID, m.targetDay, next, clear)
	if err != nil {
		m.fail(err)
		return
	}
	if !clear && sameDate(m.targetSeriesStart, m.targetDay) {
		if err := store.RenameRepeatOverrides(m.dataPath, id, next.Title); err != nil {
			m.fail(err)
			return
		}
		if urlChanged {
			if err := store.SetRepeatOverrideURLs(m.dataPath, id, next.URL); err != nil {
				m.fail(err)
				return
			}
		}
	}
	status := "Saved this and following."
	if titleChanged && !clear {
		status = "Renamed this and following."
	} else if !titleChanged && clear {
		status = "Rescheduled this and following."
	}
	m.finish(status, nil, calPick{repeatID: id})
}

func (m *calModel) commitAllRepeats(title string, start, end time.Time, freq, url string, titleChanged, timesChanged, freqChanged, urlChanged bool) {
	if !timesChanged && !freqChanged {
		next, err := model.NewRepeat(title, m.targetSeriesStart.Format(model.DateTimeFormat), m.targetSeriesEnd.Format(model.DateTimeFormat), m.targetFreq)
		if err != nil {
			m.fail(err)
			return
		}
		next.Until = m.targetUntil
		next.URL = m.seriesURL(urlChanged, url)
		if err := store.UpdateRepeat(m.dataPath, m.targetRepeatID, next); err != nil {
			m.fail(err)
			return
		}
		if err = store.RenameRepeatOverrides(m.dataPath, m.targetRepeatID, next.Title); err != nil {
			m.fail(err)
			return
		}
		if urlChanged {
			err = store.SetRepeatOverrideURLs(m.dataPath, m.targetRepeatID, next.URL)
		}
		status := "Saved all events."
		if titleChanged && !urlChanged {
			status = "Renamed all events."
		}
		m.finish(status, err, calPick{repeatID: m.targetRepeatID})
		return
	}
	name := m.targetSeriesTitle
	if titleChanged {
		name = title
	}
	anchorStart := clockFrom(m.targetSeriesStart, start)
	anchorEnd := clockEndFrom(m.targetSeriesStart, end)
	next, err := model.NewRepeat(name, anchorStart.Format(model.DateTimeFormat), anchorEnd.Format(model.DateTimeFormat), freq)
	if err != nil {
		m.fail(err)
		return
	}
	next.Until = m.targetUntil
	next.URL = m.seriesURL(urlChanged, url)
	err = store.ReplaceRepeat(m.dataPath, m.targetRepeatID, next)
	status := "Rescheduled all events."
	if titleChanged {
		status = "Saved all events."
	}
	m.finish(status, err, calPick{repeatID: m.targetRepeatID})
}

func (m calModel) seriesURL(urlChanged bool, url string) string {
	if urlChanged {
		return url
	}
	return m.targetSeriesURL
}
