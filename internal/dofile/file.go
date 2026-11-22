package dofile

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ryangerardwilson/crusade/internal/config"
	"github.com/ryangerardwilson/crusade/internal/model"
	"github.com/ryangerardwilson/crusade/internal/store"
)

// Path is the Vim buffer of the do list.
func Path() string {
	return config.DoPath()
}

// Open edits the do list in Vim. The do_item rows are the list. The file is
// derived from them: it is written fresh from the rows for each session, and
// read back only as what Vim saved, which then replaces the rows in one
// transaction. A failed Vim leaves the rows as they were.
func Open(dataPath string) error {
	if err := config.Ensure(); err != nil {
		return err
	}
	rows, err := store.LoadDoList(dataPath)
	if err != nil {
		return err
	}
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Derived from do_item; regenerated on every open.
	if err := os.WriteFile(path, []byte(model.RenderDoList(rows)), 0o644); err != nil {
		return err
	}
	if err := runEditor(path); err != nil {
		return err
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return store.ReplaceDoList(dataPath, model.ParseDoList(string(saved)))
}

func runEditor(path string) error {
	parts := editorCommand()
	cmd := exec.Command(parts[0], append(parts[1:], path)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// VimArgs is the command that opens the do list. Do mode uses vim.
var VimArgs = []string{"vim"}

func editorCommand() []string {
	if len(VimArgs) == 0 {
		return []string{"vim"}
	}
	return append([]string(nil), VimArgs...)
}
