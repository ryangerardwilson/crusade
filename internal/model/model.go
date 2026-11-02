package model

import (
	"encoding/json"
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
	ModeNLP   = "nlp"
)

type Event struct {
	Mode string
	X    time.Time
	Y    string
	Z    string
	P    float64
	Q    float64
	R    float64
	Done bool
}

type JSONEvent struct {
	JTBD struct {
		X string `json:"x"`
		Y string `json:"y"`
		Z string `json:"z"`
	} `json:"jtbd"`
	NSM struct {
		P any `json:"p"`
		Q any `json:"q"`
		R any `json:"r"`
	} `json:"nsm"`
}

func DefaultEvent(now time.Time) Event {
	return Event{
		X: time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location()),
		P: 0,
		Q: 0,
		R: 0,
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

func ParseMetric(raw string, label string) (float64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, fmt.Errorf("Metric '%s' cannot be empty", label)
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("Metric '%s' must be numeric", label)
	}
	return parsed, nil
}

func NewEvent(x, y, z, p, q, r string) (Event, error) {
	parsedX, err := ParseDateTime(x)
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
	parsedP, err := ParseMetric(p, "p")
	if err != nil {
		return Event{}, err
	}
	parsedQ, err := ParseMetric(q, "q")
	if err != nil {
		return Event{}, err
	}
	parsedR, err := ParseMetric(r, "r")
	if err != nil {
		return Event{}, err
	}
	return Event{Mode: ModeAlign, X: parsedX, Y: outcome, Z: impact, P: parsedP, Q: parsedQ, R: parsedR}, nil
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

func (e Event) IsDo() bool {
	return e.Mode == ModeDo
}

func (e Event) WithUpdates(fields map[string]string) (Event, error) {
	if e.IsDo() {
		return Event{}, fmt.Errorf("do tasks store an outcome only")
	}
	next := e
	if value, ok := fields["x"]; ok {
		parsed, err := ParseDateTime(value)
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
	if value, ok := fields["p"]; ok {
		parsed, err := ParseMetric(value, "p")
		if err != nil {
			return Event{}, err
		}
		next.P = parsed
	}
	if value, ok := fields["q"]; ok {
		parsed, err := ParseMetric(value, "q")
		if err != nil {
			return Event{}, err
		}
		next.Q = parsed
	}
	if value, ok := fields["r"]; ok {
		parsed, err := ParseMetric(value, "r")
		if err != nil {
			return Event{}, err
		}
		next.R = parsed
	}
	updated, err := NewEvent(next.X.Format(DateTimeFormat), next.Y, next.Z, FormatMetric(next.P), FormatMetric(next.Q), FormatMetric(next.R))
	if err != nil {
		return Event{}, err
	}
	updated.Done = e.Done
	return updated, nil
}

func ToJSONEvent(event Event) JSONEvent {
	var payload JSONEvent
	payload.JTBD.X = event.X.Format(DateTimeFormat)
	payload.JTBD.Y = event.Y
	payload.JTBD.Z = event.Z
	payload.NSM.P = event.P
	payload.NSM.Q = event.Q
	payload.NSM.R = event.R
	return payload
}

func FromJSON(data []byte) (Event, error) {
	var payload JSONEvent
	if err := json.Unmarshal(data, &payload); err != nil {
		return Event{}, fmt.Errorf("Invalid JSON: %w", err)
	}
	return NewEvent(
		payload.JTBD.X,
		payload.JTBD.Y,
		payload.JTBD.Z,
		metricToString(payload.NSM.P),
		metricToString(payload.NSM.Q),
		metricToString(payload.NSM.R),
	)
}

func FormatMetric(value float64) string {
	text := strconv.FormatFloat(value, 'f', 2, 64)
	text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	if text == "" || text == "-0" {
		return "0"
	}
	return text
}

func Less(a, b Event) bool {
	if !a.X.Equal(b.X) {
		return a.X.Before(b.X)
	}
	if a.Y != b.Y {
		return a.Y < b.Y
	}
	if a.Z != b.Z {
		return a.Z < b.Z
	}
	if a.P != b.P {
		return a.P < b.P
	}
	if a.Q != b.Q {
		return a.Q < b.Q
	}
	return a.R < b.R
}

func metricToString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	default:
		return fmt.Sprint(v)
	}
}
