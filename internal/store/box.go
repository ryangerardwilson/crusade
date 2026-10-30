package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ryangerardwilson/crusade/internal/box"
	"github.com/ryangerardwilson/crusade/internal/model"
)

const boxesImportedKey = "boxes_imported"

const boxModel = "model box\n" +
	"  id integer primary key\n" +
	"  path text not null\n" +
	"  label text not null\n" +
	"  parent_path text not null\n" +
	"  position integer not null\n" +
	"  depth integer not null\n"

const boxCheckModel = "model box_check\n" +
	"  id integer primary key\n" +
	"  day text not null\n" +
	"  path text not null\n"

const metaModel = "model meta\n" +
	"  id integer primary key\n" +
	"  key text not null\n" +
	"  value text\n"

const createBox = `CREATE TABLE IF NOT EXISTS box (
  "id" INTEGER PRIMARY KEY,
  "path" TEXT NOT NULL,
  "label" TEXT NOT NULL,
  "parent_path" TEXT NOT NULL,
  "position" INTEGER NOT NULL,
  "depth" INTEGER NOT NULL
)`

const createBoxCheck = `CREATE TABLE IF NOT EXISTS box_check (
  "id" INTEGER PRIMARY KEY,
  "day" TEXT NOT NULL,
  "path" TEXT NOT NULL
)`

const createMeta = `CREATE TABLE IF NOT EXISTS meta (
  "id" INTEGER PRIMARY KEY,
  "key" TEXT NOT NULL,
  "value" TEXT
)`

func ormText() string {
	return taskModel + "\n" + boxModel + "\n" + boxCheckModel + "\n" + metaModel
}

func ormUpsertSQL() string {
	models := []struct {
		name string
		body string
		pos  int
	}{
		{"task", taskModel, 0},
		{"box", boxModel, 1},
		{"box_check", boxCheckModel, 2},
		{"meta", metaModel, 3},
	}
	var b strings.Builder
	for _, model := range models {
		fmt.Fprintf(&b, "INSERT INTO sqlx_orm(model, body, position) VALUES (%s, %s, %d) ON CONFLICT(model) DO UPDATE SET body = excluded.body, position = excluded.position;\n", sqlString(model.name), sqlString(model.body), model.pos)
	}
	return b.String()
}

func adoptLegacyDatabase(dbPath string) error {
	if filepath.Base(dbPath) != DatabaseName+".sqlite" {
		return nil
	}
	if _, err := os.Stat(dbPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	legacy := filepath.Join(filepath.Dir(dbPath), legacyDatabaseName+".sqlite")
	if _, err := os.Stat(legacy); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		from := legacy + suffix
		if _, err := os.Stat(from); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		if err := os.Rename(from, dbPath+suffix); err != nil {
			return err
		}
	}
	dataDir := filepath.Dir(dbPath)
	if filepath.Base(dataDir) != "data" {
		return nil
	}
	ormDir := filepath.Join(filepath.Dir(dataDir), "orm")
	from := filepath.Join(ormDir, legacyDatabaseName)
	to := filepath.Join(ormDir, DatabaseName)
	if _, err := os.Stat(from); err != nil {
		return nil
	}
	if _, err := os.Stat(to); err == nil {
		return nil
	}
	return os.Rename(from, to)
}

func importBoxesOnce(dbPath string) error {
	if os.Getenv("BOXES_CONFIG") == "" && dbPath != DatabasePath() {
		return nil
	}
	if err := ensure(dbPath); err != nil {
		return err
	}
	value, ok, err := metaValue(dbPath, boxesImportedKey)
	if err != nil {
		return err
	}
	if ok && value == "1" {
		return nil
	}
	path := outlineSourcePath()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	outline, err := box.ParseOutline(string(data))
	if err != nil {
		return err
	}
	if err := saveOutlineRows(dbPath, outline); err != nil {
		return err
	}
	if err := importDayFiles(dbPath); err != nil {
		return err
	}
	return setMeta(dbPath, boxesImportedKey, "1")
}

func outlineSourcePath() string {
	if path := strings.TrimSpace(os.Getenv("BOXES_CONFIG")); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	settingsPath := filepath.Join(home, ".config", "boxes", "config.json")
	if data, err := os.ReadFile(settingsPath); err == nil {
		var settings struct {
			BoxesPath string `json:"boxes_path"`
		}
		if json.Unmarshal(data, &settings) == nil && strings.TrimSpace(settings.BoxesPath) != "" {
			return settings.BoxesPath
		}
	}
	return filepath.Join(home, "Documents", "notes", "rituals.txt")
}

func dayStateDir() string {
	if path := strings.TrimSpace(os.Getenv("BOXES_DATA_HOME")); path != "" {
		return filepath.Join(path, "days")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "boxes", "days")
}

func importDayFiles(dbPath string) error {
	dir := dayStateDir()
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		day := strings.TrimSuffix(name, ".json")
		if _, err := time.Parse(time.DateOnly, day); err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		var state struct {
			CheckedIDs []string `json:"checked_ids"`
		}
		if err := json.Unmarshal(data, &state); err != nil {
			return fmt.Errorf("read boxes day %s: %w", day, err)
		}
		checked := map[string]bool{}
		for _, id := range state.CheckedIDs {
			if strings.TrimSpace(id) != "" {
				checked[id] = true
			}
		}
		if err := SaveDayChecks(dbPath, day, checked); err != nil {
			return err
		}
	}
	return nil
}

type boxRow struct {
	Path     string
	Label    string
	Parent   string
	Position int
	Depth    int
}

func LoadOutline(dbPath string) (box.Outline, error) {
	if err := ensure(dbPath); err != nil {
		return box.Outline{}, err
	}
	var rows []struct {
		Path     string `json:"path"`
		Label    string `json:"label"`
		Parent   string `json:"parent_path"`
		Position int    `json:"position"`
		Depth    int    `json:"depth"`
	}
	if err := queryJSON(dbPath, "SELECT path, label, parent_path, position, depth FROM box", &rows); err != nil {
		return box.Outline{}, err
	}
	nodes := make([]boxRow, 0, len(rows))
	for _, row := range rows {
		nodes = append(nodes, boxRow{Path: row.Path, Label: row.Label, Parent: row.Parent, Position: row.Position, Depth: row.Depth})
	}
	outline := box.Outline{Items: boxChildren(nodes, "")}
	if err := outline.Validate(); err != nil {
		return box.Outline{}, err
	}
	return outline, nil
}

func boxChildren(nodes []boxRow, parent string) []box.Item {
	var kids []boxRow
	for _, node := range nodes {
		if node.Parent == parent {
			kids = append(kids, node)
		}
	}
	sort.SliceStable(kids, func(i, j int) bool {
		if kids[i].Position == kids[j].Position {
			return kids[i].Path < kids[j].Path
		}
		return kids[i].Position < kids[j].Position
	})
	items := make([]box.Item, 0, len(kids))
	for _, kid := range kids {
		items = append(items, box.Item{
			ID:       kid.Path,
			Label:    kid.Label,
			Children: boxChildren(nodes, kid.Path),
		})
	}
	return items
}

func SaveOutline(dbPath string, outline box.Outline) error {
	if err := outline.Validate(); err != nil {
		return err
	}
	if err := ensure(dbPath); err != nil {
		return err
	}
	if err := saveOutlineRows(dbPath, outline); err != nil {
		return err
	}
	return setMeta(dbPath, boxesImportedKey, "1")
}

func saveOutlineRows(dbPath string, outline box.Outline) error {
	rows := flattenOutline(outline)
	keep := map[string]bool{}
	for _, row := range rows {
		keep[row.Path] = true
	}
	var b strings.Builder
	b.WriteString("BEGIN;\nDELETE FROM box;\n")
	next, err := nextTableID(dbPath, "box")
	if err != nil {
		return err
	}
	for _, row := range rows {
		fmt.Fprintf(&b, "INSERT INTO box(id, path, label, parent_path, position, depth) VALUES (%d, %s, %s, %s, %d, %d);\n", next, sqlString(row.Path), sqlString(row.Label), sqlString(row.Parent), row.Position, row.Depth)
		next++
	}
	var checks []struct {
		Path string `json:"path"`
	}
	if err := queryJSON(dbPath, "SELECT DISTINCT path FROM box_check", &checks); err != nil {
		return err
	}
	for _, check := range checks {
		if !keep[check.Path] {
			fmt.Fprintf(&b, "DELETE FROM box_check WHERE path = %s;\n", sqlString(check.Path))
		}
	}
	b.WriteString("COMMIT;\n")
	return execSQL(dbPath, b.String())
}

func flattenOutline(outline box.Outline) []boxRow {
	var rows []boxRow
	var walk func(items []box.Item, parent string, depth int)
	walk = func(items []box.Item, parent string, depth int) {
		for index, item := range items {
			rows = append(rows, boxRow{Path: item.ID, Label: item.Label, Parent: parent, Position: index, Depth: depth})
			walk(item.Children, item.ID, depth+1)
		}
	}
	walk(outline.Items, "", 0)
	return rows
}

func LoadActiveDays(dbPath string) (map[string]bool, error) {
	if err := ensure(dbPath); err != nil {
		return nil, err
	}
	var rows []struct {
		Day string `json:"day"`
	}
	if err := queryJSON(dbPath, "SELECT DISTINCT day FROM box_check", &rows); err != nil {
		return nil, err
	}
	days := map[string]bool{}
	for _, row := range rows {
		if strings.TrimSpace(row.Day) != "" {
			days[row.Day] = true
		}
	}
	return days, nil
}

func LoadCheckCounts(dbPath string) (map[string]int, error) {
	if err := ensure(dbPath); err != nil {
		return nil, err
	}
	var rows []struct {
		Day string `json:"day"`
		N   int    `json:"n"`
	}
	if err := queryJSON(dbPath, "SELECT day, COUNT(*) AS n FROM box_check GROUP BY day", &rows); err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, row := range rows {
		if strings.TrimSpace(row.Day) != "" && row.N > 0 {
			counts[row.Day] = row.N
		}
	}
	return counts, nil
}

func LoadDayChecks(dbPath, day string) (map[string]bool, error) {
	if err := ensure(dbPath); err != nil {
		return nil, err
	}
	var rows []struct {
		Path string `json:"path"`
	}
	if err := queryJSON(dbPath, "SELECT path FROM box_check WHERE day = "+sqlString(day), &rows); err != nil {
		return nil, err
	}
	checked := map[string]bool{}
	for _, row := range rows {
		checked[row.Path] = true
	}
	return checked, nil
}

func SaveDayChecks(dbPath, day string, checked map[string]bool) error {
	if err := ensure(dbPath); err != nil {
		return err
	}
	next, err := nextTableID(dbPath, "box_check")
	if err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "BEGIN;\nDELETE FROM box_check WHERE day = %s;\n", sqlString(day))
	paths := make([]string, 0, len(checked))
	for path, on := range checked {
		if on && strings.TrimSpace(path) != "" {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		fmt.Fprintf(&b, "INSERT INTO box_check(id, day, path) VALUES (%d, %s, %s);\n", next, sqlString(day), sqlString(path))
		next++
	}
	b.WriteString("COMMIT;\n")
	return execSQL(dbPath, b.String())
}

func metaValue(dbPath, key string) (string, bool, error) {
	var rows []struct {
		Value *string `json:"value"`
	}
	if err := queryJSON(dbPath, "SELECT value FROM meta WHERE key = "+sqlString(key)+" LIMIT 1", &rows); err != nil {
		return "", false, err
	}
	if len(rows) == 0 || rows[0].Value == nil {
		return "", len(rows) > 0, nil
	}
	return *rows[0].Value, true, nil
}

func setMeta(dbPath, key, value string) error {
	var rows []struct {
		ID int `json:"id"`
	}
	if err := queryJSON(dbPath, "SELECT id FROM meta WHERE key = "+sqlString(key)+" LIMIT 1", &rows); err != nil {
		return err
	}
	if len(rows) > 0 {
		return execSQL(dbPath, "UPDATE meta SET value = "+sqlString(value)+" WHERE id = "+fmt.Sprint(rows[0].ID))
	}
	next, err := nextTableID(dbPath, "meta")
	if err != nil {
		return err
	}
	return execSQL(dbPath, fmt.Sprintf("INSERT INTO meta(id, key, value) VALUES (%d, %s, %s)", next, sqlString(key), sqlString(value)))
}

const lastModeKey = "last_mode"

// LastMode is the mode the next launch opens. A missing or unknown value is do.
func LastMode(dbPath string) (string, error) {
	value, ok, err := metaValue(dbPath, lastModeKey)
	if err != nil {
		return "", err
	}
	if !ok || !knownMode(value) {
		return model.ModeDo, nil
	}
	return value, nil
}

// SaveLastMode stores the mode the next launch opens.
func SaveLastMode(dbPath, mode string) error {
	if !knownMode(mode) {
		return fmt.Errorf("unknown mode %s", mode)
	}
	if err := ensure(dbPath); err != nil {
		return err
	}
	return setMeta(dbPath, lastModeKey, mode)
}

func knownMode(mode string) bool {
	switch mode {
	case model.ModeDo, model.ModeAlign, model.ModeBox:
		return true
	default:
		return false
	}
}

func nextTableID(dbPath, table string) (int, error) {
	switch table {
	case "box", "box_check", "meta", "task":
	default:
		return 0, fmt.Errorf("unknown table %s", table)
	}
	var rows []struct {
		N int `json:"n"`
	}
	if err := queryJSON(dbPath, "SELECT COALESCE(MAX(id), 0) AS n FROM "+table, &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 1, nil
	}
	return rows[0].N + 1, nil
}
