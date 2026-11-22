package state

import (
	"github.com/ryangerardwilson/crusade/internal/sqlite"

	"fmt"
)

// SaveCourse stores the edited body of slug. name is the title the editor
// shows; the body's title line carries it.
func SaveCourse(slug, name, body string) error {
	dbPath, err := openDB()
	if err != nil {
		return err
	}
	return upsertCourse(dbPath, slug, body, 0, true)
}

func CreateCourse(name, body string) (string, error) {
	dbPath, err := openDB()
	if err != nil {
		return "", err
	}
	slug := slugFor(name)
	_, ok, err := courseBySlug(dbPath, slug)
	if err != nil {
		return "", err
	}
	if ok {
		return "", fmt.Errorf("course already exists")
	}
	if err := upsertCourse(dbPath, slug, body, 0, true); err != nil {
		return "", err
	}
	return slug, nil
}

func DeleteStoredCourse(slug string) error {
	dbPath, err := openDB()
	if err != nil {
		return err
	}
	embedded, err := readEmbedded()
	if err != nil {
		return err
	}
	var statements []string
	for _, item := range embedded {
		if item.Slug == slug {
			statements = append(statements, "INSERT OR IGNORE INTO retired_course(slug) VALUES ("+sqlite.Quote(slug)+");\n")
			break
		}
	}
	statements = append(statements, "DELETE FROM course WHERE slug = "+sqlite.Quote(slug)+";\n")
	return execSQL(dbPath, inTx(statements...))
}

// slugRetired reports that the person deleted the bundled course slug, so a
// refresh does not bring it back.
func slugRetired(dbPath, slug string) (bool, error) {
	var rows []struct {
		N int `json:"n"`
	}
	if err := sqlite.Query(dbPath, "SELECT COUNT(*) AS n FROM retired_course WHERE slug = "+sqlite.Quote(slug), &rows); err != nil {
		return false, err
	}
	return len(rows) > 0 && rows[0].N > 0, nil
}
