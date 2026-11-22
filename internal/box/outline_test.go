package box

import (
	"reflect"
	"testing"
)

// stored is the outline Move, Work (Email, Deep work), Plan tomorrow with
// stored ids, as LoadOutline gives it.
func stored() Outline {
	return Outline{Items: []Item{
		{ID: 1, Label: "Move"},
		{ID: 2, Label: "Work", Children: []Item{{ID: 3, Label: "Email"}, {ID: 4, Label: "Deep work"}}},
		{ID: 5, Label: "Plan tomorrow"},
	}}
}

func TestParseOutlineNests(t *testing.T) {
	outline, err := ParseOutline("Move\nWork\n  Email\n  Deep work\nPlan tomorrow\n")
	if err != nil {
		t.Fatal(err)
	}
	flat := outline.Flatten()
	want := []FlatItem{
		{Label: "Move"},
		{Label: "Work", HasChildren: true},
		{Label: "Email", Depth: 1},
		{Label: "Deep work", Depth: 1},
		{Label: "Plan tomorrow"},
	}
	if !reflect.DeepEqual(flat, want) {
		t.Fatalf("flat = %#v", flat)
	}
	if outline.LeafCount() != 4 {
		t.Fatalf("leaves = %d", outline.LeafCount())
	}
	paths := []string{"move", "work", "work/email", "work/deep-work", "plan-tomorrow"}
	if got := outline.Paths(); !reflect.DeepEqual(got, paths) {
		t.Fatalf("paths = %v", got)
	}
	if _, err := ParseOutline("Work\n    Email\n"); err == nil {
		t.Fatal("indent jump accepted")
	}
}

func TestParseOutlineRejectsDuplicateLabels(t *testing.T) {
	if _, err := ParseOutline("Work\nWork\n"); err == nil {
		t.Fatal("expected duplicate label")
	}
	if _, err := ParseOutline("Work\n  Email\nHome\n  Email\n"); err != nil {
		t.Fatalf("same label under two parents: %v", err)
	}
}

func TestValidateRejectsDuplicateIDsAndBadLabels(t *testing.T) {
	if err := stored().Validate(); err != nil {
		t.Fatal(err)
	}
	dup := Outline{Items: []Item{{ID: 1, Label: "A"}, {ID: 2, Label: "B", Children: []Item{{ID: 1, Label: "C"}}}}}
	if err := dup.Validate(); err == nil {
		t.Fatal("duplicate id accepted")
	}
	if err := (Outline{Items: []Item{{Label: "!!"}}}).Validate(); err == nil {
		t.Fatal("label without a letter accepted")
	}
}

func TestStatusForAndToggle(t *testing.T) {
	outline := stored()
	if got := outline.LeafIDsFor(2); !reflect.DeepEqual(got, []int{3, 4}) {
		t.Fatalf("leaves = %v", got)
	}
	checked := map[int]bool{3: true}
	if outline.StatusFor(2, checked) != Partial || outline.StatusFor(3, checked) != Checked || outline.StatusFor(1, checked) != Unchecked {
		t.Fatal("status mismatch")
	}
	leaves, on, err := outline.Toggle(2, checked)
	if err != nil || !on || !reflect.DeepEqual(leaves, []int{3, 4}) {
		t.Fatalf("toggle partial = %v %v %v", leaves, on, err)
	}
	all := map[int]bool{3: true, 4: true}
	if outline.StatusFor(2, all) != Checked || outline.CompletedCount(all) != 2 {
		t.Fatal("parent should be checked")
	}
	if leaves, on, err := outline.Toggle(2, all); err != nil || on || len(leaves) != 2 {
		t.Fatalf("toggle checked = %v %v %v", leaves, on, err)
	}
	if _, _, err := outline.Toggle(99, all); err == nil {
		t.Fatal("unknown id toggled")
	}
}

func TestPlaceAfter(t *testing.T) {
	outline := stored()
	parent, position, err := outline.PlaceAfter(3, "Review")
	if err != nil || parent != 2 || position != 1 {
		t.Fatalf("child place = %d %d %v", parent, position, err)
	}
	parent, position, err = outline.PlaceAfter(5, "Sleep")
	if err != nil || parent != 0 || position != 3 {
		t.Fatalf("root place = %d %d %v", parent, position, err)
	}
	if _, _, err := outline.PlaceAfter(3, "deep  WORK"); err == nil {
		t.Fatal("sibling duplicate accepted")
	}
	if _, _, err := outline.PlaceAfter(1, "Email"); err != nil {
		t.Fatalf("label of a cousin rejected: %v", err)
	}
	if _, _, err := outline.PlaceAfter(99, "New"); err == nil {
		t.Fatal("unknown selection accepted")
	}
	if parent, position, err := (Outline{}).PlaceAfter(0, "First"); err != nil || parent != 0 || position != 0 {
		t.Fatalf("empty place = %d %d %v", parent, position, err)
	}
	if _, _, err := (Outline{}).PlaceAfter(0, " "); err == nil {
		t.Fatal("empty label accepted")
	}
}

func TestCheckRename(t *testing.T) {
	outline := stored()
	if err := outline.CheckRename(2, "Job"); err != nil {
		t.Fatal(err)
	}
	if err := outline.CheckRename(3, "Email!"); err != nil {
		t.Fatalf("rename to its own key: %v", err)
	}
	if err := outline.CheckRename(3, "Deep work"); err == nil {
		t.Fatal("sibling duplicate accepted")
	}
	if err := outline.CheckRename(1, "Email"); err != nil {
		t.Fatalf("label of a child elsewhere rejected: %v", err)
	}
	if err := outline.CheckRename(99, "X"); err == nil {
		t.Fatal("unknown id accepted")
	}
}

func TestKey(t *testing.T) {
	cases := map[string]string{"Deep work": "deep-work", "  A_b  ": "a_b", "Plan, tomorrow!": "plan-tomorrow", "!!": ""}
	for label, want := range cases {
		if got := Key(label); got != want {
			t.Fatalf("Key(%q) = %q want %q", label, got, want)
		}
	}
}
