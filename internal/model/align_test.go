package model

import (
	"errors"
	"strings"
	"testing"
)

func TestAlignYAMLRoundTrip(t *testing.T) {
	event, err := NewEvent("2026-01-26 18:30:00", "ship the patch", "reduce drift")
	if err != nil {
		t.Fatal(err)
	}
	if got := event.X.Format("2006-01-02 15:04:05"); got != "2026-01-26 00:00:00" {
		t.Fatalf("x = %s", got)
	}
	raw, err := ToYAML(event)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if anyOf(!strings.Contains(text, "x: 2026-01-26"), strings.Contains(text, "nsm"), strings.Contains(text, "18:30"), strings.Contains(text, "{"), strings.Contains(text, `"`), strings.Contains(text, "'")) {

		t.Fatalf("yaml:\n%s", text)
	}
	got, err := FromYAML(raw)
	if err != nil {
		t.Fatal(err)
	}
	if anyOf(got.Y != event.Y, got.Z != event.Z, !got.X.Equal(event.X), got.Mode != ModeAlign) {

		t.Fatalf("got = %#v", got)
	}
	plain, err := FromYAML([]byte("x: 2026-04-02\ny: wrote the note\nz: keep the trail\n"))
	if err != nil {
		t.Fatal(err)
	}
	if anyOf(plain.X.Format("2006-01-02") != "2026-04-02", plain.Y != "wrote the note") {

		t.Fatalf("plain = %#v", plain)
	}
	draft, err := ToYAML(Event{X: event.X})
	if err != nil {
		t.Fatal(err)
	}
	draftText := string(draft)
	if anyOf(strings.Contains(draftText, `"`), strings.Contains(draftText, "'"), !strings.Contains(draftText, "x: 2026-01-26\n"), !strings.Contains(draftText, "y:\n"), !strings.Contains(draftText, "z:\n")) {

		t.Fatalf("draft yaml:\n%s", draftText)
	}
	if _, err := FromYAML(draft); anyOf(err == nil, !errors.Is(err, ErrBlankAlign)) {

		t.Fatalf("blank draft err = %v", err)
	}
	quoted, err := FromYAML([]byte("x: \"2026-11-07\"\ny: 'ship the patch'\nz: \"reduce drift\"\n"))
	if anyOf(err != nil, quoted.Y != "ship the patch", quoted.Z != "reduce drift", quoted.X.Format("2006-01-02") != "2026-11-07") {

		t.Fatalf("quoted %#v err=%v", quoted, err)
	}
	if _, err := FromYAML([]byte("x: 2026-04-02\ny: wrote the note\nz: keep the trail\nnsm:\n  p: 1\n")); anyOf(err == nil, !strings.Contains(err.Error(), "Invalid YAML")) {

		t.Fatalf("nsm err = %v", err)
	}
	if _, err := NewEvent("tomorrow", "outcome", "impact"); err == nil {
		t.Fatal("bad date accepted")
	}
}

func anyOf(flags ...bool) bool {
	for _, flag := range flags {
		if flag {
			return true
		}
	}
	return false
}

func allOf(flags ...bool) bool {
	for _, flag := range flags {
		if !flag {
			return false
		}
	}
	return true
}
