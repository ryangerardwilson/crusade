package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCourseWriteMarksNLPForKeep seeds the courses on the runtime path with a
// nebula that logs its arguments and restores to an empty file.
func TestCourseWriteMarksNLPForKeep(t *testing.T) {
	home := t.TempDir()
	run := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
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

	if _, err := LoadCourses("0.0.0"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(run, "nlp.sqlite")
	body, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(body), "\n"), "\n")
	if len(lines) < 2 || lines[0] != "keep restore nlp "+path {
		t.Fatalf("nebula ran %q", body)
	}
	for _, line := range lines[1:] {
		if line != "keep mark nlp "+path {
			t.Fatalf("nebula ran %q", body)
		}
	}
}
