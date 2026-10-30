package box

import (
	"fmt"
	"strings"
	"unicode"
)

type Item struct {
	ID       string
	Label    string
	Children []Item
}

type Outline struct {
	Items []Item
}

type FlatItem struct {
	ID          string
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

func ParseOutline(text string) (Outline, error) {
	type pending struct {
		depth int
		item  Item
	}
	var roots []Item
	stack := []pending{}
	seen := map[string]struct{}{}
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
		parts := make([]string, 0, len(stack)+1)
		for _, parent := range stack {
			parts = append(parts, parent.item.ID)
		}
		parts = append(parts, normalizeID(label))
		id := strings.Join(parts, "/")
		if id == "" {
			return Outline{}, fmt.Errorf("line %d: item label must contain a letter or number", lineNumber+1)
		}
		if _, ok := seen[id]; ok {
			return Outline{}, fmt.Errorf("line %d: duplicate item id %q", lineNumber+1, id)
		}
		seen[id] = struct{}{}
		item := Item{ID: id, Label: label}
		if depth == 0 {
			roots = append(roots, item)
			stack = append(stack, pending{depth: depth, item: item})
			continue
		}
		parent := &roots
		for _, ancestor := range stack {
			found := findItem(parent, ancestor.item.ID)
			if found == nil {
				return Outline{}, fmt.Errorf("line %d: internal outline parent lookup failed", lineNumber+1)
			}
			parent = &found.Children
		}
		*parent = append(*parent, item)
		stack = append(stack, pending{depth: depth, item: item})
	}
	outline := Outline{Items: roots}
	if err := outline.Validate(); err != nil {
		return Outline{}, err
	}
	return outline, nil
}

func (o Outline) Validate() error {
	seen := map[string]struct{}{}
	var walk func(items []Item) error
	walk = func(items []Item) error {
		for _, item := range items {
			id := strings.TrimSpace(item.ID)
			label := strings.TrimSpace(item.Label)
			if id == "" {
				return fmt.Errorf("item %q has empty id", label)
			}
			if label == "" {
				return fmt.Errorf("item %q has empty label", id)
			}
			if strings.ContainsAny(id, " \t\r\n") {
				return fmt.Errorf("item id %q must not contain whitespace", id)
			}
			if _, ok := seen[id]; ok {
				return fmt.Errorf("duplicate item id %q", id)
			}
			seen[id] = struct{}{}
			if err := walk(item.Children); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(o.Items)
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
			flat = append(flat, FlatItem{
				ID:          item.ID,
				Label:       item.Label,
				Depth:       depth,
				HasChildren: len(item.Children) > 0,
			})
			walk(item.Children, depth+1)
		}
	}
	walk(o.Items, 0)
	return flat
}

func (o Outline) ItemByID(id string) (Item, bool) {
	for _, item := range o.Items {
		if found, ok := itemByID(item, id); ok {
			return found, true
		}
	}
	return Item{}, false
}

func (o Outline) LeafIDsFor(id string) []string {
	item, ok := o.ItemByID(id)
	if !ok {
		return nil
	}
	return item.leafIDs()
}

func (o Outline) LeafIDs() []string {
	var ids []string
	for _, item := range o.Items {
		ids = append(ids, item.leafIDs()...)
	}
	return ids
}

func (o Outline) LeafCount() int {
	return len(o.LeafIDs())
}

func (o Outline) StatusFor(id string, checked map[string]bool) Status {
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
	if count == 0 {
		return Unchecked
	}
	if count == len(leaves) {
		return Checked
	}
	return Partial
}

func (o Outline) CompletedCount(checked map[string]bool) int {
	count := 0
	for _, leaf := range o.LeafIDs() {
		if checked[leaf] {
			count++
		}
	}
	return count
}

func ApplyCheck(outline Outline, checked map[string]bool, id string, on bool) (map[string]bool, error) {
	if _, ok := outline.ItemByID(id); !ok {
		return nil, fmt.Errorf("unknown item id %q", id)
	}
	next := map[string]bool{}
	for leaf, value := range checked {
		if value {
			next[leaf] = true
		}
	}
	for _, leaf := range outline.LeafIDsFor(id) {
		if on {
			next[leaf] = true
		} else {
			delete(next, leaf)
		}
	}
	return next, nil
}

func Toggle(outline Outline, checked map[string]bool, id string) (map[string]bool, error) {
	on := outline.StatusFor(id, checked) != Checked
	return ApplyCheck(outline, checked, id, on)
}

func (i Item) leafIDs() []string {
	if len(i.Children) == 0 {
		return []string{i.ID}
	}
	var ids []string
	for _, child := range i.Children {
		ids = append(ids, child.leafIDs()...)
	}
	return ids
}

func itemByID(item Item, id string) (Item, bool) {
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

func findItem(items *[]Item, id string) *Item {
	for index := range *items {
		if (*items)[index].ID == id {
			return &(*items)[index]
		}
	}
	return nil
}

func parseOutlineLine(rawLine string) (int, string, error) {
	spaces := 0
	for _, r := range rawLine {
		switch r {
		case ' ':
			spaces++
		case '\t':
			spaces += 2
		default:
			goto done
		}
	}
done:
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

func normalizeID(label string) string {
	var b strings.Builder
	lastBreak := false
	for _, r := range strings.ToLower(strings.TrimSpace(label)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastBreak = false
		case r == '-' || r == '_':
			if b.Len() > 0 && !lastBreak {
				b.WriteRune(r)
				lastBreak = true
			}
		default:
			if b.Len() > 0 && !lastBreak {
				b.WriteRune('-')
				lastBreak = true
			}
		}
	}
	return strings.Trim(b.String(), "-_")
}
