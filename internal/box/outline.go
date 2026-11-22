package box

import (
	"fmt"
	"strings"
	"unicode"
)

// Item is one box. ID is the stored box id, or zero for a box parsed from
// text that is not stored yet.
type Item struct {
	ID       int
	Label    string
	Children []Item
}

type Outline struct {
	Items []Item
}

type FlatItem struct {
	ID          int
	Label       string
	Depth       int
	HasChildren bool
}

type Status int

const (
	Unchecked Status = iota
	Partial
	Checked
)

// ParseOutline reads an indented text outline, two spaces per level.
func ParseOutline(text string) (Outline, error) {
	var roots []Item
	var stack []*[]Item
	for lineNumber, rawLine := range strings.Split(text, "\n") {
		if strings.TrimSpace(rawLine) == "" || strings.HasPrefix(strings.TrimSpace(rawLine), "#") {
			continue
		}
		depth, label, err := parseOutlineLine(rawLine)
		if err != nil {
			return Outline{}, fmt.Errorf("line %d: %w", lineNumber+1, err)
		}
		if label == "" {
			continue
		}
		if depth > len(stack) {
			return Outline{}, fmt.Errorf("line %d: indentation jumps more than one level", lineNumber+1)
		}
		stack = stack[:depth]
		parent := &roots
		if depth > 0 {
			siblings := stack[depth-1]
			parent = &(*siblings)[len(*siblings)-1].Children
		}
		*parent = append(*parent, Item{Label: label})
		stack = append(stack, parent)
	}
	outline := Outline{Items: roots}
	if err := outline.Validate(); err != nil {
		return Outline{}, err
	}
	return outline, nil
}

// Validate checks that every box has a label with a letter or number, that
// no two boxes under one parent share a label, and that stored ids are unique.
func (o Outline) Validate() error {
	seen := map[int]bool{}
	var walk func(items []Item) error
	walk = func(items []Item) error {
		keys := map[string]bool{}
		for _, item := range items {
			if err := checkLabel(item.Label, keys); err != nil {
				return err
			}
			if item.ID != 0 {
				if seen[item.ID] {
					return fmt.Errorf("duplicate box id %d", item.ID)
				}
				seen[item.ID] = true
			}
			if err := walk(item.Children); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(o.Items)
}

func checkLabel(label string, siblings map[string]bool) error {
	label = strings.TrimSpace(label)
	if label == "" {
		return fmt.Errorf("box text cannot be empty")
	}
	key := Key(label)
	if key == "" {
		return fmt.Errorf("box text must contain a letter or number")
	}
	if siblings[key] {
		return fmt.Errorf("a box named %q already exists", label)
	}
	siblings[key] = true
	return nil
}

func MarshalOutline(outline Outline) string {
	var lines []string
	var walk func(items []Item, depth int)
	walk = func(items []Item, depth int) {
		for _, item := range items {
			lines = append(lines, strings.Repeat("  ", depth)+item.Label)
			walk(item.Children, depth+1)
		}
	}
	walk(outline.Items, 0)
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func (o Outline) Flatten() []FlatItem {
	var flat []FlatItem
	var walk func(items []Item, depth int)
	walk = func(items []Item, depth int) {
		for _, item := range items {
			flat = append(flat, FlatItem{ID: item.ID, Label: item.Label, Depth: depth, HasChildren: len(item.Children) > 0})
			walk(item.Children, depth+1)
		}
	}
	walk(o.Items, 0)
	return flat
}

// Paths is the label path of each box, as parent/child keys, in Flatten order.
func (o Outline) Paths() []string {
	var paths []string
	var walk func(items []Item, parent string)
	walk = func(items []Item, parent string) {
		for _, item := range items {
			path := Key(item.Label)
			if parent != "" {
				path = parent + "/" + path
			}
			paths = append(paths, path)
			walk(item.Children, path)
		}
	}
	walk(o.Items, "")
	return paths
}

func (o Outline) ItemByID(id int) (Item, bool) {
	for _, item := range o.Items {
		if found, ok := itemByID(item, id); ok {
			return found, true
		}
	}
	return Item{}, false
}

func (o Outline) LeafIDsFor(id int) []int {
	item, ok := o.ItemByID(id)
	if !ok {
		return nil
	}
	return item.leafIDs()
}

func (o Outline) LeafIDs() []int {
	var ids []int
	for _, item := range o.Items {
		ids = append(ids, item.leafIDs()...)
	}
	return ids
}

func (o Outline) LeafCount() int {
	return len(o.LeafIDs())
}

func (o Outline) StatusFor(id int, checked map[int]bool) Status {
	leaves := o.LeafIDsFor(id)
	if len(leaves) == 0 {
		return Unchecked
	}
	count := 0
	for _, leaf := range leaves {
		if checked[leaf] {
			count++
		}
	}
	switch count {
	case 0:
		return Unchecked
	case len(leaves):
		return Checked
	default:
		return Partial
	}
}

func (o Outline) CompletedCount(checked map[int]bool) int {
	count := 0
	for _, leaf := range o.LeafIDs() {
		if checked[leaf] {
			count++
		}
	}
	return count
}

// Toggle is the check a press of space on id makes: the leaves under id, and
// whether they become checked.
func (o Outline) Toggle(id int, checked map[int]bool) ([]int, bool, error) {
	if _, ok := o.ItemByID(id); !ok {
		return nil, false, fmt.Errorf("unknown box id %d", id)
	}
	return o.LeafIDsFor(id), o.StatusFor(id, checked) != Checked, nil
}

// PlaceAfter is where a new box labelled label goes: beside selectedID,
// right after it. The parent is zero for a root. An empty outline puts it
// first.
func (o Outline) PlaceAfter(selectedID int, label string) (int, int, error) {
	if len(o.Items) == 0 {
		return 0, 0, checkLabel(label, map[string]bool{})
	}
	parent, siblings, index, ok := locate(o.Items, 0, selectedID)
	if !ok {
		return 0, 0, fmt.Errorf("selected box was not found")
	}
	if err := checkLabel(label, siblingKeys(siblings, 0)); err != nil {
		return 0, 0, err
	}
	return parent, index + 1, nil
}

// CheckRename reports whether id may take label.
func (o Outline) CheckRename(id int, label string) error {
	_, siblings, _, ok := locate(o.Items, 0, id)
	if !ok {
		return fmt.Errorf("selected box was not found")
	}
	return checkLabel(label, siblingKeys(siblings, id))
}

func siblingKeys(items []Item, except int) map[string]bool {
	keys := map[string]bool{}
	for _, item := range items {
		if item.ID != except {
			keys[Key(item.Label)] = true
		}
	}
	return keys
}

// locate finds id and returns its parent id, its siblings, and its index.
func locate(items []Item, parent, id int) (int, []Item, int, bool) {
	for index, item := range items {
		if item.ID == id {
			return parent, items, index, true
		}
		if p, siblings, at, ok := locate(item.Children, item.ID, id); ok {
			return p, siblings, at, true
		}
	}
	return 0, nil, 0, false
}

func (i Item) leafIDs() []int {
	if len(i.Children) == 0 {
		return []int{i.ID}
	}
	var ids []int
	for _, child := range i.Children {
		ids = append(ids, child.leafIDs()...)
	}
	return ids
}

func itemByID(item Item, id int) (Item, bool) {
	if item.ID == id {
		return item, true
	}
	for _, child := range item.Children {
		if found, ok := itemByID(child, id); ok {
			return found, true
		}
	}
	return Item{}, false
}

func parseOutlineLine(rawLine string) (int, string, error) {
	spaces := 0
	for _, r := range rawLine {
		if r == ' ' {
			spaces++
		} else if r == '\t' {
			spaces += 2
		} else {
			break
		}
	}
	if spaces%2 != 0 {
		return 0, "", fmt.Errorf("indent with two spaces per level")
	}
	label := strings.TrimSpace(rawLine)
	for _, prefix := range []string{"- ", "* ", "+ ", "[ ] "} {
		if strings.HasPrefix(label, prefix) {
			label = strings.TrimSpace(strings.TrimPrefix(label, prefix))
			break
		}
	}
	if strings.HasPrefix(label, "[x] ") || strings.HasPrefix(label, "[X] ") {
		label = strings.TrimSpace(label[4:])
	}
	return spaces / 2, strings.TrimSpace(label), nil
}

// Key is the label as a lowercase slug. Two boxes under one parent may not
// share a key.
func Key(label string) string {
	var b strings.Builder
	lastBreak := false
	for _, r := range strings.ToLower(strings.TrimSpace(label)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastBreak = false
		case b.Len() > 0 && !lastBreak:
			if r == '_' {
				b.WriteRune(r)
			} else {
				b.WriteRune('-')
			}
			lastBreak = true
		}
	}
	return strings.Trim(b.String(), "-_")
}
