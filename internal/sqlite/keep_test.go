package sqlite

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var demoSchema = Schema{
	Name:    "demo",
	Version: 1,
	Tables:  "CREATE TABLE note (id INTEGER PRIMARY KEY, body TEXT NOT NULL);\n",
	Migrate: func(string) (string, error) { return "", errors.New("no migration") },
}

// fakeNebula installs a nebula that logs its arguments. Its restore makes an
// empty file, as keep does when no backup exists, or, with KEEP_FAKE_DOWN
// set, fails and makes none, as keep does when the backup is unreachable.
func fakeNebula(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "nebula.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + logPath + "'\n" +
		"[ \"$2\" = restore ] || exit 0\n" +
		"if [ -n \"$KEEP_FAKE_DOWN\" ]; then echo \"restore s3://backups/$3.sqlite: connection timed out\" >&2; exit 1; fi\n" +
		"umask 077\n: > \"$4\"\necho \"fresh $4\"\n"
	bin := filepath.Join(dir, "nebula")
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NEBULA_BIN", bin)
	t.Setenv("KEEP_FAKE_DOWN", "")
	return logPath
}

// runtimeEnv puts File in a temp runtime dir, the path a running machine
// backs up.
func runtimeEnv(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("SQLX_ROOT", "")
	t.Setenv("SQLX_RUNTIME", filepath.Join(dir, "run"))
	t.Setenv("XDG_RUNTIME_DIR", dir)
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestKeepRestoresMissingFileAndMarksWrites(t *testing.T) {
	runtimeEnv(t)
	logPath := fakeNebula(t)
	path := File("demo")
	if err := Prepare(path, demoSchema); err != nil {
		t.Fatal(err)
	}
	if err := Exec(path, "INSERT INTO note(body) VALUES ('kept');\n"); err != nil {
		t.Fatal(err)
	}
	Mark(path, "demo")
	if err := Prepare(path, demoSchema); err != nil {
		t.Fatal(err)
	}
	want := "keep restore demo " + path + "\nkeep mark demo " + path + "\n"
	if got := readLog(t, logPath); got != want {
		t.Fatalf("nebula ran %q, want %q", got, want)
	}
}

func TestUnreachableBackupFailsAndMakesNoFile(t *testing.T) {
	runtimeEnv(t)
	logPath := fakeNebula(t)
	t.Setenv("KEEP_FAKE_DOWN", "1")
	path := File("demo")
	err := Prepare(path, demoSchema)
	if err == nil || !strings.Contains(err.Error(), "connection timed out") {
		t.Fatalf("Prepare = %v, want the restore failure", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("a failed restore left %s: %v", path, err)
	}
	if got := readLog(t, logPath); got != "keep restore demo "+path+"\n" {
		t.Fatalf("nebula ran %q", got)
	}
}

func TestSQLXRootNeverReachesNebula(t *testing.T) {
	logPath := fakeNebula(t)
	t.Setenv("SQLX_ROOT", t.TempDir())
	path := File("demo")
	if err := Prepare(path, demoSchema); err != nil {
		t.Fatal(err)
	}
	Mark(path, "demo")
	if got := readLog(t, logPath); got != "" {
		t.Fatalf("SQLX_ROOT reached nebula: %q", got)
	}
}

func TestTestBinaryNeedsNebulaBin(t *testing.T) {
	runtimeEnv(t)
	logPath := fakeNebula(t)
	t.Setenv("NEBULA_BIN", "")
	t.Setenv("PATH", filepath.Dir(logPath)+string(os.PathListSeparator)+os.Getenv("PATH"))
	path := File("demo")
	if err := Prepare(path, demoSchema); err != nil {
		t.Fatal(err)
	}
	Mark(path, "demo")
	if got := readLog(t, logPath); got != "" {
		t.Fatalf("a test without NEBULA_BIN reached the nebula on PATH: %q", got)
	}
}
