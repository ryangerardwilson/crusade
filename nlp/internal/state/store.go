package state

import (
	"fmt"
	"strings"
)

// Architecture exception (gates §1.5, §1.11): retired bundled slugs are one
// newline-separated meta value. retireSQL is the only writer and slugRetired
// the only reader. Remove once a retired_course table with a unique slug
// replaces the list.
const retiredCoursesKey = "retired_courses"

func SaveCourse(slug, name, body string) error {
	dbPath, err := openDB()
	if err != nil {
		return err
	}
	return upsertCourse(dbPath, slug, name, body, 0, true)
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
	if err := upsertCourse(dbPath, slug, name, body, 0, true); err != nil {
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
			retire, err := retireSQL(dbPath, slug)
			if err != nil {
				return err
			}
			statements = append(statements, retire)
			break
		}
	}
	statements = append(statements, "DELETE FROM course WHERE slug = "+sqlString(slug)+";\n")
	return execSQL(dbPath, inTx(statements...))
}

func slugRetired(dbPath, slug string) (bool, error) {
	value, err := metaValue(dbPath, retiredCoursesKey)
	if err != nil {
		return false, err
	}
	for _, item := range strings.Split(value, "\n") {
		if item == slug {
			return true, nil
		}
	}
	return false, nil
}

// retireSQL adds slug to the retired list, or is empty when slug is already
// retired.
func retireSQL(dbPath, slug string) (string, error) {
	retired, err := slugRetired(dbPath, slug)
	if err != nil {
		return "", err
	}
	if retired {
		return "", nil
	}
	value, err := metaValue(dbPath, retiredCoursesKey)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) == "" {
		value = slug
	} else {
		value = value + "\n" + slug
	}
	return metaSQL(retiredCoursesKey, value), nil
}
