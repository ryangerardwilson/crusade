package model

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const DateTimeFormat = "2006-01-02 15:04:05"

const (
	ModeAlign = "align"
	ModeDo    = "do"
	ModeBox   = "box"
	ModeCal   = "cal"
	ModeNLP   = "nlp"
)

type Event struct {
	Mode   string
	X      time.Time
	Y      string
	Z      string
	Done   bool
	DoneAt time.Time
}

func DefaultEvent(now time.Time) Event {
	return Event{
		Mode: ModeAlign,
		X:    time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()),
	}
}

func ParseDateTime(raw string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, fmt.Errorf("Invalid datetime format: ''. Expected YYYY-MM-DD HH:MM[:SS]")
	}
	candidate := strings.TrimSuffix(value, "Z")
	candidate = strings.Replace(candidate, "T", " ", 1)
	if t, err := time.ParseInLocation(DateTimeFormat, candidate, time.Local); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04", candidate, time.Local); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.Local(), nil
	}
	return time.Time{}, fmt.Errorf("Invalid datetime format: '%s'. Expected YYYY-MM-DD HH:MM[:SS]", value)
}

func NewEvent(x, y, z string) (Event, error) {
	parsedX, err := ParseAlignDate(x)
	if err != nil {
		return Event{}, err
	}
	outcome := strings.TrimSpace(y)
	if outcome == "" {
		return Event{}, fmt.Errorf("'y' (outcome) cannot be empty")
	}
	impact := strings.TrimSpace(z)
	if impact == "" {
		return Event{}, fmt.Errorf("'z' (impact) cannot be empty")
	}
	return Event{Mode: ModeAlign, X: parsedX, Y: outcome, Z: impact}, nil
}

// ParseAlignDate reads the day an align job happens. A clock time is ignored.
func ParseAlignDate(raw string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if len(value) >= 10 {
		day, err := time.ParseInLocation("2006-01-02", value[:10], time.Local)
		if err == nil && value[:10] == day.Format("2006-01-02") {
			rest := strings.TrimSpace(value[10:])
			if rest == "" {
				return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.Local), nil
			}
			if _, err := ParseDateTime(value); err == nil {
				return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.Local), nil
			}
		}
	}
	return time.Time{}, fmt.Errorf("x needs a date in YYYY-MM-DD")
}

func NewDoEvent(when, outcome string) (Event, error) {
	parsedWhen, err := ParseDateTime(when)
	if err != nil {
		return Event{}, err
	}
	text := strings.TrimSpace(outcome)
	if text == "" {
		return Event{}, fmt.Errorf("outcome cannot be empty")
	}
	return Event{Mode: ModeDo, X: parsedWhen, Y: text}, nil
}

// Block is a span of clock time on one day. It is not a do task.
// URL is an optional meeting link.
type Block struct {
	Title string
	Start time.Time
	End   time.Time
	URL   string
}

func NewBlock(title, start, end string) (Block, error) {
	name := strings.TrimSpace(title)
	if name == "" {
		return Block{}, fmt.Errorf("a block needs a title")
	}
	parsedStart, err := ParseDateTime(start)
	if err != nil {
		return Block{}, err
	}
	parsedEnd, err := ParseDateTime(end)
	if err != nil {
		return Block{}, err
	}
	if !parsedEnd.After(parsedStart) {
		return Block{}, fmt.Errorf("a block ends after it starts")
	}
	if !sameDayBlock(parsedStart, parsedEnd) {
		return Block{}, fmt.Errorf("a block stays on one day")
	}
	return Block{Title: name, Start: parsedStart, End: parsedEnd}, nil
}

func sameDayBlock(start, end time.Time) bool {
	sy, sm, sd := start.Date()
	ey, em, ed := end.Date()
	if sy == ey && sm == em && sd == ed {
		return true
	}
	midnight := time.Date(sy, sm, sd, 0, 0, 0, 0, start.Location()).AddDate(0, 0, 1)
	return end.Equal(midnight)
}

func (block Block) OnTheHour() error {
	if block.Start.Minute() != 0 || block.Start.Second() != 0 || block.End.Minute() != 0 || block.End.Second() != 0 {
		return fmt.Errorf("a block starts and ends on the hour")
	}
	return nil
}

func (repeat Repeat) OnTheHour() error {
	return Block{Start: repeat.Start, End: repeat.End}.OnTheHour()
}

const (
	FreqDaily    = "daily"
	FreqWeekdays = "weekdays"
	FreqWeekly   = "weekly"
	FreqDays     = "days"
)

var repeatWeek = []struct {
	key   string
	label string
}{
	{"mon", "Mon"},
	{"tue", "Tue"},
	{"wed", "Wed"},
	{"thu", "Thu"},
	{"fri", "Fri"},
	{"sat", "Sat"},
	{"sun", "Sun"},
}

// Repeat is one series of time blocks. Until is the last day, or zero when the series stays open.
// URL is an optional meeting link.
type Repeat struct {
	Title string
	Start time.Time
	End   time.Time
	Freq  string
	Until time.Time
	URL   string
}

func ParseFreq(raw string) (string, bool) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch raw {
	case FreqDaily, FreqWeekdays, FreqWeekly:
		return raw, true
	}
	if raw == "" {
		return "", false
	}
	seen := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		known := false
		for _, day := range repeatWeek {
			if part == day.key {
				known = true
				break
			}
		}
		if !known || seen[part] {
			return "", false
		}
		seen[part] = true
	}
	parts := make([]string, 0, len(seen))
	for _, day := range repeatWeek {
		if seen[day.key] {
			parts = append(parts, day.key)
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, ","), true
}

func IsDayFreq(freq string) bool {
	switch freq {
	case "", FreqDaily, FreqWeekdays, FreqWeekly, FreqDays:
		return false
	default:
		_, ok := ParseFreq(freq)
		return ok
	}
}

func DayMask(freq string) uint8 {
	parsed, ok := ParseFreq(freq)
	if !ok || !IsDayFreq(parsed) {
		return 0
	}
	var mask uint8
	for _, part := range strings.Split(parsed, ",") {
		for index, day := range repeatWeek {
			if part == day.key {
				mask |= 1 << index
			}
		}
	}
	return mask
}

func FreqFromMask(mask uint8) string {
	parts := make([]string, 0, len(repeatWeek))
	for index, day := range repeatWeek {
		if mask&(1<<index) != 0 {
			parts = append(parts, day.key)
		}
	}
	return strings.Join(parts, ",")
}

func FreqLabel(freq string) string {
	parsed, ok := ParseFreq(freq)
	if !ok || !IsDayFreq(parsed) {
		return freq
	}
	labels := make([]string, 0, len(repeatWeek))
	for _, part := range strings.Split(parsed, ",") {
		for _, day := range repeatWeek {
			if part == day.key {
				labels = append(labels, day.label)
			}
		}
	}
	return strings.Join(labels, " ")
}

func WeekdayCount() int {
	return len(repeatWeek)
}

func WeekdayIndex(day time.Weekday) int {
	return (int(day) + 6) % 7
}

func WeekdayName(index int) string {
	if index < 0 || index >= len(repeatWeek) {
		return ""
	}
	return repeatWeek[index].label
}

func NewRepeat(title, start, end, freq string) (Repeat, error) {
	parsed, ok := ParseFreq(freq)
	if !ok {
		return Repeat{}, fmt.Errorf("a repeat is daily, weekdays, weekly, or chosen weekdays")
	}
	block, err := NewBlock(title, start, end)
	if err != nil {
		return Repeat{}, err
	}
	repeat := Repeat{Title: block.Title, Start: block.Start, End: block.End, Freq: parsed}
	if !repeat.OccursOn(repeat.Start) {
		return Repeat{}, fmt.Errorf("that repeat does not fall on this day")
	}
	return repeat, nil
}

func (repeat Repeat) OccursOn(day time.Time) bool {
	day = calendarDay(day)
	start := calendarDay(repeat.Start)
	if day.Before(start) {
		return false
	}
	if !repeat.Until.IsZero() && day.After(calendarDay(repeat.Until)) {
		return false
	}
	if IsDayFreq(repeat.Freq) {
		index := WeekdayIndex(day.Weekday())
		return DayMask(repeat.Freq)&(1<<index) != 0
	}
	switch repeat.Freq {
	case FreqDaily:
		return true
	case FreqWeekdays:
		weekday := day.Weekday()
		return weekday >= time.Monday && weekday <= time.Friday
	case FreqWeekly:
		return day.Weekday() == start.Weekday()
	default:
		return false
	}
}

func (repeat Repeat) SpanOn(day time.Time) (time.Time, time.Time) {
	day = calendarDay(day)
	start := time.Date(day.Year(), day.Month(), day.Day(), repeat.Start.Hour(), repeat.Start.Minute(), repeat.Start.Second(), 0, time.Local)
	end := time.Date(day.Year(), day.Month(), day.Day(), repeat.End.Hour(), repeat.End.Minute(), repeat.End.Second(), 0, time.Local)
	if !end.After(start) {
		end = end.AddDate(0, 0, 1)
	}
	return start, end
}

func calendarDay(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.Local)
}

func (e Event) IsDo() bool {
	return e.Mode == ModeDo
}

func (e Event) WithUpdates(fields map[string]string) (Event, error) {
	if e.IsDo() {
		return Event{}, fmt.Errorf("do tasks store an outcome only")
	}
	next := e
	if value, ok := fields["x"]; ok {
		parsed, err := ParseAlignDate(value)
		if err != nil {
			return Event{}, err
		}
		next.X = parsed
	}
	if value, ok := fields["y"]; ok {
		next.Y = strings.TrimSpace(value)
	}
	if value, ok := fields["z"]; ok {
		next.Z = strings.TrimSpace(value)
	}
	updated, err := NewEvent(next.X.Format("2006-01-02"), next.Y, next.Z)
	if err != nil {
		return Event{}, err
	}
	updated.Done = e.Done
	updated.DoneAt = e.DoneAt
	return updated, nil
}

func ToYAML(event Event) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("# Job to be done\n")
	buf.WriteString("# x: when this happens (YYYY-MM-DD)\n")
	buf.WriteString("# y: the outcome\n")
	buf.WriteString("# z: why it matters\n")
	writeAlignField(&buf, "x", event.X.Format("2006-01-02"))
	writeAlignField(&buf, "y", event.Y)
	writeAlignField(&buf, "z", event.Z)
	return buf.Bytes(), nil
}

func writeAlignField(buf *bytes.Buffer, key, value string) {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	lines := strings.Split(value, "\n")
	if len(lines) == 1 && lines[0] == "" {
		fmt.Fprintf(buf, "%s:\n", key)
		return
	}
	fmt.Fprintf(buf, "%s: %s\n", key, lines[0])
	for _, line := range lines[1:] {
		fmt.Fprintf(buf, "  %s\n", line)
	}
}

func FromYAML(data []byte) (Event, error) {
	fields := map[string]string{}
	current := ""
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "---" {
			if len(fields) > 0 {
				return Event{}, fmt.Errorf("Invalid YAML: one record only")
			}
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if current == "" {
				return Event{}, fmt.Errorf("Invalid YAML: expected x, y, and z")
			}
			fields[current] += "\n" + strings.TrimLeft(line, " \t")
			continue
		}
		key, value, ok := splitAlignField(line)
		if !ok {
			return Event{}, fmt.Errorf("Invalid YAML: expected x, y, and z")
		}
		key = plainScalar(key)
		if key != "x" && key != "y" && key != "z" {
			return Event{}, fmt.Errorf("Invalid YAML: unknown field %s", key)
		}
		if _, exists := fields[key]; exists {
			return Event{}, fmt.Errorf("Invalid YAML: duplicate field %s", key)
		}
		fields[key] = plainScalar(value)
		current = key
	}
	if strings.TrimSpace(fields["y"]) == "" && strings.TrimSpace(fields["z"]) == "" {
		return Event{}, ErrBlankAlign
	}
	return NewEvent(fields["x"], fields["y"], fields["z"])
}

// ErrBlankAlign means the job file still has empty y and z.
var ErrBlankAlign = errors.New("empty align draft")

func splitAlignField(line string) (string, string, bool) {
	idx := strings.Index(line, ":")
	if idx <= 0 {
		return "", "", false
	}
	value := ""
	if idx+1 < len(line) {
		value = strings.TrimPrefix(line[idx+1:], " ")
	}
	return strings.TrimSpace(line[:idx]), value, true
}

func plainScalar(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < 2 {
		return value
	}
	switch value[0] {
	case '"':
		if value[len(value)-1] == '"' {
			if unquoted, err := strconv.Unquote(value); err == nil {
				return unquoted
			}
		}
	case '\'':
		if value[len(value)-1] == '\'' {
			return strings.ReplaceAll(value[1:len(value)-1], "''", "'")
		}
	}
	return value
}

func Less(a, b Event) bool {
	if !a.X.Equal(b.X) {
		return a.X.Before(b.X)
	}
	if a.Y != b.Y {
		return a.Y < b.Y
	}
	return a.Z < b.Z
}
