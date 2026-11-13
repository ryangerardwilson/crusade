package state

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	root "github.com/ryangerardwilson/crusade/nlp/bundle"
	"github.com/ryangerardwilson/crusade/nlp/internal/course"
)

const databaseName = "nlp"

const importedCoursesKey = "imported_courses"
const appVersionKey = "app_version"

const metaModel = "model meta\n" +
	"  id integer primary key\n" +
	"  key text not null\n" +
	"  value text\n"

const courseModel = "model course\n" +
	"  id integer primary key\n" +
	"  slug text not null\n" +
	"  name text not null\n" +
	"  body text not null\n" +
	"  bundled integer not null\n"

type courseRow struct {
	ID      int    `json:"id"`
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Body    string `json:"body"`
	Bundled int    `json:"bundled"`
}

type embeddedCourse struct {
	Slug string
	Name string
	Body string
}

type legacyConfig struct {
	Courses []struct {
		Name      string `json:"name"`
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
	if err := queryJSON(dbPath, "SELECT id, slug, name, body, bundled FROM course", &rows); err != nil {
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
	if err := ensure(); err != nil {
		return "", err
	}
	return databaseName, nil
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
	imported, err := metaHas(dbPath, importedCoursesKey)
	if err != nil {
		return err
	}
	if !imported {
		for _, item := range embedded {
			if err := upsertCourse(dbPath, item.Slug, item.Name, item.Body, 1, true); err != nil {
				return err
			}
		}
		if err := importLegacyCourses(dbPath, embedded); err != nil {
			return err
		}
		if err := setMeta(dbPath, importedCoursesKey, "1"); err != nil {
			return err
		}
		return setMeta(dbPath, appVersionKey, version)
	}
	stored, err := metaValue(dbPath, appVersionKey)
	if err != nil {
		return err
	}
	refresh := stored != version
	for _, item := range embedded {
		if err := refreshBundled(dbPath, item, refresh); err != nil {
			return err
		}
	}
	if refresh {
		return setMeta(dbPath, appVersionKey, version)
	}
	return nil
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
		return upsertCourse(dbPath, item.Slug, item.Name, item.Body, 1, true)
	}
	if refresh && row.Bundled == 1 {
		return upsertCourse(dbPath, item.Slug, item.Name, item.Body, 1, true)
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
		parsed, ok := course.ParseMarkdown(entry.Name(), string(body))
		if !ok {
			continue
		}
		items = append(items, embeddedCourse{
			Slug: slugFor(entry.Name()),
			Name: parsed.Name,
			Body: string(body),
		})
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
		parsed, ok := course.ParseMarkdown(filepath.Base(coursePath), string(fileBody))
		if !ok {
			continue
		}
		slug := slugFor(filepath.Base(coursePath))
		flag := 0
		if bundled[slug] {
			flag = 1
		}
		name := parsed.Name
		if name == "" {
			name = strings.TrimSpace(entry.Name)
		}
		if err := upsertCourse(dbPath, slug, name, string(fileBody), flag, true); err != nil {
			return err
		}
	}
	return nil
}

func courseBySlug(dbPath, slug string) (courseRow, bool, error) {
	var rows []courseRow
	if err := queryJSON(dbPath, "SELECT id, slug, name, body, bundled FROM course WHERE slug = "+sqlString(slug), &rows); err != nil {
		return courseRow{}, false, err
	}
	if len(rows) == 0 {
		return courseRow{}, false, nil
	}
	return rows[0], true, nil
}

func upsertCourse(dbPath, slug, name, body string, bundled int, overwrite bool) error {
	row, ok, err := courseBySlug(dbPath, slug)
	if err != nil {
		return err
	}
	if !ok {
		sql := fmt.Sprintf("INSERT INTO course(slug, name, body, bundled) VALUES (%s, %s, %s, %d);\n",
			sqlString(slug), sqlString(name), sqlString(body), bundled)
		return execSQL(dbPath, sql)
	}
	if !overwrite {
		return nil
	}
	sql := fmt.Sprintf("UPDATE course SET name = %s, body = %s, bundled = %d WHERE id = %d;\n",
		sqlString(name), sqlString(body), bundled, row.ID)
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

func metaHas(dbPath, key string) (bool, error) {
	var rows []struct {
		N int `json:"n"`
	}
	if err := queryJSON(dbPath, "SELECT COUNT(*) AS n FROM meta WHERE key = "+sqlString(key), &rows); err != nil {
		return false, err
	}
	if len(rows) == 0 {
		return false, nil
	}
	return rows[0].N > 0, nil
}

func metaValue(dbPath, key string) (string, error) {
	var rows []struct {
		Value *string `json:"value"`
	}
	if err := queryJSON(dbPath, "SELECT value FROM meta WHERE key = "+sqlString(key)+" ORDER BY id DESC LIMIT 1", &rows); err != nil {
		return "", err
	}
	if len(rows) == 0 || rows[0].Value == nil {
		return "", nil
	}
	return *rows[0].Value, nil
}

const placePathKey = "place_path"
const placeCursorKey = "place_cursor"

// SavePlace stores the session list place in the nlp database.
func SavePlace(path, cursor string) error {
	dbPath, err := openDB()
	if err != nil {
		return err
	}
	if err := setMeta(dbPath, placePathKey, path); err != nil {
		return err
	}
	return setMeta(dbPath, placeCursorKey, cursor)
}

// LoadPlace reads the session list place. ok is false when none is stored.
func LoadPlace() (string, string, bool, error) {
	dbPath, err := openDB()
	if err != nil {
		return "", "", false, err
	}
	path, err := metaValue(dbPath, placePathKey)
	if err != nil {
		return "", "", false, err
	}
	cursor, err := metaValue(dbPath, placeCursorKey)
	if err != nil {
		return "", "", false, err
	}
	if path == "" && cursor == "" {
		return "", "", false, nil
	}
	return path, cursor, true, nil
}

func setMeta(dbPath, key, value string) error {
	has, err := metaHas(dbPath, key)
	if err != nil {
		return err
	}
	if has {
		return execSQL(dbPath, "UPDATE meta SET value = "+sqlString(value)+" WHERE key = "+sqlString(key)+";\n")
	}
	return execSQL(dbPath, "INSERT INTO meta(key, value) VALUES ("+sqlString(key)+", "+sqlString(value)+");\n")
}

func ensure() error {
	if _, err := sqlxCall("create", "database", databaseName); err != nil {
		return err
	}
	if _, err := sqlxCall(databaseName, "once", "drop_bookmark", "DROP TABLE IF EXISTS bookmark; DELETE FROM sqlx_orm WHERE model = 'bookmark';"); err != nil {
		return err
	}
	return sqlxMigrate(ormText())
}

func ormText() string {
	return metaModel + "\n" + courseModel
}

func sqlString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
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

func queryJSON(dbPath, sql string, dest any) error {
	_ = dbPath
	raw, err := sqlxCall(databaseName, "query", "--json", sql)
	if err != nil {
		return err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), dest); err != nil {
		return fmt.Errorf("sqlite json: %w", err)
	}
	return nil
}

func execSQL(dbPath, sql string) error {
	_ = dbPath
	file, err := os.CreateTemp("", databaseName+"-sql-*")
	if err != nil {
		return err
	}
	sqlPath := file.Name()
	defer os.Remove(sqlPath)
	if _, err := file.WriteString(sql); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	_, err = sqlxCall(databaseName, "exec", "--file", sqlPath)
	return err
}

func sqlxBin() string {
	if bin := os.Getenv("SQLX_BIN"); bin != "" {
		return bin
	}
	path, err := exec.LookPath("sqlx")
	if err != nil {
		return "sqlx"
	}
	return path
}

func sqlxCall(args ...string) (string, error) {
	cmd := exec.Command(sqlxBin(), args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), fmt.Errorf("%s", msg)
	}
	return stdout.String(), nil
}

func sqlxMigrate(body string) error {
	file, err := os.CreateTemp("", databaseName+"-orm-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err := file.WriteString(body); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	_, err = sqlxCall(databaseName, "migrate", name)
	return err
}
