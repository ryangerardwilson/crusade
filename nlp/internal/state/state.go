package state

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ryangerardwilson/crusade/internal/sqlite"
	root "github.com/ryangerardwilson/crusade/nlp/bundle"
	"github.com/ryangerardwilson/crusade/nlp/internal/course"
)

const databaseName = "nlp"

type courseRow struct {
	ID      int    `json:"id"`
	Slug    string `json:"slug"`
	Body    string `json:"body"`
	Bundled int    `json:"bundled"`
}

type embeddedCourse struct {
	Slug string
	Body string
}

type legacyConfig struct {
	Courses []struct {
		LocalPath string `json:"local_path"`
	} `json:"courses"`
}

func LoadCourses(version string) ([]course.Course, error) {
	dbPath, err := openDB()
	if err != nil {
		return nil, err
	}
	if err := syncCourses(dbPath, version); err != nil {
		return nil, err
	}
	var rows []courseRow
	if err := sqlite.Query(dbPath, "SELECT id, slug, body, bundled FROM course", &rows); err != nil {
		return nil, err
	}
	courses := make([]course.Course, 0, len(rows))
	for _, row := range rows {
		parsed, ok := course.ParseMarkdown(row.Slug, row.Body)
		if !ok {
			continue
		}
		parsed.SourceFile = row.Slug
		parsed.Body = row.Body
		courses = append(courses, parsed)
	}
	sort.Slice(courses, func(i, j int) bool {
		return strings.ToLower(courses[i].Name) < strings.ToLower(courses[j].Name)
	})
	return courses, nil
}

func openDB() (string, error) {
	path := sqlite.File(databaseName)
	if err := ensure(path); err != nil {
		return "", err
	}
	return path, nil
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "."
	}
	return home
}

func configDir(app string) string {
	if base := os.Getenv("XDG_CONFIG_HOME"); base != "" {
		return filepath.Join(expandPath(base), app)
	}
	return filepath.Join(homeDir(), ".config", app)
}

func syncCourses(dbPath, version string) error {
	embedded, err := readEmbedded()
	if err != nil {
		return err
	}
	stored, imported, err := bundledVersion(dbPath)
	if err != nil {
		return err
	}
	if !imported {
		for _, item := range embedded {
			if err := upsertCourse(dbPath, item.Slug, item.Body, 1, true); err != nil {
				return err
			}
		}
		if err := importLegacyCourses(dbPath, embedded); err != nil {
			return err
		}
		return setBundledVersion(dbPath, version)
	}
	refresh := stored != version
	for _, item := range embedded {
		if err := refreshBundled(dbPath, item, refresh); err != nil {
			return err
		}
	}
	if refresh {
		return setBundledVersion(dbPath, version)
	}
	return nil
}

// bundledVersion is the app version whose bundled courses were last copied.
// imported is false when they never were.
func bundledVersion(dbPath string) (string, bool, error) {
	var rows []struct {
		Version *string `json:"bundled_version"`
	}
	if err := sqlite.Query(dbPath, "SELECT bundled_version FROM learner WHERE id = 1", &rows); err != nil {
		return "", false, err
	}
	if len(rows) == 0 || rows[0].Version == nil {
		return "", false, nil
	}
	return *rows[0].Version, true, nil
}

func setBundledVersion(dbPath, version string) error {
	return execSQL(dbPath, "UPDATE learner SET bundled_version = "+sqlite.Quote(version)+" WHERE id = 1;\n")
}

func refreshBundled(dbPath string, item embeddedCourse, refresh bool) error {
	retired, err := slugRetired(dbPath, item.Slug)
	if err != nil {
		return err
	}
	if retired {
		return nil
	}
	row, ok, err := courseBySlug(dbPath, item.Slug)
	if err != nil {
		return err
	}
	if !ok {
		return upsertCourse(dbPath, item.Slug, item.Body, 1, true)
	}
	if refresh && row.Bundled == 1 {
		return upsertCourse(dbPath, item.Slug, item.Body, 1, true)
	}
	return nil
}

func readEmbedded() ([]embeddedCourse, error) {
	entries, err := fs.ReadDir(root.CourseFiles, "courses")
	if err != nil {
		return nil, err
	}
	items := []embeddedCourse{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		body, err := fs.ReadFile(root.CourseFiles, filepath.ToSlash(filepath.Join("courses", entry.Name())))
		if err != nil {
			return nil, err
		}
		if _, ok := course.ParseMarkdown(entry.Name(), string(body)); !ok {
			continue
		}
		items = append(items, embeddedCourse{Slug: slugFor(entry.Name()), Body: string(body)})
	}
	return items, nil
}

func importLegacyCourses(dbPath string, embedded []embeddedCourse) error {
	bundled := map[string]bool{}
	for _, item := range embedded {
		bundled[item.Slug] = true
	}
	path := filepath.Join(configDir("rt"), "config.json")
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var cfg legacyConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		return err
	}
	for _, entry := range cfg.Courses {
		coursePath := expandPath(strings.TrimSpace(entry.LocalPath))
		if coursePath == "" {
			continue
		}
		fileBody, err := os.ReadFile(coursePath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if _, ok := course.ParseMarkdown(filepath.Base(coursePath), string(fileBody)); !ok {
			continue
		}
		slug := slugFor(filepath.Base(coursePath))
		flag := 0
		if bundled[slug] {
			flag = 1
		}
		if err := upsertCourse(dbPath, slug, string(fileBody), flag, true); err != nil {
			return err
		}
	}
	return nil
}

func courseBySlug(dbPath, slug string) (courseRow, bool, error) {
	var rows []courseRow
	if err := sqlite.Query(dbPath, "SELECT id, slug, body, bundled FROM course WHERE slug = "+sqlite.Quote(slug), &rows); err != nil {
		return courseRow{}, false, err
	}
	if len(rows) == 0 {
		return courseRow{}, false, nil
	}
	return rows[0], true, nil
}

// upsertCourse stores body under slug. A course's name is the title line of
// its body, so it is not stored apart.
func upsertCourse(dbPath, slug, body string, bundled int, overwrite bool) error {
	row, ok, err := courseBySlug(dbPath, slug)
	if err != nil {
		return err
	}
	if !ok {
		sql := fmt.Sprintf("INSERT INTO course(slug, body, bundled) VALUES (%s, %s, %d);\n",
			sqlite.Quote(slug), sqlite.Quote(body), bundled)
		return execSQL(dbPath, sql)
	}
	if !overwrite {
		return nil
	}
	sql := fmt.Sprintf("UPDATE course SET body = %s, bundled = %d WHERE id = %d;\n",
		sqlite.Quote(body), bundled, row.ID)
	return execSQL(dbPath, sql)
}

func slugFor(name string) string {
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	base = strings.TrimPrefix(base, "course_")
	base = strings.ToLower(strings.TrimSpace(base))
	base = strings.ReplaceAll(base, "_", "-")
	base = strings.ReplaceAll(base, " ", "-")
	if base == "" {
		return "untitled"
	}
	return base
}

// SavePlace stores the session list place in the nlp database.
func SavePlace(path, cursor string) error {
	dbPath, err := openDB()
	if err != nil {
		return err
	}
	return execSQL(dbPath, fmt.Sprintf("UPDATE learner SET place_path = %s, place_cursor = %s WHERE id = 1;\n", sqlite.Quote(path), sqlite.Quote(cursor)))
}

// LoadPlace reads the session list place. ok is false when none is stored.
func LoadPlace() (string, string, bool, error) {
	dbPath, err := openDB()
	if err != nil {
		return "", "", false, err
	}
	var rows []struct {
		Path   *string `json:"place_path"`
		Cursor *string `json:"place_cursor"`
	}
	if err := sqlite.Query(dbPath, "SELECT place_path, place_cursor FROM learner WHERE id = 1", &rows); err != nil {
		return "", "", false, err
	}
	if len(rows) == 0 || rows[0].Path == nil || rows[0].Cursor == nil {
		return "", "", false, nil
	}
	return *rows[0].Path, *rows[0].Cursor, true, nil
}

// inTx wraps statements in one transaction. sqliteRun bails at the first
// failed statement, so nothing before it commits.
func inTx(statements ...string) string {
	return "BEGIN IMMEDIATE;\n" + strings.Join(statements, "") + "COMMIT;\n"
}

func ensure(dbPath string) error {
	return sqlite.Prepare(dbPath, nlpSchema())
}

func expandPath(path string) string {
	if path == "" {
		return path
	}
	if path == "~" || (len(path) > 2 && strings.HasPrefix(path, "~/")) {
		home := homeDir()
		if home == "." {
			return path
		}
		if path == "~" {
			return home
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// execSQL writes the nlp sqlite and tells nebula keep it changed unless it
// holds no course. The learner row is not a course, so a file with none is
// not marked.
func execSQL(dbPath, sql string) error {
	if err := sqlite.Exec(dbPath, sql); err != nil {
		return err
	}
	var rows []struct {
		N int `json:"n"`
	}
	if err := sqlite.Query(dbPath, "SELECT COUNT(*) AS n FROM course", &rows); err == nil && len(rows) > 0 && rows[0].N > 0 {
		sqlite.Mark(dbPath, databaseName)
	}
	return nil
}
