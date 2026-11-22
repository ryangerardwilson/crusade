package state

import (
	"fmt"
	"strings"

	"github.com/ryangerardwilson/crusade/internal/sqlite"
)

// schemaVersion is the user_version of an nlp file with these tables.
const schemaVersion = 2

// nlpTables is the schema. Every nullable column states what null means.
const nlpTables = `
-- One course the learner can practise.
CREATE TABLE course (
  id INTEGER PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE CHECK (trim(slug) <> ''),
  -- the course markdown; its title line is the course name
  body TEXT NOT NULL,
  -- 1: the course is the copy bundled with the app and follows new versions
  bundled INTEGER NOT NULL CHECK (bundled IN (0, 1))
);

-- A bundled course the learner deleted, so a refresh does not bring it back.
CREATE TABLE retired_course (
  slug TEXT PRIMARY KEY CHECK (trim(slug) <> '')
);

-- The learner on this machine. There is one row.
CREATE TABLE learner (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  -- null: the bundled courses were never copied in
  bundled_version TEXT,
  -- null with place_cursor: no session list place is saved
  place_path TEXT,
  -- null with place_path: no session list place is saved
  place_cursor TEXT,
  CHECK ((place_path IS NULL) = (place_cursor IS NULL))
);
INSERT INTO learner(id) VALUES (1);
`

func nlpSchema() sqlite.Schema {
	return sqlite.Schema{Name: databaseName, Version: schemaVersion, Tables: nlpTables, Migrate: migrateLegacy}
}

// migrateLegacy moves a file of the old schema (meta and course) to version 2
// in the one transaction sqlite.Prepare opens; a failed statement rolls it
// all back and the old tables stay.
//
// Backfill: courses are copied without the name column, which only repeated
// the body's title line; the last row of a repeated slug wins. The
// meta app_version becomes learner.bundled_version when imported_courses was
// set. place_path and place_cursor move to learner; either one set saves
// both. The newline list retired_courses becomes retired_course rows. The
// bookmark and app_once tables are dropped.
//
// Rollback: restore the file from its backup.
func migrateLegacy(path string) (string, error) {
	meta, err := sqlite.Columns(path, "meta")
	if err != nil {
		return "", err
	}
	course, err := sqlite.Columns(path, "course")
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if len(course) > 0 {
		b.WriteString("ALTER TABLE course RENAME TO course_v1;\n")
	}
	b.WriteString(nlpTables)
	if len(course) > 0 {
		b.WriteString("INSERT INTO course(slug, body, bundled) SELECT slug, body, CASE WHEN bundled <> 0 THEN 1 ELSE 0 END " +
			"FROM course_v1 c WHERE c.id = (SELECT MAX(c2.id) FROM course_v1 c2 WHERE c2.slug = c.slug);\nDROP TABLE course_v1;\n")
	}
	if len(meta) > 0 {
		retired, err := legacyRetired(path)
		if err != nil {
			return "", err
		}
		b.WriteString(retired)
		b.WriteString(migrateMetaSQL + "DROP TABLE meta;\n")
	}
	b.WriteString("DROP TABLE IF EXISTS bookmark;\nDROP TABLE IF EXISTS app_once;\n")
	return b.String(), nil
}

func metaSelect(key string) string {
	return "(SELECT value FROM meta WHERE key = '" + key + "' ORDER BY id DESC LIMIT 1)"
}

var migrateMetaSQL = "UPDATE learner SET bundled_version = CASE WHEN EXISTS (SELECT 1 FROM meta WHERE key = 'imported_courses') THEN COALESCE(" + metaSelect("app_version") + ", '') END, " +
	"place_path = CASE WHEN COALESCE(" + metaSelect("place_path") + ", '') <> '' OR COALESCE(" + metaSelect("place_cursor") + ", '') <> '' THEN COALESCE(" + metaSelect("place_path") + ", '') END, " +
	"place_cursor = CASE WHEN COALESCE(" + metaSelect("place_path") + ", '') <> '' OR COALESCE(" + metaSelect("place_cursor") + ", '') <> '' THEN COALESCE(" + metaSelect("place_cursor") + ", '') END " +
	"WHERE id = 1;\n"

func legacyRetired(path string) (string, error) {
	var rows []struct {
		Value *string `json:"value"`
	}
	if err := sqlite.Query(path, "SELECT value FROM meta WHERE key = 'retired_courses' ORDER BY id DESC LIMIT 1", &rows); err != nil {
		return "", err
	}
	if len(rows) == 0 || rows[0].Value == nil {
		return "", nil
	}
	var b strings.Builder
	for _, slug := range strings.Split(*rows[0].Value, "\n") {
		if strings.TrimSpace(slug) != "" {
			fmt.Fprintf(&b, "INSERT OR IGNORE INTO retired_course(slug) VALUES (%s);\n", sqlite.Quote(slug))
		}
	}
	return b.String(), nil
}
