package model

import (
	"fmt"
	"strings"
)

// Do item kinds.
const (
	DoKindHeading = "heading"
	DoKindBlank   = "blank"
	DoKindItem    = "item"
)

// DoItem is one line of the do list: a heading, a blank line, or an item.
// A heading keeps its line as written, # included. An item's Body is the
// outcome without its "- " bullet. A blank has no body. ID is the stored
// row, or zero for a row not saved yet.
type DoItem struct {
	ID   int
	Kind string
	Body string
}

// ParseDoLine reads one line of the do list.
func ParseDoLine(line string) DoItem {
	line = strings.TrimRight(line, "\r")
	switch {
	case strings.TrimSpace(line) == "":
		return DoItem{Kind: DoKindBlank}
	case strings.HasPrefix(line, "#"):
		return DoItem{Kind: DoKindHeading, Body: line}
	case strings.HasPrefix(line, "- "):
		body := strings.TrimPrefix(line, "- ")
		if strings.TrimSpace(body) == "" {
			return DoItem{Kind: DoKindBlank}
		}
		return DoItem{Kind: DoKindItem, Body: body}
	case strings.TrimSpace(line) == "-":
		return DoItem{Kind: DoKindBlank}
	default:
		return DoItem{Kind: DoKindItem, Body: line}
	}
}

// ParseDoList reads the do list text in order, one row per line.
func ParseDoList(text string) []DoItem {
	text = strings.TrimSuffix(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	rows := make([]DoItem, 0, len(lines))
	for _, line := range lines {
		rows = append(rows, ParseDoLine(line))
	}
	return rows
}

// Line is the row as the do list text shows it.
func (row DoItem) Line() string {
	switch row.Kind {
	case DoKindItem:
		return "- " + row.Body
	case DoKindBlank:
		return ""
	default:
		return row.Body
	}
}

// RenderDoList is the do list text of rows.
func RenderDoList(rows []DoItem) string {
	if len(rows) == 0 {
		return ""
	}
	lines := make([]string, len(rows))
	for index, row := range rows {
		lines[index] = row.Line()
	}
	return strings.Join(lines, "\n") + "\n"
}

// NewDoItem is an item with outcome text on one line.
func NewDoItem(text string) (DoItem, error) {
	body := strings.Join(strings.Fields(text), " ")
	if body == "" {
		return DoItem{}, fmt.Errorf("outcome cannot be empty")
	}
	return DoItem{Kind: DoKindItem, Body: body}, nil
}

// WithBody is the row with new text. A heading still starts with #, an item
// needs text, and a blank line has none.
func (row DoItem) WithBody(text string) (DoItem, error) {
	switch row.Kind {
	case DoKindItem:
		next, err := NewDoItem(text)
		next.ID = row.ID
		return next, err
	case DoKindHeading:
		body := strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
		if !strings.HasPrefix(body, "#") {
			return DoItem{}, fmt.Errorf("a heading starts with #")
		}
		return DoItem{ID: row.ID, Kind: DoKindHeading, Body: body}, nil
	default:
		return DoItem{}, fmt.Errorf("a blank line has no text")
	}
}

// Validate reports a row the do_item table would refuse.
func (row DoItem) Validate() error {
	if strings.ContainsAny(row.Body, "\r\n") {
		return fmt.Errorf("a do row is one line")
	}
	switch row.Kind {
	case DoKindBlank:
		if row.Body != "" {
			return fmt.Errorf("a blank line has no text")
		}
	case DoKindHeading:
		if !strings.HasPrefix(row.Body, "#") {
			return fmt.Errorf("a heading starts with #")
		}
	case DoKindItem:
		if strings.TrimSpace(row.Body) == "" {
			return fmt.Errorf("outcome cannot be empty")
		}
	default:
		return fmt.Errorf("unknown do row kind %q", row.Kind)
	}
	return nil
}

// MatchDoItems gives the rows of next the ids of the current rows they keep.
// A line left as it was keeps its id. Between kept lines, an edited line takes
// the id of the next replaced line of the same kind, in order, so an edited
// body keeps its id. A row of next with no match has id zero.
func MatchDoItems(current, next []DoItem) []DoItem {
	out := append([]DoItem(nil), next...)
	for index := range out {
		out[index].ID = 0
	}
	ci, ni := 0, 0
	for _, pair := range append(commonRows(current, next), [2]int{len(current), len(next)}) {
		matchGap(current[ci:pair[0]], out[ni:pair[1]])
		if pair[0] < len(current) && pair[1] < len(next) {
			out[pair[1]].ID = current[pair[0]].ID
		}
		ci, ni = pair[0]+1, pair[1]+1
	}
	return out
}

// matchGap pairs replaced rows with edited rows of the same kind, in order.
func matchGap(replaced, edited []DoItem) {
	at := 0
	for _, old := range replaced {
		for scan := at; scan < len(edited); scan++ {
			if edited[scan].Kind == old.Kind {
				edited[scan].ID = old.ID
				at = scan + 1
				break
			}
		}
	}
}

// commonRows is the longest run of identical lines in current and next, as
// index pairs in order.
func commonRows(current, next []DoItem) [][2]int {
	n, m := len(current), len(next)
	table := make([][]int, n+1)
	for i := range table {
		table[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if sameDoLine(current[i], next[j]) {
				table[i][j] = table[i+1][j+1] + 1
			} else {
				table[i][j] = max(table[i+1][j], table[i][j+1])
			}
		}
	}
	var pairs [][2]int
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case sameDoLine(current[i], next[j]):
			pairs = append(pairs, [2]int{i, j})
			i++
			j++
		case table[i+1][j] >= table[i][j+1]:
			i++
		default:
			j++
		}
	}
	return pairs
}

func sameDoLine(a, b DoItem) bool {
	return a.Kind == b.Kind && a.Body == b.Body
}
