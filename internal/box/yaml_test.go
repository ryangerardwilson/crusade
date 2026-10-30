package box

import (
	"strings"
	"testing"
)

func TestYAMLRoundTripPreservesLabelsAndIDs(t *testing.T) {
	outline, err := ParseOutline("Move\nWork\n  Email\n  Deep work\nPlan tomorrow\n")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := MarshalYAML(outline)
	if err != nil {
		t.Fatal(err)
	}
	again, err := ParseYAML(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := outline.Flatten()
	got := again.Flatten()
	if len(got) != len(want) {
		t.Fatalf("count = %d", len(got))
	}
	for index := range want {
		if got[index].ID != want[index].ID || got[index].Label != want[index].Label || got[index].Depth != want[index].Depth {
			t.Fatalf("item %d = %#v want %#v", index, got[index], want[index])
		}
	}
	if !strings.Contains(string(raw), "label: Email") || !strings.Contains(string(raw), "version: 1") {
		t.Fatalf("yaml = %s", raw)
	}
}

func TestParseYAMLRejectsUnknownFieldsDuplicateIDsAndBadVersion(t *testing.T) {
	if _, err := ParseYAML([]byte("version: 1\nboxes:\n  - label: Work\n    surprise: true\n")); err == nil || !strings.Contains(err.Error(), "surprise") {
		t.Fatalf("unknown field error = %v", err)
	}
	if _, err := ParseYAML([]byte("version: 1\nboxes:\n  - label: Work\n  - label: Work\n")); err == nil || !strings.Contains(err.Error(), "duplicate item id") {
		t.Fatalf("duplicate error = %v", err)
	}
	if _, err := ParseYAML([]byte("version: 2\nboxes: []\n")); err == nil || !strings.Contains(err.Error(), "unsupported YAML version") {
		t.Fatalf("version error = %v", err)
	}
	empty, err := MarshalYAML(Outline{})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseYAML(empty)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Items) != 0 {
		t.Fatalf("empty items = %#v", parsed.Items)
	}
}
