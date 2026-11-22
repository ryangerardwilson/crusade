// Package lineedit edits one line of text with basic Emacs keys and draws it
// with a block cursor. Keys are the names bubble tea's KeyMsg.String returns,
// so callers keep bubble tea at their edge and pass plain strings in. The
// cursor is a rune index into the text.
package lineedit

import (
	"unicode"
	"unicode/utf8"
)

// Block is drawn after the last character when the cursor sits at the end.
const Block = "█"

// Key applies one key to text at cursor. It reports false when the key is not
// a line-editing key, so the caller can handle it (enter, esc, tab, paste).
func Key(text string, cursor int, key string) (string, int, bool) {
	runes := []rune(text)
	cursor = clamp(cursor, 0, len(runes))
	switch key {
	case "backspace", "ctrl+h":
		return cut(runes, cursor-1, cursor)
	case "ctrl+d", "delete":
		return cut(runes, cursor, cursor+1)
	case "left", "ctrl+b":
		return text, clamp(cursor-1, 0, len(runes)), true
	case "right", "ctrl+f":
		return text, clamp(cursor+1, 0, len(runes)), true
	case "home", "ctrl+a":
		return text, 0, true
	case "end", "ctrl+e":
		return text, len(runes), true
	case "ctrl+k":
		return cut(runes, cursor, len(runes))
	case "ctrl+u":
		return cut(runes, 0, cursor)
	case "ctrl+w", "alt+backspace":
		return cut(runes, backwardWord(runes, cursor), cursor)
	case "alt+d":
		return cut(runes, cursor, forwardWord(runes, cursor))
	case "alt+f":
		return text, forwardWord(runes, cursor), true
	case "alt+b":
		return text, backwardWord(runes, cursor), true
	case "ctrl+t":
		return transpose(runes, cursor)
	}
	if r, size := utf8.DecodeRuneInString(key); size == len(key) && r != utf8.RuneError && unicode.IsPrint(r) {
		next, at := Insert(text, cursor, key)
		return next, at, true
	}
	return text, cursor, false
}

// Insert puts extra into text at cursor and returns the cursor after it.
func Insert(text string, cursor int, extra string) (string, int) {
	runes := []rune(text)
	cursor = clamp(cursor, 0, len(runes))
	add := []rune(extra)
	next := append(append(append([]rune{}, runes[:cursor]...), add...), runes[cursor:]...)
	return string(next), cursor + len(add)
}

// Show draws text with a block cursor. At the end it appends Block; on a
// character it passes that character to reverse so the cell reads as a block
// and the glyph stays visible.
func Show(text string, cursor int, reverse func(string) string) string {
	runes := []rune(text)
	cursor = clamp(cursor, 0, len(runes))
	if cursor == len(runes) {
		return text + Block
	}
	return string(runes[:cursor]) + reverse(string(runes[cursor])) + string(runes[cursor+1:])
}

func cut(runes []rune, from int, to int) (string, int, bool) {
	from = clamp(from, 0, len(runes))
	to = clamp(to, 0, len(runes))
	if to < from {
		from, to = to, from
	}
	return string(append(append([]rune{}, runes[:from]...), runes[to:]...)), from, true
}

func transpose(runes []rune, cursor int) (string, int, bool) {
	if len(runes) < 2 || cursor == 0 {
		return string(runes), cursor, true
	}
	if cursor == len(runes) {
		runes[cursor-2], runes[cursor-1] = runes[cursor-1], runes[cursor-2]
	} else {
		runes[cursor-1], runes[cursor] = runes[cursor], runes[cursor-1]
		cursor++
	}
	return string(runes), cursor, true
}

func forwardWord(runes []rune, cursor int) int {
	for cursor < len(runes) && !isWordRune(runes[cursor]) {
		cursor++
	}
	for cursor < len(runes) && isWordRune(runes[cursor]) {
		cursor++
	}
	return cursor
}

func backwardWord(runes []rune, cursor int) int {
	for cursor > 0 && !isWordRune(runes[cursor-1]) {
		cursor--
	}
	for cursor > 0 && isWordRune(runes[cursor-1]) {
		cursor--
	}
	return cursor
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func clamp(value, low, high int) int {
	return max(low, min(value, high))
}
