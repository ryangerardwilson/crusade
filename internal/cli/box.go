package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ryangerardwilson/crusade/internal/box"
	"github.com/ryangerardwilson/crusade/internal/store"
)

func runBox(dbPath, op string, rest []string, stdout io.Writer) int {
	switch op {
	case "r":
		return showBoxes(dbPath, rest, stdout)
	case "c", "u":
		if len(rest) != 0 {
			fmt.Fprintf(stdout, "Usage: crusade -m box -%s\n", op)
			return 1
		}
		return editBoxes(dbPath, stdout)
	default:
		fmt.Fprintln(stdout, "Usage: crusade -m box [-r | -c | -u]")
		return 1
	}
}

func showBoxes(dbPath string, args []string, stdout io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(stdout, "Usage: crusade -m box -r")
		return 1
	}
	outline, checked, day, err := todayBoxes(dbPath)
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	fmt.Fprintf(stdout, "%s  %d/%d\n", day, outline.CompletedCount(checked), outline.LeafCount())
	if len(outline.Flatten()) == 0 {
		fmt.Fprintln(stdout, "No boxes yet. crusade -m box -c opens the outline in Vim.")
		return 0
	}
	for _, item := range outline.Flatten() {
		mark := "[ ]"
		switch outline.StatusFor(item.ID, checked) {
		case box.Checked:
			mark = "[x]"
		case box.Partial:
			mark = "[~]"
		}
		fmt.Fprintf(stdout, "%s%s %s  %s\n", indent(item.Depth), mark, item.ID, item.Label)
	}
	return 0
}

func editBoxes(dbPath string, stdout io.Writer) int {
	outline, err := store.LoadOutline(dbPath)
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	payload, err := box.MarshalYAML(outline)
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	tmp, err := os.CreateTemp("", "crusade-boxes-*.yaml")
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		fmt.Fprintln(stdout, err)
		return 1
	}
	if err := tmp.Close(); err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	parts := editorCommand()
	cmd := exec.Command(parts[0], append(parts[1:], tmpPath)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return 0
		}
		fmt.Fprintln(stdout, err)
		return 1
	}
	edited, err := os.ReadFile(tmpPath)
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	next, err := box.ParseYAML(edited)
	if err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	if err := store.SaveOutline(dbPath, next); err != nil {
		fmt.Fprintln(stdout, err)
		return 1
	}
	fmt.Fprintf(stdout, "saved %d\n", len(next.Flatten()))
	return 0
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

func todayBoxes(dbPath string) (box.Outline, map[string]bool, string, error) {
	outline, err := store.LoadOutline(dbPath)
	if err != nil {
		return box.Outline{}, nil, "", err
	}
	day := time.Now().Local().Format(time.DateOnly)
	checked, err := store.LoadDayChecks(dbPath, day)
	if err != nil {
		return box.Outline{}, nil, "", err
	}
	return outline, checked, day, nil
}

func indent(depth int) string {
	if depth <= 0 {
		return ""
	}
	return fmt.Sprintf("%*s", depth*2, "")
}
