package dofile

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ryangerardwilson/crusade/internal/config"
	"github.com/ryangerardwilson/crusade/internal/store"
)

// Path is the freeform do list beside the crusade config file.
func Path() string {
	return filepath.Join(filepath.Dir(config.Path()), "do")
}

// Ensure creates the list from current do outcomes when the file is missing.
// An existing file is left untouched.
func Ensure(dataPath string) error {
	if err := config.Ensure(); err != nil {
		return err
	}
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	body, err := seedText(dataPath)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(body), 0o644)
}

func seedText(dataPath string) (string, error) {
	records, err := store.Load(dataPath)
	if err != nil {
		return "", err
	}
	lines := make([]string, 0, len(records))
	for _, record := range records {
		if !record.Event.IsDo() {
			continue
		}
		text := strings.Join(strings.Fields(record.Event.Y), " ")
		if text == "" {
			continue
		}
		lines = append(lines, text)
	}
	if len(lines) == 0 {
		return "", nil
	}
	return strings.Join(lines, "\n") + "\n", nil
}

// Open ensures the list and edits it with $VISUAL, $EDITOR, or vim.
func Open(dataPath string) error {
	if err := Ensure(dataPath); err != nil {
		return err
	}
	return runEditor(Path())
}

func runEditor(path string) error {
	parts := editorCommand()
	cmd := exec.Command(parts[0], append(parts[1:], path)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func editorCommand() []string {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vim"
	}
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return []string{"vim"}
	}
	return parts
}
