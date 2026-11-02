package tui

import (
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ryangerardwilson/crusade/internal/model"
	"github.com/ryangerardwilson/crusade/internal/store"
	"github.com/ryangerardwilson/crusade/internal/version"
	"github.com/ryangerardwilson/crusade/nlp"
	"golang.org/x/term"
)

const nlpHelp = `crusade -m nlp
neuro-linguistic programming trainer

  crusade -m nlp
    open the practice list
  nlp
    open the same mode

Sessions stay in the sqlx database nlp.
Each session is one page and opens into touch typing.
n creates a session, then vim edits its text. e opens the selected session in vim. ,rn renames it. dd deletes it.
? lists the shortcuts on the session list. During touch typing, ? is typed.
Tab leaves this mode, except while a session is being named or typed. q quits.
`

func stdoutIsTerm(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

var openNLPModel = func() (tea.Model, error) {
	return nlp.Open(version.Version)
}

func runNLP(dataPath string, stdout, stderr io.Writer) int {
	rememberMode(dataPath, model.ModeNLP)
	if !stdoutIsTerm(stdout) {
		fmt.Fprint(stdout, nlpHelp)
		return 0
	}
	inner, err := openNLPModel()
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	app := nlpMode{dataPath: dataPath, inner: inner, width: 80, height: 24}
	program := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

type nlpMode struct {
	dataPath string
	inner    tea.Model
	width    int
	height   int
	memory   modeMemory
	errText  string
}

func (m nlpMode) Init() tea.Cmd {
	if m.inner == nil {
		return nil
	}
	return m.inner.Init()
}

func (m nlpMode) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch message := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = message.Width
		m.height = message.Height
		return m.forwardSize(message.Width, message.Height)
	case tea.KeyMsg:
		if message.String() == "tab" && !nlp.HoldsTab(m.inner) {
			return m.cycleMode()
		}
	}
	if m.inner == nil {
		return m, nil
	}
	inner, cmd := m.inner.Update(msg)
	m.inner = inner
	return m, cmd
}

func (m nlpMode) forwardSize(width, height int) (nlpMode, tea.Cmd) {
	if m.inner == nil {
		return m, nil
	}
	innerHeight := height - 1
	if innerHeight < 1 {
		innerHeight = 1
	}
	inner, cmd := m.inner.Update(tea.WindowSizeMsg{Width: width, Height: innerHeight})
	m.inner = inner
	return m, cmd
}

func (m nlpMode) View() string {
	body := ""
	if m.inner != nil {
		body = m.inner.View()
	}
	if m.errText != "" {
		body = m.errText + "\n" + body
	}
	return renderModeBanner("NLP", m.width) + "\n" + body
}

func (m nlpMode) cycleMode() (tea.Model, tea.Cmd) {
	records, err := store.Load(m.dataPath)
	if err != nil {
		m.errText = err.Error()
		return m, nil
	}
	mem := m.memory
	saved := m
	saved.memory = modeMemory{}
	mem.nlp = &saved
	mem.hasNlp = true
	next := newAppModel(m.dataPath, records)
	next.width = m.width
	next.height = m.height
	next.memory = mem
	if mem.hasDo {
		next.restoreSession(mem.do)
	}
	next.clampState()
	rememberMode(m.dataPath, model.ModeDo)
	return next, nil
}

func openNLPMode(dataPath string, width, height int, memory modeMemory) (nlpMode, error) {
	if memory.hasNlp && memory.nlp != nil {
		next := *memory.nlp
		next.width = width
		next.height = height
		next.memory = memory
		next.errText = ""
		next, _ = next.forwardSize(width, height)
		return next, nil
	}
	inner, err := openNLPModel()
	if err != nil {
		return nlpMode{}, err
	}
	next := nlpMode{dataPath: dataPath, inner: inner, width: width, height: height, memory: memory}
	next, _ = next.forwardSize(width, height)
	return next, nil
}
