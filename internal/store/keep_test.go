package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ryangerardwilson/crusade/internal/model"
)

// TestWriteMarksCrusadeForKeep runs the store against the runtime path with a
// nebula that logs its arguments and restores to an empty file.
func TestWriteMarksCrusadeForKeep(t *testing.T) {
	testDB(t)
	run := t.TempDir()
	t.Setenv("SQLX_ROOT", "")
	t.Setenv("SQLX_RUNTIME", run)
	t.Setenv("XDG_RUNTIME_DIR", run)
	logPath := filepath.Join(t.TempDir(), "nebula.log")
	bin := filepath.Join(t.TempDir(), "nebula")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + logPath + "'\n" +
		"[ \"$2\" = restore ] || exit 0\numask 077\n: > \"$4\"\necho \"fresh $4\"\n"
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NEBULA_BIN", bin)
	path := DatabasePath()

	if err := SaveLastMode(path, model.ModeAlign); err != nil {
		t.Fatal(err)
	}
	restored := "keep restore crusade " + path + "\n"
	if got := readFile(t, logPath); got != restored {
		t.Fatalf("a file with no records ran nebula %q", got)
	}
	if _, err := AddRecord(path, mustEvent(t, "2026-02-01 00:00", "kept")); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, logPath); got != restored+"keep mark crusade "+path+"\n" {
		t.Fatalf("nebula ran %q", got)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
