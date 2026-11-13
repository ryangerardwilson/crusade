package dofile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryangerardwilson/crusade/internal/model"
	"github.com/ryangerardwilson/crusade/internal/store"
)

func TestEnsureSeedsDoOutcomesOnce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("SQLX_ROOT", filepath.Join(home, "sqlx"))
	t.Setenv("BOXES_CONFIG", filepath.Join(home, "missing-outline.txt"))
	db := filepath.Join(home, "crusade.sqlite")
	doEvent, err := model.NewDoEvent("2026-06-13 23:59:59", "mail the letter")
	if err != nil {
		t.Fatal(err)
	}
	alignEvent, err := model.NewEvent("2026-06-14 23:59:59", "ship the patch", "reduce drift")
	if err != nil {
		t.Fatal(err)
	}
	records := []store.Record{
		{ID: 1, Event: doEvent},
		{ID: 2, Event: alignEvent},
	}
	if err := store.Save(db, records); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(db); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(Path())
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "mail the letter\n" {
		t.Fatalf("seed = %q", body)
	}
	if err := os.WriteFile(Path(), []byte("# today\n- mail the letter\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(db); err != nil {
		t.Fatal(err)
	}
	body, err = os.ReadFile(Path())
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "# today\n- mail the letter\n" {
		t.Fatalf("rewritten seed:\n%s", body)
	}
}

func TestOpenUsesTheEditor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("SQLX_ROOT", filepath.Join(home, "sqlx"))
	t.Setenv("BOXES_CONFIG", filepath.Join(home, "missing-outline.txt"))
	t.Setenv("VISUAL", "nvim")
	t.Setenv("EDITOR", "omarchy-launch-editor --inline")
	logPath := filepath.Join(home, "opened")
	editor := filepath.Join(home, "editor")
	script := "#!/bin/sh\nprintf '%s\\n' \"$1\" > '" + logPath + "'\n"
	if err := os.WriteFile(editor, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	previous := VimArgs
	VimArgs = []string{editor}
	t.Cleanup(func() { VimArgs = previous })
	db := filepath.Join(home, "crusade.sqlite")
	if err := Open(db); err != nil {
		t.Fatal(err)
	}
	opened, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(opened)) != Path() {
		t.Fatalf("opened %q", opened)
	}
	info, err := os.Stat(Path())
	if err != nil || info.Size() != 0 {
		t.Fatalf("empty list stat=%v err=%v", info, err)
	}
}

func TestEditorCommandIsVim(t *testing.T) {
	t.Setenv("VISUAL", "nvim")
	t.Setenv("EDITOR", "omarchy-launch-editor --inline")
	got := editorCommand()
	if len(got) != 1 || got[0] != "vim" {
		t.Fatalf("command = %#v", got)
	}
}
