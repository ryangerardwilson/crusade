package nlp

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ryangerardwilson/crusade/nlp/internal/app"
)

// Open loads the practice sessions for the nlp mode.
func Open(version string) (tea.Model, error) {
	return app.Open(version)
}

// HoldsTab reports that tab belongs to the practice text.
func HoldsTab(m tea.Model) bool {
	return app.HoldsTab(m)
}
