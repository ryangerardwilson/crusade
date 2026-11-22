package model

import (
	"reflect"
	"testing"
)

func TestParseDoList(t *testing.T) {
	got := ParseDoList("# Work\n\n- ship it\nplain line\n- \n")
	want := []DoItem{
		{Kind: DoKindHeading, Body: "# Work"},
		{Kind: DoKindBlank},
		{Kind: DoKindItem, Body: "ship it"},
		{Kind: DoKindItem, Body: "plain line"},
		{Kind: DoKindBlank},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %#v", got)
	}
	if rows := ParseDoList(""); len(rows) != 0 {
		t.Fatalf("empty rows = %#v", rows)
	}
}

func TestRenderDoListRoundTrip(t *testing.T) {
	rows := ParseDoList("# Work\n\n- ship it\nplain line\n\n## Home\n- call mum\n")
	text := RenderDoList(rows)
	want := "# Work\n\n- ship it\n- plain line\n\n## Home\n- call mum\n"
	if text != want {
		t.Fatalf("text = %q", text)
	}
	if again := ParseDoList(text); !reflect.DeepEqual(again, rows) {
		t.Fatalf("round trip = %#v", again)
	}
	if RenderDoList(nil) != "" {
		t.Fatal("empty list renders text")
	}
}

func numbered(rows []DoItem) []DoItem {
	for index := range rows {
		rows[index].ID = index + 1
	}
	return rows
}

func ids(rows []DoItem) []int {
	out := make([]int, len(rows))
	for index, row := range rows {
		out[index] = row.ID
	}
	return out
}

func TestMatchDoItemsKeepsUnchangedAndEdited(t *testing.T) {
	current := numbered(ParseDoList("# Work\n- a\n- b\n- c\n"))
	next := ParseDoList("# Work\n- a\n- b edited\n- c\n")
	if got := ids(MatchDoItems(current, next)); !reflect.DeepEqual(got, []int{1, 2, 3, 4}) {
		t.Fatalf("edited ids = %v", got)
	}
}

func TestMatchDoItemsNewLinesGetZero(t *testing.T) {
	current := numbered(ParseDoList("# Work\n- a\n- c\n"))
	next := ParseDoList("# Work\n- a\n- new\n\n- c\n- last\n")
	if got := ids(MatchDoItems(current, next)); !reflect.DeepEqual(got, []int{1, 2, 0, 0, 3, 0}) {
		t.Fatalf("new ids = %v", got)
	}
	reordered := ParseDoList("- c\n# Work\n")
	if got := ids(MatchDoItems(current, reordered)); got[0] == got[1] && got[0] != 0 {
		t.Fatalf("reordered ids = %v", got)
	}
	if got := MatchDoItems(current, nil); len(got) != 0 {
		t.Fatalf("empty next = %v", got)
	}
}

func TestMatchDoItemsDoesNotMutateNext(t *testing.T) {
	current := numbered(ParseDoList("- a\n"))
	next := []DoItem{{ID: 9, Kind: DoKindItem, Body: "a"}}
	out := MatchDoItems(current, next)
	if out[0].ID != 1 || next[0].ID != 9 {
		t.Fatalf("out = %v next = %v", out, next)
	}
}

func TestDoItemWithBody(t *testing.T) {
	heading := DoItem{ID: 4, Kind: DoKindHeading, Body: "# Work"}
	if next, err := heading.WithBody(" ## Home "); err != nil || next != (DoItem{ID: 4, Kind: DoKindHeading, Body: "## Home"}) {
		t.Fatalf("heading = %#v err=%v", next, err)
	}
	if _, err := heading.WithBody("Home"); err == nil {
		t.Fatal("heading without # accepted")
	}
	item := DoItem{ID: 5, Kind: DoKindItem, Body: "a"}
	if next, err := item.WithBody("  ship   it "); err != nil || next != (DoItem{ID: 5, Kind: DoKindItem, Body: "ship it"}) {
		t.Fatalf("item = %#v err=%v", next, err)
	}
	if _, err := item.WithBody("   "); err == nil {
		t.Fatal("empty item accepted")
	}
	if _, err := (DoItem{ID: 6, Kind: DoKindBlank}).WithBody("text"); err == nil {
		t.Fatal("blank with text accepted")
	}
}

func TestDoItemValidate(t *testing.T) {
	good := []DoItem{{Kind: DoKindHeading, Body: "# A"}, {Kind: DoKindBlank}, {Kind: DoKindItem, Body: "x"}}
	for _, row := range good {
		if err := row.Validate(); err != nil {
			t.Fatalf("%#v: %v", row, err)
		}
	}
	bad := []DoItem{
		{Kind: DoKindHeading, Body: "A"},
		{Kind: DoKindBlank, Body: "x"},
		{Kind: DoKindItem, Body: " "},
		{Kind: DoKindItem, Body: "a\nb"},
		{Kind: "other", Body: "x"},
	}
	for _, row := range bad {
		if err := row.Validate(); err == nil {
			t.Fatalf("%#v accepted", row)
		}
	}
}

func TestNewDoItem(t *testing.T) {
	row, err := NewDoItem(" ship\nit ")
	if err != nil || row != (DoItem{Kind: DoKindItem, Body: "ship it"}) || row.Line() != "- ship it" {
		t.Fatalf("row = %#v err=%v", row, err)
	}
	if _, err := NewDoItem(" "); err == nil {
		t.Fatal("empty item accepted")
	}
}
