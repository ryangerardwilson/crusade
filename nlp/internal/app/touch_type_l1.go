package app

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const cursorBlinkInterval = 500 * time.Millisecond

type cursorBlinkMsg time.Time

func renderTypedLine(raw, target, input string, width int, showCursor bool) string {
	if width <= 0 {
		return ""
	}
	inputRunes := []rune(input)
	targetRunes := []rune(target)
	var out strings.Builder
	var run []rune
	runKind := 0
	flush := func() {
		if len(run) == 0 {
			return
		}
		text := string(run)
		run = run[:0]
		switch runKind {
		case 1:
			out.WriteString(text)
		case 2:
			out.WriteString(mutedStyle.Render(text))
		case 3:
			out.WriteString(statusStyle.Render(text))
		case 4:
			out.WriteString(errorStyle.Render(text))
		}
		runKind = 0
	}
	write := func(kind int, r rune) {
		if len(run) > 0 && kind != runKind {
			flush()
		}
		runKind = kind
		run = append(run, r)
	}
	displayPos := 0
	inputPos := 0
	cursorRendered := false
	for _, r := range raw {
		if displayPos >= width {
			break
		}
		if r == '\t' {
			for spaces := 0; spaces < 4 && displayPos < width; spaces++ {
				write(1, ' ')
				displayPos++
			}
			continue
		}
		if showCursor && !cursorRendered && inputPos == len(inputRunes) {
			write(3, '█')
			displayPos++
			cursorRendered = true
			continue
		}
		ch := r
		kind := 2
		if inputPos < len(inputRunes) {
			if inputPos < len(targetRunes) && inputRunes[inputPos] == targetRunes[inputPos] {
				ch = inputRunes[inputPos]
				kind = 3
			} else {
				ch = '█'
				kind = 4
			}
			inputPos++
		}
		write(kind, ch)
		displayPos++
	}
	if showCursor && !cursorRendered && inputPos == len(inputRunes) && displayPos < width {
		write(3, '█')
		displayPos++
	}
	for inputPos < len(inputRunes) && displayPos < width {
		write(4, '█')
		inputPos++
		displayPos++
	}
	flush()
	return out.String()
}

func cursorBlinkCmd() tea.Cmd {
	return tea.Tick(cursorBlinkInterval, func(t time.Time) tea.Msg {
		return cursorBlinkMsg(t)
	})
}
