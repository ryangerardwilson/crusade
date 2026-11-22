package lineedit

import (
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// TestImportsOnlyStandardLibrary keeps the editor free of bubble tea, lipgloss,
// and crusade packages, so both the tui and nlp can import it without a cycle.
func TestImportsOnlyStandardLibrary(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "lineedit.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range file.Imports {
		path, _ := strconv.Unquote(spec.Path.Value)
		if first, _, _ := strings.Cut(path, "/"); strings.Contains(first, ".") {
			t.Fatalf("lineedit imports %s", path)
		}
	}
}

func mark(cell string) string { return "[" + cell + "]" }

func TestShowDrawsBlockCursor(t *testing.T) {
	if got := Show("abc", 3, mark); got != "abc█" {
		t.Fatalf("end cursor %q", got)
	}
	if got := Show("abc", 1, mark); got != "a[b]c" {
		t.Fatalf("mid cursor %q", got)
	}
	if got := Show("", 0, mark); got != "█" {
		t.Fatalf("empty cursor %q", got)
	}
}

func TestKeyEmacsEditing(t *testing.T) {
	text, cursor := "go snorkelling", 14
	press := func(key string) {
		t.Helper()
		var ok bool
		text, cursor, ok = Key(text, cursor, key)
		if !ok {
			t.Fatalf("%s not handled", key)
		}
	}
	press("alt+b")
	if cursor != 3 {
		t.Fatalf("alt+b cursor %d", cursor)
	}
	press("ctrl+a")
	press("alt+f")
	if cursor != 2 {
		t.Fatalf("alt+f cursor %d", cursor)
	}
	press("ctrl+e")
	press("ctrl+h")
	if text != "go snorkellin" || cursor != 13 {
		t.Fatalf("ctrl+h %q %d", text, cursor)
	}
	press("ctrl+a")
	press("ctrl+f")
	press("ctrl+t")
	if text != "og snorkellin" || cursor != 2 {
		t.Fatalf("ctrl+t %q %d", text, cursor)
	}
	press("ctrl+k")
	press("é")
	if text != "ogé" || cursor != 3 {
		t.Fatalf("insert %q", text)
	}
	if _, _, ok := Key(text, cursor, "tab"); ok {
		t.Fatal("tab should be left to the caller")
	}
}
