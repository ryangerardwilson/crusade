package dofile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryangerardwilson/crusade/internal/model"
	"github.com/ryangerardwilson/crusade/internal/store"
)

func TestOpenRendersRowsAndStoresWhatVimSaved(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("SQLX_ROOT", filepath.Join(home, "sqlx"))
	t.Setenv("BOXES_CONFIG", filepath.Join(home, "missing-outline.txt"))
	db := filepath.Join(home, "crusade.sqlite")
	mail, err := store.AddDoItem(db, 0, "mail the letter")
	if err != nil {
		t.Fatal(err)
	}
	editor := filepath.Join(home, "editor")
	shown := filepath.Join(home, "shown")
	script := "#!/bin/sh\ncp \"$1\" '" + shown + "'\nprintf '# today\\n\\n- mail the letter now\\nbuy stamps\\n' > \"$1\"\n"
	if err := os.WriteFile(editor, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	previous := VimArgs
	VimArgs = []string{editor}
	t.Cleanup(func() { VimArgs = previous })
	if err := Open(db); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(shown)
	if err != nil || string(before) != "- mail the letter\n" {
		t.Fatalf("vim was shown %q err=%v", before, err)
	}
	rows, err := store.LoadDoList(db)
	if err != nil {
		t.Fatal(err)
	}
	want := []model.DoItem{
		{Kind: model.DoKindHeading, Body: "# today"},
		{Kind: model.DoKindBlank},
		{ID: mail, Kind: model.DoKindItem, Body: "mail the letter now"},
		{Kind: model.DoKindItem, Body: "buy stamps"},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %#v", rows)
	}
	for index, row := range rows {
		if row.Kind != want[index].Kind || row.Body != want[index].Body || (want[index].ID != 0 && row.ID != want[index].ID) {
			t.Fatalf("row %d = %#v, want %#v", index, row, want[index])
		}
	}
	VimArgs = []string{"true"}
	if err := Open(db); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(Path())
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "# today\n\n- mail the letter now\n- buy stamps\n" {
		t.Fatalf("the next open wrote %q", body)
	}
}

func TestFailedVimKeepsTheRows(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("SQLX_ROOT", filepath.Join(home, "sqlx"))
	t.Setenv("BOXES_CONFIG", filepath.Join(home, "missing-outline.txt"))
	db := filepath.Join(home, "crusade.sqlite")
	if _, err := store.AddDoItem(db, 0, "mail the letter"); err != nil {
		t.Fatal(err)
	}
	editor := filepath.Join(home, "editor")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\n: > \"$1\"\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	previous := VimArgs
	VimArgs = []string{editor}
	t.Cleanup(func() { VimArgs = previous })
	if err := Open(db); err == nil {
		t.Fatal("a failed vim was saved")
	}
	rows, err := store.LoadDoList(db)
	if err != nil || len(rows) != 1 || rows[0].Body != "mail the letter" {
		t.Fatalf("rows = %#v err=%v", rows, err)
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
