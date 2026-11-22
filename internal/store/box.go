package store

import (
	"github.com/ryangerardwilson/crusade/internal/sqlite"

	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ryangerardwilson/crusade/internal/box"
)

type boxRow struct {
	ID       int    `json:"id"`
	Parent   *int   `json:"parent_id"`
	Position int    `json:"position"`
	Label    string `json:"label"`
}

// LoadOutline reads the box outline.
func LoadOutline(dbPath string) (box.Outline, error) {
	if err := ensure(dbPath); err != nil {
		return box.Outline{}, err
	}
	var rows []struct {
		ID       int    `json:"id"`
		Parent   *int   `json:"parent_id"`
		Position int    `json:"position"`
		Label    string `json:"label"`
	}
	if err := sqlite.Query(dbPath, "SELECT id, parent_id, position, label FROM box ORDER BY position, id", &rows); err != nil {
		return box.Outline{}, err
	}
	order := map[int][]int{}
	labels := map[int]string{}
	for _, row := range rows {
		parent := 0
		if row.Parent != nil {
			parent = *row.Parent
		}
		order[parent] = append(order[parent], row.ID)
		labels[row.ID] = row.Label
	}
	var build func(parent int) []box.Item
	build = func(parent int) []box.Item {
		items := make([]box.Item, 0, len(order[parent]))
		for _, id := range order[parent] {
			items = append(items, box.Item{ID: id, Label: labels[id], Children: build(id)})
		}
		return items
	}
	return box.Outline{Items: build(0)}, nil
}

// AddBox puts a new box right after selectedID at its level, or first in an
// empty outline, and returns its id.
func AddBox(dbPath string, selectedID int, label string) (int, error) {
	outline, err := LoadOutline(dbPath)
	if err != nil {
		return 0, err
	}
	label = strings.TrimSpace(label)
	parent, position, err := outline.PlaceAfter(selectedID, label)
	if err != nil {
		return 0, err
	}
	sql := "BEGIN IMMEDIATE;\n" +
		fmt.Sprintf("UPDATE box SET position = position + 1 WHERE parent_id IS %s AND position >= %d;\n", parentSQL(parent), position) +
		fmt.Sprintf("INSERT INTO box(parent_id, position, label) VALUES (%s, %d, %s) RETURNING id;\n", parentSQL(parent), position, sqlite.Quote(label)) +
		"COMMIT;\n"
	var rows []struct {
		ID int `json:"id"`
	}
	if err := sqlite.Query(dbPath, sql, &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("%s: sqlite returned no id", dbPath)
	}
	published(dbPath)
	return rows[0].ID, nil
}

// RenameBox changes the label of id. The id and its checks stay.
func RenameBox(dbPath string, id int, label string) error {
	outline, err := LoadOutline(dbPath)
	if err != nil {
		return err
	}
	label = strings.TrimSpace(label)
	if err := outline.CheckRename(id, label); err != nil {
		return err
	}
	return changeOne(dbPath, fmt.Sprintf("UPDATE box SET label = %s WHERE id = %d;\n", sqlite.Quote(label), id), id)
}

// DeleteBox removes id, the boxes under it, and their checks.
func DeleteBox(dbPath string, id int) error {
	if err := ensure(dbPath); err != nil {
		return err
	}
	return changeOne(dbPath, "DELETE FROM box WHERE id = "+strconv.Itoa(id)+";\n", id)
}

// ToggleBox checks every leaf under id on day, or clears them when they are
// all checked, and reports whether they are now checked.
func ToggleBox(dbPath, day string, id int) (bool, error) {
	outline, err := LoadOutline(dbPath)
	if err != nil {
		return false, err
	}
	checked, err := LoadDayChecks(dbPath, day)
	if err != nil {
		return false, err
	}
	leaves, on, err := outline.Toggle(id, checked)
	if err != nil {
		return false, err
	}
	var b strings.Builder
	b.WriteString("BEGIN IMMEDIATE;\n")
	for _, leaf := range leaves {
		if on {
			fmt.Fprintf(&b, "INSERT OR IGNORE INTO box_check(day, box_id) VALUES (%s, %d);\n", sqlite.Quote(day), leaf)
		} else {
			fmt.Fprintf(&b, "DELETE FROM box_check WHERE day = %s AND box_id = %d;\n", sqlite.Quote(day), leaf)
		}
	}
	b.WriteString("COMMIT;\n")
	return on, execSQL(dbPath, b.String())
}

// SaveOutline stores the whole outline the person saved in Vim, in one
// transaction. A box whose label path is unchanged keeps its id and checks.
// A box missing from outline is deleted with its checks.
func SaveOutline(dbPath string, outline box.Outline) error {
	if err := outline.Validate(); err != nil {
		return err
	}
	current, err := LoadOutline(dbPath)
	if err != nil {
		return err
	}
	ids := map[string]int{}
	currentPaths := current.Paths()
	for index, item := range current.Flatten() {
		ids[currentPaths[index]] = item.ID
	}
	next, err := nextBoxID(dbPath)
	if err != nil {
		return err
	}
	rows, kept := outlineRows(outline, ids, next)
	return execSQL(dbPath, outlineSQL(rows, current, kept))
}

// outlineRows gives each box of outline the id of the stored box with its
// label path, or a new id counting from next.
func outlineRows(outline box.Outline, ids map[string]int, next int) ([]boxRow, map[int]bool) {
	kept := map[int]bool{}
	var rows []boxRow
	var walk func(items []box.Item, parent int, path string)
	walk = func(items []box.Item, parent int, path string) {
		for position, item := range items {
			key := box.Key(item.Label)
			if path != "" {
				key = path + "/" + key
			}
			id, ok := ids[key]
			if !ok {
				id = next
				next++
			}
			kept[id] = ok
			row := boxRow{ID: id, Position: position, Label: strings.TrimSpace(item.Label)}
			if parent != 0 {
				p := parent
				row.Parent = &p
			}
			rows = append(rows, row)
			walk(item.Children, id, key)
		}
	}
	walk(outline.Items, 0, "")
	return rows, kept
}

func outlineSQL(rows []boxRow, current box.Outline, kept map[int]bool) string {
	var b strings.Builder
	b.WriteString("BEGIN IMMEDIATE;\nPRAGMA defer_foreign_keys = ON;\n")
	for _, item := range current.Flatten() {
		if !kept[item.ID] {
			fmt.Fprintf(&b, "DELETE FROM box WHERE id = %d;\n", item.ID)
		}
	}
	for _, row := range rows {
		parent := "NULL"
		if row.Parent != nil {
			parent = strconv.Itoa(*row.Parent)
		}
		if kept[row.ID] {
			fmt.Fprintf(&b, "UPDATE box SET parent_id = %s, position = %d, label = %s WHERE id = %d;\n", parent, row.Position, sqlite.Quote(row.Label), row.ID)
		} else {
			fmt.Fprintf(&b, "INSERT INTO box(id, parent_id, position, label) VALUES (%d, %s, %d, %s);\n", row.ID, parent, row.Position, sqlite.Quote(row.Label))
		}
	}
	// A box is checked through its leaves, so a box that now has children
	// keeps no checks of its own.
	b.WriteString("DELETE FROM box_check WHERE box_id IN (SELECT parent_id FROM box WHERE parent_id IS NOT NULL);\n")
	b.WriteString("COMMIT;\n")
	return b.String()
}

func parentSQL(parent int) string {
	if parent == 0 {
		return "NULL"
	}
	return strconv.Itoa(parent)
}

func nextBoxID(dbPath string) (int, error) {
	var rows []struct {
		N int `json:"n"`
	}
	if err := sqlite.Query(dbPath, "SELECT COALESCE(MAX(id), 0) + 1 AS n FROM box", &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 1, nil
	}
	return rows[0].N, nil
}

func LoadActiveDays(dbPath string) (map[string]bool, error) {
	counts, err := LoadCheckCounts(dbPath)
	if err != nil {
		return nil, err
	}
	days := map[string]bool{}
	for day := range counts {
		days[day] = true
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
	if err := sqlite.Query(dbPath, "SELECT day, COUNT(*) AS n FROM box_check GROUP BY day", &rows); err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, row := range rows {
		if row.N > 0 {
			counts[row.Day] = row.N
		}
	}
	return counts, nil
}

// LoadDayChecks is the set of boxes checked on day.
func LoadDayChecks(dbPath, day string) (map[int]bool, error) {
	if err := ensure(dbPath); err != nil {
		return nil, err
	}
	var rows []struct {
		BoxID int `json:"box_id"`
	}
	if err := sqlite.Query(dbPath, "SELECT box_id FROM box_check WHERE day = "+sqlite.Quote(day), &rows); err != nil {
		return nil, err
	}
	checked := map[int]bool{}
	for _, row := range rows {
		checked[row.BoxID] = true
	}
	return checked, nil
}

// importBoxesOnce copies the outline and day files of the old boxes app into
// an empty box table, once, in one transaction.
func importBoxesOnce(dbPath string) error {
	if os.Getenv("BOXES_CONFIG") == "" && dbPath != DatabasePath() {
		return nil
	}
	var flags []struct {
		Done int `json:"boxes_imported"`
	}
	if err := sqlite.Query(dbPath, "SELECT boxes_imported FROM install WHERE id = 1", &flags); err != nil {
		return err
	}
	if len(flags) == 0 || flags[0].Done == 1 {
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
	next, err := nextBoxID(dbPath)
	if err != nil {
		return err
	}
	rows, _ := outlineRows(outline, map[string]int{}, next)
	ids := map[string]int{}
	for index, path := range outline.Paths() {
		ids[path] = rows[index].ID
	}
	days, err := legacyDayChecks(ids)
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("BEGIN IMMEDIATE;\nDELETE FROM box;\n")
	sql := outlineSQL(rows, box.Outline{}, map[int]bool{})
	b.WriteString(strings.TrimSuffix(strings.TrimPrefix(sql, "BEGIN IMMEDIATE;\n"), "COMMIT;\n"))
	b.WriteString(days)
	b.WriteString("UPDATE install SET boxes_imported = 1 WHERE id = 1;\nCOMMIT;\n")
	return execSQL(dbPath, b.String())
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

// legacyDayChecks is the insert of every check in the old day files. A
// checked id is a label path; ids maps it to the new box.
func legacyDayChecks(ids map[string]int) (string, error) {
	dir := dayStateDir()
	if dir == "" {
		return "", nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	var b strings.Builder
	for _, entry := range entries {
		name := entry.Name()
		day := strings.TrimSuffix(name, ".json")
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		if _, err := time.Parse(time.DateOnly, day); err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return "", err
		}
		var state struct {
			CheckedIDs []string `json:"checked_ids"`
		}
		if err := json.Unmarshal(data, &state); err != nil {
			return "", fmt.Errorf("read boxes day %s: %w", day, err)
		}
		boxIDs := []int{}
		for _, path := range state.CheckedIDs {
			if id, ok := ids[strings.TrimSpace(path)]; ok {
				boxIDs = append(boxIDs, id)
			}
		}
		sort.Ints(boxIDs)
		for _, id := range boxIDs {
			fmt.Fprintf(&b, "INSERT OR IGNORE INTO box_check(day, box_id) VALUES (%s, %d);\n", sqlite.Quote(day), id)
		}
	}
	return b.String(), nil
}

// ClearDayChecks unchecks every box on day.
func ClearDayChecks(dbPath, day string) error {
	if err := ensure(dbPath); err != nil {
		return err
	}
	return execSQL(dbPath, "DELETE FROM box_check WHERE day = "+sqlite.Quote(day)+";\n")
}
