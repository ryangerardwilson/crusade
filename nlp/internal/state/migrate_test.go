package state

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryangerardwilson/crusade/internal/sqlite"
)

// legacyNLP is an nlp file before schema version 2: meta and course.
const legacyNLP = `CREATE TABLE meta (
  id INTEGER PRIMARY KEY,
  key TEXT NOT NULL UNIQUE,
  value TEXT NOT NULL
);
INSERT INTO meta(id, key, value) VALUES
  (1, 'imported_courses', '1'),
  (2, 'app_version', 'v1'),
  (3, 'place_path', 'a/b'),
  (4, 'place_cursor', '3'),
  (5, 'retired_courses', 'x
y');
CREATE TABLE course (
  id INTEGER PRIMARY KEY,
  slug TEXT NOT NULL,
  name TEXT NOT NULL,
  body TEXT NOT NULL,
  bundled INTEGER NOT NULL
);
INSERT INTO course(id, slug, name, body, bundled) VALUES (1, 'custom', 'Custom', '# Custom

## Lesson

    echo ok
', 0);
`

// seedLegacyNLP points the package at a temp SQLX_ROOT and writes the old
// file where openDB looks for it.
func seedLegacyNLP(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SQLX_ROOT", filepath.Join(home, "sqlx"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	db := filepath.Join(home, "sqlx", "data", "nlp.sqlite")
	if err := os.MkdirAll(filepath.Dir(db), 0o700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("/usr/bin/sqlite3", db, legacyNLP).CombinedOutput(); err != nil {
		t.Fatalf("seed: %v\n%s", err, out)
	}
	return db
}

func learnerVersion(t *testing.T, db string) string {
	t.Helper()
	var rows []struct {
		Version *string `json:"bundled_version"`
	}
	if err := sqlite.Query(db, "SELECT bundled_version FROM learner WHERE id = 1", &rows); err != nil || len(rows) != 1 || rows[0].Version == nil {
		t.Fatalf("learner = %#v %v", rows, err)
	}
	return *rows[0].Version
}

func TestOpenMigratesTheLegacyNLPFile(t *testing.T) {
	db := seedLegacyNLP(t)
	path, cursor, ok, err := LoadPlace()
	if err != nil || !ok || path != "a/b" || cursor != "3" {
		t.Fatalf("place = %q %q %v %v", path, cursor, ok, err)
	}
	if version, err := sqlite.UserVersion(db); err != nil || version != schemaVersion {
		t.Fatalf("user_version = %d %v", version, err)
	}
	if got := learnerVersion(t, db); got != "v1" {
		t.Fatalf("bundled_version = %q", got)
	}
	var tables []struct {
		Name string `json:"name"`
	}
	if err := sqlite.Query(db, "SELECT name FROM sqlite_master WHERE type = 'table' AND name IN ('meta', 'course_v1')", &tables); err != nil || len(tables) != 0 {
		t.Fatalf("old tables remain: %#v %v", tables, err)
	}
	var retired []struct {
		Slug string `json:"slug"`
	}
	if err := sqlite.Query(db, "SELECT slug FROM retired_course ORDER BY slug", &retired); err != nil || len(retired) != 2 || retired[0].Slug != "x" || retired[1].Slug != "y" {
		t.Fatalf("retired = %#v %v", retired, err)
	}
	courses, err := LoadCourses("v2")
	if err != nil || !hasCourse(courses, "Custom") {
		t.Fatalf("courses = %v %v", names(courses), err)
	}
	if got := learnerVersion(t, db); got != "v2" {
		t.Fatalf("refreshed bundled_version = %q", got)
	}
	for _, course := range courses {
		if strings.EqualFold(course.SourceFile, "x") || strings.EqualFold(course.SourceFile, "y") {
			t.Fatalf("retired course came back: %v", names(courses))
		}
	}
}
